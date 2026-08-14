.PHONY: help dev build docker-up docker-down docker-logs clean

help:
	@echo "CDK Recharge System"
	@echo "==================="
	@echo "  make dev          - 本地开发（需装 Go + Node）"
	@echo "  make build        - 构建前后端"
	@echo "  make docker-up    - Docker 一键启动（构建 + 运行）"
	@echo "  make docker-down  - 停止 Docker 容器"
	@echo "  make docker-logs  - 查看 Docker 日志"
	@echo "  make clean        - 清理构建产物"

dev:
	@echo "启动本地开发环境..."
	@echo "  后端: cd backend && go run ./cmd/server"
	@echo "  前端: cd frontend && npm run dev"

build:
	@echo "构建前端..."
	cd frontend && npm ci && npm run build
	@echo "构建后端..."
	cd backend && CGO_ENABLED=1 go build -ldflags="-s -w" -o ../dist/cdk-recharge ./cmd/server
	@echo "完成 → dist/cdk-recharge + frontend/dist/"

docker-up:
	@if [ ! -f .env ]; then \
		echo "JWT_SECRET=$$(openssl rand -hex 32)" > .env; \
		echo "INSTALL_MODE=wizard" >> .env; \
		chmod 600 .env; \
		echo "✓ 已生成 .env（含 JWT_SECRET）"; \
	fi
	@if grep -q 'YOUR_DOMAIN' Caddyfile 2>/dev/null; then \
		echo ""; \
		echo "⚠  请先编辑 Caddyfile，将 :YOUR_DOMAIN 替换为实际域名"; \
		echo "   无域名可改为 :80"; \
		echo ""; \
		exit 1; \
	fi
	docker compose up -d --build

docker-down:
	docker compose down

docker-logs:
	docker compose logs -f

clean:
	rm -rf frontend/dist dist/
	cd backend && go clean
