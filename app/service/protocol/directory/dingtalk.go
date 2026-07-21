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

const defaultDingTalkBaseURL = "https://oapi.dingtalk.com"

// DingTalkAdapter uses one fixed generation of the internal-app server APIs:
// /topapi/v2/department/* and /topapi/v2/user/list.
type DingTalkAdapter struct {
	appKey             string
	appSecret          string
	corpID             string
	rootDepartmentID   int64
	baseURL            string
	httpClient         *http.Client
	accessToken        string
	includeDescendants bool
}

func NewDingTalkAdapter(credentials map[string]any, client *http.Client) (*DingTalkAdapter, error) {
	value := directoryCredentialString(credentials)
	rootID := int64(1)
	if configured := value("rootDepartmentId"); configured != "" {
		parsed, err := strconv.ParseInt(configured, 10, 64)
		if err != nil || parsed <= 0 {
			return nil, fmt.Errorf("钉钉 rootDepartmentId 必须是正整数")
		}
		rootID = parsed
	}
	adapter := &DingTalkAdapter{
		appKey: value("appKey"), appSecret: value("appSecret"), corpID: value("corpId"),
		rootDepartmentID: rootID, baseURL: strings.TrimRight(value("baseUrl"), "/"), httpClient: client,
		includeDescendants: true,
	}
	if adapter.baseURL == "" {
		adapter.baseURL = defaultDingTalkBaseURL
	}
	if adapter.httpClient == nil {
		adapter.httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	if adapter.appKey == "" || adapter.appSecret == "" || adapter.corpID == "" {
		return nil, fmt.Errorf("钉钉目录凭据必须包含 appKey、appSecret 和 corpId")
	}
	return adapter, nil
}

func (a *DingTalkAdapter) ReadSnapshot(ctx context.Context, scope SyncScope) (*Snapshot, error) {
	scope, scopeHash := effectiveScope(scope, []string{a.scopedDepartmentID(a.rootDepartmentID)})
	result := &Snapshot{Completeness: SnapshotCompleteness{PaginationComplete: true, ScopeHash: scopeHash, FieldPermissions: map[string]string{"email": "unknown", "phone": "unknown"}}}
	orgByID := map[string]OrgUnit{}
	userByID := map[string]User{}
	observedUsers := 0
	for _, configuredRoot := range scope.RootExternalOrgUnitIDs {
		rawRoot := strings.TrimPrefix(configuredRoot, a.corpID+":")
		rootID, err := strconv.ParseInt(rawRoot, 10, 64)
		if err != nil || rootID <= 0 {
			result.Completeness.PaginationComplete = false
			return result, fmt.Errorf("钉钉根部门 %q 必须是正整数", configuredRoot)
		}
		clone := *a
		clone.rootDepartmentID = rootID
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

func (a *DingTalkAdapter) ValidateDirectoryConfig(ctx context.Context) (*Diagnostic, error) {
	if _, err := a.token(ctx); err != nil {
		return &Diagnostic{Healthy: false, Warnings: []string{err.Error()}}, err
	}
	diagnostic := &Diagnostic{TokenValid: true, TenantID: a.corpID, Warnings: []string{}}
	snapshot, err := a.ReadSnapshot(ctx, SyncScope{IncludeDescendants: true, IncludeInactive: true})
	if err != nil {
		diagnostic.Warnings = append(diagnostic.Warnings, "无法完整读取钉钉通讯录，请检查权限、字段授权和应用可见范围")
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
		diagnostic.Warnings = append(diagnostic.Warnings, "当前钉钉应用的通讯录可见范围为空")
	}
	return diagnostic, nil
}

func (a *DingTalkAdapter) ListOrgUnits(ctx context.Context, cursor string) (*OrgUnitPage, error) {
	if cursor != "" {
		return &OrgUnitPage{Items: []OrgUnit{}}, nil
	}
	departments, err := a.fetchDepartments(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]OrgUnit, 0, len(departments))
	for _, department := range departments {
		parentID := ""
		if department.ParentID > 0 && department.ID != a.rootDepartmentID {
			parentID = a.scopedDepartmentID(department.ParentID)
		}
		leaderID := ""
		if len(department.ManagerUserIDs) > 0 {
			leaderID = a.scopedUserID(department.ManagerUserIDs[0])
		}
		items = append(items, OrgUnit{
			ExternalID: a.scopedDepartmentID(department.ID), ExternalParentID: parentID,
			Name: department.Name, SortOrder: department.Order, LeaderExternalID: leaderID,
			Status: "active", Raw: department.raw(),
		})
	}
	return &OrgUnitPage{Items: items}, nil
}

func (a *DingTalkAdapter) ListUsers(ctx context.Context, cursor string) (*UserPage, error) {
	if cursor != "" {
		return &UserPage{Items: []User{}}, nil
	}
	departments, err := a.fetchDepartments(ctx)
	if err != nil {
		return nil, err
	}
	byID := map[string]*User{}
	for _, department := range departments {
		var pageCursor int64
		for {
			var response struct {
				DingTalkEnvelope
				Result struct {
					HasMore    bool           `json:"has_more"`
					NextCursor int64          `json:"next_cursor"`
					List       []dingTalkUser `json:"list"`
				} `json:"result"`
			}
			payload := map[string]any{"dept_id": department.ID, "cursor": pageCursor, "size": 100, "contain_access_limit": true}
			if err := a.post(ctx, "/topapi/v2/user/list", payload, &response); err != nil {
				return nil, err
			}
			for _, source := range response.Result.List {
				a.mergeUser(byID, source, department.ID)
			}
			if !response.Result.HasMore {
				break
			}
			if response.Result.NextCursor == pageCursor {
				return nil, fmt.Errorf("钉钉用户分页游标循环: %d", pageCursor)
			}
			pageCursor = response.Result.NextCursor
		}
	}
	items := make([]User, 0, len(byID))
	for _, user := range byID {
		sort.SliceStable(user.Memberships, func(i, j int) bool {
			if user.Memberships[i].Primary != user.Memberships[j].Primary {
				return user.Memberships[i].Primary
			}
			return user.Memberships[i].OrgUnitExternalID < user.Memberships[j].OrgUnitExternalID
		})
		items = append(items, *user)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].StableID < items[j].StableID })
	return &UserPage{Items: items}, nil
}

type DingTalkEnvelope struct {
	ErrCode int    `json:"errcode"`
	ErrMsg  string `json:"errmsg"`
}

type dingTalkDepartment struct {
	ID             int64    `json:"dept_id"`
	ParentID       int64    `json:"parent_id"`
	Name           string   `json:"name"`
	Order          int      `json:"order"`
	ManagerUserIDs []string `json:"dept_manager_userid_list"`
}

func (d dingTalkDepartment) raw() map[string]any {
	return map[string]any{"dept_id": d.ID, "parent_id": d.ParentID, "name": d.Name, "order": d.Order, "dept_manager_userid_list": d.ManagerUserIDs}
}

type dingTalkUser struct {
	UserID             string  `json:"userid"`
	OpenID             string  `json:"openid"`
	UnionID            string  `json:"unionid"`
	Name               string  `json:"name"`
	Email              string  `json:"email"`
	Mobile             string  `json:"mobile"`
	Avatar             string  `json:"avatar"`
	JobNumber          string  `json:"job_number"`
	Title              string  `json:"title"`
	Active             bool    `json:"active"`
	DeptIDs            []int64 `json:"dept_id_list"`
	LeaderInDept       bool    `json:"leader_in_dept"`
	ManagerID          string  `json:"manager_userid"`
	EmailFieldPresent  bool    `json:"-"`
	MobileFieldPresent bool    `json:"-"`
}

func (u *dingTalkUser) UnmarshalJSON(data []byte) error {
	type alias dingTalkUser
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*u = dingTalkUser(decoded)
	_, u.EmailFieldPresent = raw["email"]
	_, u.MobileFieldPresent = raw["mobile"]
	return nil
}

func (u dingTalkUser) raw() map[string]any {
	return map[string]any{
		"userid": u.UserID, "openid": u.OpenID, "unionid": u.UnionID, "name": u.Name,
		"email": u.Email, "mobile": u.Mobile, "avatar": u.Avatar, "job_number": u.JobNumber,
		"title": u.Title, "active": u.Active, "dept_id_list": u.DeptIDs,
		"leader_in_dept": u.LeaderInDept, "manager_userid": u.ManagerID,
		"__email_field_present": u.EmailFieldPresent, "__mobile_field_present": u.MobileFieldPresent,
	}
}

func (a *DingTalkAdapter) mergeUser(byID map[string]*User, source dingTalkUser, queriedDepartmentID int64) {
	if source.UserID == "" {
		return
	}
	stableID := a.scopedUserID(source.UserID)
	user := byID[stableID]
	if user == nil {
		identifiers := []Identifier{{Namespace: "directory_user_id", Value: stableID, Primary: true}, {Namespace: "login_subject", Value: source.UserID}}
		if source.OpenID != "" {
			identifiers = append(identifiers, Identifier{Namespace: "open_id", Value: source.OpenID})
		}
		if source.UnionID != "" {
			identifiers = append(identifiers, Identifier{Namespace: "union_id", Value: source.UnionID})
		}
		status := "inactive"
		if source.Active {
			status = "active"
		}
		user = &User{
			StableID: stableID, TenantID: a.corpID, ProviderUserID: source.UserID, Identifiers: identifiers,
			DisplayName: source.Name, Email: source.Email, Phone: source.Mobile, EmployeeNo: source.JobNumber,
			Avatar: source.Avatar, Status: status, Raw: source.raw(),
		}
		byID[stableID] = user
	}
	departmentIDs := source.DeptIDs
	if len(departmentIDs) == 0 {
		departmentIDs = []int64{queriedDepartmentID}
	}
	for index, departmentID := range departmentIDs {
		externalID := a.scopedDepartmentID(departmentID)
		if existing := membershipIndex(user.Memberships, externalID); existing >= 0 {
			if source.LeaderInDept && departmentID == queriedDepartmentID {
				user.Memberships[existing].Leader = true
			}
			continue
		}
		user.Memberships = append(user.Memberships, Membership{
			OrgUnitExternalID: externalID, Primary: index == 0,
			Leader:   source.LeaderInDept && departmentID == queriedDepartmentID,
			Position: source.Title, Raw: map[string]any{"department_id": departmentID},
		})
	}
}

func (a *DingTalkAdapter) fetchDepartments(ctx context.Context) ([]dingTalkDepartment, error) {
	root, err := a.getDepartment(ctx, a.rootDepartmentID)
	if err != nil {
		return nil, err
	}
	departments := []dingTalkDepartment{root}
	if !a.includeDescendants {
		return departments, nil
	}
	queue := []int64{root.ID}
	seen := map[int64]bool{root.ID: true}
	for len(queue) > 0 {
		parentID := queue[0]
		queue = queue[1:]
		var response struct {
			DingTalkEnvelope
			Result []dingTalkDepartment `json:"result"`
		}
		if err := a.post(ctx, "/topapi/v2/department/listsub", map[string]any{"dept_id": parentID}, &response); err != nil {
			return nil, err
		}
		for _, department := range response.Result {
			if department.ID <= 0 || seen[department.ID] {
				continue
			}
			seen[department.ID] = true
			departments = append(departments, department)
			queue = append(queue, department.ID)
			if len(departments) > 10000 {
				return nil, fmt.Errorf("钉钉部门超过单连接安全上限 10000")
			}
		}
	}
	return departments, nil
}

func (a *DingTalkAdapter) getDepartment(ctx context.Context, departmentID int64) (dingTalkDepartment, error) {
	var response struct {
		DingTalkEnvelope
		Result dingTalkDepartment `json:"result"`
	}
	if err := a.post(ctx, "/topapi/v2/department/get", map[string]any{"dept_id": departmentID}, &response); err != nil {
		return dingTalkDepartment{}, err
	}
	if response.Result.ID <= 0 || strings.TrimSpace(response.Result.Name) == "" {
		return dingTalkDepartment{}, fmt.Errorf("钉钉根部门返回缺少 dept_id 或 name")
	}
	return response.Result, nil
}

func (a *DingTalkAdapter) token(ctx context.Context) (string, error) {
	if a.accessToken != "" {
		return a.accessToken, nil
	}
	query := url.Values{"appkey": []string{a.appKey}, "appsecret": []string{a.appSecret}}
	var response struct {
		DingTalkEnvelope
		AccessToken string `json:"access_token"`
	}
	if err := directoryJSONRequest(ctx, a.httpClient, http.MethodGet, a.baseURL+"/gettoken?"+query.Encode(), nil, &response, "钉钉"); err != nil {
		return "", err
	}
	if response.ErrCode != 0 || response.AccessToken == "" {
		return "", fmt.Errorf("钉钉获取 access token 失败: errcode=%d errmsg=%s", response.ErrCode, response.ErrMsg)
	}
	a.accessToken = response.AccessToken
	return a.accessToken, nil
}

func (a *DingTalkAdapter) post(ctx context.Context, path string, payload any, target any) error {
	return a.postWithTokenRefresh(ctx, path, payload, target, true)
}

func (a *DingTalkAdapter) postWithTokenRefresh(ctx context.Context, path string, payload any, target any, allowRefresh bool) error {
	token, err := a.token(ctx)
	if err != nil {
		return err
	}
	endpoint := a.baseURL + path + "?" + url.Values{"access_token": []string{token}}.Encode()
	if err := directoryJSONRequest(ctx, a.httpClient, http.MethodPost, endpoint, payload, target, "钉钉"); err != nil {
		return err
	}
	encoded, _ := json.Marshal(target)
	var envelope DingTalkEnvelope
	_ = json.Unmarshal(encoded, &envelope)
	if allowRefresh && (envelope.ErrCode == 88 || envelope.ErrCode == 40014 || envelope.ErrCode == 42001) {
		a.accessToken = ""
		return a.postWithTokenRefresh(ctx, path, payload, target, false)
	}
	if envelope.ErrCode != 0 {
		return fmt.Errorf("钉钉通讯录 API 失败: errcode=%d errmsg=%s", envelope.ErrCode, envelope.ErrMsg)
	}
	return nil
}

func (a *DingTalkAdapter) scopedUserID(userID string) string { return a.corpID + ":" + userID }
func (a *DingTalkAdapter) scopedDepartmentID(departmentID int64) string {
	return a.corpID + ":" + strconv.FormatInt(departmentID, 10)
}

func membershipIndex(items []Membership, externalID string) int {
	for index, item := range items {
		if item.OrgUnitExternalID == externalID {
			return index
		}
	}
	return -1
}
