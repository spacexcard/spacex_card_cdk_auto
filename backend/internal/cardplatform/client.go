// Package cardplatform 封装卡台 Open API + 公开 CDK 兑换接口。
// 文档：docs/spacexcard-cdk-zh.md、docs/spacexcard-openapi-zh.md
package cardplatform

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/tuzi/cdk-recharge-system/internal/db"
)

const defaultBase = "https://spacexcard.com"

// Config 从 site_settings（优先）与环境变量读取。
type Config struct {
	// SiteBase 如 https://spacexcard.com（不含 /openapi）
	SiteBase string
	// APIKey sk_...
	APIKey string
}

func LoadConfig() Config {
	base, _ := db.GetSetting("card_api_base")
	key, _ := db.GetSetting("card_api_key")
	if strings.TrimSpace(base) == "" {
		base = os.Getenv("CARD_API_BASE")
	}
	if strings.TrimSpace(key) == "" {
		key = os.Getenv("CARD_API_KEY")
	}
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	// 兼容用户填了 .../openapi/v1
	base = strings.TrimSuffix(base, "/openapi/v1")
	base = strings.TrimSuffix(base, "/openapi")
	if base == "" {
		base = defaultBase
	}
	return Config{SiteBase: base, APIKey: strings.TrimSpace(key)}
}

func (c Config) OpenAPIBase() string {
	return c.SiteBase + "/openapi/v1"
}

func (c Config) PublicCDKBase() string {
	return c.SiteBase + "/api/v1/cdk"
}

type Client struct {
	cfg    Config
	client *http.Client
}

func New(cfg Config) *Client {
	return &Client{
		cfg: cfg,
		client: &http.Client{
			Timeout: 45 * time.Second,
		},
	}
}

func NewFromSettings() *Client {
	return New(LoadConfig())
}

// ---- Open API envelope ----

type envelope struct {
	Code  int             `json:"code"`
	Msg   string          `json:"msg"`
	Data  json.RawMessage `json:"data"`
	Error string          `json:"error_code"`
}

// APIError 业务/HTTP 错误
type APIError struct {
	HTTPStatus int
	Code       int
	Msg        string
	ErrorCode  string
}

func (e *APIError) Error() string {
	if e.ErrorCode != "" {
		return fmt.Sprintf("cardplatform: http=%d code=%d err=%s msg=%s", e.HTTPStatus, e.Code, e.ErrorCode, e.Msg)
	}
	return fmt.Sprintf("cardplatform: http=%d code=%d msg=%s", e.HTTPStatus, e.Code, e.Msg)
}

func (c *Client) doOpenAPI(ctx context.Context, method, path string, body any, idempotencyKey string) (json.RawMessage, error) {
	if c.cfg.APIKey == "" {
		return nil, &APIError{HTTPStatus: 401, Msg: "card_api_key not configured"}
	}
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rdr = bytes.NewReader(b)
	}
	u := c.cfg.OpenAPIBase() + path
	req, err := http.NewRequestWithContext(ctx, method, u, rdr)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-API-Key", c.cfg.APIKey)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, &APIError{HTTPStatus: resp.StatusCode, Msg: "invalid json from card platform: " + truncate(string(raw), 200)}
	}
	// Open API：成功 code=0（HTTP 可能 200/202）；失败 code!=0 或 HTTP 4xx/5xx
	if resp.StatusCode == 401 || env.Code == 401 {
		return nil, &APIError{HTTPStatus: 401, Code: env.Code, Msg: nonEmpty(env.Msg, "unauthorized"), ErrorCode: env.Error}
	}
	if env.Code != 0 {
		st := resp.StatusCode
		if st < 400 {
			st = http.StatusBadRequest
		}
		return nil, &APIError{HTTPStatus: st, Code: env.Code, Msg: nonEmpty(env.Msg, "business error"), ErrorCode: env.Error}
	}
	if resp.StatusCode >= 400 {
		return nil, &APIError{HTTPStatus: resp.StatusCode, Code: env.Code, Msg: nonEmpty(env.Msg, resp.Status), ErrorCode: env.Error}
	}
	return env.Data, nil
}

func nonEmpty(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// PlanInfo 套餐实时配置（服务费 + 参考区间）
type PlanInfo struct {
	Key                 string `json:"key"`
	Label               string `json:"label"`
	Currency            string `json:"currency"`
	Enabled             bool   `json:"enabled"`
	ServiceFeeUsdMinor  int64  `json:"serviceFeeUsdMinor"`
	ExpectedAmountMinor int64  `json:"expectedAmountMinor,omitempty"`
	MinAmountMinor      int64  `json:"minAmountMinor,omitempty"`
	MaxAmountMinor      int64  `json:"maxAmountMinor,omitempty"`
}

type PlansResponse struct {
	Version int64               `json:"version"`
	Plans   map[string]PlanInfo `json:"plans"`
}

// GetPlans GET /gpt-direct/plans — 实时服务费与套餐开关
// 卡台返回 PaymentConfig：version + plans[key].serviceFeeUsdMinor
func (c *Client) GetPlans(ctx context.Context) (*PlansResponse, error) {
	data, err := c.doOpenAPI(ctx, http.MethodGet, "/gpt-direct/plans", nil, "")
	if err != nil {
		return nil, err
	}
	// 宽松解析：兼容 camelCase / snake_case
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	out := &PlansResponse{Plans: map[string]PlanInfo{}}
	if v, ok := raw["version"]; ok {
		var ver int64
		_ = json.Unmarshal(v, &ver)
		out.Version = ver
	}
	plansRaw, ok := raw["plans"]
	if !ok {
		return out, nil
	}
	var plansMap map[string]map[string]interface{}
	if err := json.Unmarshal(plansRaw, &plansMap); err != nil {
		// 回退标准结构
		var std PlansResponse
		if err2 := json.Unmarshal(data, &std); err2 != nil {
			return nil, err
		}
		return &std, nil
	}
	for k, m := range plansMap {
		p := PlanInfo{Key: k, Enabled: true}
		if s, _ := m["key"].(string); s != "" {
			p.Key = s
		}
		if s, _ := m["label"].(string); s != "" {
			p.Label = s
		}
		if s, _ := m["currency"].(string); s != "" {
			p.Currency = s
		}
		if b, ok := m["enabled"].(bool); ok {
			p.Enabled = b
		}
		p.ServiceFeeUsdMinor = jsonInt64(m, "serviceFeeUsdMinor", "service_fee_usd_minor", "serviceFeeUSDMinor")
		p.ExpectedAmountMinor = jsonInt64(m, "expectedAmountMinor", "expected_amount_minor")
		p.MinAmountMinor = jsonInt64(m, "minAmountMinor", "min_amount_minor")
		p.MaxAmountMinor = jsonInt64(m, "maxAmountMinor", "max_amount_minor")
		out.Plans[k] = p
	}
	return out, nil
}

func jsonInt64(m map[string]interface{}, keys ...string) int64 {
	for _, k := range keys {
		v, ok := m[k]
		if !ok || v == nil {
			continue
		}
		switch t := v.(type) {
		case float64:
			return int64(t)
		case json.Number:
			n, _ := t.Int64()
			return n
		case string:
			var n int64
			fmt.Sscanf(t, "%d", &n)
			return n
		}
	}
	return 0
}

type BalanceResponse struct {
	Balance              json.Number `json:"balance"`
	SpendableBalance     json.Number `json:"spendable_balance"`
	AccountReserveAmount json.Number `json:"account_reserve_amount"`
}

func (c *Client) GetBalance(ctx context.Context) (*BalanceResponse, error) {
	data, err := c.doOpenAPI(ctx, http.MethodGet, "/balance", nil, "")
	if err != nil {
		return nil, err
	}
	var out BalanceResponse
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

type IssueCDKRequest struct {
	Plan             string `json:"plan"`
	Count            int    `json:"count"`
	FundingConfirmed bool   `json:"funding_confirmed"`
}

type IssuedCDK struct {
	ID             int64  `json:"id"`
	Code           string `json:"code"`
	Plan           string `json:"plan"`
	CodePrefix     string `json:"code_prefix"`
	FeeAmountMinor int64  `json:"fee_amount_minor"`
}

type IssueCDKResult struct {
	Requested int         `json:"requested"`
	Issued    []IssuedCDK `json:"issued"`
}

// IssueCDKs POST /gpt-direct/cdks
func (c *Client) IssueCDKs(ctx context.Context, plan string, count int, idem string) (*IssueCDKResult, error) {
	if count < 1 {
		count = 1
	}
	if count > 50 {
		count = 50
	}
	body := IssueCDKRequest{Plan: plan, Count: count, FundingConfirmed: true}
	data, err := c.doOpenAPI(ctx, http.MethodPost, "/gpt-direct/cdks", body, idem)
	if err != nil {
		return nil, err
	}
	var out IssueCDKResult
	if err := json.Unmarshal(data, &out); err != nil {
		// 有的实现 data 直接是 issued 数组包一层
		var alt struct {
			Requested int         `json:"requested"`
			Issued    []IssuedCDK `json:"issued"`
		}
		if err2 := json.Unmarshal(data, &alt); err2 != nil {
			return nil, err
		}
		out = IssueCDKResult(alt)
	}
	return &out, nil
}

type CDKListItem struct {
	ID             int64  `json:"id"`
	Plan           string `json:"plan"`
	CodePrefix     string `json:"code_prefix"`
	Status         string `json:"status"`
	FeeAmountMinor int64  `json:"fee_amount_minor"`
	CreatedAt      string `json:"created_at"`
}

type CDKListResult struct {
	List  []CDKListItem `json:"list"`
	Total int           `json:"total"`
}

func (c *Client) ListCDKs(ctx context.Context, page, pageSize int) (*CDKListResult, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	path := fmt.Sprintf("/gpt-direct/cdks?page=%d&page_size=%d", page, pageSize)
	data, err := c.doOpenAPI(ctx, http.MethodGet, path, nil, "")
	if err != nil {
		return nil, err
	}
	var out CDKListResult
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	// 兼容 total 为 float64 / string 的松散 JSON
	if out.Total == 0 && len(out.List) > 0 {
		var loose map[string]any
		if json.Unmarshal(data, &loose) == nil {
			out.Total = anyToInt(loose["total"])
		}
	}
	return &out, nil
}

// CDKStatusSummary 卡台 CDK 按状态汇总（翻页聚合）。
type CDKStatusSummary struct {
	Total    int            `json:"total"`
	ByStatus map[string]int `json:"by_status"`
	Partial  bool           `json:"partial"` // 超过扫描上限时为 true
	Scanned  int            `json:"scanned"`
}

// SummarizeCDKs 分页拉取本 API 作用域下 CDK，汇总 total 与各 status。
// maxPages 限制最大扫描页数（每页 100）；0 默认 50（最多约 5000 张）。
func (c *Client) SummarizeCDKs(ctx context.Context, maxPages int) (*CDKStatusSummary, error) {
	if maxPages <= 0 {
		maxPages = 50
	}
	const pageSize = 100
	sum := &CDKStatusSummary{ByStatus: map[string]int{}}
	for page := 1; page <= maxPages; page++ {
		res, err := c.ListCDKs(ctx, page, pageSize)
		if err != nil {
			return nil, err
		}
		if page == 1 {
			sum.Total = res.Total
		}
		if len(res.List) == 0 {
			break
		}
		for _, item := range res.List {
			st := strings.TrimSpace(item.Status)
			if st == "" {
				st = "unknown"
			}
			sum.ByStatus[st]++
			sum.Scanned++
		}
		if sum.Scanned >= sum.Total || len(res.List) < pageSize {
			break
		}
		if page == maxPages && sum.Scanned < sum.Total {
			sum.Partial = true
		}
	}
	// 若卡台 total 为 0 但扫到了条目，用扫描数兜底
	if sum.Total == 0 && sum.Scanned > 0 {
		sum.Total = sum.Scanned
	}
	return sum, nil
}

// ListCDKOrdersTotal 只取 CDK 订单 total（第一页）。
func (c *Client) ListCDKOrdersTotal(ctx context.Context) (int, error) {
	raw, err := c.ListCDKOrders(ctx, 1, 1)
	if err != nil {
		return 0, err
	}
	if len(raw) == 0 {
		return 0, nil
	}
	var loose map[string]any
	if err := json.Unmarshal(raw, &loose); err != nil {
		return 0, err
	}
	return anyToInt(loose["total"]), nil
}

func anyToInt(v any) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case int64:
		return int(t)
	case json.Number:
		n, _ := t.Int64()
		return int(n)
	case string:
		var n int
		fmt.Sscanf(strings.TrimSpace(t), "%d", &n)
		return n
	default:
		return 0
	}
}

// CDKOrderListQuery 对账列表查询参数（转发卡台 OpenAPI）。
type CDKOrderListQuery struct {
	Page     int
	PageSize int
	Status   string // completed / queued / ...
	CDKID    int64
	OrderID  int64
}

// ListCDKOrders GET /gpt-direct/cdk-orders
func (c *Client) ListCDKOrders(ctx context.Context, page, pageSize int) (json.RawMessage, error) {
	return c.ListCDKOrdersQuery(ctx, CDKOrderListQuery{Page: page, PageSize: pageSize})
}

// ListCDKOrdersQuery GET /gpt-direct/cdk-orders?page=&page_size=&status=&cdk_id=&order_id=
func (c *Client) ListCDKOrdersQuery(ctx context.Context, q CDKOrderListQuery) (json.RawMessage, error) {
	page, pageSize := q.Page, q.PageSize
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	v := url.Values{}
	v.Set("page", strconv.Itoa(page))
	v.Set("page_size", strconv.Itoa(pageSize))
	if s := strings.TrimSpace(q.Status); s != "" {
		v.Set("status", strings.ToLower(s))
	}
	if q.CDKID > 0 {
		v.Set("cdk_id", strconv.FormatInt(q.CDKID, 10))
	}
	if q.OrderID > 0 {
		v.Set("order_id", strconv.FormatInt(q.OrderID, 10))
	}
	return c.doOpenAPI(ctx, http.MethodGet, "/gpt-direct/cdk-orders?"+v.Encode(), nil, "")
}

// GetCDKOrder GET /gpt-direct/cdk-orders/:id
func (c *Client) GetCDKOrder(ctx context.Context, id string) (json.RawMessage, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, &APIError{HTTPStatus: 400, Msg: "id required"}
	}
	return c.doOpenAPI(ctx, http.MethodGet, "/gpt-direct/cdk-orders/"+url.PathEscape(id), nil, "")
}

// DeleteCard DELETE /cards/{id} — 永久删卡，卡内余额退回平台余额。
func (c *Client) DeleteCard(ctx context.Context, cardID string) (json.RawMessage, error) {
	cardID = strings.TrimSpace(cardID)
	if cardID == "" {
		return nil, &APIError{HTTPStatus: 400, Msg: "card id required"}
	}
	return c.doOpenAPI(ctx, http.MethodDelete, "/cards/"+url.PathEscape(cardID), nil, "")
}

// ---- 公开兑换（无需 API Key）----

func (c *Client) doPublicCDK(ctx context.Context, method, path string, body any, device string) (int, json.RawMessage, error) {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		rdr = bytes.NewReader(b)
	}
	u := c.cfg.PublicCDKBase() + path
	req, err := http.NewRequestWithContext(ctx, method, u, rdr)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if device != "" {
		req.Header.Set("X-Redemption-Device", device)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	// 公开接口可能直接返回 JSON 对象，不一定是 code/msg envelope
	return resp.StatusCode, json.RawMessage(raw), nil
}

func (c *Client) Preview(ctx context.Context, code, device string) (int, json.RawMessage, error) {
	return c.doPublicCDK(ctx, http.MethodPost, "/preview", map[string]string{"code": code}, device)
}

func (c *Client) Preflight(ctx context.Context, body any, device string) (int, json.RawMessage, error) {
	return c.doPublicCDK(ctx, http.MethodPost, "/preflight", body, device)
}

func (c *Client) Redeem(ctx context.Context, body any, device string) (int, json.RawMessage, error) {
	return c.doPublicCDK(ctx, http.MethodPost, "/redeem", body, device)
}

func (c *Client) Result(ctx context.Context, token, device string) (int, json.RawMessage, error) {
	q := url.Values{}
	q.Set("token", token)
	return c.doPublicCDK(ctx, http.MethodGet, "/result?"+q.Encode(), nil, device)
}

// MinorToUSD 美分 → 美元展示
func MinorToUSD(minor int64) float64 {
	return float64(minor) / 100.0
}
