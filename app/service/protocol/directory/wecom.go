package directory

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const defaultWeComBaseURL = "https://qyapi.weixin.qq.com"

// WeComAdapter implements the read-only contract for an enterprise-owned
// application. Login and directory secrets deliberately remain separate.
type WeComAdapter struct {
	corpID             string
	directorySecret    string
	rootDepartmentID   string
	baseURL            string
	httpClient         *http.Client
	accessToken        string
	includeDescendants bool
}

func NewWeComAdapter(credentials map[string]any, client *http.Client) (*WeComAdapter, error) {
	value := directoryCredentialString(credentials)
	adapter := &WeComAdapter{
		corpID: value("corpId"), directorySecret: value("directorySecret"),
		rootDepartmentID: value("rootDepartmentId"), baseURL: strings.TrimRight(value("baseUrl"), "/"),
		httpClient:         client,
		includeDescendants: true,
	}
	if adapter.rootDepartmentID == "" {
		adapter.rootDepartmentID = "1"
	}
	if adapter.baseURL == "" {
		adapter.baseURL = defaultWeComBaseURL
	}
	if adapter.httpClient == nil {
		adapter.httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	if adapter.corpID == "" || adapter.directorySecret == "" {
		return nil, fmt.Errorf("企业微信目录凭据必须包含 corpId 和 directorySecret")
	}
	return adapter, nil
}

func (a *WeComAdapter) ReadSnapshot(ctx context.Context, scope SyncScope) (*Snapshot, error) {
	scope, scopeHash := effectiveScope(scope, []string{a.scopedDepartmentText(a.rootDepartmentID)})
	result := &Snapshot{Completeness: SnapshotCompleteness{PaginationComplete: true, ScopeHash: scopeHash, FieldPermissions: map[string]string{"email": "unknown", "phone": "unknown"}}}
	orgByID := map[string]OrgUnit{}
	userByID := map[string]User{}
	observedUsers := 0
	for _, configuredRoot := range scope.RootExternalOrgUnitIDs {
		root := strings.TrimPrefix(configuredRoot, a.corpID+":")
		if _, err := strconv.ParseInt(root, 10, 64); err != nil {
			result.Completeness.PaginationComplete = false
			return result, fmt.Errorf("企业微信根部门 %q 必须是数字部门 ID", configuredRoot)
		}
		clone := *a
		clone.rootDepartmentID = root
		clone.includeDescendants = scope.IncludeDescendants
		orgPage, err := clone.ListOrgUnits(ctx, "")
		if err != nil {
			result.Completeness.PaginationComplete = false
			return result, err
		}
		for _, org := range orgPage.Items {
			orgByID[org.ExternalID] = org
			result.Completeness.TraversedOrgUnitIDs = append(result.Completeness.TraversedOrgUnitIDs, org.ExternalID)
		}
		userPage, err := clone.ListUsers(ctx, "")
		if err != nil {
			result.Completeness.PaginationComplete = false
			return result, err
		}
		for _, user := range userPage.Items {
			observedUsers++
			if present, _ := user.Raw["__email_field_present"].(bool); present {
				result.Completeness.FieldPermissions["email"] = "granted"
			}
			if present, _ := user.Raw["__mobile_field_present"].(bool); present {
				result.Completeness.FieldPermissions["phone"] = "granted"
			}
			if !scope.IncludeInactive && user.Status == "inactive" {
				continue
			}
			if existing, ok := userByID[user.StableID]; ok {
				user.Memberships = mergeMemberships(existing.Memberships, user.Memberships)
			}
			userByID[user.StableID] = user
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
	result.Completeness.TraversedOrgUnitIDs = uniqueSortedStrings(result.Completeness.TraversedOrgUnitIDs)
	for _, org := range orgByID {
		result.OrgUnits = append(result.OrgUnits, org)
	}
	for _, user := range userByID {
		result.Users = append(result.Users, user)
	}
	sort.Slice(result.OrgUnits, func(i, j int) bool { return result.OrgUnits[i].ExternalID < result.OrgUnits[j].ExternalID })
	sort.Slice(result.Users, func(i, j int) bool { return result.Users[i].StableID < result.Users[j].StableID })
	return result, nil
}

func (a *WeComAdapter) ValidateDirectoryConfig(ctx context.Context) (*Diagnostic, error) {
	if _, err := a.token(ctx); err != nil {
		return &Diagnostic{Healthy: false, Warnings: []string{err.Error()}}, err
	}
	diagnostic := &Diagnostic{TokenValid: true, TenantID: a.corpID, Warnings: []string{}}
	snapshot, err := a.ReadSnapshot(ctx, SyncScope{IncludeDescendants: true, IncludeInactive: true})
	if err != nil {
		diagnostic.Warnings = append(diagnostic.Warnings, "无法完整读取企业微信通讯录，请检查 Secret、可信 IP、权限和可见范围")
		return diagnostic, err
	}
	diagnostic.Healthy = snapshot.Completeness.PaginationComplete
	diagnostic.DirectoryReadable = true
	diagnostic.VisibleOrgUnits = len(snapshot.OrgUnits)
	diagnostic.VisibleUsers = len(snapshot.Users)
	diagnostic.TraversedOrgUnitIDs = snapshot.Completeness.TraversedOrgUnitIDs
	diagnostic.PaginationComplete = snapshot.Completeness.PaginationComplete
	diagnostic.FieldPermissions = snapshot.Completeness.FieldPermissions
	diagnostic.ScopeHash = snapshot.Completeness.ScopeHash
	if len(snapshot.OrgUnits) == 0 || len(snapshot.Users) == 0 {
		diagnostic.Warnings = append(diagnostic.Warnings, "当前通讯录可见范围为空；请确认上游根部门与应用可见范围")
	}
	return diagnostic, nil
}

func (a *WeComAdapter) ListOrgUnits(ctx context.Context, cursor string) (*OrgUnitPage, error) {
	if cursor != "" {
		return &OrgUnitPage{Items: []OrgUnit{}}, nil
	}
	var response struct {
		ErrCode    int    `json:"errcode"`
		ErrMsg     string `json:"errmsg"`
		Department []struct {
			ID                int64    `json:"id"`
			Name              string   `json:"name"`
			ParentID          int64    `json:"parentid"`
			Order             int      `json:"order"`
			DepartmentLeaders []string `json:"department_leader"`
		} `json:"department"`
	}
	query := url.Values{"id": []string{a.rootDepartmentID}}
	if err := a.get(ctx, "/cgi-bin/department/list", query, &response); err != nil {
		return nil, err
	}
	items := make([]OrgUnit, 0, len(response.Department))
	for _, department := range response.Department {
		if !a.includeDescendants && strconv.FormatInt(department.ID, 10) != a.rootDepartmentID {
			continue
		}
		parentID := ""
		if department.ParentID > 0 {
			parentID = a.scopedDepartmentID(department.ParentID)
		}
		leaderID := ""
		if len(department.DepartmentLeaders) > 0 {
			leaderID = a.scopedUserID(department.DepartmentLeaders[0])
		}
		items = append(items, OrgUnit{
			ExternalID: a.scopedDepartmentID(department.ID), ExternalParentID: parentID,
			Name: department.Name, SortOrder: department.Order, LeaderExternalID: leaderID, Status: "active",
			Raw: map[string]any{"id": department.ID, "parentid": department.ParentID, "name": department.Name, "order": department.Order, "department_leader": department.DepartmentLeaders},
		})
	}
	return &OrgUnitPage{Items: items}, nil
}

func (a *WeComAdapter) ListUsers(ctx context.Context, cursor string) (*UserPage, error) {
	if cursor != "" {
		return &UserPage{Items: []User{}}, nil
	}
	var response struct {
		ErrCode  int         `json:"errcode"`
		ErrMsg   string      `json:"errmsg"`
		UserList []weComUser `json:"userlist"`
	}
	fetchChild := "0"
	if a.includeDescendants {
		fetchChild = "1"
	}
	query := url.Values{"department_id": []string{a.rootDepartmentID}, "fetch_child": []string{fetchChild}}
	if err := a.get(ctx, "/cgi-bin/user/list", query, &response); err != nil {
		return nil, err
	}
	items := make([]User, 0, len(response.UserList))
	for _, source := range response.UserList {
		if source.UserID == "" {
			continue
		}
		stableID := a.scopedUserID(source.UserID)
		identifiers := []Identifier{{Namespace: "directory_user_id", Value: stableID, Primary: true}, {Namespace: "login_subject", Value: source.UserID}}
		if source.OpenUserID != "" {
			identifiers = append(identifiers, Identifier{Namespace: "open_userid", Value: source.OpenUserID})
		}
		if source.UnionID != "" {
			identifiers = append(identifiers, Identifier{Namespace: "union_id", Value: source.UnionID})
		}
		memberships := make([]Membership, 0, len(source.Departments))
		for index, departmentID := range source.Departments {
			order := parallelInt(source.Order, index)
			memberships = append(memberships, Membership{
				OrgUnitExternalID: a.scopedDepartmentID(departmentID), Primary: index == 0,
				Leader: parallelInt(source.IsLeaderInDept, index) == 1, Position: source.Position, SortOrder: order,
				Raw: map[string]any{"department_id": departmentID, "order": order},
			})
		}
		status := "inactive"
		if source.Status == 1 {
			status = "active"
		}
		items = append(items, User{
			StableID: stableID, TenantID: a.corpID, ProviderUserID: source.UserID,
			Identifiers: identifiers, DisplayName: source.Name, Email: source.Email, Phone: source.Mobile,
			EmployeeNo: source.Alias, Avatar: source.Avatar, Status: status, Memberships: memberships, Raw: source.raw(),
		})
	}
	return &UserPage{Items: items}, nil
}

type weComUser struct {
	UserID             string  `json:"userid"`
	OpenUserID         string  `json:"open_userid"`
	UnionID            string  `json:"unionid"`
	Name               string  `json:"name"`
	Alias              string  `json:"alias"`
	Mobile             string  `json:"mobile"`
	Email              string  `json:"email"`
	Position           string  `json:"position"`
	Avatar             string  `json:"avatar"`
	Status             int     `json:"status"`
	Departments        []int64 `json:"department"`
	Order              []int   `json:"order"`
	IsLeaderInDept     []int   `json:"is_leader_in_dept"`
	EmailFieldPresent  bool    `json:"-"`
	MobileFieldPresent bool    `json:"-"`
}

func (u *weComUser) UnmarshalJSON(data []byte) error {
	type alias weComUser
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*u = weComUser(decoded)
	_, u.EmailFieldPresent = raw["email"]
	_, u.MobileFieldPresent = raw["mobile"]
	return nil
}

func (u weComUser) raw() map[string]any {
	return map[string]any{
		"userid": u.UserID, "open_userid": u.OpenUserID, "unionid": u.UnionID, "name": u.Name,
		"alias": u.Alias, "mobile": u.Mobile, "email": u.Email, "position": u.Position,
		"avatar": u.Avatar, "status": u.Status, "department": u.Departments,
		"order": u.Order, "is_leader_in_dept": u.IsLeaderInDept,
		"__email_field_present": u.EmailFieldPresent, "__mobile_field_present": u.MobileFieldPresent,
	}
}

func (a *WeComAdapter) token(ctx context.Context) (string, error) {
	if a.accessToken != "" {
		return a.accessToken, nil
	}
	query := url.Values{"corpid": []string{a.corpID}, "corpsecret": []string{a.directorySecret}}
	var response struct {
		ErrCode     int    `json:"errcode"`
		ErrMsg      string `json:"errmsg"`
		AccessToken string `json:"access_token"`
	}
	if err := directoryJSONRequest(ctx, a.httpClient, http.MethodGet, a.baseURL+"/cgi-bin/gettoken?"+query.Encode(), nil, &response, "企业微信"); err != nil {
		return "", err
	}
	if response.ErrCode != 0 || response.AccessToken == "" {
		return "", fmt.Errorf("企业微信获取 access token 失败: errcode=%d errmsg=%s", response.ErrCode, response.ErrMsg)
	}
	a.accessToken = response.AccessToken
	return a.accessToken, nil
}

func (a *WeComAdapter) get(ctx context.Context, path string, query url.Values, target any) error {
	return a.getWithTokenRefresh(ctx, path, query, target, true)
}

func (a *WeComAdapter) getWithTokenRefresh(ctx context.Context, path string, query url.Values, target any, allowRefresh bool) error {
	token, err := a.token(ctx)
	if err != nil {
		return err
	}
	query.Set("access_token", token)
	if err := directoryJSONRequest(ctx, a.httpClient, http.MethodGet, a.baseURL+path+"?"+query.Encode(), nil, target, "企业微信"); err != nil {
		return err
	}
	encoded, _ := json.Marshal(target)
	var envelope struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	_ = json.Unmarshal(encoded, &envelope)
	if allowRefresh && (envelope.ErrCode == 40014 || envelope.ErrCode == 42001) {
		a.accessToken = ""
		return a.getWithTokenRefresh(ctx, path, query, target, false)
	}
	if envelope.ErrCode != 0 {
		return fmt.Errorf("企业微信通讯录 API 失败: errcode=%d errmsg=%s", envelope.ErrCode, envelope.ErrMsg)
	}
	return nil
}

func (a *WeComAdapter) scopedUserID(userID string) string {
	return a.corpID + ":" + userID
}

func (a *WeComAdapter) scopedDepartmentID(departmentID int64) string {
	return a.corpID + ":" + strconv.FormatInt(departmentID, 10)
}

func (a *WeComAdapter) scopedDepartmentText(departmentID string) string {
	return a.corpID + ":" + departmentID
}

func uniqueSortedStrings(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}

func parallelInt(values []int, index int) int {
	if index >= 0 && index < len(values) {
		return values[index]
	}
	return 0
}

func directoryCredentialString(credentials map[string]any) func(string) string {
	return func(key string) string {
		value, _ := credentials[key].(string)
		return strings.TrimSpace(value)
	}
}
