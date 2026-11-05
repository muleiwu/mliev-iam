package captcha

import (
	"encoding/json"
	"github.com/alicebob/miniredis/v2"
	"sync"
	"sync/atomic"
	"testing"

	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	sqlitedriver "cnb.cool/mliev/open/go-web/pkg/server/database/sqlite"
	"github.com/redis/go-redis/v9"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	_ "modernc.org/sqlite"
)

func newCaptchaManagerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlitedriver.Open(t.TempDir()+"/captcha.db"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite database: %v", err)
	}
	if err := db.AutoMigrate(&model.Setting{}, &model.CaptchaConfig{}); err != nil {
		t.Fatalf("migrate captcha tables: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql database: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

func createCaptchaSceneBinding(t *testing.T, db *gorm.DB, configID uint) {
	t.Helper()

	bindings := dto.CaptchaSceneBindings{Login: configID}
	raw, err := json.Marshal(bindings)
	if err != nil {
		t.Fatalf("marshal captcha bindings: %v", err)
	}

	setting := &model.Setting{
		Key:      service.CaptchaSceneBindingsKey,
		Value:    string(raw),
		Category: service.CaptchaCategory,
		Remark:   "验证码场景绑定配置",
	}
	if err := db.Create(setting).Error; err != nil {
		t.Fatalf("create captcha binding setting: %v", err)
	}
}

func TestCaptchaManagerValidateForLoginSceneDefaultsToPass(t *testing.T) {
	db := newCaptchaManagerTestDB(t)

	manager := NewManager(db)
	if err := manager.ValidateForScene("login", nil, "127.0.0.1", nil); err != nil {
		t.Fatalf("expected login captcha to pass when unbound, got %v", err)
	}
}

func TestCaptchaManagerValidateForLoginSceneRequiresTicketWhenEnabled(t *testing.T) {
	db := newCaptchaManagerTestDB(t)
	config := &model.CaptchaConfig{
		Name:     "login captcha",
		Provider: TencentProviderType,
		Enabled:  true,
		Params: datatypes.JSONMap{
			"secretId":     "secret-id",
			"secretKey":    "secret-key",
			"captchaAppId": "123456",
			"appSecretKey": "app-secret-key",
		},
	}
	if err := db.Create(config).Error; err != nil {
		t.Fatalf("create captcha config: %v", err)
	}
	createCaptchaSceneBinding(t, db, config.ID)

	manager := NewManager(db)
	if err := manager.ValidateForScene("login", nil, "127.0.0.1", nil); err == nil || err.Error() != "请完成验证码验证" {
		t.Fatalf("expected missing ticket error, got %v", err)
	}
}

func TestCaptchaManagerValidateForLoginScenePassesWhenBoundConfigDisabled(t *testing.T) {
	db := newCaptchaManagerTestDB(t)
	config := &model.CaptchaConfig{
		Name:     "disabled login captcha",
		Provider: TencentProviderType,
		Enabled:  true,
		Params:   datatypes.JSONMap{},
	}
	if err := db.Create(config).Error; err != nil {
		t.Fatalf("create captcha config: %v", err)
	}
	if err := db.Model(config).Update("enabled", false).Error; err != nil {
		t.Fatalf("disable captcha config: %v", err)
	}
	createCaptchaSceneBinding(t, db, config.ID)

	manager := NewManager(db)
	if err := manager.ValidateForScene("login", nil, "127.0.0.1", nil); err != nil {
		t.Fatalf("expected disabled login captcha to pass, got %v", err)
	}
}

func TestCaptchaManagerRejectsTicketWithoutSessionWhenEnabled(t *testing.T) {
	db := newCaptchaManagerTestDB(t)
	config := &model.CaptchaConfig{
		Name:     "login captcha",
		Provider: TencentProviderType,
		Enabled:  true,
		Params: datatypes.JSONMap{
			"secretId":     "secret-id",
			"secretKey":    "secret-key",
			"captchaAppId": "123456",
			"appSecretKey": "app-secret-key",
		},
	}
	if err := db.Create(config).Error; err != nil {
		t.Fatalf("create captcha config: %v", err)
	}
	createCaptchaSceneBinding(t, db, config.ID)

	manager := NewManager(db)
	ticket := &dto.CaptchaTicket{
		Provider: TencentProviderType,
		Data: map[string]string{
			"ticket":      "forged-ticket",
			"randstr":     "forged-randstr",
			"fingerprint": "forged-fingerprint",
		},
	}

	// 省略 sessionUuid 或会话服务缺失都必须拒绝，否则可绕过 /api/captcha/params。
	if err := manager.ValidateForScene("login", ticket, "127.0.0.1", nil); err == nil {
		t.Fatal("expected rejection when the captcha session service is unavailable")
	}

	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	defer client.Close()
	if err := manager.ValidateForScene("login", ticket, "127.0.0.1", client); err == nil {
		t.Fatal("expected rejection when ticket has no sessionUuid")
	}
}

// 同一会话只允许消费一次；并发下也只能有一次成功。
func TestCaptchaSessionConsumeIsAtomic(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer client.Close()
	service := NewCaptchaSessionService(client)

	id, err := service.CreateSession(&dto.CaptchaParamsRequest{Scene: "login", Fingerprint: "fp-1"}, "127.0.0.1")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	var wg sync.WaitGroup
	var success int32
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := service.ConsumeSession(id, "login", "fp-1"); err == nil {
				atomic.AddInt32(&success, 1)
			}
		}()
	}
	wg.Wait()

	if success != 1 {
		t.Fatalf("expected exactly one successful consume, got %d", success)
	}
}

// 会话必须绑定场景，低风险场景的会话不能用于高风险场景。
func TestCaptchaSessionRejectsSceneMismatch(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer client.Close()
	service := NewCaptchaSessionService(client)

	id, err := service.CreateSession(&dto.CaptchaParamsRequest{Scene: "sendEmail", Fingerprint: "fp-1"}, "127.0.0.1")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	if _, err := service.ConsumeSession(id, "login", "fp-1"); err == nil {
		t.Fatal("expected scene mismatch to be rejected")
	}

	// 语义锚点：场景不匹配不消费会话，用户可用正确场景重试。
	if _, err := service.GetSession(id); err != nil {
		t.Fatalf("expected session to survive scene mismatch, got %v", err)
	}
	if _, err := service.ConsumeSession(id, "sendEmail", "fp-1"); err != nil {
		t.Fatalf("expected retry with the right scene to succeed, got %v", err)
	}
}

func TestCaptchaSessionRejectsFingerprintMismatch(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer client.Close()
	service := NewCaptchaSessionService(client)

	id, err := service.CreateSession(&dto.CaptchaParamsRequest{Scene: "login", Fingerprint: "fp-1"}, "127.0.0.1")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	if _, err := service.ConsumeSession(id, "login", "fp-other"); err == nil {
		t.Fatal("expected fingerprint mismatch to be rejected")
	}

	// 语义锚点：指纹不匹配不消费会话。
	if _, err := service.GetSession(id); err != nil {
		t.Fatalf("expected session to survive fingerprint mismatch, got %v", err)
	}
	if _, err := service.ConsumeSession(id, "login", "fp-1"); err != nil {
		t.Fatalf("expected retry with the right fingerprint to succeed, got %v", err)
	}
}

// 验证码交互失败（提供商返回 fail 或异常）时不能烧掉会话：
// 评审指出此前先 ConsumeSession 再 Verify，导致一次可重试的失败变成必须刷新页面。
func TestCaptchaValidateKeepsSessionWhenProviderFails(t *testing.T) {
	db := newCaptchaManagerTestDB(t)
	config := &model.CaptchaConfig{
		Name:     "login captcha",
		Provider: "stub",
		Enabled:  true,
		Params:   datatypes.JSONMap{},
	}
	if err := db.Create(config).Error; err != nil {
		t.Fatalf("create captcha config: %v", err)
	}
	createCaptchaSceneBinding(t, db, config.ID)

	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer client.Close()
	sessions := NewCaptchaSessionService(client)

	manager := NewManager(db)
	provider := &stubCaptchaProvider{success: false, message: "验证失败，请重新验证"}
	manager.RegisterProvider(provider)

	id, err := sessions.CreateSession(&dto.CaptchaParamsRequest{Scene: "login", Fingerprint: "fp-1"}, "127.0.0.1")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	ticket := &dto.CaptchaTicket{
		Provider:    "stub",
		SessionUuid: id,
		Data:        map[string]string{"fingerprint": "fp-1"},
	}

	if err := manager.ValidateForScene("login", ticket, "127.0.0.1", client); err == nil {
		t.Fatal("expected provider failure to reject the request")
	}
	// 失败后会话仍在，用户可以在同一会话内重试。
	if _, err := sessions.GetSession(id); err != nil {
		t.Fatalf("expected session to survive provider failure, got %v", err)
	}

	// 重试成功时只消费一次。
	provider.success = true
	if err := manager.ValidateForScene("login", ticket, "127.0.0.1", client); err != nil {
		t.Fatalf("expected retry to succeed, got %v", err)
	}
	if _, err := sessions.GetSession(id); err == nil {
		t.Fatal("expected session to be consumed after a successful validation")
	}
	// 同一会话不能重放。
	if err := manager.ValidateForScene("login", ticket, "127.0.0.1", client); err == nil {
		t.Fatal("expected replayed session to be rejected")
	}
}

// stubCaptchaProvider 用于在测试中控制二次校验结果，避免依赖外部网络。
type stubCaptchaProvider struct {
	success bool
	message string
	calls   int
}

func (*stubCaptchaProvider) GetProviderType() string { return "stub" }

func (*stubCaptchaProvider) GetProviderInfo() dto.CaptchaProviderInfo {
	return dto.CaptchaProviderInfo{Provider: "stub", Label: "stub"}
}

func (*stubCaptchaProvider) GetClientParams(*model.CaptchaConfig) map[string]interface{} {
	return map[string]interface{}{}
}

func (p *stubCaptchaProvider) Verify(*model.CaptchaConfig, map[string]string, string) (*VerifyResult, error) {
	p.calls++
	return &VerifyResult{Success: p.success, Message: p.message}, nil
}
