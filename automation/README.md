# checkout-autofill — 支付页自动填表（Playwright/Chrome）

帮你省去手动填卡号 + 地址的工作。用 session 登录 ChatGPT → 生成「菲律宾 20x」托管支付链接 → 打开 `pay.openai.com` → 自动填**随机测试卡号** + **免税州地址**。**不自动提交**（除非加 `--submit`），遇到 3DS/验证码浏览器保持打开，由你人工完成。

## 安装（首次）

```bash
cd automation
npm install
npx playwright install chromium   # 或装了本机 Chrome 可跳过，脚本会优先用 Chrome
```

## 用法

```bash
# 用某个任务/账号的 session（含 accessToken + sessionToken）整条跑通
node fill-checkout.js --session ../checkout-link-generator/session --plan ph-20x

# 只测“填表”这一步：直接给一个已生成的支付链接
node fill-checkout.js --pay-url "https://pay.openai.com/c/pay/cs_live_..."
```

### 参数

| 参数 | 说明 | 默认 |
|---|---|---|
| `--session <文件>` | session JSON 文件（含 accessToken/sessionToken） | — |
| `--plan <ph-20x\|eg-5x>` | 套餐：菲律宾20x(chatgptpro) / 埃及5x(chatgptprolite) | `ph-20x` |
| `--pay-url <url>` | 跳过登录+生成，直接打开该支付页填表 | — |
| `--address-source <local\|usaddressgen>` | 免税地址来源：本地生成 / 去 usaddressgen 抓取（失败回退本地） | `local` |
| `--card <卡号>` | 指定卡号（默认随机 Luhn 合法测试卡） | 随机 |
| `--country <US>` | 账单国家 | 跟随地址(US) |
| `--submit` | 自动点“支付”（之后 3DS 人工处理） | 关 |
| `--headless` | 无头模式（默认有头，方便你接管） | 关 |

## 说明 / 现实限制

- **测试阶段**：卡号是随机 Luhn 合法号，live 支付会被拒——此阶段只为验证“能否自动填进表单”。后期把 `lib/card.js` 的 `randomCard()` 换成 cardplatform 取卡即可。
- **3D Secure / 验证码无法自动绕过**：触发后请在保持打开的浏览器里人工完成。
- **地址**：默认本地生成美国无销售税州（DE/OR/NH/MT/AK）地址，稳定可靠；`--address-source usaddressgen` 会去 https://usaddressgen.com/tax-free-address/ 点按钮抓取，解析失败自动回退本地。
- **选择器**：针对 Stripe 托管 Checkout 常见字段 id/name/autocomplete，跨主页面与 iframe 兜底匹配；若某字段没填上会在结果里标 `✗`，人工补即可。

## 后续接 cardplatform

把 `lib/card.js` 换成从 cardplatform 取一张真实虚拟卡（卡号/有效期/CVC/账单地址），其余流程不变。
