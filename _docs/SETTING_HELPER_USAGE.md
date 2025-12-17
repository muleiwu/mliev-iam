# Setting Helper 使用指南

## 概述

Setting Helper 提供了一个便捷的方式来读取和设置系统配置，自动使用缓存来提高性能。

## 功能特性

- **缓存支持**: 自动使用缓存（Redis 或内存缓存）来提高读取性能
- **简单易用**: 提供简洁的 API 来读取和设置配置
- **默认值支持**: 支持在配置不存在时返回默认值
- **自动失效**: 更新配置时自动清除缓存

## 配置

在 `config.yaml` 中添加缓存配置：

```yaml
cache:
  driver: memory  # 缓存驱动: redis, memory, none
```

- `redis`: 使用 Redis 作为缓存后端（适合分布式部署）
- `memory`: 使用内存缓存（适合单机部署）
- `none`: 不使用缓存

## 使用方法

### 1. 获取 Helper 实例

```go
import (
    "cnb.cool/mliev/iam/mliev-iam/internal/helper"
)

// 获取 Helper 实例
h := helper.GetHelper()
```

### 2. 读取配置

```go
// 读取配置，如果不存在返回空字符串
siteName := h.GetSetting("site_name")

// 读取配置，如果不存在返回默认值
maxUsers := h.GetSettingWithDefault("max_users", "100")
```

### 3. 设置配置

```go
// 更新配置（会自动清除缓存）
err := h.SetSetting("site_name", "My IAM System")
if err != nil {
    // 处理错误
    log.Printf("Failed to set setting: %v", err)
}
```

## 示例：在控制器中使用

```go
package controller

import (
    "net/http"
    "github.com/gin-gonic/gin"
    "cnb.cool/mliev/iam/mliev-iam/internal/interfaces"
)

type SettingController struct {
    helper interfaces.HelperInterface
}

// 获取系统设置
func (c *SettingController) GetSiteName(ctx *gin.Context) {
    // 从缓存或数据库读取配置
    siteName := c.helper.GetSettingWithDefault("site_name", "Default Site Name")
    
    ctx.JSON(http.StatusOK, gin.H{
        "site_name": siteName,
    })
}

// 更新系统设置
func (c *SettingController) UpdateSiteName(ctx *gin.Context) {
    var req struct {
        SiteName string `json:"site_name" binding:"required"`
    }
    
    if err := ctx.ShouldBindJSON(&req); err != nil {
        ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    
    // 更新配置（会自动清除缓存）
    if err := c.helper.SetSetting("site_name", req.SiteName); err != nil {
        ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    
    ctx.JSON(http.StatusOK, gin.H{"message": "Setting updated successfully"})
}
```

## 数据表结构

Setting 配置存储在 `settings` 表中：

| 字段 | 类型 | 说明 |
|------|------|------|
| id | uint | 主键 |
| key | string | 配置键（唯一） |
| value | string | 配置值 |
| category | string | 分类（如 "register", "security"） |
| remark | string | 备注说明 |
| created_at | time | 创建时间 |
| updated_at | time | 更新时间 |

## 缓存策略

- **缓存键格式**: `setting:{key}`
- **缓存过期时间**: 1 小时
- **更新策略**: 更新配置时自动删除对应缓存

## 注意事项

1. 确保在应用启动时正确初始化了 Cache Assembly
2. 如果使用 Redis 缓存，确保 Redis 服务可用
3. GetSetting 方法在配置不存在时返回空字符串
4. SetSetting 方法只更新 value，如需更新其他字段请直接使用 DAO 层

## 性能优化

使用缓存后，重复读取相同配置的性能提升：

- **无缓存**: 每次从数据库查询，约 1-5ms
- **有缓存**: 从缓存读取，约 0.01-0.1ms

推荐对频繁读取的配置使用 GetSetting 方法。

