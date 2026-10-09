package main

import (
	"github.com/gxfcjkxf/lottery/backend/internal/reconciliation"
	"strings"
	"time"
)

func businessInventoryExample() reconciliation.InventorySnapshot {
	coverage := make([]reconciliation.InventoryCoverage, 0, len(reconciliation.InventorySources()))
	for _, source := range reconciliation.InventorySources() {
		c := reconciliation.InventoryCoverage{SourceTable: source, SourceRowCount: "0", ReferenceCount: "0", IssueCount: "0"}
		if source == "point_accounts" {
			c.SourceRowCount = "1"
			c.ReferenceCount = "1"
			c.IssueCount = "1"
		}
		coverage = append(coverage, c)
	}
	return reconciliation.InventorySnapshot{
		BrandID: "11111111-1111-4111-8111-111111111111", SnapshotAt: time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC), SchemaVersion: 1,
		SourceRowCount: "1", ReferenceCount: "1", IssueCount: "1", Consistent: false, IssuesTruncated: false, Fingerprint: strings.Repeat("a", 64), Coverage: coverage,
		Issues: []reconciliation.InventoryIssue{{SourceTable: "point_accounts", SourceID: "11111111-1111-4111-8111-111111111111", Code: "MISSING_PARENT_REFERENCE", ReferenceKey: "point_accounts_brand_id_brand_member_id_fkey", ParentTable: "brand_members"}},
	}
}
