package handler

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tuzi/cdk-recharge-system/internal/cardplatform"
)

// NetworkEgress GET /api/v1/admin/network/egress
// 探测本机（服1）访问公网时的出口 IP，便于卡台 API Key 配置 IP 白名单。
func NetworkEgress(c *gin.Context) {
	cfg := cardplatform.LoadConfig()
	ip, source, err := detectEgressIP(c.Request.Context())
	out := gin.H{
		"card_site_base":   cfg.SiteBase,
		"openapi_base":     cfg.OpenAPIBase(),
		"public_cdk_base":  cfg.PublicCDKBase(),
		"api_key_configured": strings.TrimSpace(cfg.APIKey) != "",
		"presets": []gin.H{
			{"id": "prod", "label": "生产", "site_base": "https://spacexcard.com", "openapi": "https://spacexcard.com/openapi/v1", "cdk": "https://spacexcard.com/api/v1/cdk"},
			{"id": "sandbox", "label": "沙盒", "site_base": "https://sandbox.spacexcard.com", "openapi": "https://sandbox.spacexcard.com/openapi/v1", "cdk": "https://sandbox.spacexcard.com/api/v1/cdk"},
		},
	}
	if err != nil {
		out["egress_ip"] = ""
		out["egress_error"] = err.Error()
		out["egress_source"] = ""
		c.JSON(http.StatusOK, out)
		return
	}
	out["egress_ip"] = ip
	out["egress_source"] = source
	out["whitelist_hint"] = "请把出口 IP 填到卡台开发者页 → API Key → IP 白名单（本站发码/拉价格从此 IP 出网）"
	c.JSON(http.StatusOK, out)
}

func detectEgressIP(ctx context.Context) (ip, source string, err error) {
	client := &http.Client{Timeout: 6 * time.Second}
	// 多个公共接口兜底
	type probe struct {
		url    string
		source string
	}
	probes := []probe{
		{"https://api.ipify.org?format=text", "ipify"},
		{"https://ifconfig.me/ip", "ifconfig.me"},
		{"https://icanhazip.com", "icanhazip"},
	}
	var last error
	for _, p := range probes {
		req, e := http.NewRequestWithContext(ctx, http.MethodGet, p.url, nil)
		if e != nil {
			last = e
			continue
		}
		req.Header.Set("User-Agent", "cdk-recharge-system/egress-check")
		resp, e := client.Do(req)
		if e != nil {
			last = e
			continue
		}
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 64))
		_ = resp.Body.Close()
		if resp.StatusCode != 200 {
			last = errStatus(resp.StatusCode)
			continue
		}
		s := strings.TrimSpace(string(b))
		// 简单校验 IPv4/IPv6 字符
		if s == "" || strings.ContainsAny(s, " \n\t<>") {
			last = errStatus(0)
			continue
		}
		return s, p.source, nil
	}
	if last == nil {
		last = errStatus(0)
	}
	return "", "", last
}

type statusErr int

func (e statusErr) Error() string {
	if e == 0 {
		return "could not detect egress IP"
	}
	return "egress probe http " + http.StatusText(int(e))
}

func errStatus(code int) error { return statusErr(code) }
