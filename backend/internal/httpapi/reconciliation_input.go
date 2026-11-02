package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"
)

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

func reconciliationBrand(w http.ResponseWriter, r *http.Request) (string, bool) {
	brand := strings.ToLower(r.Header.Get("X-Brand-ID"))
	if !uuidPattern.MatchString(brand) {
		failure(w, r, 400, "REQUEST_INVALID", "必须选择有效品牌")
		return "", false
	}
	return brand, true
}

func reconciliationPage(r *http.Request, allowOutcome bool) (limit, offset int, outcome string, ok bool) {
	limit, offset = 20, 0
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return 0, 0, "", false
	}
	for key, values := range query {
		if len(values) != 1 || len(values[0]) == 0 || key != "limit" && key != "offset" && !(allowOutcome && key == "outcome") {
			return 0, 0, "", false
		}
		switch key {
		case "limit":
			limit, err = strconv.Atoi(values[0])
			if err == nil && strconv.Itoa(limit) != values[0] {
				return 0, 0, "", false
			}
		case "offset":
			offset, err = strconv.Atoi(values[0])
			if err == nil && strconv.Itoa(offset) != values[0] {
				return 0, 0, "", false
			}
		case "outcome":
			outcome = values[0]
		}
		if err != nil {
			return 0, 0, "", false
		}
	}
	if limit < 1 || limit > 100 || offset < 0 || offset > 1000000 {
		return 0, 0, "", false
	}
	switch outcome {
	case "", "pending", "failed", "consistent", "repairable", "corrupt":
	default:
		return 0, 0, "", false
	}
	return limit, offset, outcome, true
}
