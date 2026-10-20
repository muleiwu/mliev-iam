# 认证门户界面设置

管理员在「系统设置 → 界面设置」中装修 `/auth` 下的登录、注册、授权、个人中心和协议页。

- **默认风格**保留现有门户样式。启用自定义后，留空的配置项继承所选预设。
- **保存并生效**更新全局配置，访问者刷新认证页面后生效。编辑和预览不会写入正式配置。
- **恢复默认**切回内置风格，保留已保存的自定义方案；点击保存后生效。
- **风格预设**提供经典默认、极简、品牌商务、深邃、暖纸、青庭和精工。切换预设保留所有已有装修覆盖；留空项目继承所选预设。预览中极简默认浅色、8／6 px 圆角，品牌商务默认跟随系统、16／8 px 圆角，深邃默认深色、16／8 px 圆角；所有预设均支持浅色和深色。
- **PC 端卡片位置**支持居中和居右，整张认证卡片一起移动，仅在视口宽度 ≥1024px 时生效。手机端保持居中，个人中心和协议内容布局不变。
- 应用自身名称、Logo 优先于全局品牌。无应用上下文或对应品牌字段为空时使用全局品牌。
- Logo、背景支持站内路径或 HTTP(S) 图片地址。上传支持 PNG、JPEG、WebP，使用「存储设置」中的对象存储和文件大小限制。
- 卡片圆角范围为 0–48 px，控件圆角为 0–32 px；留空表示继承，0 表示直角。
- 明暗模式支持浅色、深色、跟随系统。普通访客首次访问默认跟随系统，可通过页面右上方的主题菜单切换；系统模式实时响应系统变化，手动选择浅色或深色后保持该选择。
- 访客偏好保存在浏览器的 `mliev-auth-theme` localStorage 键，同源 `/auth` 页面和标签页共享，刷新后恢复；不会写回后台配置，也不跨设备同步。存储不可用时仍可在当前页面切换。
- **明暗模式的生效范围**：普通门户由访客偏好决定，不再以后台 `darkMode` 或预设明暗值作为首次访问默认值。后台外观预览和主题开发包继续遵循预览选择及装修配置，忽略访客偏好；嵌入个人中心继续由宿主控制。这些受控场景不显示访客主题菜单。

## 内置装修模板

| 模板 | 标识 | 视觉特点 | 默认明暗 | 卡片／控件圆角 |
| --- | --- | --- | --- | --- |
| 暖纸 | `paper` | 奶油纸色、陶土强调色、衬线标题与叠页阴影 | 浅色 | 4／4 px |
| 青庭 | `grove` | 鼠尾草绿、柔和大圆角与自然阴影 | 跟随系统 | 24／12 px |
| 精工 | `graphite` | 石墨色、细网格背景、直角边框与偏移阴影 | 跟随系统 | 0／0 px |

这三款模板参考 UI UX Pro Max 的 E-Ink / Paper、Organic Biophilic 与 Swiss Modernism 方向，并沿用身份认证产品的交互结构。模板无需外部字体或图片服务；暖纸标题使用本机衬线字体回退。切换模板只更换基础风格，品牌、图片、圆角、高级 CSS 与 PC 卡片位置继续保留。纯色或图片背景会覆盖精工的预设网格。

## 高级 CSS

CSS 按默认基础、所选预设、可视化覆盖、高级 CSS 的顺序应用，仅作用于认证前端文档。预览使用相同的主题、外壳和 UI 组件，以示例内容展示五类页面，不会执行真实登录、授权或账号操作。桌面预览固定为 1440×900，手机预览固定为 375×812，按容器宽度等比缩放；布局断点依据 iframe 的真实视口计算。

```css
:root {
  --auth-card-radius: 20px;
  --auth-control-radius: 8px;
}

.auth-entry-title { letter-spacing: .02em; }
.auth-task-shell-card { box-shadow: 0 16px 48px #16263714; }
```

常用选择器：`.auth-surface-entry`（认证页背景）、`.auth-task-shell-card`（认证卡片）、`.account-page`（个人中心）、`.agreement-page`（协议页）。自定义 CSS 上限为 32 KiB，完整配置序列化后上限为 60 KiB。

## 主题包与主题库

支持导出当前装修的开发 ZIP，在本地修改和预览后导入主题库。选择导入主题会完整载入其方案，保存后生效。详见 [主题开发包与主题库](appearance-theme-packages.zh-CN.md)。

## 接口与存储

| 接口 | 用途 |
| --- | --- |
| `GET /api/admin/settings/appearance/config` | 管理员读取默认／自定义模式及保存的方案 |
| `PUT /api/admin/settings/appearance/config` | 管理员保存 `{ mode, custom }`，返回规范化配置 |
| `POST /api/admin/attachments/brand-image` | 管理员通过 multipart `file` 上传图片，返回附件地址 |
| `GET /api/appearance` | 无需登录，读取 `{ mode, appearance }`；不暴露未启用的自定义方案 |

`custom` 还支持可选的 `sourceThemeId` 来源关联；关联丢失时保留装修快照。其他字段：`preset`、`desktopAlignment`、`brandName`、`logo`、`primaryColor`、`darkMode`、`backgroundMode`、`backgroundColor`、`backgroundImage`、`cardRadius`、`controlRadius`、`customCss`。`preset` 为 `default | minimal | brand | midnight | paper | grove | graphite`，`desktopAlignment` 为 `center | right`。颜色为 `#RRGGBB`，圆角为整数或 `null`；`mode` 为 `default` 或 `custom`。

历史配置缺失新字段时使用 `default` 和 `center`。保存时拒绝未知枚举；读取未知枚举时仅回退对应字段，保留其他装修。预设默认值在运行时解析，不会写成显式覆盖，因此切换预设后仍能继承新的默认值。

配置存储在现有 `settings` 表的 `appearance_settings_config` 键，无需迁移。客户端配置接口继续保留原有外观字段，与当前生效配置同步。读取失败或历史配置损坏时前端使用默认风格。

默认样式位于认证前端的 `src/themes/default.css`，预设配色位于 `src/themes/presets.css`，预设行为默认值位于 `src/themes/presets.ts`，协议展示样式位于 `src/themes/agreement.css`；`src/style.css` 保留 Tailwind 入口和基础样式。预览路由为 `/auth/#/appearance-preview`，只接受同源父窗口发来的版本化消息。开发时后台 Vite 将 `/auth` 代理到本地 Go 服务，因此需要先构建认证前端。
