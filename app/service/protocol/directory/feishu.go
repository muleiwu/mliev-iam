package directory

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
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

// ReadSnapshot follows Contact v3's supported hierarchy APIs: it walks
// departments/:id/children and then pages users/find_by_department for every
// visited department. Users are deduplicated by tenant_key:user_id while all
// memberships are merged.
func (a *FeishuAdapter) ReadSnapshot(ctx context.Context, scope SyncScope) (*Snapshot, error) {
	roots := append([]string(nil), scope.RootExternalOrgUnitIDs...)
	if len(roots) == 0 {
		roots = []string{"0"}
	}
	sort.Strings(roots)
	scopeJSON, _ := json.Marshal(SyncScope{RootExternalOrgUnitIDs: roots, IncludeDescendants: scope.IncludeDescendants, IncludeInactive: scope.IncludeInactive})
	scopeDigest := sha256.Sum256(scopeJSON)
	result := &Snapshot{Completeness: SnapshotCompleteness{PaginationComplete: true, ScopeHash: hex.EncodeToString(scopeDigest[:]), FieldPermissions: map[string]string{"email": "unknown", "phone": "unknown"}}}
	queue := append([]string(nil), roots...)
	seenDepartments := map[string]bool{}
	orgByID := map[string]OrgUnit{}
	usersByID := map[string]User{}
	observedUsers := 0
	// Department 0 is Feishu's virtual root and cannot be materialized. An
	// explicitly configured real root, however, must be part of the immutable
	// snapshot so direct members can reference it and all three providers expose
	// the same root semantics to the diff engine.
	for _, rootID := range roots {
		if rootID == "0" {
			continue
		}
		root, err := a.getDepartment(ctx, rootID)
		if err != nil {
			result.Completeness.PaginationComplete = false
			return result, fmt.Errorf("读取飞书同步根部门 %s 失败: %w", rootID, err)
		}
		orgByID[root.ExternalID] = root
	}
	for len(queue) > 0 {
		departmentID := queue[0]
		queue = queue[1:]
		if seenDepartments[departmentID] {
			continue
		}
		seenDepartments[departmentID] = true
		result.Completeness.TraversedOrgUnitIDs = append(result.Completeness.TraversedOrgUnitIDs, departmentID)

		if scope.IncludeDescendants {
			for pageToken := ""; ; {
				children, next, hasMore, err := a.listDepartmentChildren(ctx, departmentID, pageToken)
				if err != nil {
					result.Completeness.PaginationComplete = false
					return result, fmt.Errorf("遍历飞书部门 %s 失败: %w", departmentID, err)
				}
				for _, child := range children {
					orgByID[child.ExternalID] = child
					queue = append(queue, child.ExternalID)
				}
				if !hasMore {
					break
				}
				if next == "" || next == pageToken {
					result.Completeness.PaginationComplete = false
					return result, fmt.Errorf("飞书部门 %s 分页不完整或游标循环", departmentID)
				}
				pageToken = next
			}
		}

		for pageToken := ""; ; {
			items, next, hasMore, err := a.listUsersByDepartment(ctx, departmentID, pageToken)
			if err != nil {
				result.Completeness.PaginationComplete = false
				return result, fmt.Errorf("读取飞书部门 %s 直属用户失败: %w", departmentID, err)
			}
			for _, item := range items {
				observedUsers++
				if item.EmailFieldPresent {
					result.Completeness.FieldPermissions["email"] = "granted"
				}
				if item.MobileFieldPresent {
					result.Completeness.FieldPermissions["phone"] = "granted"
				}
				user := a.normalizeUser(item)
				if !scope.IncludeInactive && user.Status == "inactive" {
					continue
				}
				membership := Membership{OrgUnitExternalID: departmentID, Primary: departmentID == item.PrimaryDepartmentID, Leader: contains(item.LeaderDepartmentIDs, departmentID), Position: item.JobTitle, Raw: map[string]any{"department_id": departmentID}}
				if existing, ok := usersByID[user.StableID]; ok {
					user.Memberships = mergeMemberships(existing.Memberships, append(user.Memberships, membership))
				} else {
					user.Memberships = mergeMemberships(user.Memberships, []Membership{membership})
				}
				usersByID[user.StableID] = user
			}
			if !hasMore {
				break
			}
			if next == "" || next == pageToken {
				result.Completeness.PaginationComplete = false
				return result, fmt.Errorf("飞书部门 %s 用户分页不完整或游标循环", departmentID)
			}
			pageToken = next
		}
	}
	for _, permission := range []string{"email", "phone"} {
		if observedUsers > 0 && result.Completeness.FieldPermissions[permission] == "unknown" {
			result.Completeness.FieldPermissions[permission] = "not_granted"
		}
	}
	if observedUsers == 0 {
		result.Completeness.Warnings = append(result.Completeness.Warnings, "配置范围内没有可用于判断字段权限的用户；字段权限保持 unknown")
	}
	for _, org := range orgByID {
		result.OrgUnits = append(result.OrgUnits, org)
	}
	for _, user := range usersByID {
		result.Users = append(result.Users, user)
	}
	sort.Slice(result.OrgUnits, func(i, j int) bool { return result.OrgUnits[i].ExternalID < result.OrgUnits[j].ExternalID })
	sort.Slice(result.Users, func(i, j int) bool { return result.Users[i].StableID < result.Users[j].StableID })
	return result, nil
}

func (a *FeishuAdapter) getDepartment(ctx context.Context, departmentID string) (OrgUnit, error) {
	query := url.Values{"department_id_type": {"open_department_id"}}
	var response struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Department struct {
				OpenDepartmentID   string `json:"open_department_id"`
				ParentDepartmentID string `json:"parent_department_id"`
				Name               string `json:"name"`
				Order              string `json:"order"`
				LeaderUserID       string `json:"leader_user_id"`
				Status             any    `json:"status"`
			} `json:"department"`
		} `json:"data"`
	}
	path := "/open-apis/contact/v3/departments/" + url.PathEscape(departmentID)
	if err := a.get(ctx, path, query, &response); err != nil {
		return OrgUnit{}, err
	}
	item := response.Data.Department
	if item.OpenDepartmentID == "" || strings.TrimSpace(item.Name) == "" {
		return OrgUnit{}, fmt.Errorf("飞书部门详情缺少 open_department_id 或名称")
	}
	order, _ := strconv.Atoi(item.Order)
	return OrgUnit{
		ExternalID: item.OpenDepartmentID, ExternalParentID: item.ParentDepartmentID,
		Name: item.Name, SortOrder: order, LeaderExternalID: item.LeaderUserID,
		Status: normalizeFeishuStatus(item.Status),
		Raw:    map[string]any{"open_department_id": item.OpenDepartmentID, "parent_department_id": item.ParentDepartmentID, "name": item.Name, "order": item.Order, "leader_user_id": item.LeaderUserID, "status": item.Status},
	}, nil
}

func (a *FeishuAdapter) listDepartmentChildren(ctx context.Context, departmentID, pageToken string) ([]OrgUnit, string, bool, error) {
	query := url.Values{"department_id_type": {"open_department_id"}, "page_size": {"50"}}
	if pageToken != "" {
		query.Set("page_token", pageToken)
	}
	var response struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			HasMore   bool   `json:"has_more"`
			PageToken string `json:"page_token"`
			Items     []struct {
				OpenDepartmentID   string `json:"open_department_id"`
				ParentDepartmentID string `json:"parent_department_id"`
				Name               string `json:"name"`
				Order              string `json:"order"`
				LeaderUserID       string `json:"leader_user_id"`
				Status             any    `json:"status"`
			} `json:"items"`
		} `json:"data"`
	}
	path := "/open-apis/contact/v3/departments/" + url.PathEscape(departmentID) + "/children"
	if err := a.get(ctx, path, query, &response); err != nil {
		return nil, "", false, err
	}
	items := make([]OrgUnit, 0, len(response.Data.Items))
	for _, item := range response.Data.Items {
		order, _ := strconv.Atoi(item.Order)
		items = append(items, OrgUnit{ExternalID: item.OpenDepartmentID, ExternalParentID: item.ParentDepartmentID, Name: item.Name, SortOrder: order, LeaderExternalID: item.LeaderUserID, Status: normalizeFeishuStatus(item.Status), Raw: map[string]any{"open_department_id": item.OpenDepartmentID, "parent_department_id": item.ParentDepartmentID, "name": item.Name, "order": item.Order, "leader_user_id": item.LeaderUserID, "status": item.Status}})
	}
	return items, response.Data.PageToken, response.Data.HasMore, nil
}

func (a *FeishuAdapter) listUsersByDepartment(ctx context.Context, departmentID, pageToken string) ([]feishuUser, string, bool, error) {
	query := url.Values{"department_id": {departmentID}, "user_id_type": {"user_id"}, "department_id_type": {"open_department_id"}, "page_size": {"50"}}
	if pageToken != "" {
		query.Set("page_token", pageToken)
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
	if err := a.get(ctx, "/open-apis/contact/v3/users/find_by_department", query, &response); err != nil {
		return nil, "", false, err
	}
	return response.Data.Items, response.Data.PageToken, response.Data.HasMore, nil
}

func (a *FeishuAdapter) normalizeUser(item feishuUser) User {
	identifiers := []Identifier{{Namespace: "directory_user_id", Value: a.tenantKey + ":" + item.UserID, Primary: true}, {Namespace: "login_subject", Value: item.UserID}}
	if item.OpenID != "" {
		identifiers = append(identifiers, Identifier{Namespace: "open_id", Value: item.OpenID})
	}
	if item.UnionID != "" {
		identifiers = append(identifiers, Identifier{Namespace: "union_id", Value: item.UnionID})
	}
	memberships := make([]Membership, 0, len(item.DepartmentIDs))
	for index, departmentID := range item.DepartmentIDs {
		memberships = append(memberships, Membership{OrgUnitExternalID: departmentID, Primary: departmentID == item.PrimaryDepartmentID || (item.PrimaryDepartmentID == "" && index == 0), Leader: contains(item.LeaderDepartmentIDs, departmentID), Position: item.JobTitle, Raw: map[string]any{"department_id": departmentID}})
	}
	return User{StableID: a.tenantKey + ":" + item.UserID, TenantID: a.tenantKey, ProviderUserID: item.UserID, Identifiers: identifiers, DisplayName: item.Name, Email: item.Email, Phone: item.Mobile, EmployeeNo: item.EmployeeNo, Avatar: item.Avatar.Avatar240, Status: normalizeFeishuUserStatus(item.Status), Memberships: memberships, Raw: item.raw()}
}

func mergeMemberships(existing, incoming []Membership) []Membership {
	byDepartment := map[string]Membership{}
	for _, membership := range append(existing, incoming...) {
		if membership.OrgUnitExternalID != "" {
			if current, ok := byDepartment[membership.OrgUnitExternalID]; ok {
				membership.Primary = membership.Primary || current.Primary
				membership.Leader = membership.Leader || current.Leader
				if membership.Position == "" {
					membership.Position = current.Position
				}
			}
			byDepartment[membership.OrgUnitExternalID] = membership
		}
	}
	result := make([]Membership, 0, len(byDepartment))
	for _, membership := range byDepartment {
		result = append(result, membership)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].OrgUnitExternalID < result[j].OrgUnitExternalID })
	return result
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
	snapshot, err := a.ReadSnapshot(ctx, SyncScope{RootExternalOrgUnitIDs: []string{"0"}, IncludeDescendants: true, IncludeInactive: true})
	if err != nil {
		diagnostic.Warnings = append(diagnostic.Warnings, "无法完整遍历通讯录，请检查部门、用户权限和应用可见范围")
		if snapshot != nil {
			diagnostic.TraversedOrgUnitIDs = snapshot.Completeness.TraversedOrgUnitIDs
			diagnostic.PaginationComplete = snapshot.Completeness.PaginationComplete
			diagnostic.FieldPermissions = snapshot.Completeness.FieldPermissions
			diagnostic.ScopeHash = snapshot.Completeness.ScopeHash
		}
		return diagnostic, err
	}
	diagnostic.DirectoryReadable = true
	diagnostic.Healthy = snapshot.Completeness.PaginationComplete
	diagnostic.VisibleOrgUnits = len(snapshot.OrgUnits)
	diagnostic.VisibleUsers = len(snapshot.Users)
	diagnostic.TraversedOrgUnitIDs = snapshot.Completeness.TraversedOrgUnitIDs
	diagnostic.PaginationComplete = snapshot.Completeness.PaginationComplete
	diagnostic.FieldPermissions = snapshot.Completeness.FieldPermissions
	diagnostic.ScopeHash = snapshot.Completeness.ScopeHash
	if len(snapshot.OrgUnits) == 0 || len(snapshot.Users) == 0 {
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
		identifiers := []Identifier{{Namespace: "directory_user_id", Value: a.tenantKey + ":" + item.UserID, Primary: true}, {Namespace: "login_subject", Value: item.UserID}}
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
	EmailFieldPresent  bool `json:"-"`
	MobileFieldPresent bool `json:"-"`
}

func (u *feishuUser) UnmarshalJSON(data []byte) error {
	type alias feishuUser
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*u = feishuUser(decoded)
	_, u.EmailFieldPresent = raw["email"]
	_, u.MobileFieldPresent = raw["mobile"]
	return nil
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
			if attempt == 2 {
				return fmt.Errorf("飞书 API 网络请求失败: %w", err)
			}
			select {
			case <-request.Context().Done():
				return request.Context().Err()
			case <-time.After(time.Duration(attempt+1) * 100 * time.Millisecond):
			}
			continue
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
			retryAfter := parseRetryAfter(response.Header.Get("Retry-After"), time.Duration(attempt+1)*100*time.Millisecond, time.Now())
			select {
			case <-request.Context().Done():
				return request.Context().Err()
			case <-time.After(retryAfter):
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
