package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tuzi/cdk-recharge-system/internal/db"
)

// CardPlatformWebhook POST /api/v1/webhooks/cardplatform
// 卡台开发者页配置的回调地址指向此处。
// 验签：X-Signature = hex(HMAC-SHA256(webhook_secret, raw body))
// 文档：spacexcard-openapi-zh.md §7
func CardPlatformWebhook(c *gin.Context) {
	raw, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}
	secret, _ := db.GetSetting("webhook_secret")
	secret = strings.TrimSpace(secret)
	if secret == "" {
		// 未配置密钥时拒绝，避免裸奔
		log.Printf("webhook: webhook_secret not configured")
		c.Status(http.StatusServiceUnavailable)
		return
	}
	got := strings.TrimSpace(c.GetHeader("X-Signature"))
	if got == "" {
		c.Status(http.StatusUnauthorized)
		return
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(raw)
	expect := hex.EncodeToString(mac.Sum(nil))
	if subtle.ConstantTimeCompare([]byte(strings.ToLower(got)), []byte(strings.ToLower(expect))) != 1 {
		c.Status(http.StatusUnauthorized)
		return
	}

	// 尽快 200；入库 best-effort
	var payload map[string]interface{}
	_ = json.Unmarshal(raw, &payload)

	eventType := ""
	if v, ok := payload["event"].(string); ok {
		eventType = v
	}
	if v, ok := payload["type"].(string); ok && eventType == "" {
		eventType = v
	}
	// 幂等键
	idem := webhookIdemKey(payload, eventType)
	if err := db.InsertWebhookEvent(eventType, idem, string(raw)); err != nil {
		// 唯一冲突视为已处理（幂等）
		if !strings.Contains(err.Error(), "UNIQUE") && !strings.Contains(err.Error(), "unique") {
			log.Printf("webhook store: %v", err)
		}
	} else {
		// 若是 GPT 直充完成，可写审计
		if eventType == "gpt_direct.completed" {
			db.WriteAudit("webhook", "gpt_direct.completed", idem, c.ClientIP())
		}
	}
	c.Status(http.StatusOK)
}

func webhookIdemKey(p map[string]interface{}, eventType string) string {
	str := func(k string) string {
		if v, ok := p[k]; ok {
			switch t := v.(type) {
			case string:
				return t
			case float64:
				return strings.TrimSuffix(strings.TrimSuffix(jsonNumber(t), ".0"), ".")
			}
		}
		return ""
	}
	switch eventType {
	case "card_transaction":
		return strings.Join([]string{eventType, str("auth_id"), str("type"), str("status")}, "|")
	case "card_operation":
		return strings.Join([]string{eventType, str("operation"), str("operation_id"), str("status")}, "|")
	case "gpt_direct.completed":
		if id := str("order_id"); id != "" {
			return "gpt_direct.completed|order|" + id
		}
		if id := str("client_request_id"); id != "" {
			return "gpt_direct.completed|client|" + id
		}
	}
	// fallback
	h := sha256.Sum256([]byte(str("auth_id") + str("order_id") + str("operation_id") + eventType + time.Now().UTC().Format(time.RFC3339Nano)))
	return hex.EncodeToString(h[:16])
}

func jsonNumber(f float64) string {
	b, _ := json.Marshal(f)
	return string(b)
}

// AdminListWebhooks GET /api/v1/admin/webhooks/events
func AdminListWebhooks(c *gin.Context) {
	limit := 50
	rows, err := db.ListWebhookEvents(limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list failed"})
		return
	}
	// 脱敏：不返回完整 card_number
	out := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		out = append(out, gin.H{
			"id":         r.ID,
			"event_type": r.EventType,
			"idem_key":   r.IdemKey,
			"created_at": r.CreatedAt,
			"payload":    sanitizeWebhookPayload(r.Payload),
		})
	}
	urlHint := ""
	if host := c.Request.Host; host != "" {
		scheme := "https"
		if c.Request.TLS == nil && !strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https") {
			// 生产经 Caddy 会带 proto
			if p := c.GetHeader("X-Forwarded-Proto"); p != "" {
				scheme = p
			}
		} else if strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https") {
			scheme = "https"
		}
		urlHint = scheme + "://" + host + "/api/v1/webhooks/cardplatform"
	}
	sec, _ := db.GetSetting("webhook_secret")
	c.JSON(http.StatusOK, gin.H{
		"events":                out,
		"webhook_url":           urlHint,
		"webhook_secret_set":    strings.TrimSpace(sec) != "",
		"webhook_secret_hint":   maskSecret(sec),
	})
}

func sanitizeWebhookPayload(raw string) interface{} {
	var m map[string]interface{}
	if json.Unmarshal([]byte(raw), &m) != nil {
		return raw
	}
	if cn, ok := m["card_number"].(string); ok && len(cn) > 4 {
		m["card_number"] = "****" + cn[len(cn)-4:]
	}
	return m
}
