package cardplatform

import "testing"

// 2026-08-22 线上问题的回归锁：代理后台列出了 Claude 档位还能发码。
// 卡台把 ACC 的整张定价表透传过来，ACC 的 claude_max_5x/claude_max_20x 默认 enabled=true，
// 而 CDK 根本没有 Claude 兑换流程。
func TestSellablePlansDropsOtherProductLines(t *testing.T) {
	r := &PlansResponse{
		Plans: map[string]PlanInfo{
			"plus":           {Key: "plus", Label: "Plus", Enabled: true, ServiceFeeUsdMinor: 100},
			"claude_max_5x":  {Key: "claude_max_5x", Label: "Claude Max 5x", Enabled: true, ServiceFeeUsdMinor: 1000},
			"claude_max_20x": {Key: "claude_max_20x", Label: "Claude Max 20x", Enabled: true, ServiceFeeUsdMinor: 1500},
		},
	}
	// 注册表缺失（老版本卡台）走前缀回落
	got := r.SellableKeys()
	if !got["plus"] {
		t.Fatal("plus 应可卖")
	}
	for _, k := range []string{"claude_max_5x", "claude_max_20x"} {
		if got[k] {
			t.Fatalf("%s 不该出现：CDK 没有 Claude 兑换流程", k)
		}
	}
}

// 卡台注册表按产品线过滤过，有它就以它为准——Claude 压根不在 GPT 注册表里。
func TestSellablePlansPrefersRegistry(t *testing.T) {
	r := &PlansResponse{
		Plans: map[string]PlanInfo{
			"plus":          {Enabled: true, ServiceFeeUsdMinor: 100},
			"credit500":     {Enabled: true, ServiceFeeUsdMinor: 10},
			"claude_max_5x": {Enabled: true, ServiceFeeUsdMinor: 1000},
		},
		Registry: []PlanRegistryItem{
			{Key: "credit500", Label: "Codex 点数 500", Flow: "credit", SortOrder: 6, IsCredit: true,
				RequiresActiveSubscription: true, CheckoutCurrency: "PHP", CheckoutAmountMinor: 113000},
			{Key: "plus", Label: "Plus", Flow: "direct", SortOrder: 2},
		},
	}
	plans := r.SellablePlans()
	if len(plans) != 2 {
		t.Fatalf("期望 2 个可卖档位，得到 %d", len(plans))
	}
	// 顺序按卡台的 sort_order，不按 map 迭代顺序
	if plans[0].Key != "plus" || plans[1].Key != "credit500" {
		t.Fatalf("展示顺序应随卡台 sort_order，得到 %v/%v", plans[0].Key, plans[1].Key)
	}
	// ★点数的比索付款价必须透出★：只给 $0.10 服务费的话，
	// 代理会把一张 ₱1130 的码当成一毛钱的东西发。
	c := plans[1]
	if c.CheckoutCurrency != "PHP" || c.CheckoutAmountMinor != 113000 {
		t.Fatalf("点数付款价丢了：%s %d", c.CheckoutCurrency, c.CheckoutAmountMinor)
	}
	if !c.IsCredit || !c.RequiresActiveSubscription {
		t.Fatal("点数的性质字段丢了（需已有订阅才能买）")
	}
}

// ACC 定价里停用的档位不能卖：兑换时 ACC 的 createJobHandler 会挡下，
// 显示出来只会让代理发一张必然兑不掉的码。
func TestSellablePlansRespectsACCDisabled(t *testing.T) {
	r := &PlansResponse{
		Plans: map[string]PlanInfo{
			"plus":      {Enabled: true, ServiceFeeUsdMinor: 100},
			"credit250": {Enabled: false, ServiceFeeUsdMinor: 10},
		},
		Registry: []PlanRegistryItem{
			{Key: "plus", Label: "Plus", SortOrder: 2},
			{Key: "credit250", Label: "Codex 点数 250", SortOrder: 5, IsCredit: true},
		},
	}
	if got := r.SellableKeys(); got["credit250"] {
		t.Fatal("ACC 停用的档位不该可卖")
	}
}

// 卡台注册表有、ACC 定价表还没有（档位同步没跑到）：同样不能卖。
func TestSellablePlansDropsPlanMissingFromACC(t *testing.T) {
	r := &PlansResponse{
		Plans:    map[string]PlanInfo{"plus": {Enabled: true, ServiceFeeUsdMinor: 100}},
		Registry: []PlanRegistryItem{{Key: "plus"}, {Key: "brand_new", Label: "新档位"}},
	}
	if got := r.SellableKeys(); got["brand_new"] {
		t.Fatal("ACC 还不认识的档位不该可卖：下单会被判未知档位")
	}
}
