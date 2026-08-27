package service

import (
	"fmt"
	"testing"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestGetUserRoleAssignmentsReturnsRelationshipMetadata(t *testing.T) {
	db := newUserMembershipTestDB(t)
	user := createMembershipTestUser(t, db, "role-user")
	activeRole := createMembershipTestRole(t, db, "active-role")
	inactiveRole := createMembershipTestRole(t, db, "inactive-role")
	now := time.Now().Add(-time.Hour).Truncate(time.Second)
	expiredAt := now.Add(-time.Minute)

	require.NoError(t, db.Create(&model.UserRole{
		UserID: user.ID, RoleID: activeRole.ID, AssignedBy: user.ID,
		AssignedAt: now, IsActive: true, IsPrimary: true, DataScope: 2,
		ApprovalStatus: 1,
	}).Error)
	require.NoError(t, db.Create(&model.UserRole{
		UserID: user.ID, RoleID: inactiveRole.ID, AssignedBy: user.ID,
		AssignedAt: now, ExpiresAt: &expiredAt, IsActive: true, DataScope: 4,
		ApprovalStatus: 1,
	}).Error)
	require.NoError(t, db.Model(&model.UserRole{}).
		Where("user_id = ? AND role_id = ?", user.ID, inactiveRole.ID).
		Update("is_active", false).Error)

	assignments, err := NewUserService(db).GetUserRoleAssignments(user.ID)
	require.NoError(t, err)
	require.Len(t, assignments, 2)

	byRoleID := make(map[uint]*dto.UserRoleResponse, len(assignments))
	for _, assignment := range assignments {
		byRoleID[assignment.RoleID] = assignment
	}
	require.Equal(t, "active-role", byRoleID[activeRole.ID].Role.Name)
	require.True(t, byRoleID[activeRole.ID].IsPrimary)
	require.True(t, byRoleID[activeRole.ID].IsActive)
	require.Equal(t, "本部门数据", byRoleID[activeRole.ID].DataScopeName)
	require.Equal(t, "inactive-role", byRoleID[inactiveRole.ID].Role.Name)
	require.False(t, byRoleID[inactiveRole.ID].IsActive)
	require.True(t, byRoleID[inactiveRole.ID].IsExpired)

	effectiveRoles, err := NewUserService(db).GetEffectiveRoleDefinitions(user.ID)
	require.NoError(t, err)
	require.Len(t, effectiveRoles, 1)
	require.Equal(t, activeRole.Code, effectiveRoles[0].Code)
}

func TestUserDepartmentLifecycle(t *testing.T) {
	db := newUserMembershipTestDB(t)
	user := createMembershipTestUser(t, db, "department-user")
	firstDepartment := createMembershipTestDepartment(t, db, "company", model.DepartmentStatusEnabled)
	secondDepartment := createMembershipTestDepartment(t, db, "team", model.DepartmentStatusEnabled)
	service := NewUserDepartmentService(db)

	first, err := service.Create(&dto.UserDepartmentCreateDto{
		UserID: user.ID, DepartmentID: firstDepartment.ID, Position: "工程师",
	})
	require.NoError(t, err)
	require.True(t, first.IsPrimary, "first active membership must become primary")
	require.NotNil(t, first.Department)
	require.Equal(t, firstDepartment.Name, first.Department.Name)

	second, err := service.Create(&dto.UserDepartmentCreateDto{
		UserID: user.ID, DepartmentID: secondDepartment.ID, Position: "负责人",
		Type: int(model.UserDepartmentTypeLeader), IsPrimary: true,
	})
	require.NoError(t, err)
	require.True(t, second.IsPrimary)
	requireMembershipPrimary(t, db, user.ID, firstDepartment.ID, false)
	requireMembershipPrimary(t, db, user.ID, secondDepartment.ID, true)

	_, err = service.Create(&dto.UserDepartmentCreateDto{
		UserID: user.ID, DepartmentID: secondDepartment.ID,
	})
	require.ErrorIs(t, err, ErrUserDepartmentConflict)

	require.NoError(t, service.Delete(user.ID, secondDepartment.ID))
	requireMembershipPrimary(t, db, user.ID, firstDepartment.ID, true)
	require.NoError(t, service.Delete(user.ID, firstDepartment.ID))

	restored, err := service.Create(&dto.UserDepartmentCreateDto{
		UserID: user.ID, DepartmentID: firstDepartment.ID, Position: "架构师",
	})
	require.NoError(t, err)
	require.True(t, restored.IsPrimary)
	require.Equal(t, "架构师", restored.Position)
	var restoredModel model.UserDepartment
	require.NoError(t, db.Where("user_id = ? AND department_id = ?", user.ID, firstDepartment.ID).
		First(&restoredModel).Error)
	require.False(t, restoredModel.DeletedAt.Valid)
	require.True(t, restoredModel.IsActive)
	require.Nil(t, restoredModel.LeftAt)
}

func TestUserDepartmentValidationAndSetPrimary(t *testing.T) {
	db := newUserMembershipTestDB(t)
	user := createMembershipTestUser(t, db, "validation-user")
	enabledDepartment := createMembershipTestDepartment(t, db, "enabled", model.DepartmentStatusEnabled)
	disabledDepartment := createMembershipTestDepartment(t, db, "disabled", model.DepartmentStatusDisabled)
	service := NewUserDepartmentService(db)

	_, err := service.Create(&dto.UserDepartmentCreateDto{UserID: 9999, DepartmentID: enabledDepartment.ID})
	require.ErrorIs(t, err, ErrUserDepartmentUserNotFound)
	_, err = service.Create(&dto.UserDepartmentCreateDto{UserID: user.ID, DepartmentID: 9999})
	require.ErrorIs(t, err, ErrUserDepartmentDepartmentNotFound)
	_, err = service.Create(&dto.UserDepartmentCreateDto{UserID: user.ID, DepartmentID: disabledDepartment.ID})
	require.ErrorIs(t, err, ErrUserDepartmentDepartmentDisabled)
	require.ErrorIs(t, service.Delete(user.ID, enabledDepartment.ID), ErrUserDepartmentNotFound)
	_, err = service.SetPrimary(user.ID, enabledDepartment.ID)
	require.ErrorIs(t, err, ErrUserDepartmentNotFound)

	created, err := service.Create(&dto.UserDepartmentCreateDto{
		UserID: user.ID, DepartmentID: enabledDepartment.ID,
	})
	require.NoError(t, err)
	primary, err := service.SetPrimary(user.ID, enabledDepartment.ID)
	require.NoError(t, err)
	require.True(t, primary.IsPrimary)
	require.Equal(t, created.DepartmentID, primary.DepartmentID)
}

func newUserMembershipTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.User{},
		&model.Department{},
		&model.UserDepartment{},
		&model.Role{},
		&model.UserRole{},
	))
	return db
}

func createMembershipTestUser(t *testing.T, db *gorm.DB, username string) *model.User {
	t.Helper()
	user := &model.User{Username: username, IsActivated: true}
	require.NoError(t, db.Create(user).Error)
	return user
}

func createMembershipTestRole(t *testing.T, db *gorm.DB, code string) *model.Role {
	t.Helper()
	role := &model.Role{Name: code, Code: code}
	require.NoError(t, db.Create(role).Error)
	return role
}

func createMembershipTestDepartment(
	t *testing.T,
	db *gorm.DB,
	code string,
	status model.DepartmentStatus,
) *model.Department {
	t.Helper()
	department := &model.Department{Name: code, Code: code, Status: status}
	if status == model.DepartmentStatusDisabled {
		require.NoError(t, db.Session(&gorm.Session{SkipHooks: true}).Create(department).Error)
		require.NoError(t, db.Model(department).Update("status", model.DepartmentStatusDisabled).Error)
	} else {
		require.NoError(t, db.Create(department).Error)
	}
	return department
}

func requireMembershipPrimary(
	t *testing.T,
	db *gorm.DB,
	userID uint,
	departmentID uint,
	want bool,
) {
	t.Helper()
	var relation model.UserDepartment
	require.NoError(t, db.Where("user_id = ? AND department_id = ?", userID, departmentID).
		First(&relation).Error)
	require.Equal(t, want, relation.IsPrimary)
}
