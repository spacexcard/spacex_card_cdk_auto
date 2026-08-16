#!/usr/bin/env bash
# 一键推送到 GitHub（从 .push.env 读 PAT，不写进 git 历史）
#
# 身份硬约束：
#   - 仓库所有者 / 推送账号：仅 zovocard
#   - 提交 author/committer 显示名：仅 zovocard 或 zovo_card
#   - 提交邮箱：必须含 zovocard 且为 users.noreply.github.com
# 任何其它身份（如 amelia-aabb / 个人邮箱）一律拒绝推送，避免公网暴露。
#
# 用法：
#   ./scripts/push.sh
#       检查「待推送」提交作者后普通推送
#   ./scripts/push.sh --force
#       强制推送（仍检查作者）
#   ./scripts/push.sh --rewrite-authors
#       把当前分支上非法作者的提交改写成 canonical（从最早坏提交的父节点改到 HEAD）
#   ./scripts/push.sh --rewrite-authors --force
#       改写 + 强制推送（用于抹掉已在 GitHub 上的非法作者）
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

ENV_FILE="${PUSH_ENV_FILE:-$ROOT/.push.env}"

# ── 允许的公开身份（写死，不可用 .push.env 放宽）────────────────
ALLOWED_GIT_USERS=("zovocard")
ALLOWED_AUTHOR_NAMES=("zovocard" "zovo_card")
CANONICAL_AUTHOR_NAME="zovocard"
CANONICAL_AUTHOR_EMAIL="zovocard@users.noreply.github.com"
CANONICAL_GIT_USER="zovocard"
CANONICAL_GIT_REPO="zovo_card_cdk_auto"

DO_FORCE=0
DO_REWRITE=0
for arg in "$@"; do
  case "$arg" in
    --force) DO_FORCE=1 ;;
    --rewrite-authors|--fix-authors) DO_REWRITE=1 ;;
    -h|--help)
      sed -n '2,22p' "$0"
      exit 0
      ;;
    *)
      echo "未知参数: $arg"
      echo "支持: --force  --rewrite-authors  --help"
      exit 1
      ;;
  esac
done

if [[ ! -f "$ENV_FILE" ]]; then
  echo "缺少 $ENV_FILE"
  echo "请先执行："
  echo "  cp .push.env.example .push.env"
  echo "  编辑 .push.env，填入 GIT_PAT=ghp_xxxx"
  exit 1
fi

# shellcheck disable=SC1090
source "$ENV_FILE"

GIT_USER="${GIT_USER:-$CANONICAL_GIT_USER}"
GIT_REPO="${GIT_REPO:-$CANONICAL_GIT_REPO}"
GIT_BRANCH="${GIT_BRANCH:-master}"

_req_name="${GIT_AUTHOR_NAME:-$CANONICAL_AUTHOR_NAME}"
_req_email="${GIT_AUTHOR_EMAIL:-$CANONICAL_AUTHOR_EMAIL}"

name_allowed() {
  local n
  for n in "${ALLOWED_AUTHOR_NAMES[@]}"; do
    [[ "$1" == "$n" ]] && return 0
  done
  return 1
}

user_allowed() {
  local u
  for u in "${ALLOWED_GIT_USERS[@]}"; do
    [[ "$1" == "$u" ]] && return 0
  done
  return 1
}

email_allowed() {
  [[ "$1" == *"zovocard"*@users.noreply.github.com ]]
}

identity_ok() {
  name_allowed "$1" && email_allowed "$2"
}

if ! user_allowed "$GIT_USER"; then
  echo "拒绝：GIT_USER='$GIT_USER' 不在白名单（仅允许: ${ALLOWED_GIT_USERS[*]}）"
  exit 1
fi

if ! identity_ok "$_req_name" "$_req_email"; then
  echo "警告：.push.env 作者 $_req_name <$_req_email> 不合规，已改用 canonical"
  _req_name="$CANONICAL_AUTHOR_NAME"
  _req_email="$CANONICAL_AUTHOR_EMAIL"
fi

GIT_AUTHOR_NAME="$_req_name"
GIT_AUTHOR_EMAIL="$_req_email"

if [[ -z "${GIT_PAT:-}" || "$GIT_PAT" == ghp_在这里粘贴你的token* || "$GIT_PAT" == ghp_xxx* || "$GIT_PAT" == ghp_replace* ]]; then
  echo "请在 $ENV_FILE 里设置有效的 GIT_PAT"
  exit 1
fi

# 本仓库提交身份（仅影响后续新 commit）
git config user.name "$GIT_AUTHOR_NAME"
git config user.email "$GIT_AUTHOR_EMAIL"

git remote remove origin 2>/dev/null || true
git remote add origin "https://github.com/${GIT_USER}/${GIT_REPO}.git"

git fetch origin "$GIT_BRANCH" 2>/dev/null || git fetch origin 2>/dev/null || true

# 待推送范围（普通 push 只拦这个）
PUSH_RANGE=""
if git rev-parse --verify "origin/${GIT_BRANCH}" >/dev/null 2>&1; then
  PUSH_RANGE="origin/${GIT_BRANCH}..HEAD"
else
  PUSH_RANGE=""
fi

echo "作者(canonical): $GIT_AUTHOR_NAME <$GIT_AUTHOR_EMAIL>"
echo "允许显示名: ${ALLOWED_AUTHOR_NAMES[*]}"
echo "远程: https://github.com/${GIT_USER}/${GIT_REPO}.git"
echo "分支: $GIT_BRANCH"
echo "HEAD: $(git log -1 --format='%h %an <%ae> %s')"
[[ -n "$PUSH_RANGE" ]] && echo "待推范围: $PUSH_RANGE"
echo

# 列出某 rev-list 范围内不合规提交：hash\tshort\twho\tsubj
list_bad() {
  local range=("$@")
  if [[ ${#range[@]} -eq 0 || -z "${range[0]:-}" ]]; then
    return 0
  fi
  git log --format='%H%x09%h%x09%an%x09%ae%x09%cn%x09%ce%x09%s' "${range[@]}" 2>/dev/null \
    | while IFS=$'\t' read -r hash short an ae cn ce subj; do
        if ! identity_ok "$an" "$ae" || ! identity_ok "$cn" "$ce"; then
          printf '%s\t%s\t%s <%s> / committer %s <%s>\t%s\n' "$hash" "$short" "$an" "$ae" "$cn" "$ce" "$subj"
        fi
      done
}

# 当前分支历史上所有非法作者提交（用于 --rewrite-authors）
# 限制深度 500，避免误伤极老历史时一次 filter 过大；可调 AUTHOR_REWRITE_MAX
AUTHOR_REWRITE_MAX="${AUTHOR_REWRITE_MAX:-500}"
mapfile -t BAD_ON_BRANCH < <(list_bad "-n" "$AUTHOR_REWRITE_MAX" "HEAD")

mapfile -t BAD_TO_PUSH < <(
  if [[ -n "$PUSH_RANGE" ]]; then
    list_bad "$PUSH_RANGE"
  else
    # 无 origin 时：至少检查 HEAD 最近一次
    list_bad "-n" "1" "HEAD"
  fi
)

print_bad() {
  local line hash short who subj
  while IFS=$'\t' read -r hash short who subj; do
    [[ -z "${hash:-}" ]] && continue
    echo "  $short  $who  $subj"
  done
}

rewrite_from_oldest_bad() {
  local oldest oldest_parent
  if [[ ${#BAD_ON_BRANCH[@]} -eq 0 ]]; then
    echo "分支上无不合规作者，跳过改写"
    return 0
  fi
  # git log 新→旧，取最后一行 = 最早的坏提交
  oldest="$(printf '%s\n' "${BAD_ON_BRANCH[@]}" | tail -1 | cut -f1)"
  if ! git rev-parse --verify "${oldest}^" >/dev/null 2>&1; then
    echo "最早坏提交 $oldest 没有父提交（root），无法自动改写，请手动处理"
    exit 1
  fi
  oldest_parent="$(git rev-parse "${oldest}^")"
  echo "改写作者: $(git rev-parse --short "$oldest_parent")..HEAD（含 $(printf '%s\n' "${BAD_ON_BRANCH[@]}" | wc -l) 个坏提交）"
  echo "  → $CANONICAL_AUTHOR_NAME <$CANONICAL_AUTHOR_EMAIL>"

  FILTER_BRANCH_SQUELCH_WARNING=1 git filter-branch -f --env-filter "
    export GIT_AUTHOR_NAME='$CANONICAL_AUTHOR_NAME'
    export GIT_AUTHOR_EMAIL='$CANONICAL_AUTHOR_EMAIL'
    export GIT_COMMITTER_NAME='$CANONICAL_AUTHOR_NAME'
    export GIT_COMMITTER_EMAIL='$CANONICAL_AUTHOR_EMAIL'
  " "${oldest_parent}..HEAD"

  git for-each-ref --format='%(refname)' refs/original/ 2>/dev/null | while read -r ref; do
    git update-ref -d "$ref" 2>/dev/null || true
  done
  git reflog expire --expire=now --all 2>/dev/null || true

  mapfile -t BAD_ON_BRANCH < <(list_bad "-n" "$AUTHOR_REWRITE_MAX" "HEAD")
  if [[ ${#BAD_ON_BRANCH[@]} -gt 0 ]]; then
    echo "改写后仍有不合规提交："
    printf '%s\n' "${BAD_ON_BRANCH[@]}" | print_bad
    exit 1
  fi
  echo "✓ 作者已全部改为 $CANONICAL_AUTHOR_NAME"
  echo "改写后 HEAD: $(git log -1 --format='%h %an <%ae> %s')"
}

if [[ $DO_REWRITE -eq 1 ]]; then
  if [[ ${#BAD_ON_BRANCH[@]} -gt 0 ]]; then
    echo "分支上发现 ${#BAD_ON_BRANCH[@]} 个不合规提交："
    printf '%s\n' "${BAD_ON_BRANCH[@]}" | print_bad
    echo
    rewrite_from_oldest_bad
    # 改写后与远端几乎必然分叉
    if git rev-parse --verify "origin/${GIT_BRANCH}" >/dev/null 2>&1; then
      if [[ $DO_FORCE -ne 1 ]]; then
        if ! git merge-base --is-ancestor "origin/${GIT_BRANCH}" HEAD 2>/dev/null \
           || [[ "$(git rev-parse HEAD)" != "$(git rev-parse "origin/${GIT_BRANCH}")" \
                 && -n "$(git log "origin/${GIT_BRANCH}..HEAD" --oneline 2>/dev/null)" \
                 && -n "$(git log "HEAD..origin/${GIT_BRANCH}" --oneline 2>/dev/null)" ]]; then
          # 若 filter 后 tip 与 origin 不同且非快进，要求 force
          if ! git merge-base --is-ancestor "origin/${GIT_BRANCH}" HEAD 2>/dev/null; then
            echo
            echo "历史已改写且与 origin/${GIT_BRANCH} 非快进，需要 --force 才能覆盖远端非法作者。"
            echo "确认后执行："
            echo "  ./scripts/push.sh --rewrite-authors --force"
            exit 1
          fi
        fi
        # 即使是「改写后仍祖先关系」的边界情况：若 origin tip 作者仍坏，也必须 force
        origin_tip_an="$(git log -1 --format='%an' "origin/${GIT_BRANCH}")"
        origin_tip_ae="$(git log -1 --format='%ae' "origin/${GIT_BRANCH}")"
        if ! identity_ok "$origin_tip_an" "$origin_tip_ae"; then
          echo
          echo "origin/${GIT_BRANCH} tip 仍是非法作者（$origin_tip_an），必须 --force 覆盖。"
          echo "  ./scripts/push.sh --rewrite-authors --force"
          exit 1
        fi
      fi
    fi
  else
    echo "分支近 ${AUTHOR_REWRITE_MAX} 提交内无非法作者"
  fi
else
  # 普通推送：只拦待推送提交
  if [[ ${#BAD_TO_PUSH[@]} -gt 0 ]]; then
    echo "拒绝推送：待推送提交含非 zovocard 身份："
    printf '%s\n' "${BAD_TO_PUSH[@]}" | print_bad
    echo
    echo "处理："
    echo "  ./scripts/push.sh --rewrite-authors           # 改写本地"
    echo "  ./scripts/push.sh --rewrite-authors --force   # 改写并覆盖远端"
    exit 1
  fi

  # 额外提醒：远端 tip 已是非法作者（本地无新提交时）
  if git rev-parse --verify "origin/${GIT_BRANCH}" >/dev/null 2>&1; then
    oan="$(git log -1 --format='%an' "origin/${GIT_BRANCH}")"
    oae="$(git log -1 --format='%ae' "origin/${GIT_BRANCH}")"
    if ! identity_ok "$oan" "$oae"; then
      echo "警告：origin/${GIT_BRANCH} 最新提交作者是 $oan <$oae>（非法）"
      echo "       本地若无新提交，普通 push 不会抹掉它。要清理请："
      echo "         git checkout $GIT_BRANCH && git reset --hard origin/$GIT_BRANCH"
      echo "         ./scripts/push.sh --rewrite-authors --force"
      echo
    fi
  fi
fi

# 最终再扫一遍待推范围（rewrite 后）
if [[ -n "$PUSH_RANGE" ]] || git rev-parse --verify "origin/${GIT_BRANCH}" >/dev/null 2>&1; then
  if git rev-parse --verify "origin/${GIT_BRANCH}" >/dev/null 2>&1; then
    PUSH_RANGE="origin/${GIT_BRANCH}..HEAD"
  fi
fi
mapfile -t BAD_TO_PUSH < <(
  if [[ -n "${PUSH_RANGE:-}" ]]; then list_bad "$PUSH_RANGE"; fi
)
if [[ ${#BAD_TO_PUSH[@]} -gt 0 && $DO_FORCE -eq 0 ]]; then
  # force 改写场景下 range 可能仍含“远端旧 tip 视角”的差异；非 force 必须干净
  echo "待推送仍有不合规作者，中止"
  printf '%s\n' "${BAD_TO_PUSH[@]}" | print_bad
  exit 1
fi

# 强制推送时：确保 HEAD 本身作者合规
han="$(git log -1 --format='%an' HEAD)"
hae="$(git log -1 --format='%ae' HEAD)"
hcn="$(git log -1 --format='%cn' HEAD)"
hce="$(git log -1 --format='%ce' HEAD)"
if ! identity_ok "$han" "$hae" || ! identity_ok "$hcn" "$hce"; then
  echo "拒绝：HEAD 作者 $han <$hae> 不合规。请先 --rewrite-authors"
  exit 1
fi

PUSH_URL="https://${GIT_USER}:${GIT_PAT}@github.com/${GIT_USER}/${GIT_REPO}.git"

if [[ $DO_FORCE -eq 1 ]]; then
  echo "执行: git push --force ..."
  git push "$PUSH_URL" "HEAD:refs/heads/${GIT_BRANCH}" --force
else
  echo "执行: git push ..."
  git push "$PUSH_URL" "HEAD:refs/heads/${GIT_BRANCH}"
fi

git branch --set-upstream-to="origin/${GIT_BRANCH}" 2>/dev/null || true
git remote set-url origin "https://github.com/${GIT_USER}/${GIT_REPO}.git"

echo
echo "✓ 推送完成"
echo "  网页: https://github.com/${GIT_USER}/${GIT_REPO}"
echo "  提示: 勿把 .push.env 提交到 git；PAT 泄露请立即到 GitHub 撤销"
echo "  作者约束: 仅 ${ALLOWED_AUTHOR_NAMES[*]} / *@users.noreply.github.com(含 zovocard)"
