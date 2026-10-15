package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	lease, err := AcquireDirectoryLease(context.Background(), s.db, "policy-roots", nil, 30*time.Second)
	if err != nil {
		return nil, err
	}
	defer lease.Release(context.Background())
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
	if err := s.ensureNonOverlappingRoot(connectionID, &root); err != nil {
		return nil, err
	}
	normalizedScope, err := normalizeDirectoryScope(req.SyncScope)
	if err != nil {
		return nil, err
	}
	if err := validateDirectoryMappings(req.FieldMapping, req.FieldOwnership); err != nil {
		return nil, err
	}
	req.SyncScope = normalizedScope
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
	if !req.Enabled {
		if err := NewSyncJobService(s.db).CancelConnectionPending(connectionID, "目录同步策略已禁用"); err != nil {
			return nil, err
		}
	}
	return s.GetPolicy(connectionID)
}

func normalizeDirectoryScope(raw map[string]any) (map[string]any, error) {
	allowed := map[string]bool{"rootExternalOrgUnitIds": true, "includeDescendants": true, "includeInactive": true}
	for key := range raw {
		if !allowed[key] {
			return nil, fmt.Errorf("syncScope 不支持字段 %q", key)
		}
	}
	roots := []string{}
	if value, ok := raw["rootExternalOrgUnitIds"]; ok {
		switch typed := value.(type) {
		case []string:
			roots = append(roots, typed...)
		case []any:
			for _, item := range typed {
				if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
					roots = append(roots, strings.TrimSpace(text))
				} else {
					return nil, errors.New("rootExternalOrgUnitIds 必须是非空字符串数组")
				}
			}
		default:
			return nil, errors.New("rootExternalOrgUnitIds 必须是字符串数组")
		}
	}
	includeDescendants := true
	if value, ok := raw["includeDescendants"]; ok {
		var valid bool
		includeDescendants, valid = value.(bool)
		if !valid {
			return nil, errors.New("includeDescendants 必须是布尔值")
		}
	}
	includeInactive := false
	if value, ok := raw["includeInactive"]; ok {
		var valid bool
		includeInactive, valid = value.(bool)
		if !valid {
			return nil, errors.New("includeInactive 必须是布尔值")
		}
	}
	sort.Strings(roots)
	return map[string]any{"rootExternalOrgUnitIds": roots, "includeDescendants": includeDescendants, "includeInactive": includeInactive}, nil
}

func validateDirectoryMappings(mapping, ownership map[string]any) error {
	standard := map[string]bool{"username": true, "realName": true, "nickname": true, "email": true, "phone": true, "avatar": true, "employeeNo": true, "position": true}
	for target, source := range mapping {
		if !standard[target] {
			return fmt.Errorf("fieldMapping 目标字段 %q 不是标准字段", target)
		}
		if text, ok := source.(string); !ok || strings.TrimSpace(text) == "" {
			return fmt.Errorf("fieldMapping.%s 必须是非空字段名", target)
		}
	}
	for field, owner := range ownership {
		if !standard[field] {
			return fmt.Errorf("fieldOwnership 字段 %q 不是标准字段", field)
		}
		if owner != "upstream" && owner != "local" {
			return fmt.Errorf("fieldOwnership.%s 只能是 upstream 或 local", field)
		}
	}
	return nil
}

func (s *DirectorySyncService) ensureNonOverlappingRoot(connectionID uint, root *model.Department) error {
	var policies []model.DirectorySyncPolicy
	if err := s.db.Where("connection_id <> ?", connectionID).Find(&policies).Error; err != nil {
		return err
	}
	for _, policy := range policies {
		var other model.Department
		if err := s.db.First(&other, policy.RootDepartmentID).Error; err != nil {
			continue
		}
		rootPath := strings.TrimRight(root.Path, "/") + "/"
		otherPath := strings.TrimRight(other.Path, "/") + "/"
		if root.ID == other.ID || strings.HasPrefix(rootPath, otherPath) || strings.HasPrefix(otherPath, rootPath) {
			return fmt.Errorf("组织根与连接 %d 的组织根重叠（相同、祖先或后代均不允许）", policy.ConnectionID)
		}
	}
	return nil
}

func (s *DirectorySyncService) Diagnostic(ctx context.Context, connectionID uint) (*directoryprotocol.Diagnostic, error) {
	adapter, err := s.adapter(connectionID, false)
	if err != nil {
		return nil, err
	}
	if reader, ok := adapter.(directoryprotocol.SnapshotReader); ok {
		var policy model.DirectorySyncPolicy
		if err := s.db.Where("connection_id = ?", connectionID).First(&policy).Error; err != nil {
			return nil, err
		}
		scope := directoryprotocol.SyncScope{IncludeDescendants: true}
		encoded, _ := json.Marshal(policy.SyncScope)
		_ = json.Unmarshal(encoded, &scope)
		snapshot, readErr := reader.ReadSnapshot(ctx, scope)
		if snapshot == nil {
			return nil, readErr
		}
		completeness := snapshot.Completeness
		diagnostic := &directoryprotocol.Diagnostic{
			Healthy: readErr == nil && completeness.PaginationComplete, TokenValid: readErr == nil,
			DirectoryReadable: readErr == nil, VisibleOrgUnits: len(snapshot.OrgUnits), VisibleUsers: len(snapshot.Users),
			Warnings: completeness.Warnings, TraversedOrgUnitIDs: completeness.TraversedOrgUnitIDs,
			PaginationComplete: completeness.PaginationComplete, FieldPermissions: completeness.FieldPermissions, ScopeHash: completeness.ScopeHash,
		}
		if readErr != nil {
			diagnostic.Warnings = append(diagnostic.Warnings, readErr.Error())
		}
		return diagnostic, readErr
	}
	return adapter.ValidateDirectoryConfig(ctx)
}

func (s *DirectorySyncService) CreateManualRun(ctx context.Context, connectionID uint, actorID *uint) (*model.DirectorySyncRun, error) {
	return s.CreateRunWithOptions(ctx, connectionID, actorID, "manual", false)
}

func (s *DirectorySyncService) CreateManualRunWithSupersede(ctx context.Context, connectionID uint, actorID *uint, supersede bool) (*model.DirectorySyncRun, error) {
	return s.CreateRunWithOptions(ctx, connectionID, actorID, "manual", supersede)
}

// CreateRun is the single snapshot/diff entry point used by manual requests,
// schedules and event jobs. Automation must not duplicate the phase-02 logic.
func (s *DirectorySyncService) CreateRun(ctx context.Context, connectionID uint, actorID *uint, triggerType string) (*model.DirectorySyncRun, error) {
	return s.CreateRunWithOptions(ctx, connectionID, actorID, triggerType, false)
}

func (s *DirectorySyncService) CreateRunWithOptions(ctx context.Context, connectionID uint, actorID *uint, triggerType string, supersede bool) (*model.DirectorySyncRun, error) {
	policy, connection, adapter, err := s.ready(connectionID)
	if err != nil {
		return nil, err
	}
	lease, err := AcquireDirectoryLease(ctx, s.db, fmt.Sprintf("connection:%d", connectionID), nil, 2*time.Minute)
	if err != nil {
		return nil, err
	}
	stopRenewal := lease.StartRenewal(ctx)
	defer stopRenewal()
	defer lease.Release(context.Background())
	// Acquiring the connection lease proves that no previous owner is still
	// allowed to mutate this connection. Recover runs stranded by a crash before
	// deciding whether to coalesce, retry, or supersede them.
	recoveredAt := time.Now()
	if err := s.db.Model(&model.DirectorySyncRun{}).
		Where("connection_id = ? AND status = ?", connectionID, model.DirectoryRunFetching).
		Updates(map[string]any{"status": model.DirectoryRunSuperseded, "superseded_at": recoveredAt, "finished_at": recoveredAt, "error_message": "抓取进程中断，已由新租约取代"}).Error; err != nil {
		return nil, err
	}
	var approvable model.DirectorySyncRun
	err = s.db.Where("connection_id = ? AND status IN ?", connectionID, []model.DirectoryRunStatus{model.DirectoryRunPreviewReady, model.DirectoryRunApplying, model.DirectoryRunFailed}).Order("id DESC").First(&approvable).Error
	if err == nil {
		if approvable.Status == model.DirectoryRunApplying {
			if err := s.db.Model(&approvable).Updates(map[string]any{"status": model.DirectoryRunFailed, "finished_at": recoveredAt, "error_message": "应用进程中断，已由新租约接管"}).Error; err != nil {
				return nil, err
			}
			approvable.Status = model.DirectoryRunFailed
		}
		if triggerType != "manual" {
			return &approvable, nil
		}
		if !supersede {
			return nil, fmt.Errorf("连接已有待审批同步 run=%d；必须显式 supersede=true", approvable.ID)
		}
		if err := s.db.Model(&approvable).Updates(map[string]any{"status": model.DirectoryRunSuperseded, "superseded_at": recoveredAt}).Error; err != nil {
			return nil, err
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
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
	orgs, users, completeness, fetchErr := fetchDirectorySnapshot(ctx, adapter, policy.SyncScope)
	if fetchErr != nil {
		s.failRun(run.ID, fetchErr)
		return nil, fetchErr
	}
	if !completeness.PaginationComplete {
		fetchErr = errors.New("目录快照分页不完整，拒绝生成差异")
		s.failRun(run.ID, fetchErr)
		return nil, fetchErr
	}
	for _, field := range []string{"email", "phone"} {
		if directoryFieldRequiresReadPermission(policy, field) && completeness.FieldPermissions[field] != "granted" {
			fetchErr = fmt.Errorf("目录字段 %s 未取得读取权限，拒绝生成差异", field)
			s.failRun(run.ID, fetchErr)
			return nil, fetchErr
		}
	}
	if err := lease.Validate(ctx); err != nil {
		s.failRun(run.ID, err)
		return nil, err
	}
	run.ScopeHash = completeness.ScopeHash
	run.IsComplete = true
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return NewDirectorySyncServiceWithFactory(tx, s.adapterFactory).storeSnapshotAndChanges(run, policy, connection, orgs, users)
	})
	if err != nil {
		s.failRun(run.ID, err)
		return nil, err
	}
	return s.GetRun(run.ID)
}

func fetchDirectorySnapshot(ctx context.Context, adapter directoryprotocol.Adapter, rawScope model.JSONObject) ([]directoryprotocol.OrgUnit, []directoryprotocol.User, directoryprotocol.SnapshotCompleteness, error) {
	scope := directoryprotocol.SyncScope{IncludeDescendants: true}
	encodedScope, _ := json.Marshal(rawScope)
	_ = json.Unmarshal(encodedScope, &scope)
	if reader, ok := adapter.(directoryprotocol.SnapshotReader); ok {
		snapshot, err := reader.ReadSnapshot(ctx, scope)
		if err != nil {
			return nil, nil, snapshotCompleteness(snapshot), err
		}
		if snapshot == nil {
			return nil, nil, directoryprotocol.SnapshotCompleteness{}, errors.New("目录适配器返回空快照")
		}
		return snapshot.OrgUnits, snapshot.Users, snapshot.Completeness, nil
	}
	orgs := make([]directoryprotocol.OrgUnit, 0)
	seenOrgCursor := map[string]bool{}
	for cursor := ""; ; {
		if seenOrgCursor[cursor] {
			return nil, nil, directoryprotocol.SnapshotCompleteness{}, fmt.Errorf("目录部门分页游标循环: %q", cursor)
		}
		seenOrgCursor[cursor] = true
		page, err := adapter.ListOrgUnits(ctx, cursor)
		if err != nil {
			return nil, nil, directoryprotocol.SnapshotCompleteness{}, fmt.Errorf("读取目录部门失败: %w", err)
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
			return nil, nil, directoryprotocol.SnapshotCompleteness{}, fmt.Errorf("目录用户分页游标循环: %q", cursor)
		}
		seenUserCursor[cursor] = true
		page, err := adapter.ListUsers(ctx, cursor)
		if err != nil {
			return nil, nil, directoryprotocol.SnapshotCompleteness{}, fmt.Errorf("读取目录用户失败: %w", err)
		}
		for _, user := range page.Items {
			if user.StableID == "" {
				return nil, nil, directoryprotocol.SnapshotCompleteness{}, fmt.Errorf("目录返回了缺少稳定标识的用户")
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
	scopeBytes, _ := json.Marshal(scope)
	digest := sha256.Sum256(scopeBytes)
	return orgs, users, directoryprotocol.SnapshotCompleteness{PaginationComplete: true, ScopeHash: hex.EncodeToString(digest[:]), FieldPermissions: map[string]string{"email": "unknown", "phone": "unknown"}}, nil
}

func snapshotCompleteness(snapshot *directoryprotocol.Snapshot) directoryprotocol.SnapshotCompleteness {
	if snapshot == nil {
		return directoryprotocol.SnapshotCompleteness{}
	}
	return snapshot.Completeness
}

func (s *DirectorySyncService) storeSnapshotAndChanges(run *model.DirectorySyncRun, policy *model.DirectorySyncPolicy, connection *model.IdentityConnection, orgs []directoryprotocol.OrgUnit, users []directoryprotocol.User) error {
	now := time.Now()
	scopeChanged, err := s.scopeChangedSincePreviousRun(run)
	if err != nil {
		return err
	}
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
		if err := s.buildOrgChange(run, policy, &copyRecord, previousSnapshot, scopeChanged); err != nil {
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
	seenMemberships := map[string]bool{}
	membershipCount := 0
	contactOwners := map[string][]string{}
	mappedUsers := make([]directoryprotocol.User, 0, len(users))
	for _, rawItem := range users {
		item := applyDirectoryFieldMapping(rawItem, policy.FieldMapping)
		mappedUsers = append(mappedUsers, item)
		if item.Email != "" {
			key := "email:" + strings.ToLower(item.Email)
			contactOwners[key] = append(contactOwners[key], item.StableID)
		}
		if item.Phone != "" {
			key := "phone:" + item.Phone
			contactOwners[key] = append(contactOwners[key], item.StableID)
		}
	}
	duplicateContactUsers := map[string][]string{}
	for key, owners := range contactOwners {
		if len(owners) > 1 {
			for _, owner := range owners {
				duplicateContactUsers[owner] = append(duplicateContactUsers[owner], key)
			}
		}
	}
	for _, item := range mappedUsers {
		principal, err := s.upsertPrincipal(run, item)
		if err != nil {
			return err
		}
		seenPrincipals[principal.ID] = true
		if duplicates := duplicateContactUsers[item.StableID]; len(duplicates) > 0 {
			after := map[string]any{"principalId": principal.ID, "stableId": item.StableID, "duplicateContacts": duplicates, "displayName": item.DisplayName}
			if err := s.addChange(run, model.DirectoryChangeConflict, model.DirectoryObjectUser, item.StableID, "high", "完整外部快照中邮箱或手机号重复", nil, after); err != nil {
				return err
			}
		} else if err := s.buildUserChange(run, policy, principal, item); err != nil {
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
			seenMemberships[fmt.Sprintf("%d:%d", principal.ID, org.ID)] = true
			if err := s.upsertMembershipAndChange(run, principal, org, membership); err != nil {
				return err
			}
		}
	}
	var previousMemberships []model.ExternalMembership
	if err := s.db.Where("connection_id = ? AND snapshot_version <> ? AND status = ?", run.ConnectionID, run.SnapshotVersion, "active").Find(&previousMemberships).Error; err != nil {
		return err
	}
	for _, membership := range previousMemberships {
		key := fmt.Sprintf("%d:%d", membership.PrincipalID, membership.ExternalOrgUnitID)
		if seenMemberships[key] {
			continue
		}
		after := map[string]any{"membershipId": membership.ID, "principalId": membership.PrincipalID, "externalOrgUnitId": membership.ExternalOrgUnitID, "status": "inactive"}
		if err := s.addChange(run, model.DirectoryChangeDisable, model.DirectoryObjectMembership, fmt.Sprint(membership.ID), "high", "完整快照确认上游成员关系已撤销", map[string]any{"isPrimary": membership.IsPrimary, "isLeader": membership.IsLeader, "position": membership.Position}, after); err != nil {
			return err
		}
	}
	var missingPrincipals []model.ExternalPrincipal
	if err := s.db.Where("connection_id = ? AND last_synced_at < ? AND user_id IS NOT NULL", run.ConnectionID, now.Add(-time.Millisecond)).Find(&missingPrincipals).Error; err != nil {
		return err
	}
	for _, missing := range missingPrincipals {
		if !seenPrincipals[missing.ID] {
			if err := s.addChange(run, model.DirectoryChangeConflict, model.DirectoryObjectUser, fmt.Sprint(missing.ID), "high", "本次快照未出现，禁止据此停用身份或本地用户", map[string]any{"userId": missing.UserID, "displayName": missing.DisplayName}, map[string]any{"principalId": missing.ID}); err != nil {
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
		"finished_at": finished, "error_message": "", "scope_hash": run.ScopeHash, "is_complete": run.IsComplete,
	}).Error
}

func (s *DirectorySyncService) scopeChangedSincePreviousRun(run *model.DirectorySyncRun) (bool, error) {
	var previous model.DirectorySyncRun
	err := s.db.Where("connection_id = ? AND id < ? AND is_complete = ? AND scope_hash <> ''", run.ConnectionID, run.ID, true).
		Order("id DESC").First(&previous).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return previous.ScopeHash != run.ScopeHash, nil
}

func applyDirectoryFieldMapping(item directoryprotocol.User, mapping model.JSONObject) directoryprotocol.User {
	standard := map[string]string{"realName": item.DisplayName, "nickname": item.DisplayName, "email": item.Email, "phone": item.Phone, "avatar": item.Avatar, "employeeNo": item.EmployeeNo}
	value := func(source string) string {
		if result := standard[source]; result != "" {
			return result
		}
		if item.Raw != nil {
			return strings.TrimSpace(fmt.Sprint(item.Raw[source]))
		}
		return ""
	}
	for target, sourceRaw := range mapping {
		source, _ := sourceRaw.(string)
		if source == "" {
			continue
		}
		switch target {
		case "realName", "nickname":
			if mapped := value(source); mapped != "" {
				item.DisplayName = mapped
			}
		case "email":
			item.Email = value(source)
		case "phone":
			item.Phone = value(source)
		case "avatar":
			item.Avatar = value(source)
		case "employeeNo":
			item.EmployeeNo = value(source)
		}
	}
	return item
}

func (s *DirectorySyncService) buildOrgChange(run *model.DirectorySyncRun, policy *model.DirectorySyncPolicy, org *model.ExternalOrgUnit, previousSnapshot string, scopeChanged bool) error {
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
	expectedParentID := policy.RootDepartmentID
	if org.ExternalParentID != "" && org.ExternalParentID != "0" {
		var parent model.ExternalOrgUnit
		if err := s.db.Where("connection_id = ? AND external_id = ?", org.ConnectionID, org.ExternalParentID).First(&parent).Error; err == nil && parent.DepartmentID != nil {
			expectedParentID = *parent.DepartmentID
		}
	}
	if department.ParentID == nil || *department.ParentID != expectedParentID {
		after["departmentId"] = department.ID
		after["expectedParentDepartmentId"] = expectedParentID
		risk, reason := "medium", "外部部门父节点发生变化"
		if scopeChanged {
			risk, reason = "high", "同步 scope 发生变化，部门移动提升为高风险审批"
		}
		return s.addChange(run, model.DirectoryChangeMove, model.DirectoryObjectOrgUnit, org.ExternalID, risk, reason, map[string]any{"departmentId": department.ID, "parentId": department.ParentID}, after)
	}
	if department.Name != org.Name || department.Sort != org.SortOrder {
		after["departmentId"] = department.ID
		return s.addChange(run, model.DirectoryChangeUpdate, model.DirectoryObjectOrgUnit, org.ExternalID, "low", "已确认的历史部门映射", map[string]any{"departmentId": department.ID, "name": department.Name, "sortOrder": department.Sort, "snapshotVersion": previousSnapshot}, after)
	}
	return nil
}

func (s *DirectorySyncService) upsertPrincipal(run *model.DirectorySyncRun, item directoryprotocol.User) (*model.ExternalPrincipal, error) {
	inputs := []PrincipalIdentifierInput{{Namespace: model.IdentifierNamespaceDirectoryUserID, Value: item.StableID, Primary: true}}
	for _, identifier := range item.Identifiers {
		inputs = append(inputs, PrincipalIdentifierInput{Namespace: model.IdentifierNamespace(identifier.Namespace), Value: identifier.Value, Primary: identifier.Primary})
	}
	principal, err := NewExternalPrincipalResolver(s.db).ResolveForDirectory(run.ConnectionID, inputs, true)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	principal.DisplayName, principal.Email, principal.Phone, principal.Avatar = item.DisplayName, item.Email, item.Phone, item.Avatar
	principal.RawProfile, principal.LastSyncedAt = model.JSONObject(item.Raw), &now
	if item.Status == "inactive" {
		principal.Status = model.PrincipalStatusBlocked
	} else if principal.UserID != nil {
		principal.Status = model.PrincipalStatusActive
	} else {
		principal.Status = model.PrincipalStatusUnmatched
	}
	if err := s.db.Save(principal).Error; err != nil {
		return nil, err
	}
	return principal, nil
}

func (s *DirectorySyncService) buildUserChange(run *model.DirectorySyncRun, policy *model.DirectorySyncPolicy, principal *model.ExternalPrincipal, item directoryprotocol.User) error {
	after := map[string]any{"principalId": principal.ID, "stableId": item.StableID, "username": deterministicCode("sync", run.ConnectionID, item.StableID), "displayName": item.DisplayName, "email": item.Email, "phone": item.Phone, "avatar": item.Avatar, "employeeNo": item.EmployeeNo, "status": item.Status}
	if principal.UserID != nil {
		after["userId"] = *principal.UserID
		if item.Status == "inactive" {
			return s.addChange(run, model.DirectoryChangeDisable, model.DirectoryObjectUser, item.StableID, "high", "上游明确返回离职或禁用；企业身份已立即封禁，本地用户停用需审批", map[string]any{"userId": *principal.UserID}, after)
		}
		var user model.User
		if err := s.db.First(&user, *principal.UserID).Error; err != nil {
			return s.addChange(run, model.DirectoryChangeConflict, model.DirectoryObjectUser, item.StableID, "high", "历史映射的本地用户不存在", map[string]any{"userId": *principal.UserID}, after)
		}
		changed := (upstreamOwns(policy, "realName") && user.RealName != item.DisplayName) ||
			(upstreamOwns(policy, "email") && pointerString(user.Email) != item.Email) ||
			(upstreamOwns(policy, "phone") && pointerString(user.Phone) != item.Phone) ||
			(upstreamOwns(policy, "avatar") && user.Avatar != item.Avatar)
		if changed {
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
	payload := model.JSONObject(after)
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(encoded)
	snapshot := model.DirectorySyncSnapshotObject{RunID: run.ID, ConnectionID: run.ConnectionID, ObjectType: objectType, ObjectKey: key, Payload: payload, ContentHash: hex.EncodeToString(digest[:])}
	if err := s.db.Where("run_id = ? AND object_type = ? AND object_key = ?", run.ID, objectType, key).FirstOrCreate(&snapshot).Error; err != nil {
		return err
	}
	if snapshot.ContentHash != hex.EncodeToString(digest[:]) {
		return fmt.Errorf("run %d 的不可变快照对象 %s/%s 内容发生变化", run.ID, objectType, key)
	}
	change := &model.DirectorySyncChange{
		RunID: run.ID, ConnectionID: run.ConnectionID, ChangeType: changeType, ObjectType: objectType,
		ObjectKey: key, Status: model.DirectoryChangePending, RiskLevel: risk,
		BeforeData: model.JSONObject(before), AfterData: payload, MatchReason: reason, SnapshotObjectID: &snapshot.ID,
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
	run, err := s.GetRun(runID)
	if err != nil {
		return err
	}
	if run.Status == model.DirectoryRunSuperseded || !run.IsComplete {
		return errors.New("同步快照不完整或已被取代，不能审批")
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
	run, err := s.GetRun(runID)
	if err != nil {
		return err
	}
	if run.Status == model.DirectoryRunSuperseded || !run.IsComplete || run.SupersededAt != nil {
		return errors.New("同步快照不完整或已被取代，不能自动审批")
	}
	now := time.Now()
	return s.db.Model(&model.DirectorySyncChange{}).
		Where("run_id = ? AND status = ? AND risk_level = ? AND change_type IN ?", runID, model.DirectoryChangePending, "low", []model.DirectoryChangeType{model.DirectoryChangeCreate, model.DirectoryChangeUpdate}).
		Updates(map[string]any{"status": model.DirectoryChangeApproved, "reviewed_at": now}).Error
}

func (s *DirectorySyncService) ApplyRun(runID, actorID uint) (*model.DirectorySyncRun, error) {
	return s.ApplyRunWithGuard(context.Background(), runID, actorID, nil)
}

func (s *DirectorySyncService) ApplyRunWithGuard(ctx context.Context, runID, actorID uint, guard func(context.Context) error) (*model.DirectorySyncRun, error) {
	run, err := s.GetRun(runID)
	if err != nil {
		return nil, err
	}
	lease, err := AcquireDirectoryLease(ctx, s.db, fmt.Sprintf("connection:%d", run.ConnectionID), &runID, 2*time.Minute)
	if err != nil {
		return nil, err
	}
	stopRenewal := lease.StartRenewal(ctx)
	defer stopRenewal()
	defer lease.Release(context.Background())
	if guard != nil {
		if err := guard(ctx); err != nil {
			return nil, err
		}
	}
	if err := lease.Validate(ctx); err != nil {
		return nil, err
	}
	run, err = s.GetRun(runID)
	if err != nil {
		return nil, err
	}
	if run.Status != model.DirectoryRunPreviewReady && run.Status != model.DirectoryRunFailed {
		return nil, fmt.Errorf("只有预览完成或应用失败的同步可以执行")
	}
	if !run.IsComplete || run.SupersededAt != nil {
		return nil, errors.New("同步快照不完整或已被取代，不能应用")
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
	sortDirectoryChanges(changes)
	for i := range changes {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := lease.Validate(ctx); err != nil {
			return nil, err
		}
		if guard != nil {
			if err := guard(ctx); err != nil {
				return nil, err
			}
		}
		change := changes[i]
		err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			return NewDirectorySyncServiceWithFactory(tx, s.adapterFactory).applyChange(&change, &policy)
		})
		if err != nil {
			now := time.Now()
			_ = s.db.Model(&change).Updates(map[string]any{"status": model.DirectoryChangeFailed, "error_message": err.Error(), "attempt_count": gorm.Expr("attempt_count + 1"), "last_attempt_at": now}).Error
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

func sortDirectoryChanges(changes []model.DirectorySyncChange) {
	byOrg := map[string]model.DirectorySyncChange{}
	for _, change := range changes {
		if change.ObjectType == model.DirectoryObjectOrgUnit {
			byOrg[change.ObjectKey] = change
		}
	}
	var depth func(string, map[string]bool) int
	depth = func(key string, visiting map[string]bool) int {
		if visiting[key] {
			return 0
		}
		change, ok := byOrg[key]
		if !ok {
			return 0
		}
		parent := stringValue(change.AfterData["parentExternalId"])
		if parent == "" {
			return 0
		}
		visiting[key] = true
		value := 1 + depth(parent, visiting)
		delete(visiting, key)
		return value
	}
	rank := func(change model.DirectorySyncChange) int {
		if change.ObjectType == model.DirectoryObjectOrgUnit {
			return 0
		}
		if change.ObjectType == model.DirectoryObjectUser {
			return 1
		}
		return 2
	}
	sort.SliceStable(changes, func(i, j int) bool {
		ri, rj := rank(changes[i]), rank(changes[j])
		if ri != rj {
			return ri < rj
		}
		if ri != 0 {
			return changes[i].ID < changes[j].ID
		}
		di, dj := depth(changes[i].ObjectKey, map[string]bool{}), depth(changes[j].ObjectKey, map[string]bool{})
		if changes[i].ChangeType == model.DirectoryChangeDelete {
			di = -di
		}
		if changes[j].ChangeType == model.DirectoryChangeDelete {
			dj = -dj
		}
		if di != dj {
			return di < dj
		}
		return changes[i].ID < changes[j].ID
	})
}

func (s *DirectorySyncService) RetryFailedChange(changeID, actorID uint) error {
	now := time.Now()
	result := s.db.Model(&model.DirectorySyncChange{}).Where("id = ? AND status = ?", changeID, model.DirectoryChangeFailed).
		Updates(map[string]any{"status": model.DirectoryChangeApproved, "error_message": "", "reviewed_by": actorID, "reviewed_at": now})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("变更不存在或不是 failed 状态")
	}
	return nil
}

func (s *DirectorySyncService) applyChange(change *model.DirectorySyncChange, policy *model.DirectorySyncPolicy) error {
	if change.Status == model.DirectoryChangeApplied {
		return nil
	}
	if change.SnapshotObjectID == nil {
		return errors.New("变更缺少不可变快照引用")
	}
	var snapshot model.DirectorySyncSnapshotObject
	if err := s.db.First(&snapshot, *change.SnapshotObjectID).Error; err != nil {
		return err
	}
	encoded, err := json.Marshal(snapshot.Payload)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(encoded)
	if snapshot.RunID != change.RunID || snapshot.ObjectType != change.ObjectType || snapshot.ObjectKey != change.ObjectKey || snapshot.ContentHash != hex.EncodeToString(digest[:]) {
		return errors.New("审批快照与待应用变更不一致")
	}
	change.AfterData = snapshot.Payload
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
	return s.db.Model(change).Updates(map[string]any{"status": model.DirectoryChangeApplied, "applied_at": now, "error_message": "", "attempt_count": gorm.Expr("attempt_count + 1"), "last_attempt_at": now}).Error
}

func (s *DirectorySyncService) applyOrgChange(change *model.DirectorySyncChange, policy *model.DirectorySyncPolicy) error {
	orgID := uintValue(change.AfterData["externalOrgUnitId"])
	var org model.ExternalOrgUnit
	if err := s.db.First(&org, orgID).Error; err != nil {
		return err
	}
	if change.ChangeType == model.DirectoryChangeDelete {
		if org.DepartmentID == nil {
			return errors.New("外部部门尚未映射，无需删除本地部门")
		}
		if *org.DepartmentID == policy.RootDepartmentID {
			return errors.New("组织根禁止删除")
		}
		var department model.Department
		if err := s.db.First(&department, *org.DepartmentID).Error; err != nil {
			return err
		}
		var attrs map[string]any
		_ = json.Unmarshal(department.Attributes, &attrs)
		if uintValue(attrs["directoryConnectionId"]) != org.ConnectionID {
			return errors.New("部门不属于当前目录连接，禁止删除")
		}
		var childCount, memberCount int64
		if err := s.db.Model(&model.Department{}).Where("parent_id = ?", department.ID).Count(&childCount).Error; err != nil {
			return err
		}
		if err := s.db.Model(&model.UserDepartment{}).Where("department_id = ? AND is_active = ?", department.ID, true).Count(&memberCount).Error; err != nil {
			return err
		}
		if childCount > 0 || memberCount > 0 {
			return fmt.Errorf("部门仍有 %d 个子部门或 %d 个成员，生成冲突且禁止删除", childCount, memberCount)
		}
		if err := s.db.Delete(&department).Error; err != nil {
			return err
		}
		return s.db.Model(&org).Update("status", "inactive").Error
	}
	// Desired values must come from the immutable snapshot object validated by
	// applyChange. external_org_units is only a mapping table and may already
	// contain data fetched by a later run.
	desiredName := stringValue(change.AfterData["name"])
	desiredParentExternalID := stringValue(change.AfterData["parentExternalId"])
	desiredSortOrder := int(uintValue(change.AfterData["sortOrder"]))
	if desiredName == "" {
		return errors.New("部门快照缺少名称")
	}
	if org.DepartmentID != nil {
		ok, err := s.departmentWithinRoot(*org.DepartmentID, policy.RootDepartmentID)
		if err != nil || !ok {
			return fmt.Errorf("本地部门不在策略组织根内")
		}
		if change.ChangeType == model.DirectoryChangeMove {
			parentID := policy.RootDepartmentID
			if desiredParentExternalID != "" && desiredParentExternalID != "0" {
				var parent model.ExternalOrgUnit
				if err := s.db.Where("connection_id = ? AND external_id = ?", org.ConnectionID, desiredParentExternalID).First(&parent).Error; err != nil || parent.DepartmentID == nil {
					return errors.New("移动目标父部门尚未映射")
				}
				parentID = *parent.DepartmentID
			}
			if err := dao.NewDepartmentDao(s.db).MoveDepartment(*org.DepartmentID, &parentID, desiredSortOrder); err != nil {
				return err
			}
		}
		return s.db.Model(&model.Department{}).Where("id = ?", *org.DepartmentID).Updates(map[string]any{"name": desiredName, "sort": desiredSortOrder}).Error
	}
	parentID := policy.RootDepartmentID
	if desiredParentExternalID != "" && desiredParentExternalID != "0" {
		var parentOrg model.ExternalOrgUnit
		if err := s.db.Where("connection_id = ? AND external_id = ?", org.ConnectionID, desiredParentExternalID).First(&parentOrg).Error; err == nil && parentOrg.DepartmentID != nil {
			parentID = *parentOrg.DepartmentID
		}
	}
	ok, err := s.departmentWithinRoot(parentID, policy.RootDepartmentID)
	if err != nil || !ok {
		return fmt.Errorf("外部父部门映射不在策略组织根内")
	}
	department := model.Department{
		Name: desiredName, Code: deterministicCode("dir", org.ConnectionID, org.ExternalID),
		ShortName: desiredName, Type: model.DepartmentTypeDepartment, Status: model.DepartmentStatusEnabled,
		ParentID: &parentID, Sort: desiredSortOrder, IsLeaf: true, DataScope: 2,
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
		if err := RevokeLoginSessions(s.db, *principal.UserID, "", "", "user_disabled", nil); err != nil {
			return err
		}
		return dao.NewOIDCTokenDAO(s.db).RevokeByUser(*principal.UserID)
	}
	userID := uintValue(change.AfterData["userId"])
	if change.ChangeType == model.DirectoryChangeCreate && userID == 0 {
		email := nullableString(stringValue(change.AfterData["email"]))
		phone := nullableString(stringValue(change.AfterData["phone"]))
		user := model.User{
			Username: defaultString(stringValue(change.AfterData["username"]), deterministicCode("sync", change.ConnectionID, change.ObjectKey)), Email: email, Phone: phone,
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
			if err := s.db.Transaction(func(tx *gorm.DB) error {
				if err := dao.LockLoginUser(tx, userID); err != nil {
					return err
				}
				var current model.User
				if err := tx.First(&current, userID).Error; err != nil {
					return err
				}
				contactChanged := false
				if value, exists := updates["email"]; exists && value != stringPointerValue(current.Email) {
					updates["email_verified_at"] = nil
					contactChanged = true
				}
				if value, exists := updates["phone"]; exists && value != stringPointerValue(current.Phone) {
					updates["phone_verified_at"] = nil
					contactChanged = true
				}
				if err := tx.Model(&model.User{}).Where("id = ?", userID).Updates(updates).Error; err != nil {
					return err
				}
				if contactChanged {
					return InvalidateAuthenticationConfiguration(tx, userID, "directory_contact_changed")
				}
				return nil
			}); err != nil {
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
	if change.ChangeType == model.DirectoryChangeDisable {
		var relation model.UserDepartment
		err := s.db.Unscoped().Where("user_id = ? AND department_id = ?", *membership.Principal.UserID, *membership.OrgUnit.DepartmentID).First(&relation).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return s.db.Model(&membership).Update("status", "inactive").Error
		}
		if err != nil {
			return err
		}
		if uintValue(relation.Attributes["directoryConnectionId"]) != membership.ConnectionID {
			return fmt.Errorf("成员关系不属于当前目录连接，禁止撤销")
		}
		now := time.Now()
		if err := s.db.Model(&relation).Updates(map[string]any{"is_active": false, "is_primary": false, "left_at": now}).Error; err != nil {
			return err
		}
		if err := s.db.Delete(&relation).Error; err != nil {
			return err
		}
		if err := s.db.Model(&membership).Updates(map[string]any{"status": "inactive", "user_id": relation.UserID, "department_id": relation.DepartmentID}).Error; err != nil {
			return err
		}
		return s.recalculatePrimaryDepartment(relation.UserID)
	}
	ok, err := s.departmentWithinRoot(*membership.OrgUnit.DepartmentID, policy.RootDepartmentID)
	if err != nil || !ok {
		return fmt.Errorf("成员关系目标部门不在策略组织根内")
	}
	desiredPrimary := boolValue(change.AfterData["isPrimary"])
	desiredLeader := boolValue(change.AfterData["isLeader"])
	desiredPosition := stringValue(change.AfterData["position"])
	relation := model.UserDepartment{
		UserID: *membership.Principal.UserID, DepartmentID: *membership.OrgUnit.DepartmentID,
		Position: desiredPosition, Type: model.UserDepartmentTypeMember,
		IsActive: true, IsPrimary: desiredPrimary,
		Attributes: model.JSONObject{"directoryConnectionId": membership.ConnectionID, "externalMembershipId": membership.ID},
	}
	if desiredLeader {
		relation.Type = model.UserDepartmentTypeLeader
	}
	if relation.IsPrimary {
		if err := s.db.Model(&model.UserDepartment{}).Where("user_id = ? AND department_id <> ?", relation.UserID, relation.DepartmentID).Update("is_primary", false).Error; err != nil {
			return err
		}
	}
	if err := s.db.Unscoped().Where("user_id = ? AND department_id = ?", relation.UserID, relation.DepartmentID).
		Assign(map[string]any{"position": relation.Position, "type": relation.Type, "is_active": true, "is_primary": relation.IsPrimary, "deleted_at": nil, "attributes": relation.Attributes}).
		FirstOrCreate(&relation).Error; err != nil {
		return err
	}
	return s.db.Model(&membership).Updates(map[string]any{"user_id": relation.UserID, "department_id": relation.DepartmentID}).Error
}

func boolValue(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case float64:
		return typed != 0
	case int:
		return typed != 0
	case int64:
		return typed != 0
	default:
		return false
	}
}

func (s *DirectorySyncService) recalculatePrimaryDepartment(userID uint) error {
	var relations []model.UserDepartment
	if err := s.db.Where("user_id = ? AND is_active = ?", userID, true).Order("is_primary DESC, created_at ASC").Find(&relations).Error; err != nil {
		return err
	}
	if len(relations) == 0 {
		return nil
	}
	primaryID := relations[0].DepartmentID
	if err := s.db.Model(&model.UserDepartment{}).Where("user_id = ?", userID).Update("is_primary", false).Error; err != nil {
		return err
	}
	return s.db.Model(&model.UserDepartment{}).Where("user_id = ? AND department_id = ?", userID, primaryID).Update("is_primary", true).Error
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

func directoryFieldRequiresReadPermission(policy *model.DirectorySyncPolicy, field string) bool {
	if policy == nil {
		return false
	}
	_, mapped := policy.FieldMapping[field]
	if _, ownershipSpecified := policy.FieldOwnership[field]; ownershipSpecified {
		return upstreamOwns(policy, field)
	}
	// Preserve existing policies where adding an explicit mapping meant the
	// directory was expected to own and provide that field.
	return mapped
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
