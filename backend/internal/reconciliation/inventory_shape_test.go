package reconciliation

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func inventoryShape(issues int) InventorySnapshot {
	out := InventorySnapshot{BrandID: testBrand, SnapshotAt: time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC), SchemaVersion: 1,
		SourceRowCount: "0", ReferenceCount: "0", IssueCount: fmt.Sprint(issues), Consistent: issues == 0, IssuesTruncated: issues > 100,
		Fingerprint: strings.Repeat("a", 64), Coverage: []InventoryCoverage{}, Issues: []InventoryIssue{}}
	for _, table := range InventorySources() {
		c := InventoryCoverage{SourceTable: table, SourceRowCount: "0", ReferenceCount: "0", IssueCount: "0"}
		if table == "point_accounts" && issues > 0 {
			c.SourceRowCount = fmt.Sprint(issues)
			c.ReferenceCount = fmt.Sprint(issues)
			c.IssueCount = fmt.Sprint(issues)
		}
		out.Coverage = append(out.Coverage, c)
	}
	if issues > 0 {
		out.SourceRowCount = fmt.Sprint(issues)
		out.ReferenceCount = fmt.Sprint(issues)
	}
	for i := 0; i < issues && i < 100; i++ {
		out.Issues = append(out.Issues, InventoryIssue{SourceTable: "point_accounts", SourceID: fmt.Sprintf("%03d", i), Code: "MISSING_PARENT_REFERENCE", ReferenceKey: "member_id", ParentTable: "brand_members"})
	}
	return out
}
func TestInventoryShapeExactCountsTruncationAndClosedSourceSet(t *testing.T) {
	for _, n := range []int{0, 1, 100, 101} {
		if !validInventory(inventoryShape(n), testBrand) {
			t.Fatal("valid complete shape rejected", n)
		}
	}
	changes := map[string]func(*InventorySnapshot){
		"foreignbrand":          func(o *InventorySnapshot) { o.BrandID = otherBrand },
		"schema":                func(o *InventorySnapshot) { o.SchemaVersion = 2 },
		"badtime":               func(o *InventorySnapshot) { o.SnapshotAt = time.Time{} },
		"unsafe-source-cap":     func(o *InventorySnapshot) { o.SourceRowCount = "100001" },
		"numberformat":          func(o *InventorySnapshot) { o.ReferenceCount = "01" },
		"unboundedintegerinput": func(o *InventorySnapshot) { o.ReferenceCount = strings.Repeat("9", 201) },
		"fake-clean":            func(o *InventorySnapshot) { o.Consistent = true },
		"truncation":            func(o *InventorySnapshot) { o.IssuesTruncated = true },
		"partial-source":        func(o *InventorySnapshot) { o.Coverage = o.Coverage[:40] },
		"unsorted-source":       func(o *InventorySnapshot) { o.Coverage[0], o.Coverage[1] = o.Coverage[1], o.Coverage[0] },
		"sum":                   func(o *InventorySnapshot) { o.Coverage[0].SourceRowCount = "1" },
		"bad-digest":            func(o *InventorySnapshot) { o.Fingerprint = strings.Repeat("A", 64) },
		"private-key":           func(o *InventorySnapshot) { o.Issues[0].ReferenceKey = "Private reason" },
		"unsafe-source-id":      func(o *InventorySnapshot) { o.Issues[0].SourceID = "name with spaces" },
		"unknown-code":          func(o *InventorySnapshot) { o.Issues[0].Code = "CLEAN" },
		"issue-wrong-family":    func(o *InventorySnapshot) { o.Issues[0].SourceTable = "brand_members" },
		"nil-issues":            func(o *InventorySnapshot) { o.Issues = nil },
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			out := inventoryShape(1)
			change(&out)
			if validInventory(out, testBrand) {
				t.Fatal("invalid shape accepted")
			}
		})
	}
	out := inventoryShape(101)
	out.Issues = out.Issues[:99]
	if validInventory(out, testBrand) {
		t.Fatal("truncated result must display100")
	}
	out = inventoryShape(2)
	out.Issues[1] = out.Issues[0]
	if validInventory(out, testBrand) {
		t.Fatal("duplicate issue tuple")
	}
	out = inventoryShape(2)
	out.Issues[0], out.Issues[1] = out.Issues[1], out.Issues[0]
	if validInventory(out, testBrand) {
		t.Fatal("unsorted issue tuple")
	}
	out = inventoryShape(1)
	out.Issues[0].SourceTable = InventorySources()[0]
	if validInventory(out, testBrand) {
		t.Fatal("sample exceeds its coverage")
	}
}
