package service

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrCustomPermissionInvalid = errors.New("自定义权限参数错误")

func permissionInvalid(message string) error {
	return fmt.Errorf("%w: %s", ErrCustomPermissionInvalid, message)
}

type CustomPermissionService struct{ db *gorm.DB }

func NewCustomPermissionService(db *gorm.DB) *CustomPermissionService {
	return &CustomPermissionService{db: db}
}

func (s *CustomPermissionService) SavePermission(id uint, req dto.CustomPermissionRequest, actor uint) (*model.CustomPermission, error) {
	var result *model.CustomPermission
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var err error
		result, err = NewCustomPermissionService(tx).savePermission(id, req, actor)
		return err
	})
	return result, err
}

func (s *CustomPermissionService) savePermission(id uint, req dto.CustomPermissionRequest, actor uint) (*model.CustomPermission, error) {
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || len([]rune(req.Name)) > 100 || len([]rune(req.Description)) > 2000 {
		return nil, permissionInvalid("名称必填，名称或说明过长")
	}
	p := model.CustomPermission{Enabled: true, CreatedBy: actor}
	if id != 0 {
		if err := s.db.Clauses(clause.Locking{Strength: "UPDATE"}).First(&p, id).Error; err != nil {
			return nil, err
		}
		if req.Code != "" && req.Code != p.Code {
			return nil, permissionInvalid("code 创建后不可修改")
		}
	} else {
		if len(req.Code) > 100 || !model.ValidOIDCScope(req.Code) || model.IsStandardOIDCScope(req.Code) {
			return nil, permissionInvalid("code 格式无效或使用了标准 scope 名称")
		}
		p.Code = req.Code
	}
	p.Name, p.Description, p.UpdatedBy = req.Name, req.Description, actor
	if req.Enabled != nil {
		p.Enabled = *req.Enabled
	}
	// Include archived definitions: a code must not silently acquire a new meaning.
	var count int64
	if err := s.db.Unscoped().Model(&model.CustomPermission{}).Where("code = ? AND id <> ?", p.Code, id).Count(&count).Error; err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, permissionInvalid("code 已存在（包括已删除的权限）")
	}
	err := s.db.Save(&p).Error
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		err = permissionInvalid("code 已存在")
	}
	return &p, err
}

func (s *CustomPermissionService) applications() (map[uint][]dto.CustomPermissionApplication, error) {
	var apps []model.Application
	if err := s.db.Where("protocol_type = ?", "oidc").Find(&apps).Error; err != nil {
		return nil, err
	}
	result := map[uint][]dto.CustomPermissionApplication{}
	for _, app := range apps {
		c, err := model.DecodeOIDCAuthorizationConfig(app.ProtocolConfig)
		if err != nil {
			return nil, err
		}
		seen := map[uint]bool{}
		for _, id := range c.CustomPermissionIDs {
			if !seen[id] {
				result[id] = append(result[id], dto.CustomPermissionApplication{ID: app.ID, Name: app.Name})
				seen[id] = true
			}
		}
	}
	return result, nil
}

func customPage(q dto.CustomPermissionQuery) (int, int) {
	page, size := q.Page, q.PageSize
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	return page, size
}

func (s *CustomPermissionService) ListPermissions(q dto.CustomPermissionQuery) (*dto.PaginatedResponse, error) {
	query := s.db.Model(&model.CustomPermission{})
	if q.Keyword != "" {
		query = query.Where("code LIKE ? OR name LIKE ?", "%"+q.Keyword+"%", "%"+q.Keyword+"%")
	}
	if q.Enabled != nil {
		query = query.Where("enabled = ?", *q.Enabled)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	page, size := customPage(q)
	var rows []model.CustomPermission
	if err := query.Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&rows).Error; err != nil {
		return nil, err
	}
	apps, err := s.applications()
	if err != nil {
		return nil, err
	}
	items := []dto.CustomPermissionResponse{}
	for _, p := range rows {
		refs := apps[p.ID]
		if refs == nil {
			refs = []dto.CustomPermissionApplication{}
		}
		items = append(items, dto.CustomPermissionResponse{CustomPermission: p, Applications: refs})
	}
	return &dto.PaginatedResponse{Total: int(total), Page: page, Size: size, Items: items}, nil
}

func (s *CustomPermissionService) GetPermission(id uint) (*dto.CustomPermissionResponse, error) {
	var p model.CustomPermission
	if err := s.db.First(&p, id).Error; err != nil {
		return nil, err
	}
	apps, err := s.applications()
	if err != nil {
		return nil, err
	}
	refs := apps[id]
	if refs == nil {
		refs = []dto.CustomPermissionApplication{}
	}
	return &dto.CustomPermissionResponse{CustomPermission: p, Applications: refs}, nil
}

func (s *CustomPermissionService) DeletePermission(id uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var p model.CustomPermission
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&p, id).Error; err != nil {
			return err
		}
		apps, err := NewCustomPermissionService(tx).applications()
		if err != nil {
			return err
		}
		if len(apps[id]) > 0 {
			return permissionInvalid("仍有应用引用此权限，请先解除关联")
		}
		var count int64
		if err := tx.Model(&model.CustomPermissionGrant{}).Where("permission_id = ?", id).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return permissionInvalid("仍有授权记录，请先撤销授权")
		}
		return tx.Delete(&p).Error
	})
}

// Subject information is also used when displaying stale grants, so an absent
// subject is distinguishable from a database failure.
func (s *CustomPermissionService) subject(kind string, id uint) (string, bool, error) {
	var name string
	var available bool
	var err error
	switch kind {
	case "everyone":
		return "全体用户", id == 0, nil
	case "user":
		var row model.User
		err = s.db.First(&row, id).Error
		name = row.Username
		if row.RealName != "" {
			name = row.RealName + " (" + row.Username + ")"
		}
		available = row.CanLogin()
	case "department":
		var row model.Department
		err = s.db.First(&row, id).Error
		name = row.Name
		available = row.Status == model.DepartmentStatusEnabled
	case "role":
		var row model.Role
		err = s.db.First(&row, id).Error
		name = row.Name
		available = row.IsEnabled()
	case "client":
		var row model.Application
		err = s.db.First(&row, id).Error
		name = row.Name
		available = machinePermissionClient(&row)
	default:
		return "", false, permissionInvalid("未知授权对象类型")
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Sprintf("#%d（已删除）", id), false, nil
	}
	return name, available, err
}

func machinePermissionClient(app *model.Application) bool {
	if !app.IsActive || app.ProtocolType != "oidc" || app.ProtocolConfig["clientType"] != "confidential" {
		return false
	}
	switch values := app.ProtocolConfig["grantTypes"].(type) {
	case []interface{}:
		for _, v := range values {
			if v == "client_credentials" {
				return true
			}
		}
	case []string:
		for _, v := range values {
			if v == "client_credentials" {
				return true
			}
		}
	}
	return false
}

func (s *CustomPermissionService) SaveGrant(id uint, req dto.CustomPermissionGrantRequest, actor uint) (*model.CustomPermissionGrant, error) {
	if req.PermissionID == 0 || (req.SubjectType != "everyone" && req.SubjectID == 0) || (req.SubjectType == "everyone" && req.SubjectID != 0) {
		return nil, permissionInvalid("请选择有效的权限和授权对象")
	}
	if req.IncludeSubDepartments && req.SubjectType != "department" {
		return nil, permissionInvalid("仅部门授权支持包含子部门")
	}
	if len([]rune(req.Remark)) > 2000 {
		return nil, permissionInvalid("备注过长")
	}
	if (req.StartsAt == nil) != (req.EndsAt == nil) || (req.StartsAt != nil && !req.StartsAt.Before(*req.EndsAt)) {
		return nil, permissionInvalid("起止时间须同时填写，且开始时间早于结束时间")
	}
	if req.StartsAt != nil {
		start, end := req.StartsAt.UTC(), req.EndsAt.UTC()
		req.StartsAt, req.EndsAt = &start, &end
	}
	var result model.CustomPermissionGrant
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var p model.CustomPermission
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&p, req.PermissionID).Error; err != nil {
			return err
		}
		_, available, err := NewCustomPermissionService(tx).subject(req.SubjectType, req.SubjectID)
		if err != nil {
			return err
		}
		if !available {
			return permissionInvalid("授权对象不存在、未启用或不支持客户端凭据")
		}
		result = model.CustomPermissionGrant{CreatedBy: actor}
		if id != 0 {
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&result, id).Error; err != nil {
				return err
			}
		}
		result.PermissionID, result.SubjectType, result.SubjectID = req.PermissionID, req.SubjectType, req.SubjectID
		result.IncludeSubDepartments, result.StartsAt, result.EndsAt = req.IncludeSubDepartments, req.StartsAt, req.EndsAt
		result.Remark, result.UpdatedBy = req.Remark, actor
		return tx.Save(&result).Error
	})
	return &result, err
}

func (s *CustomPermissionService) ListGrants(q dto.CustomPermissionQuery) (*dto.PaginatedResponse, error) {
	now := time.Now().UTC()
	query := s.db.Model(&model.CustomPermissionGrant{})
	if q.PermissionID != 0 {
		query = query.Where("permission_id = ?", q.PermissionID)
	}
	if q.Keyword != "" {
		query = query.Where("permission_id IN (?)", s.db.Model(&model.CustomPermission{}).Select("id").Where("name LIKE ? OR code LIKE ?", "%"+q.Keyword+"%", "%"+q.Keyword+"%"))
	}
	if q.SubjectType != "" {
		query = query.Where("subject_type = ?", q.SubjectType)
	}
	if q.SubjectID != 0 {
		query = query.Where("subject_id = ?", q.SubjectID)
	}
	switch q.State {
	case "pending":
		query = query.Where("starts_at > ?", now)
	case "expired":
		query = query.Where("ends_at <= ?", now)
	case "active":
		query = query.Where("(starts_at IS NULL OR starts_at <= ?) AND (ends_at IS NULL OR ends_at > ?)", now, now)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	page, size := customPage(q)
	var rows []model.CustomPermissionGrant
	if err := query.Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&rows).Error; err != nil {
		return nil, err
	}
	items := []dto.CustomPermissionGrantResponse{}
	for _, row := range rows {
		var p model.CustomPermission
		if err := s.db.First(&p, row.PermissionID).Error; err != nil {
			return nil, err
		}
		name, available, err := s.subject(row.SubjectType, row.SubjectID)
		if err != nil {
			return nil, err
		}
		items = append(items, dto.CustomPermissionGrantResponse{CustomPermissionGrant: row, PermissionName: p.Name, PermissionCode: p.Code, PermissionEnabled: p.Enabled, SubjectName: name, SubjectAvailable: available, State: row.State(now)})
	}
	return &dto.PaginatedResponse{Total: int(total), Page: page, Size: size, Items: items}, nil
}

func (s *CustomPermissionService) RevokeGrant(id, actor uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var row model.CustomPermissionGrant
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, id).Error; err != nil {
			return err
		}
		if err := tx.Model(&row).Update("updated_by", actor).Error; err != nil {
			return err
		}
		return tx.Delete(&row).Error
	})
}
