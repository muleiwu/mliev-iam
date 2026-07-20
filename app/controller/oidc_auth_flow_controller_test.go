package controller

import (
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"cnb.cool/mliev/open/go-web/pkg/container"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

var controllerTestDBProviderPriority int64 = 1000

func TestBuildLoginVerificationSessionIDIsSceneScoped(t *testing.T) {
	phoneSessionA := buildLoginVerificationSessionID("session-a", service.VerificationTypePhone, "13800138000")
	phoneSessionB := buildLoginVerificationSessionID("session-b", service.VerificationTypePhone, "13800138000")
	emailSessionA := buildLoginVerificationSessionID("session-a", service.VerificationTypeEmail, "user@example.com")

	if phoneSessionA == phoneSessionB {
		t.Fatal("different OIDC sessions must not share login verification code keys")
	}
	if phoneSessionA == emailSessionA {
		t.Fatal("phone and email login verification code keys must be isolated")
	}
	if phoneSessionA == "13800138000" {
		t.Fatal("login verification code key must include login scene and session context")
	}
}

func TestGetClientConfigReturnsDefaultFrontendContract(t *testing.T) {
	db := registerControllerTestDatabase(t)
	app := &model.Application{
		UUID:           "client-a",
		Name:           "测试应用",
		Description:    "用于 OIDC 登录页",
		Icon:           "/attachments/018f47df-7f5b-7f88-a7bd-1d3d9a69ea12",
		ProtocolType:   string(model.ProtocolTypeOIDC),
		ProtocolConfig: map[string]interface{}{"clientId": "client-a"},
		IsActive:       true,
	}
	if err := db.Create(app).Error; err != nil {
		t.Fatalf("create application: %v", err)
	}

	ctx := newTestRouterContext(map[string]string{"client_id": "client-a"})
	OIDCAuthFlowController{}.GetClientConfig(ctx)

	resp := assertControllerResponse[dto.Response](t, ctx.payload)
	if ctx.status != http.StatusOK {
		t.Fatalf("status = %d, want %d", ctx.status, http.StatusOK)
	}
	if resp.Code != 0 {
		t.Fatalf("code = %d, want 0: %s", resp.Code, resp.Message)
	}
	data, ok := resp.Data.(ClientConfigResponse)
	if !ok {
		t.Fatalf("data type = %T, want ClientConfigResponse", resp.Data)
	}

	if data.ClientID != "client-a" || data.AppName != "测试应用" || data.Description != "用于 OIDC 登录页" {
		t.Fatalf("unexpected client identity: %+v", data)
	}
	if data.Logo != app.Icon {
		t.Fatalf("logo = %q, want %q", data.Logo, app.Icon)
	}
	if data.Appearance.PrimaryColor != "#3b82f6" || data.Appearance.DarkMode != "auto" {
		t.Fatalf("unexpected appearance defaults: %+v", data.Appearance)
	}
	if !data.Features.Registration || !data.Features.SocialLogin || !data.Features.EnterpriseLogin || !data.Features.RememberMe || !data.Features.PasswordStrength {
		t.Fatalf("expected enabled feature defaults: %+v", data.Features)
	}
	if data.Features.ForgotPassword {
		t.Fatalf("forgot password must be disabled when verification-code service is unconfigured")
	}
	if !data.LoginMethods.Username || !data.LoginMethods.Email || !data.LoginMethods.Phone {
		t.Fatalf("unexpected login method defaults: %+v", data.LoginMethods)
	}
	if data.SocialProviders == nil || len(data.SocialProviders) != 0 {
		t.Fatalf("socialProviders must be an empty array, got %#v", data.SocialProviders)
	}
	if data.Locale != "zh-CN" {
		t.Fatalf("locale = %q, want zh-CN", data.Locale)
	}
	if len(data.Agreements) != 0 || data.TermsURL != "" || data.PrivacyPolicyURL != "" {
		t.Fatalf("disabled default agreements must not be returned: %+v", data)
	}
}

func TestBuildFlowStatusClientIncludesApplicationIcon(t *testing.T) {
	app := &model.Application{
		Name:        "流程测试应用",
		Description: "用于测试授权流程品牌信息",
		Icon:        "/attachments/018f47df-7f5b-7f88-a7bd-1d3d9a69ea12",
	}

	client := buildFlowStatusClient(app, "client-a")
	if client.ClientID != "client-a" || client.Name != app.Name || client.Description != app.Description {
		t.Fatalf("unexpected flow status client: %+v", client)
	}
	if client.Logo != app.Icon {
		t.Fatalf("logo = %q, want %q", client.Logo, app.Icon)
	}
}

func TestGetClientConfigReturnsEnabledAgreementLinks(t *testing.T) {
	db := registerControllerTestDatabase(t)
	app := &model.Application{
		UUID:           "client-a",
		Name:           "测试应用",
		ProtocolType:   string(model.ProtocolTypeOIDC),
		ProtocolConfig: map[string]interface{}{"clientId": "client-a"},
		IsActive:       true,
	}
	if err := db.Create(app).Error; err != nil {
		t.Fatalf("create application: %v", err)
	}
	if _, err := service.NewAgreementService(db).UpsertGlobalAgreement(model.AgreementCodeTerms, &dto.AgreementUpsertRequest{
		Title:     "服务协议",
		ContentMD: "# Terms",
		Enabled:   true,
		Sort:      10,
	}); err != nil {
		t.Fatalf("upsert global terms: %v", err)
	}
	if _, err := service.NewAgreementService(db).CreateApplicationAgreement(app.ID, &dto.AgreementUpsertRequest{
		Code:      "extra-policy",
		Title:     "应用协议",
		ContentMD: "# App",
		Enabled:   true,
		Sort:      1,
	}); err != nil {
		t.Fatalf("create app agreement: %v", err)
	}

	ctx := newTestRouterContext(map[string]string{"client_id": "client-a"})
	OIDCAuthFlowController{}.GetClientConfig(ctx)

	resp := assertControllerResponse[dto.Response](t, ctx.payload)
	if resp.Code != 0 {
		t.Fatalf("code = %d, want 0: %s", resp.Code, resp.Message)
	}
	data, ok := resp.Data.(ClientConfigResponse)
	if !ok {
		t.Fatalf("data type = %T, want ClientConfigResponse", resp.Data)
	}
	if data.TermsURL != "/auth/#/agreements/terms" {
		t.Fatalf("termsUrl = %q", data.TermsURL)
	}
	if len(data.Agreements) != 2 {
		t.Fatalf("agreements len = %d, want 2: %+v", len(data.Agreements), data.Agreements)
	}
	if data.Agreements[0].Key != "global:terms" || data.Agreements[1].Key != "application:extra-policy" {
		t.Fatalf("unexpected agreement order: %+v", data.Agreements)
	}
}

func TestGetClientConfigRejectsUnknownClient(t *testing.T) {
	registerControllerTestDatabase(t)

	ctx := newTestRouterContext(map[string]string{"client_id": "missing-client"})
	OIDCAuthFlowController{}.GetClientConfig(ctx)

	resp := assertControllerResponse[dto.Response](t, ctx.payload)
	if ctx.status != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", ctx.status, http.StatusNotFound)
	}
	if resp.Code != http.StatusNotFound {
		t.Fatalf("code = %d, want %d", resp.Code, http.StatusNotFound)
	}
}

func TestGetClientConfigRejectsMissingClientContext(t *testing.T) {
	registerControllerTestDatabase(t)

	ctx := newTestRouterContext(nil)
	OIDCAuthFlowController{}.GetClientConfig(ctx)

	resp := assertControllerResponse[dto.Response](t, ctx.payload)
	if ctx.status != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", ctx.status, http.StatusBadRequest)
	}
	if !strings.Contains(resp.Message, "缺少client_id或session_id参数") {
		t.Fatalf("message = %q, want missing client context", resp.Message)
	}
}

func TestResolveClientConfigClientID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		clientID   string
		sessionID  string
		session    *service.OIDCSession
		sessionErr error
		want       string
		wantErr    string
	}{
		{
			name:     "client id only",
			clientID: "client-a",
			want:     "client-a",
		},
		{
			name:      "session id only resolves client",
			sessionID: "session-a",
			session:   &service.OIDCSession{ClientID: "client-a"},
			want:      "client-a",
		},
		{
			name:      "client and session must match",
			clientID:  "client-a",
			sessionID: "session-a",
			session:   &service.OIDCSession{ClientID: "client-a"},
			want:      "client-a",
		},
		{
			name:      "client and session mismatch",
			clientID:  "client-a",
			sessionID: "session-a",
			session:   &service.OIDCSession{ClientID: "client-b"},
			wantErr:   "会话与客户端不匹配",
		},
		{
			name:    "missing client context",
			wantErr: "缺少client_id或session_id参数",
		},
		{
			name:       "expired session",
			sessionID:  "session-a",
			sessionErr: errors.New("会话已过期"),
			wantErr:    "无效的会话ID或会话已过期",
		},
		{
			name:      "session without client",
			sessionID: "session-a",
			session:   &service.OIDCSession{},
			wantErr:   "会话缺少客户端信息",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := resolveClientConfigClientID(tt.clientID, tt.sessionID, func(string) (*service.OIDCSession, error) {
				return tt.session, tt.sessionErr
			})
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %q, want %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("clientID = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestValidateClientConfigSessionRejectsClientMismatch(t *testing.T) {
	err := validateClientConfigSession("client-a", &service.OIDCSession{ClientID: "client-b"})
	if err == nil {
		t.Fatal("expected mismatch error")
	}
	if !strings.Contains(err.Error(), "会话与客户端不匹配") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOIDCRefreshTokenRejectsMissingCookie(t *testing.T) {
	ctx := newTestRouterContext(nil)

	OIDCAuthFlowController{}.RefreshToken(ctx)

	resp := assertControllerResponse[dto.Response](t, ctx.payload)
	if ctx.status != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", ctx.status, http.StatusUnauthorized)
	}
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("code = %d, want %d", resp.Code, http.StatusUnauthorized)
	}
}

func registerControllerTestDatabase(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.Application{}, &model.Agreement{}, &model.AgreementAcceptance{}); err != nil {
		t.Fatalf("migrate test tables: %v", err)
	}

	priority := int(atomic.AddInt64(&controllerTestDBProviderPriority, 1))
	provider := container.NewSimpleProviderWithPriority(reflect.TypeFor[*gorm.DB](), db, priority)
	container.Register(provider)
	return db
}

func assertControllerResponse[T any](t *testing.T, payload any) T {
	t.Helper()

	resp, ok := payload.(T)
	if !ok {
		t.Fatalf("payload type = %T, want %T", payload, *new(T))
	}
	return resp
}

type testRouterContext struct {
	query     map[string]string
	values    map[string]any
	headers   http.Header
	status    int
	payload   any
	req       *http.Request
	aborted   bool
	sameSite  http.SameSite
	setCookie []http.Cookie
}

func newTestRouterContext(query map[string]string) *testRouterContext {
	req := httptest.NewRequest(http.MethodGet, "/api/oidc/client-config", nil)
	q := req.URL.Query()
	for key, value := range query {
		q.Set(key, value)
	}
	req.URL.RawQuery = q.Encode()

	return &testRouterContext{
		query:   query,
		values:  map[string]any{},
		headers: http.Header{},
		req:     req,
	}
}

func (c *testRouterContext) JSON(code int, obj any) {
	c.status = code
	c.payload = obj
}

func (c *testRouterContext) HTML(code int, _ string, obj any) {
	c.JSON(code, obj)
}

func (c *testRouterContext) String(code int, format string, values ...any) {
	c.JSON(code, map[string]any{"format": format, "values": values})
}

func (c *testRouterContext) Data(code int, _ string, data []byte) {
	c.JSON(code, data)
}

func (c *testRouterContext) Redirect(code int, location string) {
	c.JSON(code, map[string]string{"location": location})
}

func (c *testRouterContext) File(string) {}

func (c *testRouterContext) Stream(func(w io.Writer) bool) {}

func (c *testRouterContext) Status(code int) {
	c.status = code
}

func (c *testRouterContext) Query(key string) string {
	if c.query != nil {
		return c.query[key]
	}
	return c.req.URL.Query().Get(key)
}

func (c *testRouterContext) DefaultQuery(key, defaultValue string) string {
	if value := c.Query(key); value != "" {
		return value
	}
	return defaultValue
}

func (c *testRouterContext) QueryArray(key string) []string {
	return c.req.URL.Query()[key]
}

func (c *testRouterContext) PostForm(string) string { return "" }

func (c *testRouterContext) DefaultPostForm(_, defaultValue string) string {
	return defaultValue
}

func (c *testRouterContext) ShouldBind(any) error { return nil }

func (c *testRouterContext) ShouldBindJSON(any) error { return nil }

func (c *testRouterContext) ShouldBindQuery(any) error { return nil }

func (c *testRouterContext) GetRawData() ([]byte, error) { return nil, nil }

func (c *testRouterContext) FormFile(string) (*multipart.FileHeader, error) {
	return nil, errors.New("no file")
}

func (c *testRouterContext) SaveUploadedFile(*multipart.FileHeader, string) error {
	return nil
}

func (c *testRouterContext) Set(key string, value any) {
	c.values[key] = value
}

func (c *testRouterContext) Get(key string) any {
	return c.values[key]
}

func (c *testRouterContext) GetString(key string) string {
	value, _ := c.values[key].(string)
	return value
}

func (c *testRouterContext) Param(string) string { return "" }

func (c *testRouterContext) Path() string { return c.req.URL.Path }

func (c *testRouterContext) FullPath() string { return c.req.URL.Path }

func (c *testRouterContext) Method() string { return c.req.Method }

func (c *testRouterContext) ClientIP() string { return "127.0.0.1" }

func (c *testRouterContext) RemoteAddr() string { return c.req.RemoteAddr }

func (c *testRouterContext) Host() string { return c.req.Host }

func (c *testRouterContext) Hostname() string { return c.req.Host }

func (c *testRouterContext) Scheme() string { return "http" }

func (c *testRouterContext) URL() string { return c.req.URL.String() }

func (c *testRouterContext) ContentType() string { return "" }

func (c *testRouterContext) UserAgent() string { return c.req.UserAgent() }

func (c *testRouterContext) Referer() string { return c.req.Referer() }

func (c *testRouterContext) IsWebsocket() bool { return false }

func (c *testRouterContext) GetHeader(key string) string {
	if value := c.headers.Get(key); value != "" {
		return value
	}
	return c.req.Header.Get(key)
}

func (c *testRouterContext) SetHeader(key, value string) {
	c.headers.Set(key, value)
}

func (c *testRouterContext) Cookie(name string) (string, error) {
	cookie, err := c.req.Cookie(name)
	if err != nil {
		return "", err
	}
	return cookie.Value, nil
}

func (c *testRouterContext) SetCookie(name, value string, maxAge int, path, domain string, secure, httpOnly bool) {
	c.setCookie = append(c.setCookie, http.Cookie{
		Name:     name,
		Value:    value,
		Path:     path,
		Domain:   domain,
		MaxAge:   maxAge,
		Secure:   secure,
		HttpOnly: httpOnly,
	})
}

func (c *testRouterContext) SetSameSite(mode http.SameSite) {
	c.sameSite = mode
}

func (c *testRouterContext) Error(error) {}

func (c *testRouterContext) IsAborted() bool { return c.aborted }

func (c *testRouterContext) Written() bool { return c.payload != nil }

func (c *testRouterContext) GetStatus() int { return c.status }

func (c *testRouterContext) Next() {}

func (c *testRouterContext) Abort() {
	c.aborted = true
}

func (c *testRouterContext) AbortWithStatus(code int) {
	c.aborted = true
	c.status = code
}

func (c *testRouterContext) AbortWithStatusJSON(code int, obj any) {
	c.aborted = true
	c.JSON(code, obj)
}

func (c *testRouterContext) Request() *http.Request { return c.req }

func (c *testRouterContext) ResponseWriter() http.ResponseWriter { return nil }
