package platformlottery

import (
	"context"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
)

func TestDrawOverviewHistoryAndPending(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	s := Service{DB: db}
	actor := access.Account{ID: ids.New(), Type: access.AccountAdmin, SuperAdmin: true, Roles: []access.Role{{Permissions: []access.Permission{{Resource: "platform_game", Action: "write", Scope: access.ScopePlatform}}}}}
	if _, err := db.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,$2,'test-only')`, actor.ID, "overview_"+actor.ID); err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	game, err := s.Create(ctx, tx, actor, GameInput{Code: "overview", Name: "Overview", Timezone: "UTC", Model: rules.Model{Type: "DIGITS_0_9", Length: 3, Ordered: true, AllowRepeat: true}}, points.Metadata{})
	if err != nil {
		t.Fatal(err)
	}
	other, err := s.Create(ctx, tx, actor, GameInput{Code: "other_overview", Name: "Other", Timezone: "UTC", Model: game.Model}, points.Metadata{})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	// Operational results and imported evidence share one history, without duplicates.
	for _, p := range []struct {
		no               string
		start, end, draw time.Time
		result           any
	}{
		{"2025001", now.Add(-72 * time.Hour), now.Add(-49 * time.Hour), now.Add(-48 * time.Hour), `{"regular":[],"special":[],"digits":[1,2,3]}`},
		{"pending", now.Add(-time.Hour), now.Add(time.Hour), now.Add(2 * time.Hour), nil},
		{"overdue", now.Add(-3 * time.Hour), now.Add(-2 * time.Hour), now.Add(-time.Hour), nil},
	} {
		if _, err = tx.Exec(ctx, `INSERT INTO platform_game_periods(id,game_id,period_no,bet_start_at,bet_end_at,draw_at,result,published_at,published_by) VALUES($1,$2,$3,$4,$5,$6,$7,CASE WHEN $7::jsonb IS NULL THEN NULL ELSE clock_timestamp() END,CASE WHEN $7::jsonb IS NULL THEN NULL ELSE $8::uuid END)`, ids.New(), game.ID, p.no, p.start, p.end, p.draw, p.result, actor.ID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = tx.Exec(ctx, `INSERT INTO platform_draw_history(game_id,period_no,draw_year,result,source) VALUES($1,'2025001',2025,'{"regular":[],"special":[],"digits":[4,5,6]}','hkjc'),($1,'2025002',2025,'{"regular":[],"special":[],"digits":[7,8,9]}','hkjc'),($2,'other',2025,'{"regular":[],"special":[],"digits":[0,0,0]}','hkjc')`, game.ID, other.ID); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	out, err := s.DrawOverview(ctx, game.ID, "2025", 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Items) != 1 || out.Items[0].PeriodNo != "2025002" || out.Items[0].DrawDate != nil || !out.HasMore {
		t.Fatalf("history %#v", out)
	}
	if len(out.Pending) != 2 || out.Pending[0].PeriodNo != "overdue" || out.Pending[0].State != "waiting" || out.Pending[1].State != "open" {
		t.Fatalf("pending %#v", out.Pending)
	}
	page, err := s.DrawOverview(ctx, game.ID, "2025", 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if page.HasMore || len(page.Items) != 1 || page.Items[0].Result.Digits[0] != 4 || page.Items[0].Source != "hkjc" {
		t.Fatalf("duplicate or wrong evidence %#v", page)
	}
	for _, year := range []string{"bad", "02025", "9999"} {
		if _, err = s.DrawOverview(ctx, game.ID, year, 50, 0); err != ErrInvalid {
			t.Fatalf("%s: %v", year, err)
		}
	}
	if _, err = s.DrawOverview(ctx, ids.New(), "", 50, 0); err != ErrNotFound {
		t.Fatal(err)
	}
}
