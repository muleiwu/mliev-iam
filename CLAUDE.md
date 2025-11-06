# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 项目概述

MLiev IAM 是一个基于 Go 语言开发的企业级身份认证与访问管理（IAM）系统，支持单点登录（SSO）功能。项目采用分层架构设计，使用 Gin 框架、GORM ORM 和自研的轻量级依赖注入框架。

### 技术栈
- **Go 版本**: 1.25.0 (现代 Go 特性)
- **Web 框架**: Gin v1.10.1
- **ORM**: GORM v1.31.0
- **数据库**: MySQL/PostgreSQL/SQLite
- **缓存**: Redis v9.14.0
- **日志**: Zap v1.27.0
- **配置**: Viper v1.21.0
- **静态资源**: Go embed.FS (内嵌)
- **OIDC**: 完整的 OAuth2/OIDC 支持

## 开发命令

### 基本命令
```bash
# 运行项目
go run main.go

# 构建项目
go build -o mliev-iam main.go

# 安装依赖
go mod download

# 整理依赖
go mod tidy

# 生成RSA密钥对（用于OIDC）
openssl genrsa -out keys/oidc_private_key.pem 2048
openssl rsa -in keys/oidc_private_key.pem -pubout -out keys/oidc_public_key.pem

# 测试命令
go test ./...
```

### Docker 命令
```bash
# 构建镜像（注意：当前Dockerfile配置可能与SQLite不兼容）
docker build -t mliev-iam .

# 运行容器
docker run -p 8080:8080 mliev-iam

# 健康检查
curl http://localhost:8080/health
curl http://localhost:8080/health/simple
```

### 开发环境配置
1. 复制配置文件：`cp config.yaml.example config.yaml`
2. 修改 `config.yaml` 中的数据库和Redis连接信息
3. 确保 MySQL/PostgreSQL 和 Redis 服务已启动
4. 如需 OIDC RS256 签名，生成 RSA 密钥对放在 `keys/` 目录下

## 项目架构

### 核心架构设计
- **分层架构**：Controller → Service → DAO 三层设计
- **依赖注入**：基于接口的依赖注入框架，使用 Assembly 模式
- **RBAC权限模型**：完整的基于角色的访问控制系统
- **ORM集成**：GORM v1.31.0，支持软删除、钩子函数、关联关系
- **多数据库支持**：MySQL、PostgreSQL、SQLite

### 目录结构
```
mliev-iam/
├── app/                    # 应用核心代码
│   ├── controller/         # 控制器层（处理HTTP请求）
│   ├── service/           # 服务层（业务逻辑）
│   ├── dao/               # 数据访问层（数据库操作）
│   ├── model/             # 数据模型（GORM模型）
│   ├── dto/               # 数据传输对象
│   ├── middleware/        # 中间件（认证、权限、CORS等）
│   └── constants/         # 常量定义
├── cmd/                   # 命令行入口
├── config/                # 配置管理
├── internal/              # 内部包
│   ├── interfaces/        # 接口定义
│   ├── helper/           # 全局助手类
│   └── pkg/              # 基础设施包
├── static/               # 静态资源（内嵌）
├── templates/            # 模板文件（内嵌）
├── keys/                 # 密钥文件目录
└── logs/                 # 日志文件目录
```

### 依赖注入框架
项目使用自研的轻量级DI框架：
- `interfaces.AssemblyInterface`：组件装配接口
- `interfaces.ServerInterface`：服务器启动接口
- `interfaces.HelperInterface`：全局助手接口
- 通过 `cmd/run.go` 中的 `initializeServices()` 统一初始化所有组件

### 数据模型设计

#### 核心RBAC模型
- **User（用户）**：支持状态管理、多属性扩展、部门关联
- **Role（角色）**：支持层级结构、数据权限范围控制
- **Permission（权限）**：支持菜单/按钮/API/数据四种权限类型
- **Department（部门）**：支持多层级组织架构

#### 关联关系
- `UserRole`：用户角色关联，支持审批流程、时效管理
- `RolePermission`：角色权限关联，支持权限约束
- `UserDepartment`：用户部门关联，支持主部门设置

#### OIDC 模型
- `Application`：OAuth2/OIDC 客户端应用管理
- `AuthorizationCode`：授权码管理
- `OidcToken`：OIDC令牌管理
- `OidcSession`：OIDC会话管理

### 中间件系统
- `AuthMiddleware`：JWT认证中间件
- `RBACMiddleware`：基于角色的权限验证中间件
- `DataScopeMiddleware`：数据权限范围控制中间件
- `CORSMiddleware`：跨域请求处理中间件

### 权限控制机制
1. **URL级权限**：基于用户角色和权限的URL访问控制
2. **数据权限范围**：全部数据/本部门/本部门及下级/仅本人四个级别
3. **权限继承**：基于角色层级关系的权限继承
4. **条件约束**：支持基于条件的动态权限验证

## 配置管理

### 主要配置项
- `server.mode`：运行模式（debug/release/test）
- `database.*`：数据库连接配置
- `redis.*`：Redis连接配置
- `jwt.*`：JWT令牌配置
- `oidc.*`：OIDC协议配置
- `log.*`：日志配置

### OIDC 配置要点
- 支持HS256和RS256签名算法
- 可配置各种令牌的生命周期
- 支持PKCE、令牌内省、令牌撤销等高级功能
- Redis存储会话信息

## 数据库迁移

系统使用 GORM AutoMigrate 功能：
- 启动时自动创建/更新表结构
- 支持软删除（`DeletedAt` 字段）
- 自动维护时间戳（`CreatedAt`、`UpdatedAt`）
- 支持JSON字段扩展

## 安全特性
- 密码哈希存储（不存储明文密码）
- 登录失败次数限制和账户锁定
- JWT令牌认证
- CORS跨域保护
- 数据权限隔离
- 软删除数据保护

## 测试
当前项目暂无单元测试，如需测试：
- 使用 `go test ./...` 运行所有测试
- 建议在 `app/` 各层目录添加对应的 `*_test.go` 文件

## 前端管理界面
项目包含 `admin-web/` 目录，为Vue.js前端管理界面：
- 使用现代前端技术栈
- 独立的npm包管理
- 完整的用户界面和权限管理功能

## 开发注意事项
1. 所有模型都支持软删除，查询时需要注意 `DeletedAt` 条件
2. 使用 Helper 单例获取数据库、日志等基础设施服务
3. 遵循 Controller-Service-DAO 分层规范
4. 权限验证统一使用中间件处理
5. 配置文件中的敏感信息建议使用环境变量
6. OIDC 功能需要正确配置RSA密钥对（使用RS256时）
7. 项目使用 Go 1.25.0，启用了现代 Go 特性和 embed 功能
8. 静态资源通过 embed.FS 内嵌到二进制文件中