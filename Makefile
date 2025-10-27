.PHONY: all build-frontend build-oidc build-antd build-go clean clean-oidc clean-antd clean-go help

# 默认目标：编译所有内容
all: build-frontend build-go

# 帮助信息
help:
	@echo "可用的 Make 目标："
	@echo "  make all           - 编译前端和后端（默认）"
	@echo "  make build-frontend - 编译所有前端应用"
	@echo "  make build-oidc    - 编译 web-oidc 前端"
	@echo "  make build-antd    - 编译 web-antd 前端"
	@echo "  make build-go      - 编译 Go 后端"
	@echo "  make clean         - 清理所有编译产物"
	@echo "  make clean-oidc    - 清理 web-oidc 编译产物"
	@echo "  make clean-antd    - 清理 web-antd 编译产物"
	@echo "  make clean-go      - 清理 Go 二进制文件"
	@echo "  make help          - 显示此帮助信息"

# 编译所有前端应用
build-frontend: build-oidc build-antd

# 编译 web-oidc 前端
build-oidc:
	@echo "==> 开始编译 web-oidc 前端..."
	@echo "==> 清理 static/auth 目录..."
	@rm -rf static/auth
	@mkdir -p static/auth
	@echo "==> 切换到 admin-web/apps/web-oidc 目录并编译..."
	@cd admin-web/apps/web-oidc && pnpm build
	@echo "==> 复制编译产物到 static/auth..."
	@cp -r admin-web/apps/web-oidc/dist/* static/auth/
	@echo "==> web-oidc 前端编译完成！"

# 编译 web-antd 前端
build-antd:
	@echo "==> 开始编译 web-antd 前端..."
	@echo "==> 清理 static/admin 目录..."
	@rm -rf static/admin
	@mkdir -p static/admin
	@echo "==> 切换到 admin-web/apps/web-antd 目录并编译..."
	@cd admin-web/apps/web-antd && pnpm build
	@echo "==> 复制编译产物到 static/admin..."
	@cp -r admin-web/apps/web-antd/dist/* static/admin/
	@echo "==> web-antd 前端编译完成！"

# 编译 Go 后端
build-go:
	@echo "==> 开始编译 Go 后端..."
	@go build -o mliev-iam .
	@echo "==> Go 后端编译完成！二进制文件：mliev-iam"

# 清理所有编译产物
clean: clean-oidc clean-antd clean-go
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

# 清理 Go 二进制文件
clean-go:
	@echo "==> 清理 Go 二进制文件..."
	@rm -f mliev-iam mliev-iam-test go_build_cnb_cool_mliev_iam_mliev_iam

