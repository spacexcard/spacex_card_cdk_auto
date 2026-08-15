package handler

import (
	"crypto/md5"
	"crypto/rand"
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/tuzi/cdk-recharge-system/internal/auth"
	"github.com/tuzi/cdk-recharge-system/internal/cardplatform"
	"github.com/tuzi/cdk-recharge-system/internal/db"
	"github.com/tuzi/cdk-recharge-system/internal/notify"
)

// GenerateTaskID generates a unique task ID
func GenerateTaskID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return fmt.Sprintf("TASK-%s", hex.EncodeToString(b)[:16])
}

// HashSession creates a hash of session for storage
func HashSession(session string) string {
	// 去除首尾空白，保证保存/查询两侧对同一 session 生成一致的哈希
	hash := md5.Sum([]byte(strings.TrimSpace(session)))
	return hex.EncodeToString(hash[:])
}

func GenerateCDKCode(planType string) string {
	normalized := normalizeCDKPlanType(planType)
	seed := fmt.Sprintf("%s-%d-%s", normalized, time.Now().UnixNano(), randomCodeSegment(12))
	sum := sha1.Sum([]byte(seed))
	hexID := hex.EncodeToString(sum[:16])
	return fmt.Sprintf(
		"%s-%s-%s-%s-%s",
		hexID[0:8],
		hexID[8:12],
		hexID[12:16],
		hexID[16:20],
		hexID[20:32],
	)
}

func randomCodeSegment(length int) string {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	buffer := make([]byte, length)
	if _, err := rand.Read(buffer); err != nil {
		return strings.Repeat("X", length)
	}

	output := make([]byte, length)
	for i, value := range buffer {
		output[i] = alphabet[int(value)%len(alphabet)]
	}

	return string(output)
}

func parseLimit(raw string, fallback int) int {
	if raw == "" {
		return fallback
	}

	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}

	if value > 500 {
		return 500
	}

	return value
}

func normalizeCDKPlanType(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	value = strings.ReplaceAll(value, "-", "")
	value = strings.ReplaceAll(value, "_", "")
	value = strings.ReplaceAll(value, " ", "")

	switch value {
	case "pro", "gptpro":
		return "pro"
	case "5x", "gpt5x", "5":
		return "5x"
	case "20x", "gpt20x", "20":
		return "20x"
	default:
		return ""
	}
}

// VerifyCDKRequest request for CDK verification
type VerifyCDKRequest struct {
	CDKCode string `json:"cdk_code" binding:"required"`
}

// VerifyCDKResponse response for CDK verification
type VerifyCDKResponse struct {
	Valid     bool   `json:"valid"`
	PlanType  string `json:"plan_type,omitempty"`
	Error     string `json:"error,omitempty"`
	ExpiresAt string `json:"expires_at,omitempty"`
}

// CreateTaskRequest request for creating a recharge task
type CreateTaskRequest struct {
	CDKCode   string `json:"cdk_code" binding:"required"`
	SessionJSON string `json:"session_json" binding:"required"`
}

// ConfirmTaskRequest request for confirming a task
type ConfirmTaskRequest struct {
	TaskID string `json:"task_id" binding:"required"`
}

// BillingInfo represents billing information
type BillingInfo struct {
	AccountEmail       string `json:"account_email"`
	SubscriptionStatus string `json:"subscription_status"`
	PlanType          string `json:"plan_type"`
	BillingAmount     float64 `json:"billing_amount"`
	Currency          string `json:"currency"`
	NextBillingDate   string `json:"next_billing_date,omitempty"`
}

type AdminLoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type AdminLoginResponse struct {
	Token     string `json:"token"`
	Username  string `json:"username"`
	Name      string `json:"name"`
	ExpiresAt string `json:"expires_at"`
}

// ===== Recharge Portal APIs =====

func AdminLogin(c *gin.Context) {
	var req AdminLoginRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	// 登录限流：按 客户端IP + 用户名 计数
	rateKey := c.ClientIP() + "|" + strings.ToLower(strings.TrimSpace(req.Username))
	if ok, wait := loginAllowed(rateKey); !ok {
		c.JSON(http.StatusTooManyRequests, gin.H{
			"error": fmt.Sprintf("尝试次数过多，请 %d 分钟后再试", int(wait.Minutes())+1),
		})
		return
	}

	var userID int64
	var username string
	var passwordHash string
	var displayName sql.NullString
	var isActive int

	err := db.DB.QueryRow(`
		SELECT id, username, password_hash, display_name, is_active
		FROM admin_users
		WHERE username = ?
	`, strings.TrimSpace(req.Username)).Scan(&userID, &username, &passwordHash, &displayName, &isActive)
	if err == sql.ErrNoRows {
		loginFailed(rateKey)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "用户名或密码错误"})
		return
	}
	if err != nil {
		log.Printf("admin login lookup error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "系统错误"})
		return
	}
	if isActive != 1 {
		c.JSON(http.StatusForbidden, gin.H{"error": "账号已被禁用"})
		return
	}
	// 密码两侧空白常因复制粘贴产生，校验时 Trim；入库仍按原样（生成口令无空白）
	plain := strings.TrimSpace(req.Password)
	ok, upgradedHash := db.VerifyAdminPassword(passwordHash, plain)
	if !ok {
		loginFailed(rateKey)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "用户名或密码错误"})
		return
	}
	loginSucceeded(rateKey)
	// 旧 SHA-256 哈希登录成功后，无感升级为 bcrypt
	db.UpgradeAdminHash(username, upgradedHash)
	db.WriteAudit(username, "login", "", c.ClientIP())

	name := username
	if displayName.Valid && strings.TrimSpace(displayName.String) != "" {
		name = displayName.String
	}
	sess, err := issueAdminSession(c, userID, username, name)
	if err != nil {
		log.Printf("admin login session error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "系统错误"})
		return
	}
	c.JSON(http.StatusOK, sess)
}

// issueAdminSession 签发 JWT + HttpOnly cookie，供登录与安装向导共用。
func issueAdminSession(c *gin.Context, userID int64, username, displayName string) (gin.H, error) {
	expiration := time.Now().Add(24 * time.Hour)
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, auth.CustomClaims{
		UserID:   userID,
		IsAdmin:  true,
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiration),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Subject:   username,
		},
	})
	signedToken, err := token.SignedString([]byte(auth.JWTSecret()))
	if err != nil {
		return nil, err
	}
	if displayName == "" {
		displayName = username
	}
	csrf := newCSRFToken()
	setAuthCookies(c, signedToken, csrf, int(time.Until(expiration).Seconds()))
	return gin.H{
		"token":      signedToken,
		"username":   username,
		"name":       displayName,
		"expires_at": expiration.Format(time.RFC3339),
		"csrf_token": csrf,
	}, nil
}

// AdminLogout 清除认证 cookie。
func AdminLogout(c *gin.Context) {
	clearAuthCookies(c)
	c.JSON(http.StatusOK, gin.H{"message": "已退出登录"})
}

func AdminMe(c *gin.Context) {
	userID, _ := c.Get("user_id")
	username, _ := c.Get("username")
	isAdmin, _ := c.Get("is_admin")

	c.JSON(http.StatusOK, gin.H{
		"user_id":  userID,
		"username": username,
		"is_admin": isAdmin,
	})
}

// AdminChangePassword 让已登录管理员修改自己的密码。
func AdminChangePassword(c *gin.Context) {
	type ChangePasswordRequest struct {
		OldPassword string `json:"old_password" binding:"required"`
		NewPassword string `json:"new_password" binding:"required"`
	}

	var req ChangePasswordRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	if len(req.NewPassword) < 12 || db.IsWeakPassword(req.NewPassword) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "新密码至少 12 位，且不能为常见弱口令"})
		return
	}
	if req.NewPassword == req.OldPassword {
		c.JSON(http.StatusBadRequest, gin.H{"error": "新密码不能与旧密码相同"})
		return
	}

	usernameVal, _ := c.Get("username")
	username, _ := usernameVal.(string)
	if username == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未识别的登录身份"})
		return
	}

	var storedHash string
	if err := db.DB.QueryRow(`SELECT password_hash FROM admin_users WHERE username = ?`, username).Scan(&storedHash); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "系统错误"})
		return
	}

	if ok, _ := db.VerifyAdminPassword(storedHash, req.OldPassword); !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "旧密码不正确"})
		return
	}

	if err := db.SetAdminPassword(username, req.NewPassword); err != nil {
		log.Printf("change password error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "修改密码失败"})
		return
	}

	auditAdmin(c, "change_password", "")
	c.JSON(http.StatusOK, gin.H{"message": "密码已修改，请用新密码重新登录"})
}

// auditAdmin 记录当前登录管理员的一次操作。
func auditAdmin(c *gin.Context, action, detail string) {
	u, _ := c.Get("username")
	username, _ := u.(string)
	db.WriteAudit(username, action, detail, c.ClientIP())
}

// ListAuditLogs 返回最近的管理员操作审计日志。
func ListAuditLogs(c *gin.Context) {
	limit := parseLimit(c.Query("limit"), 200)
	rows, err := db.DB.Query(`
		SELECT id, COALESCE(username,''), COALESCE(action,''), COALESCE(detail,''), COALESCE(ip,''), COALESCE(created_at,'')
		FROM admin_audit_logs
		ORDER BY id DESC LIMIT ?
	`, limit)
	if err != nil {
		log.Printf("list audit error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list audit logs"})
		return
	}
	defer rows.Close()

	type AuditRecord struct {
		ID        int64  `json:"id"`
		Username  string `json:"username"`
		Action    string `json:"action"`
		Detail    string `json:"detail"`
		IP        string `json:"ip"`
		CreatedAt string `json:"created_at"`
	}
	logs := make([]AuditRecord, 0)
	for rows.Next() {
		var item AuditRecord
		if err := rows.Scan(&item.ID, &item.Username, &item.Action, &item.Detail, &item.IP, &item.CreatedAt); err != nil {
			continue
		}
		logs = append(logs, item)
	}
	c.JSON(http.StatusOK, gin.H{"logs": logs})
}

// VerifyCDK verifies a CDK code
func VerifyCDK(c *gin.Context) {
	var req VerifyCDKRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, VerifyCDKResponse{Valid: false, Error: "Invalid request"})
		return
	}
	req.CDKCode = strings.TrimSpace(req.CDKCode)

	var planType, status string
	var expiresAt sql.NullTime

	err := db.DB.QueryRow(`
		SELECT plan_type, status, expires_at
		FROM cd_keys
		WHERE code = ?
	`, req.CDKCode).Scan(&planType, &status, &expiresAt)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusOK, VerifyCDKResponse{
			Valid: false,
			Error: "CDK不存在",
		})
		return
	}

	if err != nil {
		log.Printf("DB error: %v", err)
		c.JSON(http.StatusOK, VerifyCDKResponse{
			Valid: false,
			Error: "系统错误",
		})
		return
	}

	if status == "used" {
		c.JSON(http.StatusOK, VerifyCDKResponse{
			Valid: false,
			Error: "CDK已被使用",
		})
		return
	}

	if status == "disabled" {
		c.JSON(http.StatusOK, VerifyCDKResponse{
			Valid: false,
			Error: "CDK已禁用",
		})
		return
	}

	if expiresAt.Valid && expiresAt.Time.Before(time.Now()) {
		if status != "expired" {
			_, _ = db.DB.Exec(`UPDATE cd_keys SET status = 'expired' WHERE code = ?`, req.CDKCode)
		}
		c.JSON(http.StatusOK, VerifyCDKResponse{
			Valid: false,
			Error: "CDK已过期",
		})
		return
	}

	resp := VerifyCDKResponse{
		Valid:    true,
		PlanType: planType,
	}
	if expiresAt.Valid {
		resp.ExpiresAt = expiresAt.Time.Format("2006-01-02")
	}
	c.JSON(http.StatusOK, resp)
}

// CreateRechargeTask creates a new recharge task
func CreateRechargeTask(c *gin.Context) {
	var req CreateTaskRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}
	req.CDKCode = strings.TrimSpace(req.CDKCode)

	// Verify CDK exists and is active
	var status string
	err := db.DB.QueryRow(`
		SELECT status FROM cd_keys WHERE code = ?
	`, req.CDKCode).Scan(&status)

	if err != nil || status != "active" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "CDK不可用"})
		return
	}

	// Generate task ID
	taskID := GenerateTaskID()

	// Create task
	_, err = db.DB.Exec(`
		INSERT INTO recharge_tasks (task_id, cdk_code, session_json, task_status, created_at, updated_at)
		VALUES (?, ?, ?, 'pending', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, taskID, req.CDKCode, req.SessionJSON)

	if err != nil {
		log.Printf("DB error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create task"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"task_id": taskID,
		"status": "pending",
		"message": "任务创建成功，请确认信息",
	})
}

// ConfirmRechargeTask confirms and submits a recharge task
func ConfirmRechargeTask(c *gin.Context) {
	var req ConfirmTaskRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}
	req.TaskID = strings.TrimSpace(req.TaskID)

	// Get task info
	var cdkCode string
	var sessionJSON sql.NullString
	err := db.DB.QueryRow(`
		SELECT cdk_code, session_json FROM recharge_tasks WHERE task_id = ? AND task_status = 'pending'
	`, req.TaskID).Scan(&cdkCode, &sessionJSON)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Task not found or already confirmed"})
		return
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "System error"})
		return
	}

	// Begin transaction
	tx, err := db.DB.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start transaction"})
		return
	}
	defer tx.Rollback()

	// Update task status to submitted
	_, err = tx.Exec(`
		UPDATE recharge_tasks
		SET task_status = 'submitted', updated_at = CURRENT_TIMESTAMP
		WHERE task_id = ?
	`, req.TaskID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update task"})
		return
	}

	// Mark CDK as used
	_, err = tx.Exec(`
		UPDATE cd_keys
		SET status = 'used', used_at = CURRENT_TIMESTAMP
		WHERE code = ?
	`, cdkCode)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to mark CDK"})
		return
	}

	if err = tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to commit"})
		return
	}

	// 订单成交：CDK 已使用 + session 已提交 → 推送 Telegram 通知（异步，不阻塞响应）
	notify.NotifyNewOrder(req.TaskID, cdkCode, sessionJSON.String)

	c.JSON(http.StatusOK, gin.H{
		"task_id": req.TaskID,
		"status": "submitted",
		"message": "充值申请已提交，请等待处理",
	})
}

// ===== 卡密状态查询（公开）=====
// 只返回：是否已用 + 充值邮箱（及 plan/时间）。不返回 token / session / 任务进度细节。

// LookupCDKStatus GET /api/v1/lookup/cdk?code=  或兼容 ?cdk_code=
func LookupCDKStatus(c *gin.Context) {
	code := strings.TrimSpace(c.Query("code"))
	if code == "" {
		code = strings.TrimSpace(c.Query("cdk_code"))
	}
	code = strings.TrimSpace(code)
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请输入卡密"})
		return
	}

	type out struct {
		CDKCode      string  `json:"cdk_code"`
		Status       string  `json:"status"` // unused | used | disabled | expired | processing | unknown
		Used         bool    `json:"used"`
		AccountEmail string  `json:"account_email,omitempty"`
		Plan         string  `json:"plan,omitempty"`
		UsedAt       *string `json:"used_at,omitempty"`
		Message      string  `json:"message"`
	}
	resp := out{CDKCode: code, Status: "unknown", Message: "未找到该卡密记录"}

	// 1) 本站旧 cd_keys
	var planType, keyStatus string
	var usedAt, expiresAt sql.NullTime
	err := db.DB.QueryRow(`
		SELECT COALESCE(plan_type,''), COALESCE(status,''), used_at, expires_at
		FROM cd_keys WHERE upper(trim(code)) = upper(trim(?))
	`, code).Scan(&planType, &keyStatus, &usedAt, &expiresAt)
	if err == nil {
		resp.Plan = planType
		switch strings.ToLower(keyStatus) {
		case "used":
			resp.Status, resp.Used = "used", true
			resp.Message = "卡密已使用"
		case "disabled":
			resp.Status, resp.Used = "disabled", false
			resp.Message = "卡密已禁用"
		case "expired":
			resp.Status, resp.Used = "expired", false
			resp.Message = "卡密已过期"
		case "active", "":
			resp.Status, resp.Used = "unused", false
			resp.Message = "卡密未使用"
		default:
			resp.Status = strings.ToLower(keyStatus)
			resp.Message = "卡密状态：" + keyStatus
		}
		if expiresAt.Valid && expiresAt.Time.Before(time.Now()) && resp.Status == "unused" {
			resp.Status, resp.Message = "expired", "卡密已过期"
		}
		if usedAt.Valid {
			s := usedAt.Time.Format("2006-01-02 15:04:05")
			resp.UsedAt = &s
		}
	} else if err != sql.ErrNoRows {
		log.Printf("[lookup-cdk] cd_keys: %v", err)
	}

	// 2) 本站完整码缓存 cardplatform_cdk_codes
	var cpStatus, cpPlan string
	err = db.DB.QueryRow(`
		SELECT COALESCE(status,''), COALESCE(plan,'')
		FROM cardplatform_cdk_codes WHERE upper(trim(code)) = upper(trim(?))
		ORDER BY created_at DESC LIMIT 1
	`, code).Scan(&cpStatus, &cpPlan)
	if err == nil {
		if resp.Plan == "" {
			resp.Plan = cpPlan
		}
		st := strings.ToLower(strings.TrimSpace(cpStatus))
		if st == "" {
			st = "unused"
		}
		// 缓存状态优先覆盖 unknown；used 优先于 unused
		switch st {
		case "used", "redeemed", "consumed":
			resp.Status, resp.Used = "used", true
			resp.Message = "卡密已使用"
		case "disabled":
			if resp.Status != "used" {
				resp.Status, resp.Used = "disabled", false
				resp.Message = "卡密已禁用"
			}
		case "unused", "active":
			if resp.Status == "unknown" {
				resp.Status, resp.Used = "unused", false
				resp.Message = "卡密未使用"
			}
		}
	}

	// 3) 旧 recharge_tasks：取最近一单邮箱与状态
	var taskStatus string
	var accountEmail sql.NullString
	var taskCompleted sql.NullTime
	err = db.DB.QueryRow(`
		SELECT COALESCE(task_status,''), account_email, completed_at
		FROM recharge_tasks
		WHERE upper(trim(cdk_code)) = upper(trim(?))
		ORDER BY created_at DESC LIMIT 1
	`, code).Scan(&taskStatus, &accountEmail, &taskCompleted)
	if err == nil {
		if accountEmail.Valid && strings.TrimSpace(accountEmail.String) != "" {
			resp.AccountEmail = strings.TrimSpace(accountEmail.String)
		}
		ts := strings.ToLower(taskStatus)
		switch ts {
		case "completed", "success", "done":
			resp.Status, resp.Used = "used", true
			resp.Message = "卡密已使用"
			if taskCompleted.Valid {
				s := taskCompleted.Time.Format("2006-01-02 15:04:05")
				resp.UsedAt = &s
			}
		case "pending", "submitted", "running", "queued", "processing":
			if resp.Status != "used" {
				resp.Status, resp.Used = "processing", false
				resp.Message = "卡密兑换处理中"
			}
		case "failed", "declined", "cancelled":
			// 失败不一定等于未用；若尚无 used 标记则保持原状态
			if resp.Status == "unknown" || resp.Status == "unused" {
				resp.Message = "存在失败记录，卡密可能仍可用或已核销，请以实际兑换结果为准"
			}
		}
	}

	// 4) 绑定表：有 session 可提取邮箱；有 redemption_token 可问卡台订单状态（响应中剥离 token）
	if bind, berr := db.GetBindingByCDK(code); berr == nil && bind != nil {
		if resp.AccountEmail == "" && strings.TrimSpace(bind.SessionPayload) != "" {
			if em := extractEmailFromSession(bind.SessionPayload); em != "" {
				resp.AccountEmail = em
			}
		}
		if tok := strings.TrimSpace(bind.RedemptionToken); tok != "" {
			cli := cardplatform.NewFromSettings()
			st, raw, rerr := cli.Result(c.Request.Context(), tok, deviceFrom(c))
			if rerr == nil && st >= 200 && st < 300 && len(raw) > 0 {
				var payload map[string]any
				if json.Unmarshal(raw, &payload) == nil && payload != nil {
					orderStatus := strAny(payload["status"])
					if orderStatus == "" {
						if order, ok := payload["order"].(map[string]any); ok {
							orderStatus = strAny(order["status"])
						}
					}
					email := strAny(payload["account_email"])
					if email == "" {
						if order, ok := payload["order"].(map[string]any); ok {
							email = strAny(order["account_email"])
						}
					}
					if email != "" {
						resp.AccountEmail = email
					}
					os := strings.ToLower(orderStatus)
					switch {
					case os == "completed" || os == "success" || os == "done" || os == "paid":
						resp.Status, resp.Used = "used", true
						resp.Message = "卡密已使用"
					case os == "failed" || os == "declined" || os == "cancelled":
						if resp.Status != "used" {
							resp.Message = "兑换曾失败，请确认卡密是否仍可用"
						}
					case os != "":
						if resp.Status != "used" {
							resp.Status, resp.Used = "processing", false
							resp.Message = "卡密兑换处理中"
						}
					}
				}
			}
		}
	}

	// 有邮箱则视为至少进入过兑换
	if resp.AccountEmail != "" && resp.Status == "unused" {
		resp.Status, resp.Used = "used", true
		resp.Message = "卡密已使用"
	}
	if resp.Status == "used" {
		resp.Used = true
		if resp.Message == "" || resp.Message == "未找到该卡密记录" {
			resp.Message = "卡密已使用"
		}
	}

	// 完全查无
	if resp.Status == "unknown" {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "未找到该卡密记录",
			"cdk_code": code,
			"status":  "unknown",
			"used":    false,
			"message": "未找到该卡密。请确认输入完整卡密。",
		})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// ===== Billing Tool APIs =====

// QueryBillingInfo queries billing information by session
func QueryBillingInfo(c *gin.Context) {
	sessionJSON := strings.TrimSpace(c.Query("session"))
	if sessionJSON == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Session required"})
		return
	}

	sessionHash := HashSession(sessionJSON)

	var info BillingInfo
	var nextBillingDate sql.NullTime

	err := db.DB.QueryRow(`
		SELECT account_email, subscription_status, plan_type, billing_amount, currency, next_billing_date
		FROM billing_records
		WHERE session_hash = ?
	`, sessionHash).Scan(&info.AccountEmail, &info.SubscriptionStatus, &info.PlanType,
		&info.BillingAmount, &info.Currency, &nextBillingDate)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Billing information not found"})
		return
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "System error"})
		return
	}

	if nextBillingDate.Valid {
		info.NextBillingDate = nextBillingDate.Time.Format("2006-01-02")
	}

	c.JSON(http.StatusOK, info)
}

// SaveBillingInfo saves billing information
func SaveBillingInfo(c *gin.Context) {
	type SaveBillingRequest struct {
		SessionJSON        string  `json:"session_json" binding:"required"`
		AccountEmail       string  `json:"account_email" binding:"required"`
		SubscriptionStatus string `json:"subscription_status"`
		PlanType           string  `json:"plan_type"`
		BillingAmount      float64 `json:"billing_amount"`
		Currency           string  `json:"currency"`
		NextBillingDate    string  `json:"next_billing_date"`
	}

	var req SaveBillingRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	sessionHash := HashSession(req.SessionJSON)
	currency := strings.ToUpper(strings.TrimSpace(req.Currency))
	if currency == "" {
		currency = "USD"
	}

	var nextBillingValue interface{}
	if strings.TrimSpace(req.NextBillingDate) != "" {
		parsed, err := time.Parse("2006-01-02", strings.TrimSpace(req.NextBillingDate))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "next_billing_date must use YYYY-MM-DD"})
			return
		}
		nextBillingValue = parsed.Format("2006-01-02")
	}

	_, err := db.DB.Exec(`
		INSERT OR REPLACE INTO billing_records
		(session_hash, account_email, subscription_status, plan_type, billing_amount, currency, next_billing_date, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	`, sessionHash, req.AccountEmail, req.SubscriptionStatus, req.PlanType, req.BillingAmount, currency, nextBillingValue)

	if err != nil {
		log.Printf("DB error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save billing info"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Billing information saved"})
}

// ===== System Stats =====

// GetSystemStats 管理后台总览：CDK / 兑换订单以卡台 Open API 为准（不再读本地过渡表）。
func GetSystemStats(c *gin.Context) {
	out := gin.H{
		"source":            "cardplatform",
		"configured":        false,
		"total_cdks":        0,
		"unused_cdks":       0,
		"active_cdks":       0, // 兼容旧字段 = unused
		"active_cdkeys":     0, // 兼容前端曾用的字段名
		"total_cdkeys":      0, // 兼容
		"reserved_cdks":     0,
		"consumed_cdks":     0,
		"frozen_cdks":       0,
		"disabled_cdks":     0,
		"total_cdk_orders":  0,
		"webhook_events":    0,
		"counts_partial":    false,
		"error":             "",
	}

	var webhookCount int
	if db.DB != nil {
		_ = db.DB.QueryRow(`SELECT COUNT(*) FROM webhook_events`).Scan(&webhookCount)
	}
	out["webhook_events"] = webhookCount

	cfg := cardplatform.LoadConfig()
	if cfg.APIKey == "" {
		out["error"] = "未配置卡台 API Key"
		// 仍返回 200，前端展示提示
		c.JSON(http.StatusOK, out)
		return
	}
	out["configured"] = true

	cli := cardplatform.NewFromSettings()
	ctx := c.Request.Context()

	sum, err := cli.SummarizeCDKs(ctx, 50)
	if err != nil {
		out["error"] = err.Error()
		c.JSON(http.StatusOK, out)
		return
	}
	by := sum.ByStatus
	unused := by["unused"]
	out["total_cdks"] = sum.Total
	out["total_cdkeys"] = sum.Total
	out["unused_cdks"] = unused
	out["active_cdks"] = unused
	out["active_cdkeys"] = unused
	out["reserved_cdks"] = by["reserved"]
	out["consumed_cdks"] = by["consumed"]
	out["frozen_cdks"] = by["frozen"]
	out["disabled_cdks"] = by["disabled"]
	out["by_status"] = by
	out["counts_partial"] = sum.Partial
	out["scanned"] = sum.Scanned

	if ordersTotal, err := cli.ListCDKOrdersTotal(ctx); err == nil {
		out["total_cdk_orders"] = ordersTotal
	}

	// 兼容旧前端「任务数」字段：用卡台兑换订单数
	out["total_tasks"] = out["total_cdk_orders"]

	c.JSON(http.StatusOK, out)
}

// Legacy/Admin handlers
func GenerateCDKeys(c *gin.Context) {
	type GenerateCDKeysRequest struct {
		PlanType    string `json:"plan_type" binding:"required"`
		Quantity    int    `json:"quantity" binding:"required"`
		ExpiresAt   string `json:"expires_at"`
		Description string `json:"description"`
	}

	var req GenerateCDKeysRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	if req.Quantity <= 0 || req.Quantity > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "quantity must be between 1 and 100"})
		return
	}

	req.PlanType = normalizeCDKPlanType(req.PlanType)
	if req.PlanType == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "plan_type must be one of: pro, 5x, 20x"})
		return
	}

	var expiresAt interface{}
	if strings.TrimSpace(req.ExpiresAt) != "" {
		parsed, err := time.Parse("2006-01-02", strings.TrimSpace(req.ExpiresAt))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "expires_at must use YYYY-MM-DD"})
			return
		}
		expiresAt = parsed.Format("2006-01-02")
	}

	generatedCodes := make([]string, 0, req.Quantity)
	for len(generatedCodes) < req.Quantity {
		code := GenerateCDKCode(req.PlanType)
		_, err := db.DB.Exec(`
			INSERT INTO cd_keys (code, plan_type, status, expires_at, description)
			VALUES (?, ?, 'active', ?, ?)
		`, code, req.PlanType, expiresAt, strings.TrimSpace(req.Description))
		if err != nil {
			log.Printf("generate cdk insert retry: %v", err)
			continue
		}
		generatedCodes = append(generatedCodes, code)
	}

	auditAdmin(c, "cdk_generate", fmt.Sprintf("plan=%s count=%d", req.PlanType, len(generatedCodes)))
	c.JSON(http.StatusOK, gin.H{
		"generated": len(generatedCodes),
		"codes":     generatedCodes,
	})
}

func ListCDKeys(c *gin.Context) {
	statusFilter := strings.TrimSpace(c.Query("status"))
	planTypeFilter := normalizeCDKPlanType(c.Query("plan_type"))
	queryText := strings.TrimSpace(c.Query("q"))
	limit := parseLimit(c.Query("limit"), 200)

	query := `
		SELECT id, code, COALESCE(plan_type, ''), COALESCE(status, 'active'),
		       COALESCE(created_at, ''), used_at, expires_at, COALESCE(description, '')
		FROM cd_keys
		WHERE 1 = 1
	`
	args := []interface{}{}

	if statusFilter != "" {
		query += ` AND status = ?`
		args = append(args, statusFilter)
	}
	if planTypeFilter != "" {
		query += ` AND plan_type = ?`
		args = append(args, planTypeFilter)
	}
	if queryText != "" {
		like := "%" + queryText + "%"
		query += ` AND (code LIKE ? OR plan_type LIKE ? OR description LIKE ?)`
		args = append(args, like, like, like)
	}

	query += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := db.DB.Query(query, args...)
	if err != nil {
		log.Printf("list cdk error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list CDKs"})
		return
	}
	defer rows.Close()

	type CDKeyRecord struct {
		ID          int64   `json:"id"`
		Code        string  `json:"code"`
		PlanType    string  `json:"plan_type"`
		Status      string  `json:"status"`
		CreatedAt   string  `json:"created_at"`
		UsedAt      *string `json:"used_at,omitempty"`
		ExpiresAt   *string `json:"expires_at,omitempty"`
		Description string  `json:"description,omitempty"`
	}

	keys := make([]CDKeyRecord, 0)
	for rows.Next() {
		var item CDKeyRecord
		var usedAt sql.NullString
		var expiresAt sql.NullString
		if err := rows.Scan(
			&item.ID,
			&item.Code,
			&item.PlanType,
			&item.Status,
			&item.CreatedAt,
			&usedAt,
			&expiresAt,
			&item.Description,
		); err != nil {
			log.Printf("scan cdk error: %v", err)
			continue
		}
		if usedAt.Valid {
			value := usedAt.String
			item.UsedAt = &value
		}
		if expiresAt.Valid {
			value := expiresAt.String
			item.ExpiresAt = &value
		}
		keys = append(keys, item)
	}

	c.JSON(http.StatusOK, gin.H{"keys": keys})
}

func DisableCDKey(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid CDK id"})
		return
	}

	result, err := db.DB.Exec(`
		UPDATE cd_keys
		SET status = 'disabled'
		WHERE id = ? AND status = 'active'
	`, id)
	if err != nil {
		log.Printf("disable cdk error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to disable CDK"})
		return
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "CDK not found or already unavailable"})
		return
	}

	auditAdmin(c, "cdk_disable", fmt.Sprintf("id=%d", id))
	c.JSON(http.StatusOK, gin.H{"message": "CDK disabled"})
}

func DeleteCDKey(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid CDK id"})
		return
	}

	result, err := db.DB.Exec(`DELETE FROM cd_keys WHERE id = ?`, id)
	if err != nil {
		log.Printf("delete cdk error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete CDK"})
		return
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "CDK not found"})
		return
	}

	auditAdmin(c, "cdk_delete", fmt.Sprintf("id=%d", id))
	c.JSON(http.StatusOK, gin.H{"message": "CDK deleted"})
}

func ListRechargeRequests(c *gin.Context) {
	statusFilter := strings.TrimSpace(c.Query("status"))
	queryText := strings.TrimSpace(c.Query("q"))
	limit := parseLimit(c.Query("limit"), 200)

	query := `
		SELECT t.task_id, t.cdk_code, COALESCE(k.plan_type, ''), COALESCE(t.account_email, ''),
		       COALESCE(t.task_status, ''), COALESCE(t.created_at, ''), COALESCE(t.updated_at, ''),
		       t.completed_at, COALESCE(t.notes, ''), COALESCE(t.session_json, '')
		FROM recharge_tasks t
		LEFT JOIN cd_keys k ON k.code = t.cdk_code
		WHERE 1 = 1
	`
	args := []interface{}{}

	if statusFilter != "" {
		query += ` AND t.task_status = ?`
		args = append(args, statusFilter)
	}
	if queryText != "" {
		like := "%" + queryText + "%"
		query += ` AND (t.task_id LIKE ? OR t.cdk_code LIKE ? OR t.account_email LIKE ? OR t.notes LIKE ?)`
		args = append(args, like, like, like, like)
	}

	query += ` ORDER BY t.created_at DESC LIMIT ?`
	args = append(args, limit)

	rows, err := db.DB.Query(query, args...)
	if err != nil {
		log.Printf("list recharge requests error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list recharge requests"})
		return
	}
	defer rows.Close()

	type RechargeRequestRecord struct {
		TaskID       string  `json:"task_id"`
		CDKCode      string  `json:"cdk_code"`
		PlanType     string  `json:"plan_type"`
		AccountEmail string  `json:"account_email,omitempty"`
		TaskStatus   string  `json:"task_status"`
		CreatedAt    string  `json:"created_at"`
		UpdatedAt    string  `json:"updated_at"`
		CompletedAt  *string `json:"completed_at,omitempty"`
		Notes        string  `json:"notes,omitempty"`
		SessionJSON  string  `json:"session_json,omitempty"`
	}

	requests := make([]RechargeRequestRecord, 0)
	for rows.Next() {
		var item RechargeRequestRecord
		var completedAt sql.NullString
		if err := rows.Scan(
			&item.TaskID,
			&item.CDKCode,
			&item.PlanType,
			&item.AccountEmail,
			&item.TaskStatus,
			&item.CreatedAt,
			&item.UpdatedAt,
			&completedAt,
			&item.Notes,
			&item.SessionJSON,
		); err != nil {
			log.Printf("scan recharge request error: %v", err)
			continue
		}
		if completedAt.Valid {
			value := completedAt.String
			item.CompletedAt = &value
		}
		requests = append(requests, item)
	}

	c.JSON(http.StatusOK, gin.H{"requests": requests})
}

func ApproveRecharge(c *gin.Context) {
	type ApproveRechargeRequest struct {
		AccountEmail string `json:"account_email"`
		Notes        string `json:"notes"`
	}

	taskID := strings.TrimSpace(c.Param("taskID"))
	if taskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Task id is required"})
		return
	}

	var req ApproveRechargeRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	var existingStatus string
	var existingEmail sql.NullString
	err := db.DB.QueryRow(`
		SELECT task_status, account_email
		FROM recharge_tasks
		WHERE task_id = ?
	`, taskID).Scan(&existingStatus, &existingEmail)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Task not found"})
		return
	}
	if err != nil {
		log.Printf("approve recharge lookup error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load task"})
		return
	}

	if existingStatus == "completed" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Task already completed"})
		return
	}
	if existingStatus == "failed" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Task already failed"})
		return
	}

	accountEmail := strings.TrimSpace(req.AccountEmail)
	if accountEmail == "" && existingEmail.Valid {
		accountEmail = existingEmail.String
	}

	_, err = db.DB.Exec(`
		UPDATE recharge_tasks
		SET task_status = 'completed',
		    account_email = ?,
		    notes = ?,
		    completed_at = CURRENT_TIMESTAMP,
		    updated_at = CURRENT_TIMESTAMP
		WHERE task_id = ?
	`, accountEmail, strings.TrimSpace(req.Notes), taskID)
	if err != nil {
		log.Printf("approve recharge update error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to approve task"})
		return
	}

	auditAdmin(c, "task_approve", "task="+taskID)
	c.JSON(http.StatusOK, gin.H{"message": "Task marked as completed"})
}

func RejectRecharge(c *gin.Context) {
	type RejectRechargeRequest struct {
		Reason        string `json:"reason"`
		ReactivateCDK *bool  `json:"reactivate_cdk"`
	}

	taskID := strings.TrimSpace(c.Param("taskID"))
	if taskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Task id is required"})
		return
	}

	var req RejectRechargeRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	reactivateCDK := true
	if req.ReactivateCDK != nil {
		reactivateCDK = *req.ReactivateCDK
	}

	tx, err := db.DB.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start transaction"})
		return
	}
	defer tx.Rollback()

	var cdkCode string
	var status string
	err = tx.QueryRow(`
		SELECT cdk_code, task_status
		FROM recharge_tasks
		WHERE task_id = ?
	`, taskID).Scan(&cdkCode, &status)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Task not found"})
		return
	}
	if err != nil {
		log.Printf("reject recharge lookup error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load task"})
		return
	}

	if status == "completed" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Completed tasks cannot be rejected"})
		return
	}
	if status == "failed" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Task already failed"})
		return
	}

	_, err = tx.Exec(`
		UPDATE recharge_tasks
		SET task_status = 'failed',
		    notes = ?,
		    completed_at = CURRENT_TIMESTAMP,
		    updated_at = CURRENT_TIMESTAMP
		WHERE task_id = ?
	`, strings.TrimSpace(req.Reason), taskID)
	if err != nil {
		log.Printf("reject recharge update error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to reject task"})
		return
	}

	if reactivateCDK {
		if _, err := tx.Exec(`
			UPDATE cd_keys
			SET status = 'active',
			    used_at = NULL
			WHERE code = ? AND status = 'used'
		`, cdkCode); err != nil {
			log.Printf("reactivate cdk error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to reactivate CDK"})
			return
		}
	}

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save rejection"})
		return
	}

	auditAdmin(c, "task_reject", "task="+taskID)
	c.JSON(http.StatusOK, gin.H{"message": "Task marked as failed"})
}

func ListBillingRecords(c *gin.Context) {
	queryText := strings.TrimSpace(c.Query("q"))
	limit := parseLimit(c.Query("limit"), 200)

	query := `
		SELECT id, COALESCE(account_email, ''), COALESCE(subscription_status, ''),
		       COALESCE(plan_type, ''), COALESCE(billing_amount, 0), COALESCE(currency, 'USD'),
		       next_billing_date, COALESCE(created_at, ''), COALESCE(updated_at, '')
		FROM billing_records
		WHERE 1 = 1
	`
	args := []interface{}{}

	if queryText != "" {
		like := "%" + queryText + "%"
		query += ` AND (account_email LIKE ? OR plan_type LIKE ? OR subscription_status LIKE ?)`
		args = append(args, like, like, like)
	}

	query += ` ORDER BY updated_at DESC, id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := db.DB.Query(query, args...)
	if err != nil {
		log.Printf("list billing records error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list billing records"})
		return
	}
	defer rows.Close()

	type BillingRecord struct {
		ID                 int64   `json:"id"`
		AccountEmail       string  `json:"account_email"`
		SubscriptionStatus string  `json:"subscription_status"`
		PlanType           string  `json:"plan_type"`
		BillingAmount      float64 `json:"billing_amount"`
		Currency           string  `json:"currency"`
		NextBillingDate    *string `json:"next_billing_date,omitempty"`
		CreatedAt          string  `json:"created_at"`
		UpdatedAt          string  `json:"updated_at"`
	}

	records := make([]BillingRecord, 0)
	for rows.Next() {
		var item BillingRecord
		var nextBillingDate sql.NullString
		if err := rows.Scan(
			&item.ID,
			&item.AccountEmail,
			&item.SubscriptionStatus,
			&item.PlanType,
			&item.BillingAmount,
			&item.Currency,
			&nextBillingDate,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			log.Printf("scan billing record error: %v", err)
			continue
		}
		if nextBillingDate.Valid {
			value := nextBillingDate.String
			item.NextBillingDate = &value
		}
		records = append(records, item)
	}

	c.JSON(http.StatusOK, gin.H{"records": records})
}

// Legacy handler aliases
func GetRechargeHistory(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"history": []interface{}{}})
}

func GetRechargeDetail(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{})
}

func GetStats(c *gin.Context) {
	GetSystemStats(c)
}

func VerifyCDKLegacy(c *gin.Context) {
	VerifyCDK(c)
}

func SubmitRecharge(c *gin.Context) {
	c.JSON(http.StatusBadRequest, gin.H{"error": "Use new API endpoints"})
}
