package directory

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestFeishuAdapterPaginatesAndNormalizesDirectory(t *testing.T) {
	t.Helper()
	tokenCalls := 0
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/open-apis/auth/v3/tenant_access_token/internal":
			tokenCalls++
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "tenant_access_token": "tenant-token"})
		case "/open-apis/contact/v3/departments":
			if r.Header.Get("Authorization") != "Bearer tenant-token" || r.URL.Query().Get("fetch_child") != "true" {
				t.Fatalf("部门请求缺少令牌或递归参数")
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{
				"has_more": false,
				"items":    []map[string]any{{"open_department_id": "od-1", "parent_department_id": "0", "name": "研发", "order": "10", "leader_user_id": "u-1"}},
			}})
		case "/open-apis/contact/v3/users":
			page := r.URL.Query().Get("page_token")
			if page == "" {
				_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{
					"has_more": true, "page_token": "next",
					"items": []map[string]any{{
						"user_id": "u-1", "open_id": "ou-1", "union_id": "on-1", "name": "Ada", "email": "ada@example.com", "mobile": "+8613800000000",
						"job_title": "负责人", "department_ids": []string{"od-1", "od-2"}, "primary_department_id": "od-1", "leader_department_ids": []string{"od-1"},
						"status": map[string]any{"is_activated": true}, "avatar": map[string]any{"avatar_240": "https://img.test/ada.png"},
					}},
				}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{
				"has_more": false,
				"items":    []map[string]any{{"user_id": "u-2", "name": "Grace", "department_ids": []string{"od-1"}, "status": map[string]any{"is_activated": false, "is_resigned": true}}},
			}})
		default:
			http.NotFound(w, r)
		}
	})
	client := directoryTestClient(handler)

	adapter, err := NewFeishuAdapter(map[string]any{"appId": "app", "appSecret": "secret", "tenantKey": "tenant-a", "baseUrl": "https://feishu.test"}, client)
	if err != nil {
		t.Fatal(err)
	}
	orgPage, err := adapter.ListOrgUnits(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(orgPage.Items) != 1 || orgPage.Items[0].LeaderExternalID != "u-1" || orgPage.Items[0].SortOrder != 10 {
		t.Fatalf("部门标准化错误: %#v", orgPage.Items)
	}
	firstPage, err := adapter.ListUsers(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if firstPage.NextCursor != "next" || len(firstPage.Items) != 1 {
		t.Fatalf("用户分页错误: %#v", firstPage)
	}
	ada := firstPage.Items[0]
	if ada.StableID != "tenant-a:u-1" || len(ada.Memberships) != 2 || !ada.Memberships[0].Primary || !ada.Memberships[0].Leader {
		t.Fatalf("用户多部门标准化错误: %#v", ada)
	}
	secondPage, err := adapter.ListUsers(context.Background(), firstPage.NextCursor)
	if err != nil {
		t.Fatal(err)
	}
	if len(secondPage.Items) != 1 || secondPage.Items[0].Status != "inactive" {
		t.Fatalf("离职状态标准化错误: %#v", secondPage.Items)
	}
	if tokenCalls != 1 {
		t.Fatalf("同一适配器应复用 tenant token，实际请求 %d 次", tokenCalls)
	}
}

func TestFeishuAdapterRequiresSeparatedDirectoryCredentials(t *testing.T) {
	if _, err := NewFeishuAdapter(map[string]any{"appId": "only-app"}, nil); err == nil {
		t.Fatal("缺少 appSecret/tenantKey 时应拒绝创建目录适配器")
	}
}

func TestFeishuSnapshotIncludesExplicitRootDepartment(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/open-apis/auth/v3/tenant_access_token/internal":
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "tenant_access_token": "tenant-token"})
		case "/open-apis/contact/v3/departments/root":
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"department": map[string]any{
				"open_department_id": "root", "parent_department_id": "outside", "name": "可见根",
			}}})
		case "/open-apis/contact/v3/departments/root/children":
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"has_more": false, "items": []any{}}})
		case "/open-apis/contact/v3/users/find_by_department":
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"has_more": false, "items": []map[string]any{{
				"user_id": "u-1", "name": "Ada", "department_ids": []string{"root"}, "primary_department_id": "root", "status": map[string]any{"is_activated": true},
			}}}})
		default:
			t.Fatalf("unexpected Feishu path %s", r.URL.Path)
		}
	})
	adapter, err := NewFeishuAdapter(map[string]any{"appId": "app", "appSecret": "secret", "tenantKey": "tenant-a", "baseUrl": "https://feishu.test"}, directoryTestClient(handler))
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := adapter.ReadSnapshot(context.Background(), SyncScope{RootExternalOrgUnitIDs: []string{"root"}, IncludeDescendants: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.OrgUnits) != 1 || snapshot.OrgUnits[0].ExternalID != "root" || len(snapshot.Users) != 1 || len(snapshot.Users[0].Memberships) != 1 {
		t.Fatalf("explicit root snapshot is incomplete: %#v", snapshot)
	}
}
