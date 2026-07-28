package dao

import (
	"errors"
	"fmt"

	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"gorm.io/gorm"
)

// UserDao 用户数据访问层
type UserDao struct {
	database *gorm.DB
}

// NewUserDao 创建用户DAO实例
func NewUserDao(database *gorm.DB) *UserDao {
	return &UserDao{
		database: database,
	}
}

// Create 创建用户
func (dao *UserDao) Create(user *model.User) error {
	if err := dao.database.Create(user).Error; err != nil {
		return fmt.Errorf("创建用户失败: %w", err)
	}
	return nil
}

// GetByID 根据ID获取用户
func (dao *UserDao) GetByID(id uint) (*model.User, error) {
	var user model.User
	if err := dao.database.First(&user, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("用户不存在")
		}
		return nil, fmt.Errorf("查询用户失败: %w", err)
	}
	return &user, nil
}

// GetByUUID 根据UUID获取用户
func (dao *UserDao) GetByUUID(uuid string) (*model.User, error) {
	var user model.User
	if err := dao.database.Where("uuid = ?", uuid).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("用户不存在")
		}
		return nil, fmt.Errorf("查询用户失败: %w", err)
	}
	return &user, nil
}

// GetByUsername 根据用户名获取用户
func (dao *UserDao) GetByUsername(username string) (*model.User, error) {
	var user model.User
	if err := dao.database.Where("username = ?", username).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("用户不存在")
		}
		return nil, fmt.Errorf("查询用户失败: %w", err)
	}
	return &user, nil
}

// GetByEmail 根据邮箱获取用户
func (dao *UserDao) GetByEmail(email string) (*model.User, error) {
	var user model.User
	if err := dao.database.Where("email = ?", email).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("用户不存在")
		}
		return nil, fmt.Errorf("查询用户失败: %w", err)
	}
	return &user, nil
}

// GetByPhone 根据手机号获取用户
func (dao *UserDao) GetByPhone(phone string) (*model.User, error) {
	var user model.User
	if err := dao.database.Where("phone = ?", phone).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("用户不存在")
		}
		return nil, fmt.Errorf("查询用户失败: %w", err)
	}
	return &user, nil
}

// Update 更新用户
func (dao *UserDao) Update(user *model.User) error {
	if err := dao.database.Save(user).Error; err != nil {
		return fmt.Errorf("更新用户失败: %w", err)
	}
	return nil
}

// UpdateById 根据ID更新用户指定字段
func (dao *UserDao) UpdateById(id uint, updates map[string]interface{}) error {
	if err := dao.database.Model(&model.User{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return fmt.Errorf("更新用户失败: %w", err)
	}
	return nil
}

// Delete 软删除用户
func (dao *UserDao) Delete(id uint) error {
	if err := dao.database.Delete(&model.User{}, id).Error; err != nil {
		return fmt.Errorf("删除用户失败: %w", err)
	}
	return nil
}

// BatchDelete 批量软删除用户
func (dao *UserDao) BatchDelete(ids []uint) error {
	if len(ids) == 0 {
		return fmt.Errorf("用户ID列表不能为空")
	}
	if err := dao.database.Delete(&model.User{}, ids).Error; err != nil {
		return fmt.Errorf("批量删除用户失败: %w", err)
	}
	return nil
}

// List 分页查询用户列表
func (dao *UserDao) List(req *dto.UserQueryRequest) ([]model.User, int64, error) {
	var users []model.User
	var total int64

	// 构建查询条件
	query := dao.database.Model(&model.User{})

	// 添加查询条件
	if req.UUID != "" {
		query = query.Where("uuid = ?", req.UUID)
	}
	if req.Username != "" {
		query = query.Where("username LIKE ?", "%"+req.Username+"%")
	}
	if req.Email != "" {
		query = query.Where("email LIKE ?", "%"+req.Email+"%")
	}
	if req.Phone != "" {
		query = query.Where("phone LIKE ?", "%"+req.Phone+"%")
	}
	if req.RealName != "" {
		query = query.Where("real_name LIKE ?", "%"+req.RealName+"%")
	}
	if req.Status != nil {
		query = query.Where("status = ?", *req.Status)
	}
	if req.Keyword != "" {
		keyword := "%" + req.Keyword + "%"
		query = query.Where(
			"username LIKE ? OR email LIKE ? OR real_name LIKE ? OR nickname LIKE ?",
			keyword, keyword, keyword, keyword,
		)
	}

	// 获取总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("查询用户总数失败: %w", err)
	}

	// 设置默认分页参数
	page := req.Page
	if page < 1 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize < 1 {
		pageSize = 10
	}
	if pageSize > 100 {
		pageSize = 100
	}

	// 处理排序
	orderClause := dao.buildOrderClause(req.SortBy, req.SortOrder)

	// 调试日志
	fmt.Printf("UserDao.List - 排序参数: sortBy=%s, sortOrder=%s, orderClause=%s\n", req.SortBy, req.SortOrder, orderClause)

	// 分页查询
	offset := (page - 1) * pageSize
	if err := query.Offset(offset).Limit(pageSize).Order(orderClause).Find(&users).Error; err != nil {
		return nil, 0, fmt.Errorf("查询用户列表失败: %w", err)
	}

	return users, total, nil
}

// buildOrderClause 构建排序子句
func (dao *UserDao) buildOrderClause(sortBy, sortOrder string) string {
	// 验证排序字段，防止 SQL 注入
	validSortFields := map[string]string{
		"loginCount":  "login_count",
		"lastLoginAt": "last_login_at",
		"createdAt":   "created_at",
	}

	// 如果没有指定排序字段或字段无效，使用默认排序
	dbField, ok := validSortFields[sortBy]
	if !ok {
		return "created_at DESC"
	}

	// 验证排序方向
	order := "DESC"
	if sortOrder == "asc" || sortOrder == "ASC" {
		order = "ASC"
	}

	return fmt.Sprintf("%s %s", dbField, order)
}

// ExistsUsername 检查用户名是否存在
func (dao *UserDao) ExistsUsername(username string, excludeID ...uint) (bool, error) {
	var count int64
	query := dao.database.Model(&model.User{}).Where("username = ?", username)

	// 排除指定ID（用于更新时检查）
	if len(excludeID) > 0 && excludeID[0] > 0 {
		query = query.Where("id != ?", excludeID[0])
	}

	if err := query.Count(&count).Error; err != nil {
		return false, fmt.Errorf("检查用户名是否存在失败: %w", err)
	}
	return count > 0, nil
}

// ExistsEmail 检查邮箱是否存在
func (dao *UserDao) ExistsEmail(email string, excludeID ...uint) (bool, error) {
	if email == "" {
		return false, nil
	}

	var count int64
	query := dao.database.Model(&model.User{}).Where("email = ?", email)

	// 排除指定ID（用于更新时检查）
	if len(excludeID) > 0 && excludeID[0] > 0 {
		query = query.Where("id != ?", excludeID[0])
	}

	if err := query.Count(&count).Error; err != nil {
		return false, fmt.Errorf("检查邮箱是否存在失败: %w", err)
	}
	return count > 0, nil
}

// ExistsPhone 检查手机号是否存在
func (dao *UserDao) ExistsPhone(phone string, excludeID ...uint) (bool, error) {
	if phone == "" {
		return false, nil
	}

	var count int64
	query := dao.database.Model(&model.User{}).Where("phone = ?", phone)

	// 排除指定ID（用于更新时检查）
	if len(excludeID) > 0 && excludeID[0] > 0 {
		query = query.Where("id != ?", excludeID[0])
	}

	if err := query.Count(&count).Error; err != nil {
		return false, fmt.Errorf("检查手机号是否存在失败: %w", err)
	}
	return count > 0, nil
}

// UpdateStatus 更新用户状态
func (dao *UserDao) UpdateStatus(id uint, status model.UserStatus) error {
	updates := map[string]interface{}{
		"status": status,
	}

	// 如果是激活状态，同时设置激活标志
	if status == model.UserStatusActive {
		updates["is_activated"] = true
	}

	if err := dao.database.Model(&model.User{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return fmt.Errorf("更新用户状态失败: %w", err)
	}
	return nil
}

// BatchUpdateStatus 批量更新用户状态
func (dao *UserDao) BatchUpdateStatus(ids []uint, status model.UserStatus) error {
	if len(ids) == 0 {
		return fmt.Errorf("用户ID列表不能为空")
	}

	updates := map[string]interface{}{
		"status": status,
	}

	// 如果是激活状态，同时设置激活标志
	if status == model.UserStatusActive {
		updates["is_activated"] = true
	}

	if err := dao.database.Model(&model.User{}).Where("id IN ?", ids).Updates(updates).Error; err != nil {
		return fmt.Errorf("批量更新用户状态失败: %w", err)
	}
	return nil
}

// GetUsersByDepartment 根据部门ID获取用户列表
// 现在通过 user_departments 关联表查询
func (dao *UserDao) GetUsersByDepartment(departmentID uint) ([]model.User, error) {
	var users []model.User
	// 通过 user_departments 表关联查询
	if err := dao.database.
		Joins("JOIN user_departments ON users.id = user_departments.user_id").
		Where("user_departments.department_id = ? AND user_departments.is_active = ?", departmentID, true).
		Find(&users).Error; err != nil {
		return nil, fmt.Errorf("查询部门用户失败: %w", err)
	}
	return users, nil
}

// GetActiveUsersCount 获取活跃用户数量
func (dao *UserDao) GetActiveUsersCount() (int64, error) {
	var count int64
	if err := dao.database.Model(&model.User{}).
		Where("status = ? AND is_activated = ?", model.UserStatusActive, true).
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("查询活跃用户数量失败: %w", err)
	}
	return count, nil
}
