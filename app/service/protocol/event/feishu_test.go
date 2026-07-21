package event

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFeishuEventVerifiesEncryptedPayload(t *testing.T) {
	adapter, err := NewFeishuAdapter(map[string]any{"verificationToken": "verify-token", "encryptKey": "encrypt-key"})
	require.NoError(t, err)
	payload := map[string]any{
		"header": map[string]any{"event_id": "evt-1", "event_type": "contact.user.updated_v3", "token": "verify-token"},
		"event":  map[string]any{"user": map[string]any{"user_id": "u-1"}},
	}
	plain, _ := json.Marshal(payload)
	encrypted := encryptFeishuTestPayload(t, plain, "encrypt-key")
	body, _ := json.Marshal(map[string]string{"encrypt": encrypted})
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	nonce := "nonce-1"
	digest := sha256.Sum256([]byte(timestamp + nonce + "encrypt-key" + string(body)))
	headers := http.Header{
		"X-Lark-Request-Timestamp": []string{timestamp},
		"X-Lark-Request-Nonce":     []string{nonce},
		"X-Lark-Signature":         []string{hex.EncodeToString(digest[:])},
	}
	event, err := adapter.VerifyAndParse(context.Background(), headers, body)
	require.NoError(t, err)
	require.Equal(t, "evt-1", event.ProviderEventID)
	require.Equal(t, "user", event.ObjectType)
	require.Equal(t, "u-1", event.ExternalObjectID)

	headers.Set("X-Lark-Signature", "bad-signature")
	_, err = adapter.VerifyAndParse(context.Background(), headers, body)
	require.ErrorContains(t, err, "签名无效")
}

func TestFeishuEventChallengeAndToken(t *testing.T) {
	adapter, err := NewFeishuAdapter(map[string]any{"verificationToken": "verify-token"})
	require.NoError(t, err)
	body := []byte(`{"token":"verify-token","type":"url_verification","challenge":"challenge-value"}`)
	event, err := adapter.VerifyAndParse(context.Background(), nil, body)
	require.NoError(t, err)
	require.Equal(t, "challenge-value", event.Challenge)
}

func encryptFeishuTestPayload(t *testing.T, plain []byte, encryptKey string) string {
	t.Helper()
	key := sha256.Sum256([]byte(encryptKey))
	padding := aes.BlockSize - len(plain)%aes.BlockSize
	padded := append(bytes.Clone(plain), bytes.Repeat([]byte{byte(padding)}, padding)...)
	block, err := aes.NewCipher(key[:])
	require.NoError(t, err)
	ciphertext := make([]byte, len(padded))
	previous := key[:aes.BlockSize]
	for offset := 0; offset < len(padded); offset += aes.BlockSize {
		for index := 0; index < aes.BlockSize; index++ {
			ciphertext[offset+index] = padded[offset+index] ^ previous[index]
		}
		block.Encrypt(ciphertext[offset:offset+aes.BlockSize], ciphertext[offset:offset+aes.BlockSize])
		previous = ciphertext[offset : offset+aes.BlockSize]
	}
	return base64.StdEncoding.EncodeToString(ciphertext)
}
