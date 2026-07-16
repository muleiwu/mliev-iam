package dto

// CreateMigrationSourceRequest 创建迁移源请求。
type CreateMigrationSourceRequest struct {
	Name             string                 `json:"name" binding:"required"`
	Type             string                 `json:"type" binding:"required,oneof=api database"`
	Enabled          *bool                  `json:"enabled" binding:"omitempty"`
	Sort             int                    `json:"sort" binding:"omitempty"`
	ConnectionConfig map[string]interface{} `json:"connectionConfig" binding:"omitempty"`
	LoginScript      string                 `json:"loginScript" binding:"omitempty"`
	TransformScript  string                 `json:"transformScript" binding:"omitempty"`
	Remark           string                 `json:"remark" binding:"omitempty"`
}

// UpdateMigrationSourceRequest 更新迁移源请求。指针/可空字段区分"未提供"。
type UpdateMigrationSourceRequest struct {
	Name             string                 `json:"name" binding:"omitempty"`
	Enabled          *bool                  `json:"enabled" binding:"omitempty"`
	Sort             *int                   `json:"sort" binding:"omitempty"`
	ConnectionConfig map[string]interface{} `json:"connectionConfig" binding:"omitempty"`
	LoginScript      *string                `json:"loginScript" binding:"omitempty"`
	TransformScript  *string                `json:"transformScript" binding:"omitempty"`
	Remark           *string                `json:"remark" binding:"omitempty"`
}

// TestScriptRequest 脚本试运行请求。
type TestScriptRequest struct {
	// Which 指定试运行哪个脚本：login 或 transform。
	Which  string                 `json:"which" binding:"required,oneof=login transform"`
	Script string                 `json:"script" binding:"required"`
	Input  map[string]interface{} `json:"input" binding:"omitempty"`
}

// TestConnectionRequest 旧库连通性测试请求。
type TestConnectionRequest struct {
	ConnectionConfig map[string]interface{} `json:"connectionConfig" binding:"required"`
}
