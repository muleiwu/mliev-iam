package identity

import (
	"context"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
)

type User struct {
	ExternalUserID string
	UnionID        string
	TenantID       string
	Nickname       string
	Avatar         string
	Email          string
	Phone          string
	RawUserInfo    map[string]any
}

type Token struct {
	AccessToken  string
	RefreshToken string
	TokenType    string
	ExpiresIn    int64
	Expiry       time.Time
	Raw          map[string]any
}

type Adapter interface {
	GetProviderType() model.IdentityProviderType
	ValidateConfig() error
	GetAuthURL(state string) string
	ExchangeToken(ctx context.Context, code string) (*Token, error)
	GetUserInfo(ctx context.Context, token *Token) (*User, error)
}
