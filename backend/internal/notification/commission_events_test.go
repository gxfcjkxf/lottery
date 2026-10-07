package notification

import "testing"

func TestCommissionEventPayloadRequiresFiveUniqueClosedKeys(t *testing.T) {
	valid := []byte(`{"member_id":"11111111-1111-4111-8111-111111111111","resource_id":"22222222-2222-4222-8222-222222222222","points":"-2","ledger_entry_id":"33333333-3333-4333-8333-333333333333","target_id":"22222222-2222-4222-8222-222222222222"}`)
	if !exactCommissionJSONKeys(valid, "member_id", "resource_id", "points", "ledger_entry_id", "target_id") {
		t.Fatal("valid closed commission event payload rejected")
	}
	for _, raw := range [][]byte{
		[]byte(`{"member_id":"a","member_id":"b","resource_id":"c","points":"1","ledger_entry_id":"d","target_id":"c"}`),
		[]byte(`{"member_id":"a","resource_id":"c","points":"1","ledger_entry_id":"d","target_id":"c","private":"x"}`),
		[]byte(`{"member_id":"a","resource_id":"c","points":"1","ledger_entry_id":"d"}`),
		[]byte(`{"member_id":null,"resource_id":"c","points":"1","ledger_entry_id":"d","target_id":"c"}`),
		append(append([]byte(nil), valid...), []byte(` {}`)...),
		[]byte(`[]`),
		[]byte(`{"member_id":null,"resource_id":"c","points":"1","ledger_entry_id":"d","target_id":"c"}`),
		[]byte(`{"member_id":"a","resource_id":null,"points":"1","ledger_entry_id":"d","target_id":"c"}`),
		[]byte(`{"member_id":"a","resource_id":"c","points":null,"ledger_entry_id":"d","target_id":"c"}`),
		[]byte(`{"member_id":"a","resource_id":"c","points":"1","ledger_entry_id":null,"target_id":"c"}`),
		[]byte(`{"member_id":"a","resource_id":"c","points":"1","ledger_entry_id":"d","target_id":null}`),
	} {
		if exactCommissionJSONKeys(raw, "member_id", "resource_id", "points", "ledger_entry_id", "target_id") {
			t.Errorf("accepted malformed commission event JSON: %s", raw)
		}
	}
}
