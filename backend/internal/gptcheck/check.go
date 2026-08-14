// Package gptcheck 用 ChatGPT accessToken/session 查订阅与账单链接。
// 逻辑对齐 cardplatform/browser-extension 小助手（/gpt/check → invoices）。
package gptcheck

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"

	fhttp "github.com/bogdanfinn/fhttp"
	tls_client "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
)

const (
	checkV4URL     = "https://chatgpt.com/backend-api/accounts/check/v4-2023-04-27?timezone_offset_min=-540"
	invoicesURL    = "https://chatgpt.com/backend-api/invoices?limit=20&account_id="
	sessionAuthURL = "https://chatgpt.com/api/auth/session"
	ua             = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
	maxBodyBytes   = 4 << 20
	chunkSize      = 3933
)

var (
	clientOnce sync.Once
	client     tls_client.HttpClient
	clientErr  error
)

func getClient() (tls_client.HttpClient, error) {
	clientOnce.Do(func() {
		opts := []tls_client.HttpClientOption{
			tls_client.WithTimeoutSeconds(30),
			tls_client.WithClientProfile(profiles.Chrome_131),
			tls_client.WithForceHttp1(),
			tls_client.WithNotFollowRedirects(),
		}
		client, clientErr = tls_client.NewHttpClient(tls_client.NewNoopLogger(), opts...)
	})
	return client, clientErr
}

func request(method, rawURL string, headers map[string]string) (int, []byte, error) {
	return requestWithCookies(method, rawURL, headers, nil)
}

func requestWithCookies(method, rawURL string, headers map[string]string, cookies []*fhttp.Cookie) (int, []byte, error) {
	c, err := getClient()
	if err != nil {
		return 0, nil, err
	}
	req, err := fhttp.NewRequest(method, rawURL, nil)
	if err != nil {
		return 0, nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	for _, ck := range cookies {
		req.AddCookie(ck)
	}
	resp, err := c.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	return resp.StatusCode, b, err
}

// ParseAccessToken 接受 session JSON 或裸 accessToken。
func ParseAccessToken(raw string) (string, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return "", fmt.Errorf("请输入 session JSON 或 accessToken")
	}
	if strings.HasPrefix(v, "{") {
		var data map[string]interface{}
		if err := json.Unmarshal([]byte(v), &data); err != nil {
			return "", fmt.Errorf("session JSON 解析失败")
		}
		for _, k := range []string{"accessToken", "access_token"} {
			if t, _ := data[k].(string); strings.TrimSpace(t) != "" {
				return strings.TrimSpace(t), nil
			}
		}
		return "", fmt.Errorf("JSON 中未找到 accessToken")
	}
	return v, nil
}

// parseSessionToken 从 session JSON 提取 sessionToken。
func parseSessionToken(raw string) string {
	v := strings.TrimSpace(raw)
	if !strings.HasPrefix(v, "{") {
		return ""
	}
	var data map[string]interface{}
	if json.Unmarshal([]byte(v), &data) != nil {
		return ""
	}
	for _, k := range []string{"sessionToken", "session_token"} {
		if t, _ := data[k].(string); strings.TrimSpace(t) != "" {
			return strings.TrimSpace(t)
		}
	}
	return ""
}

// sessionTokenCookies 把 sessionToken 转成 __Secure-next-auth.session-token cookie 数组（自动分片）。
func sessionTokenCookies(token string) []*fhttp.Cookie {
	base := "__Secure-next-auth.session-token"
	if len(token) <= chunkSize {
		return []*fhttp.Cookie{{Name: base, Value: token}}
	}
	var cookies []*fhttp.Cookie
	for i, idx := 0, 0; i < len(token); i, idx = i+chunkSize, idx+1 {
		end := i + chunkSize
		if end > len(token) {
			end = len(token)
		}
		cookies = append(cookies, &fhttp.Cookie{
			Name:  fmt.Sprintf("%s.%d", base, idx),
			Value: token[i:end],
		})
	}
	return cookies
}

// refreshAccessToken 用 sessionToken 换取新 accessToken。
func refreshAccessToken(sessionToken string) (string, error) {
	cookies := sessionTokenCookies(sessionToken)
	st, body, err := requestWithCookies(fhttp.MethodGet, sessionAuthURL, map[string]string{
		"Accept":     "application/json",
		"User-Agent": ua,
		"Referer":    "https://chatgpt.com/",
	}, cookies)
	if err != nil {
		return "", fmt.Errorf("刷新 session 失败: %v", err)
	}
	if st != 200 {
		return "", fmt.Errorf("刷新 session 失败 HTTP %d", st)
	}
	var data map[string]interface{}
	if json.Unmarshal(body, &data) != nil {
		return "", fmt.Errorf("刷新 session 返回非 JSON")
	}
	token := firstString(data["accessToken"], data["access_token"])
	if token == "" {
		return "", fmt.Errorf("刷新 session 未返回 accessToken")
	}
	return token, nil
}

func asMap(v interface{}) map[string]interface{} {
	if m, ok := v.(map[string]interface{}); ok {
		return m
	}
	return map[string]interface{}{}
}

func firstString(vals ...interface{}) string {
	for _, v := range vals {
		if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func defaultAccount(data map[string]interface{}) map[string]interface{} {
	accounts := asMap(data["accounts"])
	if def := firstString(data["default_account"]); def != "" {
		if a := asMap(accounts[def]); len(a) > 0 {
			return a
		}
	}
	for _, v := range accounts {
		if a := asMap(v); len(a) > 0 {
			return a
		}
	}
	return map[string]interface{}{}
}

func trustedURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" {
		return ""
	}
	host := strings.ToLower(u.Host)
	if strings.Contains(host, "stripe.com") || strings.Contains(host, "openai.com") || strings.Contains(host, "chatgpt.com") {
		return raw
	}
	return ""
}

// Result 订阅摘要 + 账单链接
type Result struct {
	Summary  map[string]interface{}   `json:"summary"`
	Invoices []map[string]interface{} `json:"invoices"`
}

// Check 对齐小助手 gptCheck：订阅 + invoices（含 hosted_invoice_url / invoice_pdf）。
// 若 accessToken 已过期（403/401），自动用 sessionToken 刷新后重试一次。
func Check(tokenInput string) (*Result, error) {
	accessToken, err := ParseAccessToken(tokenInput)
	if err != nil {
		return nil, err
	}

	checkHeaders := map[string]string{
		"Authorization": "Bearer " + accessToken,
		"Accept":        "application/json",
		"User-Agent":    ua,
		"Referer":       "https://chatgpt.com/",
	}
	st, body, err := request(http.MethodGet, checkV4URL, checkHeaders)
	if err != nil {
		return nil, fmt.Errorf("请求 ChatGPT 失败: %v", err)
	}

	// accessToken 过期/无效：尝试用 sessionToken 刷新
	if st == 401 || st == 403 {
		sessionToken := parseSessionToken(tokenInput)
		if sessionToken != "" {
			if fresh, rerr := refreshAccessToken(sessionToken); rerr == nil && fresh != "" {
				accessToken = fresh
				checkHeaders["Authorization"] = "Bearer " + accessToken
				st, body, err = request(http.MethodGet, checkV4URL, checkHeaders)
				if err != nil {
					return nil, fmt.Errorf("请求 ChatGPT 失败: %v", err)
				}
			}
		}
	}

	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("check 返回非 JSON")
	}
	if st != http.StatusOK {
		return nil, fmt.Errorf("查询失败(token 可能已失效) HTTP %d", st)
	}
	account := defaultAccount(data)
	accountInfo := asMap(account["account"])
	entitlement := asMap(account["entitlement"])
	lastActive := asMap(account["last_active_subscription"])
	accountID := firstString(accountInfo["account_id"], accountInfo["id"])

	summary := map[string]interface{}{
		"plan_type":               accountInfo["plan_type"],
		"subscription_plan":       entitlement["subscription_plan"],
		"has_active_subscription": entitlement["has_active_subscription"],
		"expires_at":              entitlement["expires_at"],
		"renews_at":               entitlement["renews_at"],
		"cancels_at":              entitlement["cancels_at"],
		"billing_currency":        entitlement["billing_currency"],
		"billing_period":          entitlement["billing_period"],
		"will_renew":              lastActive["will_renew"],
		"account_id":              accountID,
	}
	if active, ok := summary["has_active_subscription"].(bool); ok && !active {
		summary["plan_type"] = "free"
	}

	invoices := []map[string]interface{}{}
	if accountID != "" {
		invoices = fetchInvoices(accessToken, accountID)
	}
	return &Result{Summary: summary, Invoices: invoices}, nil
}

func fetchInvoices(accessToken, accountID string) []map[string]interface{} {
	st, body, err := request(http.MethodGet, invoicesURL+url.QueryEscape(accountID), map[string]string{
		"Authorization": "Bearer " + accessToken,
		"Accept":        "application/json",
		"User-Agent":    ua,
		"Referer":       "https://chatgpt.com/",
	})
	if err != nil || st != http.StatusOK {
		return nil
	}
	var data map[string]interface{}
	if json.Unmarshal(body, &data) != nil {
		return nil
	}
	rawList, _ := data["data"].([]interface{})
	out := make([]map[string]interface{}, 0, len(rawList))
	for _, it := range rawList {
		inv := asMap(it)
		if len(inv) == 0 {
			continue
		}
		var desc interface{}
		if ld, ok := asMap(inv["lines"])["data"].([]interface{}); ok && len(ld) > 0 {
			desc = asMap(ld[0])["description"]
		}
		out = append(out, map[string]interface{}{
			"id":                 inv["id"],
			"number":             inv["number"],
			"status":             inv["status"],
			"paid":               inv["paid"],
			"currency":           inv["currency"],
			"total":              inv["total"],
			"amount_paid":        inv["amount_paid"],
			"created":            inv["created"],
			"description":        desc,
			"hosted_invoice_url": trustedURL(firstString(inv["hosted_invoice_url"])),
			"invoice_pdf":        trustedURL(firstString(inv["invoice_pdf"])),
		})
	}
	return out
}
