package handler

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tuzi/cdk-recharge-system/internal/cardplatform"
	"github.com/tuzi/cdk-recharge-system/internal/db"
	"github.com/tuzi/cdk-recharge-system/internal/plansync"
)

// AdminGetCardSelectionRules GET /api/v1/admin/card-selection/rules
// 返回选卡优先级规则列表（含实时产品在线状态）
func AdminGetCardSelectionRules(c *gin.Context) {
	rules, err := db.GetCardSelectionRules()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	statusMap, _ := db.GetPlanStatusCacheMap()

	type ruleView struct {
		db.CardSelectionRule
		Online        bool    `json:"online"`
		SyncedAt      string  `json:"synced_at"`
		ServiceFeeUSD float64 `json:"service_fee_usd"`
	}

	out := make([]ruleView, 0, len(rules))
	for _, r := range rules {
		rv := ruleView{CardSelectionRule: r, Online: true}
		if ps, ok := statusMap[r.PlanKey]; ok {
			rv.Online = ps.Online
			rv.SyncedAt = ps.SyncedAt
			rv.ServiceFeeUSD = ps.ServiceFeeUSD
		}
		out = append(out, rv)
	}

	statuses, _ := db.GetPlanStatusCache()
	lastSync := latestSyncTime(statuses)

	c.JSON(http.StatusOK, gin.H{
		"rules":     out,
		"last_sync": lastSync,
		"next_sync": nextSyncIn(lastSync),
	})
}

// AdminPutCardSelectionRules PUT /api/v1/admin/card-selection/rules
// 整体替换选卡规则配置（顺序 = 优先级）
func AdminPutCardSelectionRules(c *gin.Context) {
	var body struct {
		Rules []db.CardSelectionRule `json:"rules"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	for i := range body.Rules {
		body.Rules[i].PlanKey = strings.TrimSpace(body.Rules[i].PlanKey)
		body.Rules[i].DisplayName = strings.TrimSpace(body.Rules[i].DisplayName)
		if body.Rules[i].PlanKey == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "plan_key required for each rule"})
			return
		}
		if body.Rules[i].DisplayName == "" {
			body.Rules[i].DisplayName = body.Rules[i].PlanKey
		}
		body.Rules[i].SortOrder = i + 1
	}
	if err := db.SetCardSelectionRules(body.Rules); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	auditAdmin(c, "update_card_selection_rules", fmt.Sprintf("count=%d", len(body.Rules)))
	AdminGetCardSelectionRules(c)
}

// AdminGetPlanStatus GET /api/v1/admin/card-selection/plan-status
// 返回产品状态缓存（含最后同步时间 + 预计下次同步时间）
// AdminGetPlanStatus GET /api/v1/admin/card-selection/plan-status
// 返回逻辑套餐状态缓存 + 实体产品缓存（含最后同步时间）
func AdminGetPlanStatus(c *gin.Context) {
	statuses, err := db.GetPlanStatusCache()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	products, _ := db.GetCardProducts()
	lastSync := latestSyncTime(statuses)
	if lastSync == "" {
		lastSync = latestProductSyncTime(products)
	}
	c.JSON(http.StatusOK, gin.H{
		"statuses":  statuses,
		"products":  products,
		"last_sync": lastSync,
		"next_sync": nextSyncIn(lastSync),
	})
}

// AdminSyncPlanStatus POST /api/v1/admin/card-selection/sync
// 立即触发一次产品状态同步（主动同步）
func AdminSyncPlanStatus(c *gin.Context) {
	cfg := cardplatform.LoadConfig()
	if cfg.APIKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "card_api_key not configured"})
		return
	}
	r, err := plansync.SyncNow(c.Request.Context())
	if err != nil {
		writeCardErr(c, err)
		return
	}
	auditAdmin(c, "sync_plan_status", fmt.Sprintf("plans=%d products=%d", r.Plans, r.Products))
	AdminGetPlanStatus(c)
}

// latestSyncTime 从套餐状态列表中取最新的 synced_at。
func latestSyncTime(statuses []db.PlanStatusCache) string {
	var latest string
	for _, s := range statuses {
		if latest == "" || s.SyncedAt > latest {
			latest = s.SyncedAt
		}
	}
	return latest
}

// latestProductSyncTime 从产品列表中取最新的 synced_at。
func latestProductSyncTime(products []db.CardProductCache) string {
	var latest string
	for _, p := range products {
		if latest == "" || p.SyncedAt > latest {
			latest = p.SyncedAt
		}
	}
	return latest
}

// nextSyncIn 计算距离下次自动同步的剩余时间描述。
func nextSyncIn(lastSync string) string {
	if lastSync == "" {
		return "—"
	}
	t, err := time.Parse("2006-01-02 15:04:05", lastSync)
	if err != nil {
		return "—"
	}
	next := t.Add(3 * time.Minute)
	rem := time.Until(next)
	if rem <= 0 {
		return "即将同步"
	}
	if rem < time.Minute {
		return fmt.Sprintf("%ds 后", int(rem.Seconds()))
	}
	return fmt.Sprintf("%dm%ds 后", int(rem.Minutes()), int(rem.Seconds())%60)
}
