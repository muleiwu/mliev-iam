package service

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
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

// CreateUser 创建用户
func (s *UserService) CreateUser(req *dto.CreateUserRequest) (*dto.UserResponse, error) {
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

	// 验证手机号是否已存在
	if req.Phone != "" {
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

	// 创建用户对象
	user := &model.User{
		Username:     req.Username,
		Email:        req.Email,
		Phone:        req.Phone,
		Nickname:     req.Nickname,
		RealName:     req.RealName,
		Avatar:       req.Avatar,
		Password:     req.Password, // 这里存储的是明文密码，实际应该清除
		Salt:         salt,
		PasswordHash: passwordHash,
		Status:       model.UserStatusInactive,
		IsActivated:  false,
		IsVerified:   false,
		DepartmentID: req.DepartmentID,
		Remark:       req.Remark,
	}

	// 保存到数据库
	if err := s.userDao.Create(user); err != nil {
		return nil, err
	}

	// 清除明文密码
	user.Password = ""

	// 转换为响应DTO
	response := dto.ConvertToUserResponse(user)
	return &response, nil
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
	if req.Email != "" && req.Email != user.Email {
		exists, err := s.userDao.ExistsEmail(req.Email, id)
		if err != nil {
			return nil, fmt.Errorf("检查邮箱失败: %w", err)
		}
		if exists {
			return nil, fmt.Errorf("邮箱已存在")
		}
		user.Email = req.Email
	}

	// 验证手机号是否已存在（排除自己）
	if req.Phone != "" && req.Phone != user.Phone {
		exists, err := s.userDao.ExistsPhone(req.Phone, id)
		if err != nil {
			return nil, fmt.Errorf("检查手机号失败: %w", err)
		}
		if exists {
			return nil, fmt.Errorf("手机号已存在")
		}
		user.Phone = req.Phone
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
	if req.DepartmentID != nil {
		user.DepartmentID = req.DepartmentID
	}
	if req.Remark != "" {
		user.Remark = req.Remark
	}
	if req.Attributes != nil {
		user.Attributes = req.Attributes
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
	case "deactivate":
		return s.userDao.BatchUpdateStatus(req.UserIDs, model.UserStatusDisabled)
	case "lock":
		return s.userDao.BatchUpdateStatus(req.UserIDs, model.UserStatusLocked)
	case "unlock":
		return s.userDao.BatchUpdateStatus(req.UserIDs, model.UserStatusActive)
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

// GetActiveUsersCount 获取活跃用户数量
func (s *UserService) GetActiveUsersCount() (int64, error) {
	return s.userDao.GetActiveUsersCount()
}

// generatePasswordHash 生成密码哈希
func (s *UserService) generatePasswordHash(password string) (salt, hash string, err error) {
	// 生成盐值
	saltBytes := make([]byte, 16)
	if _, err := rand.Read(saltBytes); err != nil {
		return "", "", err
	}
	salt = hex.EncodeToString(saltBytes)

	// 生成密码哈希
	h := sha256.New()
	h.Write([]byte(password + salt))
	hash = hex.EncodeToString(h.Sum(nil))

	return salt, hash, nil
}

// verifyPassword 验证密码
func (s *UserService) verifyPassword(password, salt, hash string) bool {
	h := sha256.New()
	h.Write([]byte(password + salt))
	computedHash := hex.EncodeToString(h.Sum(nil))
	return computedHash == hash
}

// ValidateUserForLogin 验证用户登录
func (s *UserService) ValidateUserForLogin(username, password string) (*dto.UserResponse, error) {
	// 根据用户名获取用户
	user, err := s.userDao.GetByUsername(username)
	if err != nil {
		return nil, fmt.Errorf("用户名或密码错误")
	}

	// 检查用户是否可以登录
	if !user.CanLogin() {
		return nil, fmt.Errorf("用户账户已禁用或锁定")
	}

	// 验证密码
	if !s.verifyPassword(password, user.Salt, user.PasswordHash) {
		// 增加失败次数
		user.IncrementFailureCount()

		// 如果失败次数超过限制，锁定账户
		if user.FailureCount >= 5 {
			user.Lock()
		}

		s.userDao.Update(user)
		return nil, fmt.Errorf("用户名或密码错误")
	}

	response := dto.ConvertToUserResponse(user)
	return &response, nil
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
