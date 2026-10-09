package betting

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/rulebook"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/jackc/pgx/v5"
)

func publicTestPeriod(t *testing.T, f bettingFixture, game, number string, sequence int, drawAt time.Time) (string, int64) {
	t.Helper()
	id := ids.New()
	ctx := context.Background()
	if _, e := f.db.Exec(ctx, `INSERT INTO periods(id,brand_id,game_id,period_no,sequence,bet_start_at,bet_end_at,draw_at,status) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'pending')`, id, f.brand, game, number, sequence, drawAt.Add(-2*time.Hour), drawAt.Add(-time.Hour), drawAt); e != nil {
		t.Fatal(e)
	}
	for _, status := range []string{"betting", "closed", "waiting_draw"} {
		if _, e := f.db.Exec(ctx, `UPDATE periods SET status=$2,version=version+1,state_reason='public read test lifecycle' WHERE id=$1`, id, status); e != nil {
			t.Fatal(e)
		}
	}
	return id, 4
}
func publicManual(t *testing.T, f bettingFixture, id string, version int64, number string, draw rules.Draw, at time.Time) rulebook.DrawResult {
	t.Helper()
	var latestMigration int
	var hasNotificationFacts bool
	if err := f.db.QueryRow(context.Background(), `SELECT
 COALESCE((SELECT max(split_part(name,'_',1)::integer) FROM schema_migrations WHERE split_part(name,'_',1) ~ '^[0-9]+$'),0),
 to_regclass('draw_notification_publications') IS NOT NULL`).Scan(&latestMigration, &hasNotificationFacts); err != nil {
		t.Fatal(err)
	}
	if latestMigration < 70 && !hasNotificationFacts {
		return legacyPublicManual(t, f, id, version, number, draw, at)
	}
	actor := judgeActor(f)
	actor.Roles[0].Permissions = append(actor.Roles[0].Permissions, access.Permission{Resource: "draw", Action: "manual_create", Scope: access.ScopeBrand})
	var result rulebook.DrawResult
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		var e error
		result, e = (rulebook.Store{DB: f.db}).ManualDraw(context.Background(), tx, f.brand, actor, id, version, number, draw, at, "verified archive fixture", policyMeta(actor.ID))
		return e
	})
	return result
}

// legacyPublicManual reproduces the draw-result insert, period lock, and audit
// record used before migration 0070 added draw-notification publication facts.
// Upgrade fixtures use it only while those historical schemas lack the table;
// fully migrated fixtures always exercise rulebook.ManualDraw above.
func legacyPublicManual(t *testing.T, f bettingFixture, periodID string, version int64, number string, draw rules.Draw, drawnAt time.Time) rulebook.DrawResult {
	t.Helper()
	ctx := context.Background()
	actor := judgeActor(f)
	var result rulebook.DrawResult
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		var gameID string
		if err := tx.QueryRow(ctx, `SELECT game_id::text FROM periods WHERE brand_id=$1 AND id=$2`, f.brand, periodID).Scan(&gameID); err != nil {
			return err
		}
		var lockedGame string
		if err := tx.QueryRow(ctx, `SELECT id::text FROM games WHERE brand_id=$1 AND id=$2 FOR UPDATE`, f.brand, gameID).Scan(&lockedGame); err != nil {
			return err
		}
		var status, current, periodNo string
		var periodVersion int64
		var drawAt time.Time
		if err := tx.QueryRow(ctx, `SELECT status,version,coalesce(draw_result_id::text,''),period_no,draw_at FROM periods WHERE brand_id=$1 AND id=$2 FOR UPDATE`, f.brand, periodID).Scan(&status, &periodVersion, &current, &periodNo, &drawAt); err != nil {
			return err
		}
		if periodVersion != version || status != "waiting_draw" || current != "" || periodNo != number {
			return fmt.Errorf("legacy manual draw fixture period state changed: status=%q version=%d period_no=%q draw_result_id=%q", status, periodVersion, periodNo, current)
		}
		var now time.Time
		if err := tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
			return err
		}
		if drawnAt.Before(drawAt) || drawnAt.After(now) {
			return fmt.Errorf("legacy manual draw fixture timestamp outside period window: draw_at=%s drawn_at=%s now=%s", drawAt, drawnAt, now)
		}

		var sourceID string
		err := tx.QueryRow(ctx, `SELECT id::text FROM draw_sources WHERE brand_id=$1 AND game_id=$2 AND type='manual'`, f.brand, gameID).Scan(&sourceID)
		if errors.Is(err, pgx.ErrNoRows) {
			sourceID = ids.New()
			_, err = tx.Exec(ctx, `INSERT INTO draw_sources(id,brand_id,game_id,type) VALUES($1,$2,$3,'manual')`, sourceID, f.brand, gameID)
		}
		if err != nil {
			return err
		}

		normalized := draw
		normalized.Regular = append([]int{}, draw.Regular...)
		normalized.Special = append([]int{}, draw.Special...)
		normalized.Digits = append([]int{}, draw.Digits...)
		if !f.game.Model.Ordered {
			sort.Ints(normalized.Regular)
			sort.Ints(normalized.Special)
		}
		raw, err := json.Marshal(normalized)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(raw)
		id := ids.New()
		var saved []byte
		err = tx.QueryRow(ctx, `INSERT INTO draw_results(id,brand_id,game_id,period_id,source_id,kind,result,result_hash,drawn_at,created_by,corrected_from_id)
VALUES($1,$2,$3,$4,$5,'manual',$6,$7,$8,$9,NULL) RETURNING id::text,brand_id::text,game_id::text,period_id::text,source_id::text,kind,result,result_hash,drawn_at,created_at,coalesce(created_by::text,''),coalesce(corrected_from_id::text,'')`,
			id, f.brand, gameID, periodID, sourceID, raw, hex.EncodeToString(sum[:]), drawnAt.UTC(), actor.ID).Scan(
			&result.ID, &result.BrandID, &result.GameID, &result.PeriodID, &result.SourceID, &result.Kind,
			&saved, &result.ResultHash, &result.DrawnAt, &result.CreatedAt, &result.CreatedBy, &result.CorrectedFromID)
		if err != nil {
			return err
		}
		if err = json.Unmarshal(saved, &result.Result); err != nil {
			return err
		}
		result.DrawnAt = result.DrawnAt.UTC()
		result.CreatedAt = result.CreatedAt.UTC()
		if _, err = tx.Exec(ctx, `UPDATE periods SET draw_result_id=$2,status='drawn',version=version+1,state_reason='result locked: manual',draw_claim_token=NULL,draw_claim_until=NULL,draw_next_poll_at=NULL WHERE id=$1`, periodID, result.ID); err != nil {
			return err
		}
		meta := policyMeta(actor.ID)
		_, err = audit.Append(ctx, tx, audit.Record{
			BrandID: f.brand, ActorType: "admin", ActorID: actor.ID, Action: "draw.lock", ResourceType: "draw_result",
			ResourceID: result.ID, Reason: "verified archive fixture", RequestID: meta.RequestID, IP: meta.IP,
			Before: map[string]any{"period_id": periodID, "period_version": periodVersion, "status": status, "draw_result_id": current},
			After:  result,
		})
		return err
	})
	return result
}
func publicFilter() PublicDrawFilter { return PublicDrawFilter{Limit: 50} }

func TestPublicDrawArchiveIsCurrentTenantScopedAndReadOnly(t *testing.T) {
	f := newBettingFixture(t, storeTestBrand)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	oldID, v := publicTestPeriod(t, f, f.game.ID, "archive-001", 100, now.Add(-2*time.Hour))
	old := publicManual(t, f, oldID, v, "archive-001", rules.Draw{Digits: []int{0, 1, 0}}, now.Add(-2*time.Hour))
	newID, v := publicTestPeriod(t, f, f.game.ID, "archive-002", 101, now.Add(-time.Hour))
	latest := publicManual(t, f, newID, v, "archive-002", rules.Draw{Digits: []int{1, 1, 1}}, now.Add(-time.Hour))
	if _, e := f.db.Exec(ctx, `INSERT INTO periods(id,brand_id,game_id,period_no,sequence,bet_start_at,bet_end_at,draw_at,status) VALUES($1,$2,$3,'future-hidden',102,clock_timestamp()+interval '1 day',clock_timestamp()+interval '2 days',clock_timestamp()+interval '3 days','pending')`, ids.New(), f.brand, f.game.ID); e != nil {
		t.Fatal(e)
	}
	var before []int64
	e := f.db.QueryRow(ctx, `SELECT ARRAY[(SELECT count(*) FROM audit_logs),(SELECT count(*) FROM point_ledger_entries),(SELECT count(*) FROM bet_orders),(SELECT sum(version) FROM periods)]`).Scan(&before)
	if e != nil {
		t.Fatal(e)
	}
	page, e := f.service.PublicDraws(ctx, f.brand, PublicDrawFilter{Limit: 1})
	if e != nil || len(page.Items) != 1 || !page.HasMore || page.Items[0].ID != latest.ID {
		t.Fatalf("first page %+v %v", page, e)
	}
	next, e := f.service.PublicDraws(ctx, f.brand, PublicDrawFilter{Limit: 1, Offset: 1})
	if e != nil || len(next.Items) != 1 || next.HasMore || next.Items[0].ID != old.ID {
		t.Fatalf("second page %+v %v", next, e)
	}
	filtered, e := f.service.PublicDraws(ctx, f.brand, PublicDrawFilter{GameID: f.game.ID, PeriodNo: "archive-001", Limit: 50})
	if e != nil || len(filtered.Items) != 1 || filtered.Items[0].ID != old.ID {
		t.Fatalf("filter %+v %v", filtered, e)
	}
	empty, e := f.service.PublicDraws(ctx, f.brand, PublicDrawFilter{PeriodNo: "does-not-exist", Limit: 50})
	if e != nil || empty.Items == nil || len(empty.Items) != 0 {
		t.Fatal(empty, e)
	}
	detail, e := f.service.PublicDraw(ctx, f.brand, old.ID)
	if e != nil || !reflect.DeepEqual(detail.Item, filtered.Items[0]) {
		t.Fatal(detail, e)
	}
	history, e := f.service.PublicPeriods(ctx, f.brand, f.game.ID, publicFilter())
	if e != nil || len(history.Items) != 3 || history.Items[0].Draw != nil || history.Items[0].Period.ID != f.period.ID {
		t.Fatal(history, e)
	}
	for _, item := range history.Items {
		if item.Period.PeriodNo == "future-hidden" {
			t.Fatal("future reservation leaked")
		}
		if item.Draw != nil && item.Draw.ID != item.Period.DrawResultID {
			t.Fatal("history pointer mismatch")
		}
	}
	for _, read := range []func() error{func() error { _, e := f.service.PublicDraw(ctx, storeTestOtherBrand, old.ID); return e }, func() error {
		_, e := f.service.PublicDraws(ctx, storeTestOtherBrand, PublicDrawFilter{GameID: f.game.ID, Limit: 50})
		return e
	}, func() error {
		_, e := f.service.PublicPeriods(ctx, storeTestOtherBrand, f.game.ID, publicFilter())
		return e
	}} {
		if e := read(); !errors.Is(e, ErrNotFound) {
			t.Fatalf("cross-brand %v", e)
		}
	}
	raw, _ := json.Marshal(page)
	for _, private := range []string{"source_id", "created_by", "corrected_from_id", "result_hash", "claim", "endpoint", "credential", "state_reason", "source_set"} {
		if strings.Contains(string(raw), private) {
			t.Fatalf("private field %s: %s", private, raw)
		}
	}
	var after []int64
	if e = f.db.QueryRow(ctx, `SELECT ARRAY[(SELECT count(*) FROM audit_logs),(SELECT count(*) FROM point_ledger_entries),(SELECT count(*) FROM bet_orders),(SELECT sum(version) FROM periods)]`).Scan(&after); e != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("read mutated %v -> %v %v", before, after, e)
	}
	if _, e = f.db.Exec(ctx, `UPDATE brands SET status='paused' WHERE id=$1`, f.brand); e != nil {
		t.Fatal(e)
	}
	if _, e = f.db.Exec(ctx, `UPDATE games SET status='paused',version=version+1 WHERE id=$1`, f.game.ID); e != nil {
		t.Fatal(e)
	}
	paused, e := f.service.PublicDraws(ctx, f.brand, publicFilter())
	if e != nil || paused.BrandStatus != "paused" || paused.Items[0].Game.Status != "paused" {
		t.Fatal(paused, e)
	}
	if _, e = f.db.Exec(ctx, `UPDATE brands SET status='disabled' WHERE id=$1`, f.brand); e != nil {
		t.Fatal(e)
	}
	if _, e = f.service.PublicDraw(ctx, f.brand, old.ID); !errors.Is(e, ErrNotFound) {
		t.Fatal(e)
	}
}

func TestPublicDrawPreservesAllThreeNumberModels(t *testing.T) {
	for i, tc := range []struct {
		model rules.Model
		draw  rules.Draw
	}{
		{rules.Model{Type: "DIGITS_0_9", Length: 3, AllowRepeat: true, Ordered: true}, rules.Draw{Digits: []int{0, 1, 0}}},
		{rules.Model{Type: "M_SELECT_N", PoolSize: 49, TotalCount: 7, RegularPool: rules.Pool{Min: 1, Max: 49}, SpecialPool: rules.Pool{Min: 1, Max: 49}, RegularCount: 6, SpecialCount: 1}, rules.Draw{Regular: []int{1, 2, 3, 4, 5, 6}, Special: []int{7}}},
		{rules.Model{Type: "X_PLUS_Y", RegularPool: rules.Pool{Min: 1, Max: 49}, SpecialPool: rules.Pool{Min: 1, Max: 49}, RegularCount: 6, SpecialCount: 1}, rules.Draw{Regular: []int{1, 2, 3, 4, 5, 6}, Special: []int{7}}},
	} {
		t.Run(tc.model.Type, func(t *testing.T) {
			f := newBettingFixture(t, storeTestBrand)
			var game rulebook.Game
			actor := bettingRuleActor(f.version.CreatedBy, f.brand)
			bettingTx(t, f.db, func(tx pgx.Tx) error {
				var e error
				game, e = (rulebook.Store{DB: f.db}).CreateGame(context.Background(), tx, f.brand, actor, fmt.Sprintf("public_model_%d", i), "Public numbers", tc.model, "Asia/Manila", "archive model", policyMeta(actor.ID))
				return e
			})
			at := time.Now().UTC().Add(-time.Hour)
			id, v := publicTestPeriod(t, f, game.ID, "number-model", 1, at)
			saved := publicManual(t, f, id, v, "number-model", tc.draw, at)
			out, e := f.service.PublicDraw(context.Background(), f.brand, saved.ID)
			if e != nil {
				t.Fatal(e)
			}
			want := tc.draw
			if want.Regular == nil {
				want.Regular = []int{}
			}
			if want.Special == nil {
				want.Special = []int{}
			}
			if want.Digits == nil {
				want.Digits = []int{}
			}
			if !reflect.DeepEqual(out.Item.Result, want) || out.Item.Game.Timezone != "Asia/Manila" {
				t.Fatal(out)
			}
		})
	}
}

func TestPublicDrawHidesSupersededResultsAndRetainsCancelledWarningState(t *testing.T) {
	f := newBettingFixture(t, storeTestBrand)
	ctx := context.Background()
	at := time.Now().UTC().Add(-time.Hour)
	id, v := publicTestPeriod(t, f, f.game.ID, "external-corrected", 100, at)
	source, oldID := ids.New(), ids.New()
	raw, _ := json.Marshal(rules.Draw{Digits: []int{1, 2, 3}})
	hash := sha256.Sum256(raw)
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		if _, e := tx.Exec(ctx, `INSERT INTO draw_sources(id,brand_id,game_id,type) VALUES($1,$2,$3,'api')`, source, f.brand, f.game.ID); e != nil {
			return e
		}
		if _, e := tx.Exec(ctx, `INSERT INTO draw_results(id,brand_id,game_id,period_id,source_id,kind,result,result_hash,drawn_at) VALUES($1,$2,$3,$4,$5,'api',$6,$7,$8)`, oldID, f.brand, f.game.ID, id, source, raw, hex.EncodeToString(hash[:]), at); e != nil {
			return e
		}
		_, e := tx.Exec(ctx, `UPDATE periods SET status='drawn',version=version+1,state_reason='external fixture result',draw_result_id=$2 WHERE id=$1`, id, oldID)
		return e
	})
	external, e := f.service.PublicDraw(ctx, f.brand, oldID)
	if e != nil || external.Item.Origin != "external" {
		t.Fatal(external, e)
	}
	corrected := publicManual(t, f, id, v+1, "external-corrected", rules.Draw{Digits: []int{3, 2, 1}}, at)
	if _, e = f.service.PublicDraw(ctx, f.brand, oldID); !errors.Is(e, ErrNotFound) {
		t.Fatalf("superseded result published %v", e)
	}
	page, e := f.service.PublicDraws(ctx, f.brand, publicFilter())
	if e != nil || len(page.Items) != 1 || page.Items[0].ID != corrected.ID {
		t.Fatal(page, e)
	}
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		_, e := (f.service).CancelPeriod(ctx, tx, f.brand, periodCancelActor(f), id, CancelPeriodInput{Version: v + 2, Mode: "judged_cancelled", Cause: "invalid_result", Reason: "result invalidated"}, policyMeta(f.version.CreatedBy))
		return e
	})
	page, e = f.service.PublicDraws(ctx, f.brand, publicFilter())
	if e != nil || len(page.Items) != 1 || page.Items[0].Period.Status != "judged_cancelled" || page.Items[0].ID != corrected.ID {
		t.Fatal(page, e)
	}
	history, e := f.service.PublicPeriods(ctx, f.brand, f.game.ID, PublicDrawFilter{PeriodNo: "external-corrected", Limit: 50})
	if e != nil || len(history.Items) != 1 || history.Items[0].Draw == nil || history.Items[0].Period.Status != "judged_cancelled" {
		t.Fatal(history, e)
	}
	for _, bad := range []PublicDrawFilter{{Limit: 0}, {Limit: 101}, {Limit: 50, Offset: -1}, {Limit: 50, GameID: "bad"}, {Limit: 50, PeriodNo: strings.Repeat("中", 27)}, {Limit: 50, PeriodNo: " "}} {
		if _, e = f.service.PublicDraws(ctx, f.brand, bad); !errors.Is(e, ErrInvalid) {
			t.Fatal(bad, e)
		}
	}
}

func TestPublicPeriodHistoryNeverInventsNumbersForWaitingOrCancelledPeriods(t *testing.T) {
	f := newBettingFixture(t, storeTestBrand)
	ctx := context.Background()
	at := time.Now().UTC().Add(-time.Hour)
	id, version := publicTestPeriod(t, f, f.game.ID, "waiting-without-result", 100, at)
	filter := PublicDrawFilter{PeriodNo: "waiting-without-result", Limit: 50}
	before, err := f.service.PublicPeriods(ctx, f.brand, f.game.ID, filter)
	if err != nil || len(before.Items) != 1 || before.Items[0].Period.Status != "waiting_draw" || before.Items[0].Draw != nil {
		t.Fatalf("waiting history %+v %v", before, err)
	}
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		_, err := f.service.CancelPeriod(ctx, tx, f.brand, periodCancelActor(f), id, CancelPeriodInput{Version: version, Mode: "judged_cancelled", Cause: "no_result", Reason: "missing result archive regression"}, policyMeta(f.version.CreatedBy))
		return err
	})
	after, err := f.service.PublicPeriods(ctx, f.brand, f.game.ID, filter)
	if err != nil || len(after.Items) != 1 || after.Items[0].Period.Status != "judged_cancelled" || after.Items[0].Draw != nil || after.Items[0].Period.DrawResultID != "" {
		t.Fatalf("cancelled history %+v %v", after, err)
	}
	results, err := f.service.PublicDraws(ctx, f.brand, filter)
	if err != nil || results.Items == nil || len(results.Items) != 0 {
		t.Fatalf("manufactured result %+v %v", results, err)
	}
}
