package identity

import (
	"context"
	"strings"
	"time"
)

type ProviderType string

const (
	ProviderTypeFeishu   ProviderType = "feishu"
	ProviderTypeWeCom    ProviderType = "wecom"
	ProviderTypeDingTalk ProviderType = "dingtalk"
)

// Provider is a transport-only adapter configuration. Legacy provider rows
// are read only by the versioned migration package.
type Provider struct {
	ID           uint
	Name         string
	Identifier   string
	Type         ProviderType
	Description  string
	ClientID     string
	ClientSecret string
	AuthURL      string
	TokenURL     string
	UserInfoURL  string
	RedirectURL  string
	Scopes       string
	Icon         string
	SortOrder    int
	IsDefault    bool
	Attributes   map[string]any
	Remark       string
}

func (p *Provider) GetScopes() []string {
	if p == nil || strings.TrimSpace(p.Scopes) == "" {
		return nil
	}
	parts := strings.Split(p.Scopes, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if value := strings.TrimSpace(part); value != "" {
			result = append(result, value)
		}
	}
	return result
}

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
	GetProviderType() ProviderType
	ValidateConfig() error
	GetAuthURL(state string) string
	ExchangeToken(ctx context.Context, code string) (*Token, error)
	GetUserInfo(ctx context.Context, token *Token) (*User, error)
}
