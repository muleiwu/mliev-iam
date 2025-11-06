.PHONY: all install-frontend build-frontend build-packages build-oidc build-install build-oidc-only build-antd build-antd-only build-go release dev-build-frontend dev-go dev run clean clean-oidc clean-antd clean-go clean-packages help

# 默认目标：编译所有内容
all: build-frontend build-go

# 帮助信息
help:
	@echo "可用的 Make 目标："
	@echo "  make all                - 编译前端和后端（默认）"
	@echo "  make install-frontend   - 安装前端依赖（pnpm install）"
	@echo "  make build-frontend     - 编译所有前端应用（包含核心包）"
	@echo "  make build-packages     - 仅编译 @vben-core 核心包"
	@echo "  make build-oidc         - 编译 web-oidc 前端"
	@echo "  make build-install      - 编译 web-install 前端"
	@echo "  make build-antd         - 编译 web-antd 前端"
	@echo "  make build-go           - 编译 Go 后端"
	@echo "  make release            - 交叉编译发布（goreleaser）"
	@echo "  make dev-build-frontend - 开发模式（前端 watch）"
	@echo "  make dev-go             - 开发模式（Go 后端）"
	@echo "  make dev                - 开发模式（前端 watch + Go 后端）"
	@echo "  make run                - 编译前端并运行 Go 后端"
	@echo "  make clean              - 清理所有编译产物"
	@echo "  make clean-oidc         - 清理 web-oidc 编译产物"
	@echo "  make clean-antd         - 清理 web-antd 编译产物"
	@echo "  make clean-packages     - 清理核心包编译产物"
	@echo "  make clean-go           - 清理 Go 二进制文件"
	@echo "  make help               - 显示此帮助信息"

# 安装前端依赖
install-frontend:
	@echo "==> 开始安装前端依赖..."
	@echo "==> 安装 web-oidc 前端依赖..."
	@cd admin-web/apps/web-oidc && pnpm install
	@echo "==> 安装 web-antd 前端依赖..."
	@cd admin-web/apps/web-antd && pnpm install
	@echo "==> 前端依赖安装完成！"

# 编译 @vben-core 核心包（前端应用的依赖）
build-packages:
	@echo "==> 开始编译 @vben-core 核心包..."
	@cd admin-web && pnpm build --filter='@vben-core/*'
	@echo "==> @vben-core 核心包编译完成！"

# 编译所有前端应用（先确保核心包已构建）
build-frontend: build-packages
	@$(MAKE) build-oidc-only
	@$(MAKE) build-antd-only

# 编译 web-oidc 前端（包含核心包依赖）
build-oidc: build-packages build-oidc-only

# 编译 web-oidc 前端（仅应用本身，不构建核心包）
build-oidc-only:
	@echo "==> 开始编译 web-oidc 前端..."
	@echo "==> 清理 static/auth 目录..."
	@rm -rf static/auth
	@mkdir -p static/auth
	@echo "==> 切换到 admin-web/apps/web-oidc 目录并编译..."
	@cd admin-web/apps && pnpm build:oidc
	@echo "==> 复制编译产物到 static/auth..."
	@cp -r admin-web/apps/web-oidc/dist/* static/auth/
	@echo "==> web-oidc 前端编译完成！"

# 编译 web-antd 前端（包含核心包依赖）
build-antd: build-packages build-antd-only

# 编译 web-antd 前端（仅应用本身，不构建核心包）
build-antd-only:
	@echo "==> 开始编译 web-antd 前端..."
	@echo "==> 清理 static/admin 目录..."
	@rm -rf static/admin
	@mkdir -p static/admin
	@echo "==> 切换到 admin-web/apps/web-antd 目录并编译..."
	@cd admin-web/apps && pnpm build:antd
	@echo "==> 复制编译产物到 static/admin..."
	@cp -r admin-web/apps/web-antd/dist/* static/admin/
	@echo "==> web-antd 前端编译完成！"

# 编译 Go 后端
build-go:
	@echo "==> 开始编译 Go 后端..."
	@go build -o mliev-iam .
	@echo "==> Go 后端编译完成！二进制文件：mliev-iam"

# 交叉编译发布（使用 goreleaser）
release:
	@echo "==> 开始交叉编译发布..."
	@goreleaser release --snapshot --clean
	@echo "==> 交叉编译完成！产物位于 dist/ 目录"

dev-build-frontend:
	@echo "==> 启动开发模式..."
	@echo "==> 启动 web-oidc 前端 watch 模式（后台运行）..."
	@cd admin-web/apps/web-oidc && pnpm dev:static &
	@echo "==> 启动 web-install 前端 watch 模式（后台运行）..."
	@cd admin-web/apps/web-install && pnpm dev:static &
	@echo "==> 启动 web-antd 前端 watch 模式（后台运行）..."
	@cd admin-web/apps/web-antd && pnpm dev:static

dev-go:
	@echo "==> 启动 Go 后端 (mode=debug)..."
	@mode=debug go run .

# 开发模式：启动前端 watch 模式和 Go 后端
dev:
	@echo "==> 启动开发模式..."
	@echo "==> 启动 web-oidc 前端 watch 模式（后台运行）..."
	@cd admin-web/apps/web-oidc && pnpm dev:static &
	@echo "==> 启动 web-install 前端 watch 模式（后台运行）..."
	@cd admin-web/apps/web-install && pnpm dev:static &
	@echo "==> 启动 web-antd 前端 watch 模式（后台运行）..."
	@cd admin-web/apps/web-antd && pnpm dev:static &
	@sleep 3
	@echo "==> 启动 Go 后端 (mode=debug)..."
	@mode=debug go run .

# 编译前端并运行 Go 后端
run: build-frontend run-go
	@echo "==> 启动 Go 后端..."
	@go run .

# 编译前端并运行 Go 后端
run-go:
	@echo "==> 启动 Go 后端..."
	@go run .

# 清理所有编译产物
clean: clean-oidc clean-antd clean-packages clean-go
	@echo "==> 所有编译产物已清理完成！"

# 清理 web-oidc 编译产物
clean-oidc:
	@echo "==> 清理 web-oidc 编译产物..."
	@rm -rf static/auth
	@rm -rf admin-web/apps/web-oidc/dist

# 清理 web-antd 编译产物
clean-antd:
	@echo "==> 清理 web-antd 编译产物..."
	@rm -rf static/admin
	@rm -rf admin-web/apps/web-antd/dist

# 清理 @vben-core 核心包编译产物
clean-packages:
	@echo "==> 清理 @vben-core 核心包编译产物..."
	@find admin-web/packages/@core -type d -name "dist" -exec rm -rf {} + 2>/dev/null || true
	@echo "==> 核心包编译产物已清理！"

# 清理 Go 二进制文件
clean-go:
	@echo "==> 清理 Go 二进制文件..."
	@rm -f mliev-iam mliev-iam-test go_build_cnb_cool_mliev_iam_mliev_iam

