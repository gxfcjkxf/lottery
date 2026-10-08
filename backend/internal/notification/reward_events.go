package notification

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strconv"

	"github.com/jackc/pgx/v5"
)

const maxRewardActionVersion int64 = 9007199254740991

type rewardEvent struct {
	MemberID   string          `json:"member_id"`
	ResourceID string          `json:"resource_id"`
	Points     string          `json:"points"`
	ActionID   string          `json:"action_id"`
	Version    json.RawMessage `json:"version"`
	AuditLogID string          `json:"audit_log_id"`
}

func exactRewardJSONKeys(raw []byte) bool {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	token, err := decoder.Token()
	delim, ok := token.(json.Delim)
	if err != nil || !ok || delim != '{' {
		return false
	}
	expected := map[string]bool{
		"member_id": false, "resource_id": false, "points": false,
		"action_id": false, "version": false, "audit_log_id": false,
	}
	for decoder.More() {
		token, err = decoder.Token()
		key, isString := token.(string)
		if err != nil || !isString {
			return false
		}
		if _, exists := expected[key]; !exists || expected[key] {
			return false
		}
		expected[key] = true
		var value json.RawMessage
		if decoder.Decode(&value) != nil || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return false
		}
	}
	token, err = decoder.Token()
	close, ok := token.(json.Delim)
	if err != nil || !ok || close != '}' {
		return false
	}
	for _, present := range expected {
		if !present {
			return false
		}
	}
	var trailing any
	return decoder.Decode(&trailing) == io.EOF
}

func parseRewardEvent(raw []byte) (rewardEvent, int64, error) {
	var in rewardEvent
	if !exactRewardJSONKeys(raw) || json.Unmarshal(raw, &in) != nil ||
		!uuid.MatchString(in.MemberID) || !uuid.MatchString(in.ResourceID) ||
		!uuid.MatchString(in.ActionID) || !uuid.MatchString(in.AuditLogID) || !positive(&in.Points) {
		return rewardEvent{}, 0, ErrInvalid
	}
	versionText := string(in.Version)
	version, err := strconv.ParseInt(versionText, 10, 64)
	if err != nil || strconv.FormatInt(version, 10) != versionText || version < 1 || version > maxRewardActionVersion {
		return rewardEvent{}, 0, ErrInvalid
	}
	return in, version, nil
}

func validateRewardEvent(ctx context.Context, tx pgx.Tx, brand, kind, aggregate string, raw []byte) (string, Payload, error) {
	var state string
	switch kind {
	case "reward.order.granted":
		state = "granted"
	case "reward.order.revocation_pending":
		state = "revocation_pending"
	case "reward.order.revoked":
		state = "revoked"
	default:
		return "", Payload{}, ErrInvalid
	}
	in, version, err := parseRewardEvent(raw)
	if err != nil {
		return "", Payload{}, err
	}
	if !uuid.MatchString(brand) || !uuid.MatchString(aggregate) || in.ActionID != aggregate {
		return "", Payload{}, ErrInvalid
	}
	var valid bool
	// The action row is the immutable event witness. In particular, this query
	// deliberately does not compare the historic state with the order's current
	// state: a pending action remains a valid notification after a later retry.
	err = tx.QueryRow(ctx, `SELECT EXISTS(
 SELECT 1
 FROM reward_order_actions a
 JOIN reward_orders o ON o.brand_id=a.brand_id AND o.id=a.order_id
 JOIN audit_logs au ON au.id=a.audit_log_id AND au.brand_id=a.brand_id
 LEFT JOIN point_ledger_entries l ON l.id=a.ledger_entry_id AND l.brand_id=a.brand_id
	 WHERE a.brand_id=$1::uuid AND a.id=$2::uuid AND a.order_id=$4::uuid
   AND a.audit_log_id=$8::uuid
   AND o.member_id=$3::uuid AND o.points=$7::text::bigint
   AND a.version=$5 AND a.state_after=$6
	   AND au.actor_type='admin' AND au.actor_id=a.actor_id
	   AND au.action='reward.order.'||a.operation AND au.resource_type='reward_order' AND au.resource_id=o.id
	   AND au.reason=a.reason
   AND au.after_json->>'action_id'=a.id::text AND au.after_json->>'version'=a.version::text
   AND au.after_json->>'state'=a.state_after
   AND (
	    (a.state_after='granted' AND a.operation='grant' AND a.version=1 AND a.state_before IS NULL
	     AND a.actor_id=o.created_by AND a.audit_log_id=o.creation_audit_log_id
	     AND a.ledger_entry_id=o.grant_ledger_entry_id AND o.grant_ledger_entry_id IS NOT NULL
	     AND l.member_id=o.member_id AND l.entry_type='reward_grant' AND l.reference_type='reward_order'
	     AND l.reference_id=o.id AND l.operation_key='reward-grant:'||o.id::text AND l.reversal_of IS NULL
	     AND l.actor_type='admin' AND l.actor_id=a.actor_id AND l.reason=a.reason
     AND l.delta_snapshot=jsonb_build_object(
       'recharge',jsonb_build_object('available','0','manual_frozen','0','system_frozen','0','withdrawal','0'),
       'winning',jsonb_build_object('available','0','manual_frozen','0','system_frozen','0','withdrawal','0'),
       'gift',jsonb_build_object('available',o.points::text,'manual_frozen','0','system_frozen','0','withdrawal','0'),
       'commission',jsonb_build_object('available','0','manual_frozen','0','system_frozen','0','withdrawal','0'))
     AND l.source_allocation=jsonb_build_array(jsonb_build_object('source','gift','state','available','points',o.points::text)))
    OR
    (a.state_after='revocation_pending' AND a.operation IN('revoke','retry') AND a.version>=2
     AND a.ledger_entry_id IS NULL
     AND ((a.operation='revoke' AND a.state_before='granted') OR (a.operation='retry' AND a.state_before='revocation_pending')))
    OR
    (a.state_after='revoked' AND a.operation IN('revoke','retry') AND a.version>=2
     AND ((a.operation='revoke' AND a.state_before='granted') OR (a.operation='retry' AND a.state_before='revocation_pending'))
     AND a.ledger_entry_id=o.revoke_ledger_entry_id AND o.revoke_ledger_entry_id IS NOT NULL
     AND l.member_id=o.member_id AND l.entry_type='reward_reversal' AND l.reference_type='reward_order'
     AND l.reference_id=o.id AND l.operation_key='reward-revoke:'||o.id::text
	     AND l.reversal_of=o.grant_ledger_entry_id AND l.actor_type='admin' AND l.actor_id=a.actor_id
	     AND l.reason=a.reason
     AND l.delta_snapshot=jsonb_build_object(
       'recharge',jsonb_build_object('available','0','manual_frozen','0','system_frozen','0','withdrawal','0'),
       'winning',jsonb_build_object('available','0','manual_frozen','0','system_frozen','0','withdrawal','0'),
       'gift',jsonb_build_object('available',('-'||o.points::text),'manual_frozen','0','system_frozen','0','withdrawal','0'),
       'commission',jsonb_build_object('available','0','manual_frozen','0','system_frozen','0','withdrawal','0'))
     AND l.source_allocation=jsonb_build_array(jsonb_build_object('source','gift','state','available','points',o.points::text))
     AND EXISTS(SELECT 1 FROM reward_order_actions g
       JOIN point_ledger_entries gl ON gl.id=g.ledger_entry_id AND gl.brand_id=g.brand_id
       JOIN audit_logs ga ON ga.id=g.audit_log_id AND ga.brand_id=g.brand_id
       WHERE g.brand_id=o.brand_id AND g.order_id=o.id AND g.operation='grant' AND g.version=1
         AND g.state_before IS NULL AND g.state_after='granted'
         AND g.ledger_entry_id=o.grant_ledger_entry_id AND gl.member_id=o.member_id
	         AND gl.entry_type='reward_grant' AND gl.reference_type='reward_order' AND gl.reference_id=o.id
	         AND gl.operation_key='reward-grant:'||o.id::text AND gl.reversal_of IS NULL
	         AND gl.actor_type='admin' AND gl.actor_id=g.actor_id AND gl.reason=g.reason
	         AND ga.actor_type='admin' AND ga.actor_id=g.actor_id AND ga.action='reward.order.grant'
	         AND ga.resource_type='reward_order' AND ga.resource_id=o.id
	         AND ga.reason=g.reason
         AND ga.after_json->>'action_id'=g.id::text AND ga.after_json->>'version'='1'
         AND ga.after_json->>'state'='granted'
         AND gl.delta_snapshot=jsonb_build_object(
           'recharge',jsonb_build_object('available','0','manual_frozen','0','system_frozen','0','withdrawal','0'),
           'winning',jsonb_build_object('available','0','manual_frozen','0','system_frozen','0','withdrawal','0'),
           'gift',jsonb_build_object('available',o.points::text,'manual_frozen','0','system_frozen','0','withdrawal','0'),
           'commission',jsonb_build_object('available','0','manual_frozen','0','system_frozen','0','withdrawal','0'))
         AND gl.source_allocation=jsonb_build_array(jsonb_build_object('source','gift','state','available','points',o.points::text))))
   )
	)`, brand, aggregate, in.MemberID, in.ResourceID, version, state, in.Points, in.AuditLogID).Scan(&valid)
	if err != nil {
		return "", Payload{}, err
	}
	if !valid {
		return "", Payload{}, ErrInvalid
	}
	return in.MemberID, Payload{ResourceID: in.ResourceID, Points: &in.Points}, nil
}
