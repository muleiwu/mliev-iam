package controller

import (
	"net/http"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/internal/interfaces"
	"github.com/gin-gonic/gin"
)

type IndexController struct {
	BaseResponse
}

// IndexData 首页数据结构
type IndexData struct {
	Title      string
	Version    string
	Now        time.Time
	Features   []Feature
	Security   []SecurityItem
	Scenarios  []Scenario
	Advantages []Advantage
}

// Feature 功能特性
type Feature struct {
	Icon        string
	Title       string
	Description string
}

// SecurityItem 安全特性
type SecurityItem struct {
	Icon        string
	Title       string
	Description string
}

// Scenario 应用场景
type Scenario struct {
	Icon        string
	Title       string
	Description string
	Features    []string
}

// Advantage 技术优势
type Advantage struct {
	Icon        string
	Title       string
	Description string
}

func (receiver IndexController) GetIndex(c *gin.Context, helper interfaces.HelperInterface) {
	helper.GetLogger().Info("访问首页")

	// 准备首页数据
	data := IndexData{
		Title:   "企业级身份认证管理系统",
		Version: "1.0.0",
		Now:     time.Now(),
		Features: []Feature{
			{
				Icon:        "fas fa-key",
				Title:       "OIDC 认证",
				Description: "完整支持 OpenID Connect 协议，提供标准化的身份认证和用户信息获取",
			},
			{
				Icon:        "fas fa-lock",
				Title:       "OAuth2 授权",
				Description: "支持 OAuth2.0 授权框架，实现安全的 API 访问控制和权限管理",
			},
			{
				Icon:        "fas fa-certificate",
				Title:       "SAML 集成",
				Description: "兼容 SAML 2.0 标准，支持企业级单点登录和身份联邦",
			},
			{
				Icon:        "fas fa-users",
				Title:       "社交登录",
				Description: "集成主流社交平台，支持微信、QQ、GitHub 等第三方登录",
			},
		},
		Security: []SecurityItem{
			{
				Icon:        "fas fa-shield-alt",
				Title:       "多层安全防护",
				Description: "采用多层次安全架构，从网络传输到数据存储全程加密保护",
			},
			{
				Icon:        "fas fa-certificate",
				Title:       "合规性认证",
				Description: "符合 ISO 27001、GDPR 等国际安全标准和隐私保护法规",
			},
			{
				Icon:        "fas fa-fingerprint",
				Title:       "多因素认证",
				Description: "支持短信、邮箱、TOTP 等多种二次验证方式，提升账户安全性",
			},
			{
				Icon:        "fas fa-chart-line",
				Title:       "实时监控",
				Description: "7×24 小时安全监控，异常行为实时告警，快速响应安全威胁",
			},
		},
		Scenarios: []Scenario{
			{
				Icon:        "fas fa-building",
				Title:       "企业单点登录",
				Description: "为企业内部系统提供统一的身份认证入口，员工只需一次登录即可访问所有授权应用，提升工作效率的同时确保安全合规。",
				Features:    []string{"统一身份管理", "简化登录流程", "提升用户体验"},
			},
			{
				Icon:        "fas fa-mobile-alt",
				Title:       "移动应用认证",
				Description: "为移动应用提供安全的身份认证服务，支持原生应用和 H5 应用，确保移动端用户身份安全和数据保护。",
				Features:    []string{"移动端优化", "生物识别支持", "离线认证能力"},
			},
			{
				Icon:        "fas fa-cloud",
				Title:       "SaaS 应用集成",
				Description: "为 SaaS 服务提供商提供标准化的身份认证接口，支持多租户架构，满足不同企业的定制化需求。",
				Features:    []string{"多租户支持", "白标解决方案", "灵活配置选项"},
			},
			{
				Icon:        "fas fa-exchange-alt",
				Title:       "API 访问控制",
				Description: "为微服务架构和 API 网关提供安全的访问控制机制，实现细粒度的权限管理和流量控制。",
				Features:    []string{"细粒度权限", "速率限制", "审计日志"},
			},
		},
		Advantages: []Advantage{
			{
				Icon:        "fas fa-tachometer-alt",
				Title:       "高性能架构",
				Description: "基于 Go 语言开发，支持高并发访问，单节点可处理万级 QPS 请求",
			},
			{
				Icon:        "fas fa-server",
				Title:       "高可用部署",
				Description: "支持集群部署和负载均衡，提供故障转移和自动恢复能力",
			},
			{
				Icon:        "fas fa-expand-arrows-alt",
				Title:       "弹性扩展",
				Description: "支持水平扩展，可根据业务需求动态调整服务容量",
			},
			{
				Icon:        "fas fa-database",
				Title:       "多数据库支持",
				Description: "支持 MySQL、PostgreSQL 等主流数据库，满足不同场景需求",
			},
			{
				Icon:        "fas fa-memory",
				Title:       "缓存优化",
				Description: "集成 Redis 缓存，提升响应速度，减少数据库压力",
			},
			{
				Icon:        "fas fa-chart-bar",
				Title:       "监控告警",
				Description: "完善的监控指标和告警机制，确保服务稳定运行",
			},
		},
	}

	// 渲染模板
	c.HTML(http.StatusOK, "index.html", data)
}
