package dto

// InstallCheckResponse 安装检查响应
type InstallCheckResponse struct {
	Ready              bool   `json:"ready"`
	Installed          bool   `json:"installed"`                 // 是否已安装
	DatabaseConnected  bool   `json:"databaseConnected"`         // 数据库是否连接
	DatabaseConfigured bool   `json:"databaseConfigured"`        // 数据库是否已配置
	Message            string `json:"message,omitempty"`         // 提示信息
	CurrentDatabase    string `json:"currentDatabase,omitempty"` // 当前数据库名称
}

// InstallSubmitRequest 安装提交请求
type InstallSubmitRequest struct {
	Database DatabaseConfig   `json:"database"`                 // 数据库配置
	Redis    RedisConfig      `json:"redis" binding:"required"` // Redis配置
	Admin    AdminAccountInfo `json:"admin" binding:"required"` // 管理员账户信息
}

// DatabaseConfig 数据库配置
type DatabaseConfig struct {
	Driver   string `json:"driver"`   // 数据库驱动类型：mysql、postgresql、sqlite
	FilePath string `json:"filePath"` // SQLite 数据库文件路径（仅sqlite时使用）
	Host     string `json:"host"`     // 数据库主机地址
	Port     int    `json:"port"`     // 数据库端口
	Username string `json:"username"` // 数据库用户名
	Password string `json:"password"` // 数据库密码
	Database string `json:"database"` // 数据库名称
	Charset  string `json:"charset"`  // 字符集，默认 utf8mb4
}

// AdminAccountInfo 管理员账户信息
type AdminAccountInfo struct {
	Username string `json:"username" binding:"required,min=3,max=50"` // 用户名
	Password string `json:"password" binding:"required,min=6"`        // 密码
	Email    string `json:"email" binding:"required,email"`           // 邮箱
	RealName string `json:"realName" binding:"required,max=50"`       // 真实姓名
	Phone    string `json:"phone" binding:"omitempty,max=20"`         // 手机号
}

// InstallSubmitResponse 安装提交响应
type InstallSubmitResponse struct {
	Success bool   `json:"success"` // 是否成功
	Message string `json:"message"` // 提示信息
}

// RedisConfig Redis配置
type RedisConfig struct {
	Host     string `json:"host"`     // Redis主机地址
	Port     int    `json:"port"`     // Redis端口
	Password string `json:"password"` // Redis密码
	DB       int    `json:"db"`       // Redis数据库编号
}
