package notification

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strconv"

	"github.com/jackc/pgx/v5"
)

func exactCommissionJSONKeys(raw []byte, expected ...string) bool {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	token, err := decoder.Token()
	delim, ok := token.(json.Delim)
	if err != nil || !ok || delim != '{' {
		return false
	}
	seen := make(map[string]bool, len(expected))
	for decoder.More() {
		token, err = decoder.Token()
		key, isString := token.(string)
		if err != nil || !isString || seen[key] {
			return false
		}
		seen[key] = true
		var value json.RawMessage
		if decoder.Decode(&value) != nil {
			return false
		}
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return false
		}
	}
	token, err = decoder.Token()
	close, ok := token.(json.Delim)
	if err != nil || !ok || close != '}' || len(seen) != len(expected) {
		return false
	}
	for _, key := range expected {
		if !seen[key] {
			return false
		}
	}
	var trailing any
	return decoder.Decode(&trailing) == io.EOF
}

// Commission events carry private witnesses in the outbox. Only the public
// resource reference and signed/positive point amount are materialized.
func validateCommissionEvent(ctx context.Context, tx pgx.Tx, brand, kind, aggregate string, raw []byte) (string, Payload, error) {
	if !exactCommissionJSONKeys(raw, "member_id", "resource_id", "points", "ledger_entry_id", "target_id") {
		return "", Payload{}, ErrInvalid
	}
	var in struct {
		MemberID      string `json:"member_id"`
		ResourceID    string `json:"resource_id"`
		Points        string `json:"points"`
		LedgerEntryID string `json:"ledger_entry_id"`
		TargetID      string `json:"target_id"`
	}
	if json.Unmarshal(raw, &in) != nil || !uuid.MatchString(brand) || !uuid.MatchString(aggregate) ||
		!uuid.MatchString(in.MemberID) || !uuid.MatchString(in.ResourceID) || !uuid.MatchString(in.LedgerEntryID) ||
		!uuid.MatchString(in.TargetID) {
		return "", Payload{}, ErrInvalid
	}
	amount, err := strconv.ParseInt(in.Points, 10, 64)
	if err != nil || amount == 0 || strconv.FormatInt(amount, 10) != in.Points {
		return "", Payload{}, ErrInvalid
	}
	if kind == "commission.paid" && (amount <= 0 || in.ResourceID != aggregate || in.TargetID != aggregate) {
		return "", Payload{}, ErrInvalid
	}
	var valid bool
	switch kind {
	case "commission.corrected":
		if in.ResourceID != aggregate || in.TargetID != aggregate {
			return "", Payload{}, ErrInvalid
		}
		err = tx.QueryRow(ctx, `SELECT valid_commission_correction_notification_event($1,$2,$3,$4::jsonb)`, brand, kind, aggregate, raw).Scan(&valid)
	case "commission.paid":
		err = tx.QueryRow(ctx, `SELECT EXISTS(
 SELECT 1 FROM commission_payment_targets t
 JOIN point_ledger_entries l ON l.id=t.ledger_entry_id AND l.brand_id=t.brand_id
 WHERE t.brand_id=$1 AND t.id=$2 AND t.state='paid' AND t.points=$5::bigint
 AND t.member_id=$3 AND t.ledger_entry_id=$4
 AND l.member_id=t.member_id AND l.entry_type='commission' AND l.reference_type='commission_payment_target'
 AND l.reference_id=t.id AND l.operation_key='commission-payment:'||t.id::text AND l.actor_type='system' AND l.actor_id IS NULL
 AND l.delta_snapshot=jsonb_set(point_zero_snapshot(),'{commission,available}',to_jsonb(t.points::text))
 AND l.source_allocation=jsonb_build_array(jsonb_build_object('source','commission','state','available','points',t.points::text))
 )`, brand, aggregate, in.MemberID, in.LedgerEntryID, in.Points).Scan(&valid)
	case "commission.adjusted":
		if in.ResourceID != aggregate {
			return "", Payload{}, ErrInvalid
		}
		err = tx.QueryRow(ctx, `SELECT EXISTS(
 SELECT 1 FROM commission_adjustments a
 JOIN commission_payment_targets t ON t.brand_id=a.brand_id AND t.id=a.target_id
 JOIN point_ledger_entries l ON l.id=a.ledger_entry_id AND l.brand_id=a.brand_id
 WHERE a.brand_id=$1 AND a.id=$2 AND a.delta_points=$5::bigint AND a.delta_points<>0 AND a.target_id=$6
 AND t.member_id=$3 AND a.ledger_entry_id=$4
 AND l.member_id=t.member_id AND l.entry_type='commission_adjustment' AND l.reference_type='commission_adjustment'
 AND l.reference_id=a.id AND l.operation_key='commission-adjustment:'||a.id::text AND l.actor_type='admin' AND l.actor_id=a.created_by
 AND l.delta_snapshot=jsonb_set(point_zero_snapshot(),'{commission,available}',to_jsonb(a.delta_points::text))
 AND l.source_allocation=jsonb_build_array(jsonb_build_object('source','commission','state','available','points',abs(a.delta_points::numeric)::text))
			)`, brand, aggregate, in.MemberID, in.LedgerEntryID, in.Points, in.TargetID).Scan(&valid)
	default:
		return "", Payload{}, ErrInvalid
	}
	if err != nil {
		return "", Payload{}, err
	}
	if !valid {
		return "", Payload{}, ErrInvalid
	}
	return in.MemberID, Payload{ResourceID: in.ResourceID, Points: &in.Points}, nil
}
