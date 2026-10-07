package commission

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestDiscoveryReadDTOUsesSafeVersionAndOmitsWorkerInternals(t *testing.T) {
	from := time.Date(2026, 10, 1, 0, 0, 0, 0, time.FixedZone("UTC+8", 8*60*60))
	to := from.Add(24 * time.Hour)
	cycleID := batchAnchorID
	logID := batchAdminID
	errorCode := "source_unavailable"
	discovery := Discovery{
		ID: batchAnchorID, BrandID: batchBrandID, State: "failed", Version: maxCycleVersion,
		CycleID: &cycleID, WindowFrom: &from, WindowTo: &to, NextCheckAt: from,
		LastErrorCode: &errorCode, LastAuditLogID: &logID, CreatedAt: from, UpdatedAt: to,
	}
	raw, err := json.Marshal(discovery)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if !strings.Contains(text, `"version":9007199254740991`) ||
		!strings.Contains(text, `"cycle_id":"`+cycleID+`"`) ||
		strings.Contains(text, "snapshot") || strings.Contains(text, "cursor") || strings.Contains(text, "account_id") {
		t.Fatalf("discovery JSON has unsafe version or internal data: %s", text)
	}
}

func TestDiscoveryPageHasNonNilItemsAndStringCount(t *testing.T) {
	page := DiscoveryPage{BrandID: batchBrandID, Items: []Discovery{}, TotalCount: "9007199254740993", Limit: 25, Offset: 50}
	raw, err := json.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if !strings.Contains(text, `"items":[]`) || !strings.Contains(text, `"total_count":"9007199254740993"`) {
		t.Fatalf("discovery page lost empty-list or exact count encoding: %s", text)
	}
}
