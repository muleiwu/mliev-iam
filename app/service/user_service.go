package service

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/app/utils"
	"cnb.cool/mliev/iam/mliev-iam/internal/pkg/passhash"
	"gorm.io/gorm"
)

// UserService 用户业务逻辑层
type UserService struct {
	userDao     *dao.UserDao
	userRoleDao *dao.UserRoleDao
	roleDao     *dao.RoleDao
	database    *gorm.DB
}

// NewUserService 创建用户服务实例
func NewUserService(database *gorm.DB) *UserService {
	return &UserService{
		userDao:     dao.NewUserDao(database),
		userRoleDao: dao.NewUserRoleDao(database),
		roleDao:     dao.NewRoleDao(database),
		database:    database,
	}
}

// GenerateAvailableUsername 生成符合账户格式且未被占用的内部用户名。
func (s *UserService) GenerateAvailableUsername() (string, error) {
	return s.generateAvailableUsername(utils.GenerateUUIDv7)
}

func (s *UserService) generateAvailableUsername(generateID func() (string, error)) (string, error) {
	for i := 0; i < 5; i++ {
		id, err := generateID()
		if err != nil {
			return "", fmt.Errorf("生成用户名失败: %w", err)
		}

		token := strings.ReplaceAll(id, "-", "")
		if len(token) < 16 {
			return "", fmt.Errorf("生成用户名失败: 随机标识长度不足")
		}

		username := "u_" + token[:16]
		exists, err := s.userDao.ExistsUsername(username)
		if err != nil {
			return "", fmt.Errorf("检查用户名失败: %w", err)
		}
		if !exists {
			return username, nil
		}
	}

	return "", fmt.Errorf("生成用户名失败: 用户名冲突")
}

// CreateUser 创建用户
func (s *UserService) CreateUser(req *dto.CreateUserRequest) (*dto.UserResponse, error) {
	// 校验账户格式：禁止纯数字、禁止含 "@"（保证登录标识符判断无歧义）
	if err := utils.ValidateUsernameFormat(req.Username); err != nil {
		return nil, err
	}

	// 验证用户名是否已存在
	exists, err := s.userDao.ExistsUsername(req.Username)
	if err != nil {
		return nil, fmt.Errorf("检查用户名失败: %w", err)
	}
	if exists {
		return nil, fmt.Errorf("用户名已存在")
	}

	// 验证邮箱是否已存在
	if req.Email != "" {
		exists, err = s.userDao.ExistsEmail(req.Email)
		if err != nil {
			return nil, fmt.Errorf("检查邮箱失败: %w", err)
		}
		if exists {
			return nil, fmt.Errorf("邮箱已存在")
		}
	}

	// 规范化手机号为 E.164 并验证是否已存在
	if req.Phone != "" {
		normalized, err := utils.NormalizePhoneToE164(req.Phone, utils.DefaultPhoneRegion)
		if err != nil {
			return nil, err
		}
		req.Phone = normalized

		exists, err = s.userDao.ExistsPhone(req.Phone)
		if err != nil {
			return nil, fmt.Errorf("检查手机号失败: %w", err)
		}
		if exists {
			return nil, fmt.Errorf("手机号已存在")
		}
	}

	// 生成密码哈希
	salt, passwordHash, err := s.generatePasswordHash(req.Password)
	if err != nil {
		return nil, fmt.Errorf("生成密码哈希失败: %w", err)
	}

	// 处理可选字段：空字符串转为 nil
	var email, phone *string
	if req.Email != "" {
		email = &req.Email
	}
	if req.Phone != "" {
		phone = &req.Phone
	}

	// 创建用户对象
	user := &model.User{
		Username:     req.Username,
		Email:        email,
		Phone:        phone,
		Nickname:     req.Nickname,
		RealName:     req.RealName,
		Avatar:       req.Avatar,
		Salt:         salt,
		PasswordHash: passwordHash,
		Status:       model.UserStatusActive,
		IsActivated:  true,
		Remark:       req.Remark,
		Attributes:   filterCreateAttributes(req.Attributes),
	}

	// 保存到数据库
	if err := s.userDao.Create(user); err != nil {
		return nil, err
	}

	// 转换为响应DTO
	response := dto.ConvertToUserResponse(user)
	return &response, nil
}

// reservedCreateAttributeKeys 创建用户时禁止外部写入的扩展属性键。
// 邮箱/手机验证状态只能经验证流程置位，避免导入数据篡改 verified 语义。
var reservedCreateAttributeKeys = map[string]struct{}{
	"email_verified":        {},
	"phone_number_verified": {},
}

// filterCreateAttributes 过滤保留键后返回可安全落库的扩展属性；输入为空或过滤后为空时返回 nil。
func filterCreateAttributes(attrs map[string]interface{}) model.JSONObject {
	if len(attrs) == 0 {
		return nil
	}
	filtered := make(model.JSONObject, len(attrs))
	for k, v := range attrs {
		if _, reserved := reservedCreateAttributeKeys[k]; reserved {
			continue
		}
		filtered[k] = v
	}
	if len(filtered) == 0 {
		return nil
	}
	return filtered
}

// GetUserByID 根据ID获取用户
func (s *UserService) GetUserByID(id uint) (*dto.UserResponse, error) {
	user, err := s.userDao.GetByID(id)
	if err != nil {
		return nil, err
	}

	response := dto.ConvertToUserResponse(user)
	return &response, nil
}

// GetUserModelByID 根据ID获取用户模型（返回Model，供OIDC使用）
func (s *UserService) GetUserModelByID(id uint) (*model.User, error) {
	user, err := s.userDao.GetByID(id)
	if err != nil {
		return nil, err
	}

	return user, nil
}

// UpdateUser 更新用户信息
func (s *UserService) UpdateUser(id uint, req *dto.UpdateUserRequest) (*dto.UserResponse, error) {
	// 获取用户
	user, err := s.userDao.GetByID(id)
	if err != nil {
		return nil, err
	}

	// 验证邮箱是否已存在（排除自己）
	if req.Email != "" {
		// 检查是否与当前值不同
		needUpdate := true
		if user.Email != nil && *user.Email == req.Email {
			needUpdate = false
		}

		if needUpdate {
			exists, err := s.userDao.ExistsEmail(req.Email, id)
			if err != nil {
				return nil, fmt.Errorf("检查邮箱失败: %w", err)
			}
			if exists {
				return nil, fmt.Errorf("邮箱已存在")
			}
			user.Email = &req.Email
		}
	} else {
		// 空字符串表示清空邮箱
		user.Email = nil
	}

	// 规范化手机号为 E.164 并验证是否已存在（排除自己）
	if req.Phone != "" {
		normalized, err := utils.NormalizePhoneToE164(req.Phone, utils.DefaultPhoneRegion)
		if err != nil {
			return nil, err
		}
		req.Phone = normalized

		// 检查是否与当前值不同
		needUpdate := true
		if user.Phone != nil && *user.Phone == req.Phone {
			needUpdate = false
		}

		if needUpdate {
			exists, err := s.userDao.ExistsPhone(req.Phone, id)
			if err != nil {
				return nil, fmt.Errorf("检查手机号失败: %w", err)
			}
			if exists {
				return nil, fmt.Errorf("手机号已存在")
			}
			user.Phone = &req.Phone
		}
	} else {
		// 空字符串表示清空手机号
		user.Phone = nil
	}

	// 更新字段
	if req.Nickname != "" {
		user.Nickname = req.Nickname
	}
	if req.RealName != "" {
		user.RealName = req.RealName
	}
	if req.Avatar != "" {
		user.Avatar = req.Avatar
	}
	if req.Remark != "" {
		user.Remark = req.Remark
	}
	if req.Attributes != nil {
		user.Attributes = req.Attributes
	}
	if req.IsActivated != nil {
		user.IsActivated = *req.IsActivated
	}
	if req.Status != nil {
		user.Status = *req.Status
	}
	if req.EmailVerified != nil {
		if *req.EmailVerified {
			if user.EmailVerifiedAt == nil {
				now := time.Now()
				user.EmailVerifiedAt = &now
			}
		} else {
			user.EmailVerifiedAt = nil
		}
	}
	if req.PhoneVerified != nil {
		if *req.PhoneVerified {
			if user.PhoneVerifiedAt == nil {
				now := time.Now()
				user.PhoneVerifiedAt = &now
			}
		} else {
			user.PhoneVerifiedAt = nil
		}
	}

	// 保存更新
	if err := s.userDao.Update(user); err != nil {
		return nil, err
	}

	response := dto.ConvertToUserResponse(user)
	return &response, nil
}

// DeleteUser 删除用户
func (s *UserService) DeleteUser(id uint) error {
	// 检查用户是否存在
	_, err := s.userDao.GetByID(id)
	if err != nil {
		return err
	}

	return s.userDao.Delete(id)
}

// GetUserList 获取用户列表
func (s *UserService) GetUserList(req *dto.UserQueryRequest) (*dto.UserListResponse, error) {
	users, total, err := s.userDao.List(req)
	if err != nil {
		return nil, err
	}

	// 转换为响应DTO
	userResponses := dto.ConvertToUserResponses(users)

	// 设置默认分页参数
	page := req.Page
	if page < 1 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize < 1 {
		pageSize = 10
	}

	return &dto.UserListResponse{
		Total: int(total),
		Page:  page,
		Size:  pageSize,
		Items: userResponses,
	}, nil
}

// UpdateUserStatus 更新用户状态
func (s *UserService) UpdateUserStatus(id uint, req *dto.UserStatusUpdateRequest) error {
	// 检查用户是否存在
	_, err := s.userDao.GetByID(id)
	if err != nil {
		return err
	}

	// 更新状态
	if err := s.userDao.UpdateStatus(id, req.Status); err != nil {
		return err
	}

	// 如果有备注，更新备注
	if req.Remark != "" {
		updates := map[string]interface{}{
			"remark": req.Remark,
		}
		if err := s.userDao.UpdateById(id, updates); err != nil {
			return fmt.Errorf("更新备注失败: %w", err)
		}
	}

	return nil
}

// ChangePassword 修改密码
func (s *UserService) ChangePassword(id uint, req *dto.ChangePasswordRequest) error {
	// 获取用户
	user, err := s.userDao.GetByID(id)
	if err != nil {
		return err
	}

	// 验证旧密码
	if !s.verifyPassword(req.OldPassword, user.Salt, user.PasswordHash) {
		return fmt.Errorf("旧密码错误")
	}

	// 生成新密码哈希
	salt, passwordHash, err := s.generatePasswordHash(req.NewPassword)
	if err != nil {
		return fmt.Errorf("生成密码哈希失败: %w", err)
	}

	// 更新密码
	updates := map[string]interface{}{
		"salt":            salt,
		"password_hash":   passwordHash,
		"password_expiry": time.Now().AddDate(0, 3, 0), // 3个月后过期
	}

	return s.userDao.UpdateById(id, updates)
}

// BatchOperation 批量操作用户
func (s *UserService) BatchOperation(req *dto.BatchUserRequest) error {
	if len(req.UserIDs) == 0 {
		return fmt.Errorf("用户ID列表不能为空")
	}

	switch req.Action {
	case "activate":
		return s.userDao.BatchUpdateStatus(req.UserIDs, model.UserStatusActive)
	case "deactivate", "disable":
		return s.userDao.BatchUpdateStatus(req.UserIDs, model.UserStatusDisabled)
	case "delete":
		return s.userDao.BatchDelete(req.UserIDs)
	default:
		return fmt.Errorf("不支持的操作类型: %s", req.Action)
	}
}

// ResetPassword 重置密码（管理员操作）
func (s *UserService) ResetPassword(id uint, newPassword string) error {
	// 检查用户是否存在
	_, err := s.userDao.GetByID(id)
	if err != nil {
		return err
	}

	// 生成新密码哈希
	salt, passwordHash, err := s.generatePasswordHash(newPassword)
	if err != nil {
		return fmt.Errorf("生成密码哈希失败: %w", err)
	}

	// 更新密码
	updates := map[string]interface{}{
		"salt":            salt,
		"password_hash":   passwordHash,
		"password_expiry": time.Now().AddDate(0, 3, 0), // 3个月后过期
		"failure_count":   0,                           // 重置失败次数
	}

	return s.userDao.UpdateById(id, updates)
}

// GetUsersByDepartment 根据部门获取用户列表
func (s *UserService) GetUsersByDepartment(departmentID uint) ([]dto.UserResponse, error) {
	users, err := s.userDao.GetUsersByDepartment(departmentID)
	if err != nil {
		return nil, err
	}

	return dto.ConvertToUserResponses(users), nil
}

// GetUserDepartments 获取用户的部门列表
func (s *UserService) GetUserDepartments(userID uint) ([]*dto.UserDepartmentResponseDto, error) {
	// 使用 UserDepartmentDao 获取用户的部门关系
	userDeptDao := dao.NewUserDepartmentDao(s.database)
	userDepts, err := userDeptDao.FindByUserWithDepartment(userID)
	if err != nil {
		return nil, fmt.Errorf("获取用户部门失败: %w", err)
	}

	return dto.ToUserDepartmentResponseDtoList(userDepts), nil
}

// GetActiveUsersCount 获取活跃用户数量
func (s *UserService) GetActiveUsersCount() (int64, error) {
	return s.userDao.GetActiveUsersCount()
}

// generatePasswordHash 生成密码哈希。
// 新记录统一经 passhash（argon2id）生成；salt 字段保留为空字符串（哈希自带 salt）。
// 返回值保持既有调用方签名 (salt, hash, err) 不变。
func (s *UserService) generatePasswordHash(password string) (salt, hash string, err error) {
	h, err := passhash.Hash(password)
	if err != nil {
		return "", "", fmt.Errorf("生成密码哈希失败: %w", err)
	}
	return "", h, nil
}

// GeneratePasswordHash 生成密码哈希（公开方法）
func (s *UserService) GeneratePasswordHash(password string) (salt, hash string, err error) {
	return s.generatePasswordHash(password)
}

// verifyAndUpgrade 验证密码并在需要时返回升级后的新哈希。
//   - 标准编码格式（argon2id / bcrypt / pbkdf2）：交给 passhash 自动分派；
//     若源算法非 target(argon2id)，newHash 非空表示应持久化升级。
//   - 历史 SHA-256 + salt 非标准格式：回落到旧校验；成功后用 argon2id 重哈希。
//
// 返回 ok=是否匹配；newHash!="" 表示调用方应把新哈希写回（salt 置空）。
func (s *UserService) verifyAndUpgrade(password, salt, hash string) (ok bool, newHash string) {
	if passhash.IsRecognized(hash) {
		return passhash.VerifyAndUpgrade(password, hash)
	}
	// 历史 SHA-256 + salt 格式回落。
	h := sha256.New()
	h.Write([]byte(password + salt))
	if hex.EncodeToString(h.Sum(nil)) != hash {
		return false, ""
	}
	// 校验通过，升级为 argon2id；升级失败不影响本次登录（下次再试）。
	if upgraded, err := passhash.Hash(password); err == nil {
		return true, upgraded
	}
	return true, ""
}

// verifyPassword 验证密码（仅判定是否匹配，不处理升级）。
func (s *UserService) verifyPassword(password, salt, hash string) bool {
	ok, _ := s.verifyAndUpgrade(password, salt, hash)
	return ok
}

// ValidateUserForLogin 验证用户登录。
// account 可以是 用户名 / 手机号 / 邮箱，按输入自动判断类型查找用户。
// 因用户账户(username) 禁止全数字、禁止含 "@"，三类标识符不会冲突。
func (s *UserService) ValidateUserForLogin(account, password string) (*dto.UserResponse, error) {
	// 根据标识符类型获取用户
	var (
		user *model.User
		err  error
	)
	switch utils.DetectLoginIdentifier(account) {
	case utils.LoginIdentifierPhone:
		user, err = s.userDao.GetByPhone(account)
	case utils.LoginIdentifierEmail:
		user, err = s.userDao.GetByEmail(account)
	default:
		user, err = s.userDao.GetByUsername(account)
	}
	if err != nil {
		// 本地查无此人：尝试旧系统迁移登录（渐进式过渡 / 一次性导入的旧用户）。
		// 校验通过会即时迁移进主 users 表并返回；不匹配则维持原错误。
		// 用户名 / 手机号 / 邮箱三类标识符均支持旧系统迁移登录。
		if resp, legacyErr := NewLegacyLoginService(s.database).TryLegacyLogin(account, password); legacyErr == nil {
			return resp, nil
		}
		return nil, fmt.Errorf("用户名或密码错误")
	}

	// 检查用户是否可以登录
	if !user.CanLogin() {
		return nil, fmt.Errorf("用户账户已禁用")
	}

	// 验证密码（同时拿到可能的升级哈希）
	ok, newHash := s.verifyAndUpgrade(password, user.Salt, user.PasswordHash)
	if !ok {
		return nil, fmt.Errorf("用户名或密码错误")
	}

	// 透明迁移：旧算法（bcrypt / 历史 SHA-256+salt）登录成功后升级为 argon2id。
	// 失败不影响本次登录（下次登录还会再尝试）。
	if newHash != "" {
		_ = s.userDao.UpdateById(user.ID, map[string]interface{}{
			"salt":          "",
			"password_hash": newHash,
		})
	}

	response := dto.ConvertToUserResponse(user)
	return &response, nil
}

// ErrLoginAccountNotFound 登录标识符在主 users 表中查无此人（区别于禁用/未验证等失败）。
// 错误消息经 fmt.Errorf("%s%w", label, ...) 包装成"手机号未注册"等提示；
// 调用方用 errors.Is 判断是否可回退到迁移数据认领。
var ErrLoginAccountNotFound = errors.New("未注册")

// GetVerifiedUserForCodeLogin 根据已验证邮箱或手机号获取可登录用户。
func (s *UserService) GetVerifiedUserForCodeLogin(verificationType VerificationType, receiver string) (*dto.UserResponse, error) {
	var (
		user *model.User
		err  error
	)
	identifierLabel := "手机号"

	switch verificationType {
	case VerificationTypeEmail:
		identifierLabel = "邮箱"
		user, err = s.userDao.GetByEmail(receiver)
	case VerificationTypePhone:
		user, err = s.userDao.GetByPhone(receiver)
	default:
		return nil, fmt.Errorf("不支持的验证码登录类型")
	}

	if err != nil {
		return nil, fmt.Errorf("%s%w", identifierLabel, ErrLoginAccountNotFound)
	}

	if !user.CanLogin() {
		return nil, fmt.Errorf("用户账户已禁用")
	}

	if verificationType == VerificationTypeEmail && user.EmailVerifiedAt == nil {
		return nil, fmt.Errorf("邮箱未验证，无法使用验证码登录")
	}
	if verificationType == VerificationTypePhone && user.PhoneVerifiedAt == nil {
		return nil, fmt.Errorf("手机号未验证，无法使用验证码登录")
	}

	response := dto.ConvertToUserResponse(user)
	return &response, nil
}

// MarkContactVerified 把邮箱/手机标记为已验证。
// 仅供已通过验证码等方式证明联系方式归属的流程调用（如注册时验证码校验通过后）。
func (s *UserService) MarkContactVerified(userID uint, emailVerified, phoneVerified bool) error {
	if !emailVerified && !phoneVerified {
		return nil
	}
	now := time.Now()
	updates := map[string]interface{}{}
	if emailVerified {
		updates["email_verified_at"] = now
	}
	if phoneVerified {
		updates["phone_verified_at"] = now
	}
	return s.userDao.UpdateById(userID, updates)
}

// ============= RBAC权限相关方法 =============

// CheckUserURLPermission 检查用户是否有URL访问权限
func (s *UserService) CheckUserURLPermission(userID uint, path, method string) (bool, error) {
	// 获取用户角色
	userRoles, err := s.userRoleDao.GetByUserID(userID)
	if err != nil {
		return false, err
	}

	if len(userRoles) == 0 {
		return false, nil
	}

	// 获取角色ID列表
	roleIDs := make([]uint, len(userRoles))
	for i, ur := range userRoles {
		if !ur.IsActive {
			continue
		}
		roleIDs[i] = ur.RoleID
	}

	// 检查角色是否有相应的URL权限
	permissionDao := dao.NewPermissionDao(s.database)
	hasPermission, err := permissionDao.CheckRoleURLPermission(roleIDs, path, method)
	if err != nil {
		return false, err
	}

	return hasPermission, nil
}

// CheckUserPermission 检查用户是否有特定权限
func (s *UserService) CheckUserPermission(userID uint, permissionCode string) (bool, error) {
	// 获取用户角色
	userRoles, err := s.userRoleDao.GetByUserID(userID)
	if err != nil {
		return false, err
	}

	if len(userRoles) == 0 {
		return false, nil
	}

	// 获取活跃角色ID列表
	roleIDs := make([]uint, 0, len(userRoles))
	for _, ur := range userRoles {
		if ur.IsActive {
			roleIDs = append(roleIDs, ur.RoleID)
		}
	}

	if len(roleIDs) == 0 {
		return false, nil
	}

	// 检查角色是否有相应的权限
	permissionDao := dao.NewPermissionDao(s.database)
	hasPermission, err := permissionDao.CheckRolePermission(roleIDs, permissionCode)
	if err != nil {
		return false, err
	}

	return hasPermission, nil
}

// GetUserRoles 获取用户角色列表
func (s *UserService) GetUserRoles(userID uint) ([]model.Role, error) {
	// 获取用户角色关联
	userRoles, err := s.userRoleDao.GetByUserID(userID)
	if err != nil {
		return nil, err
	}

	if len(userRoles) == 0 {
		return []model.Role{}, nil
	}

	// 获取活跃角色ID列表
	roleIDs := make([]uint, 0, len(userRoles))
	for _, ur := range userRoles {
		if ur.IsActive {
			roleIDs = append(roleIDs, ur.RoleID)
		}
	}

	if len(roleIDs) == 0 {
		return []model.Role{}, nil
	}

	// 获取角色详情
	roles, err := s.roleDao.GetByIDs(roleIDs)
	if err != nil {
		return nil, err
	}

	return roles, nil
}

// ============= 用户角色管理方法 =============

// AssignUserRole 为用户分配角色
func (s *UserService) AssignUserRole(userID, roleID, assignedBy uint, req *dto.AssignUserRoleRequest) error {
	return s.userRoleDao.AssignRole(userID, roleID, assignedBy, req)
}

// RemoveUserRole 移除用户角色
func (s *UserService) RemoveUserRole(userID, roleID uint) error {
	return s.userRoleDao.RemoveRole(userID, roleID)
}

// GetUserWithRoles 获取用户及其角色信息
func (s *UserService) GetUserWithRoles(userID uint) (*dto.UserResponse, error) {
	// 获取用户信息
	user, err := s.userDao.GetByID(userID)
	if err != nil {
		return nil, err
	}

	// 构建响应
	response := dto.ConvertToUserResponse(user)

	// 这里可以添加角色信息到响应中
	// 获取用户角色信息如果需要的话
	// userRoles, _ := s.userRoleDao.GetByUserID(userID)
	// response.Roles = userRoles // 需要在DTO中添加Roles字段

	return &response, nil
}

// BatchAssignUserRoles 批量分配角色给用户
func (s *UserService) BatchAssignUserRoles(userID uint, roleIDs []uint, assignedBy uint) error {
	return s.userRoleDao.BatchAssignRoles(userID, roleIDs, assignedBy)
}

// BatchRemoveUserRoles 批量移除用户角色
func (s *UserService) BatchRemoveUserRoles(userID uint, roleIDs []uint) error {
	return s.userRoleDao.BatchRemoveRoles(userID, roleIDs)
}

// SetUserPrimaryRole 设置用户主要角色
func (s *UserService) SetUserPrimaryRole(userID, roleID uint) error {
	return s.userRoleDao.SetPrimaryRole(userID, roleID)
}

// GetUserPermissions 获取用户权限列表
func (s *UserService) GetUserPermissions(userID uint) ([]*model.Permission, error) {
	// 获取用户角色
	userRoles, err := s.userRoleDao.GetByUserID(userID)
	if err != nil {
		return nil, err
	}

	if len(userRoles) == 0 {
		return []*model.Permission{}, nil
	}

	// 获取活跃角色ID列表
	roleIDs := make([]uint, 0, len(userRoles))
	for _, ur := range userRoles {
		if ur.IsActive {
			roleIDs = append(roleIDs, ur.RoleID)
		}
	}

	if len(roleIDs) == 0 {
		return []*model.Permission{}, nil
	}

	// 获取角色权限
	permissionDao := dao.NewPermissionDao(s.database)
	permissions, err := permissionDao.GetByRoleIDs(roleIDs)
	if err != nil {
		return nil, err
	}

	return permissions, nil
}

// GetUserMenuTree 获取用户菜单树
func (s *UserService) GetUserMenuTree(userID uint) ([]*model.Permission, error) {
	// 获取用户所有权限
	permissions, err := s.GetUserPermissions(userID)
	if err != nil {
		return nil, err
	}

	// 过滤出菜单权限并返回树形结构
	menuPermissions := make([]*model.Permission, 0)
	for _, perm := range permissions {
		if perm.Type == model.PermissionTypeMenu && perm.ShowInMenu {
			menuPermissions = append(menuPermissions, perm)
		}
	}

	return menuPermissions, nil
}
