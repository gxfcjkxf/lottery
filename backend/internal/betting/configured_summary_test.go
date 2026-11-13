package betting

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/rules"
)

func assertConfiguredSummary(t *testing.T, full, summary ConfiguredOrder) {
	t.Helper()
	originalSnapshot := full.Snapshot
	full.Snapshot = nil
	compactSnapshot := summary.Snapshot
	summary.Snapshot = nil
	if !reflect.DeepEqual(full, summary) {
		t.Fatal("summary changed order data")
	}
	var original, compact rules.ConfiguredSnapshot
	if err := json.Unmarshal(originalSnapshot, &original); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(compactSnapshot, &compact); err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(compactSnapshot, &fields); err != nil {
		t.Fatal(err)
	}
	if len(fields) != 1 || fields["catalog"] == nil {
		t.Fatal("summary includes executable snapshot")
	}
	if original.Catalog.Funding != compact.Catalog.Funding {
		t.Fatal("funding changed")
	}
	var check func([]rules.ConfiguredQuoteLine)
	check = func(lines []rules.ConfiguredQuoteLine) {
		for _, line := range lines {
			var before, after *rules.ConfiguredPlay
			for i := range original.Catalog.Plays {
				if original.Catalog.Plays[i].ID == line.PlayID {
					before = &original.Catalog.Plays[i]
				}
			}
			for i := range compact.Catalog.Plays {
				if compact.Catalog.Plays[i].ID == line.PlayID {
					after = &compact.Catalog.Plays[i]
				}
			}
			if before == nil || after == nil || !reflect.DeepEqual(before.Label, after.Label) || before.Selection.Domain != after.Selection.Domain {
				t.Fatal("play label changed", line.PlayID)
			}
			for _, id := range line.Options {
				found := false
				for _, option := range original.Catalog.Domains[before.Selection.Domain].Options {
					if option.ID != id {
						continue
					}
					for _, display := range compact.Catalog.Domains[after.Selection.Domain].Options {
						if display.ID == id && reflect.DeepEqual(display.Label, option.Label) {
							found = true
						}
					}
				}
				if !found {
					t.Fatal("option label missing or changed", id)
				}
			}
			check(line.Legs)
		}
	}
	check(full.Quote.Lines)
}

func TestConfiguredOrderSummaries(t *testing.T) {
	for _, combinations := range []bool{false, true} {
		name := "singles"
		if combinations {
			name = "combinations"
		}
		t.Run(name, func(t *testing.T) {
			c := itemFixture(t, combinations)
			ctx := context.Background()
			fundBettingWallet(t, c.f, 10000)
			if _, err := placeItems(t, c, "summary-items"); err != nil {
				t.Fatal(err)
			}
			full, err := c.f.service.ConfiguredOrders(ctx, c.f.brand, c.f.member, c.g.ID, 0)
			if err != nil {
				t.Fatal(err)
			}
			summaries, err := c.f.service.ConfiguredOrderSummaries(ctx, c.f.brand, c.f.member, c.g.ID, 0)
			if err != nil || len(full) == 0 || len(full) != len(summaries) {
				t.Fatal("summary list", err)
			}
			for i := range full {
				assertConfiguredSummary(t, full[i], summaries[i])
			}
			tx, err := c.f.db.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			page, err := HistoryTx(ctx, tx, c.f.brand, c.f.member, c.g.ID, "", 20)
			if err != nil || len(page.Items) != len(full) {
				t.Fatal("history summaries", err)
			}
			for i, item := range page.Items {
				assertConfiguredSummary(t, full[i], item.Order.(ConfiguredOrder))
			}

			for _, filter := range [][3]string{{storeTestOtherBrand, c.f.member, c.g.ID}, {c.f.brand, storeTestOtherBrand, c.g.ID}, {c.f.brand, c.f.member, storeTestOtherBrand}} {
				rows, err := c.f.service.ConfiguredOrderSummaries(ctx, filter[0], filter[1], filter[2], 0)
				if err != nil || len(rows) != 0 {
					t.Fatal("summary scope", err)
				}
			}
			rows, err := c.f.service.ConfiguredOrderSummaries(ctx, c.f.brand, "", "", len(full))
			if err != nil || len(rows) != 0 {
				t.Fatal("summary offset", err)
			}
		})
	}
}
