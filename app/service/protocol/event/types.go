package event

import (
	"context"
	"net/http"
)

type DirectoryEvent struct {
	ProviderEventID  string            `json:"providerEventId"`
	EventType        string            `json:"eventType"`
	ObjectType       string            `json:"objectType"`
	ExternalObjectID string            `json:"externalObjectId"`
	Challenge        string            `json:"challenge,omitempty"`
	Payload          map[string]any    `json:"payload"`
	CallbackResponse *CallbackResponse `json:"-"`
}

type CallbackResponse struct {
	Body        []byte
	ContentType string
}

type Adapter interface {
	VerifyAndParse(ctx context.Context, headers http.Header, body []byte) (*DirectoryEvent, error)
}
