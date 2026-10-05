package points

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var ErrPolicyLimit = errors.New("point policy limit exceeded")

type Policy struct {
	BrandID             string  `json:"brand_id"`
	Version             int64   `json:"version"`
	MaxBalancePoints    *Amount `json:"max_balance_points"`
	MaxRechargePoints   *Amount `json:"max_recharge_points"`
	MaxAdjustmentPoints *Amount `json:"max_adjustment_points"`
	AuditLogID          string  `json:"audit_log_id,omitempty"`
}

type PolicyInput struct {
	Version             int64   `json:"version"`
	MaxBalancePoints    *Amount `json:"max_balance_points"`
	MaxRechargePoints   *Amount `json:"max_recharge_points"`
	MaxAdjustmentPoints *Amount `json:"max_adjustment_points"`
	Reason              string  `json:"reason"`
}

// UnmarshalJSON treats policy updates as complete replacements. Explicit null
// is required to clear a cap; omitted or repeated fields are ambiguous and
// therefore rejected.
func (in *PolicyInput) UnmarshalJSON(data []byte) error {
	if in == nil {
		return ErrInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return fmt.Errorf("%w: policy input must be an object", ErrInvalid)
	}
	var parsed PolicyInput
	var seen [5]bool
	for decoder.More() {
		token, err = decoder.Token()
		if err != nil {
			return fmt.Errorf("%w: malformed policy field", ErrInvalid)
		}
		name, ok := token.(string)
		if !ok {
			return fmt.Errorf("%w: malformed policy field name", ErrInvalid)
		}
		index := -1
		switch name {
		case "version":
			index = 0
		case "max_balance_points":
			index = 1
		case "max_recharge_points":
			index = 2
		case "max_adjustment_points":
			index = 3
		case "reason":
			index = 4
		default:
			return fmt.Errorf("%w: unknown policy field", ErrInvalid)
		}
		if seen[index] {
			return fmt.Errorf("%w: duplicate policy field", ErrInvalid)
		}
		seen[index] = true
		var raw json.RawMessage
		if err = decoder.Decode(&raw); err != nil {
			return fmt.Errorf("%w: malformed policy field value", ErrInvalid)
		}
		switch index {
		case 0:
			if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) || json.Unmarshal(raw, &parsed.Version) != nil {
				return fmt.Errorf("%w: version must be an integer", ErrInvalid)
			}
		case 1, 2, 3:
			if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
				break
			}
			var amount Amount
			if err = json.Unmarshal(raw, &amount); err != nil {
				return err
			}
			value := amount
			switch index {
			case 1:
				parsed.MaxBalancePoints = &value
			case 2:
				parsed.MaxRechargePoints = &value
			case 3:
				parsed.MaxAdjustmentPoints = &value
			}
		case 4:
			trimmed := bytes.TrimSpace(raw)
			if len(trimmed) < 2 || trimmed[0] != '"' || trimmed[len(trimmed)-1] != '"' || json.Unmarshal(trimmed, &parsed.Reason) != nil {
				return fmt.Errorf("%w: reason must be a string", ErrInvalid)
			}
		}
	}
	if _, err = decoder.Token(); err != nil {
		return fmt.Errorf("%w: malformed policy object", ErrInvalid)
	}
	for _, present := range seen {
		if !present {
			return fmt.Errorf("%w: policy replacement is missing a field", ErrInvalid)
		}
	}
	var trailing any
	if err = decoder.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("%w: trailing policy JSON", ErrInvalid)
	}
	*in = parsed
	return nil
}

type policyRow interface {
	Scan(dest ...any) error
}

func scanPolicy(row policyRow) (Policy, error) {
	var policy Policy
	var balance, recharge, adjustment pgtype.Int8
	err := row.Scan(&policy.BrandID, &policy.Version, &balance, &recharge, &adjustment)
	if errors.Is(err, pgx.ErrNoRows) {
		return Policy{}, ErrNotFound
	}
	if err != nil {
		return Policy{}, err
	}
	if balance.Valid {
		value := Amount(balance.Int64)
		policy.MaxBalancePoints = &value
	}
	if recharge.Valid {
		value := Amount(recharge.Int64)
		policy.MaxRechargePoints = &value
	}
	if adjustment.Valid {
		value := Amount(adjustment.Int64)
		policy.MaxAdjustmentPoints = &value
	}
	return policy, nil
}

const policySelect = `SELECT brand_id::text,version,max_balance_points,max_recharge_points,max_adjustment_points FROM brand_point_policies`

func (s Store) ReadPolicy(ctx context.Context, brand string) (Policy, error) {
	if !uuidPattern.MatchString(brand) {
		return Policy{}, ErrInvalid
	}
	return scanPolicy(s.DB.QueryRow(ctx, policySelect+` WHERE brand_id=$1`, brand))
}

func (s Store) LockedPolicy(ctx context.Context, tx pgx.Tx, brand string) (Policy, error) {
	if tx == nil || !uuidPattern.MatchString(brand) {
		return Policy{}, ErrInvalid
	}
	return scanPolicy(tx.QueryRow(ctx, policySelect+` WHERE brand_id=$1 FOR SHARE`, brand))
}

func validPolicyCap(value *Amount) bool {
	return value == nil || *value > 0
}

func validActorUUID(value string) bool {
	return uuidPattern.MatchString(value)
}

func policyCapArg(value *Amount) any {
	if value == nil {
		return nil
	}
	return int64(*value)
}

func (s Store) UpdatePolicy(ctx context.Context, tx pgx.Tx, brand string, in PolicyInput, meta Metadata) (Policy, error) {
	reason := strings.TrimSpace(in.Reason)
	if tx == nil || !uuidPattern.MatchString(brand) || in.Version <= 0 ||
		!validPolicyCap(in.MaxBalancePoints) || !validPolicyCap(in.MaxRechargePoints) || !validPolicyCap(in.MaxAdjustmentPoints) ||
		reason == "" || len([]byte(reason)) > 500 || !utf8.ValidString(reason) ||
		meta.ActorType != "admin" || !validActorUUID(meta.ActorID) || meta.RequestID == "" || len([]byte(meta.RequestID)) > 80 || !utf8.ValidString(meta.RequestID) {
		return Policy{}, ErrInvalid
	}
	before, err := scanPolicy(tx.QueryRow(ctx, policySelect+` WHERE brand_id=$1 FOR UPDATE`, brand))
	if err != nil {
		return Policy{}, err
	}
	if before.Version != in.Version {
		return Policy{}, ErrConflict
	}
	if before.Version == math.MaxInt64 {
		return Policy{}, ErrOverflow
	}
	var updated Policy
	updated, err = scanPolicy(tx.QueryRow(ctx, `UPDATE brand_point_policies
		SET version=version+1,max_balance_points=$3,max_recharge_points=$4,max_adjustment_points=$5
		WHERE brand_id=$1 AND version=$2
		RETURNING brand_id::text,version,max_balance_points,max_recharge_points,max_adjustment_points`,
		brand, in.Version, policyCapArg(in.MaxBalancePoints), policyCapArg(in.MaxRechargePoints), policyCapArg(in.MaxAdjustmentPoints)))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Policy{}, ErrConflict
		}
		return Policy{}, err
	}
	auditID, err := audit.Append(ctx, tx, audit.Record{
		BrandID: brand, ActorType: "admin", ActorID: meta.ActorID, Action: "points.policy.update",
		ResourceType: "brand_point_policy", ResourceID: brand, Reason: reason,
		RequestID: meta.RequestID, IP: meta.IP, Before: before, After: updated,
	})
	if err != nil {
		return Policy{}, err
	}
	updated.AuditLogID = auditID
	return updated, nil
}

func policyBalanceTotal(balance Balance) (Amount, error) {
	if err := balance.Validate(); err != nil {
		return 0, err
	}
	var total Amount
	for _, source := range balance {
		for _, amount := range source {
			next, err := checkedAdd(total, amount)
			if err != nil {
				return 0, err
			}
			total = next
		}
	}
	return total, nil
}

func (p Policy) CheckBalance(before, after Balance, reversal bool) error {
	beforeTotal, err := policyBalanceTotal(before)
	if err != nil {
		return err
	}
	afterTotal, err := policyBalanceTotal(after)
	if err != nil {
		return err
	}
	if reversal || p.MaxBalancePoints == nil || afterTotal <= beforeTotal {
		return nil
	}
	if *p.MaxBalancePoints <= 0 {
		return ErrInvalid
	}
	if afterTotal > *p.MaxBalancePoints {
		return ErrPolicyLimit
	}
	return nil
}

func (p Policy) CheckRecharge(amount Amount) error {
	if amount <= 0 {
		return ErrInvalid
	}
	if p.MaxRechargePoints == nil {
		return nil
	}
	if *p.MaxRechargePoints <= 0 {
		return ErrInvalid
	}
	if amount > *p.MaxRechargePoints {
		return ErrPolicyLimit
	}
	return nil
}

func (p Policy) CheckAdjustment(delta Balance) error {
	var total Amount
	nonzero := false
	for _, source := range delta {
		for _, amount := range source {
			if amount == 0 {
				continue
			}
			nonzero = true
			if amount == Amount(math.MinInt64) {
				return ErrOverflow
			}
			if amount < 0 {
				amount = -amount
			}
			var err error
			total, err = checkedAdd(total, amount)
			if err != nil {
				return err
			}
		}
	}
	if !nonzero {
		return fmt.Errorf("%w: empty adjustment", ErrInvalid)
	}
	if p.MaxAdjustmentPoints == nil {
		return nil
	}
	if *p.MaxAdjustmentPoints <= 0 {
		return ErrInvalid
	}
	if total > *p.MaxAdjustmentPoints {
		return ErrPolicyLimit
	}
	return nil
}
