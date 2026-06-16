package script

import (
	"context"
	"testing"
	"time"
)

func TestRunForMapReturnsObject(t *testing.T) {
	e := NewEngine(0)
	out, err := e.RunForMap(context.Background(), `
		return { success: input.password === "secret", user: { username: input.identifier } };
	`, map[string]any{"identifier": "alice", "password": "secret"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out["success"] != true {
		t.Fatalf("expected success=true, got %v", out["success"])
	}
	user, ok := out["user"].(map[string]any)
	if !ok || user["username"] != "alice" {
		t.Fatalf("expected user.username=alice, got %v", out["user"])
	}
}

func TestCryptoSha256(t *testing.T) {
	e := NewEngine(0)
	out, err := e.Run(context.Background(), `return crypto.sha256("abc");`, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// sha256("abc")
	const want = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	if out != want {
		t.Fatalf("sha256 mismatch: got %v", out)
	}
}

func TestTimeoutInterruptsInfiniteLoop(t *testing.T) {
	e := NewEngine(200 * time.Millisecond)
	start := time.Now()
	_, err := e.Run(context.Background(), `while (true) {}`, nil)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("timeout took too long: %v", elapsed)
	}
}
