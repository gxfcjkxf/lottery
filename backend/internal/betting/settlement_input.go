package betting

import (
	"bytes"
	"encoding/json"
	"io"
)

// All write fields are required. In particular missing mode is not permission
// to unset a policy, and duplicate fields cannot change idempotency semantics.
func decodeSettlementInput(raw []byte, out any, nullable string, fields ...string) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	t, e := d.Token()
	if e != nil || t != json.Delim('{') {
		return ErrInvalid
	}
	seen := map[string]bool{}
	allowed := map[string]bool{}
	for _, f := range fields {
		allowed[f] = true
	}
	for d.More() {
		t, e = d.Token()
		if e != nil {
			return ErrInvalid
		}
		k, ok := t.(string)
		if !ok || seen[k] || !allowed[k] {
			return ErrInvalid
		}
		seen[k] = true
		var v json.RawMessage
		if d.Decode(&v) != nil || bytes.Equal(v, []byte("null")) && k != nullable {
			return ErrInvalid
		}
	}
	if _, e = d.Token(); e != nil || len(seen) != len(fields) {
		return ErrInvalid
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return ErrInvalid
	}
	if json.Unmarshal(raw, out) != nil {
		return ErrInvalid
	}
	return nil
}
func (in *SettlementPolicyInput) UnmarshalJSON(raw []byte) error {
	type alias SettlementPolicyInput
	var v alias
	if e := decodeSettlementInput(raw, &v, "mode", "version", "mode", "reason"); e != nil {
		return e
	}
	*in = SettlementPolicyInput(v)
	return nil
}
func (in *SettlementStartInput) UnmarshalJSON(raw []byte) error {
	type alias SettlementStartInput
	var v alias
	if e := decodeSettlementInput(raw, &v, "", "version", "policy_version", "draw_result_id", "reason"); e != nil {
		return e
	}
	*in = SettlementStartInput(v)
	return nil
}
func (in *SettlementActionInput) UnmarshalJSON(raw []byte) error {
	type alias SettlementActionInput
	var v alias
	if e := decodeSettlementInput(raw, &v, "", "version", "reason"); e != nil {
		return e
	}
	*in = SettlementActionInput(v)
	return nil
}
