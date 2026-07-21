package directory

import (
	"context"
	"encoding/json"
	"net/http"
	"sync/atomic"
	"testing"
)

func TestDingTalkAdapterRecursesPaginatesAndNormalizes(t *testing.T) {
	var tokenCalls atomic.Int32
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var payload map[string]any
		if r.Method == http.MethodPost {
			_ = json.NewDecoder(r.Body).Decode(&payload)
		}
		switch r.URL.Path {
		case "/gettoken":
			tokenCalls.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 0, "access_token": "ding-token"})
		case "/topapi/v2/department/get":
			_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 0, "result": map[string]any{"dept_id": 1, "parent_id": 0, "name": "总部", "order": 1}})
		case "/topapi/v2/department/listsub":
			if r.URL.Query().Get("access_token") != "ding-token" {
				t.Fatal("钉钉通讯录请求缺少 access token")
			}
			if int(payload["dept_id"].(float64)) == 1 {
				_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 0, "result": []map[string]any{{"dept_id": 2, "parent_id": 1, "name": "研发", "order": 10, "dept_manager_userid_list": []string{"ada"}}}})
			} else {
				_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 0, "result": []any{}})
			}
		case "/topapi/v2/user/list":
			departmentID := int(payload["dept_id"].(float64))
			cursor := int(payload["cursor"].(float64))
			if departmentID == 1 {
				_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 0, "result": map[string]any{"has_more": false, "list": []map[string]any{{"userid": "ada", "openid": "open-ada", "unionid": "union-ada", "name": "Ada", "email": "ada@example.com", "title": "负责人", "active": true, "dept_id_list": []int{1, 2}}}}})
			} else if cursor == 0 {
				_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 0, "result": map[string]any{"has_more": true, "next_cursor": 10, "list": []map[string]any{{"userid": "ada", "name": "Ada", "title": "负责人", "active": true, "leader_in_dept": true, "dept_id_list": []int{1, 2}}}}})
			} else {
				_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 0, "result": map[string]any{"has_more": false, "list": []map[string]any{{"userid": "grace", "name": "Grace", "active": false, "dept_id_list": []int{2}}}}})
			}
		default:
			http.NotFound(w, r)
		}
	})
	client := directoryTestClient(handler)

	adapter, err := NewDingTalkAdapter(map[string]any{
		"appKey": "app-key", "appSecret": "app-secret", "corpId": "ding-corp", "baseUrl": "https://dingtalk.test",
	}, client)
	if err != nil {
		t.Fatal(err)
	}
	orgs, err := adapter.ListOrgUnits(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(orgs.Items) != 2 || orgs.Items[1].ExternalID != "ding-corp:2" || orgs.Items[1].LeaderExternalID != "ding-corp:ada" {
		t.Fatalf("钉钉部门标准化错误: %#v", orgs.Items)
	}
	users, err := adapter.ListUsers(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(users.Items) != 2 || users.Items[0].StableID != "ding-corp:ada" || len(users.Items[0].Memberships) != 2 || !users.Items[0].Memberships[1].Leader {
		t.Fatalf("钉钉用户或多部门归一化错误: %#v", users.Items)
	}
	if users.Items[1].Status != "inactive" {
		t.Fatalf("钉钉停用状态未归一化: %#v", users.Items[1])
	}
	if tokenCalls.Load() != 1 {
		t.Fatalf("同一适配器应复用 token，实际请求 %d 次", tokenCalls.Load())
	}
}

func TestDingTalkAdapterRequiresInternalAppCredentials(t *testing.T) {
	if _, err := NewDingTalkAdapter(map[string]any{"appKey": "only-key"}, nil); err == nil {
		t.Fatal("缺少 appSecret/corpId 时应拒绝创建钉钉目录适配器")
	}
}

func TestDingTalkAdapterRefreshesExpiredAccessTokenOnce(t *testing.T) {
	var tokenCalls atomic.Int32
	var apiCalls atomic.Int32
	client := directoryTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/gettoken":
			call := tokenCalls.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 0, "access_token": "token-" + string(rune('0'+call))})
		case "/topapi/v2/department/get":
			if apiCalls.Add(1) == 1 {
				_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 40014, "errmsg": "invalid access token"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 0, "result": map[string]any{"dept_id": 1, "name": "总部"}})
		case "/topapi/v2/department/listsub":
			_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 0, "result": []any{}})
		}
	}))
	adapter, err := NewDingTalkAdapter(map[string]any{"appKey": "key", "appSecret": "secret", "corpId": "corp", "baseUrl": "https://dingtalk.test"}, client)
	if err != nil {
		t.Fatal(err)
	}
	page, err := adapter.ListOrgUnits(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || tokenCalls.Load() != 2 || apiCalls.Load() != 2 {
		t.Fatalf("钉钉 token 刷新错误: tokens=%d api=%d page=%#v", tokenCalls.Load(), apiCalls.Load(), page)
	}
}
