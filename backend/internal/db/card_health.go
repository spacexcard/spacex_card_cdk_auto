package db

import (
	"fmt"
	"strings"
)

// CardFailEvent 一张卡在一笔订单上的失败观察。
type CardFailEvent struct {
	ID               int64  `json:"id"`
	CardID           int64  `json:"card_id"`
	CardLastFour     string `json:"card_last_four"`
	OrderID          int64  `json:"order_id"`
	CDKCode          string `json:"cdk_code,omitempty"`
	AccountEmailNorm string `json:"account_email_norm"`
	EmailSource      string `json:"email_source"`
	ErrorCode        string `json:"error_code"`
	OrderStatus      string `json:"order_status"`
	Verdict          string `json:"verdict"`
	CreatedAt        string `json:"created_at"`
}

// CardBlockEntry 本站坏卡黑名单。
type CardBlockEntry struct {
	CardID         int64  `json:"card_id"`
	CardLastFour   string `json:"card_last_four"`
	Reason         string `json:"reason"`
	DistinctEmails int    `json:"distinct_emails"`
	FailCount      int    `json:"fail_count"`
	FreezeStatus   string `json:"freeze_status"`
	FreezeError    string `json:"freeze_error,omitempty"`
	BlockedAt      string `json:"blocked_at"`
	UnblockedAt    string `json:"unblocked_at,omitempty"`
	Notes          string `json:"notes,omitempty"`
	Active         bool   `json:"active"`
}

// CardFailStats 某卡失败聚合。
type CardFailStats struct {
	CardID         int64
	FailCount      int
	DistinctEmails int
	Emails         []string
}

// InsertCardFailEvent 幂等写入失败事件（同 order_id+card_id 只记一次）。
// inserted=false 表示已存在。
func InsertCardFailEvent(ev CardFailEvent) (inserted bool, err error) {
	if DB == nil {
		return false, fmt.Errorf("db not ready")
	}
	if ev.CardID <= 0 {
		return false, fmt.Errorf("card_id required")
	}
	ev.AccountEmailNorm = strings.ToLower(strings.TrimSpace(ev.AccountEmailNorm))
	if ev.AccountEmailNorm == "" {
		ev.AccountEmailNorm = "unknown"
	}
	ev.CardLastFour = strings.TrimSpace(ev.CardLastFour)
	ev.CDKCode = strings.TrimSpace(ev.CDKCode)
	ev.EmailSource = strings.TrimSpace(ev.EmailSource)
	ev.ErrorCode = strings.TrimSpace(ev.ErrorCode)
	ev.OrderStatus = strings.ToLower(strings.TrimSpace(ev.OrderStatus))
	ev.Verdict = strings.TrimSpace(ev.Verdict)
	if ev.OrderID < 0 {
		ev.OrderID = 0
	}

	res, err := DB.Exec(`
		INSERT OR IGNORE INTO card_fail_events
			(card_id, card_last_four, order_id, cdk_code, account_email_norm, email_source,
			 error_code, order_status, verdict, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	`, ev.CardID, ev.CardLastFour, ev.OrderID, ev.CDKCode, ev.AccountEmailNorm, ev.EmailSource,
		ev.ErrorCode, ev.OrderStatus, ev.Verdict)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// GetCardFailStats 统计某卡失败次数与去重邮箱数（不含 unknown 时仍计入 fail，但 distinct 可配置）。
func GetCardFailStats(cardID int64) (*CardFailStats, error) {
	if DB == nil {
		return nil, fmt.Errorf("db not ready")
	}
	if cardID <= 0 {
		return nil, fmt.Errorf("card_id required")
	}
	rows, err := DB.Query(`
		SELECT account_email_norm FROM card_fail_events
		WHERE card_id = ?
		ORDER BY id ASC
	`, cardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	st := &CardFailStats{CardID: cardID}
	seen := map[string]struct{}{}
	for rows.Next() {
		var em string
		if err := rows.Scan(&em); err != nil {
			return nil, err
		}
		st.FailCount++
		em = strings.ToLower(strings.TrimSpace(em))
		if em == "" || em == "unknown" {
			continue
		}
		if _, ok := seen[em]; !ok {
			seen[em] = struct{}{}
			st.Emails = append(st.Emails, em)
		}
	}
	st.DistinctEmails = len(st.Emails)
	return st, rows.Err()
}

// ListCardFailEvents 最近失败事件。
func ListCardFailEvents(limit int) ([]CardFailEvent, error) {
	if DB == nil {
		return nil, fmt.Errorf("db not ready")
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := DB.Query(`
		SELECT id, card_id, card_last_four, order_id, cdk_code, account_email_norm, email_source,
		       error_code, order_status, verdict, created_at
		FROM card_fail_events
		ORDER BY id DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CardFailEvent
	for rows.Next() {
		var e CardFailEvent
		if err := rows.Scan(&e.ID, &e.CardID, &e.CardLastFour, &e.OrderID, &e.CDKCode, &e.AccountEmailNorm,
			&e.EmailSource, &e.ErrorCode, &e.OrderStatus, &e.Verdict, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// IsCardBlocked 卡是否在本站活跃黑名单。
func IsCardBlocked(cardID int64) (bool, error) {
	if DB == nil || cardID <= 0 {
		return false, nil
	}
	var n int
	err := DB.QueryRow(`
		SELECT COUNT(1) FROM card_blocklist
		WHERE card_id = ? AND unblocked_at IS NULL
	`, cardID).Scan(&n)
	return n > 0, err
}

// UpsertCardBlock 写入/刷新活跃拉黑。
func UpsertCardBlock(entry CardBlockEntry) error {
	if DB == nil {
		return fmt.Errorf("db not ready")
	}
	if entry.CardID <= 0 {
		return fmt.Errorf("card_id required")
	}
	entry.CardLastFour = strings.TrimSpace(entry.CardLastFour)
	entry.Reason = strings.TrimSpace(entry.Reason)
	if entry.Reason == "" {
		entry.Reason = "multi_email_fail"
	}
	entry.FreezeStatus = strings.TrimSpace(entry.FreezeStatus)
	entry.FreezeError = strings.TrimSpace(entry.FreezeError)
	entry.Notes = strings.TrimSpace(entry.Notes)
	_, err := DB.Exec(`
		INSERT INTO card_blocklist
			(card_id, card_last_four, reason, distinct_emails, fail_count, freeze_status, freeze_error, blocked_at, unblocked_at, notes)
		VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, NULL, ?)
		ON CONFLICT(card_id) DO UPDATE SET
			card_last_four = CASE WHEN excluded.card_last_four != '' THEN excluded.card_last_four ELSE card_blocklist.card_last_four END,
			reason = excluded.reason,
			distinct_emails = excluded.distinct_emails,
			fail_count = excluded.fail_count,
			freeze_status = CASE WHEN excluded.freeze_status != '' THEN excluded.freeze_status ELSE card_blocklist.freeze_status END,
			freeze_error = excluded.freeze_error,
			blocked_at = CURRENT_TIMESTAMP,
			unblocked_at = NULL,
			notes = CASE WHEN excluded.notes != '' THEN excluded.notes ELSE card_blocklist.notes END
	`, entry.CardID, entry.CardLastFour, entry.Reason, entry.DistinctEmails, entry.FailCount,
		entry.FreezeStatus, entry.FreezeError, entry.Notes)
	return err
}

// UpdateCardBlockFreeze 更新冻结结果。
func UpdateCardBlockFreeze(cardID int64, status, freezeErr string) error {
	if DB == nil || cardID <= 0 {
		return fmt.Errorf("db not ready")
	}
	_, err := DB.Exec(`
		UPDATE card_blocklist
		SET freeze_status = ?, freeze_error = ?
		WHERE card_id = ? AND unblocked_at IS NULL
	`, strings.TrimSpace(status), strings.TrimSpace(freezeErr), cardID)
	return err
}

// UnblockCard 解除本站拉黑（不自动解冻卡台侧，由调用方决定）。
func UnblockCard(cardID int64, notes string) error {
	if DB == nil || cardID <= 0 {
		return fmt.Errorf("invalid card")
	}
	notes = strings.TrimSpace(notes)
	_, err := DB.Exec(`
		UPDATE card_blocklist
		SET unblocked_at = CURRENT_TIMESTAMP,
		    notes = CASE WHEN ? != '' THEN ? ELSE notes END
		WHERE card_id = ? AND unblocked_at IS NULL
	`, notes, notes, cardID)
	return err
}

// ListCardBlocklist 活跃 + 可选历史。
func ListCardBlocklist(includeInactive bool, limit int) ([]CardBlockEntry, error) {
	if DB == nil {
		return nil, fmt.Errorf("db not ready")
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := `
		SELECT card_id, card_last_four, reason, distinct_emails, fail_count,
		       freeze_status, freeze_error, blocked_at,
		       COALESCE(unblocked_at, ''), notes
		FROM card_blocklist
	`
	if !includeInactive {
		q += ` WHERE unblocked_at IS NULL`
	}
	q += ` ORDER BY blocked_at DESC LIMIT ?`
	rows, err := DB.Query(q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CardBlockEntry
	for rows.Next() {
		var e CardBlockEntry
		var unblocked string
		if err := rows.Scan(&e.CardID, &e.CardLastFour, &e.Reason, &e.DistinctEmails, &e.FailCount,
			&e.FreezeStatus, &e.FreezeError, &e.BlockedAt, &unblocked, &e.Notes); err != nil {
			return nil, err
		}
		e.UnblockedAt = unblocked
		e.Active = unblocked == ""
		out = append(out, e)
	}
	return out, rows.Err()
}

// ListActiveBlockedCardIDs 活跃坏卡 ID 列表。
func ListActiveBlockedCardIDs() ([]int64, error) {
	if DB == nil {
		return nil, nil
	}
	rows, err := DB.Query(`SELECT card_id FROM card_blocklist WHERE unblocked_at IS NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// CardHealthPolicy 本站卡健康策略（site_settings）。
type CardHealthPolicy struct {
	Enabled        bool   `json:"enabled"`
	FailThreshold  int    `json:"fail_threshold"`  // 默认 2
	FreezeOnBlock  bool   `json:"freeze_on_block"` // 判定坏卡后调卡台冻结
	// RequireKnownEmail：失败记录邮箱均为 unknown 时不拉黑（避免无邮箱误伤）
	RequireKnownEmail bool `json:"require_known_email"`
}

func DefaultCardHealthPolicy() CardHealthPolicy {
	return CardHealthPolicy{
		Enabled:       true,
		FailThreshold: 2,
		// 不再冻结卡台侧的卡：拉黑仅记在本站，兑换时经 exclude_card_ids 让 CDK 不选它，
		// 卡台状态不变、直充用户依旧可用。FreezeOnBlock 保留字段兼容但已不触发真冻结。
		FreezeOnBlock:     false,
		RequireKnownEmail: true,
	}
}

