# SpaceX Card · CDK 卡密系统接入文档

CDK(激活码/卡密)让你把 GPT 直充做成「一次性兑换码」生意:你在卡台**买 CDK**(扣服务费、授权由你名下资金承担开卡),拿到一次性码 → 分发/售卖 → 买家在**任意独立前端**输入码兑换 → 兑换时自动**消耗你的账户余额、用你名下的卡**开通订阅。前端可换多套主题、独立分发,后端始终指向卡台。

- **兑换接口 Base**:`https://spacexcard.com/api/v1/cdk`(公开,无需登录,凭有效码兑换)
- **发码/管理**:网页「开发者 · 隐藏 GPT 直充」页,或 Open API `https://spacexcard.com/openapi/v1`
- **计费**:发码时按你的账户余额扣服务费;兑换时的开卡/充值/订阅实付由你(CDK 所有者)名下资金承担,受发码时授权的资金上限约束。

> 需先在网页端用 USDT 充值开通功能后,方可购买 CDK、查看并下载本文档。

---

## 1. 名词与资金模型

| 概念 | 说明 |
| --- | --- |
| CDK 所有者 | 购买/发放 CDK 的账户。兑换消耗的就是**所有者的余额**,自动用**所有者名下的卡**开卡。 |
| 服务费 | 发码时一次性从所有者余额扣除(各套餐不同,见开发者页费率带)。 |
| 资金上限 | 发码时按套餐估算的授权上限(`owner_funding_cap_minor`),单次兑换实付不得超过它。 |
| 一次性码 | 完整码只在**发码响应**返回一次,请务必保存;之后列表只显示码前缀。 |

套餐:`plus` / `pro_5x` / `pro_20x`。

---

## 2. 公开兑换流程(独立前端调用)

四步,全部无需登录。兑换结果**绑定兑换时的设备与 IP**,只能在原设备查询。

请求可带 `X-Redemption-Device` 头(自定义设备标识);未带则用 `User-Agent`。

### 2.1 预览 `POST /api/v1/cdk/preview`

```json
{ "code": "SXC-XXXX-XXXX-XXXX-XXXX" }
```

返回套餐、可兑换状态与一个 `redemption_token`(后续步骤用)。码无效/已用/已冻结/已删除时统一返回「CDK 无效或不可用」。

### 2.2 预检 `POST /api/v1/cdk/preflight`

校验买家的 ChatGPT 凭证(session 或邮箱),返回 `preflight_token`。

```json
{
  "redemption_token": "<第 2.1 步返回>",
  "credential": { "mode": "session", "session": "<ChatGPT session token>" }
}
```

或邮箱模式(仅支持 mail.tm / outlook):

```json
{
  "redemption_token": "<...>",
  "credential": { "mode": "mailbox", "email": "user@outlook.com", "password": "..." }
}
```

> session 获取:登录 ChatGPT 后访问 `https://chatgpt.com/api/auth/session`,复制返回 JSON 里的 access token。

### 2.3 兑换 `POST /api/v1/cdk/redeem`

```json
{
  "redemption_token": "<...>",
  "preflight_token": "<第 2.2 步返回>",
  "client_request_id": "<你生成的唯一 ID,幂等用>"
}
```

受理后返回订单初始状态。**结果不确定或 review 状态时严禁重复提交**,改用下一步查询。

### 2.4 查结果 `GET /api/v1/cdk/result?token=<redemption_token>`

轮询到终态为止。状态:

| 状态 | 含义 |
| --- | --- |
| `queued` / `running` | 开通中,请稍候 |
| `review` / `pending` | 支付结果待对账,**不得重试** |
| `completed` | 已开通 |
| `declined` / `failed_precharge` / `cancelled` | 未成功,可重试或联系发码方 |

---

## 3. 独立/多主题前端接入

兑换前端是**纯前端**,只调用上面 4 个接口,可任意换皮、独立部署分发(不部署在卡台)。参考仓库内 `cdk-standalone/index.html`(单文件,零依赖)。

- **配置后端地址**:页面读取 `?api=<base>`、`localStorage.cdk_api_base` 或内置 `CONFIGURED_BASE`,指向 `https://spacexcard.com`。
- **跨域**:`/api/v1/cdk/*` 已对任意来源放行(无 cookie、凭码兑换),你的前端可托管在任意域名。
- **换主题**:色板/字体集中在 `:root` 与顶部 CSS 段,复制一份改样式即成新主题。

---

## 4. Open API 程序化发码

用 API Key 自助批量发码(从你自己的余额扣服务费),便于对接自有商城/发货系统。鉴权与「开放 API」一致(`X-API-Key: sk_xxx`),建议带 `Idempotency-Key` 防重复扣费。

### 4.1 发码 `POST /openapi/v1/gpt-direct/cdks`

```json
{ "plan": "plus", "count": 1, "funding_confirmed": true }
```

- `funding_confirmed` 必须为 `true`,表示你承担该 CDK 兑换时的开卡/充值/订阅实付。
- `count` 可选(默认 1,单次≤50)。逐张独立扣费入库;`Idempotency-Key` 相同的重试不会重复扣。

返回(明文码**仅此一次**):

```json
{
  "code": 0, "msg": "ok",
  "data": { "requested": 1, "issued": [
    { "id": 123, "code": "SXC-XXXX-XXXX-XXXX-XXXX", "plan": "plus", "code_prefix": "SXC-XXXX-XXXX", "fee_amount_minor": 100 }
  ] }
}
```

### 4.2 列码 `GET /openapi/v1/gpt-direct/cdks?page=1&page_size=20`

返回你名下 CDK 列表(只含码前缀与状态,不含完整码)。

可选查询参数:

| 参数 | 说明 |
| --- | --- |
| `page` / `page_size` | 分页；`page_size` 1–100，默认 20 |
| `status` | `unused` / `reserved` / `consumed` / `frozen` / `disabled` 等 |
| `plan` | `plus` / `pro_5x` / `pro_20x` |
| `q` | 模糊：CDK id 或 `code_prefix` 子串（可搜前缀片段） |

示例:`GET /openapi/v1/gpt-direct/cdks?page=1&page_size=50&status=unused&q=GPTD-AB12`

**兑换选卡**:卡台运营配置默认渠道/卡头(如渠道1 + G5554LC)。先看渠道是否开启,关则换渠道;渠道开则优先指定卡头,停用后用同渠道其它卡头;无卡则开卡。发码跟随平台默认,无需在 Open API 传参。

---

## 5. Webhook 与订单对账

白标服务应以 Webhook 作为订单同步主路径,`cdk-orders` 只用于单笔补漏或低频对账。

### 5.1 配置发码 Key 的 Webhook

在「开发者 → 我的 API 密钥」中点击对应 Key 的 **Webhook**,填写公网 HTTPS 地址并订阅:

```text
gpt_direct.*
cdk.*
```

也可通过网页登录态调用:

```http
GET /api/v1/dev/keys/{key_id}/webhook
PUT /api/v1/dev/keys/{key_id}/webhook
```

平台会推送:

- `gpt_direct.completed` / `failed` / `cancelled`:订单全终态;
- `gpt_direct.progress`:仅在 `status` 或 `stage` 变化时推送,每单最多 30 条;
- `gpt_direct.event`:公开时间线增量;
- `cdk.reserved` / `consumed` / `released` / `frozen` / `unfrozen` / `disabled`:码生命周期。

请求头 `X-Signature = hex(HMAC-SHA256(webhook_secret, 原始请求体))`。接收端应先验签,再按稳定 `event_id` 幂等处理并尽快返回 `2xx`;失败最多共投递 3 次。

回调只发给签发该 CDK 的 API Key / App ID。账号、卡号均脱敏,永不返回完整 CDK、完整卡号、凭据、API Key 或代理信息。

### 5.2 对账接口

```http
GET /openapi/v1/gpt-direct/cdk-orders?page=1&page_size=20
GET /openapi/v1/gpt-direct/cdk-orders/{order_id}
```

列表可按 `updated_after`(RFC3339)、`status`、`cdk_id`、`order_id` 筛选。返回 CDK 前缀/状态、脱敏账号与卡号、订单阶段、实付/报价、服务费和资金状态、时间字段;详情另含公开 `events`。当前 Key 不能读取同一用户其他 Key 签发的 CDK 订单。

---

## 6. CDK 管理(网页端)

在「开发者 · 隐藏 GPT 直充 → 我的 CDK」可对**未使用**的 CDK:

- **冻结 / 解冻**:冻结后暂不可兑换,可随时解冻恢复。可逆。
- **删除并退款**:删除后卡密**永久失效**;若是购买所得(非赠送),**已付服务费自动退回余额**。已使用/已预留/待对账的卡不可删除。

退款幂等、单事务原子:同一张卡只退一次,绝不重复退款。

---

## 7. 错误与状态码

| HTTP | 含义 |
| --- | --- |
| 400 | 参数错误 / 码无效不可用 / 预检或兑换被拒 |
| 401 | API Key 鉴权失败 |
| 403 | 未开启充值 / 无 GPT 直充权限 / IP 不在白名单 |
| 404 | 兑换结果不存在(token 无效或非原设备查询) |
| 409 | 状态冲突(如对已使用的卡执行管理操作) |

面向买家的报错只返回通用文案,不暴露上游细节。
