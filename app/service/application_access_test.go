package service

import (
	"encoding/json"
	"fmt"
	"testing"

	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestApplicationPublicAccessTransitions(t *testing.T) {
	db := newApplicationAccessTestDB(t)
	app := &model.Application{
		UUID: "public-access", Name: "public-access", ProtocolType: "oidc",
		ProtocolConfig: map[string]interface{}{"clientId": "public-access"},
		AccessScope:    &model.ApplicationAccessScope{UserIDs: []uint{42}},
		IsActive:       true,
	}
	if err := db.Create(app).Error; err != nil {
		t.Fatal(err)
	}
	service := NewApplicationService(db)
	for _, step := range []struct {
		name        string
		body        string
		allowListed bool
		allowNew    bool
	}{
		{"public overrides old grants", `{"isPublic":true,"userIds":[42],"departmentIds":[10],"roleIds":[30]}`, true, true},
		{"unchecking restores restricted access", `{"isPublic":false,"userIds":[42]}`, true, false},
		{"explicit restricted empty denies everyone", `{"isPublic":false,"userIds":[],"departmentIds":[],"roleIds":[]}`, false, false},
		{"public without grants allows new users", `{"isPublic":true}`, true, true},
		{"legacy restricted update remains supported", `{"userIds":[42]}`, true, false},
		{"legacy clearing still allows everyone", `{}`, true, true},
	} {
		t.Run(step.name, func(t *testing.T) {
			var request dto.ApplicationAccessScopeRequest
			if err := json.Unmarshal([]byte(step.body), &request); err != nil {
				t.Fatal(err)
			}
			if err := service.UpdateAccessScope(app.ID, &request); err != nil {
				t.Fatal(err)
			}
			response, err := service.GetAccessScope(app.ID, nil)
			if err != nil {
				t.Fatal(err)
			}
			if response.IsPublic != step.allowNew {
				t.Errorf("returned isPublic = %v, want %v", response.IsPublic, step.allowNew)
			}
			if len(response.UserIDs) != len(request.UserIDs) ||
				len(response.DepartmentIDs) != len(request.DepartmentIDs) ||
				len(response.RoleIDs) != len(request.RoleIDs) {
				t.Errorf("saved grants were lost: %+v", response)
			}
			for _, check := range []struct {
				userID  uint
				allowed bool
			}{{42, step.allowListed}, {99, step.allowNew}} {
				if actual := NewApplicationService(db).CheckUserAccess(app.ID, check.userID); actual != check.allowed {
					t.Errorf("user %d access = %v, want %v", check.userID, actual, check.allowed)
				}
			}
		})
	}
}

func TestApplicationAccessAfterClearingScope(t *testing.T) {
	for _, request := range []struct {
		name  string
		scope dto.ApplicationAccessScopeRequest
	}{
		{name: "omitted lists"},
		{name: "empty lists", scope: dto.ApplicationAccessScopeRequest{
			UserIDs: []uint{}, DepartmentIDs: []uint{}, RoleIDs: []uint{}, IncludeSubDepartments: true,
		}},
	} {
		t.Run(request.name, func(t *testing.T) {
			db := newApplicationAccessTestDB(t)
			app := &model.Application{
				UUID: "clear-scope", Name: "clear-scope", ProtocolType: "oidc",
				ProtocolConfig: map[string]interface{}{"clientId": "clear-scope"},
				AccessScope:    &model.ApplicationAccessScope{UserIDs: []uint{42}},
				IsActive:       true,
			}
			if err := db.Create(app).Error; err != nil {
				t.Fatal(err)
			}
			service := NewApplicationService(db)
			if service.CheckUserAccess(app.ID, 99) {
				t.Fatal("unlisted user can access restricted application")
			}
			if err := service.UpdateAccessScope(app.ID, &request.scope); err != nil {
				t.Fatal(err)
			}
			if !NewApplicationService(db).CheckUserAccess(app.ID, 99) {
				t.Fatal("new user cannot access application after clearing its scope")
			}
		})
	}
}

func TestApplicationAccessScopeModes(t *testing.T) {
	public, restricted := true, false
	tests := []struct {
		name         string
		scope        *model.ApplicationAccessScope
		userID       uint
		seed         func(*testing.T, *gorm.DB)
		shouldAccess bool
	}{
		{
			name:         "explicit public overrides restricted scope",
			scope:        &model.ApplicationAccessScope{IsPublic: &public, UserIDs: []uint{99}},
			userID:       6,
			shouldAccess: true,
		},
		{
			name:         "explicit restricted empty denies access",
			scope:        &model.ApplicationAccessScope{IsPublic: &restricted},
			userID:       6,
			shouldAccess: false,
		},
		{
			name:         "empty scope allows access",
			userID:       1,
			shouldAccess: true,
		},
		{
			name:         "direct user grant",
			scope:        &model.ApplicationAccessScope{UserIDs: []uint{2}},
			userID:       2,
			shouldAccess: true,
		},
		{
			name:   "direct department grant",
			scope:  &model.ApplicationAccessScope{DepartmentIDs: []uint{10}},
			userID: 3,
			seed: func(t *testing.T, db *gorm.DB) {
				t.Helper()
				createAccessDepartment(t, db, 10, "direct", "/10/")
				createUserDepartment(t, db, 3, 10)
			},
			shouldAccess: true,
		},
		{
			name: "child department grant",
			scope: &model.ApplicationAccessScope{
				DepartmentIDs:         []uint{20},
				IncludeSubDepartments: true,
			},
			userID: 4,
			seed: func(t *testing.T, db *gorm.DB) {
				t.Helper()
				createAccessDepartment(t, db, 20, "parent", "/20/")
				createAccessDepartment(t, db, 21, "child", "/20/21/")
				createUserDepartment(t, db, 4, 21)
			},
			shouldAccess: true,
		},
		{
			name:   "role grant",
			scope:  &model.ApplicationAccessScope{RoleIDs: []uint{30}},
			userID: 5,
			seed: func(t *testing.T, db *gorm.DB) {
				t.Helper()
				role := &model.Role{ID: 30, Name: "access-role", Code: "access-role"}
				if err := db.Create(role).Error; err != nil {
					t.Fatalf("create role: %v", err)
				}
				userRole := &model.UserRole{
					UserID:     5,
					RoleID:     30,
					AssignedBy: 1,
					IsActive:   true,
					DataScope:  1,
					ApplyScope: 1,
					IsPrimary:  true,
				}
				if err := db.Create(userRole).Error; err != nil {
					t.Fatalf("create user role: %v", err)
				}
			},
			shouldAccess: true,
		},
		{
			name:         "unmatched restricted scope denies access",
			scope:        &model.ApplicationAccessScope{UserIDs: []uint{99}},
			userID:       6,
			shouldAccess: false,
		},
	}

	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db := newApplicationAccessTestDB(t)
			if test.seed != nil {
				test.seed(t, db)
			}
			app := &model.Application{
				UUID:           fmt.Sprintf("client-%d", index),
				Name:           test.name,
				ProtocolType:   string(model.ProtocolTypeOIDC),
				ProtocolConfig: map[string]interface{}{"clientId": fmt.Sprintf("client-%d", index)},
				AccessScope:    test.scope,
				IsActive:       true,
			}
			if err := db.Create(app).Error; err != nil {
				t.Fatalf("create application: %v", err)
			}

			access := NewApplicationService(db).CheckUserAccess(app.ID, test.userID)
			if access != test.shouldAccess {
				t.Fatalf("CheckUserAccess() = %v, want %v", access, test.shouldAccess)
			}
		})
	}
}

func newApplicationAccessTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&model.Application{},
		&model.User{},
		&model.Department{},
		&model.UserDepartment{},
		&model.Role{},
		&model.UserRole{},
	); err != nil {
		t.Fatalf("migrate access tables: %v", err)
	}
	return db
}

func createAccessDepartment(t *testing.T, db *gorm.DB, id uint, code, path string) {
	t.Helper()
	department := &model.Department{
		ID:     id,
		Name:   code,
		Code:   code,
		Status: model.DepartmentStatusEnabled,
		Path:   path,
	}
	if err := db.Session(&gorm.Session{SkipHooks: true}).Create(department).Error; err != nil {
		t.Fatalf("create department: %v", err)
	}
}

func createUserDepartment(t *testing.T, db *gorm.DB, userID, departmentID uint) {
	t.Helper()
	relation := &model.UserDepartment{
		UserID:       userID,
		DepartmentID: departmentID,
		IsActive:     true,
	}
	if err := db.Create(relation).Error; err != nil {
		t.Fatalf("create user department: %v", err)
	}
}
