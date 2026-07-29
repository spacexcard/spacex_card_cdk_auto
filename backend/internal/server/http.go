package server

import (
	"context"
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tuzi/cdk-recharge-system/internal/config"
	"github.com/tuzi/cdk-recharge-system/internal/db"
	"github.com/tuzi/cdk-recharge-system/internal/handler"
)

type Server struct {
	engine *gin.Engine
	cfg    *config.Config
}

func New(ctx context.Context, cfg *config.Config) (*Server, error) {
	// 生产模式下强制要求安全的 JWT secret，避免 token 被伪造
	if err := enforceJWTSecret(cfg.Server.Mode); err != nil {
		return nil, err
	}

	// Initialize database
	if err := db.Init(&cfg.Database); err != nil {
		return nil, err
	}

	engine := gin.Default()

	// 不信任任意代理头（X-Forwarded-For 等），保证限流拿到的是真实来源 IP。
	// 若部署在已知反代后，用 TRUSTED_PROXIES（逗号分隔）显式声明。
	if tp := strings.TrimSpace(os.Getenv("TRUSTED_PROXIES")); tp != "" {
		_ = engine.SetTrustedProxies(splitAndTrim(tp))
	} else {
		_ = engine.SetTrustedProxies(nil)
	}

	// Setup middleware
	engine.Use(SecurityHeadersMiddleware())
	engine.Use(CORSMiddleware())

	// Setup routes
	setupRoutes(engine)

	return &Server{
		engine: engine,
		cfg:    cfg,
	}, nil
}

// enforceJWTSecret 在 release 模式下拒绝使用空/默认密钥启动。
func enforceJWTSecret(mode string) error {
	secret := os.Getenv("JWT_SECRET")
	weak := secret == "" ||
		secret == "your-secret-key-change-in-production" ||
		secret == "dev-secret-key-change-in-production" ||
		len(secret) < 16
	if mode == "release" && weak {
		return fmt.Errorf("不安全的 JWT_SECRET：release 模式必须设置至少 16 位的随机 JWT_SECRET 环境变量")
	}
	if weak {
		log.Printf("⚠️  当前 JWT_SECRET 不安全（默认/过短）。仅可用于本地开发，部署前请设置强随机 JWT_SECRET（release 模式将拒绝启动）。")
	}
	return nil
}

func splitAndTrim(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func setupRoutes(r *gin.Engine) {
	webDir := strings.TrimSpace(os.Getenv("WEB_DIR"))

	// Root route for quick manual checks in browser (仅当未托管前端时)
	if webDir == "" {
		r.GET("/", func(c *gin.Context) {
			c.JSON(200, gin.H{
				"status":   "ok",
				"message":  "Recharge System backend is running",
				"health":   "/health",
				"api_base": "/api/v1",
			})
		})
	}

	// Health check
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status": "ok",
			"message": "Recharge System is running",
		})
	})

	// API v1 routes
	api := r.Group("/api/v1")
	{
		api.GET("", func(c *gin.Context) {
			c.JSON(200, gin.H{
				"status":  "ok",
				"message": "API v1 is available",
			})
		})

		auth := api.Group("/auth")
		{
			auth.POST("/admin/login", handler.AdminLogin)
			auth.POST("/admin/logout", handler.AdminLogout)
			auth.GET("/admin/me", JWTAuthMiddleware(), handler.AdminMe)
			auth.POST("/admin/change-password", JWTAuthMiddleware(), AdminAuthMiddleware(), handler.AdminChangePassword)
		}

		// 首次安装向导（仅 pending；完成后 410）
		setup := api.Group("/setup")
		{
			setup.GET("/status", handler.SetupStatus)
			setup.POST("/bootstrap", handler.SetupBootstrap)
		}

		// 公开站点配置（品牌/皮肤，无密钥）
		api.GET("/public/site", handler.PublicSiteConfig)

		// 卡台 CDK 公开兑换 BFF + 实时服务费展示
		pubCDK := api.Group("/public/cdk")
		{
			pubCDK.GET("/plans", handler.PublicCDKPlans)
			pubCDK.POST("/preview", handler.PublicCDKPreview)
			pubCDK.POST("/preflight", handler.PublicCDKPreflight)
			pubCDK.POST("/redeem", handler.PublicCDKRedeem)
			pubCDK.GET("/result", handler.PublicCDKResult)
		}

		// 卡台 Webhook（须在卡台开发者页配置 https://你的域名/api/v1/webhooks/cardplatform）
		api.POST("/webhooks/cardplatform", handler.CardPlatformWebhook)

		// 账单：粘贴 session 查 ChatGPT 订阅 + hosted_invoice（小助手同款）
		api.POST("/public/billing/check", handler.SessionBillingCheck)
		api.POST("/billing/check", handler.SessionBillingCheck)

		// Stats routes
		stats := api.Group("/stats")
		stats.Use(JWTAuthMiddleware(), AdminAuthMiddleware())
		{
			stats.GET("/system", handler.GetSystemStats)
		}

		// Admin routes
		admin := api.Group("/admin")
		admin.Use(JWTAuthMiddleware())
		admin.Use(AdminAuthMiddleware())
		{
			// 版本：本机 VERSION + GitHub 最新 release/tag
			admin.GET("/system/version", handler.AdminSystemVersion)
			admin.GET("/system/update/status", handler.AdminSystemUpdateStatus)
			admin.POST("/system/update", handler.AdminSystemUpdate)

			// 卡台 Open API：实时价格 / 余额 / 发码 / 列码 / CDK 订单
			admin.GET("/cardplatform/ping", handler.CardPlatformPing)
			admin.GET("/cardplatform/plans", handler.CardPlatformPlans)
			admin.GET("/cardplatform/balance", handler.CardPlatformBalance)
			admin.GET("/cardplatform/cdks", handler.CardPlatformListCDKs)
			admin.POST("/cardplatform/cdks", handler.CardPlatformIssueCDKs)
			admin.GET("/cardplatform/cdk-orders", handler.CardPlatformListCDKOrders)
			admin.GET("/cardplatform/cdk-orders/:id", handler.CardPlatformGetCDKOrder)

			// Webhook 事件列表 + 配置提示
			admin.GET("/webhooks/events", handler.AdminListWebhooks)

			// 本机出口 IP（卡台 API 白名单）
			admin.GET("/network/egress", handler.NetworkEgress)

			// Audit logs
			admin.GET("/audit-logs", handler.ListAuditLogs)

			// 站点设置（品牌/皮肤/卡台密钥保险箱）
			admin.GET("/settings", handler.AdminGetSettings)
			admin.PUT("/settings", handler.AdminPutSettings)
		}
	}

	// 托管前端 SPA（当设置了 WEB_DIR 时）：真实存在的文件直出，其余回退到 index.html
	if webDir != "" {
		indexFile := filepath.Join(webDir, "index.html")
		r.NoRoute(func(c *gin.Context) {
			p := c.Request.URL.Path
			if strings.HasPrefix(p, "/api") || strings.HasPrefix(p, "/health") {
				c.JSON(404, gin.H{"error": "not found"})
				return
			}
			if c.Request.Method != "GET" && c.Request.Method != "HEAD" {
				c.JSON(404, gin.H{"error": "not found"})
				return
			}
			// 防目录穿越：path.Clean 后再拼接，确保不越出 webDir
			clean := path.Clean("/" + strings.TrimPrefix(p, "/"))
			if clean != "/" {
				fp := filepath.Join(webDir, filepath.FromSlash(clean))
				if st, err := os.Stat(fp); err == nil && !st.IsDir() {
					c.File(fp)
					return
				}
			}
			c.File(indexFile)
		})
	}
}

func (s *Server) Run(addr string) error {
	return s.engine.Run(addr)
}
