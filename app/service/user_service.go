package service

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"cnb.cool/mliev/examples/go-web/app/dao"
	"cnb.cool/mliev/examples/go-web/app/dto"
	"cnb.cool/mliev/examples/go-web/app/model"
	"gorm.io/gorm"
)

// UserService 用户业务逻辑层
type UserService struct {
	userDao  *dao.UserDao
	database *gorm.DB
}

// NewUserService 创建用户服务实例
func NewUserService(database *gorm.DB) *UserService {
	return &UserService{
		userDao:  dao.NewUserDao(database),
		database: database,
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

	// 更新登录信息
	user.UpdateLastLogin("") // IP地址在controller层获取
	if err := s.userDao.Update(user); err != nil {
		// 登录信息更新失败不影响登录
		fmt.Printf("更新登录信息失败: %v\n", err)
	}

	response := dto.ConvertToUserResponse(user)
	return &response, nil
}
