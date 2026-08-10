// Package plansync 每3分钟从卡台拉取一次产品状态并缓存到本地 SQLite。
package plansync

import (
	"context"
	"log"
	"time"

	"github.com/tuzi/cdk-recharge-system/internal/cardplatform"
	"github.com/tuzi/cdk-recharge-system/internal/db"
)

const syncInterval = 3 * time.Minute

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
func SyncNow(ctx context.Context) (int, error) {
	return doSync(ctx)
}

func syncOnce(ctx context.Context) {
	ctx2, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	n, err := doSync(ctx2)
	if err != nil {
		log.Printf("[plan-sync] error: %v", err)
		return
	}
	log.Printf("[plan-sync] synced %d plans", n)
}

func doSync(ctx context.Context) (int, error) {
	cfg := cardplatform.LoadConfig()
	if cfg.APIKey == "" {
		return 0, nil // 未配置 API Key，静默跳过
	}
	cli := cardplatform.New(cfg)
	plans, err := cli.GetPlans(ctx)
	if err != nil {
		return 0, err
	}
	n := 0
	for key, p := range plans.Plans {
		if err := db.UpsertPlanStatus(key, p.Label, p.Enabled, p.ServiceFeeUsdMinor); err != nil {
			log.Printf("[plan-sync] upsert %s: %v", key, err)
		} else {
			n++
		}
	}
	return n, nil
}
