package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

const adminAuthTransactionPrefix = "admin-auth:transaction:"

type AdminAuthTransaction struct {
	State              string `json:"state"`
	Nonce              string `json:"nonce"`
	CodeVerifier       string `json:"codeVerifier"`
	CodeChallenge      string `json:"codeChallenge"`
	RedirectURI        string `json:"redirectUri"`
	ReturnTo           string `json:"returnTo"`
	BrowserBindingHash string `json:"browserBindingHash"`
	CreatedAt          int64  `json:"createdAt"`
}

type AdminAuthFlowService struct {
	store  adminAuthTransactionStore
	ttl    time.Duration
	prefix string
}

func NewAdminAuthFlowService(redisClient *redis.Client) *AdminAuthFlowService {
	var store adminAuthTransactionStore
	if redisClient != nil {
		store = redisAdminAuthTransactionStore{client: redisClient}
	}
	return &AdminAuthFlowService{store: store, ttl: 10 * time.Minute, prefix: adminAuthTransactionPrefix}
}

type adminAuthTransactionStore interface {
	Put(context.Context, string, []byte, time.Duration) error
	Get(context.Context, string) ([]byte, error)
	GetDel(context.Context, string) ([]byte, error)
}

type redisAdminAuthTransactionStore struct{ client *redis.Client }

func (s redisAdminAuthTransactionStore) Put(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	return s.client.Set(ctx, key, value, ttl).Err()
}

func (s redisAdminAuthTransactionStore) Get(ctx context.Context, key string) ([]byte, error) {
	return s.client.Get(ctx, key).Bytes()
}

func (s redisAdminAuthTransactionStore) GetDel(ctx context.Context, key string) ([]byte, error) {
	return s.client.GetDel(ctx, key).Bytes()
}

func randomAdminAuthValue(size int) (string, error) {
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func (s *AdminAuthFlowService) Start(ctx context.Context, redirectURI, returnTo string) (*AdminAuthTransaction, string, error) {
	if s.store == nil {
		return nil, "", errors.New("Redis 未初始化")
	}
	state, err := randomAdminAuthValue(32)
	if err != nil {
		return nil, "", fmt.Errorf("生成 state 失败: %w", err)
	}
	nonce, err := randomAdminAuthValue(32)
	if err != nil {
		return nil, "", fmt.Errorf("生成 nonce 失败: %w", err)
	}
	verifier, err := randomAdminAuthValue(32)
	if err != nil {
		return nil, "", fmt.Errorf("生成 PKCE verifier 失败: %w", err)
	}
	browserToken, err := randomAdminAuthValue(32)
	if err != nil {
		return nil, "", fmt.Errorf("生成浏览器绑定失败: %w", err)
	}
	challengeDigest := sha256.Sum256([]byte(verifier))
	browserDigest := sha256.Sum256([]byte(browserToken))
	transaction := &AdminAuthTransaction{
		State:              state,
		Nonce:              nonce,
		CodeVerifier:       verifier,
		CodeChallenge:      base64.RawURLEncoding.EncodeToString(challengeDigest[:]),
		RedirectURI:        redirectURI,
		ReturnTo:           returnTo,
		BrowserBindingHash: base64.RawURLEncoding.EncodeToString(browserDigest[:]),
		CreatedAt:          time.Now().Unix(),
	}
	payload, err := json.Marshal(transaction)
	if err != nil {
		return nil, "", fmt.Errorf("序列化后台认证事务失败: %w", err)
	}
	if err := s.store.Put(ctx, s.keyPrefix()+state, payload, s.ttl); err != nil {
		return nil, "", fmt.Errorf("保存后台认证事务失败: %w", err)
	}
	return transaction, browserToken, nil
}

func (s *AdminAuthFlowService) Consume(ctx context.Context, state, browserToken string) (*AdminAuthTransaction, error) {
	if s.store == nil || state == "" || len(state) > 128 || browserToken == "" {
		return nil, errors.New("后台认证事务无效")
	}
	key := s.keyPrefix() + state
	payload, err := s.store.Get(ctx, key)
	if err != nil {
		return nil, errors.New("后台认证事务不存在或已过期")
	}
	var preview AdminAuthTransaction
	if err := json.Unmarshal(payload, &preview); err != nil {
		return nil, errors.New("后台认证事务数据无效")
	}
	if !validAdminBrowserBinding(preview.BrowserBindingHash, browserToken) {
		return nil, errors.New("后台认证事务与当前浏览器不匹配")
	}

	consumed, err := s.store.GetDel(ctx, key)
	if err != nil {
		return nil, errors.New("后台认证事务已使用或已过期")
	}
	var transaction AdminAuthTransaction
	if err := json.Unmarshal(consumed, &transaction); err != nil || transaction.State != state {
		return nil, errors.New("后台认证事务数据无效")
	}
	if !validAdminBrowserBinding(transaction.BrowserBindingHash, browserToken) {
		return nil, errors.New("后台认证事务与当前浏览器不匹配")
	}
	return &transaction, nil
}

func validAdminBrowserBinding(expectedHash, token string) bool {
	digest := sha256.Sum256([]byte(token))
	actual := base64.RawURLEncoding.EncodeToString(digest[:])
	return expectedHash != "" && subtle.ConstantTimeCompare([]byte(expectedHash), []byte(actual)) == 1
}

// IsSuperAdmin verifies the same role invariant used to mint administrator JWTs.
func IsSuperAdmin(db *gorm.DB, userID uint) (bool, error) {
	role, err := dao.NewRoleDao(db).GetByCode("super_admin")
	if err != nil {
		return false, err
	}
	if !role.IsSystem() || !role.IsEnabled() {
		return false, nil
	}
	assignment, err := dao.NewUserRoleDao(db).GetByUserIDAndRoleID(userID, role.ID)
	if err != nil {
		return false, nil
	}
	return assignment.IsValid(), nil
}

// NewFirstPartyAuthFlowService isolates transactions for a compiled client.
func NewFirstPartyAuthFlowService(client *redis.Client, namespace string) *AdminAuthFlowService {
	if namespace == "" || namespace == "admin" {
		panic("extension auth namespace must be distinct")
	}
	s := NewAdminAuthFlowService(client)
	s.prefix = "first-party:" + namespace + ":transaction:"
	return s
}
func (s *AdminAuthFlowService) keyPrefix() string {
	if s.prefix == "" {
		return adminAuthTransactionPrefix
	}
	return s.prefix
}
