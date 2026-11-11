# 前端安全依赖升级与验证（2026-11-11）

已升级 `frontend-oidc` 和 `admin-web` 的可修复安全依赖，保留稳定版 vee-validate 4.15.1，并完成 Zod 4、Vitest 4 和 happy-dom 的兼容迁移。本报告记录本地升级与验证结果，不涉及线上部署。

## 审计结果

使用 npm 官方 registry 执行完整审计及 `--prod` 审计。数字为安全公告匹配条目，包含开发/构建依赖，不等同于生产页面可利用漏洞数量。

| 项目 | 升级前（严重/高危/中危/低危） | 升级后 | 升级后 `--prod` |
| --- | --- | --- | --- |
| frontend-oidc | 4 / 20 / 19 / 4，共 47 | 0 / 0 / 0 / 0 | 0 / 0 / 0 / 0 |
| admin-web | 6 / 140 / 116 / 28，共 290 | 0 / 0 / 0 / 1 | 0 / 0 / 0 / 1 |

机器可读数据、实际安装版本和剩余依赖路径见同目录的 `frontend-security-upgrade-2026-11-11.json`。没有添加审计忽略项或降低审计级别。管理端仍保留原 registry 配置，审计及本次安装通过命令级参数临时使用官方 registry。

### 唯一剩余项

- 包：`deepmerge@4.3.1`。
- 公告：[GHSA-gcqx-w48m-w2h4 / CVE-2026-93753](https://github.com/advisories/GHSA-gcqx-w48m-w2h4)，低危；核实时上游没有修复版本。
- 完整审计路径：`unbuild → @rollup/plugin-node-resolve → deepmerge`。
- `--prod` 审计路径：`internal/vite-config → nitropack → @rollup/plugin-node-resolve → deepmerge`。workspace 中构建包的 dependencies 也会进入这一分类。
- 使用场景：构建工具内部的对象合并；业务前端没有直接导入该包。公告描述攻击者控制合并源对象时可污染返回对象的原型，因此不将它视为已修复。
- 处置：保留原始 audit 输出，避免向构建配置合并过程提供不可信对象；跟踪 deepmerge 及 Rollup 插件更新，有修复版后继续升级并复验。未为这一条配置忽略规则。

原先记录的 node-forge 无补丁公告已随上游依赖更新从当前依赖树消失。`lodash.set` 已替换，不作为例外保留。

## 主要改动

| 范围 | 版本/处理 |
| --- | --- |
| Vue / Vite / Vitest | 3.5.45 / 7.3.8 / 4.1.12 |
| 表单校验 | Zod 4.6.7；vee-validate 继续使用 4.15.1；移除 `@vee-validate/zod` |
| 管理端默认值 | zod-defaults 0.2.3 |
| OIDC HTML 清洗和图表 | DOMPurify 3.4.17；@unovis/vue、@unovis/ts 同步至 1.8.1，清除旧 MapLibre peer |
| 管理端请求、国际化、图表 | Axios 1.21.1；vue-i18n/@intlify 11.4.18；ECharts 6.1.0 |
| 管理端测试环境 | happy-dom 21.0.4 |
| 安全路径赋值 | `lodash.set` 改为 lodash-es 4.18.1 的 `set`，保留共享库原导出名称 |

### 表单适配

两端新增本地 `toTypedSchema`，实现 vee-validate 4 的公开 TypedSchema 接口，处理异步校验、转换后的输出、联合类型错误、数组/嵌套路径、默认值和必填描述。

- 默认值合并保留用户输入、显式 null 和数组替换语义，过滤原型相关危险键。
- 在 schema 处理边界使用 Vue `toRaw`，解决深层只读响应式 schema 与 Zod 4 内部属性的兼容问题。
- 管理端更新旧 Zod 类型与默认值读取方式；保留数字空值、显式默认值及动态校验。
- OIDC 显式描述表单输入类型；注册分步校验只选择当前配置中存在的字段，适配 Zod 4 对不存在的 `.pick()` 字段的严格检查。
- 业务 HTTP 请求结构不变。浏览器验证确认注册、找回密码和资料补全仍按原字段提交。

### 工具链与传递依赖

- 更新管理端 catalog 和两端 lockfile；保留 pnpm 10.14.0 / 10.27.0。
- 迁移 Vitest 4 配置，删除旧 workspace 包装文件并保留测试发现范围；资源加载单测使用手动 load/error 事件，不请求外网脚本。
- 使用按父依赖限定的 overrides 修复 glob、minimatch、serialize-javascript、selector parser 等传递依赖。没有全局覆盖这些包的主版本。
- `depcheck@1.4.7` 仍使用旧 minimatch 默认导出，增加 pnpm patch 将其接到 minimatch 11 的具名导出；补丁记录在 `admin-web/patches/depcheck@1.4.7.patch`。
- ECharts 6/zrender 6 固定的 tslib 2.3.0 在浏览器预构建中报错，限定这两条依赖链使用 tslib 2.8.1。
- 显式给构建和 lint workspace 提供 TypeScript 5.9.3，消除自动安装的 TypeScript 6 peer；没有升级 TypeScript、路由和状态管理主版本。
- 管理端使用 vue-tsc 3.2.2，避免 3.3.14 对原有模板的额外误报。未为消除误报而修改已有的应用元数据代码。
- 不属于安全更新的 VXE Table / VXE PC UI 保留原安装版本 4.17.18 / 4.10.29。
- 调整 Vue class 类型、模板 ref 绑定、默认值工厂和菜单 key；为内容区与标签栏 ref 绑定新增行为回归测试。

## 验证结果

| 检查 | 结果 |
| --- | --- |
| OIDC 全量 Vitest | 32 个测试文件，504 项通过（原基线 492） |
| 管理端全量 Vitest | 56 个测试文件，436 项通过（原基线 416） |
| 两端类型检查 | 通过 |
| OIDC 生产构建与 theme-kit | 通过 |
| 管理端 workspace 生产构建 | 11 个构建任务全部通过 |
| frozen-lockfile + strict-peer-dependencies | 两端通过，无新增 peer 冲突 |
| ESLint / Stylelint 配置加载 | 通过 |
| depcheck + minimatch 11 功能 fixture | 通过 |
| archiver ZIP fixture | 通过 |
| git diff --check | 通过 |

新增测试覆盖适配层的转换值、异步 refinement、嵌套数组/联合错误、密码确认、默认值优先级、optional/nullable、重置、动态规则、响应式 schema，以及安全 `set` 和布局 ref 绑定。

浏览器使用独立本地 fixture、真实组件和受控 API 响应完成以下检查，未捕获页面异常：

1. 注册：错误邮箱和确认密码提示、两步注册及最终提交。
2. 找回密码：验证码长度校验及成功提交。
3. 资料补全：邮箱验证与提交。
4. Unovis：实际 SVG 图表渲染。
5. 管理端动态表单：默认值、动态规则、校验失败/成功、提交和重置。
6. ECharts：实际 Canvas 绘图，检测到 39,822 个非透明像素。

浏览器 fixture 的验证码接口返回未启用；腾讯/极验的会话、重试及票据传递由已有模拟 SDK 单测覆盖，本次没有调用线上验证码或真实业务写接口。截图、执行结果及临时 fixture 证据保存在 `/tmp/iam-security-browser/`；测试服务器和仓库内临时 fixture 已清理。

## 复验命令与构建目录

OIDC 子仓库：

```sh
npm_config_registry=https://registry.npmjs.org pnpm install --frozen-lockfile --strict-peer-dependencies
pnpm exec vue-tsc -b
pnpm exec vitest run
IAM_AUTH_OUT_DIR=$(mktemp -d /tmp/iam-security-auth.XXXXXX) pnpm build
pnpm audit --registry=https://registry.npmjs.org
pnpm audit --prod --registry=https://registry.npmjs.org
```

管理端子仓库：

```sh
npm_config_registry=https://registry.npmjs.org pnpm install --frozen-lockfile --strict-peer-dependencies
pnpm --filter @vben/web-antd typecheck
pnpm exec vitest run
IAM_WEB_OUT_DIR=$(mktemp -d /tmp/iam-security-admin.XXXXXX) pnpm exec turbo run build --filter=@vben/web-antd --env-mode=loose --force
pnpm audit --registry=https://registry.npmjs.org
pnpm audit --prod --registry=https://registry.npmjs.org
```

管理端审计因上述低危例外仍返回非零退出码，这不应被误报为零漏洞。

最后一轮构建产物分别位于 `/tmp/iam-security-auth.A0zPOp` 和 `/tmp/iam-security-admin.ljPgOj`。Turbo 默认隔离自定义环境变量，因此最终构建显式使用 `--env-mode=loose` 传递输出目录。首次 workspace 构建曾刷新本地被 Git 忽略的 `static/admin` 产物；未部署线上服务。

开始时记录的 30 个已有修改/未跟踪业务文件，收尾时哈希全部一致。升级代码在两个前端子仓库分别提交，主仓库记录子模块引用和本报告。
