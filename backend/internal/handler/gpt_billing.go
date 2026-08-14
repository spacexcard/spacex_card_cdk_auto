package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tuzi/cdk-recharge-system/internal/db"
	"github.com/tuzi/cdk-recharge-system/internal/gptcheck"
)

func accounthubBaseURL() string {
	if u := strings.TrimSpace(os.Getenv("ACCOUNTHUB_BASE_URL")); u != "" {
		return strings.TrimRight(u, "/")
	}
	return "http://localhost:8788"
}

// extractEmailFromSession 从 session JSON 提取 user.email。
func extractEmailFromSession(raw string) string {
	s := strings.TrimSpace(raw)
	if !strings.HasPrefix(s, "{") {
		return ""
	}
	var data map[string]interface{}
	if json.Unmarshal([]byte(s), &data) != nil {
		return ""
	}
	if user, ok := data["user"].(map[string]interface{}); ok {
		if email, ok := user["email"].(string); ok {
			return strings.TrimSpace(email)
		}
	}
	if email, ok := data["email"].(string); ok {
		return strings.TrimSpace(email)
	}
	return ""
}

type acchubInvoiceResp struct {
	InvoiceURL string                   `json:"invoice_url"`
	Invoices   []map[string]interface{} `json:"invoices"`
	Source     string                   `json:"source"`
	Error      string                   `json:"error"`
}

// queryAccounthubInvoices 通过邮箱调 accounthub 获取账单。
func queryAccounthubInvoices(email string) (*acchubInvoiceResp, error) {
	base := accounthubBaseURL()
	u := fmt.Sprintf("%s/gpt/invoices-by-email?email=%s", base, url.QueryEscape(email))
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(u)
	if err != nil {
		return nil, fmt.Errorf("accounthub 请求失败: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != 200 {
		var errResp struct{ Error string `json:"error"` }
		json.Unmarshal(body, &errResp)
		return nil, fmt.Errorf("accounthub %d: %s", resp.StatusCode, errResp.Error)
	}
	var out acchubInvoiceResp
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("accounthub 响应解析失败")
	}
	return &out, nil
}

// SessionBillingCheck POST /api/v1/public/billing/check
// 支持：
//  1. cdk_code — 用兑换时绑定的 session 查账单
//  2. token_input / session — 直接贴 session / accessToken
func SessionBillingCheck(c *gin.Context) {
	var req struct {
		TokenInput string `json:"token_input"`
		Session    string `json:"session"`
		CDKCode    string `json:"cdk_code"`
		Code       string `json:"code"`
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

	// CDK 模式优先走 accounthub：用内部刷新过的 session 查账单
	if source == "cdk" {
		email := extractEmailFromSession(raw)
		if email != "" {
			if inv, err := queryAccounthubInvoices(email); err == nil {
				summary := map[string]interface{}{
					"email": email,
				}
				invoices := inv.Invoices
				if invoices == nil && inv.InvoiceURL != "" {
					invoices = []map[string]interface{}{
						{"hosted_invoice_url": inv.InvoiceURL},
					}
				}
				if invoices == nil {
					invoices = []map[string]interface{}{}
				}
				c.JSON(http.StatusOK, gin.H{
					"summary":     summary,
					"invoices":    invoices,
					"invoice_url": inv.InvoiceURL,
					"auth_source": "accounthub",
				})
				return
			} else {
				log.Printf("[billing] accounthub fallback for %s: %v", email, err)
			}
		}
	}

	// 回退：直接用 session/accessToken 查
	res, err := gptcheck.Check(raw)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"summary":     res.Summary,
		"invoices":    res.Invoices,
		"auth_source": source,
	})
}
