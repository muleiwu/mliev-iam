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
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// dicebearAvatarPrefix 预设头像服务地址前缀（与 frontend-oidc AvatarPicker 保持一致）。
const dicebearAvatarPrefix = "https://dicebear.mliev.com/"

// isAllowedAvatarURL 校验自助设置的头像地址：仅允许对象存储附件（/attachments/{uuid}）
// 或 DiceBear 预设头像服务，防止把任意外部 URL 存为头像。
func isAllowedAvatarURL(avatar string) bool {
	if rest, ok := strings.CutPrefix(avatar, "/attachments/"); ok {
		_, err := uuid.Parse(rest)
		return err == nil
	}
	return strings.HasPrefix(avatar, dicebearAvatarPrefix)
}

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

// CreateUnactivatedUser provisions a credential-free account for reviewed
// directory imports. It cannot log in until explicitly activated later.
func (s *UserService) CreateUnactivatedUser(realName string) (*dto.UserResponse, error) {
	username, err := s.GenerateAvailableUsername()
	if err != nil {
		return nil, err
	}
	return s.CreateUnactivatedUserWithUsername(username, realName)
}

// CreateUnactivatedUserWithUsername provisions an explicitly named account
// without credentials. The caller can include provisioning in its transaction.
func (s *UserService) CreateUnactivatedUserWithUsername(username, realName string) (*dto.UserResponse, error) {
	if err := utils.ValidateUsernameFormat(username); err != nil {
		return nil, err
	}
	if strings.TrimSpace(realName) == "" || len([]rune(realName)) > 50 {
		return nil, fmt.Errorf("姓名须为1至50字")
	}
	user := model.User{Username: username, RealName: realName, Nickname: realName, Status: model.UserStatusActive, IsActivated: false}
	if err := s.userDao.Create(&user); err != nil {
		return nil, err
	}
	response := dto.ConvertToUserResponse(&user)
	return &response, nil
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
	// 账户允许纯数字；"+" 和 "@" 分别留给手机号和邮箱登录。
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
	revokeAccess := false
	if req.IsActivated != nil {
		user.IsActivated = *req.IsActivated
		revokeAccess = !*req.IsActivated
	}
	if req.Status != nil {
		user.Status = *req.Status
		if *req.Status == model.UserStatusDisabled {
			user.IsActivated = false
			revokeAccess = true
		}
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

	// 保存更新；停用账号时必须与会话、OIDC Token 撤销处于同一事务。
	if revokeAccess {
		if err := s.database.Transaction(func(tx *gorm.DB) error {
			if err := dao.NewUserDao(tx).Update(user); err != nil {
				return err
			}
			return revokeUserAccess(tx, []uint{id})
		}); err != nil {
			return nil, err
		}
		user, err = s.userDao.GetByID(id)
		if err != nil {
			return nil, err
		}
	} else if err := s.userDao.Update(user); err != nil {
		return nil, err
	}

	response := dto.ConvertToUserResponse(user)
	return &response, nil
}

// UpdateMyProfile 个人中心自助更新（仅白名单字段，空值表示不修改）。
// 不复用 UpdateUser：其空 Email/Phone 语义为"清空该字段"，自助场景会造成数据丢失。
func (s *UserService) UpdateMyProfile(id uint, req *dto.UpdateMyProfileRequest) (*dto.UserResponse, error) {
	user, err := s.userDao.GetByID(id)
	if err != nil {
		return nil, err
	}

	if req.Nickname != "" {
		user.Nickname = req.Nickname
	}
	if req.RealName != "" {
		user.RealName = req.RealName
	}
	if req.Avatar != "" {
		if !isAllowedAvatarURL(req.Avatar) {
			return nil, errors.New("头像地址不合法")
		}
		user.Avatar = req.Avatar
	}

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

	return s.database.Transaction(func(tx *gorm.DB) error {
		if err := RevokeLoginSessions(tx, id, "", "", "user_deleted", nil); err != nil {
			return err
		}
		return dao.NewUserDao(tx).Delete(id)
	})
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
	return s.database.Transaction(func(tx *gorm.DB) error {
		userDao := dao.NewUserDao(tx)
		if _, err := userDao.GetByID(id); err != nil {
			return err
		}

		if err := userDao.UpdateStatus(id, req.Status); err != nil {
			return err
		}

		if req.Status == model.UserStatusDisabled {
			if err := revokeUserAccess(tx, []uint{id}); err != nil {
				return err
			}
		}

		if req.Remark != "" {
			if err := userDao.UpdateById(id, map[string]interface{}{"remark": req.Remark}); err != nil {
				return fmt.Errorf("更新备注失败: %w", err)
			}
		}
		return nil
	})
}

// ChangePassword 修改密码
func (s *UserService) ChangePassword(id uint, req *dto.ChangePasswordRequest, currentSessionIDs ...string) error {
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

	return s.updatePasswordAndRevoke(id, updates, "password_changed", currentSessionIDs, &id)
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
		return s.database.Transaction(func(tx *gorm.DB) error {
			if err := dao.NewUserDao(tx).BatchUpdateStatus(req.UserIDs, model.UserStatusDisabled); err != nil {
				return err
			}
			return revokeUserAccess(tx, req.UserIDs)
		})
	case "delete":
		return s.database.Transaction(func(tx *gorm.DB) error {
			for _, id := range req.UserIDs {
				if err := RevokeLoginSessions(tx, id, "", "", "user_deleted", nil); err != nil {
					return err
				}
			}
			return dao.NewUserDao(tx).BatchDelete(req.UserIDs)
		})
	default:
		return fmt.Errorf("不支持的操作类型: %s", req.Action)
	}
}

// revokeUserAccess 让当前及历史内部会话失效，并撤销用户全部 OIDC Token。
// 调用方负责在同一事务中先写入业务状态（如 status=disabled）。
func revokeUserAccess(database *gorm.DB, userIDs []uint) error {
	if len(userIDs) == 0 {
		return nil
	}
	for _, id := range userIDs {
		if err := RevokeLoginSessions(database, id, "", "", "user_disabled", nil); err != nil {
			return err
		}
	}
	now := time.Now()
	if err := database.Model(&model.User{}).Where("id IN ?", userIDs).Updates(map[string]interface{}{
		"is_activated":       false,
		"session_revoked_at": now,
	}).Error; err != nil {
		return fmt.Errorf("撤销用户会话失败: %w", err)
	}
	if err := dao.NewOIDCTokenDAO(database).RevokeByUsers(userIDs); err != nil {
		return fmt.Errorf("撤销用户OIDC令牌失败: %w", err)
	}
	return nil
}

// ResetPassword 重置密码（管理员操作）
func (s *UserService) ResetPassword(id uint, newPassword string, actorIDs ...uint) error {
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

	var actor *uint
	if len(actorIDs) > 0 {
		actor = &actorIDs[0]
	}
	return s.updatePasswordAndRevoke(id, updates, "password_reset", nil, actor)
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

// RecordSuccessfulLogin 原子记录一次成功登录，避免并发登录时覆盖累计次数。
func (s *UserService) RecordSuccessfulLogin(userID uint, clientIP string) error {
	if err := s.userDao.UpdateById(userID, map[string]interface{}{
		"login_count":   gorm.Expr("login_count + ?", 1),
		"last_login_at": time.Now(),
		"last_login_ip": clientIP,
	}); err != nil {
		return fmt.Errorf("记录用户登录信息失败: %w", err)
	}
	return nil
}

// ValidateUserForLogin 验证用户登录。
// account 可以是 用户名 / 手机号 / 邮箱，按输入自动判断类型查找用户。
// 裸数字按账户名查询，手机号由前端发送带 "+" 的 E.164 格式。
func (s *UserService) ValidateUserForLogin(account, password string) (*dto.UserResponse, error) {
	// 根据标识符类型获取用户
	var (
		user *model.User
		err  error
	)
	account = strings.TrimSpace(account)
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

	// 无密码账号（验证码认领迁移/社交注册）：主表没有哈希，密码校验必败。
	// 若其关联了已迁移的旧用户记录，用旧系统凭据（legacy_users 里的旧哈希）走脚本校验；
	// 通过则登录，并把本次明文密码以 argon2id 补进主表，之后走原生密码路径。
	if user.PasswordHash == "" {
		if !NewLegacyLoginService(s.database).VerifyMigratedUserPassword(user.ID, account, password) {
			return nil, fmt.Errorf("用户名或密码错误")
		}
		if _, newHash, err := s.generatePasswordHash(password); err == nil {
			_ = s.userDao.UpdateById(user.ID, map[string]interface{}{
				"salt":          "",
				"password_hash": newHash,
			})
		}
		response := dto.ConvertToUserResponse(user)
		return &response, nil
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

// GetVerifiedUserForPasswordReset 根据已验证邮箱或手机号获取可找回密码的用户。
// 与 GetVerifiedUserForCodeLogin 逻辑一致，但错误文案面向找回密码场景。
func (s *UserService) GetVerifiedUserForPasswordReset(verificationType VerificationType, receiver string) (*dto.UserResponse, error) {
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
		return nil, fmt.Errorf("不支持的找回密码验证类型")
	}

	if err != nil {
		return nil, fmt.Errorf("该%s未绑定任何账号", identifierLabel)
	}

	if !user.CanLogin() {
		return nil, fmt.Errorf("用户账户已禁用")
	}

	if verificationType == VerificationTypeEmail && user.EmailVerifiedAt == nil {
		return nil, fmt.Errorf("邮箱未验证，无法用于找回密码，请联系管理员")
	}
	if verificationType == VerificationTypePhone && user.PhoneVerifiedAt == nil {
		return nil, fmt.Errorf("手机号未验证，无法用于找回密码，请联系管理员")
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

// ContactInUse 检查邮箱/手机号是否已被其他用户占用（排除 excludeUserID 自身）。
func (s *UserService) ContactInUse(verificationType VerificationType, value string, excludeUserID uint) (bool, error) {
	switch verificationType {
	case VerificationTypeEmail:
		return s.userDao.ExistsEmail(value, excludeUserID)
	case VerificationTypePhone:
		return s.userDao.ExistsPhone(value, excludeUserID)
	default:
		return false, fmt.Errorf("不支持的联系方式类型")
	}
}

// UpdateVerifiedContact 更新已通过验证码验证归属的联系方式。
// 调用方须先完成验证码校验；value 须已规范化（手机号 E.164、邮箱去空白）。
// 单次 UpdateById 同时写入新值与 verified_at，避免出现"新值未验证"的中间态。
func (s *UserService) UpdateVerifiedContact(userID uint, verificationType VerificationType, value string) error {
	now := time.Now()
	var updates map[string]interface{}

	switch verificationType {
	case VerificationTypeEmail:
		exists, err := s.userDao.ExistsEmail(value, userID)
		if err != nil {
			return fmt.Errorf("检查邮箱失败: %w", err)
		}
		if exists {
			return fmt.Errorf("邮箱已存在")
		}
		updates = map[string]interface{}{"email": value, "email_verified_at": now}
	case VerificationTypePhone:
		exists, err := s.userDao.ExistsPhone(value, userID)
		if err != nil {
			return fmt.Errorf("检查手机号失败: %w", err)
		}
		if exists {
			return fmt.Errorf("手机号已存在")
		}
		updates = map[string]interface{}{"phone": value, "phone_verified_at": now}
	default:
		return fmt.Errorf("不支持的联系方式类型")
	}

	return s.userDao.UpdateById(userID, updates)
}

// SetInitialPassword 为无密码用户（社交注册）设置初始密码。
// 已有密码的用户必须走 ChangePassword 验证旧密码，防止会话劫持后直接改密。
func (s *UserService) SetInitialPassword(userID uint, newPassword string, currentSessionIDs ...string) error {
	user, err := s.userDao.GetByID(userID)
	if err != nil {
		return err
	}

	if user.PasswordHash != "" {
		return fmt.Errorf("已设置密码，请使用修改密码功能")
	}

	salt, passwordHash, err := s.generatePasswordHash(newPassword)
	if err != nil {
		return fmt.Errorf("生成密码哈希失败: %w", err)
	}

	updates := map[string]interface{}{
		"salt":            salt,
		"password_hash":   passwordHash,
		"password_expiry": time.Now().AddDate(0, 3, 0), // 3个月后过期，与 ResetPassword 一致
		"failure_count":   0,
	}

	return s.updatePasswordAndRevoke(userID, updates, "password_changed", currentSessionIDs, &userID)
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

// GetEffectiveRoleDefinitions 获取用户当前有效的角色定义，供鉴权逻辑使用。
func (s *UserService) GetEffectiveRoleDefinitions(userID uint) ([]model.Role, error) {
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

// GetUserRoleAssignments 获取用户的角色分配关系，供管理端展示和维护使用。
// 与鉴权查询不同，这里保留未激活和已过期的关系，以便管理员查看其状态。
func (s *UserService) GetUserRoleAssignments(userID uint) ([]*dto.UserRoleResponse, error) {
	userRoles, err := s.userRoleDao.GetByUserID(userID)
	if err != nil {
		return nil, err
	}

	return dto.ConvertToUserRoleResponses(userRoles), nil
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

func (s *UserService) updatePasswordAndRevoke(userID uint, updates map[string]interface{}, reason string, currentSessionIDs []string, actor *uint) error {
	return s.database.Transaction(func(tx *gorm.DB) error {
		if err := dao.LockLoginUser(tx, userID); err != nil {
			return err
		}
		except := ""
		if len(currentSessionIDs) > 0 {
			except = currentSessionIDs[0]
			if _, err := dao.NewLoginSessionDAO(tx).Valid(except, userID); err != nil {
				return err
			}
		}
		if err := dao.NewUserDao(tx).UpdateById(userID, updates); err != nil {
			return err
		}
		return RevokeLoginSessions(tx, userID, "", except, reason, actor)
	})
}
