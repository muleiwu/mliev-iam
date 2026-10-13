package dto

// PushTemplateBinding maps a remote template variable to an IAM value.
type PushTemplateBinding struct {
	Source string `json:"source"`
	Value  string `json:"value,omitempty"`
}

type PushCredentials struct {
	BaseURL   string `json:"baseUrl"`
	AppID     string `json:"appId"`
	AppSecret string `json:"appSecret"`
}

func (p PushSettings) Credentials() PushCredentials {
	return PushCredentials{BaseURL: p.BaseURL, AppID: p.AppID, AppSecret: p.AppSecret}
}

type PushChannelsQueryRequest struct {
	PushCredentials
	Type     string `json:"type"`
	Page     int    `json:"page"`
	PageSize int    `json:"pageSize"`
}

type PushChannelReadiness struct {
	State        string   `json:"state"`
	BlockerCodes []string `json:"blockerCodes"`
}

type PushChannel struct {
	ID                int                  `json:"id"`
	Name              string               `json:"name"`
	Type              string               `json:"type"`
	MessageTemplateID int                  `json:"messageTemplateId"`
	TemplateName      string               `json:"templateName"`
	Readiness         PushChannelReadiness `json:"readiness"`
}

type PushChannelsPage struct {
	Items []PushChannel `json:"items"`
	Total int64         `json:"total"`
	Page  int           `json:"page"`
	Size  int           `json:"size"`
}

type PushChannelTemplate struct {
	ID           int      `json:"id"`
	TemplateName string   `json:"templateName"`
	ContentType  string   `json:"contentType"`
	Content      string   `json:"content"`
	Variables    []string `json:"variables"` // null is invalid; [] means no variables.
	Description  string   `json:"description"`
}

type PushChannelDetail struct {
	PushChannel
	Template          *PushChannelTemplate `json:"template"`
	SignatureRequired bool                 `json:"signatureRequired"`
	SignatureNames    []string             `json:"signatureNames"`
}

type PushTestRequest struct {
	Config   PushSettings `json:"config"`
	Type     string       `json:"type"`
	Receiver string       `json:"receiver"`
}

type PushTestResponse struct {
	TaskID    string `json:"taskId"`
	Status    string `json:"status"`
	CreatedAt string `json:"createdAt"`
}
