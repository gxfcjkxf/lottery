package platformlottery

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
)

func TestAuditSnapshotsRetainLockedValues(t *testing.T) {
	db := testdb.NewUnseeded(t)
	ctx := context.Background()
	a := access.Account{ID: ids.New(), Type: access.AccountAdmin, SuperAdmin: true, Roles: []access.Role{{Permissions: []access.Permission{{Resource: "platform_game", Action: "write", Scope: access.ScopePlatform}}}}}
	if _, err := db.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,'audit_snapshot_operator','test-only')`, a.ID); err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	s := Service{DB: db}
	raw, err := os.ReadFile("../rules/config/examples/lucky_group_preview.json")
	if err != nil {
		t.Fatal(err)
	}
	var snapshot rules.ConfiguredSnapshot
	if err = json.Unmarshal(raw, &snapshot); err != nil {
		t.Fatal(err)
	}
	g, err := s.Create(ctx, tx, a, GameInput{Code: "audit_snapshots", Name: "Before", Model: snapshot.Catalog.Model, Timezone: "UTC"}, points.Metadata{})
	if err != nil {
		t.Fatal(err)
	}
	check := func(action, resource, field string, old, next any) {
		t.Helper()
		var before, after []byte
		if err := tx.QueryRow(ctx, `SELECT before_json,after_json FROM audit_logs WHERE action=$1 AND resource_id=$2 ORDER BY created_at DESC,id DESC LIMIT 1`, action, resource).Scan(&before, &after); err != nil {
			t.Fatal(err)
		}
		var b, a map[string]any
		if err := json.Unmarshal(before, &b); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(after, &a); err != nil {
			t.Fatal(err)
		}
		normalize := func(v any) any {
			if raw, ok := v.(string); ok {
				if instant, err := time.Parse(time.RFC3339Nano, raw); err == nil {
					return instant.UTC().Format(time.RFC3339Nano)
				}
			}
			return v
		}
		if normalize(b[field]) != normalize(old) || normalize(a[field]) != normalize(next) {
			t.Fatalf("%s: %s before=%v after=%v", action, field, b[field], a[field])
		}
	}
	g, err = s.UpdateName(ctx, tx, a, g.ID, NameInput{Name: "After", ExpectedVersion: g.Version}, points.Metadata{})
	if err != nil {
		t.Fatal(err)
	}
	check("platform_game.name.update", g.ID, "name", "Before", "After")
	g, err = s.SetLifecycle(ctx, tx, a, g.ID, LifecycleInput{ExpectedVersion: g.Version, Status: "offline"}, points.Metadata{})
	if err != nil {
		t.Fatal(err)
	}
	check("platform_game.lifecycle", g.ID, "status", "active", "offline")
	g, err = s.SetLifecycle(ctx, tx, a, g.ID, LifecycleInput{ExpectedVersion: g.Version, Status: "active"}, points.Metadata{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SaveConfiguredRules(ctx, tx, a, g.ID, ConfiguredRulesInput{Snapshot: raw}, points.Metadata{}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	p, err := s.CreatePeriod(ctx, tx, a, g.ID, PeriodInput{PeriodNo: "1", BetStartAt: now.Add(time.Hour), BetEndAt: now.Add(2 * time.Hour), DrawAt: now.Add(3 * time.Hour)}, points.Metadata{})
	if err != nil {
		t.Fatal(err)
	}
	old := p.BetStartAt
	p, err = s.UpdatePeriodOpening(ctx, tx, a, p.ID, PeriodOpeningInput{ExpectedBetStartAt: old, BetStartAt: now.Add(30 * time.Minute)}, points.Metadata{})
	if err != nil {
		t.Fatal(err)
	}
	check("platform_game.period.opening", p.ID, "bet_start_at", old.Format(time.RFC3339), p.BetStartAt.Format(time.RFC3339))
}
