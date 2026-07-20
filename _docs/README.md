# IAM系统API文档

本目录包含IAM系统的API接口文档，使用OpenAPI 3.0（Swagger 3.0）格式编写。

## 文档列表

### 品牌 Logo - [`brand/README.md`](brand/README.md)

MLiev IAM 正式品牌 Logo 源文件、设计含义、标准色和使用规则。

### 企业身份源对接评估 - `ENTERPRISE_IDENTITY_PROVIDER_INTEGRATION_ASSESSMENT.md`

评估飞书、企业微信、钉钉作为 IAM 上游企业身份源的对接方案，包含当前项目能力盘点、平台差异、建议领域模型、同步流程、IAM 统一认证服务能力清单和实施路线。

### 身份凭据主密钥使用与轮换 - [`IDENTITY_CREDENTIALS_KEY_MANAGEMENT.md`](IDENTITY_CREDENTIALS_KEY_MANAGEMENT.md)

说明身份凭据主密钥的保护范围、首次配置、Kubernetes 注入、版本化密钥环、安全轮换、存量密文迁移、旧密钥删除门槛和故障恢复流程。

### 1. 部门管理API - `department_api.yaml`

部门管理模块的完整API文档，包含以下功能：

#### 🔧 基础CRUD操作
- **创建部门** - `POST /admin/departments`
- **获取部门列表** - `GET /admin/departments`
- **获取部门详情** - `GET /admin/departments/{id}`
- **更新部门信息** - `PUT /admin/departments/{id}`
- **删除部门** - `DELETE /admin/departments/{id}`

#### 🌳 树形结构管理
- **获取部门树** - `GET /admin/departments/tree`
- **获取根部门** - `GET /admin/departments/roots`
- **获取子部门** - `GET /admin/departments/{id}/children`
- **获取后代部门** - `GET /admin/departments/{id}/descendants`
- **移动部门** - `PUT /admin/departments/{id}/move`

#### 📊 状态和批量管理
- **更新状态** - `PUT /admin/departments/{id}/status`
- **批量操作** - `POST /admin/departments/batch`

#### 📈 统计和查询
- **活跃部门统计** - `GET /admin/departments/stats/active-count`
- **按负责人查询** - `GET /admin/managers/{managerId}/departments`
- **部门用户列表** - `GET /admin/departments/{departmentId}/users`

#### 🔧 维护工具
- **验证层级结构** - `POST /admin/departments/validate-hierarchy`
- **修复层级结构** - `POST /admin/departments/repair-hierarchy`

## 如何使用文档

### 1. 在线查看
您可以使用以下工具来查看和测试API：

#### Swagger UI
将YAML文件导入到Swagger UI中：
- 在线版本: https://editor.swagger.io/
- 本地安装: https://swagger.io/tools/swagger-ui/

#### Postman
1. 打开Postman
2. 点击 Import -> File -> 选择 `department_api.yaml`
3. Postman会自动生成所有API接口的请求示例

#### Insomnia
1. 打开Insomnia
2. 创建新的Request Collection
3. 导入OpenAPI规范文件

### 2. 代码生成
使用OpenAPI Generator可以生成多种语言的客户端SDK：

```bash
# 安装OpenAPI Generator
npm install @openapitools/openapi-generator-cli -g

# 生成JavaScript/TypeScript客户端
openapi-generator-cli generate -i department_api.yaml -g typescript-fetch -o ./clients/typescript

# 生成Java客户端
openapi-generator-cli generate -i department_api.yaml -g java -o ./clients/java

# 生成Python客户端
openapi-generator-cli generate -i department_api.yaml -g python -o ./clients/python
```

### 3. 本地服务器测试

#### 使用Docker运行Swagger UI
```bash
docker run -p 8080:8080 -e SWAGGER_JSON=/app/department_api.yaml -v $(pwd):/app swaggerapi/swagger-ui
```

然后访问 http://localhost:8080 查看文档。

#### 使用Node.js运行简单服务器
```bash
# 安装依赖
npm install -g swagger-ui-serve

# 启动服务器
swagger-ui-serve department_api.yaml
```

## API特性说明

### 认证方式
所有API接口都需要JWT Bearer Token认证：
```
Authorization: Bearer <your-jwt-token>
```

### 响应格式
所有API响应都遵循统一格式：
```json
{
  "code": 0,          // 状态码，0表示成功
  "message": "操作成功",// 响应消息
  "data": {}          // 响应数据，可能为null
}
```

### 错误处理
API使用标准HTTP状态码和自定义错误码：
- `200` - 操作成功
- `400` - 请求参数错误
- `401` - 未授权访问
- `403` - 权限不足
- `404` - 资源不存在
- `500` - 服务器内部错误

### 分页参数
支持分页的接口统一使用以下参数：
- `page` - 页码（从1开始）
- `pageSize` - 每页数量（最大100）

### 数据类型说明

#### 部门类型 (type)
- `0` - 公司
- `1` - 事业部
- `2` - 部门
- `3` - 团队

#### 部门状态 (status)
- `0` - 禁用
- `1` - 启用
- `2` - 冻结

#### 数据权限范围 (dataScope)
- `1` - 全部数据权限
- `2` - 本部门数据权限
- `3` - 本部门及下级数据权限
- `4` - 仅本人数据权限

## 开发说明

### 添加新的API文档
1. 在 `_docs` 目录下创建新的YAML文件
2. 遵循OpenAPI 3.0规范
3. 更新本README文件添加新文档的说明

### 文档维护
- 当API接口有变更时，及时更新对应的YAML文档
- 保持示例数据的准确性和完整性
- 确保错误响应码和消息与实际实现一致

### 验证文档
使用Swagger Editor验证文档格式：
```bash
# 使用在线编辑器
# 访问 https://editor.swagger.io/
# 粘贴YAML内容进行验证

# 或使用CLI工具
swagger-codegen validate -i department_api.yaml
```

## 联系方式

如有API文档相关问题，请联系：
- 邮箱: api-support@example.com
- 项目地址: https://github.com/your-org/iam-system
