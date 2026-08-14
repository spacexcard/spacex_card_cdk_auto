#!/usr/bin/env bash
# CDK Recharge System — Docker 一键部署
# 用法:
#   curl -fsSL <raw-url>/deploy/docker-deploy.sh | bash -s -- --domain gpt66.cc
#   或本地: ./deploy/docker-deploy.sh --domain gpt66.cc
set -euo pipefail

DOMAIN=""
REPO_URL="https://github.com/spacexcard/spacex_card_cdk_auto.git"
INSTALL_DIR="/opt/cdk-recharge"

usage() {
  echo "用法: $0 [选项]"
  echo "  --domain DOMAIN    绑定域名（如 gpt66.cc），不传则用 :80 纯 IP 访问"
  echo "  --dir DIR          安装目录（默认 /opt/cdk-recharge）"
  echo "  --skip-build       跳过 Docker 构建（已有镜像时）"
  exit 1
}

SKIP_BUILD=0
while [[ $# -gt 0 ]]; do
  case "$1" in
    --domain) DOMAIN="$2"; shift 2 ;;
    --dir)    INSTALL_DIR="$2"; shift 2 ;;
    --skip-build) SKIP_BUILD=1; shift ;;
    -h|--help) usage ;;
    *) echo "未知参数: $1"; usage ;;
  esac
done

echo "=========================================="
echo "  CDK Recharge System — Docker 部署"
echo "=========================================="
echo "  域名: ${DOMAIN:-'(无，HTTP 模式)'}"
echo "  目录: ${INSTALL_DIR}"
echo

# 1) 检查 Docker
if ! command -v docker &>/dev/null; then
  echo "[1/6] 安装 Docker..."
  curl -fsSL https://get.docker.com | sh
  systemctl enable --now docker
else
  echo "[1/6] Docker 已安装 ✓"
fi

# docker compose (v2 plugin)
if docker compose version &>/dev/null; then
  DC="docker compose"
elif command -v docker-compose &>/dev/null; then
  DC="docker-compose"
else
  echo "[1/6] 安装 docker-compose-plugin..."
  apt-get update -qq && apt-get install -y -qq docker-compose-plugin 2>/dev/null \
    || yum install -y docker-compose-plugin 2>/dev/null \
    || { echo "请手动安装 docker compose plugin"; exit 1; }
  DC="docker compose"
fi
echo "  compose: $DC ✓"

# 2) 拉取/更新代码
if [[ -d "$INSTALL_DIR/.git" ]]; then
  echo "[2/6] 更新代码..."
  cd "$INSTALL_DIR"
  git pull --ff-only origin master 2>/dev/null || git pull origin master
else
  echo "[2/6] 克隆代码..."
  git clone "$REPO_URL" "$INSTALL_DIR"
  cd "$INSTALL_DIR"
fi

# 3) 生成 JWT Secret（首次）
JWT_FILE="$INSTALL_DIR/.jwt_secret"
if [[ ! -f "$JWT_FILE" ]]; then
  JWT_SECRET=$(openssl rand -hex 32)
  echo "$JWT_SECRET" > "$JWT_FILE"
  chmod 600 "$JWT_FILE"
  echo "[3/6] 生成 JWT Secret ✓"
else
  JWT_SECRET=$(cat "$JWT_FILE")
  echo "[3/6] JWT Secret 已存在 ✓"
fi

# 4) 写入 .env（docker compose 自动读取）
ENV_FILE="$INSTALL_DIR/.env"
if [[ ! -f "$ENV_FILE" ]]; then
  cat > "$ENV_FILE" <<EOF
JWT_SECRET=${JWT_SECRET}
INSTALL_MODE=wizard
EOF
  chmod 600 "$ENV_FILE"
  echo "[4/6] 创建 .env ✓"
else
  # 确保 JWT_SECRET 存在
  if ! grep -q "^JWT_SECRET=" "$ENV_FILE"; then
    echo "JWT_SECRET=${JWT_SECRET}" >> "$ENV_FILE"
  fi
  echo "[4/6] .env 已存在 ✓"
fi

# 5) 配置 Caddyfile
echo "[5/6] 配置 Caddy..."
if [[ -n "$DOMAIN" ]]; then
  cat > "$INSTALL_DIR/Caddyfile" <<CADDY
${DOMAIN} {
    encode zstd gzip

    @blocked path /.env* /app.env* /.git/* /data/*
    handle @blocked {
        respond 404
    }

    reverse_proxy cdk:8080 {
        header_up X-Real-IP {client_ip}
        header_up X-Forwarded-Proto {scheme}
        header_up X-Forwarded-Host {host}
    }
}
CADDY
  echo "  域名: ${DOMAIN} (自动 HTTPS) ✓"
else
  cat > "$INSTALL_DIR/Caddyfile" <<CADDY
:80 {
    encode zstd gzip

    @blocked path /.env* /app.env* /.git/* /data/*
    handle @blocked {
        respond 404
    }

    reverse_proxy cdk:8080 {
        header_up X-Real-IP {client_ip}
        header_up X-Forwarded-Proto {scheme}
        header_up X-Forwarded-Host {host}
    }
}
CADDY
  echo "  HTTP 模式 (:80) ✓"
fi

# docker-compose.yml 注入 JWT_SECRET
# docker compose 会自动从 .env 读取变量，但我们需要把它传给容器
# 修改 docker-compose.yml 中的 environment 部分添加 JWT_SECRET
# 通过 .env 文件 + docker-compose env_file 实现
if ! grep -q "env_file" "$INSTALL_DIR/docker-compose.yml" 2>/dev/null; then
  sed -i '/environment:/i\    env_file:\n      - .env' "$INSTALL_DIR/docker-compose.yml" 2>/dev/null || true
fi

# 6) 构建 & 启动
echo "[6/6] 构建并启动容器..."
cd "$INSTALL_DIR"
if [[ "$SKIP_BUILD" -eq 0 ]]; then
  $DC build --no-cache cdk
fi
$DC up -d

echo
echo "=========================================="
echo "  部署完成！"
echo "=========================================="
if [[ -n "$DOMAIN" ]]; then
  echo "  访问: https://${DOMAIN}"
  echo "  请确保域名 DNS 已指向本机 IP"
else
  echo "  访问: http://$(curl -s ifconfig.me 2>/dev/null || echo '<本机IP>'):80"
fi
echo "  首次打开进入安装向导，设置管理员账号和卡台 API Key"
echo
echo "  常用命令:"
echo "    cd ${INSTALL_DIR}"
echo "    $DC logs -f          # 查看日志"
echo "    $DC restart           # 重启"
echo "    $DC down              # 停止"
echo "    $DC up -d --build     # 更新后重建"
echo
