#!/usr/bin/env python3
"""删除 ChatGPT 账号已绑定的支付卡片 (payment method)。

流程: 用 session 里的 accessToken 调 backend-api -> 列卡 -> 删卡。
chatgpt.com 在 Cloudflare 后, 但 Python requests(HTTP/1.1 + OpenSSL 指纹)实测能通过。

用法:
    python3 delete_card.py session.json --list          # 只列出
    python3 delete_card.py session.json                  # 列出 + 确认后删全部
    python3 delete_card.py session.json --all --yes      # 删全部, 不确认
    python3 delete_card.py session.json --pm pm_xxx       # 删指定卡
第一个参数也可直接是 accessToken 字符串或完整 session JSON 字符串。
"""
from __future__ import annotations

import argparse
import base64
import json
import sys

try:
    import requests
except ImportError:
    sys.exit("缺少依赖, 请先: pip install requests")

BASE = "https://chatgpt.com/backend-api/payments"
UA = ("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 "
      "(KHTML, like Gecko) Chrome/132.0.0.0 Safari/537.36")


def load_session(arg: str) -> tuple[str, str]:
    """返回 (accessToken, account_id)。arg 可为文件路径 / JSON 字符串 / 裸 accessToken。"""
    text = arg
    try:
        with open(arg, encoding="utf-8") as f:
            text = f.read()
    except (OSError, IOError):
        pass
    text = (text or "").strip()
    if not text:
        sys.exit("session 为空")

    access, account = "", ""
    if text.startswith("{"):
        data = json.loads(text)
        access = str(data.get("accessToken") or "").strip()
        account = str((data.get("account") or {}).get("id") or "").strip()
    else:
        access = text

    if not access:
        sys.exit("未找到 accessToken")
    if not account:
        account = account_from_token(access)
    return access, account


def account_from_token(access: str) -> str:
    """从 accessToken 的 JWT payload 里取 chatgpt_account_id。"""
    try:
        payload = access.split(".")[1]
        payload += "=" * (-len(payload) % 4)
        claims = json.loads(base64.urlsafe_b64decode(payload))
        auth = claims.get("https://api.openai.com/auth", {})
        return str(auth.get("chatgpt_account_id") or "")
    except Exception:
        return ""


def headers(access: str) -> dict:
    return {
        "Authorization": "Bearer " + access,
        "Accept": "*/*",
        "User-Agent": UA,
        "Referer": "https://chatgpt.com/",
        "Origin": "https://chatgpt.com",
    }


def list_cards(access: str, account: str):
    return requests.get(f"{BASE}/payment_methods",
                        params={"account_id": account},
                        headers=headers(access), timeout=30)


def delete_card(access: str, account: str, pm: str):
    return requests.delete(f"{BASE}/payment_method/{pm}",
                           params={"account_id": account},
                           headers=headers(access), timeout=30)


def describe(card: dict) -> str:
    c = card.get("card") or {}
    brand = c.get("brand") or card.get("brand") or "?"
    last4 = c.get("last4") or card.get("last4") or "????"
    exp = ""
    if c.get("exp_month") and c.get("exp_year"):
        exp = f" {c['exp_month']:0>2}/{str(c['exp_year'])[-2:]}"
    return f"{brand} ****{last4}{exp}"


def main() -> None:
    ap = argparse.ArgumentParser(description="删除 ChatGPT 已绑定支付卡片")
    ap.add_argument("session", help="session JSON 文件 / JSON 字符串 / accessToken")
    ap.add_argument("--account-id", default="", help="手动指定 account_id(覆盖自动解析)")
    ap.add_argument("--list", action="store_true", help="只列出, 不删除")
    ap.add_argument("--pm", default="", help="删除指定 pm_xxx")
    ap.add_argument("--all", action="store_true", help="删除全部卡片")
    ap.add_argument("--yes", action="store_true", help="跳过确认")
    args = ap.parse_args()

    access, account = load_session(args.session)
    if args.account_id:
        account = args.account_id
    if not account:
        sys.exit("无法确定 account_id, 请用 --account-id 指定")
    print(f"account_id: {account}")

    r = list_cards(access, account)
    ctype = r.headers.get("content-type", "")
    if r.status_code != 200 or "json" not in ctype:
        hint = "(疑似 Cloudflare 拦截或 token 失效)" if "json" not in ctype else ""
        sys.exit(f"列卡失败 HTTP {r.status_code} {hint}: {r.text[:200]}")

    data = r.json()
    cards = data.get("payment_methods", []) or []
    print(f"已绑卡片数: {len(cards)}"
          + (f" | 默认: {data.get('default_payment_method_id')}" if data.get("default_payment_method_id") else ""))
    for c in cards:
        print(f"  - {c.get('id')}  {describe(c)}")

    if args.list or not cards:
        return

    if args.pm:
        targets = [args.pm]
    else:  # 默认或 --all 都是删全部
        targets = [c.get("id") for c in cards if c.get("id")]

    if not args.yes:
        ans = input(f"确认删除 {len(targets)} 张卡片? [y/N] ").strip().lower()
        if ans != "y":
            print("已取消")
            return

    ok_count = 0
    for pm in targets:
        dr = delete_card(access, account, pm)
        ok = False
        try:
            ok = dr.status_code == 200 and dr.json().get("success") is True
        except Exception:
            pass
        print(f"删除 {pm}: HTTP {dr.status_code} " + ("✓" if ok else "✗ " + dr.text[:120]))
        ok_count += 1 if ok else 0
    print(f"完成: {ok_count}/{len(targets)} 成功")


if __name__ == "__main__":
    main()
