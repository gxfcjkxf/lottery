package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"unicode/utf8"

	"github.com/gxfcjkxf/lottery/backend/internal/reconciliation"
)

type reconciliationCreateInput struct {
	Reason     string `json:"reason"`
	CheckScope string `json:"check_scope"`
}

func reconciliationGetHasNoBody(w http.ResponseWriter, r *http.Request) bool {
	if r.Body == nil {
		return true
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1))
	if err != nil || len(raw) != 0 {
		failure(w, r, 400, "REQUEST_INVALID", "查询接口不接受正文")
		return false
	}
	return true
}

func reconciliationClosedObject(raw []byte, allowed map[string]bool) (map[string]json.RawMessage, error) {
	invalid := errors.New("closed reconciliation object required")
	if !utf8.Valid(raw) {
		return nil, invalid
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	t, err := d.Token()
	if err != nil || t != json.Delim('{') {
		return nil, invalid
	}
	fields := map[string]json.RawMessage{}
	for d.More() {
		t, err = d.Token()
		key, ok := t.(string)
		if err != nil || !ok || fields[key] != nil || !allowed[key] {
			return nil, invalid
		}
		var value json.RawMessage
		if d.Decode(&value) != nil || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return nil, invalid
		}
		fields[key] = value
	}
	t, err = d.Token()
	if err != nil || t != json.Delim('}') {
		return nil, invalid
	}
	var tail any
	if d.Decode(&tail) != io.EOF {
		return nil, invalid
	}
	return fields, nil
}

func (in *reconciliationCreateInput) UnmarshalJSON(raw []byte) error {
	fields, err := reconciliationClosedObject(raw, map[string]bool{"reason": true, "check_scope": true})
	if err != nil {
		return err
	}
	var next reconciliationCreateInput
	if json.Unmarshal(fields["reason"], &next.Reason) != nil {
		return errors.New("reconciliation reason is required")
	}
	scope, present := fields["check_scope"]
	if !present || json.Unmarshal(scope, &next.CheckScope) != nil || !reconciliation.ValidScope(next.CheckScope) {
		return errors.New("valid reconciliation scope is required")
	}
	*in = next
	return nil
}

type reconciliationRetryInput struct {
	Version int64  `json:"version"`
	Reason  string `json:"reason"`
}

func (in *reconciliationRetryInput) UnmarshalJSON(raw []byte) error {
	fields, err := reconciliationClosedObject(raw, map[string]bool{"version": true, "reason": true})
	if err != nil {
		return err
	}
	var next reconciliationRetryInput
	if json.Unmarshal(fields["version"], &next.Version) != nil || next.Version < 1 || next.Version >= 9007199254740991 || json.Unmarshal(fields["reason"], &next.Reason) != nil {
		return errors.New("reconciliation version and reason are required")
	}
	*in = next
	return nil
}
