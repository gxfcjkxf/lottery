package commissionreview

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
)

const sourceNamespace = "commission-history-source/v1\x00"

// Snapshot combines the complete review with a digest of its saved financial
// evidence. The digest is evidence identity only; it is not an approval token.
type Snapshot struct {
	Report       Report `json:"report"`
	SourceDigest string `json:"source_digest"`
}

// SnapshotTx reads the source manifest in one SQL statement after the full
// inspection. It owns no transaction boundary; callers should hold any cycle
// and finance-row locks needed for the proposal or review being audited.
func SnapshotTx(ctx context.Context, tx pgx.Tx, brandID, cycleID string) (Snapshot, error) {
	if ctx == nil || tx == nil || !canonicalUUID.MatchString(brandID) || !canonicalUUID.MatchString(cycleID) {
		return Snapshot{}, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	report, err := InspectTx(ctx, tx, brandID, cycleID)
	if err != nil {
		return Snapshot{}, err
	}
	h, err := newSourceHasher(report)
	if err != nil {
		return Snapshot{}, err
	}
	rows, err := tx.Query(ctx, sourceRowsSQL, brandID, cycleID)
	if err != nil {
		return Snapshot{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var family string
		var raw string
		if err := rows.Scan(&family, &raw); err != nil {
			return Snapshot{}, err
		}
		if err := writeCanonicalSourceRow(h, family, []byte(raw)); err != nil {
			return Snapshot{}, err
		}
	}
	if err := rows.Err(); err != nil {
		return Snapshot{}, err
	}
	return Snapshot{Report: report, SourceDigest: hex.EncodeToString(h.Sum(nil))}, nil
}

type sourceRow struct {
	section string
	value   json.RawMessage
}

// Keep this as a fixed query against the schema captured by InspectTx's local
// search_path. The JSON is used only as hash input and is never returned or
// persisted by SnapshotTx.
const sourceRowsSQL = `
WITH cycle_scope AS (
 SELECT c.* FROM commission_cycles c WHERE c.brand_id=$1 AND c.id=$2
), order_scope AS (
 SELECT DISTINCT t.order_id FROM commission_cycle_targets t
 WHERE t.brand_id=$1 AND t.cycle_id=$2
), payment_scope AS (
 SELECT p.id FROM commission_payments p WHERE p.brand_id=$1 AND p.cycle_id=$2
), payment_target_scope AS (
 SELECT t.id FROM commission_payment_targets t JOIN payment_scope p ON p.id=t.payment_id WHERE t.brand_id=$1
), adjustment_scope AS (
 SELECT a.id,a.target_id FROM commission_adjustments a JOIN payment_scope p ON p.id=a.payment_id WHERE a.brand_id=$1
), plan_scope AS (
 SELECT p.id FROM commission_correction_plans p WHERE p.brand_id=$1 AND p.cycle_id=$2
), execution_scope AS (
 SELECT x.id FROM commission_correction_executions x WHERE x.brand_id=$1 AND x.cycle_id=$2
), execution_target_scope AS (
 SELECT t.id FROM commission_correction_execution_targets t WHERE t.brand_id=$1 AND t.cycle_id=$2
), ledger_scope AS (
 SELECT l.id FROM point_ledger_entries l WHERE l.brand_id=$1 AND (
  l.id IN (SELECT o.debit_entry_id FROM bet_orders o JOIN order_scope s ON s.order_id=o.id WHERE o.brand_id=$1) OR
  l.id IN (SELECT o.refund_entry_id FROM bet_orders o JOIN order_scope s ON s.order_id=o.id WHERE o.brand_id=$1) OR
  l.id IN (SELECT o.payout_entry_id FROM bet_orders o JOIN order_scope s ON s.order_id=o.id WHERE o.brand_id=$1) OR
  (l.reference_type='settlement_calculation' AND l.reference_id IN (SELECT c.id FROM settlement_calculations c JOIN order_scope s ON s.order_id=c.order_id WHERE c.brand_id=$1)) OR
  (l.reference_type='draw_correction' AND l.reference_id IN (SELECT t.correction_id FROM draw_correction_targets t JOIN order_scope s ON s.order_id=t.order_id WHERE t.brand_id=$1)) OR
  l.id IN (SELECT ledger_entry_id FROM commission_payment_targets WHERE brand_id=$1 AND payment_id IN (SELECT id FROM payment_scope) AND ledger_entry_id IS NOT NULL) OR
  l.id IN (SELECT ledger_entry_id FROM commission_adjustments WHERE brand_id=$1 AND payment_id IN (SELECT id FROM payment_scope) AND ledger_entry_id IS NOT NULL) OR
  l.id IN (SELECT ledger_entry_id FROM commission_correction_execution_targets WHERE brand_id=$1 AND cycle_id=$2 AND ledger_entry_id IS NOT NULL) OR
  (l.reference_type='commission_payment_target' AND l.reference_id IN (SELECT id FROM payment_target_scope)) OR
  (l.reference_type='commission_adjustment' AND l.reference_id IN (SELECT id FROM adjustment_scope)) OR
  (l.reference_type='commission_correction_target' AND l.reference_id IN (SELECT id FROM execution_target_scope)) OR
  (l.reference_type='commission_correction_plan_target' AND l.reference_id IN (
   SELECT t.id FROM commission_correction_plan_targets t JOIN plan_scope p ON p.id=t.plan_id))
 )
), related_ids AS (
 SELECT id FROM cycle_scope UNION SELECT order_id FROM order_scope UNION
 SELECT id FROM payment_scope UNION SELECT id FROM payment_target_scope UNION
 SELECT id FROM adjustment_scope UNION SELECT id FROM plan_scope UNION
 SELECT id FROM execution_scope UNION SELECT id FROM execution_target_scope UNION
 SELECT id FROM ledger_scope UNION
 SELECT c.id FROM settlement_calculations c JOIN order_scope s ON s.order_id=c.order_id WHERE c.brand_id=$1 UNION
 SELECT id FROM commission_runs WHERE brand_id=$1 AND cycle_id=$2 UNION
 SELECT id FROM commission_calculations WHERE brand_id=$1 AND cycle_id=$2 UNION
 SELECT id FROM commission_earnings WHERE brand_id=$1 AND cycle_id=$2 UNION
 SELECT id FROM commission_cycle_steps WHERE brand_id=$1 AND cycle_id=$2 UNION
 SELECT id FROM commission_correction_plan_targets WHERE brand_id=$1 AND plan_id IN (SELECT id FROM plan_scope) UNION
 SELECT id FROM commission_correction_plan_steps WHERE brand_id=$1 AND plan_id IN (SELECT id FROM plan_scope) UNION
 SELECT id FROM commission_correction_execution_steps WHERE brand_id=$1 AND execution_id IN (SELECT id FROM execution_scope)
), bet_revisions AS (
 SELECT DISTINCT (o.commission_rule_snapshot->'financial_policy'->>'revision_id')::uuid AS id
 FROM bet_orders o JOIN order_scope s ON s.order_id=o.id
 WHERE o.brand_id=$1 AND o.commission_rule_snapshot->'financial_policy'->>'revision_id' IS NOT NULL
), agent_revisions AS (
 SELECT DISTINCT (path_item->>'revision_id')::uuid AS id
 FROM bet_orders o JOIN order_scope s ON s.order_id=o.id
 CROSS JOIN LATERAL jsonb_array_elements(o.commission_rule_snapshot->'agent_path') path_item
 WHERE o.brand_id=$1 AND path_item->>'revision_id' IS NOT NULL
 UNION SELECT DISTINCT (o.commission_rule_snapshot->'agency_policy'->>'revision_id')::uuid
 FROM bet_orders o JOIN order_scope s ON s.order_id=o.id
 WHERE o.brand_id=$1 AND o.commission_rule_snapshot->'agency_policy'->>'revision_id' IS NOT NULL
)
SELECT section, row_json::text FROM (
 SELECT 'cycle'::text AS section,to_jsonb(c) AS row_json FROM cycle_scope c
 UNION ALL SELECT 'cycle_target',to_jsonb(t) FROM commission_cycle_targets t WHERE t.brand_id=$1 AND t.cycle_id=$2
 UNION ALL SELECT 'bet_order',to_jsonb(o) FROM bet_orders o JOIN order_scope s ON s.order_id=o.id WHERE o.brand_id=$1
 UNION ALL SELECT 'settlement_calculation',to_jsonb(c) FROM settlement_calculations c JOIN order_scope s ON s.order_id=c.order_id WHERE c.brand_id=$1
 UNION ALL SELECT 'commission_policy_revision',to_jsonb(r) FROM commission_policy_revisions r WHERE r.brand_id=$1 AND r.id IN (SELECT id FROM bet_revisions)
 UNION ALL SELECT 'agent_config_revision',to_jsonb(r) FROM agent_config_revisions r WHERE r.brand_id=$1 AND r.id IN (SELECT id FROM agent_revisions)
 UNION ALL SELECT 'run',to_jsonb(r) FROM commission_runs r WHERE r.brand_id=$1 AND r.cycle_id=$2
 UNION ALL SELECT 'cycle_step',to_jsonb(s) FROM commission_cycle_steps s WHERE s.brand_id=$1 AND s.cycle_id=$2
 UNION ALL SELECT 'calculation',to_jsonb(x) FROM commission_calculations x WHERE x.brand_id=$1 AND x.cycle_id=$2
 UNION ALL SELECT 'allocation',to_jsonb(a) FROM commission_allocations a WHERE a.brand_id=$1 AND a.cycle_id=$2
 UNION ALL SELECT 'earning',to_jsonb(e) FROM commission_earnings e WHERE e.brand_id=$1 AND e.cycle_id=$2
 UNION ALL SELECT 'payment',to_jsonb(p) FROM commission_payments p WHERE p.brand_id=$1 AND p.cycle_id=$2
 UNION ALL SELECT 'payment_target',to_jsonb(t) FROM commission_payment_targets t JOIN payment_scope p ON p.id=t.payment_id WHERE t.brand_id=$1
 UNION ALL SELECT 'adjustment',to_jsonb(a) FROM commission_adjustments a JOIN payment_scope p ON p.id=a.payment_id WHERE a.brand_id=$1
 UNION ALL SELECT 'adjustment_head',to_jsonb(h) FROM commission_adjustment_heads h WHERE h.brand_id=$1 AND h.target_id IN (SELECT target_id FROM adjustment_scope UNION SELECT id FROM payment_target_scope)
 UNION ALL SELECT 'correction_plan',to_jsonb(p) FROM commission_correction_plans p WHERE p.brand_id=$1 AND p.cycle_id=$2
 UNION ALL SELECT 'correction_plan_target',to_jsonb(t) FROM commission_correction_plan_targets t JOIN plan_scope p ON p.id=t.plan_id WHERE t.brand_id=$1
 UNION ALL SELECT 'correction_plan_step',to_jsonb(s) FROM commission_correction_plan_steps s JOIN plan_scope p ON p.id=s.plan_id WHERE s.brand_id=$1
 UNION ALL SELECT 'correction_execution',to_jsonb(x) FROM commission_correction_executions x WHERE x.brand_id=$1 AND x.cycle_id=$2
 UNION ALL SELECT 'correction_execution_target',to_jsonb(t) FROM commission_correction_execution_targets t WHERE t.brand_id=$1 AND t.cycle_id=$2
 UNION ALL SELECT 'correction_balance_head',to_jsonb(h) FROM commission_correction_balance_heads h WHERE h.brand_id=$1 AND h.cycle_id=$2
 UNION ALL SELECT 'correction_cycle_hold',to_jsonb(h) FROM commission_correction_cycle_holds h WHERE h.brand_id=$1 AND h.cycle_id=$2
 UNION ALL SELECT 'correction_execution_step',to_jsonb(s) FROM commission_correction_execution_steps s JOIN execution_scope x ON x.id=s.execution_id WHERE s.brand_id=$1
 UNION ALL SELECT 'point_ledger_entry',to_jsonb(l) FROM point_ledger_entries l WHERE l.brand_id=$1 AND l.id IN (SELECT id FROM ledger_scope)
 UNION ALL SELECT 'financial_audit',to_jsonb(a) FROM audit_logs a
  WHERE a.brand_id=$1 AND NOT a.action LIKE 'commission.history_recovery.%' AND (
   (a.actor_type='system' AND a.resource_id IN (SELECT id FROM related_ids)) OR a.after_json->>'ledger_entry_id' IN (SELECT id::text FROM ledger_scope)
   OR a.id IN (SELECT creation_audit_log_id FROM cycle_scope)
   OR a.id IN (SELECT audit_log_id FROM commission_cycle_steps WHERE brand_id=$1 AND cycle_id=$2)
   OR a.id IN (SELECT audit_log_id FROM commission_calculations WHERE brand_id=$1 AND cycle_id=$2)
   OR a.id IN (SELECT approval_audit_log_id FROM commission_payments WHERE brand_id=$1 AND cycle_id=$2)
   OR a.id IN (SELECT approval_audit_log_id FROM commission_correction_executions WHERE brand_id=$1 AND cycle_id=$2)
   OR a.id IN (SELECT creation_audit_log_id FROM commission_payments WHERE brand_id=$1 AND cycle_id=$2)
   OR a.id IN (SELECT last_audit_log_id FROM commission_payments WHERE brand_id=$1 AND cycle_id=$2)
   OR a.id IN (SELECT audit_log_id FROM commission_payment_targets WHERE brand_id=$1 AND payment_id IN (SELECT id FROM payment_scope))
   OR a.id IN (SELECT audit_log_id FROM commission_adjustments WHERE brand_id=$1 AND payment_id IN (SELECT id FROM payment_scope))
   OR a.id IN (SELECT creation_audit_log_id FROM commission_correction_plans WHERE brand_id=$1 AND cycle_id=$2)
   OR a.id IN (SELECT last_audit_log_id FROM commission_correction_plans WHERE brand_id=$1 AND cycle_id=$2)
   OR a.id IN (SELECT audit_log_id FROM commission_correction_plan_steps WHERE brand_id=$1 AND plan_id IN (SELECT id FROM plan_scope))
   OR a.id IN (SELECT audit_log_id FROM commission_correction_execution_targets WHERE brand_id=$1 AND cycle_id=$2)
   OR a.id IN (SELECT audit_log_id FROM commission_correction_execution_steps WHERE brand_id=$1 AND execution_id IN (SELECT id FROM execution_scope))
   OR a.id IN (SELECT audit_log_id FROM commission_correction_cycle_holds WHERE brand_id=$1 AND cycle_id=$2)
   OR a.id IN (SELECT audit_log_id FROM commission_policy_revisions WHERE brand_id=$1 AND id IN (SELECT id FROM bet_revisions))
   OR a.id IN (SELECT audit_log_id FROM agent_config_revisions WHERE brand_id=$1 AND id IN (SELECT id FROM agent_revisions))
  )
) rows
ORDER BY section COLLATE "C",row_json::text COLLATE "C"`

func sourceDigest(report Report, rows []sourceRow) (string, error) {
	h, err := newSourceHasher(report)
	if err != nil {
		return "", err
	}
	type canonicalRow struct {
		section string
		value   []byte
	}
	canonicalRows := make([]canonicalRow, 0, len(rows))
	for _, row := range rows {
		value, err := canonicalJSON(row.value)
		if err != nil {
			return "", fmt.Errorf("canonicalize %s source row: %w", row.section, err)
		}
		canonicalRows = append(canonicalRows, canonicalRow{section: row.section, value: value})
	}
	sort.Slice(canonicalRows, func(i, j int) bool {
		if canonicalRows[i].section != canonicalRows[j].section {
			return canonicalRows[i].section < canonicalRows[j].section
		}
		return bytes.Compare(canonicalRows[i].value, canonicalRows[j].value) < 0
	})
	for _, row := range canonicalRows {
		writeDigestPart(h, []byte(row.section))
		writeDigestPart(h, row.value)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func newSourceHasher(report Report) (hash.Hash, error) {
	reportJSON, err := json.Marshal(report)
	if err != nil {
		return nil, err
	}
	normalizedReport, err := canonicalObjectWithout(reportJSON, "snapshot_at")
	if err != nil {
		return nil, err
	}
	h := sha256.New()
	h.Write([]byte(sourceNamespace))
	writeDigestPart(h, normalizedReport)
	return h, nil
}

func writeCanonicalSourceRow(h hash.Hash, family string, raw []byte) error {
	canonical, err := canonicalJSON(raw)
	if err != nil {
		return fmt.Errorf("canonicalize %s source row: %w", family, err)
	}
	writeDigestPart(h, []byte(family))
	writeDigestPart(h, canonical)
	return nil
}

type digestWriter interface{ Write([]byte) (int, error) }

func writeDigestPart(w digestWriter, value []byte) {
	var size [8]byte
	n := uint64(len(value))
	for i := len(size) - 1; i >= 0; i-- {
		size[i] = byte(n)
		n >>= 8
	}
	_, _ = w.Write(size[:])
	_, _ = w.Write(value)
}

func canonicalObjectWithout(raw []byte, omitted string) ([]byte, error) {
	value, err := decodeCanonicalValue(raw)
	if err != nil {
		return nil, err
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("expected JSON object")
	}
	delete(object, omitted)
	return json.Marshal(object)
}

func canonicalJSON(raw []byte) ([]byte, error) {
	value, err := decodeCanonicalValue(raw)
	if err != nil {
		return nil, err
	}
	return json.Marshal(value)
}

func decodeCanonicalValue(raw []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	value, err := decodeCanonicalToken(decoder)
	if err != nil {
		return nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err == nil {
		return nil, errors.New("multiple JSON values")
	} else if !errors.Is(err, io.EOF) {
		return nil, err
	}
	return value, nil
}

func decodeCanonicalToken(decoder *json.Decoder) (any, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return token, nil
	}
	switch delim {
	case '{':
		object := make(map[string]any)
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return nil, err
			}
			key, ok := keyToken.(string)
			if !ok {
				return nil, errors.New("JSON object key is not a string")
			}
			if _, exists := object[key]; exists {
				return nil, fmt.Errorf("duplicate JSON object key %q", key)
			}
			value, err := decodeCanonicalToken(decoder)
			if err != nil {
				return nil, err
			}
			object[key] = value
		}
		if _, err := decoder.Token(); err != nil {
			return nil, err
		}
		return object, nil
	case '[':
		array := make([]any, 0)
		for decoder.More() {
			value, err := decodeCanonicalToken(decoder)
			if err != nil {
				return nil, err
			}
			array = append(array, value)
		}
		if _, err := decoder.Token(); err != nil {
			return nil, err
		}
		return array, nil
	default:
		return nil, fmt.Errorf("unexpected JSON delimiter %q", delim)
	}
}
