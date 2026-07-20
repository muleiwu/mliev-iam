package directory

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"sort"
	"testing"
)

// This fixture proves that provider-specific identifiers and envelopes do not
// leak into the semantic directory contract consumed by the diff engine.
func TestDirectoryAdaptersProduceEquivalentSemanticContract(t *testing.T) {
	client := directoryTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Host {
		case "feishu.test":
			serveFeishuContractFixture(t, w, r)
		case "wecom.test":
			serveWeComContractFixture(w, r)
		case "dingtalk.test":
			serveDingTalkContractFixture(t, w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	feishu, feishuErr := NewFeishuAdapter(map[string]any{"appId": "app", "appSecret": "secret", "tenantKey": "tenant", "baseUrl": "https://feishu.test"}, client)
	wecom, wecomErr := NewWeComAdapter(map[string]any{"corpId": "tenant", "directorySecret": "secret", "baseUrl": "https://wecom.test"}, client)
	dingtalk, dingtalkErr := NewDingTalkAdapter(map[string]any{"appKey": "app", "appSecret": "secret", "corpId": "tenant", "baseUrl": "https://dingtalk.test"}, client)
	adapters := []Adapter{
		mustDirectoryAdapter(t, feishu, feishuErr),
		mustDirectoryAdapter(t, wecom, wecomErr),
		mustDirectoryAdapter(t, dingtalk, dingtalkErr),
	}
	var expected []string
	for index, adapter := range adapters {
		orgs, err := adapter.ListOrgUnits(context.Background(), "")
		if err != nil {
			t.Fatal(err)
		}
		users, err := adapter.ListUsers(context.Background(), "")
		if err != nil {
			t.Fatal(err)
		}
		projection := semanticDirectoryProjection(orgs.Items, users.Items)
		if index == 0 {
			expected = projection
			continue
		}
		if !reflect.DeepEqual(projection, expected) {
			t.Fatalf("provider %d semantic contract differs\nwant: %#v\n got: %#v", index, expected, projection)
		}
	}
}

func mustDirectoryAdapter(t *testing.T, adapter Adapter, err error) Adapter {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	return adapter
}

func semanticDirectoryProjection(orgs []OrgUnit, users []User) []string {
	projection := make([]string, 0, len(orgs)+len(users))
	for _, org := range orgs {
		projection = append(projection, "org|"+org.Name)
	}
	for _, user := range users {
		primary, leaders := 0, 0
		position := ""
		for _, membership := range user.Memberships {
			if membership.Primary {
				primary++
			}
			if membership.Leader {
				leaders++
			}
			if position == "" {
				position = membership.Position
			}
		}
		projection = append(projection, fmt.Sprintf("user|%s|%s|%s|%s|%d|%d|%d", user.DisplayName, user.Email, user.Status, position, len(user.Memberships), primary, leaders))
	}
	sort.Strings(projection)
	return projection
}

func serveFeishuContractFixture(t *testing.T, w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/open-apis/auth/v3/tenant_access_token/internal":
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "tenant_access_token": "token"})
	case "/open-apis/contact/v3/departments":
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"items": []map[string]any{{"open_department_id": "root", "name": "总部"}, {"open_department_id": "eng", "parent_department_id": "root", "name": "研发"}}}})
	case "/open-apis/contact/v3/users":
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"items": []map[string]any{
			{"user_id": "ada", "name": "Ada", "email": "ada@example.com", "job_title": "Lead", "department_ids": []string{"eng", "root"}, "primary_department_id": "eng", "leader_department_ids": []string{"eng"}, "status": map[string]any{"is_activated": true}},
			{"user_id": "grace", "name": "Grace", "email": "grace@example.com", "job_title": "Engineer", "department_ids": []string{"eng"}, "status": map[string]any{"is_resigned": true}},
		}}})
	default:
		t.Fatalf("unexpected Feishu fixture path %s", r.URL.Path)
	}
}

func serveWeComContractFixture(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/cgi-bin/gettoken":
		_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 0, "access_token": "token"})
	case "/cgi-bin/department/list":
		_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 0, "department": []map[string]any{{"id": 1, "name": "总部"}, {"id": 2, "parentid": 1, "name": "研发"}}})
	case "/cgi-bin/user/list":
		_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 0, "userlist": []map[string]any{
			{"userid": "ada", "name": "Ada", "email": "ada@example.com", "position": "Lead", "department": []int{2, 1}, "is_leader_in_dept": []int{1, 0}, "status": 1},
			{"userid": "grace", "name": "Grace", "email": "grace@example.com", "position": "Engineer", "department": []int{2}, "status": 5},
		}})
	}
}

func serveDingTalkContractFixture(t *testing.T, w http.ResponseWriter, r *http.Request) {
	var payload map[string]any
	if r.Method == http.MethodPost {
		_ = json.NewDecoder(r.Body).Decode(&payload)
	}
	switch r.URL.Path {
	case "/gettoken":
		_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 0, "access_token": "token"})
	case "/topapi/v2/department/get":
		_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 0, "result": map[string]any{"dept_id": 1, "name": "总部"}})
	case "/topapi/v2/department/listsub":
		if int(payload["dept_id"].(float64)) == 1 {
			_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 0, "result": []map[string]any{{"dept_id": 2, "parent_id": 1, "name": "研发"}}})
		} else {
			_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 0, "result": []any{}})
		}
	case "/topapi/v2/user/list":
		departmentID := int(payload["dept_id"].(float64))
		users := []map[string]any{{"userid": "ada", "name": "Ada", "email": "ada@example.com", "title": "Lead", "active": true, "dept_id_list": []int{1, 2}, "leader_in_dept": departmentID == 2}}
		if departmentID == 2 {
			users = append(users, map[string]any{"userid": "grace", "name": "Grace", "email": "grace@example.com", "title": "Engineer", "active": false, "dept_id_list": []int{2}})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 0, "result": map[string]any{"has_more": false, "list": users}})
	default:
		t.Fatalf("unexpected DingTalk fixture path %s", r.URL.Path)
	}
}
