package handler

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	authCookieName = "auth_token"
	csrfCookieName = "csrf_token"
)

// isSecureRequest 判断当前请求是否走 HTTPS（直连 TLS 或反代标记）。
func isSecureRequest(c *gin.Context) bool {
	return c.Request.TLS != nil || strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https")
}

// newCSRFToken 生成随机 CSRF token。
func newCSRFToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return hex.EncodeToString(b)
}

// setAuthCookies 下发 HttpOnly 的认证 cookie 与可被前端读取的 CSRF cookie。
func setAuthCookies(c *gin.Context, token, csrf string, maxAgeSeconds int) {
	secure := isSecureRequest(c)
	c.SetSameSite(http.SameSiteLaxMode)
	// auth_token：HttpOnly，JS 读不到，规避 XSS 偷 token
	c.SetCookie(authCookieName, token, maxAgeSeconds, "/", "", secure, true)
	// csrf_token：非 HttpOnly，供前端读取后放进 X-CSRF-Token 头做双提交校验
	c.SetCookie(csrfCookieName, csrf, maxAgeSeconds, "/", "", secure, false)
}

// clearAuthCookies 清除认证相关 cookie（登出）。
func clearAuthCookies(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(authCookieName, "", -1, "/", "", false, true)
	c.SetCookie(csrfCookieName, "", -1, "/", "", false, false)
}
