// Package credentials exposes the installed credential keyring to extensions.
// The purpose is authenticated with the ciphertext and must be stable and unique.
package credentials

import (
	"cnb.cool/mliev/iam/mliev-iam/internal/pkg/credentialcrypto"
	"encoding/json"
	"errors"
)

func Seal(purpose string, plaintext []byte) (string, error) {
	if purpose == "" {
		return "", errors.New("credential purpose is required")
	}
	c, err := credentialcrypto.FromEnvironment()
	if err != nil {
		return "", err
	}
	e, err := c.Encrypt(plaintext, []byte("extension:"+purpose))
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(e)
	return string(data), err
}
func Open(purpose, ciphertext string) ([]byte, error) {
	if purpose == "" {
		return nil, errors.New("credential purpose is required")
	}
	c, err := credentialcrypto.FromEnvironment()
	if err != nil {
		return nil, err
	}
	var e credentialcrypto.Envelope
	if err := json.Unmarshal([]byte(ciphertext), &e); err != nil {
		return nil, errors.New("invalid credential envelope")
	}
	return c.Decrypt(e, []byte("extension:"+purpose))
}
