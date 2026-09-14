.PHONY: all help install-frontend build-packages build-frontend build-oidc build-oidc-only build-antd build-antd-only build-go release dev-build-frontend dev-go dev run run-go clean clean-oidc clean-antd clean-packages clean-go migrate-new migrate-status migrate-up migrate-down

# 默认目标：编译所有内容
all: build-frontend build-go

# 帮助信息
help:
	@echo "可用的 Make 目标："
	@echo "  make all                - 编译前端和后端（默认）"
	@echo "  make install-frontend   - 安装前端依赖（pnpm install）"
	@echo "  make build-frontend     - 编译所有前端应用（包含核心包）"
	@echo "  make build-packages     - 仅编译 @vben-core 核心包"
	@echo "  make build-oidc         - 编译 frontend-oidc（OIDC 登录/同意 UI）"
	@echo "  make build-antd         - 编译 web-antd 前端"
	@echo "  make build-go           - 编译 Go 后端"
	@echo "  make release            - 交叉编译发布（goreleaser）"
	@echo "  make dev-build-frontend - 开发模式（前端 watch）"
	@echo "  make dev-go             - 开发模式（Go 后端）"
	@echo "  make dev                - 开发模式（前端 watch + Go 后端）"
	@echo "  make run                - 编译前端并运行 Go 后端"
	@echo "  make clean              - 清理所有编译产物"
	@echo "  make clean-oidc         - 清理 frontend-oidc 编译产物"
	@echo "  make clean-antd         - 清理 web-antd 编译产物"
	@echo "  make clean-packages     - 清理核心包编译产物"
	@echo "  make clean-go           - 清理 Go 二进制文件"
	@echo "  make help               - 显示此帮助信息"

# 安装前端依赖
install-frontend:
	@echo "==> 开始安装前端依赖..."
	@echo "==> 安装 frontend-oidc 前端依赖（独立项目，OIDC 登录 UI）..."
	@cd frontend-oidc && pnpm install
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
	@rm -rf static/install
	@$(MAKE) build-oidc-only
	@$(MAKE) build-antd-only

# 编译 OIDC 登录 UI（frontend-oidc，独立项目，不依赖 @vben-core）
# vite.config.ts 的 outDir 已指向 ../static/auth，且 emptyOutDir=true 会自动清理
build-oidc: build-oidc-only

build-oidc-only:
	@echo "==> 开始编译 frontend-oidc..."
	@cd frontend-oidc && pnpm build
	@echo "==> frontend-oidc 编译完成！产物已写入 static/auth"

# 编译 web-antd 前端（包含核心包依赖）
build-antd: build-packages build-antd-only

# 编译 web-antd 前端（仅应用本身，不构建核心包）
# vite.config.mts 的 outDir 已指向 ../../../static/admin，且 emptyOutDir=true 会自动清理
build-antd-only:
	@echo "==> 开始编译 web-antd 前端..."
	@rm -rf static/admin
	@cd admin-web && pnpm build:antd
	@echo "==> web-antd 前端编译完成！产物已写入 static/admin"


# vite.config.mts 的 outDir 已指向 ../../../static/install，且 emptyOutDir=true 会自动清理

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
	@echo "==> 启动 frontend-oidc watch 模式（后台运行）..."
	@cd frontend-oidc && pnpm dev &
	@echo "==> 启动 web-antd 前端 watch 模式（后台运行）..."

dev-go:
	@echo "==> 启动 Go 后端 (mode=debug)..."
	@mode=debug go run .

# 开发模式：启动前端 watch 模式和 Go 后端
dev:
	@echo "==> 启动开发模式..."
	@echo "==> 启动 frontend-oidc watch 模式（后台运行）..."
	@cd frontend-oidc && pnpm dev &
	@echo "==> 启动 web-antd 前端 watch 模式（后台运行）..."
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

# 清理 frontend-oidc 编译产物
clean-oidc:
	@echo "==> 清理 frontend-oidc 编译产物..."
	@rm -rf static/auth
	@rm -rf frontend-oidc/dist

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

# ============================================================
# 数据库迁移（Goose, 多方言）
#   每次 schema 变更，在 migrations/{mysql,postgresql,sqlite} 下各生成一份 .sql。
#   流程：
#     1. make migrate-new NAME=add_xxx_field   三个方言各生成一个空模板
#     2. 按各方言语法分别填写 -- +goose Up / -- +goose Down
#     3. 启动服务（install 流程）自动执行对应方言目录下的 goose.Up
# ============================================================

migrate-new:
	@if [ -z "$(NAME)" ]; then echo "用法: make migrate-new NAME=add_xxx_field"; exit 1; fi
	@TS=$$(date +%Y%m%d%H%M%S); \
	for D in mysql postgresql sqlite; do \
	  FILE="migrations/$$D/$${TS}_$(NAME).sql"; \
	  printf -- '-- +goose Up\n-- +goose StatementBegin\n\n-- +goose StatementEnd\n\n-- +goose Down\n-- +goose StatementBegin\n\n-- +goose StatementEnd\n' > $$FILE; \
	  echo "==> $$FILE"; \
	done; \
	echo "==> 三份模板已生成，记得按方言语法填写 up/down"

migrate-status:
	@echo "==> 启动服务前请使用 admin 后台查询 goose_db_version 表"

migrate-up:
	@echo "==> 运行迁移：启动进程即可自动执行，或使用 sql: UPDATE system；或通过 go run ./cmd/migrate up"

migrate-down:
	@echo "==> 不建议在生产 down；如需回滚请手写补偿迁移"

