# 配置热重载和服务重启指南

## 概述

本系统支持配置热重载功能，可以在不停止进程的情况下重新加载配置文件并重启服务。支持两种触发方式：系统信号和 HTTP API。

## 功能特性

- ✅ 支持通过 SIGHUP 信号触发重启
- ✅ 支持通过 HTTP API 触发重启
- ✅ 优雅关闭现有连接（5秒超时）
- ✅ 自动重新加载配置文件
- ✅ 重新初始化所有服务
- ✅ 完整的日志记录

## 使用方法

### 方法一：通过系统信号

向运行中的进程发送 SIGHUP 信号：

```bash
# 查找进程 ID
ps aux | grep mliev-iam

# 发送 SIGHUP 信号
kill -HUP <PID>
```

或者使用 pkill：

```bash
pkill -HUP mliev-iam
```

### 方法二：通过 HTTP API

使用管理员权限调用重载 API：

```bash
# 需要先登录获取 access token
curl -X POST http://localhost:8080/api/admin/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "username": "admin",
    "password": "admin123"
  }'

# 使用 token 调用重载接口
curl -X POST http://localhost:8080/api/admin/system/reload \
  -H "Authorization: Bearer <access_token>" \
  -H "Content-Type: application/json"
```

响应示例：

```json
{
  "code": 200,
  "message": "success",
  "data": {
    "message": "配置重载请求已接受，服务将在短时间内重启",
    "status": "accepted"
  }
}
```

## 重启流程

1. **接收信号/API请求**
   - 系统接收到 SIGHUP 信号或 API 重载请求

2. **停止服务**
   - HTTP 服务器优雅关闭（等待现有请求完成，最多5秒）
   - 其他服务按顺序停止

3. **重新加载配置**
   - 清除配置缓存
   - 重新读取 `config.yaml` 文件
   - 重新初始化配置项

4. **重启服务**
   - 重新初始化数据库连接
   - 重新初始化 Redis 连接
   - 重新启动 HTTP 服务器
   - 重新注册路由和中间件

5. **完成**
   - 服务恢复正常运行
   - 记录完整的操作日志

## 日志示例

```
[INFO] 收到 SIGHUP 信号，开始重启服务...
[INFO] 正在停止所有服务...
[INFO] 正在关闭HTTP服务器...
[INFO] HTTP服务器已优雅关闭
[INFO] 所有服务已停止
[INFO] 正在重新加载配置...
配置文件已成功重新加载
[INFO] 配置已成功重新加载
[INFO] 正在重新启动服务...
[INFO] 服务器启动于 :8080
```

## 注意事项

1. **配置文件路径**
   - 系统会从当前目录或 `./config` 目录读取 `config.yaml`
   - 确保配置文件存在且格式正确

2. **权限要求**
   - API 重载需要管理员权限（AdminAuthMiddleware）
   - 确保使用有效的管理员 token

3. **连接处理**
   - HTTP 服务器会等待现有请求完成（最多5秒）
   - 超时后会强制关闭
   - 建议在低流量时段执行重载

4. **数据库连接**
   - 重启时会重新建立数据库连接
   - 确保数据库配置正确

5. **重载频率**
   - API 重载请求采用非阻塞设计
   - 如果已有重载请求在队列中，新请求会被忽略

## 系统信号说明

- **SIGHUP**：触发配置重载和服务重启
- **SIGINT**：优雅关闭并退出（Ctrl+C）
- **SIGTERM**：优雅关闭并退出

## 故障排查

### 重载失败

如果重载失败，检查以下内容：

1. 配置文件语法是否正确
   ```bash
   # 验证 YAML 格式
   yamllint config.yaml
   ```

2. 查看日志输出
   ```bash
   tail -f logs/app.log
   ```

3. 数据库连接是否正常
   ```bash
   # 测试数据库连接
   mysql -h <host> -u <user> -p <database>
   ```

### API 调用失败

1. 确认已登录并获取有效 token
2. 检查 token 是否过期
3. 确认用户具有管理员权限

## API 端点说明

### POST /api/admin/system/reload

重新加载配置并重启服务

**请求头：**
```
Authorization: Bearer <access_token>
Content-Type: application/json
```

**响应：**
```json
{
  "code": 200,
  "message": "success",
  "data": {
    "message": "配置重载请求已接受，服务将在短时间内重启",
    "status": "accepted"
  }
}
```

### GET /api/admin/system/info

获取系统信息

**请求头：**
```
Authorization: Bearer <access_token>
```

**响应：**
```json
{
  "code": 200,
  "message": "success",
  "data": {
    "version": "1.0.0",
    "status": "running"
  }
}
```

## 最佳实践

1. **配置管理**
   - 使用版本控制管理配置文件
   - 重载前备份当前配置
   - 在测试环境先验证配置更改

2. **监控**
   - 监控重载操作的日志
   - 设置告警以检测重载失败
   - 记录配置变更历史

3. **自动化**
   - 可以将重载集成到 CI/CD 流程
   - 配置文件更新后自动触发重载

4. **回滚**
   - 如果重载后出现问题，恢复备份的配置文件
   - 再次执行重载操作

## 实现细节

### 代码结构

- `cmd/run.go`：主启动逻辑和信号处理
- `internal/pkg/reload/reload.go`：重载通道管理
- `app/controller/admin/system_controller.go`：API 控制器
- `internal/pkg/env/impl/env.go`：配置重载实现
- `internal/pkg/http_server/impl/http_server.go`：HTTP 服务器优雅关闭

### 技术要点

1. **信号处理**：使用 Go 的 `signal.Notify` 监听系统信号
2. **优雅关闭**：使用 `http.Server.Shutdown()` 实现优雅关闭
3. **通道通信**：使用 buffered channel 协调重载请求
4. **配置缓存**：使用 `sync.Map` 实现线程安全的配置缓存

## 相关文件

- 实现文档：`_docs/PROTOCOL_CONFIG_ENCRYPTION_REFACTORING.md`
- 项目规范：`_docs/PROJECT_SPECIFICATION.md`
- 测试指南：`_docs/TESTING_GUIDE.md`

