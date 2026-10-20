# 认证门户界面设置

管理员在「系统设置 → 界面设置」中装修 `/auth` 下的登录、注册、授权、个人中心和协议页。

- **默认风格**保留现有门户样式。启用自定义后，留空的配置项仍使用默认值。
- **保存并生效**更新全局配置，访问者刷新认证页面后生效。编辑和预览不会写入正式配置。
- **恢复默认**切回内置风格，保留已保存的自定义方案；点击保存后生效。
- 应用自身名称、Logo 优先于全局品牌。无应用上下文或对应品牌字段为空时使用全局品牌。
- Logo、背景支持站内路径或 HTTP(S) 图片地址。上传支持 PNG、JPEG、WebP，使用「存储设置」中的对象存储和文件大小限制。
- 卡片圆角范围为 0–48 px，控件圆角为 0–32 px；留空表示继承，0 表示直角。
- 明暗模式支持浅色、深色、跟随系统。个人中心嵌入时仍接受现有宿主的明暗模式设置。

## 高级 CSS

CSS 在默认风格和可视化配置之后加载，仅作用于认证前端文档。预览使用相同的主题、外壳和 UI 组件，以示例内容展示五类页面，不会执行真实登录、授权或账号操作。

```css
:root {
  --auth-card-radius: 20px;
  --auth-control-radius: 8px;
}

.auth-entry-title { letter-spacing: .02em; }
.auth-task-shell-card { box-shadow: 0 16px 48px #16263714; }
```

常用选择器：`.auth-surface-entry`（认证页背景）、`.auth-task-shell-card`（认证卡片）、`.account-page`（个人中心）、`.agreement-page`（协议页）。自定义 CSS 上限为 32 KiB，完整配置序列化后上限为 60 KiB。

## 接口与存储

| 接口 | 用途 |
| --- | --- |
| `GET /api/admin/settings/appearance/config` | 管理员读取默认／自定义模式及保存的方案 |
| `PUT /api/admin/settings/appearance/config` | 管理员保存 `{ mode, custom }`，返回规范化配置 |
| `POST /api/admin/attachments/brand-image` | 管理员通过 multipart `file` 上传图片，返回附件地址 |
| `GET /api/appearance` | 无需登录，读取 `{ mode, appearance }`；不暴露未启用的自定义方案 |

`custom` 字段：`brandName`、`logo`、`primaryColor`、`darkMode`、`backgroundMode`、`backgroundColor`、`backgroundImage`、`cardRadius`、`controlRadius`、`customCss`。颜色为 `#RRGGBB`，圆角为整数或 `null`；`mode` 为 `default` 或 `custom`。

配置存储在现有 `settings` 表的 `appearance_settings_config` 键，无需迁移。客户端配置接口继续保留原有外观字段，与当前生效配置同步。读取失败或历史配置损坏时前端使用默认风格。

默认样式位于认证前端的 `src/themes/default.css`，协议展示样式位于 `src/themes/agreement.css`；`src/style.css` 保留 Tailwind 入口和基础样式。预览路由为 `/auth/#/appearance-preview`，只接受同源父窗口发来的版本化消息。开发时后台 Vite 将 `/auth` 代理到本地 Go 服务，因此需要先构建认证前端。
