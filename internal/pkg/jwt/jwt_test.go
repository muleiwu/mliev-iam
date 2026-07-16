package jwt

import "testing"

func newTestService() *JWTService {
	return NewJWTService("test-secret-key-0123456789abcdef0123456789abcdef", "mliev-iam-test")
}

// TestGenerateTokenRejectsEmptyRole 确认空 role 被拒绝（历史上会静默回退为 admin，导致提权）。
func TestGenerateTokenRejectsEmptyRole(t *testing.T) {
	svc := newTestService()
	if _, err := svc.GenerateToken(TokenSubject{UserID: 1, Username: "u"}, 1); err == nil {
		t.Fatal("空 role 必须返回错误，否则会静默回退为 admin 造成提权")
	}
}

// TestGenerateTokenPreservesExplicitRole 确认显式 role 被原样写入并可解析。
func TestGenerateTokenPreservesExplicitRole(t *testing.T) {
	svc := newTestService()
	for _, role := range []string{"user", "admin"} {
		token, err := svc.GenerateToken(TokenSubject{UserID: 1, Username: "u", Role: role}, 1)
		if err != nil {
			t.Fatalf("生成 role=%s 的 token 失败: %v", role, err)
		}
		claims, err := svc.ValidateToken(token)
		if err != nil {
			t.Fatalf("校验 role=%s 的 token 失败: %v", role, err)
		}
		if claims.Role != role {
			t.Fatalf("role 不一致：期望 %q，得到 %q", role, claims.Role)
		}
	}
}

// TestRefreshTokenPreservesRole 确认刷新保留原 role（不经过签发回退逻辑）。
func TestRefreshTokenPreservesRole(t *testing.T) {
	svc := newTestService()
	token, err := svc.GenerateToken(TokenSubject{UserID: 1, Username: "u", Role: "user"}, 1)
	if err != nil {
		t.Fatalf("生成 token 失败: %v", err)
	}
	refreshed, err := svc.RefreshToken(token, 1)
	if err != nil {
		t.Fatalf("刷新 token 失败: %v", err)
	}
	claims, err := svc.ValidateToken(refreshed)
	if err != nil {
		t.Fatalf("校验刷新后的 token 失败: %v", err)
	}
	if claims.Role != "user" {
		t.Fatalf("刷新后 role 被篡改：期望 user，得到 %q", claims.Role)
	}
}
