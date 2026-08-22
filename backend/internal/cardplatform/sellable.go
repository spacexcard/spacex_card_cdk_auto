package cardplatform

import (
	"sort"
	"strings"
)

// 档位可见性的唯一判据都在这个文件里。
//
// 背景（2026-08-22 线上问题）：代理后台把 Claude 档位也列了出来，还能发码——
// 但 Claude 直冲根本没上线，CDK 也没有 Claude 兑换流程。
// 追下去发现是两层都没有把关：
//
//	① 卡台 OpenAPI 的 plans 是**把 ACC 的定价表原样透传**的，ACC 的
//	   PAYMENT_PLAN_KEYS 里就有 claude_pro/claude_max_5x/claude_max_20x，
//	   而且 claude_max_5x/20x 的默认 enabled 还是 true；
//	② 代理侧「拿不到卡台注册表就按定价键渲染」的回落，把这些键全铺到了界面上。
//
// 所以这里按两个闸串联判定，任一不过就不显示、也不许发码：
//
//	闸一 卡台注册表：注册表是按产品线（GPT 直冲）过滤过、且只含已启用档位的。
//	闸二 ACC 定价开关：兑换时 ACC 的 createJobHandler 会查 config.plans[plan].enabled，
//	     这里不查的话，代理能发出一张「兑换时必然被 ACC 挡下」的码。
//
// 两个闸缺一不可：卡台开了 ACC 没开 → 码发得出兑不掉；ACC 开了卡台没开（Claude
// 正是这种）→ 卖了一个我们压根没有兑换流程的东西。

// nonCDKPlanPrefixes 老版本卡台（不下发注册表）时的兜底黑名单。
//
// ★这是过渡期补丁，不是长久判据★：卡台一旦下发 registry，闸一就是产品线过滤，
// 这份前缀名单不再参与判断。留着是为了「卡台还没升级时代理也不该看到 Claude」。
var nonCDKPlanPrefixes = []string{"claude_"}

// SellablePlan 一个「代理真的能发码、用户真的能兑换」的档位。
type SellablePlan struct {
	Key                        string  `json:"key"`
	Label                      string  `json:"label"`
	Flow                       string  `json:"flow"`
	Currency                   string  `json:"currency"`
	SortOrder                  int     `json:"sort_order"`
	IsCredit                   bool    `json:"is_credit"`
	RequiresActiveSubscription bool    `json:"requires_active_subscription"`
	ServiceFeeUsdMinor         int64   `json:"serviceFeeUsdMinor"`
	ServiceFeeUSD              float64 `json:"service_fee_usd"`
	ExpectedAmountMinor        int64   `json:"expectedAmountMinor,omitempty"`
	// 上游实际付款价：点数按 PHP 计价，代理垫的是这笔钱，不是服务费。
	CheckoutCurrency    string `json:"checkout_currency,omitempty"`
	CheckoutAmountMinor int64  `json:"checkout_amount_minor,omitempty"`
}

// SellablePlans 按展示顺序返回可发码的档位。
//
// 卡台下发注册表时以注册表为准（顺序、文案、性质都来自卡台，代理侧零清单）；
// 老版本卡台没有注册表时回落到定价键，并按前缀剔除非本系统售卖的产品线。
func (r *PlansResponse) SellablePlans() []SellablePlan {
	if r == nil {
		return nil
	}
	out := make([]SellablePlan, 0, len(r.Plans))

	if len(r.Registry) > 0 {
		for _, item := range r.Registry {
			key := strings.TrimSpace(item.Key)
			if key == "" {
				continue
			}
			info, ok := r.Plans[key]
			// 注册表有、ACC 定价里没有：ACC 还没认识这个档位（同步没跑到），
			// 兑换必然失败，不能显示。
			if !ok || !info.Enabled {
				continue
			}
			out = append(out, SellablePlan{
				Key: key, Label: nonEmpty(item.Label, nonEmpty(info.Label, key)),
				Flow: item.Flow, Currency: info.Currency, SortOrder: item.SortOrder,
				IsCredit: item.IsCredit, RequiresActiveSubscription: item.RequiresActiveSubscription,
				ServiceFeeUsdMinor: info.ServiceFeeUsdMinor,
				ServiceFeeUSD:      MinorToUSD(info.ServiceFeeUsdMinor),
				ExpectedAmountMinor: info.ExpectedAmountMinor,
				CheckoutCurrency:    item.CheckoutCurrency,
				CheckoutAmountMinor: item.CheckoutAmountMinor,
			})
		}
		sort.SliceStable(out, func(i, j int) bool {
			if out[i].SortOrder != out[j].SortOrder {
				return out[i].SortOrder < out[j].SortOrder
			}
			return out[i].Key < out[j].Key
		})
		return out
	}

	for key, info := range r.Plans {
		if !info.Enabled || !cdkSellableKey(key) {
			continue
		}
		out = append(out, SellablePlan{
			Key: key, Label: nonEmpty(info.Label, key), Currency: info.Currency,
			ServiceFeeUsdMinor:  info.ServiceFeeUsdMinor,
			ServiceFeeUSD:       MinorToUSD(info.ServiceFeeUsdMinor),
			ExpectedAmountMinor: info.ExpectedAmountMinor,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// SellableKeys 可发码档位的键集合，供发码前置校验用。
func (r *PlansResponse) SellableKeys() map[string]bool {
	plans := r.SellablePlans()
	out := make(map[string]bool, len(plans))
	for _, p := range plans {
		out[p.Key] = true
	}
	return out
}

// cdkSellableKey 老版本卡台回落路径：按产品线前缀剔除本系统不售卖的档位。
func cdkSellableKey(key string) bool {
	k := strings.ToLower(strings.TrimSpace(key))
	if k == "" {
		return false
	}
	for _, p := range nonCDKPlanPrefixes {
		if strings.HasPrefix(k, p) {
			return false
		}
	}
	return true
}
