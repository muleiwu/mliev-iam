package directory

import (
	"context"
	"encoding/json"
	"net/http"
	"sync/atomic"
	"testing"
)

func TestWeComAdapterNormalizesOrganizationAndUsers(t *testing.T) {
	tokenCalls := 0
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/cgi-bin/gettoken":
			tokenCalls++
			if r.URL.Query().Get("corpid") != "corp-a" || r.URL.Query().Get("corpsecret") != "directory-secret" {
				t.Fatalf("企业微信 token 参数错误: %s", r.URL.RawQuery)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 0, "access_token": "wecom-token"})
		case "/cgi-bin/department/list":
			if r.URL.Query().Get("access_token") != "wecom-token" || r.URL.Query().Get("id") != "1" {
				t.Fatalf("企业微信部门请求缺少 token 或根部门")
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 0, "department": []map[string]any{
				{"id": 1, "parentid": 0, "name": "总部", "order": 1},
				{"id": 2, "parentid": 1, "name": "研发", "order": 10, "department_leader": []string{"ada"}},
			}})
		case "/cgi-bin/user/list":
			if r.URL.Query().Get("fetch_child") != "1" {
				t.Fatal("企业微信用户请求必须递归读取子部门")
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 0, "userlist": []map[string]any{
				{"userid": "ada", "open_userid": "open-ada", "unionid": "union-ada", "name": "Ada", "email": "ada@example.com", "mobile": "13800000000", "position": "负责人", "department": []int{2, 1}, "order": []int{10, 20}, "is_leader_in_dept": []int{1, 0}, "status": 1},
				{"userid": "grace", "name": "Grace", "department": []int{2}, "status": 2},
			}})
		default:
			http.NotFound(w, r)
		}
	})
	client := directoryTestClient(handler)

	adapter, err := NewWeComAdapter(map[string]any{
		"corpId": "corp-a", "directorySecret": "directory-secret", "baseUrl": "https://wecom.test",
	}, client)
	if err != nil {
		t.Fatal(err)
	}
	orgs, err := adapter.ListOrgUnits(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(orgs.Items) != 2 || orgs.Items[1].ExternalID != "corp-a:2" || orgs.Items[1].ExternalParentID != "corp-a:1" || orgs.Items[1].LeaderExternalID != "corp-a:ada" {
		t.Fatalf("企业微信部门标准化错误: %#v", orgs.Items)
	}
	users, err := adapter.ListUsers(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(users.Items) != 2 {
		t.Fatalf("企业微信用户数错误: %#v", users.Items)
	}
	ada := users.Items[0]
	if ada.StableID != "corp-a:ada" || len(ada.Identifiers) != 3 || len(ada.Memberships) != 2 || !ada.Memberships[0].Primary || !ada.Memberships[0].Leader {
		t.Fatalf("企业微信用户标准化错误: %#v", ada)
	}
	if users.Items[1].Status != "inactive" {
		t.Fatalf("企业微信禁用状态未归一化: %#v", users.Items[1])
	}
	if tokenCalls != 1 {
		t.Fatalf("同一适配器应复用 token，实际请求 %d 次", tokenCalls)
	}
}

func TestWeComAdapterRefreshesExpiredAccessTokenOnce(t *testing.T) {
	var tokenCalls atomic.Int32
	var departmentCalls atomic.Int32
	client := directoryTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/cgi-bin/gettoken":
			call := tokenCalls.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 0, "access_token": "token-" + string(rune('0'+call))})
		case "/cgi-bin/department/list":
			if departmentCalls.Add(1) == 1 {
				_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 42001, "errmsg": "access token expired"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 0, "department": []map[string]any{{"id": 1, "name": "总部"}}})
		}
	}))
	adapter, err := NewWeComAdapter(map[string]any{"corpId": "corp-a", "directorySecret": "secret", "baseUrl": "https://wecom.test"}, client)
	if err != nil {
		t.Fatal(err)
	}
	page, err := adapter.ListOrgUnits(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || tokenCalls.Load() != 2 || departmentCalls.Load() != 2 {
		t.Fatalf("企业微信 token 刷新错误: tokens=%d departments=%d page=%#v", tokenCalls.Load(), departmentCalls.Load(), page)
	}
}

func TestWeComAdapterRequiresSeparatedDirectorySecret(t *testing.T) {
	if _, err := NewWeComAdapter(map[string]any{"corpId": "corp-only"}, nil); err == nil {
		t.Fatal("缺少 directorySecret 时应拒绝创建企业微信目录适配器")
	}
}
