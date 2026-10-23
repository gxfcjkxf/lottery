package betting

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"testing"
	"time"
)

func TestPlayerArchiveYearDateOrderingAndTenantIsolation(t *testing.T) {
	f := newBettingFixture(t, storeTestBrand)
	ctx := context.Background()
	if _, err := f.db.Exec(ctx, `UPDATE platform_settings SET timezone='Asia/Singapore' WHERE id=true`); err != nil {
		t.Fatal(err)
	}
	location, err := time.LoadLocation("Asia/Singapore")
	if err != nil {
		t.Fatal(err)
	}
	year := time.Now().In(location).Year() - 1
	first := time.Date(year, 1, 1, 0, 30, 0, 0, location)
	second := first.AddDate(0, 0, 1)
	for i, entry := range []struct {
		number string
		at     time.Time
	}{{"9", first}, {"10", second}} {
		id, v := publicTestPeriod(t, f, f.game.ID, entry.number, 500+i, entry.at)
		publicManual(t, f, id, v, entry.number, rules.Draw{Digits: []int{1, 2, 3 + i}}, entry.at)
	}
	page, err := f.service.DrawArchive(ctx, f.brand, f.game.ID, "standard", first.Format("2006"), 0)
	if err != nil || page.Daily || len(page.Items) != 2 || page.Items[0].PeriodNo != "10" || page.Items[1].PeriodNo != "9" {
		t.Fatalf("annual archive: %+v %v", page, err)
	}
	id, v := publicTestPeriod(t, f, f.game.ID, "11", 502, first.Add(time.Hour))
	publicManual(t, f, id, v, "11", rules.Draw{Digits: []int{2, 3, 4}}, first.Add(time.Hour))
	daily, err := f.service.DrawArchive(ctx, f.brand, f.game.ID, "standard", first.Format("2006-01-02"), 0)
	if err != nil || !daily.Daily || len(daily.Items) != 2 || daily.Items[0].PeriodNo != "11" {
		t.Fatalf("daily archive: %+v %v", daily, err)
	}
	empty, err := f.service.DrawArchive(ctx, f.brand, f.game.ID, "standard", "", 0)
	if err != nil || len(empty.Items) != 0 || empty.Selected != time.Now().In(location).Format("2006-01-02") {
		t.Fatalf("default today: %+v %v", empty, err)
	}
	if _, err = f.service.DrawArchive(ctx, storeTestOtherBrand, f.game.ID, "standard", "", 0); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-brand archive: %v", err)
	}
	var actor string
	if err = f.db.QueryRow(ctx, `SELECT id::text FROM admin_accounts LIMIT 1`).Scan(&actor); err != nil {
		t.Fatal(err)
	}
	game := ids.New()
	model, _ := json.Marshal(bettingDefinition().Model)
	if _, err = f.db.Exec(ctx, `INSERT INTO platform_games(id,code,name,model,timezone,created_by) VALUES($1,'archive_test','Archive test',$2,'Asia/Singapore',$3)`, game, model, actor); err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.Exec(ctx, `INSERT INTO brand_platform_games(brand_id,game_id,connected_by) VALUES($1,$2,$3)`, f.brand, game, actor); err != nil {
		t.Fatal(err)
	}
	// Local midnight is 16:00 UTC on the preceding calendar day.
	for i, at := range []time.Time{time.Date(year, 1, 1, 15, 59, 0, 0, time.UTC), time.Date(year, 1, 1, 16, 1, 0, 0, time.UTC), time.Date(year, 1, 1, 16, 2, 0, 0, time.UTC)} {
		if _, err = f.db.Exec(ctx, `INSERT INTO platform_game_periods(id,game_id,period_no,bet_start_at,bet_end_at,draw_at,result,published_at,published_by) VALUES($1,$2,$3,$4,$5,$6,'{"regular":null,"special":null,"digits":[1,2,3]}',$6,$7)`, ids.New(), game, []string{"9", "10", "11"}[i], at.Add(-time.Hour), at.Add(-time.Minute), at, actor); err != nil {
			t.Fatal(err)
		}
	}
	configured, err := f.service.DrawArchive(ctx, f.brand, game, "configured", first.AddDate(0, 0, 1).Format("2006-01-02"), 0)
	if err != nil || !configured.Daily || len(configured.Items) != 2 || configured.Items[0].PeriodNo != "11" {
		t.Fatalf("configured local midnight: %+v %v", configured, err)
	}
	if configured.Items[0].Result.Regular == nil || configured.Items[0].Result.Special == nil {
		t.Fatal("archive groups must be JSON arrays")
	}
	if _, err = f.service.DrawArchive(ctx, storeTestOtherBrand, game, "configured", "", 0); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-brand platform archive: %v", err)
	}
}

func TestPlayerArchiveUsesPublicationAtDrawTime(t *testing.T) {
	f := newBettingFixture(t, storeTestBrand)
	ctx := context.Background()
	var actor string
	if err := f.db.QueryRow(ctx, `SELECT id::text FROM admin_accounts LIMIT 1`).Scan(&actor); err != nil {
		t.Fatal(err)
	}
	game, period := ids.New(), ids.New()
	model := rules.Model{Type: "X_PLUS_Y", RegularCount: 6, SpecialCount: 1, RegularPool: rules.Pool{Min: 1, Max: 49}, SpecialPool: rules.Pool{Min: 1, Max: 49}}
	raw, err := json.Marshal(model)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.Exec(ctx, `INSERT INTO platform_games(id,code,name,model,timezone,created_by) VALUES($1,'historical_attributes','Historical attributes',$2,'Asia/Singapore',$3)`, game, raw, actor); err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.Exec(ctx, `INSERT INTO brand_platform_games(brand_id,game_id,connected_by) VALUES($1,$2,$3)`, f.brand, game, actor); err != nil {
		t.Fatal(err)
	}
	at := time.Now().Add(-2 * time.Hour)
	if _, err = f.db.Exec(ctx, `INSERT INTO platform_game_periods(id,game_id,period_no,bet_start_at,bet_end_at,draw_at,result,published_at,published_by) VALUES($1,$2,'2026001',$3,$4,$5,'{"regular":[1,2,3,4,5,6],"special":[7],"digits":[]}',$5,$6)`, period, game, at.Add(-time.Hour), at.Add(-time.Minute), at, actor); err != nil {
		t.Fatal(err)
	}
	var latest string
	for i, zodiac := range []string{"rat", "dragon"} {
		latest = ids.New()
		snapshot, _ := json.Marshal(map[string]any{"bindings": map[string]any{"attributes": map[string]any{"wave": map[string]any{"red": []int{1, 2, 3, 4, 5, 6, 7}}, "zodiac": map[string]any{zodiac: []int{1, 2, 3, 4, 5, 6, 7}}, "element": map[string]any{"wood": []int{1, 2, 3, 4, 5, 6, 7}}}}})
		if _, err = f.db.Exec(ctx, `INSERT INTO configured_rule_versions(id,game_id,snapshot,snapshot_hash,cases,created_by,status) VALUES($1,$2,$3,$4,'[]',$5,'published')`, latest, game, snapshot, latest, actor); err != nil {
			t.Fatal(err)
		}
		publishedAt := at.Add(-time.Hour)
		if i == 1 {
			publishedAt = at.Add(time.Hour)
		}
		if _, err = f.db.Exec(ctx, `INSERT INTO audit_logs(id,actor_type,actor_id,action,resource_type,resource_id,request_id,created_at) VALUES($1,'admin',$2,'configured_rule.publish','platform_game',$3,'archive-attribute-test',$4)`, ids.New(), actor, latest, publishedAt); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = f.db.Exec(ctx, `INSERT INTO configured_rule_heads(game_id,version_id) VALUES($1,$2)`, game, latest); err != nil {
		t.Fatal(err)
	}
	archive, err := f.service.DrawArchive(ctx, f.brand, game, "configured", "", 0)
	if err != nil || !archive.MarkSix || len(archive.Items) != 1 {
		t.Fatalf("archive %+v %v", archive, err)
	}
	if got := archive.Items[0].Attributes["07"]; got.Zodiac != "rat" || got.Wave != "red" || got.Element != "wood" {
		t.Fatalf("historical attribute replaced: %+v", got)
	}
}
