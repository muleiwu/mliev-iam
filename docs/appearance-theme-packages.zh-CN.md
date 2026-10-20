# 主题开发包与主题库

在「系统设置 → 界面设置」中使用「导出主题包」「导入主题包」与「我的主题」。只有管理员可以读写主题库或导出开发包。

## 使用流程

1. 选择内置预设，调整外观。点击「导出主题包」，填写名称、版本以及可选作者和描述。
2. 导出当前编辑区的有效配置，包括未保存的修改，导出操作不会保存装修。默认模式只导出默认外观，不包含停用的自定义方案；预览器的设备和临时明暗选择不写入主题。
3. 解压 ZIP，在目录中运行 `node preview.mjs`（Node.js 22+），打开终端提示的本地地址。无需安装 npm 依赖或启动 IAM。
4. 修改 `theme.json`、`theme.css` 或 `assets/`，预览自动刷新；JSON 读取错误会显示在画布上方。可查看五类示例页面、明暗模式和桌面／手机布局。
5. 将目录重新压缩成 ZIP 后导入。主题加入列表，但不会自动改变草稿或线上外观。
6. 点击主题的「使用」，完整载入包内品牌、颜色、背景、布局和 CSS；有未保存修改时会提示替换。点击「保存并生效」后才影响访问者。

内置预设仍保留当前装修覆盖。主题库中的方案是导入时的快照；修改当前装修后保存不会改写原包，列表会显示「已修改」。再次发布请增加版本号，同名同版本导入会被拒绝。正在生效的主题须先切换并保存后才能删除。删除库记录不会删除可能被其他装修使用的图片。

## 包结构

```text
theme.json                 名称、版本、外观和资源清单
theme.css                  唯一可编辑主题 CSS
assets/                    主题图片
preview/                   可信的基础样式与示例预览运行时
preview.mjs                仅监听 127.0.0.1 的本地预览服务器
README.md                  与包一同导出的开发指南
external-resources.json    未打包的外部资源地址
```

ZIP 根目录或单一顶层文件夹均可导入。导入只读取主题清单、CSS 和声明的图片，不执行包内 HTML、JavaScript 或启动脚本。再次导出时会重新附带当前 IAM 的可信预览运行时。

`theme.json` 示例：

```json
{
  "formatVersion": 1,
  "name": "我的纸感主题",
  "version": "1.0.0",
  "author": "设计团队",
  "description": "适用于团队身份门户",
  "appearance": {
    "preset": "paper",
    "desktopAlignment": "right",
    "brandName": "团队门户",
    "logo": "assets/logo.png",
    "primaryColor": "#9a4c20",
    "darkMode": "auto",
    "backgroundMode": "default",
    "backgroundColor": "",
    "backgroundImage": "",
    "cardRadius": 12,
    "controlRadius": 8
  },
  "assets": [{ "path": "assets/logo.png", "mimeType": "image/png" }]
}
```

`appearance` 的其他规则见 [界面设置](appearance-settings.zh-CN.md)。CSS 写在 `theme.css` 中，不要在清单中添加 `customCss` 或 `sourceThemeId`。主题只依赖一个内置预设。名称最多 100 字符、版本最多 32 字符、作者最多 100 字符、描述最多 1000 字符。

```css
:root { --auth-card-radius: 18px; }
.auth-entry-title { letter-spacing: .02em; }
.auth-surface-entry { background-image: url("assets/background.png"); }
.dark .auth-entry-title { color: #f1e8db; }
```

新增图片时同步登记 `assets` 清单。背景、Logo 与 CSS 中的本地资源必须使用声明的 `assets/` 路径。外部 HTTP(S) 资源保留地址，不会下载入包，因此本地预览仍可能需要联网。

## 图片与限制

站内 `/attachments/{uuid}` 图片直接从对象存储读取并去重打包。缺失或无法读取的站内图片会使导出失败，不会生成缺图的包。导入时会先完成校验，再上传图片并重写引用；失败时回滚主题与附件记录并清理本次上传对象，已有装修和附件不受影响。

- CSS ≤32 KiB；完整配置序列化后 ≤60 KiB。
- ZIP ≤32 MiB；解压内容 ≤64 MiB；最多 128 个条目。
- 图片支持 PNG、JPEG、WebP，扩展名和实际内容须匹配；最大边长 8192px，最多 3200 万像素。
- 图片还受目标站点存储设置的单文件大小限制。包含图片的导入需要配置对象存储，纯 CSS／配置包不需要。
- 拒绝未知格式版本、越界路径、符号链接、重复条目、缺失资源和损坏图片。

## 管理接口与部署

| 方法与路径 | 行为 |
| --- | --- |
| `POST /api/admin/settings/appearance/export` | JSON `{name, version, author?, description?, settings}`，返回 ZIP |
| `POST /api/admin/appearance-themes/import` | multipart `file`，导入并返回主题详情 |
| `GET /api/admin/appearance-themes?page=1&pageSize=5` | 返回 `{items,total,page,pageSize}`，列表不包含大段 CSS |
| `GET /api/admin/appearance-themes/:id` | 返回完整 `appearance` 快照和元信息 |
| `DELETE /api/admin/appearance-themes/:id` | 删除非生效主题，冲突返回 409 |

配置中的可选 `sourceThemeId` 表示来源主题。实际展示使用已保存的外观快照；关联不存在时取消关联并保留快照。

新增 `20261020000000_create_appearance_themes.sql`，支持 MySQL、PostgreSQL、SQLite。认证前端的 `pnpm build` 同时生成 `static/auth/theme-kit/`，由 Go 静态资源机制嵌入发布程序。只运行 `vite build` 不会生成开发包运行时，请使用完整构建命令。新增图片解码依赖用于验证完整 WebP 内容。
