package betting

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rulebook"
	"github.com/jackc/pgx/v5"
)

func TestCatalogExposesOnlyPublishedRulesWithoutReservingTransactions(t *testing.T) {
	f := newBettingFixture(t, storeTestBrand)
	ctx := context.Background()
	actor := bettingRuleActor(f.version.CreatedBy, f.brand, "write")
	rs := rulebook.Store{DB: f.db}
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		p, e := rs.CreatePlay(ctx, tx, f.brand, actor, f.game.ID, "unpublished", "Unpublished", "draft fixture", points.Metadata{})
		if e != nil {
			return e
		}
		_, e = rs.CreateVersion(ctx, tx, f.brand, actor, p.ID, bettingDefinition(), "immediate", "unpublished draft", points.Metadata{})
		return e
	})
	list, err := f.service.Games(ctx, f.brand, 50, 0)
	if err != nil || len(list) != 1 || list[0].ID != f.game.ID {
		t.Fatalf("games=%+v err=%v", list, err)
	}
	out, err := f.service.Catalog(ctx, f.brand, f.game.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Plays) != 1 || out.Plays[0].RuleVersionID != f.version.ID || out.Plays[0].DefinitionHash != f.version.DefinitionHash || out.Period == nil || out.Period.ID != f.period.ID || out.PolicyVersions != (PolicyVersions{Brand: 1, Game: 1}) || out.BrandStatus != "active" || time.Since(out.ServerTime) > time.Minute {
		t.Fatalf("catalog=%+v", out)
	}
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"created_by", "reviewed_by", "created_at", "password", "credential_ref", "endpoint", "Unpublished"} {
		if strings.Contains(string(raw), private) {
			t.Fatalf("private/unpublished catalog field %s", private)
		}
	}
	var orders, entries int
	if err = f.db.QueryRow(ctx, `SELECT (SELECT count(*) FROM bet_orders),(SELECT count(*) FROM point_ledger_entries)`).Scan(&orders, &entries); err != nil || orders != 0 || entries != 0 {
		t.Fatalf("catalog reserved transaction: %d/%d err=%v", orders, entries, err)
	}
	if _, err = f.service.Catalog(ctx, storeTestOtherBrand, f.game.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-brand catalog=%v", err)
	}
	for _, invalid := range []string{"", "not-uuid"} {
		if _, err = f.service.Catalog(ctx, f.brand, invalid); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid ID=%v", err)
		}
	}
	if _, err = f.service.Games(ctx, f.brand, 101, 0); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid page=%v", err)
	}
}

func TestCatalogSupportsPausedBrandAndGameWithoutPretendingBettingAllowed(t *testing.T) {
	f := newBettingFixture(t, storeTestBrand)
	ctx := context.Background()
	if _, err := f.db.Exec(ctx, `UPDATE brands SET status='paused' WHERE id=$1`, f.brand); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(ctx, `UPDATE games SET status='paused',version=version+1 WHERE id=$1`, f.game.ID); err != nil {
		t.Fatal(err)
	}
	out, err := f.service.Catalog(ctx, f.brand, f.game.ID)
	if err != nil || out.BrandStatus != "paused" || out.Game.Status != "paused" || out.Period == nil || len(out.Plays) != 1 {
		t.Fatalf("paused catalog=%+v err=%v", out, err)
	}
	if _, err = f.db.Exec(ctx, `UPDATE brands SET status='disabled' WHERE id=$1`, f.brand); err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.Games(ctx, f.brand, 50, 0); !errors.Is(err, ErrNotFound) {
		t.Fatalf("disabled catalog=%v", err)
	}
}

func TestCatalogHasExplicitEmptyAndUpcomingPeriodStates(t *testing.T) {
	f := newBettingFixture(t, storeTestBrand)
	ctx := context.Background()
	var game rulebook.Game
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		var err error
		game, err = (rulebook.Store{DB: f.db}).CreateGame(ctx, tx, f.brand, bettingRuleActor(f.version.CreatedBy, f.brand), "empty_catalog", "Empty catalog", bettingDefinition().Model, "UTC", "catalog fixture", points.Metadata{})
		return err
	})
	out, err := f.service.Catalog(ctx, f.brand, game.ID)
	if err != nil || out.Period != nil || out.Plays == nil || len(out.Plays) != 0 {
		t.Fatalf("empty catalog=%+v err=%v", out, err)
	}
	now := time.Now().UTC()
	for i, offset := range []time.Duration{2 * time.Hour, time.Hour} {
		if _, err = f.db.Exec(ctx, `INSERT INTO periods(id,brand_id,game_id,period_no,sequence,bet_start_at,bet_end_at,draw_at,status) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'pending')`, ids.New(), f.brand, game.ID, ids.New(), i+1, now.Add(offset), now.Add(offset+time.Minute), now.Add(offset+2*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	out, err = f.service.Catalog(ctx, f.brand, game.ID)
	if err != nil || out.Period == nil || out.Period.Status != "pending" || !out.Period.BetStartAt.Equal(now.Add(time.Hour)) {
		t.Fatalf("upcoming period=%+v err=%v", out.Period, err)
	}
}
