package commissionreview

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestCanonicalJSONSortsKeysAndPreservesLargeIntegers(t *testing.T) {
	left, err := canonicalJSON([]byte(`{"z":9007199254740993123456789,"a":{"y":2,"x":1}}`))
	if err != nil {
		t.Fatal(err)
	}
	right, err := canonicalJSON([]byte(`{"a":{"x":1,"y":2},"z":9007199254740993123456789}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(left) != string(right) {
		t.Fatalf("canonical forms differ:\n%s\n%s", left, right)
	}
	if !strings.Contains(string(left), "9007199254740993123456789") {
		t.Fatalf("large integer lost precision: %s", left)
	}
}

func TestCanonicalJSONRejectsDuplicateObjectKeys(t *testing.T) {
	if _, err := canonicalJSON([]byte(`{"points":1,"points":2}`)); err == nil {
		t.Fatal("duplicate object key accepted")
	}
}

func TestSourceDigestNormalizesReportTimeAndTracksSavedEvidence(t *testing.T) {
	base := Report{
		FormatVersion: 1,
		BrandID:       "brand",
		CycleID:       "cycle",
		SnapshotAt:    time.Date(2026, 10, 9, 1, 2, 3, 0, time.UTC),
		Payments:      []Payment{},
	}
	rows := []sourceRow{{section: "ledger", value: json.RawMessage(`{"delta":9007199254740993,"nested":{"b":2,"a":1}}`)}}
	a, err := sourceDigest(base, rows)
	if err != nil {
		t.Fatal(err)
	}
	base.SnapshotAt = base.SnapshotAt.Add(time.Hour)
	b, err := sourceDigest(base, []sourceRow{{section: "ledger", value: json.RawMessage(`{"nested":{"a":1,"b":2},"delta":9007199254740993}`)}})
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatalf("irrelevant snapshot time or JSON key order changed digest: %s != %s", a, b)
	}
	changed, err := sourceDigest(base, []sourceRow{{section: "ledger", value: json.RawMessage(`{"delta":9007199254740994,"nested":{"a":1,"b":2}}`)}})
	if err != nil {
		t.Fatal(err)
	}
	if a == changed {
		t.Fatal("financial evidence change did not change digest")
	}
	duplicated, err := sourceDigest(base, append(rows, rows[0]))
	if err != nil {
		t.Fatal(err)
	}
	if a == duplicated {
		t.Fatal("duplicate source row was silently discarded")
	}
}
