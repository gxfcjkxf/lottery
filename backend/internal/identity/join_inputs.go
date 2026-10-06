package identity

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
)

// A custom decoder must retain DisallowUnknownFields semantics and reject
// duplicate or null selectors, rather than silently taking the last code.
func decodeAuthObject(data []byte, out any) error {
	allowed := map[string]bool{}
	typ := reflect.TypeOf(out).Elem()
	for i := 0; i < typ.NumField(); i++ {
		tag := strings.Split(typ.Field(i).Tag.Get("json"), ",")[0]
		if tag != "" && tag != "-" {
			allowed[tag] = true
		}
	}
	d := json.NewDecoder(bytes.NewReader(data))
	t, e := d.Token()
	if e != nil || t != json.Delim('{') {
		return errors.New("authentication input must be an object")
	}
	seen := map[string]bool{}
	for d.More() {
		t, e = d.Token()
		key, ok := t.(string)
		if e != nil || !ok || !allowed[key] || seen[key] {
			return errors.New("unknown or duplicate authentication input")
		}
		seen[key] = true
		var value json.RawMessage
		if d.Decode(&value) != nil || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return errors.New("null authentication input")
		}
	}
	t, e = d.Token()
	if e != nil || t != json.Delim('}') {
		return errors.New("invalid authentication object")
	}
	var tail any
	if d.Decode(&tail) != io.EOF {
		return errors.New("extra authentication input")
	}
	return json.Unmarshal(data, out)
}
func (v *RegisterInput) UnmarshalJSON(b []byte) error {
	type plain RegisterInput
	var p plain
	if e := decodeAuthObject(b, &p); e != nil {
		return e
	}
	*v = RegisterInput(p)
	return nil
}
func (v *LoginInput) UnmarshalJSON(b []byte) error {
	type plain LoginInput
	var p plain
	if e := decodeAuthObject(b, &p); e != nil {
		return e
	}
	*v = LoginInput(p)
	return nil
}
func (v *TelegramInput) UnmarshalJSON(b []byte) error {
	type plain TelegramInput
	var p plain
	if e := decodeAuthObject(b, &p); e != nil {
		return e
	}
	*v = TelegramInput(p)
	return nil
}
func (v *OperatorInput) UnmarshalJSON(b []byte) error {
	type plain OperatorInput
	var p plain
	if e := decodeAuthObject(b, &p); e != nil {
		return e
	}
	*v = OperatorInput(p)
	return nil
}
