#!/usr/bin/env bash
# 一键推送到 GitHub（从 .push.env 读 PAT，不写进 git 历史）
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

ENV_FILE="${PUSH_ENV_FILE:-$ROOT/.push.env}"
EXAMPLE="$ROOT/.push.env.example"

if [[ ! -f "$ENV_FILE" ]]; then
  echo "缺少 $ENV_FILE"
  echo "请先执行："
  echo "  cp .push.env.example .push.env"
  echo "  编辑 .push.env，填入 GIT_PAT=ghp_xxxx"
  exit 1
fi

# shellcheck disable=SC1090
source "$ENV_FILE"

GIT_USER="${GIT_USER:-spacexcard}"
GIT_REPO="${GIT_REPO:-spacex_card_cdk_auto}"
GIT_BRANCH="${GIT_BRANCH:-master}"
GIT_AUTHOR_NAME="${GIT_AUTHOR_NAME:-spacexcard}"
GIT_AUTHOR_EMAIL="${GIT_AUTHOR_EMAIL:-spacexcard@users.noreply.github.com}"

if [[ -z "${GIT_PAT:-}" || "$GIT_PAT" == ghp_在这里粘贴你的token* || "$GIT_PAT" == ghp_xxx* ]]; then
  echo "请在 $ENV_FILE 里设置有效的 GIT_PAT"
  exit 1
fi

# 本仓库提交身份（显示在 commit 上）
git config user.name "$GIT_AUTHOR_NAME"
git config user.email "$GIT_AUTHOR_EMAIL"

# 干净 remote（不含 token）
git remote remove origin 2>/dev/null || true
git remote add origin "https://github.com/${GIT_USER}/${GIT_REPO}.git"

echo "作者: $(git config user.name) <$(git config user.email)>"
echo "远程: https://github.com/${GIT_USER}/${GIT_REPO}.git"
echo "分支: $GIT_BRANCH"
echo "最近提交:"
git log -1 --format='  %h %an <%ae> %s'
echo

# 用 PAT 推送（不把 token 写入 remote 配置）
PUSH_URL="https://${GIT_USER}:${GIT_PAT}@github.com/${GIT_USER}/${GIT_REPO}.git"

if [[ "${1:-}" == "--force" ]]; then
  echo "执行: git push --force ..."
  git push "$PUSH_URL" "HEAD:refs/heads/${GIT_BRANCH}" --force
else
  echo "执行: git push ..."
  git push "$PUSH_URL" "HEAD:refs/heads/${GIT_BRANCH}"
fi

# 绑定上游为干净 URL
git branch --set-upstream-to="origin/${GIT_BRANCH}" 2>/dev/null || true
git remote set-url origin "https://github.com/${GIT_USER}/${GIT_REPO}.git"

echo
echo "✓ 推送完成"
echo "  网页: https://github.com/${GIT_USER}/${GIT_REPO}"
echo "  提示: 勿把 .push.env 提交到 git；PAT 泄露请立即到 GitHub 撤销"
