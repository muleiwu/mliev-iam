package social

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"cnb.cool/mliev/iam/mliev-iam/app/service/protocol/oidcclient"
	jose "github.com/go-jose/go-jose/v4"
)

// CheckConnection checks the public signing keys without simulating a login or
// sending client credentials. It deliberately does not run in ValidateConfig.
func (a *OIDCAdapter) CheckConnection(ctx context.Context) error {
	var doc struct {
		Keys []json.RawMessage `json:"keys"`
	}
	if err := oidcclient.GetJSON(ctx, a.client, a.jwksURI, "JWKS", &doc); err != nil {
		return err
	}
	hasSigningKey := false
	for _, raw := range doc.Keys {
		var key jose.JSONWebKey
		var metadata struct {
			KeyOps    []string `json:"key_ops"`
			Algorithm string   `json:"alg"`
		}
		if json.Unmarshal(raw, &metadata) != nil {
			return fmt.Errorf("OIDC JWKS 公钥格式无效（HTTP 200），请检查提供方 keys 配置")
		}
		// Match go-oidc's handling of unrelated or unsupported keys in a mixed set.
		if metadata.Algorithm != "" && !supportedOIDCSigningAlgorithm(metadata.Algorithm) {
			continue
		}
		if err := json.Unmarshal(raw, &key); err != nil {
			if errors.Is(err, jose.ErrUnsupportedKeyType) {
				continue
			}
			return fmt.Errorf("OIDC JWKS 公钥格式无效（HTTP 200），请检查提供方 keys 配置")
		}
		if len(metadata.KeyOps) > 0 && !slices.Contains(metadata.KeyOps, "verify") {
			continue
		}
		if usableOIDCSigningKey(&key, a.signingAlgs) {
			hasSigningKey = true
		}
	}
	if hasSigningKey {
		return nil
	}
	return fmt.Errorf("OIDC JWKS 无可用签名公钥（HTTP 200），请确认 keys 包含与 ID Token 签名算法兼容的公开验签密钥")
}

func usableOIDCSigningKey(key *jose.JSONWebKey, algorithms []string) bool {
	if !key.Valid() || !key.IsPublic() || (key.Use != "" && key.Use != "sig") {
		return false
	}
	for _, algorithm := range algorithms {
		if key.Algorithm != "" && key.Algorithm != algorithm {
			continue
		}
		switch public := key.Key.(type) {
		case *rsa.PublicKey:
			if public.N.Sign() > 0 && public.E >= 3 && public.E%2 == 1 && (strings.HasPrefix(algorithm, "RS") || strings.HasPrefix(algorithm, "PS")) {
				return true
			}
		case *ecdsa.PublicKey:
			curveSize := public.Curve.Params().BitSize
			if public.Curve.IsOnCurve(public.X, public.Y) && ((algorithm == "ES256" && curveSize == 256) || (algorithm == "ES384" && curveSize == 384) || (algorithm == "ES512" && curveSize == 521)) {
				return true
			}
		case ed25519.PublicKey:
			if algorithm == "EdDSA" && len(public) == ed25519.PublicKeySize {
				return true
			}
		}
	}
	return false
}
