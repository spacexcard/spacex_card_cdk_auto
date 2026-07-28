package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tuzi/cdk-recharge-system/internal/db"
	"github.com/tuzi/cdk-recharge-system/internal/gptcheck"
)

// SessionBillingCheck POST /api/v1/public/billing/check
// 支持：
//  1) cdk_code — 用兑换时绑定的 session 查账单
//  2) token_input / session — 直接贴 session / accessToken
func SessionBillingCheck(c *gin.Context) {
	var req struct {
		TokenInput string `json:"token_input"`
		Session    string `json:"session"`
		CDKCode    string `json:"cdk_code"`
		Code       string `json:"code"` // 兼容
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	raw := strings.TrimSpace(req.TokenInput)
	if raw == "" {
		raw = strings.TrimSpace(req.Session)
	}
	cdk := strings.TrimSpace(req.CDKCode)
	if cdk == "" {
		cdk = strings.TrimSpace(req.Code)
	}

	source := "session"
	if raw == "" && cdk != "" {
		sess, err := db.GetSessionByCDK(cdk)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "查询绑定失败"})
			return
		}
		if strings.TrimSpace(sess) == "" {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "未找到该卡密绑定的 session。请确认兑换时使用了 session 凭证，或改用直接粘贴 session 查询。",
			})
			return
		}
		raw = sess
		source = "cdk"
	}

	if raw == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请输入卡密，或粘贴 session JSON / accessToken"})
		return
	}

	res, err := gptcheck.Check(raw)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"summary":     res.Summary,
		"invoices":    res.Invoices,
		"auth_source": source, // cdk | session
	})
}
