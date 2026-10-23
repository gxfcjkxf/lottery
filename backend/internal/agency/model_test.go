package agency

import (
	"encoding/json"
	"testing"
)

func TestOptionalReasonCannotHideMissingAgentFields(t *testing.T) {
	cases := []struct {
		name, body string
		decode     func([]byte) error
	}{
		{"create", `{"policy_version":1,"username":"example_agent","password":"agent123","parent_id":null,"parent_version":null,"config":{"ratio":"0","mode":null,"status":"active","can_create_children":true},"reason":"test"}`, func(b []byte) error { var v CreateInput; return json.Unmarshal(b, &v) }},
		{"update", `{"version":1,"policy_version":1,"parent_version":null,"config":{"ratio":"0","mode":null,"status":"active","can_create_children":true},"reason":"test"}`, func(b []byte) error { var v UpdateInput; return json.Unmarshal(b, &v) }},
		{"policy", `{"version":1,"config":{"enabled":true,"max_depth":5,"loss_ratio_cap": "1", "turnover_ratio_cap": "1","mode":"loss","cycle":"weekly"},"reason":"test"}`, func(b []byte) error { var v PolicyInput; return json.Unmarshal(b, &v) }},
		{"child", `{"version":1,"policy_version":1,"parent_version":1,"ratio":"0","mode":null,"reason":"test"}`, func(b []byte) error { var v ChildInput; return json.Unmarshal(b, &v) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if e := c.decode([]byte(c.body)); e != nil {
				t.Fatal(e)
			}
			var fields map[string]json.RawMessage
			if e := json.Unmarshal([]byte(c.body), &fields); e != nil {
				t.Fatal(e)
			}
			for key, value := range fields {
				delete(fields, key)
				b, e := json.Marshal(fields)
				if e != nil {
					t.Fatal(e)
				}
				err := c.decode(b)
				if key == "reason" && err != nil {
					t.Fatal("optional reason rejected", err)
				}
				if key != "reason" && err == nil {
					t.Fatalf("accepted omitted %s", key)
				}
				fields[key] = value
			}
		})
	}
}
