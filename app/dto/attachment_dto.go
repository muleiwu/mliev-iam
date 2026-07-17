package dto

// UploadAvatarResponse 头像上传响应。URL 为 /attachments/{uuid} 相对路径，
// 访问时由后端 302 跳转到真实存储地址。
type UploadAvatarResponse struct {
	UUID     string `json:"uuid"`
	URL      string `json:"url"`
	FileName string `json:"fileName"`
	FileSize int64  `json:"fileSize"`
	MimeType string `json:"mimeType"`
}
