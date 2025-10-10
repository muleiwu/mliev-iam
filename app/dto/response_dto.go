package dto

// Response 统一响应结构
type Response struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// BaseQueryRequest 基础查询请求结构
type BaseQueryRequest struct {
	Page     int    `form:"page" json:"page" validate:"min=1" binding:"omitempty,min=1"`
	PageSize int    `form:"pageSize" json:"pageSize" validate:"min=1,max=100" binding:"omitempty,min=1,max=100"`
	OrderBy  string `form:"orderBy" json:"orderBy" binding:"omitempty"`
	OrderDir string `form:"orderDir" json:"orderDir" validate:"omitempty,oneof=asc desc" binding:"omitempty,oneof=asc desc"`
}

// PaginatedResponse 分页响应结构
type PaginatedResponse struct {
	Total int         `json:"total"`
	Page  int         `json:"page"`
	Size  int         `json:"size"`
	Items interface{} `json:"items"`
}
