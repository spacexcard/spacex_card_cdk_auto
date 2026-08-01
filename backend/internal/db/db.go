package db

import (
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/tuzi/cdk-recharge-system/internal/config"
	"golang.org/x/crypto/bcrypt"
)

var DB *sql.DB

const (
	DefaultAdminUsername = "admin"
	// 历史上内置的默认密码，仅用于在启动时识别“仍在使用弱口令”并告警，不再用于创建账号。
	legacyDefaultPassword = "admin123456"
)

func Init(cfg *config.DatabaseConfig) error {
	// 使用 SQLite。默认 ../data/cdk_recharge.db（相对于backend目录）；
	// 部署/容器内可用环境变量 DB_PATH 覆盖（如 /app/data/cdk_recharge.db）。
	dbPath := "../data/cdk_recharge.db"
	if p := strings.TrimSpace(os.Getenv("DB_PATH")); p != "" {
		dbPath = p
	}
	log.Printf("使用数据库: %s", dbPath)
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return err
	}

	if err := db.Ping(); err != nil {
		return err
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)

	DB = db
	log.Println("✓ SQLite 数据库连接成功: data/cdk_recharge.db")

	// 创建表
	if err := createTables(); err != nil {
		return err
	}

	return nil
}

func createTables() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS cd_keys (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			code TEXT UNIQUE NOT NULL,
			plan_type TEXT,
			status TEXT DEFAULT 'active',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			used_at DATETIME,
			expires_at DATETIME,
			description TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_cd_keys_code ON cd_keys(code)`,
		`CREATE INDEX IF NOT EXISTS idx_cd_keys_status ON cd_keys(status)`,

		// 卡台 GPT 直充 CDK：发码时落库完整码，列表仅回前缀时用本表补全（admin 本站复制）
		`CREATE TABLE IF NOT EXISTS cardplatform_cdk_codes (
			upstream_id INTEGER,
			code TEXT NOT NULL UNIQUE,
			code_prefix TEXT,
			plan TEXT,
			fee_amount_minor INTEGER DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_cp_cdk_upstream ON cardplatform_cdk_codes(upstream_id)`,
		`CREATE INDEX IF NOT EXISTS idx_cp_cdk_prefix ON cardplatform_cdk_codes(code_prefix)`,

		`CREATE TABLE IF NOT EXISTS recharge_tasks (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			task_id TEXT UNIQUE NOT NULL,
			cdk_code TEXT NOT NULL,
			session_json TEXT,
			account_email TEXT,
			task_status TEXT DEFAULT 'pending',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			completed_at DATETIME,
			notes TEXT,
			FOREIGN KEY(cdk_code) REFERENCES cd_keys(code)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_recharge_tasks_cdk ON recharge_tasks(cdk_code)`,
		`CREATE INDEX IF NOT EXISTS idx_recharge_tasks_task_id ON recharge_tasks(task_id)`,
		`CREATE INDEX IF NOT EXISTS idx_recharge_tasks_status ON recharge_tasks(task_status)`,

		`CREATE TABLE IF NOT EXISTS billing_records (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_hash TEXT UNIQUE NOT NULL,
			account_email TEXT,
			subscription_status TEXT,
			plan_type TEXT,
			billing_amount REAL,
			currency TEXT DEFAULT 'USD',
			next_billing_date DATETIME,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_billing_records_session ON billing_records(session_hash)`,
		`CREATE INDEX IF NOT EXISTS idx_billing_records_email ON billing_records(account_email)`,

		`CREATE TABLE IF NOT EXISTS admin_users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT UNIQUE NOT NULL,
			password_hash TEXT NOT NULL,
			display_name TEXT,
			is_active INTEGER DEFAULT 1,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_admin_users_username ON admin_users(username)`,

		`CREATE TABLE IF NOT EXISTS admin_audit_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT,
			action TEXT NOT NULL,
			detail TEXT,
			ip TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_created ON admin_audit_logs(created_at)`,

		// 站点配置 key-value（品牌/皮肤/安装锁/加密密钥元数据等）
		`CREATE TABLE IF NOT EXISTS site_settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,

		// 卡台 Webhook 事件（幂等入库；配合轮询双通道）
		`CREATE TABLE IF NOT EXISTS webhook_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			event_type TEXT,
			idem_key TEXT UNIQUE NOT NULL,
			payload TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_webhook_created ON webhook_events(created_at)`,

		// 兑换时绑定的 CDK → session，供账单页「凭卡密查询」
		`CREATE TABLE IF NOT EXISTS cdk_session_bindings (
			cdk_code TEXT PRIMARY KEY,
			session_payload TEXT NOT NULL,
			redemption_token TEXT,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_cdk_bind_token ON cdk_session_bindings(redemption_token)`,
	}

	for _, query := range queries {
		if _, err := DB.Exec(query); err != nil {
			log.Printf("警告: 创建表时出错 (可能已存在): %v", err)
		}
	}

	log.Println("✓ 数据表已准备就绪")
	if err := migrateCDKPlanTypes(); err != nil {
		return err
	}
	if err := migrateLegacyCDKCodes(); err != nil {
		return err
	}
	if err := ensureDefaultAdmin(); err != nil {
		return err
	}
	return nil
}

// legacySHA256 是旧的（不安全）哈希算法，仅用于兼容历史数据并在登录时升级为 bcrypt。
func legacySHA256(password string) string {
	hash := sha256.Sum256([]byte(password))
	return hex.EncodeToString(hash[:])
}

// HashAdminPassword 现在使用 bcrypt（带盐、慢哈希）。
// WriteAudit 记录一条管理员操作审计日志（best-effort，失败只记日志不影响主流程）。
func WriteAudit(username, action, detail, ip string) {
	if DB == nil {
		return
	}
	if _, err := DB.Exec(
		`INSERT INTO admin_audit_logs (username, action, detail, ip) VALUES (?, ?, ?, ?)`,
		username, action, detail, ip,
	); err != nil {
		log.Printf("写入审计日志失败: %v", err)
	}
}

func HashAdminPassword(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// VerifyAdminPassword 校验密码。返回 (是否正确, 需要写回的新哈希)。
// 若数据库里仍是旧的 SHA-256 哈希且校验通过，会返回一个 bcrypt 新哈希用于升级存储。
func VerifyAdminPassword(stored, plain string) (bool, string) {
	if strings.HasPrefix(stored, "$2") {
		return bcrypt.CompareHashAndPassword([]byte(stored), []byte(plain)) == nil, ""
	}
	// 旧的 SHA-256 十六进制哈希：常量时间比较，命中后升级为 bcrypt
	if subtle.ConstantTimeCompare([]byte(legacySHA256(plain)), []byte(stored)) == 1 {
		if upgraded, err := HashAdminPassword(plain); err == nil {
			return true, upgraded
		}
		return true, ""
	}
	return false, ""
}

// SetAdminPassword 更新指定管理员的密码（bcrypt）。
func SetAdminPassword(username, newPlain string) error {
	hash, err := HashAdminPassword(newPlain)
	if err != nil {
		return err
	}
	_, err = DB.Exec(`UPDATE admin_users SET password_hash = ?, updated_at = CURRENT_TIMESTAMP WHERE username = ?`, hash, username)
	return err
}

// UpgradeAdminHash 把登录时升级出来的 bcrypt 哈希写回数据库。
func UpgradeAdminHash(username, newHash string) {
	if newHash == "" {
		return
	}
	if _, err := DB.Exec(`UPDATE admin_users SET password_hash = ? WHERE username = ?`, newHash, username); err != nil {
		log.Printf("升级管理员密码哈希失败: %v", err)
	}
}

func randomPassword(n int) string {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnpqrstuvwxyz23456789"
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "Chg-Me-" + legacySHA256(fmt.Sprintf("%d", time.Now().UnixNano()))[:12]
	}
	out := make([]byte, n)
	for i, b := range buf {
		out[i] = alphabet[int(b)%len(alphabet)]
	}
	return string(out)
}

// InstallMode: wizard（推荐，等 Web 安装）| auto（启动时建管理员，兼容旧行为）
func InstallMode() string {
	m := strings.ToLower(strings.TrimSpace(os.Getenv("INSTALL_MODE")))
	if m == "auto" {
		return "auto"
	}
	// 默认 wizard：开源/生产更安全
	return "wizard"
}

// IsInstalled 任一管理员存在或 install_completed_at 已写，即视为已安装。
func IsInstalled() bool {
	if AdminCount() > 0 {
		return true
	}
	v, _ := GetSetting("install_completed_at")
	return strings.TrimSpace(v) != ""
}

func AdminCount() int {
	if DB == nil {
		return 0
	}
	var n int
	_ = DB.QueryRow(`SELECT COUNT(*) FROM admin_users`).Scan(&n)
	return n
}

func GetSetting(key string) (string, error) {
	var v string
	err := DB.QueryRow(`SELECT value FROM site_settings WHERE key = ?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return v, err
}

func SetSetting(key, value string) error {
	_, err := DB.Exec(`
		INSERT INTO site_settings (key, value, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP
	`, key, value)
	return err
}

func DeleteSetting(key string) error {
	_, err := DB.Exec(`DELETE FROM site_settings WHERE key = ?`, key)
	return err
}

// EnsureSetupToken 未安装时准备一次性安装令牌（hash 入库，明文仅首次创建时返回供打日志）。
// 若 env SETUP_BOOTSTRAP_TOKEN 已设，则用其；否则随机生成。已有 hash 时返回空 plain。
func EnsureSetupToken() (plain string, firstTime bool, err error) {
	if IsInstalled() {
		return "", false, nil
	}
	// 已有 hash：不再二次生成（避免每次重启换 token）
	if h, _ := GetSetting("setup_token_hash"); strings.TrimSpace(h) != "" {
		return "", false, nil
	}
	plain = strings.TrimSpace(os.Getenv("SETUP_BOOTSTRAP_TOKEN"))
	if plain == "" {
		plain = randomPassword(24)
	}
	sum := sha256.Sum256([]byte(plain))
	if err := SetSetting("setup_token_hash", hex.EncodeToString(sum[:])); err != nil {
		return "", false, err
	}
	return plain, true, nil
}

// VerifySetupToken 常量时间比较安装令牌。
func VerifySetupToken(plain string) bool {
	stored, err := GetSetting("setup_token_hash")
	if err != nil || stored == "" || plain == "" {
		return false
	}
	sum := sha256.Sum256([]byte(plain))
	got := hex.EncodeToString(sum[:])
	return subtle.ConstantTimeCompare([]byte(got), []byte(stored)) == 1
}

// MarkInstalled 写安装完成标记并作废 setup token。
func MarkInstalled(username string) error {
	if err := SetSetting("install_completed_at", time.Now().UTC().Format(time.RFC3339)); err != nil {
		return err
	}
	_ = DeleteSetting("setup_token_hash")
	WriteAudit(username, "install_bootstrap", "setup completed", "")
	return nil
}

// IsWeakPassword 拦截常见弱口令与历史默认口令。
func IsWeakPassword(p string) bool {
	p = strings.TrimSpace(p)
	if len(p) < 12 {
		return true
	}
	lower := strings.ToLower(p)
	weak := []string{
		legacyDefaultPassword, "password", "password123", "admin", "admin123",
		"adminadmin", "12345678", "123456789012", "qwerty123456", "changeme",
		"letmein12345", "welcome12345",
	}
	for _, w := range weak {
		if lower == w {
			return true
		}
	}
	// 全数字或与用户名相同在调用方再判
	return false
}

func ensureDefaultAdmin() error {
	// 先确保有安装令牌（wizard 模式、未安装时）
	if !IsInstalled() && InstallMode() == "wizard" {
		if plain, _, err := EnsureSetupToken(); err != nil {
			return err
		} else if plain != "" {
			log.Printf("============================================================")
			log.Printf("✓ 首次安装模式 (INSTALL_MODE=wizard)")
			log.Printf("  打开 /ops/setup 完成管理员创建（仅一次）")
			log.Printf("  Setup Token（仅此一次显示，bootstrap 请求头 X-Setup-Token）: %s", plain)
			log.Printf("============================================================")
		} else {
			log.Printf("ℹ 等待 Web 安装向导：/ops/setup（需正确 X-Setup-Token）")
		}
	}

	username := os.Getenv("ADMIN_USERNAME")
	if strings.TrimSpace(username) == "" {
		username = DefaultAdminUsername
	}

	var count int
	var existingHash string
	err := DB.QueryRow(`SELECT COUNT(*), COALESCE(MAX(password_hash), '') FROM admin_users WHERE username = ?`, username).Scan(&count, &existingHash)
	if err != nil {
		return err
	}

	if count > 0 {
		// 已存在：若仍是历史弱口令，则在启动时大声告警，提示尽快改密。
		if ok, _ := VerifyAdminPassword(existingHash, legacyDefaultPassword); ok {
			log.Printf("⚠️  管理员 %s 仍在使用历史默认密码（admin123456），存在严重风险，请立刻登录后台修改密码！", username)
		}
		// 补写 install 标记（从旧库升级）
		if v, _ := GetSetting("install_completed_at"); v == "" {
			_ = SetSetting("install_completed_at", time.Now().UTC().Format(time.RFC3339))
			_ = DeleteSetting("setup_token_hash")
		}
		return nil
	}

	// wizard：无 ADMIN_PASSWORD 时不自动建号，等 Web bootstrap
	password := strings.TrimSpace(os.Getenv("ADMIN_PASSWORD"))
	if InstallMode() == "wizard" && password == "" {
		return nil
	}

	// auto 或显式 ADMIN_PASSWORD：启动时建管理员
	generated := false
	if password == "" {
		password = randomPassword(16)
		generated = true
	}

	hash, err := HashAdminPassword(password)
	if err != nil {
		return err
	}
	if _, err := DB.Exec(`
		INSERT INTO admin_users (username, password_hash, display_name, is_active, updated_at)
		VALUES (?, ?, ?, 1, CURRENT_TIMESTAMP)
	`, username, hash, "System Admin"); err != nil {
		return err
	}
	_ = MarkInstalled(username)

	if generated {
		log.Printf("============================================================")
		log.Printf("✓ 已创建管理员账号: %s", username)
		log.Printf("  初始随机密码（仅此一次显示，请立即登录修改）: %s", password)
		log.Printf("  也可通过环境变量 ADMIN_PASSWORD 指定初始密码。")
		log.Printf("============================================================")
	} else {
		log.Printf("✓ 已用 ADMIN_PASSWORD 创建管理员账号: %s", username)
	}
	return nil
}

// CreateAdminUser 事务安全创建管理员（安装向导用）。若已有任意管理员则失败。
func CreateAdminUser(username, password, displayName string) error {
	if DB == nil {
		return fmt.Errorf("db not ready")
	}
	username = strings.TrimSpace(username)
	if username == "" {
		return fmt.Errorf("username required")
	}
	if IsWeakPassword(password) {
		return fmt.Errorf("password too weak")
	}
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var n int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM admin_users`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return fmt.Errorf("already_installed")
	}
	hash, err := HashAdminPassword(password)
	if err != nil {
		return err
	}
	if displayName == "" {
		displayName = "System Admin"
	}
	if _, err := tx.Exec(`
		INSERT INTO admin_users (username, password_hash, display_name, is_active, updated_at)
		VALUES (?, ?, ?, 1, CURRENT_TIMESTAMP)
	`, username, hash, displayName); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		INSERT INTO site_settings (key, value, updated_at) VALUES ('install_completed_at', ?, CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP
	`, time.Now().UTC().Format(time.RFC3339)); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM site_settings WHERE key = ?`, "setup_token_hash"); err != nil {
		return err
	}
	if _, err := tx.Exec(
		`INSERT INTO admin_audit_logs (username, action, detail, ip) VALUES (?, ?, ?, ?)`,
		username, "install_bootstrap", "setup completed", "",
	); err != nil {
		log.Printf("audit install: %v", err)
	}
	return tx.Commit()
}

// RandomPassword 导出给安装向导生成强口令。
func RandomPassword(n int) string {
	if n < 12 {
		n = 16
	}
	return randomPassword(n)
}

// WebhookEvent 入库行
type WebhookEvent struct {
	ID        int64
	EventType string
	IdemKey   string
	Payload   string
	CreatedAt string
}

func InsertWebhookEvent(eventType, idemKey, payload string) error {
	if DB == nil {
		return fmt.Errorf("db not ready")
	}
	_, err := DB.Exec(
		`INSERT INTO webhook_events (event_type, idem_key, payload) VALUES (?, ?, ?)`,
		eventType, idemKey, payload,
	)
	return err
}

func ListWebhookEvents(limit int) ([]WebhookEvent, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := DB.Query(`
		SELECT id, COALESCE(event_type,''), idem_key, payload, COALESCE(created_at,'')
		FROM webhook_events ORDER BY id DESC LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []WebhookEvent
	for rows.Next() {
		var e WebhookEvent
		if err := rows.Scan(&e.ID, &e.EventType, &e.IdemKey, &e.Payload, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func normalizeCDKCode(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

// BindCDKRedemptionToken preview 成功后：码 ↔ redemption_token
func BindCDKRedemptionToken(cdkCode, redemptionToken string) error {
	if DB == nil {
		return fmt.Errorf("db not ready")
	}
	code := normalizeCDKCode(cdkCode)
	tok := strings.TrimSpace(redemptionToken)
	if code == "" || tok == "" {
		return nil
	}
	_, err := DB.Exec(`
		INSERT INTO cdk_session_bindings (cdk_code, session_payload, redemption_token, updated_at)
		VALUES (?, '', ?, CURRENT_TIMESTAMP)
		ON CONFLICT(cdk_code) DO UPDATE SET
			redemption_token = excluded.redemption_token,
			updated_at = CURRENT_TIMESTAMP
	`, code, tok)
	return err
}

// BindCDKSession 预检/兑换时写入 session（可按码或 redemption_token 关联）
func BindCDKSession(cdkCode, redemptionToken, sessionPayload string) error {
	if DB == nil {
		return fmt.Errorf("db not ready")
	}
	code := normalizeCDKCode(cdkCode)
	tok := strings.TrimSpace(redemptionToken)
	sess := strings.TrimSpace(sessionPayload)
	if sess == "" {
		return nil
	}
	// 优先已知码
	if code != "" {
		_, err := DB.Exec(`
			INSERT INTO cdk_session_bindings (cdk_code, session_payload, redemption_token, updated_at)
			VALUES (?, ?, ?, CURRENT_TIMESTAMP)
			ON CONFLICT(cdk_code) DO UPDATE SET
				session_payload = excluded.session_payload,
				redemption_token = COALESCE(NULLIF(excluded.redemption_token,''), cdk_session_bindings.redemption_token),
				updated_at = CURRENT_TIMESTAMP
		`, code, sess, tok)
		return err
	}
	// 仅有 token：更新已绑定行
	if tok != "" {
		res, err := DB.Exec(`
			UPDATE cdk_session_bindings
			SET session_payload = ?, updated_at = CURRENT_TIMESTAMP
			WHERE redemption_token = ?
		`, sess, tok)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n > 0 {
			return nil
		}
		// 尚无行：用 token 伪作 key 前缀保存（账单页仍建议用完整码）
		_, err = DB.Exec(`
			INSERT INTO cdk_session_bindings (cdk_code, session_payload, redemption_token, updated_at)
			VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		`, "RT:"+tok, sess, tok)
		return err
	}
	return nil
}

// GetSessionByCDK 账单查询：凭卡密取绑定 session
func GetSessionByCDK(cdkCode string) (string, error) {
	if DB == nil {
		return "", fmt.Errorf("db not ready")
	}
	code := normalizeCDKCode(cdkCode)
	if code == "" {
		return "", nil
	}
	var sess string
	err := DB.QueryRow(`
		SELECT session_payload FROM cdk_session_bindings
		WHERE cdk_code = ? AND TRIM(session_payload) != ''
	`, code).Scan(&sess)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return sess, err
}

// CDKBinding 本地码 ↔ token ↔ session 绑定（公开兑换进度/账单依赖）
type CDKBinding struct {
	CDKCode         string
	RedemptionToken string
	SessionPayload  string
	UpdatedAt       string
}

// GetBindingByCDK 按卡密取绑定（session 可为空：仅 preview 过）
func GetBindingByCDK(cdkCode string) (*CDKBinding, error) {
	if DB == nil {
		return nil, fmt.Errorf("db not ready")
	}
	code := normalizeCDKCode(cdkCode)
	if code == "" {
		return nil, nil
	}
	var b CDKBinding
	err := DB.QueryRow(`
		SELECT cdk_code, COALESCE(redemption_token,''), COALESCE(session_payload,''), COALESCE(updated_at,'')
		FROM cdk_session_bindings
		WHERE cdk_code = ?
	`, code).Scan(&b.CDKCode, &b.RedemptionToken, &b.SessionPayload, &b.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &b, nil
}

// FindCodeByRedemptionToken 预检绑 session 时反查完整卡密
func FindCodeByRedemptionToken(redemptionToken string) (string, error) {
	if DB == nil {
		return "", fmt.Errorf("db not ready")
	}
	tok := strings.TrimSpace(redemptionToken)
	if tok == "" {
		return "", nil
	}
	var code string
	err := DB.QueryRow(`
		SELECT cdk_code FROM cdk_session_bindings
		WHERE redemption_token = ?
		LIMIT 1
	`, tok).Scan(&code)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return code, err
}

func migrateCDKPlanTypes() error {
	updates := []struct {
		from string
		to   string
	}{
		{"GPT-Pro", "pro"},
		{"GPT-Plus", "5x"},
		{"Pro", "pro"},
		{"PLUS", "5x"},
	}

	for _, item := range updates {
		if _, err := DB.Exec(`UPDATE cd_keys SET plan_type = ? WHERE plan_type = ?`, item.to, item.from); err != nil {
			return err
		}
	}

	return nil
}

func migrateLegacyCDKCodes() error {
	rows, err := DB.Query(`SELECT id, code, plan_type FROM cd_keys`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type item struct {
		id       int64
		code     string
		planType string
	}

	var items []item
	for rows.Next() {
		var current item
		if err := rows.Scan(&current.id, &current.code, &current.planType); err != nil {
			return err
		}
		if strings.Count(current.code, "-") == 4 && len(current.code) >= 32 {
			continue
		}
		items = append(items, current)
	}

	for _, current := range items {
		newCode := legacyUUIDCode(current.planType, current.id)
		if _, err := DB.Exec(`UPDATE cd_keys SET code = ? WHERE id = ?`, newCode, current.id); err != nil {
			return err
		}
		if _, err := DB.Exec(`UPDATE recharge_tasks SET cdk_code = ? WHERE cdk_code = ?`, newCode, current.code); err != nil {
			return err
		}
	}

	return nil
}

func legacyUUIDCode(planType string, id int64) string {
	seed := fmt.Sprintf("%s-%d-%d", planType, id, time.Now().UnixNano())
	sum := sha1.Sum([]byte(seed))
	hexID := hex.EncodeToString(sum[:16])
	return fmt.Sprintf("%s-%s-%s-%s-%s", hexID[0:8], hexID[8:12], hexID[12:16], hexID[16:20], hexID[20:32])
}

// SaveCardplatformCDKCode 发码成功后缓存完整码（列表接口卡台只回 prefix）。
func SaveCardplatformCDKCode(upstreamID int64, code, prefix, plan string, feeMinor int64) error {
	if DB == nil {
		return fmt.Errorf("db not init")
	}
	code = strings.TrimSpace(code)
	if code == "" {
		return nil
	}
	prefix = strings.TrimSpace(prefix)
	if prefix == "" && len(code) >= 14 {
		prefix = code[:14]
	}
	_, err := DB.Exec(`
		INSERT INTO cardplatform_cdk_codes (upstream_id, code, code_prefix, plan, fee_amount_minor, created_at)
		VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(code) DO UPDATE SET
			upstream_id = excluded.upstream_id,
			code_prefix = excluded.code_prefix,
			plan = excluded.plan,
			fee_amount_minor = excluded.fee_amount_minor
	`, upstreamID, code, prefix, strings.TrimSpace(plan), feeMinor)
	return err
}

// LookupCardplatformCDKCode 按上游 id 或 prefix 取完整码。
func LookupCardplatformCDKCode(upstreamID int64, prefix string) (string, bool) {
	if DB == nil {
		return "", false
	}
	prefix = strings.TrimSpace(prefix)
	var code string
	if upstreamID > 0 {
		err := DB.QueryRow(`SELECT code FROM cardplatform_cdk_codes WHERE upstream_id = ? ORDER BY created_at DESC LIMIT 1`, upstreamID).Scan(&code)
		if err == nil && strings.TrimSpace(code) != "" {
			return strings.TrimSpace(code), true
		}
	}
	if prefix != "" {
		err := DB.QueryRow(`SELECT code FROM cardplatform_cdk_codes WHERE code_prefix = ? OR code LIKE ? ORDER BY created_at DESC LIMIT 1`,
			prefix, prefix+"%").Scan(&code)
		if err == nil && strings.TrimSpace(code) != "" {
			return strings.TrimSpace(code), true
		}
	}
	return "", false
}

func Close() error {
	if DB != nil {
		return DB.Close()
	}
	return nil
}
