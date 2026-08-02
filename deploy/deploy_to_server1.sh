#!/usr/bin/env bash
# 部署 CDK 白标到生产机（主机/跳板用环境变量，勿把真实 IP 写进公开仓库）
# 例：DEPLOY_HOST=x.x.x.x DEPLOY_JUMP=y.y.y.y DEPLOY_SSH_PORT=22 ./deploy/deploy_to_server1.sh
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
HOST="${DEPLOY_HOST:?set DEPLOY_HOST}"
JUMP="${DEPLOY_JUMP:-}"
SSH_PORT="${DEPLOY_SSH_PORT:-22}"
REMOTE_DIR=/opt/cdk-recharge
if [[ -n "$JUMP" ]]; then
  SSH=(ssh -o BatchMode=yes -o StrictHostKeyChecking=accept-new -J "root@${JUMP}:${SSH_PORT}" -p "$SSH_PORT" "root@${HOST}")
  SCP=(scp -o BatchMode=yes -o StrictHostKeyChecking=accept-new -o "ProxyJump=root@${JUMP}:${SSH_PORT}" -P "$SSH_PORT")
else
  SSH=(ssh -o BatchMode=yes -o StrictHostKeyChecking=accept-new -p "$SSH_PORT" "root@${HOST}")
  SCP=(scp -o BatchMode=yes -o StrictHostKeyChecking=accept-new -P "$SSH_PORT")
fi

echo "==> build backend"
cd "$ROOT/backend"
go build -ldflags="-s -w" -o "$ROOT/dist/cdk-recharge" ./cmd/server/main.go

echo "==> build frontend"
cd "$ROOT/frontend"
npm run build

echo "==> pack"
rm -rf "$ROOT/dist/web"
mkdir -p "$ROOT/dist/web" "$ROOT/dist/data"
cp -a "$ROOT/frontend/dist/." "$ROOT/dist/web/"
PACK_ITEMS=(cdk-recharge web)
if [[ -f "$ROOT/VERSION" ]]; then
  cp -f "$ROOT/VERSION" "$ROOT/dist/VERSION"
  PACK_ITEMS+=(VERSION)
fi
tar -C "$ROOT/dist" -czf "$ROOT/dist/cdk-bundle.tgz" "${PACK_ITEMS[@]}"

echo "==> upload"
"${SSH[@]}" "mkdir -p $REMOTE_DIR/data $REMOTE_DIR/web"
"${SCP[@]}" "$ROOT/dist/cdk-bundle.tgz" "root@${HOST}:/tmp/cdk-bundle.tgz"
"${SCP[@]}" "$ROOT/deploy/cdk-recharge.service" "root@${HOST}:/etc/systemd/system/cdk-recharge.service"

echo "==> extract + env"
JWT_NEW=$(openssl rand -hex 24)
# 若远程已有 app.env 则保留 JWT；否则写入
"${SSH[@]}" bash -s <<REMOTE
set -euo pipefail
cd $REMOTE_DIR
# 备份当前二进制，便于回滚
if [[ -x $REMOTE_DIR/cdk-recharge ]]; then
  cp -a $REMOTE_DIR/cdk-recharge $REMOTE_DIR/cdk-recharge.bak.\$(date +%Y%m%d%H%M%S)
fi
tar -xzf /tmp/cdk-bundle.tgz -C $REMOTE_DIR
chmod +x $REMOTE_DIR/cdk-recharge
if [[ ! -f $REMOTE_DIR/app.env ]]; then
  cat > $REMOTE_DIR/app.env <<EOF
SERVER_HOST=127.0.0.1
SERVER_PORT=9080
SERVER_MODE=release
DB_PATH=$REMOTE_DIR/data/cdk_recharge.db
WEB_DIR=$REMOTE_DIR/web
JWT_SECRET=${JWT_NEW}
JWT_EXPIRATION_HOURS=24
INSTALL_MODE=wizard
# 公网经 Caddy/CF 反代时不要写 127.0.0.1，否则浏览器安装会报 setup not allowed from this address
# 安全靠 X-Setup-Token；若只内网装可再改为 127.0.0.1,::1
SETUP_ALLOW_CIDRS=
TRUSTED_PROXIES=127.0.0.1
EOF
  chmod 600 $REMOTE_DIR/app.env
  echo "created app.env"
else
  echo "kept existing app.env"
fi
systemctl daemon-reload
systemctl enable cdk-recharge
systemctl restart cdk-recharge
sleep 1
systemctl --no-pager --full status cdk-recharge | head -25
curl -sS http://127.0.0.1:9080/health || true
echo
journalctl -u cdk-recharge -n 40 --no-pager | tail -40
REMOTE

echo "==> done. 完成 Caddy + DNS 后访问 https://gptcdk.ai"
