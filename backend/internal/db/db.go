package db

import (
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
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
			status TEXT DEFAULT '',
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

		// 自动选卡权重规则（管理员可配置优先级）
		`CREATE TABLE IF NOT EXISTS card_selection_rules (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			sort_order INTEGER NOT NULL DEFAULT 0,
			plan_key TEXT NOT NULL,
			display_name TEXT NOT NULL,
			bin_prefix TEXT DEFAULT '',
			channel TEXT DEFAULT '',
			enabled INTEGER DEFAULT 1,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,

		// 卡台产品状态缓存（每3分钟后台同步，plan_key 用于逻辑套餐）
		`CREATE TABLE IF NOT EXISTS plan_status_cache (
			plan_key TEXT PRIMARY KEY,
			label TEXT DEFAULT '',
			online INTEGER DEFAULT 1,
			service_fee_usd_minor INTEGER DEFAULT 0,
			synced_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,

		// 卡台实体产品缓存（/openapi/v1/products 同步，product_code 唯一）
		`CREATE TABLE IF NOT EXISTS card_product_cache (
			product_code TEXT PRIMARY KEY,
			issuer TEXT DEFAULT '',
			bin TEXT DEFAULT '',
			network TEXT DEFAULT '',
			issuing_area TEXT DEFAULT '',
			scene TEXT DEFAULT '',
			card_group TEXT DEFAULT '',
			description TEXT DEFAULT '',
			bin_heads TEXT DEFAULT '',
			enabled INTEGER DEFAULT 1,
			suspended_at TEXT DEFAULT '',
			synced_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
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
	if err := migrateDefaultCardSelectionRules(); err != nil {
		return err
	}
	if err := migrateCardplatformCDKStatusCol(); err != nil {
		log.Printf("migrateCardplatformCDKStatusCol: %v", err)
	}
	if err := ensureDefaultAdmin(); err != nil {
		return err
	}
	return nil
}

// migrateCardplatformCDKStatusCol 为已存完整码表补 status（禁用后列表要展示）。
func migrateCardplatformCDKStatusCol() error {
	if DB == nil {
		return nil
	}
	var n int
	err := DB.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('cardplatform_cdk_codes') WHERE name='status'`).Scan(&n)
	if err != nil || n > 0 {
		return err
	}
	_, err = DB.Exec(`ALTER TABLE cardplatform_cdk_codes ADD COLUMN status TEXT DEFAULT ''`)
	return err
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

// SaveCardplatformCDKCode 把完整码写入本站 SQLite（发码 / 回填）。
// 卡台列表只回 code_prefix，完整码仅本站 DB 可补全。
func SaveCardplatformCDKCode(upstreamID int64, code, prefix, plan string, feeMinor int64) error {
	return SaveCardplatformCDKCodeWithStatus(upstreamID, code, prefix, plan, feeMinor, "unused")
}

// SaveCardplatformCDKCodeWithStatus 同上，并写入/更新 status。
func SaveCardplatformCDKCodeWithStatus(upstreamID int64, code, prefix, plan string, feeMinor int64, status string) error {
	if DB == nil {
		return fmt.Errorf("db not init")
	}
	code = strings.TrimSpace(code)
	if code == "" {
		return fmt.Errorf("empty code")
	}
	prefix = strings.TrimSpace(prefix)
	if prefix == "" && len(code) >= 14 {
		prefix = code[:14]
	}
	plan = strings.TrimSpace(plan)
	status = strings.TrimSpace(strings.ToLower(status))
	if status == "" {
		status = "unused"
	}
	_, err := DB.Exec(`
		INSERT INTO cardplatform_cdk_codes (upstream_id, code, code_prefix, plan, fee_amount_minor, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(code) DO UPDATE SET
			upstream_id = excluded.upstream_id,
			code_prefix = excluded.code_prefix,
			plan = CASE WHEN excluded.plan != '' THEN excluded.plan ELSE cardplatform_cdk_codes.plan END,
			fee_amount_minor = CASE WHEN excluded.fee_amount_minor > 0 THEN excluded.fee_amount_minor ELSE cardplatform_cdk_codes.fee_amount_minor END,
			status = CASE WHEN excluded.status != '' THEN excluded.status ELSE cardplatform_cdk_codes.status END
	`, upstreamID, code, prefix, plan, feeMinor, status)
	if err != nil {
		return fmt.Errorf("save cardplatform cdk: %w", err)
	}
	return nil
}

// UpdateCardplatformCDKStatus 按上游 id 更新本站缓存的状态（禁用/解禁后刷新列表）。
func UpdateCardplatformCDKStatus(upstreamID int64, status string) error {
	if DB == nil || upstreamID <= 0 {
		return nil
	}
	status = strings.TrimSpace(strings.ToLower(status))
	if status == "" {
		return nil
	}
	_, err := DB.Exec(`UPDATE cardplatform_cdk_codes SET status = ? WHERE upstream_id = ?`, status, upstreamID)
	return err
}

// CountCardplatformCDKCodes 本站已存完整码数量（运维/健康检查）。
func CountCardplatformCDKCodes() int {
	if DB == nil {
		return 0
	}
	var n int
	_ = DB.QueryRow(`SELECT COUNT(*) FROM cardplatform_cdk_codes`).Scan(&n)
	return n
}

// StoredCDKCode 本站已存的完整码行。
type StoredCDKCode struct {
	UpstreamID     int64  `json:"id"`
	Code           string `json:"code"`
	CodePrefix     string `json:"code_prefix"`
	Plan           string `json:"plan"`
	FeeAmountMinor int64  `json:"fee_amount_minor"`
	Status         string `json:"status"`
	CreatedAt      string `json:"created_at"`
}

// ListCardplatformStoredCDKCodes 列出本站 SQLite 中的完整码（可按 plan / q / status 过滤）。
// limit<=0 时默认 5000，硬顶 10000。
func ListCardplatformStoredCDKCodes(plan, q string, limit int) ([]StoredCDKCode, error) {
	return ListCardplatformStoredCDKCodesFilter(plan, q, "", limit)
}

// ListCardplatformStoredCDKCodesFilter 可按 status 过滤。
func ListCardplatformStoredCDKCodesFilter(plan, q, status string, limit int) ([]StoredCDKCode, error) {
	if DB == nil {
		return nil, fmt.Errorf("db not init")
	}
	if limit <= 0 {
		limit = 5000
	}
	if limit > 10000 {
		limit = 10000
	}
	plan = strings.TrimSpace(plan)
	q = strings.TrimSpace(q)
	status = strings.TrimSpace(strings.ToLower(status))
	sql := `
		SELECT COALESCE(upstream_id,0), code, COALESCE(code_prefix,''), COALESCE(plan,''),
		       COALESCE(fee_amount_minor,0), COALESCE(status,''), COALESCE(created_at,'')
		FROM cardplatform_cdk_codes WHERE 1=1`
	args := []any{}
	if plan != "" {
		sql += ` AND plan = ?`
		args = append(args, plan)
	}
	if status != "" {
		sql += ` AND status = ?`
		args = append(args, status)
	}
	if q != "" {
		sql += ` AND (code LIKE ? OR code_prefix LIKE ? OR CAST(upstream_id AS TEXT) = ?)`
		like := "%" + q + "%"
		args = append(args, like, like, q)
	}
	sql += ` ORDER BY created_at DESC, rowid DESC LIMIT ?`
	args = append(args, limit)
	rows, err := DB.Query(sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]StoredCDKCode, 0, 64)
	for rows.Next() {
		var it StoredCDKCode
		if err := rows.Scan(&it.UpstreamID, &it.Code, &it.CodePrefix, &it.Plan, &it.FeeAmountMinor, &it.Status, &it.CreatedAt); err != nil {
			return nil, err
		}
		it.Code = strings.TrimSpace(it.Code)
		if it.Code == "" {
			continue
		}
		if it.Status == "" {
			it.Status = "unused"
		}
		out = append(out, it)
	}
	return out, rows.Err()
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

// ---- 自动选卡权重配置 ----

// CardSelectionRule 一条选卡规则（按 sort_order 排列优先级）。
type CardSelectionRule struct {
	ID          int64  `json:"id"`
	SortOrder   int    `json:"sort_order"`
	PlanKey     string `json:"plan_key"`
	DisplayName string `json:"display_name"`
	BinPrefix   string `json:"bin_prefix"`
	Channel     string `json:"channel"`
	Enabled     bool   `json:"enabled"`
	CreatedAt   string `json:"created_at"`
}

// PlanStatusCache 卡台产品状态缓存条目。
type PlanStatusCache struct {
	PlanKey            string  `json:"plan_key"`
	Label              string  `json:"label"`
	Online             bool    `json:"online"`
	ServiceFeeUsdMinor int64   `json:"service_fee_usd_minor"`
	ServiceFeeUSD      float64 `json:"service_fee_usd"`
	SyncedAt           string  `json:"synced_at"`
}

// GetCardSelectionRules 按 sort_order 返回所有选卡规则。
func GetCardSelectionRules() ([]CardSelectionRule, error) {
	if DB == nil {
		return nil, fmt.Errorf("db not ready")
	}
	rows, err := DB.Query(`
		SELECT id, sort_order, plan_key, display_name,
		       COALESCE(bin_prefix,''), COALESCE(channel,''),
		       enabled, COALESCE(created_at,'')
		FROM card_selection_rules ORDER BY sort_order ASC, id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CardSelectionRule
	for rows.Next() {
		var r CardSelectionRule
		var enabled int
		if err := rows.Scan(&r.ID, &r.SortOrder, &r.PlanKey, &r.DisplayName,
			&r.BinPrefix, &r.Channel, &enabled, &r.CreatedAt); err != nil {
			return nil, err
		}
		r.Enabled = enabled == 1
		out = append(out, r)
	}
	return out, rows.Err()
}

// SetCardSelectionRules 在事务内整体替换选卡规则列表。
func SetCardSelectionRules(rules []CardSelectionRule) error {
	if DB == nil {
		return fmt.Errorf("db not ready")
	}
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM card_selection_rules`); err != nil {
		return err
	}
	for i, r := range rules {
		sortOrder := r.SortOrder
		if sortOrder == 0 {
			sortOrder = i + 1
		}
		enabled := 0
		if r.Enabled {
			enabled = 1
		}
		if _, err := tx.Exec(`
			INSERT INTO card_selection_rules (sort_order, plan_key, display_name, bin_prefix, channel, enabled)
			VALUES (?, ?, ?, ?, ?, ?)
		`, sortOrder, strings.TrimSpace(r.PlanKey), strings.TrimSpace(r.DisplayName),
			strings.TrimSpace(r.BinPrefix), strings.TrimSpace(r.Channel), enabled); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetPlanStatusCache 返回所有产品状态缓存（slice，供列表展示）。
func GetPlanStatusCache() ([]PlanStatusCache, error) {
	if DB == nil {
		return nil, fmt.Errorf("db not ready")
	}
	rows, err := DB.Query(`
		SELECT plan_key, COALESCE(label,''), online,
		       COALESCE(service_fee_usd_minor,0), COALESCE(synced_at,'')
		FROM plan_status_cache ORDER BY plan_key
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PlanStatusCache
	for rows.Next() {
		var p PlanStatusCache
		var online int
		if err := rows.Scan(&p.PlanKey, &p.Label, &online,
			&p.ServiceFeeUsdMinor, &p.SyncedAt); err != nil {
			return nil, err
		}
		p.Online = online == 1
		p.ServiceFeeUSD = float64(p.ServiceFeeUsdMinor) / 100.0
		out = append(out, p)
	}
	return out, rows.Err()
}

// GetPlanStatusCacheMap 返回 plan_key → PlanStatusCache 的 map，方便 O(1) 查找。
func GetPlanStatusCacheMap() (map[string]PlanStatusCache, error) {
	list, err := GetPlanStatusCache()
	if err != nil {
		return nil, err
	}
	m := make(map[string]PlanStatusCache, len(list))
	for _, p := range list {
		m[p.PlanKey] = p
	}
	return m, nil
}

// UpsertPlanStatus 插入或更新一条产品状态缓存。
func UpsertPlanStatus(planKey, label string, online bool, feeMinor int64) error {
	if DB == nil {
		return fmt.Errorf("db not ready")
	}
	onlineInt := 0
	if online {
		onlineInt = 1
	}
	_, err := DB.Exec(`
		INSERT INTO plan_status_cache (plan_key, label, online, service_fee_usd_minor, synced_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(plan_key) DO UPDATE SET
			label = excluded.label,
			online = excluded.online,
			service_fee_usd_minor = excluded.service_fee_usd_minor,
			synced_at = excluded.synced_at
	`, strings.TrimSpace(planKey), label, onlineInt, feeMinor)
	return err
}

// CardProductCache 卡台实体产品缓存条目。
type CardProductCache struct {
	ProductCode string   `json:"product_code"`
	Issuer      string   `json:"issuer"`
	BIN         string   `json:"bin"`
	Network     string   `json:"network"`
	IssuingArea string   `json:"issuing_area"`
	Scene       string   `json:"scene"`
	CardGroup   string   `json:"card_group"`
	Description string   `json:"description"`
	BinHeads    []string `json:"bin_heads"` // 反序列化自 JSON 存储
	Enabled     bool     `json:"enabled"`
	SuspendedAt string   `json:"suspended_at"`
	SyncedAt    string   `json:"synced_at"`
}

// UpsertCardProduct 插入或更新一条产品缓存。
func UpsertCardProduct(p CardProductCache) error {
	if DB == nil {
		return fmt.Errorf("db not ready")
	}
	onlineInt := 0
	if p.Enabled {
		onlineInt = 1
	}
	binHeadsJSON, _ := json.Marshal(p.BinHeads)
	_, err := DB.Exec(`
		INSERT INTO card_product_cache
			(product_code, issuer, bin, network, issuing_area, scene, card_group,
			 description, bin_heads, enabled, suspended_at, synced_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(product_code) DO UPDATE SET
			issuer = excluded.issuer,
			bin = excluded.bin,
			network = excluded.network,
			issuing_area = excluded.issuing_area,
			scene = excluded.scene,
			card_group = excluded.card_group,
			description = excluded.description,
			bin_heads = excluded.bin_heads,
			enabled = excluded.enabled,
			suspended_at = excluded.suspended_at,
			synced_at = excluded.synced_at
	`, p.ProductCode, p.Issuer, p.BIN, p.Network, p.IssuingArea, p.Scene, p.CardGroup,
		p.Description, string(binHeadsJSON), onlineInt, p.SuspendedAt)
	return err
}

// MarkCardProductsOfflineExcept 将不在 present 集合中的缓存产品标为已下线。
// 卡台 OpenAPI /products 只返回 enabled=true 的可开产品；下架后不再出现在列表，
// 若不在此收口，历史 VISA 等会永久显示「在线」。
func MarkCardProductsOfflineExcept(present map[string]bool) (int, error) {
	if DB == nil {
		return 0, fmt.Errorf("db not ready")
	}
	if present == nil {
		present = map[string]bool{}
	}
	rows, err := DB.Query(`SELECT product_code FROM card_product_cache WHERE enabled = 1`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var stale []string
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			return 0, err
		}
		code = strings.TrimSpace(code)
		if code == "" || present[code] {
			continue
		}
		stale = append(stale, code)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	n := 0
	for _, code := range stale {
		res, err := DB.Exec(`
			UPDATE card_product_cache
			SET enabled = 0, synced_at = CURRENT_TIMESTAMP
			WHERE product_code = ? AND enabled = 1
		`, code)
		if err != nil {
			return n, err
		}
		if aff, _ := res.RowsAffected(); aff > 0 {
			n++
		}
	}
	return n, nil
}

// GetCardProducts 返回所有产品缓存（在线优先，再按 product_code）。
func GetCardProducts() ([]CardProductCache, error) {
	if DB == nil {
		return nil, fmt.Errorf("db not ready")
	}
	rows, err := DB.Query(`
		SELECT product_code, COALESCE(issuer,''), COALESCE(bin,''), COALESCE(network,''),
		       COALESCE(issuing_area,''), COALESCE(scene,''), COALESCE(card_group,''),
		       COALESCE(description,''), COALESCE(bin_heads,'[]'), enabled,
		       COALESCE(suspended_at,''), COALESCE(synced_at,'')
		FROM card_product_cache
		ORDER BY enabled DESC, network, product_code
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CardProductCache
	for rows.Next() {
		var p CardProductCache
		var enabled int
		var binHeadsJSON string
		if err := rows.Scan(&p.ProductCode, &p.Issuer, &p.BIN, &p.Network,
			&p.IssuingArea, &p.Scene, &p.CardGroup, &p.Description,
			&binHeadsJSON, &enabled, &p.SuspendedAt, &p.SyncedAt); err != nil {
			return nil, err
		}
		p.Enabled = enabled == 1
		_ = json.Unmarshal([]byte(binHeadsJSON), &p.BinHeads)
		if p.BinHeads == nil {
			p.BinHeads = []string{}
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// migrateDefaultCardSelectionRules 首次启动时写入默认选卡优先级。
func migrateDefaultCardSelectionRules() error {
	if DB == nil {
		return nil
	}
	var count int
	if err := DB.QueryRow(`SELECT COUNT(*) FROM card_selection_rules`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil // 已有用户配置，不覆盖
	}
	defaults := []struct {
		sortOrder   int
		planKey     string
		displayName string
		binPrefix   string
		channel     string
	}{
		// 只写入美卡；香港卡（PP5450RC/PP5583RC）不进默认优先级
		{1, "P5378OX", "渠道1 · P5378OX · 537872 · 美国通用卡", "537872", "ch1"},
		{2, "PP5259RC", "渠道4 · PP5259RC · 525962/555671/544015 · 美国随机", "525962", "ch4"},
		{3, "USMAB01", "渠道3 · USMAB01 · 555671/525962/544015 · AI订阅通用", "USMAB01", "ch3"},
	}
	for _, d := range defaults {
		if _, err := DB.Exec(`
			INSERT INTO card_selection_rules (sort_order, plan_key, display_name, bin_prefix, channel, enabled)
			VALUES (?, ?, ?, ?, ?, 1)
		`, d.sortOrder, d.planKey, d.displayName, d.binPrefix, d.channel); err != nil {
			log.Printf("migrateDefaultCardSelectionRules: %v", err)
		}
	}
	log.Println("✓ 已写入默认选卡优先级规则")
	return nil
}

func Close() error {
	if DB != nil {
		return DB.Close()
	}
	return nil
}
