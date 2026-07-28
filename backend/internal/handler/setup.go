package handler

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/gin-gonic/gin"
	"github.com/tuzi/cdk-recharge-system/internal/db"
)

// setup 写接口限流：每 IP 窗口内最多 N 次（防扫端口撞 bootstrap）
const (
	setupMaxAttempts = 5
	setupLockWindow  = time.Hour
)

var (
	setupMu       sync.Mutex
	setupAttempts = map[string]*loginAttempt{}
)

func setupAllowed(ip string) (bool, time.Duration) {
	setupMu.Lock()
	defer setupMu.Unlock()
	a := setupAttempts[ip]
	if a == nil {
		return true, 0
	}
	now := time.Now()
	if now.Before(a.lockedUntil) {
		return false, time.Until(a.lockedUntil)
	}
	if !a.lockedUntil.IsZero() && now.After(a.lockedUntil) {
		delete(setupAttempts, ip)
	}
	return true, 0
}

func setupFailed(ip string) {
	setupMu.Lock()
	defer setupMu.Unlock()
	now := time.Now()
	a := setupAttempts[ip]
	if a == nil || now.Sub(a.windowStart) > setupLockWindow {
		a = &loginAttempt{windowStart: now}
		setupAttempts[ip] = a
	}
	a.failures++
	if a.failures >= setupMaxAttempts {
		a.lockedUntil = now.Add(setupLockWindow)
	}
}

func setupSucceeded(ip string) {
	setupMu.Lock()
	defer setupMu.Unlock()
	delete(setupAttempts, ip)
}

// SetupStatus GET /api/v1/setup/status — 仅返回是否已安装，不泄露细节。
func SetupStatus(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"installed":    db.IsInstalled(),
		"install_mode": db.InstallMode(),
	})
}

type setupBootstrapRequest struct {
	Mode            string `json:"mode"` // generate | manual
	Username        string `json:"username"`
	Password        string `json:"password"`
	ConfirmPassword string `json:"confirm_password"`
}

// SetupBootstrap POST /api/v1/setup/bootstrap — 仅一次；需 X-Setup-Token。
func SetupBootstrap(c *gin.Context) {
	ip := c.ClientIP()
	if ok, wait := setupAllowed(ip); !ok {
		c.JSON(http.StatusTooManyRequests, gin.H{
			"error": fmt.Sprintf("尝试过多，请 %d 分钟后再试", int(wait.Minutes())+1),
		})
		return
	}

	if db.IsInstalled() {
		c.JSON(http.StatusGone, gin.H{"error": "already_installed"})
		return
	}

	if !setupCIDRAllowed(ip) {
		setupFailed(ip)
		c.JSON(http.StatusForbidden, gin.H{"error": "setup not allowed from this address"})
		return
	}

	token := strings.TrimSpace(c.GetHeader("X-Setup-Token"))
	if token == "" {
		token = strings.TrimSpace(c.Query("setup_token"))
	}
	if !db.VerifySetupToken(token) {
		setupFailed(ip)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid setup token"})
		return
	}

	var req setupBootstrapRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	mode := strings.ToLower(strings.TrimSpace(req.Mode))
	if mode == "" {
		mode = "generate"
	}
	if mode != "generate" && mode != "manual" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "mode must be generate or manual"})
		return
	}

	username := strings.TrimSpace(req.Username)
	if username == "" {
		username = db.DefaultAdminUsername
	}
	if !validAdminUsername(username) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username must be 3-32 chars [a-zA-Z0-9_]"})
		return
	}

	var password string
	if mode == "generate" {
		password = db.RandomPassword(16)
	} else {
		password = req.Password
		if password != req.ConfirmPassword {
			c.JSON(http.StatusBadRequest, gin.H{"error": "passwords do not match"})
			return
		}
		if db.IsWeakPassword(password) || strings.EqualFold(password, username) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "password too weak (min 12, not common, not equal to username)"})
			return
		}
		if !hasLetterAndDigit(password) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "password must include letters and digits"})
			return
		}
	}

	if err := db.CreateAdminUser(username, password, "System Admin"); err != nil {
		if strings.Contains(err.Error(), "already_installed") {
			c.JSON(http.StatusGone, gin.H{"error": "already_installed"})
			return
		}
		if strings.Contains(err.Error(), "weak") {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		log.Printf("setup bootstrap error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "setup failed"})
		return
	}

	// 读回 user id，安装成功后立刻签发登录态，避免用户手抄密码再登录失败
	var userID int64
	_ = db.DB.QueryRow(`SELECT id FROM admin_users WHERE username = ?`, username).Scan(&userID)
	if userID == 0 {
		log.Printf("setup bootstrap: admin created but id lookup failed for %s", username)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "setup failed"})
		return
	}

	// 自检：生成口令必须能通过校验，否则回滚并报错（防止“生成成功但登不进”）
	var storedHash string
	if err := db.DB.QueryRow(`SELECT password_hash FROM admin_users WHERE id = ?`, userID).Scan(&storedHash); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "setup failed"})
		return
	}
	if ok, _ := db.VerifyAdminPassword(storedHash, password); !ok {
		log.Printf("setup bootstrap: password self-check failed for %s", username)
		_, _ = db.DB.Exec(`DELETE FROM admin_users WHERE id = ?`, userID)
		_, _ = db.DB.Exec(`DELETE FROM site_settings WHERE key IN ('install_completed_at','setup_token_hash')`)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "password self-check failed, please retry"})
		return
	}

	sess, err := issueAdminSession(c, userID, username, "System Admin")
	if err != nil {
		log.Printf("setup bootstrap session error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "setup failed"})
		return
	}

	setupSucceeded(ip)
	// 密码仅在本响应返回一次；同时已下发 cookie，前端可直接进后台
	c.JSON(http.StatusOK, gin.H{
		"username":   username,
		"password":   password,
		"token":      sess["token"],
		"name":       sess["name"],
		"expires_at": sess["expires_at"],
		"csrf_token": sess["csrf_token"],
		"auto_login": true,
		"message":    "安装成功。已自动登录；请立即保存密码以备下次使用",
	})
}

func validAdminUsername(u string) bool {
	if len(u) < 3 || len(u) > 32 {
		return false
	}
	for _, r := range u {
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_') {
			return false
		}
	}
	return true
}

func hasLetterAndDigit(s string) bool {
	var letter, digit bool
	for _, r := range s {
		if unicode.IsLetter(r) {
			letter = true
		}
		if unicode.IsDigit(r) {
			digit = true
		}
	}
	return letter && digit
}

// setupCIDRAllowed：SETUP_ALLOW_CIDRS 逗号分隔；空则允许全部（由 setup token 兜底）。
// 生产建议设 127.0.0.1,::1 或管理机出口。
func setupCIDRAllowed(ipStr string) bool {
	raw := strings.TrimSpace(os.Getenv("SETUP_ALLOW_CIDRS"))
	if raw == "" {
		return true
	}
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.Contains(part, "/") {
			_, n, err := net.ParseCIDR(part)
			if err == nil && n.Contains(ip) {
				return true
			}
			continue
		}
		if p := net.ParseIP(part); p != nil && p.Equal(ip) {
			return true
		}
	}
	return false
}
