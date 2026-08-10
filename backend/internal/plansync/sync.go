// Package plansync 每3分钟从卡台同步逻辑套餐状态和实体产品列表。
package plansync

import (
	"context"
	"log"
	"time"

	"github.com/tuzi/cdk-recharge-system/internal/cardplatform"
	"github.com/tuzi/cdk-recharge-system/internal/db"
)

const syncInterval = 3 * time.Minute

// SyncResult 同步结果摘要。
type SyncResult struct {
	Plans    int
	Products int
}

// Start 启动后台产品状态同步（goroutine；ctx.Done() 时优雅退出）。
func Start(ctx context.Context) {
	go run(ctx)
}

func run(ctx context.Context) {
	// 启动时立即同步一次
	syncOnce(ctx)
	ticker := time.NewTicker(syncInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Println("[plan-sync] stopped")
			return
		case <-ticker.C:
			syncOnce(ctx)
		}
	}
}

// SyncNow 供 handler 主动触发（同步调用，有 ctx 超时保护）。
func SyncNow(ctx context.Context) (SyncResult, error) {
	ctx2, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return doSync(ctx2)
}

func syncOnce(ctx context.Context) {
	ctx2, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	r, err := doSync(ctx2)
	if err != nil {
		log.Printf("[plan-sync] error: %v", err)
		return
	}
	log.Printf("[plan-sync] synced %d plans, %d products", r.Plans, r.Products)
}

func doSync(ctx context.Context) (SyncResult, error) {
	cfg := cardplatform.LoadConfig()
	if cfg.APIKey == "" {
		return SyncResult{}, nil // 未配置 API Key，静默跳过
	}
	cli := cardplatform.New(cfg)
	var res SyncResult

	// 1. 同步逻辑套餐（plus / pro_5x / pro_20x）
	plans, err := cli.GetPlans(ctx)
	if err != nil {
		return res, err
	}
	for key, p := range plans.Plans {
		if err := db.UpsertPlanStatus(key, p.Label, p.Enabled, p.ServiceFeeUsdMinor); err != nil {
			log.Printf("[plan-sync] upsert plan %s: %v", key, err)
		} else {
			res.Plans++
		}
	}

	// 2. 同步实体产品（product_code + BIN + enabled）
	products, err := cli.GetProducts(ctx)
	if err != nil {
		// 产品接口失败不阻断套餐同步结果
		log.Printf("[plan-sync] GetProducts error: %v", err)
		return res, nil
	}
	for _, p := range products {
		cp := db.CardProductCache{
			ProductCode: p.ProductCode,
			Issuer:      p.Issuer,
			BIN:         p.BIN,
			Network:     p.Network,
			IssuingArea: p.IssuingArea,
			Scene:       p.Scene,
			CardGroup:   p.CardGroup,
			Description: p.Description,
			BinHeads:    p.BinHeads,
			Enabled:     p.Enabled,
			SuspendedAt: p.SuspendedAt,
		}
		if err := db.UpsertCardProduct(cp); err != nil {
			log.Printf("[plan-sync] upsert product %s: %v", p.ProductCode, err)
		} else {
			res.Products++
		}
	}
	return res, nil
}
