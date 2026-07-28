# SpaceX Card · CDK 卡密系統接入文檔

CDK(激活碼/卡密)讓你把 GPT 直充做成「一次性兌換碼」生意:你在卡台**買 CDK**(扣服務費、授權由你名下資金承擔開卡),拿到一次性碼 → 分發/售賣 → 買家在**任意獨立前端**輸入碼兌換 → 兌換時自動**消耗你的賬戶餘額、用你名下的卡**開通訂閱。前端可換多套主題、獨立分發,後端始終指向卡台。

- **兌換接口 Base**:`https://spacexcard.com/api/v1/cdk`(公開,無需登錄,憑有效碼兌換)
- **發碼/管理**:網頁「開發者 · 隱藏 GPT 直充」頁,或 Open API `https://spacexcard.com/openapi/v1`
- **計費**:發碼時按你的賬戶餘額扣服務費;兌換時的開卡/充值/訂閱實付由你(CDK 所有者)名下資金承擔,受發碼時授權的資金上限約束。

> 需先在網頁端用 USDT 充值開通功能後,方可購買 CDK、查看並下載本文檔。

---

## 1. 名詞與資金模型

| 概念 | 說明 |
| --- | --- |
| CDK 所有者 | 購買/發放 CDK 的賬戶。兌換消耗的就是**所有者的餘額**,自動用**所有者名下的卡**開卡。 |
| 服務費 | 發碼時一次性從所有者餘額扣除(各套餐不同,見開發者頁費率帶)。 |
| 資金上限 | 發碼時按套餐估算的授權上限(`owner_funding_cap_minor`),單次兌換實付不得超過它。 |
| 一次性碼 | 完整碼只在**發碼響應**返回一次,請務必保存;之後列表只顯示碼前綴。 |

套餐:`plus` / `pro_5x` / `pro_20x`。

---

## 2. 公開兌換流程(獨立前端調用)

四步,全部無需登錄。兌換結果**綁定兌換時的設備與 IP**,只能在原設備查詢。

請求可帶 `X-Redemption-Device` 頭(自定義設備標識);未帶則用 `User-Agent`。

### 2.1 預覽 `POST /api/v1/cdk/preview`

```json
{ "code": "SXC-XXXX-XXXX-XXXX-XXXX" }
```

返回套餐、可兌換狀態與一個 `redemption_token`(後續步驟用)。碼無效/已用/已凍結/已刪除時統一返回「CDK 無效或不可用」。

### 2.2 預檢 `POST /api/v1/cdk/preflight`

校驗買家的 ChatGPT 憑證(session 或郵箱),返回 `preflight_token`。

```json
{
  "redemption_token": "<第 2.1 步返回>",
  "credential": { "mode": "session", "session": "<ChatGPT session token>" }
}
```

或郵箱模式(僅支持 mail.tm / outlook):

```json
{
  "redemption_token": "<...>",
  "credential": { "mode": "mailbox", "email": "user@outlook.com", "password": "..." }
}
```

> session 獲取:登錄 ChatGPT 後訪問 `https://chatgpt.com/api/auth/session`,複製返回 JSON 里的 access token。

### 2.3 兌換 `POST /api/v1/cdk/redeem`

```json
{
  "redemption_token": "<...>",
  "preflight_token": "<第 2.2 步返回>",
  "client_request_id": "<你生成的唯一 ID,冪等用>"
}
```

受理後返回訂單初始狀態。**結果不確定或 review 狀態時嚴禁重復提交**,改用下一步查詢。

### 2.4 查結果 `GET /api/v1/cdk/result?token=<redemption_token>`

輪詢到終態為止。狀態:

| 狀態 | 含義 |
| --- | --- |
| `queued` / `running` | 開通中,請稍候 |
| `review` / `pending` | 支付結果待對賬,**不得重試** |
| `completed` | 已開通 |
| `declined` / `failed_precharge` / `cancelled` | 未成功,可重試或聯繫發碼方 |

---

## 3. 獨立/多主題前端接入

兌換前端是**純前端**,只調用上面 4 個接口,可任意換皮、獨立部署分發(不部署在卡台)。參考倉庫內 `cdk-standalone/index.html`(單文件,零依賴)。

- **配置後端地址**:頁面讀取 `?api=<base>`、`localStorage.cdk_api_base` 或內置 `CONFIGURED_BASE`,指向 `https://spacexcard.com`。
- **跨域**:`/api/v1/cdk/*` 已對任意來源放行(無 cookie、憑碼兌換),你的前端可托管在任意域名。
- **換主題**:色板/字體集中在 `:root` 與頂部 CSS 段,複製一份改樣式即成新主題。

---

## 4. Open API 程序化發碼

用 API Key 自助批量發碼(從你自己的餘額扣服務費),便於對接自有商城/發貨系統。鑒權與「開放 API」一致(`X-API-Key: sk_xxx`),建議帶 `Idempotency-Key` 防重復扣費。

### 4.1 發碼 `POST /openapi/v1/gpt-direct/cdks`

```json
{ "plan": "plus", "count": 1, "funding_confirmed": true }
```

- `funding_confirmed` 必須為 `true`,表示你承擔該 CDK 兌換時的開卡/充值/訂閱實付。
- `count` 可選(默認 1,單次≤50)。逐張獨立扣費入庫;`Idempotency-Key` 相同的重試不會重復扣。

返回(明文碼**僅此一次**):

```json
{
  "code": 0, "msg": "ok",
  "data": { "requested": 1, "issued": [
    { "id": 123, "code": "SXC-XXXX-XXXX-XXXX-XXXX", "plan": "plus", "code_prefix": "SXC-XXXX-XXXX", "fee_amount_minor": 100 }
  ] }
}
```

### 4.2 列碼 `GET /openapi/v1/gpt-direct/cdks?page=1&page_size=20`

返回你名下 CDK 列表(只含碼前綴與狀態,不含完整碼)。

---

## 5. CDK 管理(網頁端)

在「開發者 · 隱藏 GPT 直充 → 我的 CDK」可對**未使用**的 CDK:

- **凍結 / 解凍**:凍結後暫不可兌換,可隨時解凍恢復。可逆。
- **刪除並退款**:刪除後卡密**永久失效**;若是購買所得(非贈送),**已付服務費自動退回餘額**。已使用/已預留/待對賬的卡不可刪除。

退款冪等、單事務原子:同一張卡只退一次,絕不重復退款。

---

## 6. 錯誤與狀態碼

| HTTP | 含義 |
| --- | --- |
| 400 | 參數錯誤 / 碼無效不可用 / 預檢或兌換被拒 |
| 401 | API Key 鑒權失敗 |
| 403 | 未開啓充值 / 無 GPT 直充權限 / IP 不在白名單 |
| 404 | 兌換結果不存在(token 無效或非原設備查詢) |
| 409 | 狀態衝突(如對已使用的卡執行管理操作) |

面向買家的報錯只返回通用文案,不暴露上游細節。
