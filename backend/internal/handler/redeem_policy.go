package handler

import (
	"encoding/json"
	"strings"

	"github.com/tuzi/cdk-recharge-system/internal/db"
)

const siteRedeemPolicyKey = "site_redeem_policy"

// SiteRedeemPolicy 本站可控兑换策略（不依赖 ACC；经卡台协议下发 no_auto_card_switch / 发码偏好）。
// 一卡几付的硬限制仍由卡台账户侧容量策略执行；此处把偏好与「是否自动换卡」交给本站。
type SiteRedeemPolicy struct {
	Enabled bool `json:"enabled"`
	// NoAutoCardSwitch=true → 兑换时向卡台传 no_auto_card_switch（失败不自动换卡）
	NoAutoCardSwitch bool `json:"no_auto_card_switch"`
	// AutoOpenWhenNoCard：展示/说明用；卡台 CDK 兑换默认 auto_open，本字段预留与文档对齐
	AutoOpenWhenNoCard bool `json:"auto_open_when_no_card"`
	// 每卡新账号上限（展示 + 写入审计；硬限以卡台为准）
	MaxNewAccountsPerCard int `json:"max_new_accounts_per_card"`
	// 单任务最多卡数（预留）
	MaxCardsPerTask int `json:"max_cards_per_task"`
	// 失败冷却小时（预留展示）
	FailCooldownHours int `json:"fail_cooldown_hours"`
	// 限定发卡地区文案
	IssuingArea string `json:"issuing_area"`
	// 持卡人
	HolderFirst string `json:"holder_first"`
	HolderLast  string `json:"holder_last"`
	// 指定产品码：空则用「选卡配置」第一条启用规则的 plan_key
	ProductCode string `json:"product_code"`
	// 渠道：one/three/four；空则从产品缓存推断
	Issuer string `json:"issuer"`
}

func defaultSiteRedeemPolicy() SiteRedeemPolicy {
	return SiteRedeemPolicy{
		Enabled:               false,
		NoAutoCardSwitch:      true, // 启用本站策略时默认不让卡台自动换卡
		AutoOpenWhenNoCard:    true,
		MaxNewAccountsPerCard: 4,
		MaxCardsPerTask:       3,
		FailCooldownHours:      24,
		IssuingArea:           "United States",
		HolderFirst:           "GPT",
		HolderLast:            "Direct",
	}
}

func loadSiteRedeemPolicy() SiteRedeemPolicy {
	p := defaultSiteRedeemPolicy()
	raw, err := db.GetSetting(siteRedeemPolicyKey)
	if err != nil || strings.TrimSpace(raw) == "" {
		return p
	}
	_ = json.Unmarshal([]byte(raw), &p)
	if p.MaxNewAccountsPerCard <= 0 {
		p.MaxNewAccountsPerCard = 4
	}
	if p.MaxCardsPerTask <= 0 {
		p.MaxCardsPerTask = 3
	}
	if p.FailCooldownHours <= 0 {
		p.FailCooldownHours = 24
	}
	return p
}

func saveSiteRedeemPolicy(p SiteRedeemPolicy) error {
	if p.MaxNewAccountsPerCard <= 0 {
		p.MaxNewAccountsPerCard = 4
	}
	if p.MaxCardsPerTask <= 0 {
		p.MaxCardsPerTask = 3
	}
	if p.FailCooldownHours < 0 {
		p.FailCooldownHours = 0
	}
	p.ProductCode = strings.TrimSpace(p.ProductCode)
	p.Issuer = strings.ToLower(strings.TrimSpace(p.Issuer))
	p.IssuingArea = strings.TrimSpace(p.IssuingArea)
	p.HolderFirst = strings.TrimSpace(p.HolderFirst)
	p.HolderLast = strings.TrimSpace(p.HolderLast)
	b, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return db.SetSetting(siteRedeemPolicyKey, string(b))
}

// resolveIssueCardPref 发码偏好：策略指定产品 > 选卡配置首条启用规则。
func resolveIssueCardPref(policy SiteRedeemPolicy) (issuer, segmentType, segmentKey string) {
	if !policy.Enabled {
		return "", "", ""
	}
	segmentKey = strings.TrimSpace(policy.ProductCode)
	issuer = strings.ToLower(strings.TrimSpace(policy.Issuer))
	if segmentKey == "" {
		rules, err := db.GetCardSelectionRules()
		if err == nil {
			for _, r := range rules {
				if !r.Enabled {
					continue
				}
				pk := strings.TrimSpace(r.PlanKey)
				if pk == "" {
					continue
				}
				segmentKey = pk
				if ch := strings.TrimSpace(r.Channel); ch != "" {
					issuer = strings.ToLower(ch)
				}
				break
			}
		}
	}
	if segmentKey == "" {
		return issuer, "", ""
	}
	// 从产品缓存补渠道
	if issuer == "" {
		if prods, err := db.GetCardProducts(); err == nil {
			for _, pr := range prods {
				if strings.EqualFold(pr.ProductCode, segmentKey) {
					issuer = strings.ToLower(strings.TrimSpace(pr.Issuer))
					break
				}
			}
		}
	}
	if issuer == "" {
		issuer = "one"
	}
	return issuer, "product", segmentKey
}
