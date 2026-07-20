package directory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const defaultFeishuBaseURL = "https://open.feishu.cn"

type FeishuAdapter struct {
	appID       string
	appSecret   string
	tenantKey   string
	baseURL     string
	httpClient  *http.Client
	accessToken string
}

func NewFeishuAdapter(credentials map[string]any, client *http.Client) (*FeishuAdapter, error) {
	value := func(key string) string {
		if v, ok := credentials[key].(string); ok {
			return strings.TrimSpace(v)
		}
		return ""
	}
	adapter := &FeishuAdapter{
		appID: value("appId"), appSecret: value("appSecret"), tenantKey: value("tenantKey"),
		baseURL: strings.TrimRight(value("baseUrl"), "/"), httpClient: client,
	}
	if adapter.baseURL == "" {
		adapter.baseURL = defaultFeishuBaseURL
	}
	if adapter.httpClient == nil {
		adapter.httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	if adapter.appID == "" || adapter.appSecret == "" || adapter.tenantKey == "" {
		return nil, fmt.Errorf("飞书目录凭据必须包含 appId、appSecret 和 tenantKey")
	}
	return adapter, nil
}

func (a *FeishuAdapter) ValidateDirectoryConfig(ctx context.Context) (*Diagnostic, error) {
	if _, err := a.token(ctx); err != nil {
		return &Diagnostic{Healthy: false, Warnings: []string{err.Error()}}, err
	}
	diagnostic := &Diagnostic{TokenValid: true, TenantID: a.tenantKey, Warnings: []string{}}
	orgs, err := a.ListOrgUnits(ctx, "")
	if err != nil {
		diagnostic.Warnings = append(diagnostic.Warnings, "无法读取通讯录部门，请检查 contact:department 权限和应用可见范围")
		return diagnostic, err
	}
	users, err := a.ListUsers(ctx, "")
	if err != nil {
		diagnostic.Warnings = append(diagnostic.Warnings, "无法读取通讯录用户，请检查 contact:user 权限和敏感字段权限")
		return diagnostic, err
	}
	diagnostic.DirectoryReadable = true
	diagnostic.Healthy = true
	diagnostic.VisibleOrgUnits = len(orgs.Items)
	diagnostic.VisibleUsers = len(users.Items)
	if len(orgs.Items) == 0 || len(users.Items) == 0 {
		diagnostic.Warnings = append(diagnostic.Warnings, "当前应用的通讯录可见范围为空或仅包含根部门")
	}
	return diagnostic, nil
}

func (a *FeishuAdapter) ListOrgUnits(ctx context.Context, cursor string) (*OrgUnitPage, error) {
	query := url.Values{
		"department_id_type": []string{"open_department_id"},
		"fetch_child":        []string{"true"},
		"page_size":          []string{"50"},
	}
	if cursor != "" {
		query.Set("page_token", cursor)
	}
	var response struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			HasMore   bool   `json:"has_more"`
			PageToken string `json:"page_token"`
			Items     []struct {
				OpenDepartmentID   string         `json:"open_department_id"`
				ParentDepartmentID string         `json:"parent_department_id"`
				Name               string         `json:"name"`
				Order              string         `json:"order"`
				LeaderUserID       string         `json:"leader_user_id"`
				Status             any            `json:"status"`
				Raw                map[string]any `json:"-"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := a.get(ctx, "/open-apis/contact/v3/departments", query, &response); err != nil {
		return nil, err
	}
	items := make([]OrgUnit, 0, len(response.Data.Items))
	for _, item := range response.Data.Items {
		order, _ := strconv.Atoi(item.Order)
		status := normalizeFeishuStatus(item.Status)
		items = append(items, OrgUnit{
			ExternalID: item.OpenDepartmentID, ExternalParentID: item.ParentDepartmentID,
			Name: item.Name, SortOrder: order, LeaderExternalID: item.LeaderUserID,
			Status: status, Raw: map[string]any{"open_department_id": item.OpenDepartmentID, "parent_department_id": item.ParentDepartmentID, "name": item.Name, "order": item.Order, "leader_user_id": item.LeaderUserID, "status": item.Status},
		})
	}
	next := ""
	if response.Data.HasMore {
		next = response.Data.PageToken
	}
	return &OrgUnitPage{Items: items, NextCursor: next}, nil
}

func (a *FeishuAdapter) ListUsers(ctx context.Context, cursor string) (*UserPage, error) {
	query := url.Values{
		"user_id_type":       []string{"user_id"},
		"department_id_type": []string{"open_department_id"},
		"page_size":          []string{"50"},
	}
	if cursor != "" {
		query.Set("page_token", cursor)
	}
	var response struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			HasMore   bool         `json:"has_more"`
			PageToken string       `json:"page_token"`
			Items     []feishuUser `json:"items"`
		} `json:"data"`
	}
	if err := a.get(ctx, "/open-apis/contact/v3/users", query, &response); err != nil {
		return nil, err
	}
	items := make([]User, 0, len(response.Data.Items))
	for _, item := range response.Data.Items {
		if item.UserID == "" {
			continue
		}
		identifiers := []Identifier{{Namespace: "directory_user_id", Value: a.tenantKey + ":" + item.UserID, Primary: true}}
		if item.OpenID != "" {
			identifiers = append(identifiers, Identifier{Namespace: "open_id", Value: item.OpenID})
		}
		if item.UnionID != "" {
			identifiers = append(identifiers, Identifier{Namespace: "union_id", Value: item.UnionID})
		}
		memberships := make([]Membership, 0, len(item.DepartmentIDs))
		for index, departmentID := range item.DepartmentIDs {
			memberships = append(memberships, Membership{
				OrgUnitExternalID: departmentID, Primary: departmentID == item.PrimaryDepartmentID || (item.PrimaryDepartmentID == "" && index == 0),
				Leader: contains(item.LeaderDepartmentIDs, departmentID), Position: item.JobTitle,
				Raw: map[string]any{"department_id": departmentID},
			})
		}
		items = append(items, User{
			StableID: a.tenantKey + ":" + item.UserID, TenantID: a.tenantKey, ProviderUserID: item.UserID,
			Identifiers: identifiers, DisplayName: item.Name, Email: item.Email, Phone: item.Mobile,
			EmployeeNo: item.EmployeeNo, Avatar: item.Avatar.Avatar240, Status: normalizeFeishuUserStatus(item.Status),
			Memberships: memberships, Raw: item.raw(),
		})
	}
	next := ""
	if response.Data.HasMore {
		next = response.Data.PageToken
	}
	return &UserPage{Items: items, NextCursor: next}, nil
}

type feishuUser struct {
	UserID              string   `json:"user_id"`
	OpenID              string   `json:"open_id"`
	UnionID             string   `json:"union_id"`
	Name                string   `json:"name"`
	Email               string   `json:"email"`
	Mobile              string   `json:"mobile"`
	EmployeeNo          string   `json:"employee_no"`
	JobTitle            string   `json:"job_title"`
	DepartmentIDs       []string `json:"department_ids"`
	PrimaryDepartmentID string   `json:"primary_department_id"`
	LeaderDepartmentIDs []string `json:"leader_department_ids"`
	Status              struct {
		IsActivated bool `json:"is_activated"`
		IsExited    bool `json:"is_exited"`
		IsFrozen    bool `json:"is_frozen"`
		IsResigned  bool `json:"is_resigned"`
	} `json:"status"`
	Avatar struct {
		Avatar240 string `json:"avatar_240"`
	} `json:"avatar"`
}

func (u feishuUser) raw() map[string]any {
	return map[string]any{
		"user_id": u.UserID, "open_id": u.OpenID, "union_id": u.UnionID, "name": u.Name,
		"email": u.Email, "mobile": u.Mobile, "employee_no": u.EmployeeNo, "job_title": u.JobTitle,
		"department_ids": u.DepartmentIDs, "primary_department_id": u.PrimaryDepartmentID,
		"leader_department_ids": u.LeaderDepartmentIDs,
		"status":                map[string]any{"is_activated": u.Status.IsActivated, "is_exited": u.Status.IsExited, "is_frozen": u.Status.IsFrozen, "is_resigned": u.Status.IsResigned},
	}
}

func normalizeFeishuUserStatus(status struct {
	IsActivated bool `json:"is_activated"`
	IsExited    bool `json:"is_exited"`
	IsFrozen    bool `json:"is_frozen"`
	IsResigned  bool `json:"is_resigned"`
}) string {
	if status.IsExited || status.IsResigned || status.IsFrozen || !status.IsActivated {
		return "inactive"
	}
	return "active"
}

func normalizeFeishuStatus(value any) string {
	if text, ok := value.(string); ok && (text == "deleted" || text == "inactive") {
		return "inactive"
	}
	return "active"
}

func (a *FeishuAdapter) token(ctx context.Context) (string, error) {
	if a.accessToken != "" {
		return a.accessToken, nil
	}
	payload, _ := json.Marshal(map[string]string{"app_id": a.appID, "app_secret": a.appSecret})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/open-apis/auth/v3/tenant_access_token/internal", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/json")
	var response struct {
		Code              int    `json:"code"`
		Msg               string `json:"msg"`
		TenantAccessToken string `json:"tenant_access_token"`
	}
	if err := a.do(request, &response); err != nil {
		return "", err
	}
	if response.Code != 0 || response.TenantAccessToken == "" {
		return "", fmt.Errorf("飞书获取 tenant access token 失败: code=%d msg=%s", response.Code, response.Msg)
	}
	a.accessToken = response.TenantAccessToken
	return a.accessToken, nil
}

func (a *FeishuAdapter) get(ctx context.Context, path string, query url.Values, target any) error {
	token, err := a.token(ctx)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, a.baseURL+path+"?"+query.Encode(), nil)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	if err := a.do(request, target); err != nil {
		return err
	}
	encoded, _ := json.Marshal(target)
	var envelope struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	_ = json.Unmarshal(encoded, &envelope)
	if envelope.Code != 0 {
		return fmt.Errorf("飞书通讯录 API 失败: code=%d msg=%s", envelope.Code, envelope.Msg)
	}
	return nil
}

func (a *FeishuAdapter) do(request *http.Request, target any) error {
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 && request.GetBody != nil {
			body, err := request.GetBody()
			if err != nil {
				return err
			}
			request.Body = body
		}
		response, err := a.httpClient.Do(request)
		if err != nil {
			return err
		}
		body, readErr := io.ReadAll(io.LimitReader(response.Body, 4<<20))
		_ = response.Body.Close()
		if readErr != nil {
			return readErr
		}
		if response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500 {
			if attempt == 2 {
				return fmt.Errorf("飞书 API 暂时不可用: HTTP %d", response.StatusCode)
			}
			retryAfter, _ := strconv.Atoi(response.Header.Get("Retry-After"))
			if retryAfter <= 0 || retryAfter > 2 {
				retryAfter = attempt + 1
			}
			select {
			case <-request.Context().Done():
				return request.Context().Err()
			case <-time.After(time.Duration(retryAfter) * 100 * time.Millisecond):
			}
			continue
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return fmt.Errorf("飞书 API 请求失败: HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
		}
		if err := json.Unmarshal(body, target); err != nil {
			return fmt.Errorf("解析飞书 API 响应失败: %w", err)
		}
		return nil
	}
	return fmt.Errorf("飞书 API 请求重试耗尽")
}

func contains(items []string, expected string) bool {
	for _, item := range items {
		if item == expected {
			return true
		}
	}
	return false
}
