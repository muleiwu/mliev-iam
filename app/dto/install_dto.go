package dto

// InstallCheckResponse remains compatible with existing readiness consumers.
type InstallCheckResponse struct {
	Ready              bool   `json:"ready"`
	Installed          bool   `json:"installed"`
	DatabaseConnected  bool   `json:"databaseConnected"`
	DatabaseConfigured bool   `json:"databaseConfigured"`
	Message            string `json:"message,omitempty"`
	CurrentDatabase    string `json:"currentDatabase,omitempty"`
}

// AdminAccountInfo is transient initialization input, never application config.
type AdminAccountInfo struct {
	Username string `json:"username" binding:"required,min=3,max=50"`
	Password string `json:"password" binding:"required,min=12"`
	Email    string `json:"email" binding:"required,email"`
	RealName string `json:"realName" binding:"required,max=50"`
	Phone    string `json:"phone" binding:"omitempty,max=20"`
}
