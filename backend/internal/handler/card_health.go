package handler

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tuzi/cdk-recharge-system/internal/cardplatform"
	"github.com/tuzi/cdk-recharge-system/internal/db"
)

const cardHealthPolicyKey = "card_health_policy"

// CardFailVerdict 失败归因结论。
const (
	verdictNeedMore      = "need_more"       // 未达失败次数
	verdictEmailSuspect  = "email_suspect"   // 同邮箱反复失败 → 更像号/邮箱问题
	verdictCardSuspect   = "card_suspect"    // 多邮箱失败 → 更像卡问题
	verdictUnknownEmails = "unknown_emails"  // 缺邮箱，暂不判卡
	verdictAlreadyBlocked = "already_blocked"
)

// EvaluateCardFailVerdict 纯逻辑：同卡失败次数 + 去重邮箱 → 归因。
//
// 规则（产品约定）：
//   - 失败次数 < threshold → need_more
//   - 有已知邮箱且 distinct_emails >= 2 → card_suspect（不同账号都挂，卡不好用）
//   - 有已知邮箱且 distinct_emails == 1 → email_suspect（同一邮箱反复挂，像邮箱/号问题）
//   - 无已知邮箱且 requireKnownEmail → unknown_emails
//   - 无已知邮箱且 !requireKnownEmail 且 fail>=threshold → card_suspect（保守当卡问题）
func EvaluateCardFailVerdict(failCount, distinctEmails int, threshold int, requireKnownEmail bool) string {
	if threshold < 1 {
		threshold = 2
	}
	if failCount < threshold {
		return verdictNeedMore
	}
	if distinctEmails >= 2 {
		return verdictCardSuspect
	}
	if distinctEmails == 1 {
		return verdictEmailSuspect
	}
	// distinctEmails == 0：全是 unknown
	if requireKnownEmail {
		return verdictUnknownEmails
	}
	return verdictCardSuspect
}

func loadCardHealthPolicy() db.CardHealthPolicy {
	p := db.DefaultCardHealthPolicy()
	raw, err := db.GetSetting(cardHealthPolicyKey)
	if err != nil || strings.TrimSpace(raw) == "" {
		return p
	}
	_ = json.Unmarshal([]byte(raw), &p)
	if p.FailThreshold < 1 {
		p.FailThreshold = 2
	}
	return p
}

func saveCardHealthPolicy(p db.CardHealthPolicy) error {
	if p.FailThreshold < 1 {
		p.FailThreshold = 2
	}
	b, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return db.SetSetting(cardHealthPolicyKey, string(b))
}

// normalizeAccountEmail 规范化邮箱（小写 trim）；空则 unknown。
func normalizeAccountEmail(email string) string {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return "unknown"
	}
	return email
}

// ObserveCardOrderOutcome 观察一笔终态订单，更新失败统计；坏卡则拉黑并可选冻结。
// 可从 webhook / result 轮询 / 管理端手动触发。
func ObserveCardOrderOutcome(ctx context.Context, in CardOrderObservation) *CardHealthObserveResult {
	res := &CardHealthObserveResult{}
	policy := loadCardHealthPolicy()
	res.PolicyEnabled = policy.Enabled
	if !policy.Enabled {
		res.Verdict = "disabled"
		return res
	}
	if in.CardID <= 0 {
		res.Verdict = "no_card"
		return res
	}

	status := strings.ToLower(strings.TrimSpace(in.Status))
	res.CardID = in.CardID
	res.Status = status

	// 成功：不拉黑；仅记日志（不写 fail 表）
	if status == "completed" || status == "success" {
		res.Verdict = "success"
		return res
	}
	// 仅失败终态
	if status != "failed_precharge" && status != "declined" && status != "failed" {
		res.Verdict = "ignored_status"
		return res
	}

	email := normalizeAccountEmail(in.AccountEmail)
	emailSrc := strings.TrimSpace(in.EmailSource)
	if emailSrc == "" {
		if email != "unknown" {
			emailSrc = "provided"
		} else {
			emailSrc = "unknown"
		}
	}

	ev := db.CardFailEvent{
		CardID:           in.CardID,
		CardLastFour:     strings.TrimSpace(in.CardLastFour),
		OrderID:          in.OrderID,
		CDKCode:          strings.TrimSpace(in.CDKCode),
		AccountEmailNorm: email,
		EmailSource:      emailSrc,
		ErrorCode:        strings.TrimSpace(in.ErrorCode),
		OrderStatus:      status,
	}

	// 先算当前（含本条）趋势：若库内已有同单则 Insert 会 ignore
	inserted, err := db.InsertCardFailEvent(ev)
	if err != nil {
		log.Printf("[card-health] insert fail event card=%d order=%d: %v", in.CardID, in.OrderID, err)
		res.Verdict = "error"
		res.Error = err.Error()
		return res
	}
	res.EventInserted = inserted

	stats, err := db.GetCardFailStats(in.CardID)
	if err != nil {
		res.Verdict = "error"
		res.Error = err.Error()
		return res
	}
	res.FailCount = stats.FailCount
	res.DistinctEmails = stats.DistinctEmails

	if blocked, _ := db.IsCardBlocked(in.CardID); blocked {
		res.Verdict = verdictAlreadyBlocked
		res.Blocked = true
		return res
	}

	verdict := EvaluateCardFailVerdict(stats.FailCount, stats.DistinctEmails, policy.FailThreshold, policy.RequireKnownEmail)
	res.Verdict = verdict

	// 回写本条 verdict（best-effort；同单已存在时可能写不到）
	if inserted {
		_, _ = db.DB.Exec(`UPDATE card_fail_events SET verdict = ? WHERE order_id = ? AND card_id = ?`,
			verdict, in.OrderID, in.CardID)
	}

	if verdict != verdictCardSuspect {
		return res
	}

	// 拉黑
	entry := db.CardBlockEntry{
		CardID:         in.CardID,
		CardLastFour:   in.CardLastFour,
		Reason:         "multi_email_fail",
		DistinctEmails: stats.DistinctEmails,
		FailCount:      stats.FailCount,
		FreezeStatus:   "skipped",
		Notes:          "不同邮箱在该卡上失败≥阈值，本站判定为卡问题",
	}
	if policy.FreezeOnBlock {
		entry.FreezeStatus = "pending"
	}
	if err := db.UpsertCardBlock(entry); err != nil {
		log.Printf("[card-health] block card=%d: %v", in.CardID, err)
		res.Error = err.Error()
		return res
	}
	res.Blocked = true
	db.WriteAudit("system", "card_health.block",
		"card_id="+strconv.FormatInt(in.CardID, 10)+
			" fails="+strconv.Itoa(stats.FailCount)+
			" emails="+strconv.Itoa(stats.DistinctEmails),
		"")

	if policy.FreezeOnBlock {
		cli := cardplatform.NewFromSettings()
		freezeCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		if err := cli.FreezeCard(freezeCtx, in.CardID, true); err != nil {
			log.Printf("[card-health] freeze card=%d failed: %v", in.CardID, err)
			_ = db.UpdateCardBlockFreeze(in.CardID, "failed", err.Error())
			res.FreezeStatus = "failed"
			res.Error = err.Error()
		} else {
			_ = db.UpdateCardBlockFreeze(in.CardID, "ok", "")
			res.FreezeStatus = "ok"
			log.Printf("[card-health] froze card=%d last4=%s fails=%d emails=%d",
				in.CardID, in.CardLastFour, stats.FailCount, stats.DistinctEmails)
		}
	}
	return res
}

// CardOrderObservation 一笔订单观察输入。
type CardOrderObservation struct {
	CardID       int64
	CardLastFour string
	OrderID      int64
	CDKCode      string
	AccountEmail string
	EmailSource  string
	ErrorCode    string
	Status       string
	Message      string
}

// CardHealthObserveResult 观察结果。
type CardHealthObserveResult struct {
	PolicyEnabled  bool   `json:"policy_enabled"`
	CardID         int64  `json:"card_id,omitempty"`
	Status         string `json:"status,omitempty"`
	EventInserted  bool   `json:"event_inserted"`
	FailCount      int    `json:"fail_count"`
	DistinctEmails int    `json:"distinct_emails"`
	Verdict        string `json:"verdict"`
	Blocked        bool   `json:"blocked"`
	FreezeStatus   string `json:"freeze_status,omitempty"`
	Error          string `json:"error,omitempty"`
}

// resolveEmailForOrder 尽量拿到完整邮箱：本地 session > 入参 > unknown。
func resolveEmailForOrder(cdkCode, fallback string) (email, source string) {
	if code := strings.TrimSpace(cdkCode); code != "" {
		if sess, err := db.GetSessionByCDK(code); err == nil && strings.TrimSpace(sess) != "" {
			if em := extractEmailFromSession(sess); em != "" {
				return normalizeAccountEmail(em), "session"
			}
		}
	}
	if em := strings.TrimSpace(fallback); em != "" {
		// webhook 可能是 us***@x.com 掩码，仍可作为弱身份
		return normalizeAccountEmail(em), "payload"
	}
	return "unknown", "unknown"
}

// observeFromWebhookPayload 从卡台 webhook 解析并观察。
func observeFromWebhookPayload(payload map[string]interface{}) {
	if payload == nil {
		return
	}
	eventType := ""
	if v, ok := payload["event"].(string); ok {
		eventType = v
	}
	if v, ok := payload["type"].(string); ok && eventType == "" {
		eventType = v
	}
	eventType = strings.ToLower(strings.TrimSpace(eventType))
	// 终态：failed / completed（completed 直接 return success）
	if !strings.HasPrefix(eventType, "gpt_direct.") {
		return
	}

	cardID := anyToInt64(payload["card_id"])
	orderID := anyToInt64(payload["order_id"])
	status := strings.ToLower(strAny(payload["status"]))
	if status == "" {
		switch {
		case strings.Contains(eventType, "failed"):
			status = "failed_precharge"
		case strings.Contains(eventType, "completed"):
			status = "completed"
		case strings.Contains(eventType, "cancelled"):
			status = "cancelled"
		}
	}
	last4 := strAny(payload["card_last_four"])
	if last4 == "" {
		if cn := strAny(payload["card_number"]); len(cn) >= 4 {
			last4 = cn[len(cn)-4:]
		}
	}
	cdkID := anyToInt64(payload["cdk_id"])
	prefix := strAny(payload["code_prefix"])
	cdkCode, _ := db.LookupCardplatformCDKCode(cdkID, prefix)
	emailRaw := strAny(payload["account_email"])
	email, src := resolveEmailForOrder(cdkCode, emailRaw)
	msg := strAny(payload["message"])
	errCode := strAny(payload["error_code"])
	if errCode == "" {
		errCode = strAny(payload["last_error_code"])
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		res := ObserveCardOrderOutcome(ctx, CardOrderObservation{
			CardID:       cardID,
			CardLastFour: last4,
			OrderID:      orderID,
			CDKCode:      cdkCode,
			AccountEmail: email,
			EmailSource:  src,
			ErrorCode:    errCode,
			Status:       status,
			Message:      msg,
		})
		if res != nil && (res.Blocked || res.EventInserted) {
			log.Printf("[card-health] webhook order=%d card=%d verdict=%s blocked=%v freeze=%s",
				orderID, cardID, res.Verdict, res.Blocked, res.FreezeStatus)
		}
	}()
}

// observeFromPublicResult 兑换 result 轮询时补充观察（webhook 未配也能学）。
func observeFromPublicResult(ctx context.Context, payload map[string]any, cdkCode string) {
	if payload == nil {
		return
	}
	order, _ := payload["order"].(map[string]any)
	if order == nil {
		// 扁平结构
		order = payload
	}
	status := strings.ToLower(strAny(order["status"]))
	if status != "failed_precharge" && status != "declined" && status != "failed" && status != "completed" {
		return
	}
	orderID := anyToInt64(order["id"])
	if orderID == 0 {
		orderID = anyToInt64(order["order_id"])
	}
	cardID := anyToInt64(order["card_id"])
	last4 := strAny(order["card_last_four"])
	emailRaw := strAny(order["account_email"])

	// 公开 result 常不带 card_id：用 OpenAPI 补全
	if cardID == 0 && orderID > 0 {
		cli := cardplatform.NewFromSettings()
		if raw, err := cli.GetCDKOrder(ctx, strconv.FormatInt(orderID, 10)); err == nil {
			if m := extractOrderMap(raw); m != nil {
				if cardID == 0 {
					cardID = anyToInt64(m["card_id"])
				}
				if last4 == "" {
					last4 = strAny(m["card_last_four"])
				}
				if emailRaw == "" {
					emailRaw = strAny(m["account_email"])
				}
				if status == "" {
					status = strings.ToLower(strAny(m["status"]))
				}
			}
			// detail envelope: {order:{...}}
			var wrap map[string]any
			if json.Unmarshal(raw, &wrap) == nil {
				if om, ok := wrap["order"].(map[string]any); ok {
					if cardID == 0 {
						cardID = anyToInt64(om["card_id"])
					}
					if last4 == "" {
						last4 = strAny(om["card_last_four"])
					}
					if emailRaw == "" {
						emailRaw = strAny(om["account_email"])
					}
				}
			}
		}
	}
	if cardID == 0 {
		return
	}
	email, src := resolveEmailForOrder(cdkCode, emailRaw)
	_ = ObserveCardOrderOutcome(ctx, CardOrderObservation{
		CardID:       cardID,
		CardLastFour: last4,
		OrderID:      orderID,
		CDKCode:      cdkCode,
		AccountEmail: email,
		EmailSource:  src,
		Status:       status,
		Message:      strAny(order["message"]),
	})
}

// ---- Admin API ----

// AdminGetCardHealthPolicy GET /api/v1/admin/card-health/policy
func AdminGetCardHealthPolicy(c *gin.Context) {
	p := loadCardHealthPolicy()
	c.JSON(http.StatusOK, gin.H{
		"policy": p,
		"note":   "同卡失败达到阈值后：多邮箱→判卡问题并冻结；单邮箱→判邮箱/号问题不冻卡",
	})
}

// AdminPutCardHealthPolicy PUT /api/v1/admin/card-health/policy
func AdminPutCardHealthPolicy(c *gin.Context) {
	var p db.CardHealthPolicy
	if err := c.ShouldBindJSON(&p); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if p.FailThreshold < 1 {
		p.FailThreshold = 2
	}
	if err := saveCardHealthPolicy(p); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	db.WriteAudit(adminName(c), "card_health.policy", "updated", c.ClientIP())
	c.JSON(http.StatusOK, gin.H{"ok": true, "policy": p})
}

// AdminListCardHealth GET /api/v1/admin/card-health
func AdminListCardHealth(c *gin.Context) {
	include := c.Query("all") == "1" || c.Query("include_inactive") == "1"
	blocks, err := db.ListCardBlocklist(include, 100)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	events, err := db.ListCardFailEvents(80)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// 脱敏邮箱：只露前缀
	for i := range events {
		events[i].AccountEmailNorm = maskEmailForAdmin(events[i].AccountEmailNorm)
		if events[i].CDKCode != "" && len(events[i].CDKCode) > 12 {
			events[i].CDKCode = events[i].CDKCode[:8] + "…"
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"policy":    loadCardHealthPolicy(),
		"blocklist": blocks,
		"events":    events,
	})
}

// AdminUnblockCard POST /api/v1/admin/card-health/unblock
func AdminUnblockCard(c *gin.Context) {
	var body struct {
		CardID      int64  `json:"card_id"`
		Unfreeze    bool   `json:"unfreeze"`
		Notes       string `json:"notes"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.CardID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "card_id required"})
		return
	}
	if err := db.UnblockCard(body.CardID, body.Notes); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	freezeMsg := ""
	if body.Unfreeze {
		cli := cardplatform.NewFromSettings()
		if err := cli.FreezeCard(c.Request.Context(), body.CardID, false); err != nil {
			freezeMsg = err.Error()
		} else {
			freezeMsg = "unfroze"
		}
	}
	db.WriteAudit(adminName(c), "card_health.unblock",
		"card_id="+strconv.FormatInt(body.CardID, 10)+" unfreeze="+freezeMsg, c.ClientIP())
	c.JSON(http.StatusOK, gin.H{"ok": true, "unfreeze": freezeMsg})
}

// AdminReobserveCardOrder POST /api/v1/admin/card-health/observe
// 手动对一笔订单做健康观察（补漏）。
func AdminReobserveCardOrder(c *gin.Context) {
	var body struct {
		OrderID int64 `json:"order_id"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.OrderID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "order_id required"})
		return
	}
	cli := cardplatform.NewFromSettings()
	raw, err := cli.GetCDKOrder(c.Request.Context(), strconv.FormatInt(body.OrderID, 10))
	if err != nil {
		writeCardErr(c, err)
		return
	}
	obs := CardOrderObservation{OrderID: body.OrderID}
	if m := extractOrderMap(raw); m != nil {
		obs.CardID = anyToInt64(m["card_id"])
		obs.CardLastFour = strAny(m["card_last_four"])
		obs.Status = strings.ToLower(strAny(m["status"]))
		obs.AccountEmail = strAny(m["account_email"])
		obs.Message = strAny(m["message"])
		cdkID := anyToInt64(m["cdk_id"])
		prefix := strAny(m["code_prefix"])
		if code, ok := db.LookupCardplatformCDKCode(cdkID, prefix); ok {
			obs.CDKCode = code
		}
	}
	var wrap map[string]any
	if json.Unmarshal(raw, &wrap) == nil {
		if om, ok := wrap["order"].(map[string]any); ok {
			if obs.CardID == 0 {
				obs.CardID = anyToInt64(om["card_id"])
			}
			if obs.Status == "" {
				obs.Status = strings.ToLower(strAny(om["status"]))
			}
			if obs.AccountEmail == "" {
				obs.AccountEmail = strAny(om["account_email"])
			}
			if obs.CardLastFour == "" {
				obs.CardLastFour = strAny(om["card_last_four"])
			}
		}
	}
	email, src := resolveEmailForOrder(obs.CDKCode, obs.AccountEmail)
	obs.AccountEmail = email
	obs.EmailSource = src
	res := ObserveCardOrderOutcome(c.Request.Context(), obs)
	c.JSON(http.StatusOK, gin.H{"ok": true, "result": res})
}

func maskEmailForAdmin(em string) string {
	em = strings.TrimSpace(em)
	if em == "" || em == "unknown" {
		return em
	}
	at := strings.LastIndex(em, "@")
	if at <= 0 {
		if len(em) <= 3 {
			return em
		}
		return em[:2] + "***"
	}
	local := em[:at]
	if len(local) > 2 {
		local = local[:2]
	}
	return local + "***" + em[at:]
}

func adminName(c *gin.Context) string {
	if u, ok := c.Get("username"); ok {
		if s, ok := u.(string); ok && s != "" {
			return s
		}
	}
	if u, ok := c.Get("admin"); ok {
		if s, ok := u.(string); ok && s != "" {
			return s
		}
	}
	return "admin"
}
