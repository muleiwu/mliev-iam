package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	directoryprotocol "cnb.cool/mliev/iam/mliev-iam/app/service/protocol/directory"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type DirectoryAdapterFactory func(*model.IdentityConnection, map[string]any) (directoryprotocol.Adapter, error)

type DirectorySyncService struct {
	db             *gorm.DB
	adapterFactory DirectoryAdapterFactory
}

func NewDirectorySyncService(db *gorm.DB) *DirectorySyncService {
	return &DirectorySyncService{
		db: db,
		adapterFactory: func(connection *model.IdentityConnection, credentials map[string]any) (directoryprotocol.Adapter, error) {
			return directoryprotocol.NewAdapter(connection, credentials, nil)
		},
	}
}

func NewDirectorySyncServiceWithFactory(db *gorm.DB, factory DirectoryAdapterFactory) *DirectorySyncService {
	service := NewDirectorySyncService(db)
	service.adapterFactory = factory
	return service
}

func (s *DirectorySyncService) GetPolicy(connectionID uint) (*dto.DirectoryPolicyResponse, error) {
	if _, err := s.enterpriseConnection(connectionID); err != nil {
		return nil, err
	}
	var policy model.DirectorySyncPolicy
	err := s.db.Preload("RootDepartment").Where("connection_id = ?", connectionID).First(&policy).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	configured, err := s.directoryCredentialsConfigured(connectionID)
	if err != nil {
		return nil, err
	}
	if errors.Is(err, gorm.ErrRecordNotFound) || policy.ID == 0 {
		return &dto.DirectoryPolicyResponse{Policy: nil, DirectoryCredentials: configured}, nil
	}
	return &dto.DirectoryPolicyResponse{Policy: &policy, DirectoryCredentials: configured}, nil
}

func (s *DirectorySyncService) SavePolicy(connectionID uint, req *dto.DirectoryPolicyRequest) (*dto.DirectoryPolicyResponse, error) {
	connection, err := s.enterpriseConnection(connectionID)
	if err != nil {
		return nil, err
	}
	if req.RootDepartmentID == 0 {
		return nil, fmt.Errorf("必须选择本地组织根")
	}
	var root model.Department
	if err := s.db.First(&root, req.RootDepartmentID).Error; err != nil {
		return nil, fmt.Errorf("本地组织根不存在: %w", err)
	}
	var used int64
	if err := s.db.Model(&model.DirectorySyncPolicy{}).
		Where("root_department_id = ? AND connection_id <> ?", req.RootDepartmentID, connectionID).Count(&used).Error; err != nil {
		return nil, err
	}
	if used > 0 {
		return nil, fmt.Errorf("该组织根已绑定其他目录连接")
	}
	if req.MatchPolicy == "" {
		req.MatchPolicy = model.DirectoryMatchManual
	}
	if req.MatchPolicy != model.DirectoryMatchManual && req.MatchPolicy != model.DirectoryMatchCreateUnmatched {
		return nil, fmt.Errorf("不支持的匹配策略 %q", req.MatchPolicy)
	}
	if req.DeprovisionPolicy == "" {
		req.DeprovisionPolicy = model.DirectoryDeprovisionBlockIdentityRequireApproval
	}
	if req.DeprovisionPolicy != model.DirectoryDeprovisionBlockIdentityRequireApproval {
		return nil, fmt.Errorf("当前阶段仅支持 %s", model.DirectoryDeprovisionBlockIdentityRequireApproval)
	}
	if len(req.Credentials) > 0 {
		if err := NewIdentityConnectionService(s.db).SaveCredentials(connectionID, model.CredentialPurposeDirectory, req.Credentials); err != nil {
			return nil, fmt.Errorf("保存目录凭据失败: %w", err)
		}
	}
	if req.Enabled {
		configured, err := s.directoryCredentialsConfigured(connectionID)
		if err != nil {
			return nil, err
		}
		if !configured {
			return nil, fmt.Errorf("启用目录同步前必须配置目录凭据")
		}
	}
	policy := &model.DirectorySyncPolicy{
		ConnectionID: connectionID, RootDepartmentID: req.RootDepartmentID,
		SyncScope: model.JSONObject(req.SyncScope), FieldMapping: model.JSONObject(req.FieldMapping),
		FieldOwnership: model.JSONObject(req.FieldOwnership), MatchPolicy: req.MatchPolicy,
		DeprovisionPolicy: req.DeprovisionPolicy, Enabled: req.Enabled,
	}
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		var existing model.DirectorySyncPolicy
		err := tx.Where("connection_id = ?", connectionID).First(&existing).Error
		if err == nil {
			policy.ID = existing.ID
			policy.CreatedAt = existing.CreatedAt
			if err := tx.Save(policy).Error; err != nil {
				return err
			}
		} else if errors.Is(err, gorm.ErrRecordNotFound) {
			if err := tx.Create(policy).Error; err != nil {
				return err
			}
		} else {
			return err
		}
		if req.Enabled && !connection.Capabilities.Contains(string(model.ConnectionCapabilityDirectorySync)) {
			connection.Capabilities = append(connection.Capabilities, string(model.ConnectionCapabilityDirectorySync))
			return tx.Model(connection).Update("capabilities", connection.Capabilities).Error
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("保存目录策略失败: %w", err)
	}
	return s.GetPolicy(connectionID)
}

func (s *DirectorySyncService) Diagnostic(ctx context.Context, connectionID uint) (*directoryprotocol.Diagnostic, error) {
	adapter, err := s.adapter(connectionID, false)
	if err != nil {
		return nil, err
	}
	return adapter.ValidateDirectoryConfig(ctx)
}

func (s *DirectorySyncService) CreateManualRun(ctx context.Context, connectionID uint, actorID *uint) (*model.DirectorySyncRun, error) {
	return s.CreateRun(ctx, connectionID, actorID, "manual")
}

// CreateRun is the single snapshot/diff entry point used by manual requests,
// schedules and event jobs. Automation must not duplicate the phase-02 logic.
func (s *DirectorySyncService) CreateRun(ctx context.Context, connectionID uint, actorID *uint, triggerType string) (*model.DirectorySyncRun, error) {
	policy, connection, adapter, err := s.ready(connectionID)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	run := &model.DirectorySyncRun{
		ConnectionID: connectionID, SnapshotVersion: uuid.NewString(), TriggerType: defaultString(triggerType, "manual"),
		Status: model.DirectoryRunFetching, StartedAt: &now, CreatedBy: actorID,
	}
	if err := s.db.Create(run).Error; err != nil {
		return nil, err
	}
	orgs, users, fetchErr := fetchDirectorySnapshot(ctx, adapter)
	if fetchErr != nil {
		s.failRun(run.ID, fetchErr)
		return nil, fetchErr
	}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		return NewDirectorySyncServiceWithFactory(tx, s.adapterFactory).storeSnapshotAndChanges(run, policy, connection, orgs, users)
	})
	if err != nil {
		s.failRun(run.ID, err)
		return nil, err
	}
	return s.GetRun(run.ID)
}

func fetchDirectorySnapshot(ctx context.Context, adapter directoryprotocol.Adapter) ([]directoryprotocol.OrgUnit, []directoryprotocol.User, error) {
	orgs := make([]directoryprotocol.OrgUnit, 0)
	seenOrgCursor := map[string]bool{}
	for cursor := ""; ; {
		if seenOrgCursor[cursor] {
			return nil, nil, fmt.Errorf("目录部门分页游标循环: %q", cursor)
		}
		seenOrgCursor[cursor] = true
		page, err := adapter.ListOrgUnits(ctx, cursor)
		if err != nil {
			return nil, nil, fmt.Errorf("读取目录部门失败: %w", err)
		}
		orgs = append(orgs, page.Items...)
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	usersByID := map[string]directoryprotocol.User{}
	seenUserCursor := map[string]bool{}
	for cursor := ""; ; {
		if seenUserCursor[cursor] {
			return nil, nil, fmt.Errorf("目录用户分页游标循环: %q", cursor)
		}
		seenUserCursor[cursor] = true
		page, err := adapter.ListUsers(ctx, cursor)
		if err != nil {
			return nil, nil, fmt.Errorf("读取目录用户失败: %w", err)
		}
		for _, user := range page.Items {
			if user.StableID == "" {
				return nil, nil, fmt.Errorf("目录返回了缺少稳定标识的用户")
			}
			usersByID[user.StableID] = user
		}
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	users := make([]directoryprotocol.User, 0, len(usersByID))
	for _, user := range usersByID {
		users = append(users, user)
	}
	sort.Slice(users, func(i, j int) bool { return users[i].StableID < users[j].StableID })
	return orgs, users, nil
}

func (s *DirectorySyncService) storeSnapshotAndChanges(run *model.DirectorySyncRun, policy *model.DirectorySyncPolicy, connection *model.IdentityConnection, orgs []directoryprotocol.OrgUnit, users []directoryprotocol.User) error {
	now := time.Now()
	orgMap := make(map[string]*model.ExternalOrgUnit, len(orgs))
	seenOrg := map[string]bool{}
	for _, item := range orgs {
		if item.ExternalID == "" || strings.TrimSpace(item.Name) == "" {
			return fmt.Errorf("目录部门缺少 externalId 或名称")
		}
		seenOrg[item.ExternalID] = true
		var record model.ExternalOrgUnit
		err := s.db.Where("connection_id = ? AND external_id = ?", run.ConnectionID, item.ExternalID).First(&record).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		previousSnapshot := record.SnapshotVersion
		record.ConnectionID = run.ConnectionID
		record.ExternalID = item.ExternalID
		record.ExternalParentID = item.ExternalParentID
		record.Name = item.Name
		record.SortOrder = item.SortOrder
		record.Status = defaultString(item.Status, "active")
		record.LeaderExternalID = item.LeaderExternalID
		record.SnapshotVersion = run.SnapshotVersion
		record.LastSeenAt = now
		record.RawData = model.JSONObject(item.Raw)
		if record.ID == 0 {
			if err := s.db.Create(&record).Error; err != nil {
				return err
			}
		} else if err := s.db.Save(&record).Error; err != nil {
			return err
		}
		copyRecord := record
		orgMap[item.ExternalID] = &copyRecord
		if err := s.buildOrgChange(run, policy, &copyRecord, previousSnapshot); err != nil {
			return err
		}
	}
	var previouslyMappedOrgs []model.ExternalOrgUnit
	if err := s.db.Where("connection_id = ? AND snapshot_version <> ? AND department_id IS NOT NULL", run.ConnectionID, run.SnapshotVersion).Find(&previouslyMappedOrgs).Error; err != nil {
		return err
	}
	for _, missing := range previouslyMappedOrgs {
		if !seenOrg[missing.ExternalID] {
			if err := s.addChange(run, model.DirectoryChangeDelete, model.DirectoryObjectOrgUnit, missing.ExternalID, "high", "本次快照不可见，仅生成待审批删除建议", map[string]any{"departmentId": missing.DepartmentID, "name": missing.Name}, map[string]any{"externalOrgUnitId": missing.ID}); err != nil {
				return err
			}
		}
	}

	seenPrincipals := map[uint]bool{}
	membershipCount := 0
	for _, item := range users {
		principal, err := s.upsertPrincipal(run, item)
		if err != nil {
			return err
		}
		seenPrincipals[principal.ID] = true
		if err := s.buildUserChange(run, policy, principal, item); err != nil {
			return err
		}
		for _, membership := range item.Memberships {
			org := orgMap[membership.OrgUnitExternalID]
			if org == nil {
				if err := s.addChange(run, model.DirectoryChangeConflict, model.DirectoryObjectMembership, item.StableID+":"+membership.OrgUnitExternalID, "high", "用户所属部门不在当前可见范围", nil, map[string]any{"principalId": principal.ID, "orgUnitExternalId": membership.OrgUnitExternalID}); err != nil {
					return err
				}
				continue
			}
			membershipCount++
			if err := s.upsertMembershipAndChange(run, principal, org, membership); err != nil {
				return err
			}
		}
	}
	var missingPrincipals []model.ExternalPrincipal
	if err := s.db.Where("connection_id = ? AND last_synced_at < ? AND user_id IS NOT NULL", run.ConnectionID, now.Add(-time.Millisecond)).Find(&missingPrincipals).Error; err != nil {
		return err
	}
	for _, missing := range missingPrincipals {
		if !seenPrincipals[missing.ID] {
			if err := s.addChange(run, model.DirectoryChangeDisable, model.DirectoryObjectUser, fmt.Sprint(missing.ID), "high", "本次快照不可见，禁止自动停用本地用户", map[string]any{"userId": missing.UserID, "displayName": missing.DisplayName}, map[string]any{"principalId": missing.ID}); err != nil {
				return err
			}
		}
	}
	var changeCount, conflictCount int64
	if err := s.db.Model(&model.DirectorySyncChange{}).Where("run_id = ?", run.ID).Count(&changeCount).Error; err != nil {
		return err
	}
	if err := s.db.Model(&model.DirectorySyncChange{}).Where("run_id = ? AND change_type = ?", run.ID, model.DirectoryChangeConflict).Count(&conflictCount).Error; err != nil {
		return err
	}
	finished := time.Now()
	return s.db.Model(run).Updates(map[string]any{
		"status": model.DirectoryRunPreviewReady, "total_users": len(users), "total_org_units": len(orgs),
		"total_memberships": membershipCount, "change_count": changeCount, "conflict_count": conflictCount,
		"finished_at": finished, "error_message": "",
	}).Error
}

func (s *DirectorySyncService) buildOrgChange(run *model.DirectorySyncRun, policy *model.DirectorySyncPolicy, org *model.ExternalOrgUnit, previousSnapshot string) error {
	after := map[string]any{"externalOrgUnitId": org.ID, "externalId": org.ExternalID, "parentExternalId": org.ExternalParentID, "name": org.Name, "sortOrder": org.SortOrder, "leaderExternalId": org.LeaderExternalID}
	if org.DepartmentID == nil {
		return s.addChange(run, model.DirectoryChangeCreate, model.DirectoryObjectOrgUnit, org.ExternalID, "low", "外部部门尚未映射", nil, after)
	}
	if ok, err := s.departmentWithinRoot(*org.DepartmentID, policy.RootDepartmentID); err != nil {
		return err
	} else if !ok {
		return s.addChange(run, model.DirectoryChangeConflict, model.DirectoryObjectOrgUnit, org.ExternalID, "high", "历史部门映射已越过当前组织根", map[string]any{"departmentId": org.DepartmentID}, after)
	}
	var department model.Department
	if err := s.db.First(&department, *org.DepartmentID).Error; err != nil {
		return s.addChange(run, model.DirectoryChangeConflict, model.DirectoryObjectOrgUnit, org.ExternalID, "high", "历史映射的本地部门不存在", map[string]any{"departmentId": org.DepartmentID}, after)
	}
	if department.Name != org.Name || department.Sort != org.SortOrder {
		after["departmentId"] = department.ID
		return s.addChange(run, model.DirectoryChangeUpdate, model.DirectoryObjectOrgUnit, org.ExternalID, "low", "已确认的历史部门映射", map[string]any{"departmentId": department.ID, "name": department.Name, "sortOrder": department.Sort, "snapshotVersion": previousSnapshot}, after)
	}
	return nil
}

func (s *DirectorySyncService) upsertPrincipal(run *model.DirectorySyncRun, item directoryprotocol.User) (*model.ExternalPrincipal, error) {
	var identifier model.ExternalIdentifier
	err := s.db.Where("connection_id = ? AND namespace = ? AND value = ?", run.ConnectionID, model.IdentifierNamespaceDirectoryUserID, item.StableID).First(&identifier).Error
	var principal model.ExternalPrincipal
	if err == nil {
		if err := s.db.First(&principal, identifier.PrincipalID).Error; err != nil {
			return nil, err
		}
	} else if errors.Is(err, gorm.ErrRecordNotFound) {
		principal = model.ExternalPrincipal{ConnectionID: run.ConnectionID, Status: model.PrincipalStatusUnmatched}
		if err := s.db.Create(&principal).Error; err != nil {
			return nil, err
		}
	} else {
		return nil, err
	}
	now := time.Now()
	principal.DisplayName = item.DisplayName
	principal.Email = item.Email
	principal.Phone = item.Phone
	principal.Avatar = item.Avatar
	principal.RawProfile = model.JSONObject(item.Raw)
	principal.LastSyncedAt = &now
	if item.Status == "inactive" {
		principal.Status = model.PrincipalStatusBlocked
	} else if principal.UserID != nil {
		principal.Status = model.PrincipalStatusActive
	} else {
		principal.Status = model.PrincipalStatusUnmatched
	}
	if err := s.db.Save(&principal).Error; err != nil {
		return nil, err
	}
	identifiers := item.Identifiers
	if len(identifiers) == 0 {
		identifiers = []directoryprotocol.Identifier{{Namespace: string(model.IdentifierNamespaceDirectoryUserID), Value: item.StableID, Primary: true}}
	}
	for _, itemIdentifier := range identifiers {
		if itemIdentifier.Value == "" {
			continue
		}
		record := model.ExternalIdentifier{ConnectionID: run.ConnectionID, PrincipalID: principal.ID, Namespace: model.IdentifierNamespace(itemIdentifier.Namespace), Value: itemIdentifier.Value, IsPrimary: itemIdentifier.Primary}
		if err := s.db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "connection_id"}, {Name: "namespace"}, {Name: "value"}},
			DoUpdates: clause.Assignments(map[string]any{"principal_id": principal.ID, "is_primary": itemIdentifier.Primary, "updated_at": time.Now()}),
		}).Create(&record).Error; err != nil {
			return nil, fmt.Errorf("保存外部标识 %s 失败: %w", itemIdentifier.Namespace, err)
		}
	}
	return &principal, nil
}

func (s *DirectorySyncService) buildUserChange(run *model.DirectorySyncRun, policy *model.DirectorySyncPolicy, principal *model.ExternalPrincipal, item directoryprotocol.User) error {
	after := map[string]any{"principalId": principal.ID, "stableId": item.StableID, "displayName": item.DisplayName, "email": item.Email, "phone": item.Phone, "avatar": item.Avatar, "employeeNo": item.EmployeeNo, "status": item.Status}
	if principal.UserID != nil {
		after["userId"] = *principal.UserID
		if item.Status == "inactive" {
			return s.addChange(run, model.DirectoryChangeDisable, model.DirectoryObjectUser, item.StableID, "high", "上游明确返回离职或禁用；企业身份已立即封禁，本地用户停用需审批", map[string]any{"userId": *principal.UserID}, after)
		}
		var user model.User
		if err := s.db.First(&user, *principal.UserID).Error; err != nil {
			return s.addChange(run, model.DirectoryChangeConflict, model.DirectoryObjectUser, item.StableID, "high", "历史映射的本地用户不存在", map[string]any{"userId": *principal.UserID}, after)
		}
		if user.RealName != item.DisplayName || pointerString(user.Email) != item.Email || pointerString(user.Phone) != item.Phone || user.Avatar != item.Avatar {
			return s.addChange(run, model.DirectoryChangeUpdate, model.DirectoryObjectUser, item.StableID, "low", "稳定外部标识及已确认历史映射", map[string]any{"userId": user.ID, "realName": user.RealName, "email": pointerString(user.Email), "phone": pointerString(user.Phone), "avatar": user.Avatar}, after)
		}
		return nil
	}
	if item.Status == "inactive" {
		return s.addChange(run, model.DirectoryChangeConflict, model.DirectoryObjectUser, item.StableID, "high", "上游用户已离职或禁用，不自动创建本地账号", nil, after)
	}
	matches, err := s.findUniqueContactMatch(item.Email, item.Phone)
	if err != nil {
		return err
	}
	if len(matches) == 1 {
		after["userId"] = matches[0].ID
		return s.addChange(run, model.DirectoryChangeLink, model.DirectoryObjectUser, item.StableID, "medium", "唯一邮箱或手机号仅生成待确认匹配", nil, after)
	}
	if len(matches) > 1 {
		ids := make([]uint, 0, len(matches))
		for _, user := range matches {
			ids = append(ids, user.ID)
		}
		after["candidateUserIds"] = ids
		return s.addChange(run, model.DirectoryChangeConflict, model.DirectoryObjectUser, item.StableID, "high", "邮箱或手机号命中多个本地用户", nil, after)
	}
	if policy.MatchPolicy == model.DirectoryMatchCreateUnmatched {
		return s.addChange(run, model.DirectoryChangeCreate, model.DirectoryObjectUser, item.StableID, "low", "未匹配用户按策略建议新建", nil, after)
	}
	return s.addChange(run, model.DirectoryChangeConflict, model.DirectoryObjectUser, item.StableID, "medium", "未找到稳定映射，当前策略要求人工处理", nil, after)
}

func (s *DirectorySyncService) upsertMembershipAndChange(run *model.DirectorySyncRun, principal *model.ExternalPrincipal, org *model.ExternalOrgUnit, item directoryprotocol.Membership) error {
	var record model.ExternalMembership
	err := s.db.Where("connection_id = ? AND principal_id = ? AND external_org_unit_id = ?", run.ConnectionID, principal.ID, org.ID).First(&record).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	record.ConnectionID = run.ConnectionID
	record.PrincipalID = principal.ID
	record.ExternalOrgUnitID = org.ID
	record.UserID = principal.UserID
	record.DepartmentID = org.DepartmentID
	record.IsPrimary = item.Primary
	record.IsLeader = item.Leader
	record.Position = item.Position
	record.Status = "active"
	record.SnapshotVersion = run.SnapshotVersion
	record.LastSeenAt = time.Now()
	record.RawData = model.JSONObject(item.Raw)
	if record.ID == 0 {
		if err := s.db.Create(&record).Error; err != nil {
			return err
		}
	} else if err := s.db.Save(&record).Error; err != nil {
		return err
	}
	after := map[string]any{"membershipId": record.ID, "principalId": principal.ID, "externalOrgUnitId": org.ID, "isPrimary": item.Primary, "isLeader": item.Leader, "position": item.Position}
	if principal.UserID == nil || org.DepartmentID == nil {
		return s.addChange(run, model.DirectoryChangeCreate, model.DirectoryObjectMembership, fmt.Sprint(record.ID), "low", "成员关系将在用户与部门映射建立后应用", nil, after)
	}
	var local model.UserDepartment
	err = s.db.Unscoped().Where("user_id = ? AND department_id = ?", *principal.UserID, *org.DepartmentID).First(&local).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return s.addChange(run, model.DirectoryChangeCreate, model.DirectoryObjectMembership, fmt.Sprint(record.ID), "low", "已映射用户与部门缺少成员关系", nil, after)
	}
	if err != nil {
		return err
	}
	isLeader := local.Type == model.UserDepartmentTypeLeader
	if local.IsPrimary != item.Primary || local.Position != item.Position || isLeader != item.Leader || local.DeletedAt.Valid || !local.IsActive {
		return s.addChange(run, model.DirectoryChangeUpdate, model.DirectoryObjectMembership, fmt.Sprint(record.ID), "low", "同步多部门、主部门、负责人和职位", map[string]any{"userId": local.UserID, "departmentId": local.DepartmentID, "isPrimary": local.IsPrimary, "position": local.Position, "type": local.Type}, after)
	}
	return nil
}

func (s *DirectorySyncService) addChange(run *model.DirectorySyncRun, changeType model.DirectoryChangeType, objectType model.DirectoryObjectType, key, risk, reason string, before, after map[string]any) error {
	change := &model.DirectorySyncChange{
		RunID: run.ID, ConnectionID: run.ConnectionID, ChangeType: changeType, ObjectType: objectType,
		ObjectKey: key, Status: model.DirectoryChangePending, RiskLevel: risk,
		BeforeData: model.JSONObject(before), AfterData: model.JSONObject(after), MatchReason: reason,
	}
	return s.db.Create(change).Error
}

func (s *DirectorySyncService) findUniqueContactMatch(email, phone string) ([]model.User, error) {
	query := s.db.Model(&model.User{})
	conditions := make([]string, 0, 2)
	values := make([]any, 0, 2)
	if email != "" {
		conditions = append(conditions, "LOWER(email) = LOWER(?)")
		values = append(values, email)
	}
	if phone != "" {
		conditions = append(conditions, "phone = ?")
		values = append(values, phone)
	}
	if len(conditions) == 0 {
		return nil, nil
	}
	var users []model.User
	err := query.Where("("+strings.Join(conditions, " OR ")+")", values...).Limit(3).Find(&users).Error
	return users, err
}

func (s *DirectorySyncService) ListRuns(connectionID uint, page, pageSize int) ([]model.DirectorySyncRun, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 20
	}
	query := s.db.Model(&model.DirectorySyncRun{}).Where("connection_id = ?", connectionID)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var runs []model.DirectorySyncRun
	err := query.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&runs).Error
	return runs, total, err
}

func (s *DirectorySyncService) GetRun(runID uint) (*model.DirectorySyncRun, error) {
	var run model.DirectorySyncRun
	if err := s.db.Preload("Connection").First(&run, runID).Error; err != nil {
		return nil, err
	}
	return &run, nil
}

func (s *DirectorySyncService) ListChanges(runID uint, status, changeType, risk string, page, pageSize int) ([]model.DirectorySyncChange, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 500 {
		pageSize = 100
	}
	query := s.db.Model(&model.DirectorySyncChange{}).Where("run_id = ?", runID)
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if changeType != "" {
		query = query.Where("change_type = ?", changeType)
	}
	if risk != "" {
		query = query.Where("risk_level = ?", risk)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var changes []model.DirectorySyncChange
	err := query.Order("CASE object_type WHEN 'org_unit' THEN 1 WHEN 'user' THEN 2 ELSE 3 END, id ASC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&changes).Error
	return changes, total, err
}

func (s *DirectorySyncService) DecideChange(changeID uint, approve bool, actorID uint) error {
	status := model.DirectoryChangeRejected
	if approve {
		status = model.DirectoryChangeApproved
	}
	now := time.Now()
	result := s.db.Model(&model.DirectorySyncChange{}).
		Where("id = ? AND status = ?", changeID, model.DirectoryChangePending).
		Updates(map[string]any{"status": status, "reviewed_by": actorID, "reviewed_at": now})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("变更不存在或已完成审核")
	}
	return nil
}

func (s *DirectorySyncService) ApproveRun(runID, actorID uint) error {
	if _, err := s.GetRun(runID); err != nil {
		return err
	}
	now := time.Now()
	return s.db.Model(&model.DirectorySyncChange{}).
		Where("run_id = ? AND status = ? AND risk_level = ? AND change_type IN ?", runID, model.DirectoryChangePending, "low", []model.DirectoryChangeType{model.DirectoryChangeCreate, model.DirectoryChangeUpdate}).
		Updates(map[string]any{"status": model.DirectoryChangeApproved, "reviewed_by": actorID, "reviewed_at": now}).Error
}

// ApproveAutomationSafeChanges only approves the explicitly safe subset. It
// is intentionally equivalent to the admin bulk-safe action so worker and UI
// cannot drift into different governance behavior.
func (s *DirectorySyncService) ApproveAutomationSafeChanges(runID uint) error {
	if _, err := s.GetRun(runID); err != nil {
		return err
	}
	now := time.Now()
	return s.db.Model(&model.DirectorySyncChange{}).
		Where("run_id = ? AND status = ? AND risk_level = ? AND change_type IN ?", runID, model.DirectoryChangePending, "low", []model.DirectoryChangeType{model.DirectoryChangeCreate, model.DirectoryChangeUpdate}).
		Updates(map[string]any{"status": model.DirectoryChangeApproved, "reviewed_at": now}).Error
}

func (s *DirectorySyncService) ApplyRun(runID, actorID uint) (*model.DirectorySyncRun, error) {
	run, err := s.GetRun(runID)
	if err != nil {
		return nil, err
	}
	if run.Status != model.DirectoryRunPreviewReady && run.Status != model.DirectoryRunFailed {
		return nil, fmt.Errorf("只有预览完成或应用失败的同步可以执行")
	}
	var policy model.DirectorySyncPolicy
	if err := s.db.Where("connection_id = ?", run.ConnectionID).First(&policy).Error; err != nil {
		return nil, err
	}
	if err := s.db.Model(run).Updates(map[string]any{"status": model.DirectoryRunApplying, "finished_at": nil, "error_message": ""}).Error; err != nil {
		return nil, err
	}
	var changes []model.DirectorySyncChange
	if err := s.db.Where("run_id = ? AND status = ?", runID, model.DirectoryChangeApproved).
		Order("CASE object_type WHEN 'org_unit' THEN 1 WHEN 'user' THEN 2 ELSE 3 END, id ASC").Find(&changes).Error; err != nil {
		return nil, err
	}
	for i := range changes {
		change := changes[i]
		err := s.db.Transaction(func(tx *gorm.DB) error {
			return NewDirectorySyncServiceWithFactory(tx, s.adapterFactory).applyChange(&change, &policy)
		})
		if err != nil {
			_ = s.db.Model(&change).Updates(map[string]any{"status": model.DirectoryChangeFailed, "error_message": err.Error()}).Error
			s.failRun(runID, err)
			return nil, fmt.Errorf("应用变更 %d 失败: %w", change.ID, err)
		}
	}
	var applied int64
	if err := s.db.Model(&model.DirectorySyncChange{}).Where("run_id = ? AND status = ?", runID, model.DirectoryChangeApplied).Count(&applied).Error; err != nil {
		return nil, err
	}
	var remaining int64
	if err := s.db.Model(&model.DirectorySyncChange{}).Where("run_id = ? AND status IN ?", runID, []model.DirectoryChangeStatus{model.DirectoryChangePending, model.DirectoryChangeApproved, model.DirectoryChangeFailed}).Count(&remaining).Error; err != nil {
		return nil, err
	}
	finished := time.Now()
	finalStatus := model.DirectoryRunSucceeded
	if remaining > 0 {
		finalStatus = model.DirectoryRunPreviewReady
	}
	if err := s.db.Model(run).Updates(map[string]any{"status": finalStatus, "applied_count": applied, "finished_at": finished}).Error; err != nil {
		return nil, err
	}
	return s.GetRun(runID)
}

func (s *DirectorySyncService) applyChange(change *model.DirectorySyncChange, policy *model.DirectorySyncPolicy) error {
	if change.Status == model.DirectoryChangeApplied {
		return nil
	}
	switch change.ObjectType {
	case model.DirectoryObjectOrgUnit:
		if err := s.applyOrgChange(change, policy); err != nil {
			return err
		}
	case model.DirectoryObjectUser:
		if err := s.applyUserChange(change, policy); err != nil {
			return err
		}
	case model.DirectoryObjectMembership:
		if err := s.applyMembershipChange(change, policy); err != nil {
			return err
		}
	default:
		return fmt.Errorf("未知变更对象 %q", change.ObjectType)
	}
	now := time.Now()
	return s.db.Model(change).Updates(map[string]any{"status": model.DirectoryChangeApplied, "applied_at": now, "error_message": ""}).Error
}

func (s *DirectorySyncService) applyOrgChange(change *model.DirectorySyncChange, policy *model.DirectorySyncPolicy) error {
	orgID := uintValue(change.AfterData["externalOrgUnitId"])
	var org model.ExternalOrgUnit
	if err := s.db.First(&org, orgID).Error; err != nil {
		return err
	}
	if change.ChangeType == model.DirectoryChangeDelete {
		return nil // Department deletion remains a review-only operation in this phase.
	}
	if org.DepartmentID != nil {
		ok, err := s.departmentWithinRoot(*org.DepartmentID, policy.RootDepartmentID)
		if err != nil || !ok {
			return fmt.Errorf("本地部门不在策略组织根内")
		}
		return s.db.Model(&model.Department{}).Where("id = ?", *org.DepartmentID).Updates(map[string]any{"name": org.Name, "sort": org.SortOrder}).Error
	}
	parentID := policy.RootDepartmentID
	if org.ExternalParentID != "" && org.ExternalParentID != "0" {
		var parentOrg model.ExternalOrgUnit
		if err := s.db.Where("connection_id = ? AND external_id = ?", org.ConnectionID, org.ExternalParentID).First(&parentOrg).Error; err == nil && parentOrg.DepartmentID != nil {
			parentID = *parentOrg.DepartmentID
		}
	}
	ok, err := s.departmentWithinRoot(parentID, policy.RootDepartmentID)
	if err != nil || !ok {
		return fmt.Errorf("外部父部门映射不在策略组织根内")
	}
	department := model.Department{
		Name: org.Name, Code: deterministicCode("dir", org.ConnectionID, org.ExternalID),
		ShortName: org.Name, Type: model.DepartmentTypeDepartment, Status: model.DepartmentStatusEnabled,
		ParentID: &parentID, Sort: org.SortOrder, IsLeaf: true, DataScope: 2,
		Attributes: mustJSON(map[string]any{"directoryConnectionId": org.ConnectionID, "externalOrgUnitId": org.ExternalID}),
	}
	if err := s.db.Create(&department).Error; err != nil {
		return err
	}
	var parent model.Department
	if err := s.db.First(&parent, parentID).Error; err != nil {
		return err
	}
	path := strings.TrimRight(parent.Path, "/") + "/" + fmt.Sprint(department.ID)
	if err := s.db.Model(&department).Updates(map[string]any{"path": path, "level": parent.Level + 1}).Error; err != nil {
		return err
	}
	return s.db.Model(&org).Update("department_id", department.ID).Error
}

func (s *DirectorySyncService) applyUserChange(change *model.DirectorySyncChange, policy *model.DirectorySyncPolicy) error {
	principalID := uintValue(change.AfterData["principalId"])
	var principal model.ExternalPrincipal
	if err := s.db.First(&principal, principalID).Error; err != nil {
		return err
	}
	if change.ChangeType == model.DirectoryChangeDisable {
		if err := s.db.Model(&principal).Update("status", model.PrincipalStatusBlocked).Error; err != nil {
			return err
		}
		if principal.UserID == nil {
			return nil
		}
		var otherActive int64
		if err := s.db.Model(&model.ExternalPrincipal{}).
			Where("user_id = ? AND id <> ? AND status = ?", *principal.UserID, principal.ID, model.PrincipalStatusActive).
			Count(&otherActive).Error; err != nil {
			return err
		}
		if otherActive > 0 {
			return nil
		}
		now := time.Now()
		if err := s.db.Model(&model.User{}).Where("id = ?", *principal.UserID).Updates(map[string]any{
			"status": model.UserStatusDisabled, "is_activated": false, "session_revoked_at": now,
		}).Error; err != nil {
			return err
		}
		return dao.NewOIDCTokenDAO(s.db).RevokeByUser(*principal.UserID)
	}
	userID := uintValue(change.AfterData["userId"])
	if change.ChangeType == model.DirectoryChangeCreate && userID == 0 {
		email := nullableString(stringValue(change.AfterData["email"]))
		phone := nullableString(stringValue(change.AfterData["phone"]))
		user := model.User{
			Username: deterministicCode("sync", change.ConnectionID, change.ObjectKey), Email: email, Phone: phone,
			Nickname: stringValue(change.AfterData["displayName"]), RealName: stringValue(change.AfterData["displayName"]),
			Avatar: stringValue(change.AfterData["avatar"]), Status: model.UserStatusActive, IsActivated: true,
			Attributes: model.JSONObject{"directoryConnectionId": change.ConnectionID, "directoryStableId": change.ObjectKey},
		}
		if err := s.db.Create(&user).Error; err != nil {
			return err
		}
		userID = user.ID
	}
	if userID == 0 && principal.UserID != nil {
		userID = *principal.UserID
	}
	if userID == 0 {
		return fmt.Errorf("用户变更缺少已批准的本地用户映射")
	}
	if change.ChangeType == model.DirectoryChangeUpdate {
		updates := map[string]any{}
		if upstreamOwns(policy, "realName") {
			updates["real_name"] = stringValue(change.AfterData["displayName"])
			updates["nickname"] = stringValue(change.AfterData["displayName"])
		}
		if upstreamOwns(policy, "avatar") {
			updates["avatar"] = stringValue(change.AfterData["avatar"])
		}
		if upstreamOwns(policy, "email") && stringValue(change.AfterData["email"]) != "" {
			updates["email"] = stringValue(change.AfterData["email"])
		}
		if upstreamOwns(policy, "phone") && stringValue(change.AfterData["phone"]) != "" {
			updates["phone"] = stringValue(change.AfterData["phone"])
		}
		if len(updates) > 0 {
			if err := s.db.Model(&model.User{}).Where("id = ?", userID).Updates(updates).Error; err != nil {
				return err
			}
		}
	}
	status := model.PrincipalStatusActive
	if principal.Status == model.PrincipalStatusBlocked {
		status = model.PrincipalStatusBlocked
	}
	return s.db.Model(&principal).Updates(map[string]any{"user_id": userID, "status": status}).Error
}

func (s *DirectorySyncService) applyMembershipChange(change *model.DirectorySyncChange, policy *model.DirectorySyncPolicy) error {
	membershipID := uintValue(change.AfterData["membershipId"])
	var membership model.ExternalMembership
	if err := s.db.Preload("Principal").Preload("OrgUnit").First(&membership, membershipID).Error; err != nil {
		return err
	}
	if membership.Principal == nil || membership.Principal.UserID == nil || membership.OrgUnit == nil || membership.OrgUnit.DepartmentID == nil {
		return fmt.Errorf("成员关系依赖的用户或部门映射尚未建立")
	}
	ok, err := s.departmentWithinRoot(*membership.OrgUnit.DepartmentID, policy.RootDepartmentID)
	if err != nil || !ok {
		return fmt.Errorf("成员关系目标部门不在策略组织根内")
	}
	relation := model.UserDepartment{
		UserID: *membership.Principal.UserID, DepartmentID: *membership.OrgUnit.DepartmentID,
		Position: membership.Position, Type: model.UserDepartmentTypeMember,
		IsActive: true, IsPrimary: membership.IsPrimary,
		Attributes: model.JSONObject{"directoryConnectionId": membership.ConnectionID, "externalMembershipId": membership.ID},
	}
	if membership.IsLeader {
		relation.Type = model.UserDepartmentTypeLeader
	}
	if err := s.db.Unscoped().Where("user_id = ? AND department_id = ?", relation.UserID, relation.DepartmentID).
		Assign(map[string]any{"position": relation.Position, "type": relation.Type, "is_active": true, "is_primary": relation.IsPrimary, "deleted_at": nil, "attributes": relation.Attributes}).
		FirstOrCreate(&relation).Error; err != nil {
		return err
	}
	return s.db.Model(&membership).Updates(map[string]any{"user_id": relation.UserID, "department_id": relation.DepartmentID}).Error
}

func (s *DirectorySyncService) ready(connectionID uint) (*model.DirectorySyncPolicy, *model.IdentityConnection, directoryprotocol.Adapter, error) {
	connection, err := s.enterpriseConnection(connectionID)
	if err != nil {
		return nil, nil, nil, err
	}
	var policy model.DirectorySyncPolicy
	if err := s.db.Where("connection_id = ? AND enabled = ?", connectionID, true).First(&policy).Error; err != nil {
		return nil, nil, nil, fmt.Errorf("目录同步策略未启用: %w", err)
	}
	adapter, err := s.adapterForConnection(connection)
	return &policy, connection, adapter, err
}

func (s *DirectorySyncService) adapter(connectionID uint, requireEnabled bool) (directoryprotocol.Adapter, error) {
	if requireEnabled {
		_, _, adapter, err := s.ready(connectionID)
		return adapter, err
	}
	connection, err := s.enterpriseConnection(connectionID)
	if err != nil {
		return nil, err
	}
	return s.adapterForConnection(connection)
}

func (s *DirectorySyncService) adapterForConnection(connection *model.IdentityConnection) (directoryprotocol.Adapter, error) {
	credentials, err := NewIdentityConnectionService(s.db).CredentialValues(connection.ID, model.CredentialPurposeDirectory)
	if err != nil {
		return nil, fmt.Errorf("读取目录凭据失败: %w", err)
	}
	return s.adapterFactory(connection, credentials)
}

func (s *DirectorySyncService) enterpriseConnection(connectionID uint) (*model.IdentityConnection, error) {
	var connection model.IdentityConnection
	if err := s.db.First(&connection, connectionID).Error; err != nil {
		return nil, err
	}
	if connection.Category != model.ConnectionCategoryEnterprise {
		return nil, fmt.Errorf("连接 %d 不是企业身份连接", connectionID)
	}
	return &connection, nil
}

func (s *DirectorySyncService) directoryCredentialsConfigured(connectionID uint) (bool, error) {
	var count int64
	err := s.db.Model(&model.IdentityConnectionCredential{}).Where("connection_id = ? AND purpose = ?", connectionID, model.CredentialPurposeDirectory).Count(&count).Error
	return count > 0, err
}

func (s *DirectorySyncService) departmentWithinRoot(departmentID, rootID uint) (bool, error) {
	current := departmentID
	visited := map[uint]bool{}
	for current != 0 {
		if current == rootID {
			return true, nil
		}
		if visited[current] {
			return false, fmt.Errorf("本地部门层级存在循环")
		}
		visited[current] = true
		var department model.Department
		if err := s.db.Select("id", "parent_id").First(&department, current).Error; err != nil {
			return false, err
		}
		if department.ParentID == nil {
			break
		}
		current = *department.ParentID
	}
	return false, nil
}

func (s *DirectorySyncService) failRun(runID uint, cause error) {
	now := time.Now()
	_ = s.db.Model(&model.DirectorySyncRun{}).Where("id = ?", runID).Updates(map[string]any{"status": model.DirectoryRunFailed, "error_message": cause.Error(), "finished_at": now}).Error
}

func upstreamOwns(policy *model.DirectorySyncPolicy, field string) bool {
	if policy == nil || len(policy.FieldOwnership) == 0 {
		return true
	}
	value, exists := policy.FieldOwnership[field]
	if !exists {
		return true
	}
	text, _ := value.(string)
	return text == "upstream" || text == "directory" || text == ""
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func pointerString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func nullableString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	return fmt.Sprint(value)
}

func uintValue(value any) uint {
	switch typed := value.(type) {
	case uint:
		return typed
	case int:
		return uint(typed)
	case int64:
		return uint(typed)
	case float64:
		return uint(typed)
	default:
		var result uint
		_, _ = fmt.Sscan(fmt.Sprint(value), &result)
		return result
	}
}

func deterministicCode(prefix string, connectionID uint, key string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d:%s", connectionID, key)))
	return prefix + "_" + hex.EncodeToString(sum[:8])
}

func mustJSON(value any) []byte {
	data := model.JSONObject{}
	if object, ok := value.(map[string]any); ok {
		data = model.JSONObject(object)
	}
	bytes, _ := data.Value()
	if text, ok := bytes.(string); ok {
		return []byte(text)
	}
	return nil
}
