package controller

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"github.com/stretchr/testify/require"
)

// Opt-in because Go-only environments do not require an installed browser.
// OIDC_BROWSER_PYTHON must contain Playwright; OIDC_BROWSER_EXECUTABLE is optional.
func TestOIDCCORSBrowserFlow(t *testing.T) {
	python := os.Getenv("OIDC_BROWSER_PYTHON")
	if python == "" {
		t.Skip("set OIDC_BROWSER_PYTHON to run the real cross-origin browser flow")
	}
	page := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<!doctype html><title>OIDC CORS test RP</title><main>OIDC test client</main>"))
	})
	rp := httptest.NewServer(page)
	t.Cleanup(rp.Close)
	foreign := httptest.NewServer(page)
	t.Cleanup(foreign.Close)
	f := newCORSHTTPFixture(t, []string{rp.URL}, true)
	iam := httptest.NewServer(f.engine)
	t.Cleanup(iam.Close)
	verifier := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNO123456"
	hash := sha256.Sum256([]byte(verifier))
	code := &model.AuthorizationCode{Code: "browser-fixture-code", ClientID: f.apps[0].UUID, UserID: f.user.ID, LoginSessionID: f.session.ID, RedirectURI: rp.URL + "/callback", Scopes: []string{"openid", "profile"}, CodeChallenge: base64.RawURLEncoding.EncodeToString(hash[:]), CodeChallengeMethod: "S256", ExpiresAt: time.Now().Add(time.Minute)}
	require.NoError(t, f.db.Create(code).Error)
	fixture, err := json.Marshal(map[string]string{"rp_url": rp.URL, "foreign_url": foreign.URL, "issuer": iam.URL + "/oidc/" + f.apps[0].UUID, "client_id": f.apps[0].UUID, "redirect_uri": code.RedirectURI, "authorization_code": code.Code, "code_verifier": verifier})
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, python, filepath.Join("..", "..", "scripts", "check_oidc_cors_browser.py"))
	command.Stdin = bytes.NewReader(fixture)
	output, err := command.CombinedOutput()
	require.NoError(t, err, "%s", output)
	t.Log(string(output))
	verify := exec.CommandContext(ctx, python, filepath.Join("..", "..", "scripts", "check_oidc_cors.py"), "--issuer", iam.URL+"/oidc/"+f.apps[0].UUID, "--origin", rp.URL)
	output, err = verify.CombinedOutput()
	require.NoError(t, err, "%s", output)
	t.Log(string(output))
}
