package service

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	directoryprotocol "cnb.cool/mliev/iam/mliev-iam/app/service/protocol/directory"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type staticDirectoryAdapter struct {
	orgs  []directoryprotocol.OrgUnit
	users []directoryprotocol.User
}

func (a *staticDirectoryAdapter) ValidateDirectoryConfig(context.Context) (*directoryprotocol.Diagnostic, error) {
	return &directoryprotocol.Diagnostic{Healthy: true, TokenValid: true, DirectoryReadable: true, VisibleOrgUnits: len(a.orgs), VisibleUsers: len(a.users)}, nil
}

func (a *staticDirectoryAdapter) ListOrgUnits(_ context.Context, cursor string) (*directoryprotocol.OrgUnitPage, error) {
	if cursor != "" {
		return &directoryprotocol.OrgUnitPage{}, nil
	}
	return &directoryprotocol.OrgUnitPage{Items: a.orgs}, nil
}

func (a *staticDirectoryAdapter) ListUsers(_ context.Context, cursor string) (*directoryprotocol.UserPage, error) {
	if cursor != "" {
		return &directoryprotocol.UserPage{}, nil
	}
	return &directoryprotocol.UserPage{Items: a.users}, nil
}

func TestDirectorySyncPreviewIsolatedAndIdempotent(t *testing.T) {
	db := openDirectorySyncTestDB(t)
	connection, root := seedDirectoryConnection(t, db, "feishu-main")
	adapter := &staticDirectoryAdapter{
		orgs: []directoryprotocol.OrgUnit{{ExternalID: "od-rd", ExternalParentID: "0", Name: "研发部", SortOrder: 10, LeaderExternalID: "u-1", Status: "active"}},
		users: []directoryprotocol.User{{
			StableID: "tenant-a:u-1", TenantID: "tenant-a", ProviderUserID: "u-1", DisplayName: "Ada", Email: "ada@example.com", Phone: "+8613800000000", Avatar: "https://img.test/ada.png", Status: "active",
			Identifiers: []directoryprotocol.Identifier{{Namespace: "directory_user_id", Value: "tenant-a:u-1", Primary: true}, {Namespace: "open_id", Value: "ou-1"}, {Namespace: "union_id", Value: "on-1"}},
			Memberships: []directoryprotocol.Membership{{OrgUnitExternalID: "od-rd", Primary: true, Leader: true, Position: "负责人"}},
		}},
	}
	service := NewDirectorySyncServiceWithFactory(db, func(*model.IdentityConnection, map[string]any) (directoryprotocol.Adapter, error) {
		return adapter, nil
	})
	_, err := service.SavePolicy(connection.ID, &dto.DirectoryPolicyRequest{
		RootDepartmentID: root.ID, MatchPolicy: model.DirectoryMatchCreateUnmatched,
		Enabled: true, Credentials: map[string]any{"appId": "app", "appSecret": "secret", "tenantKey": "tenant-a"},
	})
	require.NoError(t, err)

	usersBefore, departmentsBefore, membershipsBefore := localDirectoryCounts(t, db)
	run, err := service.CreateManualRun(context.Background(), connection.ID, nil)
	require.NoError(t, err)
	require.Equal(t, model.DirectoryRunPreviewReady, run.Status)
	require.Equal(t, 3, run.ChangeCount)
	usersAfter, departmentsAfter, membershipsAfter := localDirectoryCounts(t, db)
	require.Equal(t, usersBefore, usersAfter, "预览不得创建本地用户")
	require.Equal(t, departmentsBefore, departmentsAfter, "预览不得创建本地部门")
	require.Equal(t, membershipsBefore, membershipsAfter, "预览不得创建本地成员关系")

	// Simulate a later fetch overwriting the mutable mirror tables before the
	// administrator applies this run. Application must still use run snapshot.
	var mirroredOrg model.ExternalOrgUnit
	require.NoError(t, db.Where("connection_id = ? AND external_id = ?", connection.ID, "od-rd").First(&mirroredOrg).Error)
	require.NoError(t, db.Model(&mirroredOrg).Updates(map[string]any{"name": "被后续抓取覆盖", "sort_order": 999}).Error)
	var mirroredMembership model.ExternalMembership
	require.NoError(t, db.Where("connection_id = ?", connection.ID).First(&mirroredMembership).Error)
	require.NoError(t, db.Model(&mirroredMembership).Updates(map[string]any{"position": "错误职位", "is_primary": false, "is_leader": false}).Error)

	require.NoError(t, service.ApproveRun(run.ID, 1))
	applied, err := service.ApplyRun(run.ID, 1)
	require.NoError(t, err)
	require.Equal(t, model.DirectoryRunSucceeded, applied.Status)
	require.EqualValues(t, 3, applied.AppliedCount)
	usersAfter, departmentsAfter, membershipsAfter = localDirectoryCounts(t, db)
	require.Equal(t, usersBefore+1, usersAfter)
	require.Equal(t, departmentsBefore+1, departmentsAfter)
	require.Equal(t, membershipsBefore+1, membershipsAfter)
	require.NoError(t, db.First(&mirroredOrg, mirroredOrg.ID).Error)
	require.NotNil(t, mirroredOrg.DepartmentID)
	var localDepartment model.Department
	require.NoError(t, db.First(&localDepartment, *mirroredOrg.DepartmentID).Error)
	require.Equal(t, "研发部", localDepartment.Name)
	require.Equal(t, 10, localDepartment.Sort)
	var localMembership model.UserDepartment
	require.NoError(t, db.Where("department_id = ?", localDepartment.ID).First(&localMembership).Error)
	require.True(t, localMembership.IsPrimary)
	require.Equal(t, model.UserDepartmentTypeLeader, localMembership.Type)
	require.Equal(t, "负责人", localMembership.Position)

	var principal model.ExternalPrincipal
	require.NoError(t, db.Where("connection_id = ?", connection.ID).First(&principal).Error)
	require.NotNil(t, principal.UserID)
	require.Equal(t, model.PrincipalStatusActive, principal.Status)

	secondRun, err := service.CreateManualRun(context.Background(), connection.ID, nil)
	require.NoError(t, err)
	require.Zero(t, secondRun.ChangeCount, "相同快照重复执行不应产生重复差异")
}

func TestDirectorySyncUniqueContactRequiresApprovalAndRootIsExclusive(t *testing.T) {
	db := openDirectorySyncTestDB(t)
	connection, root := seedDirectoryConnection(t, db, "feishu-a")
	email := "existing@example.com"
	existing := model.User{Username: "existing", Email: &email, RealName: "Existing", Status: model.UserStatusActive, IsActivated: true}
	require.NoError(t, db.Create(&existing).Error)
	adapter := &staticDirectoryAdapter{users: []directoryprotocol.User{{StableID: "tenant-a:u-2", DisplayName: "External", Email: email, Status: "active", Identifiers: []directoryprotocol.Identifier{{Namespace: "directory_user_id", Value: "tenant-a:u-2", Primary: true}}}}}
	service := NewDirectorySyncServiceWithFactory(db, func(*model.IdentityConnection, map[string]any) (directoryprotocol.Adapter, error) {
		return adapter, nil
	})
	_, err := service.SavePolicy(connection.ID, &dto.DirectoryPolicyRequest{RootDepartmentID: root.ID, MatchPolicy: model.DirectoryMatchManual, Enabled: true, Credentials: map[string]any{"appId": "a", "appSecret": "s", "tenantKey": "tenant-a"}})
	require.NoError(t, err)
	run, err := service.CreateManualRun(context.Background(), connection.ID, nil)
	require.NoError(t, err)
	var change model.DirectorySyncChange
	require.NoError(t, db.Where("run_id = ? AND object_type = ?", run.ID, model.DirectoryObjectUser).First(&change).Error)
	require.Equal(t, model.DirectoryChangeLink, change.ChangeType)
	require.Equal(t, "medium", change.RiskLevel)
	var principal model.ExternalPrincipal
	require.NoError(t, db.Where("connection_id = ?", connection.ID).First(&principal).Error)
	require.Nil(t, principal.UserID, "唯一联系方式只应生成待确认匹配")

	second, _ := seedDirectoryConnection(t, db, "feishu-b")
	secondService := NewDirectorySyncServiceWithFactory(db, func(*model.IdentityConnection, map[string]any) (directoryprotocol.Adapter, error) {
		return adapter, nil
	})
	_, err = secondService.SavePolicy(second.ID, &dto.DirectoryPolicyRequest{RootDepartmentID: root.ID, MatchPolicy: model.DirectoryMatchManual, Enabled: false})
	require.ErrorContains(t, err, "组织根重叠")
}

func TestDirectoryDeprovisionKeepsUserWithOtherIdentityThenRevokesSessions(t *testing.T) {
	db := openDirectorySyncTestDB(t)
	connection, root := seedDirectoryConnection(t, db, "feishu-deprovision")
	adapter := &staticDirectoryAdapter{
		orgs: []directoryprotocol.OrgUnit{{ExternalID: "od-1", ExternalParentID: "0", Name: "研发", Status: "active"}},
		users: []directoryprotocol.User{{
			StableID: "tenant:u-1", DisplayName: "Ada", Status: "active",
			Identifiers: []directoryprotocol.Identifier{{Namespace: "directory_user_id", Value: "tenant:u-1", Primary: true}},
			Memberships: []directoryprotocol.Membership{{OrgUnitExternalID: "od-1", Primary: true}},
		}},
	}
	service := NewDirectorySyncServiceWithFactory(db, func(*model.IdentityConnection, map[string]any) (directoryprotocol.Adapter, error) {
		return adapter, nil
	})
	_, err := service.SavePolicy(connection.ID, &dto.DirectoryPolicyRequest{RootDepartmentID: root.ID, MatchPolicy: model.DirectoryMatchCreateUnmatched, Enabled: true, Credentials: map[string]any{"appId": "a", "appSecret": "s", "tenantKey": "tenant"}})
	require.NoError(t, err)
	initial, err := service.CreateManualRun(context.Background(), connection.ID, nil)
	require.NoError(t, err)
	require.NoError(t, service.ApproveRun(initial.ID, 1))
	_, err = service.ApplyRun(initial.ID, 1)
	require.NoError(t, err)
	var principal model.ExternalPrincipal
	require.NoError(t, db.Where("connection_id = ?", connection.ID).First(&principal).Error)
	require.NotNil(t, principal.UserID)

	otherConnection := &model.IdentityConnection{Name: "其他有效身份", Identifier: "other-identity", Category: model.ConnectionCategorySocial, ProviderType: "github", Capabilities: model.StringList{"authentication"}, Status: model.ConnectionStatusEnabled}
	require.NoError(t, db.Create(otherConnection).Error)
	otherPrincipal := &model.ExternalPrincipal{ConnectionID: otherConnection.ID, UserID: principal.UserID, Status: model.PrincipalStatusActive, DisplayName: "Ada"}
	require.NoError(t, db.Create(otherPrincipal).Error)
	token := &model.OIDCToken{AccessToken: "access-token", ClientID: "client", UserID: *principal.UserID, ExpiresAt: time.Now().Add(time.Hour)}
	require.NoError(t, db.Create(token).Error)

	adapter.users[0].Status = "inactive"
	inactiveRun, err := service.CreateManualRun(context.Background(), connection.ID, nil)
	require.NoError(t, err)
	var disable model.DirectorySyncChange
	require.NoError(t, db.Where("run_id = ? AND change_type = ?", inactiveRun.ID, model.DirectoryChangeDisable).First(&disable).Error)
	require.NoError(t, service.DecideChange(disable.ID, true, 1))
	_, err = service.ApplyRun(inactiveRun.ID, 1)
	require.NoError(t, err)
	var user model.User
	require.NoError(t, db.First(&user, *principal.UserID).Error)
	require.Equal(t, model.UserStatusActive, user.Status, "存在其他有效身份时不得停用整个用户")
	require.NoError(t, db.First(token, token.ID).Error)
	require.False(t, token.IsRevoked)

	require.NoError(t, db.Model(otherPrincipal).Update("status", model.PrincipalStatusBlocked).Error)
	finalRun, err := service.CreateManualRun(context.Background(), connection.ID, nil)
	require.NoError(t, err)
	disable = model.DirectorySyncChange{}
	require.NoError(t, db.Where("run_id = ? AND change_type = ?", finalRun.ID, model.DirectoryChangeDisable).First(&disable).Error)
	require.NoError(t, service.DecideChange(disable.ID, true, 1))
	_, err = service.ApplyRun(finalRun.ID, 1)
	require.NoError(t, err)
	require.NoError(t, db.First(&user, user.ID).Error)
	require.Equal(t, model.UserStatusDisabled, user.Status)
	require.NotNil(t, user.SessionRevokedAt)
	require.NoError(t, db.First(token, token.ID).Error)
	require.True(t, token.IsRevoked, "最终停用应撤销下游应用令牌")
}

func TestDirectoryRunSupersedePreventsOldSnapshotApproval(t *testing.T) {
	db := openDirectorySyncTestDB(t)
	connection, root := seedDirectoryConnection(t, db, "feishu-supersede")
	adapter := &staticDirectoryAdapter{users: []directoryprotocol.User{{
		StableID: "tenant:user-1", DisplayName: "First", Status: "active",
		Identifiers: []directoryprotocol.Identifier{{Namespace: "directory_user_id", Value: "tenant:user-1", Primary: true}},
	}}}
	service := NewDirectorySyncServiceWithFactory(db, func(*model.IdentityConnection, map[string]any) (directoryprotocol.Adapter, error) {
		return adapter, nil
	})
	_, err := service.SavePolicy(connection.ID, &dto.DirectoryPolicyRequest{
		RootDepartmentID: root.ID, MatchPolicy: model.DirectoryMatchCreateUnmatched, Enabled: true,
		Credentials: map[string]any{"appId": "app", "appSecret": "secret", "tenantKey": "tenant"},
	})
	require.NoError(t, err)
	first, err := service.CreateManualRun(context.Background(), connection.ID, nil)
	require.NoError(t, err)
	_, err = service.CreateManualRun(context.Background(), connection.ID, nil)
	require.ErrorContains(t, err, "supersede=true")

	adapter.users[0].DisplayName = "Second"
	second, err := service.CreateManualRunWithSupersede(context.Background(), connection.ID, nil, true)
	require.NoError(t, err)
	require.NotEqual(t, first.ID, second.ID)
	require.NoError(t, db.First(first, first.ID).Error)
	require.Equal(t, model.DirectoryRunSuperseded, first.Status)
	require.NotNil(t, first.SupersededAt)
	require.ErrorContains(t, service.ApproveRun(first.ID, 1), "已被取代")
	require.ErrorContains(t, service.ApproveAutomationSafeChanges(first.ID), "已被取代")
}

func TestDirectoryOperationLeaseTakeoverKeepsNewOwner(t *testing.T) {
	db := openDirectorySyncTestDB(t)
	ctx := context.Background()
	first, err := AcquireDirectoryLease(ctx, db, "connection:42", nil, time.Minute)
	require.NoError(t, err)
	_, err = AcquireDirectoryLease(ctx, db, "connection:42", nil, time.Minute)
	require.ErrorContains(t, err, "已由")

	require.NoError(t, db.Model(&model.DirectoryOperationLease{}).Where("lease_key = ?", "connection:42").Update("lease_expires_at", time.Now().Add(-time.Second)).Error)
	second, err := AcquireDirectoryLease(ctx, db, "connection:42", nil, time.Minute)
	require.NoError(t, err)
	require.ErrorContains(t, first.Validate(ctx), "已丢失")
	require.NoError(t, first.Release(ctx), "旧 owner 释放不得删除新 owner 的租约")
	require.NoError(t, second.Validate(ctx))
	require.NoError(t, second.Release(ctx))
}

func TestDirectoryFieldPermissionFollowsExplicitOwnership(t *testing.T) {
	policy := &model.DirectorySyncPolicy{
		FieldMapping:   model.JSONObject{"email": "email", "phone": "mobile"},
		FieldOwnership: model.JSONObject{"email": "local", "phone": "upstream"},
	}
	require.False(t, directoryFieldRequiresReadPermission(policy, "email"), "本地拥有的字段不应要求上游读取权限")
	require.True(t, directoryFieldRequiresReadPermission(policy, "phone"), "上游拥有的字段必须验证读取权限")
	require.False(t, directoryFieldRequiresReadPermission(policy, "avatar"), "未配置字段不应意外阻断旧策略")

	legacy := &model.DirectorySyncPolicy{FieldMapping: model.JSONObject{"email": "email"}}
	require.True(t, directoryFieldRequiresReadPermission(legacy, "email"), "旧策略的显式映射应继续要求字段权限")
}

func openDirectorySyncTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	t.Setenv("IDENTITY_CREDENTIALS_KEY", base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")))
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.User{}, &model.Department{}, &model.UserDepartment{},
		&model.OIDCToken{},
		&model.IdentityConnection{}, &model.IdentityConnectionCredential{},
		&model.ExternalPrincipal{}, &model.ExternalIdentifier{},
		&model.ExternalPrincipalConflict{},
		&model.DirectorySyncPolicy{}, &model.ExternalOrgUnit{}, &model.ExternalMembership{},
		&model.DirectorySyncRun{}, &model.DirectorySyncChange{}, &model.DirectorySyncSnapshotObject{},
		&model.DirectoryOperationLease{},
	))
	return db
}

func seedDirectoryConnection(t *testing.T, db *gorm.DB, identifier string) (*model.IdentityConnection, *model.Department) {
	t.Helper()
	root := &model.Department{Name: "同步根-" + identifier, Code: "root-" + identifier, Type: model.DepartmentTypeCompany, Status: model.DepartmentStatusEnabled, IsLeaf: true, DataScope: 2}
	require.NoError(t, db.Create(root).Error)
	require.NoError(t, db.Model(root).Updates(map[string]any{"path": "/" + identifier, "level": 1}).Error)
	connection := &model.IdentityConnection{Name: identifier, Identifier: identifier, Category: model.ConnectionCategoryEnterprise, ProviderType: "feishu", Capabilities: model.StringList{string(model.ConnectionCapabilityAuthentication)}, Status: model.ConnectionStatusDisabled}
	require.NoError(t, db.Create(connection).Error)
	return connection, root
}

func localDirectoryCounts(t *testing.T, db *gorm.DB) (int64, int64, int64) {
	t.Helper()
	var users, departments, memberships int64
	require.NoError(t, db.Model(&model.User{}).Count(&users).Error)
	require.NoError(t, db.Model(&model.Department{}).Count(&departments).Error)
	require.NoError(t, db.Model(&model.UserDepartment{}).Count(&memberships).Error)
	return users, departments, memberships
}
