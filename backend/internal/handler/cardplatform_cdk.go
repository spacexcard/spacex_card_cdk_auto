package handler

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tuzi/cdk-recharge-system/internal/cardplatform"
	"github.com/tuzi/cdk-recharge-system/internal/db"
)

func writeCardErr(c *gin.Context, err error) {
	if ae, ok := err.(*cardplatform.APIError); ok {
		status := ae.HTTPStatus
		if status < 400 {
			status = http.StatusBadRequest
		}
		if status > 599 {
			status = http.StatusBadGateway
		}
		c.JSON(status, gin.H{
			"error":      ae.Msg,
			"error_code": ae.ErrorCode,
			"code":       ae.Code,
		})
		return
	}
	c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
}

// CardPlatformPlans GET /api/v1/admin/cardplatform/plans
// 实时套餐服务费（CDK 收费价）
func CardPlatformPlans(c *gin.Context) {
	cli := cardplatform.NewFromSettings()
	plans, err := cli.GetPlans(c.Request.Context())
	if err != nil {
		writeCardErr(c, err)
		return
	}
	// 附加美元展示字段，方便前端
	type planView struct {
		cardplatform.PlanInfo
		ServiceFeeUSD float64 `json:"service_fee_usd"`
	}
	out := gin.H{
		"version": plans.Version,
		"plans":   map[string]planView{},
		"base":    cardplatform.LoadConfig().SiteBase,
	}
	m := map[string]planView{}
	for k, p := range plans.Plans {
		if p.Key == "" {
			p.Key = k
		}
		m[k] = planView{PlanInfo: p, ServiceFeeUSD: cardplatform.MinorToUSD(p.ServiceFeeUsdMinor)}
	}
	out["plans"] = m
	c.JSON(http.StatusOK, out)
}

// CardPlatformBalance GET /api/v1/admin/cardplatform/balance
func CardPlatformBalance(c *gin.Context) {
	cli := cardplatform.NewFromSettings()
	bal, err := cli.GetBalance(c.Request.Context())
	if err != nil {
		writeCardErr(c, err)
		return
	}
	c.JSON(http.StatusOK, bal)
}

// CardPlatformIssueCDKs POST /api/v1/admin/cardplatform/cdks
// body: { plan, count, funding_confirmed }
func CardPlatformIssueCDKs(c *gin.Context) {
	var req struct {
		Plan             string `json:"plan"`
		Count            int    `json:"count"`
		FundingConfirmed bool   `json:"funding_confirmed"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	plan := strings.TrimSpace(req.Plan)
	switch plan {
	case "plus", "pro_5x", "pro_20x":
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "plan must be plus | pro_5x | pro_20x"})
		return
	}
	if !req.FundingConfirmed {
		c.JSON(http.StatusBadRequest, gin.H{"error": "funding_confirmed must be true（确认承担兑换时开卡/充值/订阅实付）"})
		return
	}
	if req.Count < 1 {
		req.Count = 1
	}
	if req.Count > 50 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "count max 50"})
		return
	}

	cli := cardplatform.NewFromSettings()
	idem := c.GetHeader("Idempotency-Key")
	if idem == "" {
		b := make([]byte, 16)
		_, _ = rand.Read(b)
		idem = "cdk-issue-" + hex.EncodeToString(b)
	}
	res, err := cli.IssueCDKs(c.Request.Context(), plan, req.Count, idem)
	if err != nil {
		writeCardErr(c, err)
		return
	}
	u, _ := c.Get("username")
	username, _ := u.(string)
	db.WriteAudit(username, "cardplatform_issue_cdk", "plan="+plan+" count="+strconv.Itoa(req.Count), c.ClientIP())
	// 规范化：保证前端总能拿到完整 code 字段；绝不把 code_prefix 填进 code
	issued := make([]gin.H, 0, len(res.Issued))
	for _, it := range res.Issued {
		code := strings.TrimSpace(it.Code)
		prefix := strings.TrimSpace(it.CodePrefix)
		if code == "" {
			// 防御上游异常：只回了前缀时仍原样暴露前缀字段，但不伪造 code
			issued = append(issued, gin.H{
				"id": it.ID, "code": "", "plan": it.Plan,
				"code_prefix": prefix, "fee_amount_minor": it.FeeAmountMinor,
				"incomplete": true,
			})
			continue
		}
		if prefix == "" && len(code) >= 14 {
			prefix = code[:14]
		}
		// 本站持久化完整码：列表页卡台只回 prefix 时仍可复制完整码
		if err := db.SaveCardplatformCDKCode(it.ID, code, prefix, it.Plan, it.FeeAmountMinor); err != nil {
			// 不阻断发码响应
		}
		issued = append(issued, gin.H{
			"id": it.ID, "code": code, "plan": it.Plan,
			"code_prefix": prefix, "fee_amount_minor": it.FeeAmountMinor,
			"code_length": len(code),
			"full_code":  code,
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"requested": res.Requested,
		"issued":    issued,
		"count":     len(issued),
	})
}

// CardPlatformListCDKs GET /api/v1/admin/cardplatform/cdks?page=&page_size=
func CardPlatformListCDKs(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	ps, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	cli := cardplatform.NewFromSettings()
	res, err := cli.ListCDKs(c.Request.Context(), page, ps)
	if err != nil {
		writeCardErr(c, err)
		return
	}
	// 用本站发码缓存补全 code，便于列表点击复制完整码
	type rowOut struct {
		ID             int64  `json:"id"`
		Plan           string `json:"plan"`
		CodePrefix     string `json:"code_prefix"`
		Status         string `json:"status"`
		FeeAmountMinor int64  `json:"fee_amount_minor"`
		CreatedAt      string `json:"created_at"`
		Code           string `json:"code,omitempty"`
		FullCode       string `json:"full_code,omitempty"`
		HasFullCode    bool   `json:"has_full_code"`
	}
	out := make([]rowOut, 0, len(res.List))
	for _, it := range res.List {
		full, ok := db.LookupCardplatformCDKCode(it.ID, it.CodePrefix)
		row := rowOut{
			ID: it.ID, Plan: it.Plan, CodePrefix: it.CodePrefix, Status: it.Status,
			FeeAmountMinor: it.FeeAmountMinor, CreatedAt: it.CreatedAt,
			HasFullCode: ok,
		}
		if ok {
			row.Code = full
			row.FullCode = full
		}
		out = append(out, row)
	}
	c.JSON(http.StatusOK, gin.H{"list": out, "total": res.Total})
}

// CardPlatformListCDKOrders GET /api/v1/admin/cardplatform/cdk-orders
// 对账列表：卡台 CDK 兑换订单；若上游暂未带 code_prefix/cdk_status，则本站按 cdk_id 补齐。
func CardPlatformListCDKOrders(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	ps, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	cli := cardplatform.NewFromSettings()
	raw, err := cli.ListCDKOrders(c.Request.Context(), page, ps)
	if err != nil {
		writeCardErr(c, err)
		return
	}
	if len(raw) == 0 {
		c.JSON(http.StatusOK, gin.H{"list": []any{}, "total": 0})
		return
	}
	enriched := enrichCDKOrderList(c, cli, raw)
	c.JSON(http.StatusOK, enriched)
}

// CardPlatformGetCDKOrder GET /api/v1/admin/cardplatform/cdk-orders/:id
func CardPlatformGetCDKOrder(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id required"})
		return
	}
	cli := cardplatform.NewFromSettings()
	raw, err := cli.GetCDKOrder(c.Request.Context(), id)
	if err != nil {
		writeCardErr(c, err)
		return
	}
	if len(raw) == 0 {
		c.JSON(http.StatusOK, gin.H{})
		return
	}
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		c.Data(http.StatusOK, "application/json; charset=utf-8", raw)
		return
	}
	enrichOneCDKOrder(c, cli, m)
	c.JSON(http.StatusOK, m)
}

// enrichCDKOrderList 解析 list/total，按需从列码接口补 code_prefix / cdk_status。
func enrichCDKOrderList(c *gin.Context, cli *cardplatform.Client, raw json.RawMessage) gin.H {
	var envelope map[string]any
	if err := json.Unmarshal(raw, &envelope); err != nil {
		// 无法解析时原样包一层，避免吞数据
		return gin.H{"list": []any{}, "total": 0, "parse_error": true}
	}
	listAny, _ := envelope["list"].([]any)
	total := 0
	switch t := envelope["total"].(type) {
	case float64:
		total = int(t)
	case int:
		total = t
	}
	// 收集需要补全的 cdk_id
	needCDK := false
	ids := map[int64]bool{}
	for _, it := range listAny {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		if strAny(m["code_prefix"]) == "" || strAny(m["cdk_status"]) == "" {
			needCDK = true
		}
		if id := int64Any(m["cdk_id"]); id > 0 {
			ids[id] = true
		}
	}
	cdkMap := map[int64]cardplatform.CDKListItem{}
	if needCDK && len(ids) > 0 {
		// 拉若干页 CDK 建索引（白标量级通常不大）
		for page := 1; page <= 20; page++ {
			res, err := cli.ListCDKs(c.Request.Context(), page, 100)
			if err != nil || res == nil || len(res.List) == 0 {
				break
			}
			for _, item := range res.List {
				cdkMap[item.ID] = item
			}
			if len(res.List) < 100 || len(cdkMap) >= res.Total {
				break
			}
		}
	}
	outList := make([]any, 0, len(listAny))
	for _, it := range listAny {
		m, ok := it.(map[string]any)
		if !ok {
			outList = append(outList, it)
			continue
		}
		if id := int64Any(m["cdk_id"]); id > 0 {
			if item, ok := cdkMap[id]; ok {
				if strAny(m["code_prefix"]) == "" {
					m["code_prefix"] = item.CodePrefix
				}
				if strAny(m["cdk_status"]) == "" {
					m["cdk_status"] = item.Status
				}
			}
		}
		// 归一化 CDK 生命周期展示字段
		m["cdk_lifecycle"] = cdkLifecycleLabel(strAny(m["cdk_status"]), strAny(m["status"]), strAny(m["service_fee_status"]))
		outList = append(outList, m)
	}
	return gin.H{"list": outList, "total": total}
}

func enrichOneCDKOrder(c *gin.Context, cli *cardplatform.Client, m map[string]any) {
	id := int64Any(m["cdk_id"])
	if id > 0 && (strAny(m["code_prefix"]) == "" || strAny(m["cdk_status"]) == "") {
		for page := 1; page <= 20; page++ {
			res, err := cli.ListCDKs(c.Request.Context(), page, 100)
			if err != nil || res == nil {
				break
			}
			found := false
			for _, item := range res.List {
				if item.ID == id {
					if strAny(m["code_prefix"]) == "" {
						m["code_prefix"] = item.CodePrefix
					}
					if strAny(m["cdk_status"]) == "" {
						m["cdk_status"] = item.Status
					}
					found = true
					break
				}
			}
			if found || len(res.List) < 100 {
				break
			}
		}
	}
	m["cdk_lifecycle"] = cdkLifecycleLabel(strAny(m["cdk_status"]), strAny(m["status"]), strAny(m["service_fee_status"]))
}

func cdkLifecycleLabel(cdkStatus, orderStatus, feeStatus string) string {
	switch strings.ToLower(strings.TrimSpace(cdkStatus)) {
	case "consumed":
		return "已消耗"
	case "reserved":
		return "预留中"
	case "unused":
		if strings.EqualFold(feeStatus, "released") ||
			strings.EqualFold(orderStatus, "declined") ||
			strings.EqualFold(orderStatus, "failed_precharge") ||
			strings.EqualFold(orderStatus, "cancelled") {
			return "已释放"
		}
		return "未使用"
	case "frozen":
		return "已冻结"
	case "disabled":
		return "已禁用"
	}
	if strings.EqualFold(feeStatus, "released") {
		return "服务费已释放"
	}
	if cdkStatus != "" {
		return cdkStatus
	}
	return "—"
}

func strAny(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	b, _ := json.Marshal(v)
	return strings.Trim(string(b), `"`)
}

func int64Any(v any) int64 {
	switch t := v.(type) {
	case float64:
		return int64(t)
	case int64:
		return t
	case int:
		return int64(t)
	case json.Number:
		n, _ := t.Int64()
		return n
	case string:
		n, _ := strconv.ParseInt(strings.TrimSpace(t), 10, 64)
		return n
	default:
		return 0
	}
}

// ---- 公开兑换 BFF（浏览器只打本站，本站转发卡台）----

func deviceFrom(c *gin.Context) string {
	if d := strings.TrimSpace(c.GetHeader("X-Redemption-Device")); d != "" {
		return d
	}
	return c.GetHeader("User-Agent")
}

func proxyPublicJSON(c *gin.Context, status int, raw json.RawMessage) {
	if len(raw) == 0 {
		c.Status(status)
		return
	}
	c.Data(status, "application/json; charset=utf-8", raw)
}

// PublicCDKPreview POST /api/v1/public/cdk/preview
func PublicCDKPreview(c *gin.Context) {
	var body map[string]any
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	code := str(body["code"])
	cli := cardplatform.NewFromSettings()
	st, raw, err := cli.Preview(c.Request.Context(), code, deviceFrom(c))
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	// 成功时记下 code ↔ redemption_token，供后续绑定 session / 账单查卡密
	if st >= 200 && st < 300 {
		if tok := extractJSONString(raw, "redemption_token", "token"); tok != "" {
			_ = db.BindCDKRedemptionToken(code, tok)
		}
		// 嵌套 data
		if tok := extractJSONNestedString(raw, "data", "redemption_token"); tok != "" {
			_ = db.BindCDKRedemptionToken(code, tok)
		}
	}
	proxyPublicJSON(c, st, raw)
}

// PublicCDKPreflight POST /api/v1/public/cdk/preflight
func PublicCDKPreflight(c *gin.Context) {
	var body map[string]any
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	cli := cardplatform.NewFromSettings()
	st, raw, err := cli.Preflight(c.Request.Context(), body, deviceFrom(c))
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	// 预检成功：把 credential.session 绑到卡密（账单页可凭卡密查）
	if st >= 200 && st < 300 {
		tok := str(body["redemption_token"])
		sess := extractCredentialSession(body["credential"])
		if sess != "" {
			_ = db.BindCDKSession("", tok, sess)
		}
	}
	proxyPublicJSON(c, st, raw)
}

// PublicCDKRedeem POST /api/v1/public/cdk/redeem
func PublicCDKRedeem(c *gin.Context) {
	var body map[string]any
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	cli := cardplatform.NewFromSettings()
	st, raw, err := cli.Redeem(c.Request.Context(), body, deviceFrom(c))
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	proxyPublicJSON(c, st, raw)
}

// PublicCDKResult GET /api/v1/public/cdk/result?token=
func PublicCDKResult(c *gin.Context) {
	token := strings.TrimSpace(c.Query("token"))
	if token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "token required"})
		return
	}
	cli := cardplatform.NewFromSettings()
	st, raw, err := cli.Result(c.Request.Context(), token, deviceFrom(c))
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	proxyPublicJSON(c, st, raw)
}

// PublicCDKPlans GET /api/v1/public/cdk/plans
// 公开展示服务费参考价（不暴露 API Key；若未配置 Key 则返回文档默认价）
func PublicCDKPlans(c *gin.Context) {
	cli := cardplatform.NewFromSettings()
	cfg := cardplatform.LoadConfig()
	if cfg.APIKey == "" {
		// 文档默认服务费（美分）
		c.JSON(http.StatusOK, gin.H{
			"version": 0,
			"source":  "docs_default",
			"plans": map[string]any{
				"plus":    gin.H{"key": "plus", "label": "Plus", "serviceFeeUsdMinor": 100, "service_fee_usd": 1, "enabled": true},
				"pro_5x":  gin.H{"key": "pro_5x", "label": "Pro 5x", "serviceFeeUsdMinor": 500, "service_fee_usd": 5, "enabled": true},
				"pro_20x": gin.H{"key": "pro_20x", "label": "Pro 20x", "serviceFeeUsdMinor": 1000, "service_fee_usd": 10, "enabled": true},
			},
			"note": "配置卡台 API Key 后将返回账户实时价",
		})
		return
	}
	plans, err := cli.GetPlans(c.Request.Context())
	if err != nil {
		// 降级文档默认
		c.JSON(http.StatusOK, gin.H{
			"version": 0,
			"source":  "docs_default_fallback",
			"error":   err.Error(),
			"plans": map[string]any{
				"plus":    gin.H{"key": "plus", "label": "Plus", "serviceFeeUsdMinor": 100, "service_fee_usd": 1, "enabled": true},
				"pro_5x":  gin.H{"key": "pro_5x", "label": "Pro 5x", "serviceFeeUsdMinor": 500, "service_fee_usd": 5, "enabled": true},
				"pro_20x": gin.H{"key": "pro_20x", "label": "Pro 20x", "serviceFeeUsdMinor": 1000, "service_fee_usd": 10, "enabled": true},
			},
		})
		return
	}
	m := map[string]any{}
	for k, p := range plans.Plans {
		if p.Key == "" {
			p.Key = k
		}
		m[k] = gin.H{
			"key":                p.Key,
			"label":              p.Label,
			"currency":           p.Currency,
			"enabled":            p.Enabled,
			"serviceFeeUsdMinor": p.ServiceFeeUsdMinor,
			"service_fee_usd":    cardplatform.MinorToUSD(p.ServiceFeeUsdMinor),
			"expectedAmountMinor": p.ExpectedAmountMinor,
		}
	}
	c.JSON(http.StatusOK, gin.H{"version": plans.Version, "source": "cardplatform_live", "plans": m})
}

func str(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(toString(v))
}

func toString(v any) string {
	b, _ := json.Marshal(v)
	return strings.Trim(string(b), `"`)
}

func extractJSONString(raw json.RawMessage, keys ...string) string {
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return ""
	}
	for _, k := range keys {
		if s := str(m[k]); s != "" {
			return s
		}
	}
	return ""
}

func extractJSONNestedString(raw json.RawMessage, nest, key string) string {
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return ""
	}
	inner, _ := m[nest].(map[string]any)
	if inner == nil {
		return ""
	}
	return str(inner[key])
}

func extractCredentialSession(cred any) string {
	m, ok := cred.(map[string]any)
	if !ok {
		return ""
	}
	// session 模式
	if s := str(m["session"]); s != "" {
		return s
	}
	// 整段 JSON 也可
	if s := str(m["accessToken"]); s != "" {
		return s
	}
	// mailbox 不存密码做账单（无 token）
	return ""
}
