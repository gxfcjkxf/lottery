package notification

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/jackc/pgx/v5"
)

func validateWithdrawalEvent(ctx context.Context, tx pgx.Tx, brand, kind, aggregate string, raw []byte) (string, Payload, error) {
	var in struct {
		MemberID   string  `json:"member_id"`
		ResourceID string  `json:"resource_id"`
		Points     *string `json:"points"`
		Status     string  `json:"status"`
		Version    int64   `json:"version"`
		AuditLogID string  `json:"audit_log_id"`
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || len(fields) != 6 || json.Unmarshal(raw, &in) != nil || !uuid.MatchString(brand) || !uuid.MatchString(aggregate) || !uuid.MatchString(in.MemberID) || !uuid.MatchString(in.AuditLogID) || in.ResourceID != aggregate || !positive(in.Points) || in.Version < 1 || in.Version > 3 || kind != "withdrawal.order."+in.Status || !ValidTemplateKey(kind) {
		return "", Payload{}, ErrInvalid
	}
	for _, key := range []string{"member_id", "resource_id", "points", "status", "version", "audit_log_id"} {
		if _, ok := fields[key]; !ok {
			return "", Payload{}, ErrInvalid
		}
	}
	var valid bool
	// Match the immutable historic transition, not the current order state: the
	// delivery worker may observe reviewing only after the order is already paid.
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM withdrawal_orders o JOIN withdrawal_order_transitions t ON t.brand_id=o.brand_id AND t.order_id=o.id WHERE o.brand_id=$1 AND o.id=$2 AND o.member_id=$3 AND o.points=$4::text::bigint AND t.version=$5 AND t.to_state=$6 AND t.audit_log_id=$7)`, brand, aggregate, in.MemberID, *in.Points, in.Version, strings.TrimPrefix(kind, "withdrawal.order."), in.AuditLogID).Scan(&valid)
	if err != nil {
		return "", Payload{}, err
	}
	if !valid {
		return "", Payload{}, ErrInvalid
	}
	return in.MemberID, Payload{ResourceID: aggregate, Points: in.Points}, nil
}
