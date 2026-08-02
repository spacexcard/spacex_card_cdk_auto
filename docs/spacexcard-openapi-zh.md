# SpaceX Card 開放 API 文檔

通過開放 API 程序化完成開卡、查卡、卡充值、退款、凍結、消費查詢等操作。所有調用按你的賬戶餘額與專屬費率計費（與網頁端一致）。

- **生產 Base URL**：`https://spacexcard.com/openapi/v1`
- **沙盒 Base URL**：`https://sandbox.spacexcard.com/openapi/v1`
- **數據格式**：請求與響應均為 `application/json`，UTF-8
- **憑證獲取**：登錄後在「開發者」頁生成 `app_id` / `app_secret`

### 代理接入前測試

新代理應先登錄 `https://sandbox.spacexcard.com`，在「開發者」頁建立一套僅用於沙盒的 API 密鑰，再用沙盒 Base URL 完成聯調。沙盒賬戶、密鑰和數據與生產完全隔離；開卡、充值、退款及交易均為模擬結果，不會請求真實發卡渠道或產生真實資金變動。

```bash
export SPACEXCARD_API_BASE='https://sandbox.spacexcard.com/openapi/v1'
export SPACEXCARD_API_KEY='sk_沙盒密鑰'

curl "$SPACEXCARD_API_BASE/products" \
  -H "X-API-Key: $SPACEXCARD_API_KEY"
curl "$SPACEXCARD_API_BASE/balance" \
  -H "X-API-Key: $SPACEXCARD_API_KEY"
```

沙盒驗證通過後，只需改用生產 Base URL 和單獨建立的生產密鑰。不要在沙盒中使用生產密鑰，也不要把沙盒返回的卡片或交易當作真實資產。

## 0. 當前賬戶與卡片約束

- 鏈上充值單筆最低 `50U`（USDT/USDC）；共享 TRC20 意向低於 `50U` 無法創建，專屬地址收到低於 `50U` 的轉賬不會自動入賬，會進入人工處理。
- 普通賬戶全賬戶共同保留一筆 `20U` 風險保證金，不按卡片或交易重復扣取。該金額仍包含在 `balance` 中，不產生單獨扣款流水；`GET /balance` 同時返回 `balance`、`spendable_balance` 和 `account_reserve_amount`，開卡、卡充值、批量開卡、轉賬、商城和 GPT 直充等用戶主動消費只能使用 `spendable_balance`。
- 子賬戶不單獨保留 `20U`；主賬戶及其全部子賬戶共享主賬戶的保證金、拒付率、退款率、單日拒付計數和白名單資格。子賬戶風險手續費優先扣子賬戶餘額，不足部分由主賬戶代扣，並分別記錄資金流水；主賬戶風險保證金可用於核銷該類手續費。子賬戶的 `account_reserve_amount` 和 `spendable_balance` 按其自身餘額返回，不代表另行佔用一筆保證金。
- 拒付費、授權撤銷費、消費退款費、小額消費費和受限商戶處理費屬於平颱風險費用，可以核銷保證金；調用方不要用總餘額自行判斷用戶操作是否可執行。
- 拒付規則：非白名單真實拒付前 3 筆免費，第 4 筆起基礎 `$0.30`，比例檔位附加 `$0.30/$0.50/$0.80`；所有賬戶授權撤銷固定按金額 `1%` 收取，且不參與當前拒付/退款風控；消費退款金額低於 `1U` 時免收退款手續費，達到 `1U` 後繼續執行比例與最低 `$0.50` 規則；Google 商戶低於 `1U` 的消費驗證退款不計入退款筆數、退款率分母或退款率，但保留完整流水；小額真實消費（`0 < amount < 0.50U`）每筆 `$0.30`。
- 產品和卡詳情返回 `restricted_merchants` 有效禁用商戶陣列，以及兼容字段 `google_chatgpt_blocked`。未配置卡頭時，非香港發行卡默認禁用 `GOOGLE CHATGPT`，香港發行卡默認無禁用商戶；產品管理可按前 6 位卡頭配置多個商戶，顯式空陣列表示全部放行。命中任一規則的真實授權/清算時，第一次凍結卡，第二次刪卡、退回卡內餘額並收取 `$0.50`。
- 提現若觸及最後 `20U`，屬於保證金退出，必須刪除全部卡片並等待 `30` 天觀察期；30 天前管理端打款接口也會拒絕，不能只依賴前端按鈕狀態。

---

## 1. 接入流程

1. 在「開發者」頁生成密鑰，得到 `app_id`（`ak_` 開頭，公開標識）與 `app_secret`（`sk_` 開頭，請求鑒權用，務必保密）。
2. （可選）為密鑰設置 IP 白名單、配置回調地址。
3. 調 `GET /products` 獲取可開卡產品 → `POST /cards/open` 開卡（建議帶冪等鍵）。
4. 用 `GET /cards/{id}` 取卡號/有效期/CVV，`GET /cards/{id}/transactions` 查消費，或配置 Webhook 實時接收卡事件。
5. `GET /balance` 查餘額，`GET /balance-logs` 對賬。

> 餘額不足會導致開卡/充值失敗。請先在網頁端用 USDT 充值（支持 TRON / Ethereum / BNB Chain / X Layer）。

---

## 2. 鑒權

每個請求在 Header 攜帶 `app_secret`（`sk_` 開頭），二選一：

```
X-API-Key: sk_xxxxxxxxxxxxxxxx
```
或
```
Authorization: Bearer sk_xxxxxxxxxxxxxxxx
```

可選：再帶 `X-App-Id: ak_xxxx` 做雙重校驗。

- 密鑰可在開發者頁 **啓用 / 停用**、設置 **IP 白名單**（僅允許指定 IP 調用）。
- 鑒權失敗返回 `401`；IP 不在白名單返回 `403`。

---

## 3. 請求與響應格式

所有響應為 HTTP `200`，業務結果看 body 中的 `code`：

```jsonc
// 成功
{ "code": 0, "msg": "ok", "data": <結果> }
// 失敗
{ "code": 400, "msg": "錯誤說明" }
```

| HTTP 狀態 | 含義 |
|------|------|
| 200 | 請求已處理（以 body 內 `code` 為準，`0` = 業務成功）|
| 400 | 參數錯誤 / 業務失敗（餘額不足、卡不存在等，詳見 `msg`）|
| 401 | 密鑰無效 / 已停用 |
| 403 | IP 不在白名單 |
| 429 | 請求過於頻繁，請退避後重試 |
| 500 | 服務端錯誤 |
| 503 | 渠道暫時不可用（發卡渠道上游故障，已臨時熔斷）|

**常見業務錯誤（`msg` 文案）**：餘額不足、卡產品不存在或已下架、最低開卡金額限制、卡不存在、卡狀態異常無法操作、退款金額超出卡內餘額、退款後卡內餘額不得少於 $1 等。

### 渠道暫時不可用（可編程區分）

當某發卡渠道上游故障時，系統會探活確認後**臨時熔斷**該渠道，開卡 / 充值接口返回 **HTTP `503`**，body 帶穩定的 `error_code`：

```jsonc
{ "code": 503, "error_code": "channel_unavailable", "msg": "渠道暫時不可用，請稍後再試或選擇其他渠道" }
```

- 請按 `error_code == "channel_unavailable"` 判定（勿匹配 `msg` 文案），據此**稍後重試**或**改用其他渠道的產品**（不同 `issuer`）。
- 渠道恢復後自動放開，無需人工介入；該狀態只影響對應渠道，其他渠道正常。
- 此為暫時性故障，非扣費失敗——熔斷攔截發生在扣費之前，不會產生扣款。


---

## 4. 冪等

開卡、充值、退款等寫操作，帶一個唯一的 `Idempotency-Key` 頭：

```
Idempotency-Key: 你的唯一訂單號
```

同一密鑰下、相同 `Idempotency-Key` 的請求只會真正執行一次；重試會**原樣返回首次結果**（響應頭帶 `Idempotent-Replayed: true`），可避免網絡重試導致**重復開卡 / 重復扣費**。

---

## 5. 數據字典

**卡狀態 `status`**

| 狀態 | 說明 |
| --- | --- |
| ACTIVE | 激活（正常使用）|
| FROZEN | 已凍結 |
| CANCELLED | 已注銷 |
| DELETED | 已刪除 |

**交易類型 `type`**

| 類型 | 說明 |
| --- | --- |
| Authorization | 消費授權 |
| Settlement | 清算 |
| Refund | 消費退款 |
| Reversal | 授權撤銷 |

**交易狀態 `status`**

| 狀態 | 說明 |
| --- | --- |
| PENDING | 清算中 |
| COMPLETE | 清算完成 |
| DECLINED | 交易失敗（失敗原因見 `description`）|

---

## 6. 接口詳解

### 6.1 獲取可開卡產品

`GET /products`

返回當前可開卡的產品列表，**含你的專屬開卡費/退款率**。

**響應字段（`data` 數組項）**

| 字段 | 類型 | 說明 |
| --- | --- | --- |
| product_code | string | 產品碼（開卡用）|
| network | string | 卡組織：VISA / MasterCard |
| issuing_area | string | 發行區域 |
| card_type | string | 卡類型：save=儲值卡 |
| open_fee | number | 開卡費（你的專屬價）|
| recharge_fee | number | 卡充值手續費率 |
| rtf_rate | number | 消費退款手續費率 |
| min_amount | number | 產品最低開卡/卡充值金額（鏈上賬戶充值另受 50U 平台下限約束）|
| max_amount | number | 最高金額 |
| restricted_merchants | string[] | 當前卡頭禁用商戶規則；空陣列表示不禁用商戶 |
| google_chatgpt_blocked | boolean | 兼容字段；有效規則會命中 `GOOGLE CHATGPT` 時為 true |

**請求**
```bash
curl https://spacexcard.com/openapi/v1/products -H "X-API-Key: sk_你的密鑰"
```

**響應**
```json
{
  "code": 0,
  "msg": "ok",
  "data": [
    {
      "product_code": "P5378OX",
      "issuer": "one",
      "network": "MasterCard",
      "issuing_area": "United States",
      "card_type": "save",
      "open_fee": 1.5,
      "recharge_fee": 0,
      "rtf_rate": 0.1,
      "min_amount": 10,
      "max_amount": 10000,
      "restricted_merchants": ["GOOGLE CHATGPT"],
      "google_chatgpt_blocked": true
    }
  ]
}
```

> `issuer` 為發卡渠道：`one` / `two`。兩渠道卡段、發行地、費率可能不同，開卡時用對應 `product_code` 即可，無需關心底層差異；所有卡接口（充值/退款/凍結/刪卡/查詢）對兩渠道通用。

---

### 6.2 開卡

`POST /cards/open`

**請求參數**

| 參數 | 類型 | 必填 | 說明 |
| --- | --- | --- | --- |
| product_code | string | 是 | 產品碼 |
| first_name | string | 是 | 持卡人名 |
| last_name | string | 是 | 持卡人姓 |
| init_amount | number | 是 | 初始充值金額（≥ 產品最低金額）|

開卡將從賬戶可消費餘額扣除 **開卡費 + 初始充值金額**；賬戶必須保留 20U 風險保證金。產品或卡詳情中的 `restricted_merchants` 是該卡頭的有效禁用商戶列表。

**請求**
```bash
curl -X POST https://spacexcard.com/openapi/v1/cards/open \
  -H "X-API-Key: sk_你的密鑰" \
  -H "Idempotency-Key: order-20260604-001" \
  -H "Content-Type: application/json" \
  -d '{"product_code":"P5378OX","first_name":"John","last_name":"Doe","init_amount":20}'
```

**響應**
```json
{
  "code": 0,
  "msg": "ok",
  "data": {
    "id": 123,
    "user_id": 31,
    "issuer": "one",
    "vm_card_id": "card55202606040031562947331",
    "card_number": "5378727109708264",
    "cvv": "123",
    "expire": "08/29",
    "product_code": "P5378OX",
    "network": "MasterCard",
    "issuing_area": "United States",
    "available_amount": 20,
    "status": "ACTIVE",
    "open_fee": 1.5,
    "first_name": "John",
    "last_name": "Doe",
    "created_at": "2026-06-04T00:31:56Z"
  }
}
```

> 開卡響應即時返回 `cvv` / `expire`（有效期 MM/YY），請妥善保存；後續 `GET /cards/{id}` 也可再取。卡列表接口出於安全不返回 CVV。

---

### 6.3 批量開卡

`POST /cards/batch-open`

在 6.2 參數基礎上增加 `count`（開卡數量）。先預檢總餘額，再逐張開卡；單張失敗自動退款並繼續。

**請求**
```bash
curl -X POST https://spacexcard.com/openapi/v1/cards/batch-open \
  -H "X-API-Key: sk_你的密鑰" -H "Content-Type: application/json" \
  -d '{"product_code":"P5378OX","first_name":"John","last_name":"Doe","init_amount":20,"count":3}'
```

**響應**
```json
{
  "code": 0,
  "msg": "ok",
  "data": {
    "success": [ { "id": 124, "card_number": "5378727100000001", "status": "ACTIVE" } ],
    "failed":  [ { "index": 2, "error": "餘額不足" } ]
  }
}
```

---

### 6.4 我的卡列表

`GET /cards?page=1&page_size=20`

**請求參數**：`page`（頁碼，默認 1）、`page_size`（每頁數量，默認 20）、`q`（按卡號/備注模糊搜索，選填）、`sync`（傳 `1` 時實時同步當前頁各卡餘額/狀態後再返回，便於核對“卡里還有多少錢”；不傳則返回緩存值，更快）。

> `available_amount` 默認為緩存值（由 Webhook/消費回調更新，可能有滯後）。需要**實時餘額**時加 `&sync=1`，系統會逐卡向上游查詢當前頁餘額後返回（單卡 60 秒內最多同步一次，避免觸發上游限速）。

**請求**
```bash
# 實時餘額：加 sync=1（不加則返回更快的緩存值）
curl "https://spacexcard.com/openapi/v1/cards?page=1&page_size=20&sync=1" -H "X-API-Key: sk_你的密鑰"
```

**響應**
```json
{
  "code": 0,
  "msg": "ok",
  "data": {
    "total": 2,
    "list": [
      {
        "id": 123,
        "vm_card_id": "card55202606040031562947331",
        "card_number": "5378727109708264",
        "product_code": "P5378OX",
        "network": "MasterCard",
        "issuing_area": "United States",
        "available_amount": 18.8,
        "status": "ACTIVE",
        "first_name": "John",
        "last_name": "Doe",
        "created_at": "2026-06-04T00:31:56Z"
      }
    ]
  }
}
```

---

### 6.5 卡詳情（含卡號 / 有效期 / CVV）

`GET /cards/{id}`

`{id}` 為本地卡 ID。實時返回完整卡信息（含敏感字段）。

**響應字段（`data`）**

| 字段 | 類型 | 說明 |
| --- | --- | --- |
| card_id | string | 卡 ID |
| card_number | string | 完整卡號 |
| cvv | string | CVV 安全碼 |
| expire | string | 有效期 MM/YY |
| status | string | 卡狀態 |
| user_name | string | 持卡人姓名 |
| available_amount | number | 卡內可用餘額 |
| card_type | string | 卡類型 |
| first_name / last_name | string | 持卡人名 / 姓 |
| create_time | string | 開卡時間 |
| card_address | object | 賬單地址 |
| limit | object | 額度設置（額度卡有效）|

**請求**
```bash
curl https://spacexcard.com/openapi/v1/cards/123 -H "X-API-Key: sk_你的密鑰"
```

**響應**
```json
{
  "code": 0,
  "msg": "ok",
  "data": {
    "card_id": "card55202606040031562947331",
    "card_number": "5378727109708264",
    "cvv": "123",
    "expire": "08/29",
    "status": "ACTIVE",
    "user_name": "John Doe",
    "available_amount": 18.8,
    "card_type": "save",
    "first_name": "John",
    "last_name": "Doe",
    "create_time": "2026-06-04 00:31:56",
    "card_address": {
      "address_line_one": "",
      "address_line_two": "",
      "city": "",
      "state": "",
      "country": "",
      "post_code": ""
    }
  }
}
```

---

### 6.6 卡消費記錄

`GET /cards/{id}/transactions?page=1&page_size=50&sync=0`

**響應字段（`data` 數組項）**

| 字段 | 類型 | 說明 |
| --- | --- | --- |
| auth_id | string | 交易 ID |
| card_id | string | 上游卡 ID（渠道卡標識）|
| auth_time | string | 交易授權時間 |
| auth_amount | number | 授權金額 |
| auth_currency | string | 授權幣種 |
| settle_amount | number | 結算金額 |
| settle_currency | string | 結算幣種 |
| status | string | 交易狀態（見數據字典）|
| type | string | 交易類型（見數據字典）|
| merchant_name | string | 交易商戶 |
| merchant_amount | number | 商戶本幣金額；本地歷史投影可能為 0 |
| merchant_currency | string | 商戶本幣幣種；沒有時為空字符串 |
| create_time | string | 上游創建時間；本地歷史投影可能為空，實時 Webhook 才保證原始值 |
| description | string | 交易詳情 / 失敗原因 |

**響應**
```json
{
  "code": 0,
  "msg": "ok",
  "data": [
    {
      "auth_id": "1059958172",
      "card_id": "card55202606040031562947331",
      "auth_time": "2026-06-04 02:29:40",
      "auth_amount": 9.99,
      "auth_currency": "USD",
      "settle_amount": 9.99,
      "settle_currency": "USD",
      "status": "COMPLETE",
      "type": "Settlement",
      "merchant_name": "OPENAI",
      "merchant_amount": 9.99,
      "merchant_currency": "USD",
      "create_time": "",
      "description": ""
    },
    {
      "auth_id": "1059957962",
      "card_id": "card55202606040031562947331",
      "auth_time": "2026-06-04 02:20:34",
      "auth_amount": 5.00,
      "auth_currency": "USD",
      "settle_amount": 0,
      "settle_currency": "USD",
      "status": "DECLINED",
      "type": "Authorization",
      "merchant_name": "STEAM",
      "merchant_amount": 5.00,
      "merchant_currency": "USD",
      "create_time": "",
      "description": "Insufficient funds"
    }
  ]
}
```

> 另有 `GET /cards/all-transactions` 一次性聚合你名下所有卡的消費記錄（響應結構同上，每項額外帶 `card_number`、`local_card_id`）。

---

### 6.7 卡段 OpenAI 最新支付（三檔價位）

`GET /cards/{id}/openai-payments`

返回該卡**所屬卡段**（卡頭/BIN，渠道1/2 按產品、渠道3 按卡頭）在 **OpenAI** 商戶三個價位檔（Plus / 5x / 20x）各自「最新一筆」支付的金額與時間，作為該卡段的**最新行情參考價**（用於參考下單）。取卡段而非單卡：新卡/未刷過某檔的卡也能拿到該卡段的最新價。

**檔位（按結算金額 USD 區間）**

| tier | label | 金額區間 |
| --- | --- | --- |
| plus | Plus | $15 – $20 |
| x5 | 5x | $90 – $100 |
| x20 | 20x | $140 – $160 |

**響應字段（`data` 數組，固定 3 項，按上表順序）**

| 字段 | 類型 | 說明 |
| --- | --- | --- |
| tier | string | 檔位標識：plus / x5 / x20 |
| label | string | 檔位名：Plus / 5x / 20x |
| min_usd | number | 檔位下限（USD）|
| max_usd | number | 檔位上限（USD）|
| amount | number | 該卡段該檔最新一筆支付金額（USD），無則 0 |
| time | string | 該卡段該檔最新一筆授權時間，無則空字符串 |
| found | boolean | 該檔是否有匹配記錄 |

**響應**
```json
{
  "code": 0,
  "msg": "ok",
  "data": [
    { "tier": "plus", "label": "Plus", "min_usd": 15, "max_usd": 20, "amount": 16.24, "time": "2026-06-16 09:43:25", "found": true },
    { "tier": "x5", "label": "5x", "min_usd": 90, "max_usd": 100, "amount": 99.00, "time": "2026-06-15 20:11:03", "found": true },
    { "tier": "x20", "label": "20x", "min_usd": 140, "max_usd": 160, "amount": 150.00, "time": "2026-06-16 03:02:55", "found": true }
  ]
}
```

> 僅統計非拒付（成功 / 掛賬）的 OPENAI 商戶交易。某檔無記錄時 `found=false`、`amount=0`、`time=""`。

---

### 6.8 卡充值記錄

`GET /cards/{id}/recharges`

**響應**
```json
{
  "code": 0,
  "msg": "ok",
  "data": [
    {
      "id": 88,
      "user_id": 31,
      "card_id": 123,
      "vm_card_id": "card55202606040031562947331",
      "amount": 20,
      "fee": 0,
      "status": "success",
      "created_at": "2026-06-04T01:59:55Z"
    }
  ]
}
```

---

### 6.9 卡充值

`POST /cards/recharge`

**請求參數**

| 參數 | 類型 | 必填 | 說明 |
| --- | --- | --- | --- |
| card_id | number | 是 | 本地卡 ID |
| amount | number | 是 | 充值金額 |

從賬戶可消費餘額扣除 `金額 + 手續費`（手續費 = 金額 × 卡充值費率）；賬戶必須保留 20U 風險保證金。產品或卡詳情中的 `restricted_merchants` 是該卡頭的有效禁用商戶列表。

**請求**
```bash
curl -X POST https://spacexcard.com/openapi/v1/cards/recharge \
  -H "X-API-Key: sk_xxx" -H "Content-Type: application/json" \
  -d '{"card_id":123,"amount":50}'
```

**響應**
```json
{ "code": 0, "msg": "充值成功" }
```

---

### 6.10 卡退款（卡內餘額退回平台餘額）

`POST /cards/refund`

**請求參數**

| 參數 | 類型 | 必填 | 說明 |
| --- | --- | --- | --- |
| card_id | number | 是 | 本地卡 ID |
| amount | number | 是 | 退款金額（≤ 卡內可用餘額；退款後卡內餘額需 ≥ $1）|

主動從卡退回平台餘額**不收手續費**，全額到賬。

**請求**
```bash
curl -X POST https://spacexcard.com/openapi/v1/cards/refund \
  -H "X-API-Key: sk_xxx" -H "Content-Type: application/json" \
  -d '{"card_id":123,"amount":10}'
```

**響應**
```json
{ "code": 0, "msg": "退款成功，餘額已退回" }
```

---

### 6.11 凍結 / 解凍

`POST /cards/freeze`

**請求參數**

| 參數 | 類型 | 必填 | 說明 |
| --- | --- | --- | --- |
| card_id | number | 是 | 本地卡 ID |
| freeze | boolean | 是 | true=凍結，false=解凍 |

**請求**
```bash
curl -X POST https://spacexcard.com/openapi/v1/cards/freeze \
  -H "X-API-Key: sk_xxx" -H "Content-Type: application/json" \
  -d '{"card_id":123,"freeze":true}'
```

**響應**
```json
{ "code": 0, "msg": "ok" }
```

---

### 6.12 刪卡

`DELETE /cards/{id}`

永久刪除卡，卡內剩餘餘額**全額退回**平台餘額。

**請求**
```bash
curl -X DELETE https://spacexcard.com/openapi/v1/cards/123 -H "X-API-Key: sk_xxx"
```

**響應**
```json
{ "code": 0, "msg": "刪卡成功，餘額已退回" }
```

---

### 6.13 賬戶餘額

`GET /balance`

**請求**
```bash
curl https://spacexcard.com/openapi/v1/balance -H "X-API-Key: sk_你的密鑰"
```

**響應**
```json
{ "code": 0, "msg": "ok", "data": { "balance": 128.5, "spendable_balance": 108.5, "account_reserve_amount": 20, "account_reserve_enabled": true, "minimum_deposit_amount": 50, "currency": "USD" } }
```

`balance` 是賬面餘額，`spendable_balance` 是扣除持續鎖定保證金後的用戶可主動消費餘額。風險費用可能使賬面餘額低於 0；請始終以接口返回的業務錯誤和 `spendable_balance` 為準。

子賬戶的 `account_reserve_amount` 固定返回 `0`，因為保證金只保留在主賬戶；子賬戶仍與主賬戶共享風險主體。子賬戶風險手續費不足時，服務端會自動從主賬戶代扣，不要求客戶端自行拼接或轉移這筆費用。

---

### 6.14 賬戶流水（對賬）

`GET /balance-logs?page=1&page_size=20`

**響應字段（`data.list` 項）**：`created_at` 時間、`type` 類型（`recharge` 充值 / `open_card` 開卡 / `card_recharge` 卡充值 / `refund` 退款 / `decline_fee` 消費失敗費 / `reversal_fee` 授權撤銷費 / `refund_fee` 消費退款費 / `small_tx_fee` 小額消費費 / `merchant_violation_fee` 受限商戶處理費 / `admin` 調整等）、`amount` 金額（正增負減）、`before`/`after` 變動前後餘額、`remark` 備注。

**響應**
```json
{
  "code": 0,
  "msg": "ok",
  "data": {
    "total": 12,
    "list": [
      {
        "created_at": "2026-06-04T01:59:55Z",
        "type": "card_recharge",
        "amount": -20,
        "before": 38.8,
        "after": 18.8,
        "remark": "卡 5378727109708264 充值 $20.00 (手續費 $0.00)"
      }
    ]
  }
}
```

---

### 6.15 會員等級

`GET /vip`

返回當前賬戶的會員等級、累計數據與各檔達標門檻。會員按**累計充值**自動達標（也可後台手動授予），達標後享更低開卡費與充值費率，高等級含低等級全部權益。

**等級（tier）**

| tier | 名稱 | 達標（累計充值）| 權益（開卡費 / 充值費率 / 退款費）|
| --- | --- | --- | --- |
| `super` | 超級SVIP | ≥ $3,000 | $1 / 1% / 7% |
| `supreme` | 至尊SVIP | ≥ $20,000 | $0.5 / 1% / 5% |
| `legend` | 傳奇SVIP | ≥ $100,000 | $0.5 / 0.8% / 3% |
| `none` | 普通 | — | 產品默認 / 全局默認 / 10%（港卡 15%）|

**響應字段（`data`）**

| 字段 | 類型 | 說明 |
| --- | --- | --- |
| tier | string | `none` / `super` / `supreme` / `legend` |
| tier_name | string | 等級中文名 |
| is_svip | boolean | 是否超級SVIP 及以上 |
| is_supreme_svip | boolean | 是否至尊SVIP 及以上 |
| is_legend_svip | boolean | 是否傳奇SVIP |
| active_cards | number | 名下有效卡數（僅供參考，不作為達標門檻）|
| cumulative_recharge | number | 累計充值（USDT，已入賬）|
| recharge_fee_rate | number | 當前生效充值費率（如 0.01 = 1%）|
| thresholds.super / .supreme / .legend | object | 各檔門檻 `{cards, recharge}`；當前僅按 `recharge` 達標，`cards` 恆為 `0` |

**響應**
```json
{
  "code": 0,
  "msg": "ok",
  "data": {
    "tier": "super",
    "tier_name": "超級SVIP",
    "is_svip": true,
    "is_supreme_svip": false,
    "is_legend_svip": false,
    "active_cards": 63,
    "cumulative_recharge": 4200.50,
    "recharge_fee_rate": 0.01,
    "thresholds": {
      "super": { "cards": 0, "recharge": 3000 },
      "supreme": { "cards": 0, "recharge": 20000 },
      "legend": { "cards": 0, "recharge": 100000 }
    }
  }
}
```

---

### 6.16 GPT 直充 API

GPT 直充接口使用你名下的卡為 GPT 賬號開通或升級套餐。它與卡開通、卡充值是兩條獨立鏈路：

- **商城直付**：沒有額外的 API / CDK 服務費。
- **開放 API 訂單**：按套餐收取 API 服務費，創建訂單時凍結，成功後結算；確定未扣款的終態會釋放。
- **CDK**：購買或通過開放 API 發放 CDK 時預付服務費；CDK 兌換後的上游開卡、充值、訂閱實付資金由 CDK 所有者承擔。
- GPT 上游訂閱金額不是固定美元價。先調用預檢接口取得當前地區、幣種和賬號狀態下的實時報價。

#### 6.16.1 權限與服務費

開放 API 還需要有效的 `app_secret`。賬戶必須是正常的主賬戶、沒有卡片操作限制；除管理員外，還必須已有一筆已入賬充值，並滿足以下任一條件：

1. 管理員手工開通 GPT 直充；
2. 管理員賬戶；
3. 超級 SVIP、至尊 SVIP 或傳奇 SVIP。

VIP 達標後自動開放，不需要管理員再寫入 `gpt_direct_enabled`。子賬戶不能使用 GPT 直充。服務或上游不可用時，權限仍可顯示，但寫操作會返回相應錯誤。

| `plan` | 套餐 | API / CDK 服務費 | 上游訂閱價格 |
| --- | --- | ---: | --- |
| `plus` | Plus | **1 U** | 以預檢報價為準 |
| `pro_5x` | Pro 5x | **5 U** | 以預檢報價為準 |
| `pro_20x` | Pro 20x | **10 U** | 以預檢報價為準 |

服務費在價格配置中以美元最小單位返回：`100`、`500`、`1000`。以 `GET /gpt-direct/plans` 返回的 `version` 和 `enabled` 為準；套餐可能被管理員臨時暫停。價格配置版本變化後，創建訂單應重新預檢並帶最新 `pricing_version`。

#### 6.16.2 獲取套餐和實時配置

`GET /gpt-direct/plans`

**響應**
```json
{
  "code": 0,
  "msg": "ok",
  "data": {
    "version": 2,
    "plans": {
      "plus": {
        "key": "plus",
        "label": "Plus",
        "currency": "PHP",
        "enabled": true,
        "serviceFeeUsdMinor": 100,
        "expectedAmountMinor": 98214,
        "minAmountMinor": 90000,
        "maxAmountMinor": 110000
      },
      "pro_5x": {
        "key": "pro_5x",
        "label": "Pro 5x",
        "currency": "PHP",
        "enabled": true,
        "serviceFeeUsdMinor": 500
      },
      "pro_20x": {
        "key": "pro_20x",
        "label": "Pro 20x",
        "currency": "PHP",
        "enabled": true,
        "serviceFeeUsdMinor": 1000
      }
    }
  }
}
```

`expectedAmountMinor`、`minAmountMinor`、`maxAmountMinor` 是配置參考範圍，不是訂單最終扣款；訂單以預檢返回的 `quotes[plan]` 為準。

#### 6.16.3 憑據預檢

`POST /gpt-direct/preflight`

預檢會驗證 GPT 憑據、查詢當前套餐和支付方式，並取得實時報價。預檢憑證有效期 **10 分鐘且只能消費一次**。

**請求參數**

| 參數 | 類型 | 必填 | 說明 |
| --- | --- | --- | --- |
| `credential.mode` | string | 是 | `session` 或 `mailbox` |
| `credential.session` | string | session 模式 | GPT Session 值；不要提交 Access Token |
| `credential.email` | string | mailbox 模式 | 郵箱地址 |
| `credential.password` | string | mailbox 模式 | 郵箱密碼 |
| `payment_country` | string | 否 | 當前開放 API 默認 `PH` |
| `payment_currency` | string | 否 | 當前開放 API 默認 `PHP` |

```bash
curl -X POST https://spacexcard.com/openapi/v1/gpt-direct/preflight \
  -H "X-API-Key: sk_你的密鑰" \
  -H "Content-Type: application/json" \
  -d '{
    "credential": {"mode":"session","session":"你的_SESSION"},
    "payment_country":"PH",
    "payment_currency":"PHP"
  }'
```

**響應關鍵字段**
```json
{
  "code": 0,
  "msg": "ok",
  "data": {
    "email": "user@example.com",
    "currentPlan": "free",
    "credentialMode": "session",
    "preflight_token": "一次性預檢令牌",
    "preflight_expires_at": "2026-06-04T02:10:00Z",
    "payment_country": "PH",
    "payment_currency": "PHP",
    "quotes": {
      "plus": {
        "country": "PH",
        "currency": "PHP",
        "plan": "plus",
        "amountMinor": 98214,
        "amountMajor": 982.14,
        "minorUnitExponent": 2,
        "fetchedAt": 1780538400000
      }
    },
    "quote_error": ""
  }
}
```

`quote_error` 非空時不要創建對應套餐訂單。Session、郵箱密碼等敏感憑據不會在訂單響應中返回；服務端只保存短期加密副本。

#### 6.16.4 創建直充訂單

`POST /gpt-direct/orders`

建議先預檢，再創建訂單。開放 API 當前使用手動選擇的 `card_id`，該卡必須屬於當前 API 用戶並處於可用狀態。

**請求參數**

| 參數 | 類型 | 必填 | 說明 |
| --- | --- | --- | --- |
| `card_id` | number | 是 | 用於支付的名下卡 ID |
| `plan` | string | 是 | `plus` / `pro_5x` / `pro_20x` |
| `credential` | object | 是 | 與預檢相同的憑據結構；使用預檢令牌時以服務端預檢結果為準 |
| `preflight_token` | string | 否 | 預檢返回的一次性令牌；推薦使用 |
| `client_request_id` | string | 是 | 商戶側訂單號，當前用戶下最長 80 字符；重試必須保持不變 |
| `pricing_version` | integer | 否 | 預檢時看到的價格版本；不一致會要求刷新 |

```bash
curl -X POST https://spacexcard.com/openapi/v1/gpt-direct/orders \
  -H "X-API-Key: sk_你的密鑰" \
  -H "Idempotency-Key: merchant-order-20260718-001" \
  -H "Content-Type: application/json" \
  -d '{
    "card_id":123,
    "plan":"plus",
    "credential":{"mode":"session","session":"你的_SESSION"},
    "preflight_token":"一次性預檢令牌",
    "client_request_id":"merchant-order-20260718-001",
    "pricing_version":2
  }'
```

成功返回 HTTP `202`，訂單先進入安全隊列，不能把 `202` 當作已開通：

```json
{ "code": 0, "msg": "accepted", "data": {
  "id": 981,
  "plan": "plus",
  "source": "api",
  "status": "queued",
  "service_fee_minor": 100,
  "service_fee_status": "held",
  "pricing_version": 2,
  "quoted_amount_minor": 98214,
  "currency": "PHP",
  "client_request_id": "merchant-order-20260718-001"
} }
```

同一用戶的 `client_request_id` 會返回原訂單，不能用於創建第二筆訂單。建議同時攜帶唯一 `Idempotency-Key`，網絡重試時兩個編號都保持不變。

#### 6.16.5 查詢、詳情與取消

- `GET /gpt-direct/orders?page=1&page_size=20`：只返回當前 API 用戶創建的訂單；默認 20 條，最大 100 條。響應為 `data.list` 和 `data.total`。
- `GET /gpt-direct/orders/{id}`：返回 `data.order`、`data.events` 和可選的 `data.card_usage`。憑據、支付意圖和代理憑據不會返回。
- `POST /gpt-direct/orders/{id}/cancel`：只能取消尚未開始支付的 `queued`、`awaiting_card` 或 `funding_pending` 訂單。支付已提交、已扣款或進入人工對賬後返回 HTTP `409`，不要重復創建訂單。

常見訂單狀態：

| 狀態 | 含義 |
| --- | --- |
| `queued` / `running` | 已接收，正在檢查憑據或處理支付 |
| `pending` / `requires_action` | 上游仍在等待或需要後續對賬 |
| `completed` | 已確認目標套餐開通 |
| `declined` | 上游拒付，未必代表服務費已釋放前的最終狀態 |
| `failed_precharge` | 扣款前失敗；若未發生扣款，服務費會釋放 |
| `cancelled` | 在支付開始前取消 |

#### 6.16.6 發放和查詢 CDK

`POST /gpt-direct/cdks` 用當前 API 用戶餘額購買併發放 CDK。CDK 所有者必須確認自己承擔自動開卡、充值和訂閱的實付資金。

**請求**
```json
{ "plan": "pro_5x", "count": 1, "funding_confirmed": true }
```

`count` 默認 1，單次最多 50 張；每張 CDK 分別按 `1 / 5 / 10 U` 服務費扣款。明文 `code` 只在成功響應返回一次，請立即加密保存，不要寫入日誌。

`GET /gpt-direct/cdks?page=1&page_size=20` 返回當前用戶購買或持有的 CDK（`data.list`、`data.total`）。

可選：`page`、`page_size`（1–100）、`status`、`plan`、`q`（id 精確或 `code_prefix` 模糊）。示例：`?page=1&page_size=50&status=unused&q=GPTD-AB12`。

**兌換選卡（平台側）**：運營可配置默認渠道+卡頭；先判斷渠道是否開啟（關則換渠道），再優先指定卡頭；卡頭停用則用同渠道其餘卡頭；無卡則開卡。Open API 發碼跟隨平台默認偏好。

CDK 狀態包括 `unused`、`reserved`、`consumed`、`frozen`、`disabled`；訂單成功後從 `reserved` 變為 `consumed`。列表不返回完整明文碼。

#### 6.16.7 GPT 直充錯誤處理

除通用 HTTP 錯誤外，調用方應按 `error_code` 編程處理：

- `GPT_DIRECT_ACCESS_DENIED` / `RECHARGE_REQUIRED`：賬戶尚未滿足權限、充值或風控條件。
- `GPT_SESSION_INVALID`、`SESSION_REQUIRED`：重新提交憑據預檢。
- `GPT_PLAN_ALREADY_ACTIVE`：目標賬號套餐仍有效，不要重試扣款。
- `INSUFFICIENT_BALANCE`：API 服務費餘額不足。
- `GPT_DIRECT_ORDER_REJECTED`：讀取訂單或事件後決定是否重試；不要盲目新建訂單。

---

## 7. 回調 Webhook（卡事件與 GPT 直充完成）

配置回調地址後，你名下任意一張卡發生**授權 / 清算 / 退款 / 拒付 / 授權撤銷**等事件，或 GPT 直充訂單首次確認完成時，平台都會主動 **POST** 一條事件到該地址，無需輪詢即可對賬。

> 當前支持三類事件：卡交易使用 `event = card_transaction`；卡操作使用 `event = card_operation`；GPT 直充完成使用 `type = gpt_direct.completed`。上游提供開卡、充值、凍結/解凍、刪卡或卡餘額退款結果時，平台會在能安全解析卡主時轉發操作回調。
>
> 當前運行中的發卡渠道為渠道 1 和渠道 3；渠道 2 的 Webhook 已退役。兩條渠道的事件已在平台側**歸一化**為下面同一套結構，接收端無需區分渠道。

### 7.1 配置回調地址

在「開發者」頁填寫回調地址並保存：

- 地址必須是有效的 **https** URL（不接受 `http`、`localhost`、內網 / 私有 IP；長度 ≤ 256）。
- **首次**保存回調地址時，系統自動生成簽名密鑰 `webhook_secret`（形如 `whsec_xxxx…`），在開發者頁可見，用於校驗請求來源（見 7.7）。
- 清空回調地址即停止推送。

### 7.2 請求

| 項 | 值 |
| --- | --- |
| 方法 | `POST` |
| Content-Type | `application/json` |
| 請求頭 | `X-Signature`：請求體的 HMAC-SHA256 簽名（見 7.7）|
| 期望響應 | HTTP `2xx`；其它狀態碼或超時（約 10s）視為失敗並重試 |

**請求體（JSON）**
```json
{
  "event": "card_transaction",
  "auth_id": "1059958172",
  "vm_card_id": "card55202606040031562947331",
  "card_id": 123,
  "card_number": "5378721234568264",
  "auth_time": "2026-07-20 10:20:30",
  "auth_amount": 9.99,
  "auth_currency": "USD",
  "settle_amount": 9.99,
  "settle_currency": "USD",
  "merchant_name": "GOOGLE *CHATGPT 766999 GB",
  "merchant": "GOOGLE *CHATGPT 766999 GB",
  "merchant_location": "United Kingdom",
  "merchant_region": "England",
  "merchant_country": "GB",
  "description": "GOOGLE *CHATGPT 766999 GB",
  "failed_reason": "",
  "bill_status": "Settled",
  "merchant_amount": 9.99,
  "merchant_currency": "USD",
  "create_time": "2026-07-20 10:20:31",
  "status": "COMPLETE",
  "type": "Settlement",
  "source": "webhook",
  "channel": "three"
}
```

### 7.3 事件字段

| 字段 | 類型 | 說明 |
| --- | --- | --- |
| `event` | string | 事件類型，目前固定為 `card_transaction`（卡交易）|
| `auth_id` | string | 上游交易 / 授權唯一號，**冪等鍵**（見 7.7）|
| `vm_card_id` | string | 卡在系統內的上游卡標識 |
| `card_id` | number | 卡在平台的數字 ID（對應 `GET /cards/{id}`）|
| `card_number` | string | **完整卡號**（請按需妥善保管，勿明文落日誌）|
| `auth_time` | string | 平台歸一化的授權時間；渠道 1 保持上游格式，渠道 3 將上游 UTC `billTime` 轉為北京時間（`YYYY-MM-DD HH:mm:ss`） |
| `auth_amount` | number | 授權金額 |
| `auth_currency` | string | 授權/交易幣種 |
| `settle_amount` | number | 結算金額（USD，正數）。清算 / 退款為實際金額；授權與拒付可能為 `0` |
| `settle_currency` | string | 結算幣種 |
| `merchant_name` | string | 商戶名稱或商戶描述原文；描述中的驗證碼不會被單獨提取 |
| `merchant` | string | `merchant_name` 的兼容別名 |
| `merchant_location` | string | 商戶地區/位置；上游沒有提供時為空字符串 |
| `merchant_region` / `merchant_country` | string | 上游提供的地區/國家別名；沒有時為空字符串 |
| `description` | string | 交易描述原文；不要依賴平台另行生成驗證碼字段 |
| `failed_reason` | string | 上游失敗原因；沒有時為空字符串 |
| `bill_status` | string | 上游賬單狀態原值；實時回調通常是文本，渠道 3 REST 補賬也可能是數字代碼字符串（如 `"99"`） |
| `merchant_amount` / `merchant_currency` | number / string | 上游或渠道歸一化的商戶本幣金額和幣種；沒有時為 `0` / 空字符串 |
| `create_time` | string | 上游創建時間；渠道 3 保留上游 UTC 原文，渠道 1 按上游回調提供；沒有時為空字符串 |
| `status` | string | 交易狀態：`PENDING` / `COMPLETE` / `DECLINED`（見 [§5 數據字典](#5-數據字典)）|
| `type` | string | 交易類型：`Authorization` / `Settlement` / `Refund` / `Reversal`（見 [§5 數據字典](#5-數據字典)）|
| `source` | string | `webhook` 為實時上游回調，`reconciled` 為單卡對賬補錄 |
| `channel` | string | 渠道內部標記：渠道 1 為 `one`，渠道 3 為 `three`；渠道 2 Webhook 已退役。**附加字段**，可忽略 |

> 舊字段全部保留。交易接收端應按 `event + auth_id + type + status` 冪等；同一授權從 `PENDING` 變為 `COMPLETE` 時不能只按 `auth_id` 丟棄後續狀態。
>
> 渠道 3 的驗證碼如果位於上游 `merchantDescription` / `merchantName` 中，會原樣進入歸一化事件的 `description` / `merchant_name`；平台不會生成獨立的 `verification_code` 或 `otp` 字段。

### 7.4 卡操作事件

開卡、充值、凍結、解凍、刪卡和卡餘額退款等操作如果有上游狀態回調，平台發送獨立的 `card_operation` 事件。無法安全歸類的上游通用卡狀態事件使用 `operation = status_change`。商戶退款仍使用 `card_transaction`，`type = Refund`。

```json
{
  "event": "card_operation",
  "operation": "recharge",
  "operation_id": "R202607200001",
  "event_type": "CardRechargeStatusChanged",
  "bill_number": "R202607200001",
  "vm_card_id": "three-card-202607200001",
  "card_id": 123,
  "card_number": "5378721234568264",
  "status": "COMPLETE",
  "success": true,
  "operate_status": "Success",
  "operate_status_value": 1,
  "bill_status": "Completed",
  "failed_reason": "",
  "amount": 10,
  "currency": "USD",
  "balance": 20,
  "balance_after": 30,
  "card_status": "Activated",
  "card_status_value": 1,
  "occurred_at": "2026-07-20 10:20:30",
  "source": "webhook",
  "channel": "three"
}
```

| 字段 | 類型 | 說明 |
| --- | --- | --- |
| `event` | string | 固定為 `card_operation` |
| `operation` | string | `open_card` / `recharge` / `freeze` / `unfreeze` / `delete` / `refund` / `status_change` |
| `operation_id` | string | 上游操作單號；刪卡無單號時使用卡號 |
| `event_type` | string | 上游操作事件名 |
| `bill_number` | string | 上游賬單/操作單號，沒有時為空 |
| `vm_card_id` / `card_id` / `card_number` | string / number / string | 能解析到本地卡時返回；`vm_card_id` 是本地上游卡標識，不是完整卡號；開卡回調可能暫時沒有 `card_id` |
| `card_number_masked` | string | 上游脫敏卡號（如有） |
| `status` | string | `PENDING` / `COMPLETE` / `FAILED` |
| `success` | boolean | 上游是否明確成功 |
| `operate_status` / `operate_status_value` | string / number | 上游操作狀態原值 |
| `bill_status` | string | 上游賬單狀態原值 |
| `failed_reason` | string | 上游失敗原因 |
| `amount` / `currency` | number / string | 本次操作金額和幣種 |
| `event_amount` / `settled_amount` / `refund_amount` / `revoked_amount` | number | 上游各階段原始金額 |
| `balance` / `balance_after` | number | 上游回調中的操作前後餘額 |
| `card_status` / `card_status_value` | string / number | 上游卡狀態原值 |
| `occurred_at` | string | 上游事件時間 |
| `source` / `channel` | string | 回調來源和渠道；當前實時來源為 `webhook` |

> 開卡回調可能早於本地卡記錄創建。無法通過操作單號或卡號安全找到卡主時只保留內部記錄，不會把事件發送給錯誤用戶；待卡主可解析後才發送。

### 7.5 GPT 直充完成事件

訂單第一次從非完成狀態進入 `completed` 後，平台異步投遞以下事件。中間的排隊、預檢、支付等待狀態不會單獨觸發 Webhook。

```json
{
  "type": "gpt_direct.completed",
  "order_id": 981,
  "client_request_id": "merchant-order-20260718-001",
  "plan": "plus",
  "account_email": "user@example.com",
  "status": "completed",
  "final_amount_minor": 98214,
  "currency": "PHP",
  "completed_at": 1784397600
}
```

| 字段 | 類型 | 說明 |
| --- | --- | --- |
| `type` | string | 固定為 `gpt_direct.completed`；GPT 事件使用 `type`，不是卡事件的 `event` |
| `order_id` | number | 平台 GPT 直充訂單 ID |
| `client_request_id` | string | 創建訂單時的商戶訂單號 |
| `plan` | string | `plus` / `pro_5x` / `pro_20x` |
| `account_email` | string | 預檢識別到的 GPT 賬號郵箱 |
| `status` | string | 固定為 `completed` |
| `final_amount_minor` | number | 上游最終扣款金額的最小單位；不是 API 服務費 |
| `currency` | string | 付款幣種，例如 `PHP` |
| `completed_at` | number | Unix 時間戳（秒） |

API 服務費是否結算請以訂單詳情的 `service_fee_status` 為準；不要用 `final_amount_minor` 計算平台服務費。接收端應以 `order_id` 或 `client_request_id` 冪等處理，且仍要允許網絡重試造成的重復通知。

### 7.6 事件場景對照（type × status）

| type | status | 含義 | 對卡餘額 |
| --- | --- | --- | --- |
| `Authorization` | `PENDING` | 消費授權成功，預扣（凍結）額度，尚未清算 | 佔用可用額度 |
| `Settlement` | `COMPLETE` | 清算完成，真實扣款落地 | 扣減 |
| 任意 | `DECLINED` | 交易被拒（餘額不足 / 風控等）| 不變 |
| `Refund` | `COMPLETE` / `PENDING` | 消費退款，金額退回卡內 | 增加 |
| `Reversal` | `COMPLETE` | 授權撤銷（預授權取消，非真實退款）| 釋放佔用 |

> 一筆消費通常先收到 `Authorization` / `PENDING`，清算後再收到 `Settlement` / `COMPLETE`，兩條 `auth_id` 相同——按 7.7 冪等去重。

### 7.7 簽名校驗（務必校驗）

`X-Signature` = `HMAC-SHA256(webhook_secret, 原始請求體字節)` 的**十六進制小寫**串。必須用**原始 body** 計算（不要先反序列化再重新序列化，否則字節不一致導致校驗失敗），並用常量時間比較：

```js
const crypto = require('crypto')
// 用 raw body（Buffer），不要用已解析的對象
app.post('/webhook', express.raw({ type: '*/*' }), (req, res) => {
  const expect = crypto.createHmac('sha256', WEBHOOK_SECRET).update(req.body).digest('hex')
  const got = req.headers['x-signature'] || ''
  const ok = expect.length === got.length &&
    crypto.timingSafeEqual(Buffer.from(expect), Buffer.from(got))
  if (!ok) return res.status(401).end()

  const evt = JSON.parse(req.body.toString('utf8'))
  // TODO: 卡事件按 evt.auth_id、GPT 事件按 evt.order_id 冪等處理
  res.status(200).end()
})
```

### 7.8 投遞、重試與冪等

- **異步投遞**，不阻塞上游事件處理；每次投遞超時約 10 秒。
- 未收到 `2xx` 會**退避重試**，最多共 3 次（間隔約 `2s`、`4s` 遞增）；仍失敗則丟棄該次投遞（不會無限重投）。
- 接收端應**盡快返回 2xx**（重活丟隊列異步做），否則易觸發重試與重復投遞。
- **冪等**：交易事件請以 `event + auth_id + type + status` 去重；操作事件請以 `event + operation + operation_id + status` 去重；GPT 事件請以 `order_id` 或 `client_request_id` 去重，切勿重復入賬。
- `source = reconciled` 表示單卡對賬補錄，不代表新的上游扣款；接收端應按同一業務號合併或更新記錄。
- 平台側拒付、撤銷、退款、小額消費和受限商戶費用也按業務號冪等；同一事件回放不會再次收費或重復執行凍結/刪卡處置。
- **不保證嚴格順序**：極端情況下 `COMPLETE` 可能早於 `PENDING` 到達，請以最終態為準。

### 7.9 安全建議

- 只接受 `https`；先校驗 `X-Signature` 再處理，拒絕簽名不符的請求。
- `card_number` 是完整卡號，按合規要求存儲 / 傳輸，切勿寫入明文日誌或轉發到不可信下游。
- 平台僅從公網地址回源，回調地址不可指向內網 / 本機。
- 平台不會把商戶描述中的驗證碼單獨提取成站內通知或獨立回調字段；需要時請讀取 `merchant_name` / `description`。

---

如需協助，請在「開發者」頁聯繫客服或加入開發者群。
