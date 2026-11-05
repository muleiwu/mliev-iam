package captcha

import (
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newProofTestSession(t *testing.T) (*CaptchaSessionService, *miniredis.Miniredis) {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return NewCaptchaSessionService(client), server
}

// 未知 provider 也必须拿到非空摘要，否则 SetNX 会被跳过，凭证可跨会话重放。
func TestProofKeyIsNeverEmptyForUnknownProvider(t *testing.T) {
	key := ProofKey("third-party", map[string]string{
		"token":       "token-a",
		"fingerprint": "fp-1",
	})
	if key == "" {
		t.Fatal("expected a non-empty proof key for an unknown provider")
	}
}

// 同一份凭证换一个指纹仍必须得到同一个键，否则攻击者改指纹即可重放。
func TestProofKeyIgnoresFingerprintForUnknownProvider(t *testing.T) {
	a := ProofKey("third-party", map[string]string{"token": "token-a", "fingerprint": "fp-1"})
	b := ProofKey("third-party", map[string]string{"token": "token-a", "fingerprint": "fp-2"})
	if a != b {
		t.Fatalf("expected fingerprint-independent key, got %s and %s", a, b)
	}

	c := ProofKey("third-party", map[string]string{"token": "token-b", "fingerprint": "fp-1"})
	if a == c {
		t.Fatal("expected different credentials to produce different keys")
	}
}

// 内置提供商沿用官方字段，顺序固定，不受 map 遍历顺序影响。
func TestProofKeyUsesOfficialFields(t *testing.T) {
	geetest := ProofKey(GeetestProviderType, map[string]string{
		"lot_number":     "lot-1",
		"captcha_output": "output-1",
		"pass_token":     "pass-1",
		"gen_time":       "1700000000",
		"fingerprint":    "fp-1",
	})
	reordered := ProofKey(GeetestProviderType, map[string]string{
		"fingerprint":    "fp-2",
		"pass_token":     "pass-1",
		"gen_time":       "1700000001",
		"captcha_output": "output-1",
		"lot_number":     "lot-1",
	})
	if geetest != reordered {
		t.Fatal("expected the same key for the same proof regardless of extra fields")
	}

	tencent := ProofKey(TencentProviderType, map[string]string{"ticket": "ticket-1", "randstr": "rand-1"})
	if tencent == geetest {
		t.Fatal("expected different providers to produce different keys")
	}
}

// 已提交为 used 的凭证不能在失败分支里被释放，否则「先成功再失败」即可复活。
func TestProofCommitThenReleaseKeepsProofUsed(t *testing.T) {
	sessions, server := newProofTestSession(t)
	data := map[string]string{"lot_number": "lot-1", "captcha_output": "o", "pass_token": "p"}

	key, acquired, err := sessions.BeginProof(GeetestProviderType, data)
	if err != nil {
		t.Fatalf("begin proof: %v", err)
	}
	if !acquired {
		t.Fatal("expected the first begin to acquire the proof")
	}

	if err := sessions.CommitProof(key); err != nil {
		t.Fatalf("commit proof: %v", err)
	}
	got, err := server.Get(key)
	if err != nil {
		t.Fatalf("read proof state: %v", err)
	}
	if got != proofStateUsed {
		t.Fatalf("expected proof state %q, got %q", proofStateUsed, got)
	}

	sessions.ReleaseProof(key)
	got, err = server.Get(key)
	if err != nil {
		t.Fatalf("expected used proof to survive release, got err %v", err)
	}
	if got != proofStateUsed {
		t.Fatalf("expected used proof to survive release, got %q", got)
	}

	// 即使释放过，同一份凭证仍然不能再被抢占。
	if _, acquired, err := sessions.BeginProof(GeetestProviderType, data); err != nil || acquired {
		t.Fatalf("expected used proof to stay reserved, got acquired=%v err=%v", acquired, err)
	}
}

// 未提交的凭证在校验失败后应被释放，允许用户重试。
func TestProofReleaseClearsPendingProof(t *testing.T) {
	sessions, server := newProofTestSession(t)
	data := map[string]string{"lot_number": "lot-2", "captcha_output": "o", "pass_token": "p"}

	key, acquired, err := sessions.BeginProof(GeetestProviderType, data)
	if err != nil || !acquired {
		t.Fatalf("begin proof: acquired=%v err=%v", acquired, err)
	}
	if got, err := server.Get(key); err != nil || got != proofStatePending {
		t.Fatalf("expected pending proof, got %q (%v)", got, err)
	}

	sessions.ReleaseProof(key)
	if server.Exists(key) {
		t.Fatal("expected pending proof to be released")
	}

	if _, acquired, err := sessions.BeginProof(GeetestProviderType, data); err != nil || !acquired {
		t.Fatalf("expected released proof to be acquirable again, got acquired=%v err=%v", acquired, err)
	}
}
