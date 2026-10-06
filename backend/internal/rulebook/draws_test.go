package rulebook

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/betting"
	"github.com/gxfcjkxf/lottery/backend/internal/drawfeed"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/jackc/pgx/v5"
)

func drawActor(id string) access.Account {
	a := actor(id)
	for i := range a.Roles {
		a.Roles[i].Permissions = append(a.Roles[i].Permissions,
			access.Permission{Resource: "draw_source", Action: "write", Scope: access.ScopeBrand},
			access.Permission{Resource: "draw", Action: "manual_create", Scope: access.ScopeBrand},
		)
	}
	return a
}

func waitingDrawPeriod(t *testing.T, s Store, g Game, no string, sequence int64, drawAt time.Time, status string) Period {
	t.Helper()
	var p Period
	transact(t, s.DB, func(tx pgx.Tx) error {
		var err error
		p, err = scanPeriod(tx.QueryRow(context.Background(), `INSERT INTO periods
			(id,brand_id,game_id,period_no,sequence,bet_start_at,bet_end_at,draw_at,status)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING `+periodFields,
			ids.New(), brand, g.ID, no, sequence, drawAt.Add(-2*time.Hour), drawAt.Add(-time.Hour), drawAt, status))
		return err
	})
	return p
}

func digitDraw(a, b, c int) rules.Draw { return rules.Draw{Digits: []int{a, b, c}} }

func drawCount(t *testing.T, s Store, query string, args ...any) int {
	t.Helper()
	var count int
	if err := s.DB.QueryRow(context.Background(), query, args...).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func drawDatabaseNow(t *testing.T, s Store) time.Time {
	t.Helper()
	var now time.Time
	if err := s.DB.QueryRow(context.Background(), `SELECT clock_timestamp()`).Scan(&now); err != nil {
		t.Fatal(err)
	}
	return now
}

func writeDrawSources(t *testing.T, s Store, a access.Account, g Game, configs []drawfeed.SourceConfig) SourceSet {
	t.Helper()
	var result SourceSet
	transact(t, s.DB, func(tx pgx.Tx) error {
		var err error
		result, err = s.WriteSources(context.Background(), tx, brand, a, g.ID, g.Version, configs, "configure draw sources", points.Metadata{})
		return err
	})
	return result
}

func sourceConfig(id, kind string, priority int, enabled bool) drawfeed.SourceConfig {
	c := drawfeed.SourceConfig{ID: id, Name: "Source " + kind, Type: kind, Priority: priority, Enabled: enabled, Endpoint: "https://example.com/results"}
	if kind == "dom" {
		c.Selector = "#result"
	}
	return c
}

func seedExternalDraw(t *testing.T, s Store, p Period, g Game, result rules.Draw, sourceID string) DrawResult {
	t.Helper()
	ctx := context.Background()
	var got DrawResult
	transact(t, s.DB, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO draw_sources(id,brand_id,game_id,type) VALUES($1,$2,$3,'api')`, sourceID, brand, g.ID); err != nil {
			return err
		}
		lockedGame, lockedPeriod, err := lockDrawPeriod(ctx, tx, brand, p.ID)
		if err != nil {
			return err
		}
		got, err = appendDrawResult(ctx, tx, lockedGame, lockedPeriod, sourceID, "api", drawfeed.Candidate{
			PeriodNo: p.PeriodNo, Draw: result, DrawnAt: p.DrawAt.Add(time.Minute),
		}, "", "seed external draw", points.Metadata{})
		return err
	})
	return got
}

func TestDrawSourcesRevisionValidationAndStableIdentities(t *testing.T) {
	s, a0, _, g, _, _ := fixture(t)
	a := drawActor(a0.ID)
	ctx := context.Background()

	first := writeDrawSources(t, s, a, g, nil)
	if first.Revision != 1 || first.GameVersion != g.Version+1 || first.Sources == nil || len(first.Sources) != 0 {
		t.Fatalf("empty source revision = %+v", first)
	}
	var raw string
	if err := s.DB.QueryRow(ctx, `SELECT jsonb_typeof(sources) FROM draw_source_sets WHERE id=$1`, first.ID).Scan(&raw); err != nil || raw != "array" {
		t.Fatalf("empty sources JSON type=%q, err=%v", raw, err)
	}
	g.Version = first.GameVersion
	disabled := sourceConfig("0199a000-0000-7000-8000-000000000011", "api", 1, false)
	second := writeDrawSources(t, s, a, g, []drawfeed.SourceConfig{disabled})
	if second.Revision != 2 || second.GameVersion != first.GameVersion+1 || len(second.Sources) != 1 || second.Sources[0].Enabled {
		t.Fatalf("disabled source was rejected or enabled: %+v", second)
	}
	current, err := s.Sources(ctx, brand, g.ID)
	if err != nil || current.ID != second.ID || current.Revision != 2 || current.Sources[0].ID != disabled.ID {
		t.Fatalf("current sources = %+v, err=%v", current, err)
	}
	if _, err = s.Sources(ctx, "0199a000-0000-7000-8000-000000000002", g.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-brand source read error=%v", err)
	}

	other := actor(a.ID)
	for _, tc := range []struct {
		name, game string
		version    int64
		configs    []drawfeed.SourceConfig
		want       error
	}{
		{"stale game version", g.ID, first.GameVersion, []drawfeed.SourceConfig{disabled}, ErrVersion},
		{"same identity different type", g.ID, second.GameVersion, []drawfeed.SourceConfig{sourceConfig(disabled.ID, "dom", 1, true)}, ErrInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tx, err := s.DB.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			_, err = s.WriteSources(ctx, tx, brand, a, tc.game, tc.version, tc.configs, "rejected source update", points.Metadata{})
			_ = tx.Rollback(ctx)
			if !errors.Is(err, tc.want) {
				t.Fatalf("WriteSources error=%v, want %v", err, tc.want)
			}
		})
	}
	// Reusing an immutable ID in a second game cannot transfer its ownership.
	var g2 Game
	transact(t, s.DB, func(tx pgx.Tx) error {
		var err error
		g2, err = s.CreateGame(ctx, tx, brand, a, "daily_four", "Daily Four", g.Model, g.Timezone, "create second game", points.Metadata{})
		return err
	})
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.WriteSources(ctx, tx, brand, a, g2.ID, g2.Version, []drawfeed.SourceConfig{disabled}, "transfer source", points.Metadata{})
	_ = tx.Rollback(ctx)
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("cross-game source identity error=%v", err)
	}
	if n := drawCount(t, s, `SELECT count(*) FROM draw_sources WHERE id=$1`, disabled.ID); n != 1 {
		t.Fatalf("source identity rows=%d", n)
	}

	noWrite := other
	tx, err = s.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.WriteSources(ctx, tx, brand, noWrite, g.ID, second.GameVersion, nil, "no permission", points.Metadata{})
	_ = tx.Rollback(ctx)
	if !errors.Is(err, ErrDenied) {
		t.Fatalf("missing source permission error=%v", err)
	}
	super := a
	super.SuperAdmin = true
	tx, err = s.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.WriteSources(ctx, tx, brand, super, g.ID, second.GameVersion, nil, "super write", points.Metadata{})
	_ = tx.Rollback(ctx)
	if !errors.Is(err, ErrDenied) {
		t.Fatalf("super-admin source write error=%v", err)
	}
}

func TestManualDrawValidatesPeriodResultAndPreviousChronologicalDraw(t *testing.T) {
	s, a0, _, g, _, _ := fixture(t)
	a := drawActor(a0.ID)
	ctx := context.Background()
	now := drawDatabaseNow(t, s)
	past := now.Add(-3 * time.Hour)
	p := waitingDrawPeriod(t, s, g, "20261006001", 1, past, "waiting_draw")
	var result DrawResult
	transact(t, s.DB, func(tx pgx.Tx) error {
		var err error
		result, err = s.ManualDraw(ctx, tx, brand, a, p.ID, p.Version, p.PeriodNo, digitDraw(1, 2, 1), now.Add(-time.Minute), "manual result", points.Metadata{RequestID: "manual-draw"})
		return err
	})
	if result.Kind != "manual" || result.CreatedBy != a.ID || result.PeriodID != p.ID || result.CorrectedFromID != "" || result.ResultHash == "" {
		t.Fatalf("manual result = %+v", result)
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.ManualDraw(ctx, tx, brand, a, p.ID, p.Version+1, p.PeriodNo, digitDraw(7, 8, 9), now.Add(-time.Minute), "overwrite manual result", points.Metadata{})
	_ = tx.Rollback(ctx)
	if !errors.Is(err, ErrState) {
		t.Fatalf("manual result overwrite error=%v", err)
	}
	if drawCount(t, s, `SELECT count(*) FROM draw_sources WHERE game_id=$1 AND type='manual'`, g.ID) != 1 {
		t.Fatal("manual source identity not created exactly once")
	}

	// The earlier sequence has a later draw time than the initial period, so
	// chronological order chooses this result even though sequence order does not.
	older := waitingDrawPeriod(t, s, g, "20261006002", 99, past.Add(30*time.Minute), "waiting_draw")
	seedExternalDraw(t, s, older, g, digitDraw(3, 2, 1), ids.New())
	chronological := waitingDrawPeriod(t, s, g, "20261006003", 2, past.Add(time.Hour), "waiting_draw")
	tx, err = s.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.ManualDraw(ctx, tx, brand, a, chronological.ID, chronological.Version, chronological.PeriodNo, digitDraw(3, 2, 1), now.Add(-time.Minute), "repeat", points.Metadata{})
	_ = tx.Rollback(ctx)
	if !errors.Is(err, ErrDrawAbnormal) {
		t.Fatalf("same chronological result error=%v", err)
	}

	invalids := []struct {
		name     string
		period   Period
		version  int64
		periodNo string
		result   rules.Draw
		drawnAt  time.Time
		want     error
	}{
		{"bad period number", waitingDrawPeriod(t, s, g, "20261006004", 3, past, "waiting_draw"), 1, "wrong", digitDraw(2, 3, 4), now.Add(-time.Minute), ErrInvalid},
		{"bad numbers", waitingDrawPeriod(t, s, g, "20261006005", 4, past, "waiting_draw"), 1, "20261006005", digitDraw(10, 2, 4), now.Add(-time.Minute), ErrInvalid},
		{"future timestamp", waitingDrawPeriod(t, s, g, "20261006006", 5, past, "waiting_draw"), 1, "20261006006", digitDraw(2, 3, 4), now.Add(time.Hour), ErrInvalid},
		{"before draw time", waitingDrawPeriod(t, s, g, "20261006007", 6, past, "waiting_draw"), 1, "20261006007", digitDraw(2, 3, 4), past.Add(-time.Second), ErrInvalid},
		{"stale period version", waitingDrawPeriod(t, s, g, "20261006008", 7, past, "waiting_draw"), 8, "20261006008", digitDraw(2, 3, 4), now.Add(-time.Minute), ErrVersion},
	}
	for _, tc := range invalids {
		t.Run(tc.name, func(t *testing.T) {
			tx, err := s.DB.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			_, err = s.ManualDraw(ctx, tx, brand, a, tc.period.ID, tc.version, tc.periodNo, tc.result, tc.drawnAt, "invalid result", points.Metadata{})
			_ = tx.Rollback(ctx)
			if !errors.Is(err, tc.want) {
				t.Fatalf("ManualDraw error=%v, want %v", err, tc.want)
			}
		})
	}
	for i, status := range []string{"pending", "closed"} {
		period := waitingDrawPeriod(t, s, g, "2026100601"+string(rune('0'+i)), int64(20+i), past, status)
		tx, err := s.DB.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		_, err = s.ManualDraw(ctx, tx, brand, a, period.ID, period.Version, period.PeriodNo, digitDraw(4, 5, 6), now.Add(-time.Minute), "wrong state", points.Metadata{})
		_ = tx.Rollback(ctx)
		if !errors.Is(err, ErrState) {
			t.Fatalf("manual draw in %s error=%v", status, err)
		}
	}
	settling := waitingDrawPeriod(t, s, g, "20261006012", 22, past, "waiting_draw")
	transact(t, s.DB, func(tx pgx.Tx) error {
		if _, err := s.ManualDraw(ctx, tx, brand, a, settling.ID, settling.Version, settling.PeriodNo, digitDraw(4, 5, 6), now.Add(-time.Minute), "prepare settling period", points.Metadata{}); err != nil {
			return err
		}
		return nil
	})
	settler := a
	settler.Roles[0].Permissions = append(settler.Roles[0].Permissions, access.Permission{Resource: "settlement_policy", Action: "write", Scope: access.ScopeBrand}, access.Permission{Resource: "settlement", Action: "run", Scope: access.ScopeBrand})
	bs := betting.Service{DB: s.DB}
	mode := "manual"
	meta := points.Metadata{ActorType: "admin", ActorID: a.ID, RequestID: ids.New()}
	transact(t, s.DB, func(tx pgx.Tx) error {
		policy, e := bs.SaveSettlementPolicy(ctx, tx, brand, settler, betting.SettlementPolicyInput{Version: 1, Mode: &mode, Reason: "explicit draw test settlement"}, meta)
		if e != nil {
			return e
		}
		c, e := bs.PeriodSettlementContext(ctx, brand, settling.ID)
		if e != nil {
			return e
		}
		_, e = bs.StartSettlement(ctx, tx, brand, settler, settling.ID, betting.SettlementStartInput{Version: c.PeriodVersion, PolicyVersion: policy.Version, DrawResultID: *c.DrawResultID, Reason: "actual settlement admission"}, meta)
		return e
	})
	tx, err = s.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.ManualDraw(ctx, tx, brand, a, settling.ID, settling.Version+2, settling.PeriodNo, digitDraw(7, 8, 9), now.Add(-time.Minute), "draw after settlement started", points.Metadata{})
	_ = tx.Rollback(ctx)
	if !errors.Is(err, ErrState) {
		t.Fatalf("manual draw in settling state error=%v", err)
	}
}

func TestManualCorrectionHistoryImmutabilityAndRollback(t *testing.T) {
	s, a0, _, g, _, _ := fixture(t)
	a := drawActor(a0.ID)
	ctx := context.Background()
	now := drawDatabaseNow(t, s)
	p := waitingDrawPeriod(t, s, g, "20261006021", 1, now.Add(-2*time.Hour), "waiting_draw")
	external := seedExternalDraw(t, s, p, g, digitDraw(1, 2, 3), ids.New())
	var corrected DrawResult
	transact(t, s.DB, func(tx pgx.Tx) error {
		var err error
		corrected, err = s.ManualDraw(ctx, tx, brand, a, p.ID, p.Version+1, p.PeriodNo, digitDraw(3, 2, 1), now.Add(-time.Minute), "correct external result", points.Metadata{})
		return err
	})
	if corrected.CorrectedFromID != external.ID || corrected.Kind != "manual" {
		t.Fatalf("corrected result=%+v, original=%+v", corrected, external)
	}
	if n := drawCount(t, s, `SELECT count(*) FROM draw_results WHERE period_id=$1`, p.ID); n != 2 {
		t.Fatalf("correction lost result history: %d", n)
	}
	attemptSource := sourceConfig(ids.New(), "api", 1, true)
	set := writeDrawSources(t, s, a, g, []drawfeed.SourceConfig{attemptSource})
	transact(t, s.DB, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO draw_attempt_batches(id,brand_id,game_id,period_id,source_set_id,observed_period_version,status,attempts) VALUES($1,$2,$3,$4,$5,1,'failed',jsonb_build_array(jsonb_build_object('source_id',$6::text,'status','failed','code','timeout')))`, ids.New(), brand, g.ID, p.ID, set.ID, attemptSource.ID)
		return err
	})

	// The append transaction rolls back both result and audit evidence together.
	rollbackPeriod := waitingDrawPeriod(t, s, g, "20261006022", 2, now.Add(-time.Hour), "waiting_draw")
	beforeAudits := drawCount(t, s, `SELECT count(*) FROM audit_logs WHERE action='draw.lock'`)
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.ManualDraw(ctx, tx, brand, a, rollbackPeriod.ID, rollbackPeriod.Version, rollbackPeriod.PeriodNo, digitDraw(4, 5, 6), now.Add(-time.Minute), "rollback", points.Metadata{})
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if drawCount(t, s, `SELECT count(*) FROM draw_results WHERE period_id=$1`, rollbackPeriod.ID) != 0 || drawCount(t, s, `SELECT count(*) FROM audit_logs WHERE action='draw.lock'`) != beforeAudits {
		t.Fatal("rolled-back draw left result or audit rows")
	}

	for _, statement := range []struct {
		name, sql string
		args      []any
	}{
		{"result update", `UPDATE draw_results SET result='{"digits":[9,9,9]}' WHERE id=$1`, []any{external.ID}},
		{"result delete", `DELETE FROM draw_results WHERE id=$1`, []any{external.ID}},
		{"source update", `UPDATE draw_sources SET type='dom' WHERE id=$1`, []any{external.SourceID}},
		{"source delete", `DELETE FROM draw_sources WHERE id=$1`, []any{external.SourceID}},
	} {
		t.Run(statement.name, func(t *testing.T) {
			tx, err := s.DB.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			_, err = tx.Exec(ctx, statement.sql, statement.args...)
			_ = tx.Rollback(ctx)
			if err == nil {
				t.Fatalf("immutable mutation accepted: %s", statement.name)
			}
		})
	}
	for _, query := range []string{
		`UPDATE draw_attempt_batches SET status='accepted' WHERE period_id=$1`,
		`DELETE FROM draw_attempt_batches WHERE period_id=$1`,
	} {
		tx, err := s.DB.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		_, err = tx.Exec(ctx, query, p.ID)
		_ = tx.Rollback(ctx)
		if err == nil {
			t.Fatalf("attempt evidence mutation accepted: %q", query)
		}
	}
	// The period guard rejects pointer clearing and a fabricated version jump.
	for _, query := range []string{
		`UPDATE periods SET draw_result_id=NULL,version=version+1 WHERE id=$1`,
		`UPDATE periods SET draw_result_id=NULL,version=version+7 WHERE id=$1`,
	} {
		tx, err := s.DB.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		_, err = tx.Exec(ctx, query, p.ID)
		_ = tx.Rollback(ctx)
		if err == nil {
			t.Fatalf("period guard accepted mutation %q", query)
		}
	}
	if drawCount(t, s, `SELECT count(*) FROM audit_logs WHERE action='draw.lock' AND resource_id=$1`, corrected.ID) != 1 {
		t.Fatal("successful corrected draw audit is missing")
	}
}

func TestDrawHistoryPaginationAttemptsAndBrandBoundary(t *testing.T) {
	s, a0, _, g, _, _ := fixture(t)
	a := drawActor(a0.ID)
	ctx := context.Background()
	now := drawDatabaseNow(t, s)
	p := waitingDrawPeriod(t, s, g, "20261006031", 1, now.Add(-2*time.Hour), "waiting_draw")
	external := seedExternalDraw(t, s, p, g, digitDraw(1, 2, 3), ids.New())
	var correction DrawResult
	transact(t, s.DB, func(tx pgx.Tx) error {
		var err error
		correction, err = s.ManualDraw(ctx, tx, brand, a, p.ID, p.Version+1, p.PeriodNo, digitDraw(4, 5, 6), now.Add(-time.Minute), "correct", points.Metadata{})
		return err
	})
	config := sourceConfig("0199a000-0000-7000-8000-000000000031", "api", 1, true)
	set := writeDrawSources(t, s, a, g, []drawfeed.SourceConfig{config})
	transact(t, s.DB, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO draw_attempt_batches(id,brand_id,game_id,period_id,source_set_id,observed_period_version,status,attempts) VALUES($1,$2,$3,$4,$5,$6,'failed',jsonb_build_array(jsonb_build_object('source_id',$7::text,'status','failed','code','timeout')))`, ids.New(), brand, g.ID, p.ID, set.ID, p.Version+2, config.ID)
		return err
	})
	first, err := s.Draw(ctx, brand, p.ID, 1, 0)
	if err != nil || first.Current == nil || first.Current.ID != correction.ID || len(first.History) != 1 || first.History[0].ID != correction.ID || first.Attempts == nil || first.History == nil {
		t.Fatalf("first draw history=%+v, err=%v", first, err)
	}
	second, err := s.Draw(ctx, brand, p.ID, 1, 1)
	if err != nil || second.Current == nil || second.Current.ID != correction.ID || len(second.History) != 1 || second.History[0].ID != external.ID || len(second.Attempts) != 0 {
		t.Fatalf("second draw history=%+v, err=%v", second, err)
	}
	foreign, err := s.Draw(ctx, "0199a000-0000-7000-8000-000000000002", p.ID, 10, 0)
	if !errors.Is(err, ErrNotFound) || foreign.History == nil || foreign.Attempts == nil {
		t.Fatalf("cross-brand draw=%+v, err=%v", foreign, err)
	}
	if _, err = s.Draw(ctx, brand, p.ID, 0, 0); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid history limit error=%v", err)
	}
}

func TestConcurrentManualDrawHasOneOptimisticVersionWinner(t *testing.T) {
	s, a0, _, g, _, _ := fixture(t)
	a := drawActor(a0.ID)
	now := drawDatabaseNow(t, s)
	p := waitingDrawPeriod(t, s, g, "20261006041", 1, now.Add(-time.Hour), "waiting_draw")
	start := make(chan struct{})
	type answer struct {
		result DrawResult
		err    error
	}
	answers := make(chan answer, 2)
	var wg sync.WaitGroup
	for _, digits := range [][]int{{1, 2, 3}, {4, 5, 6}} {
		wg.Add(1)
		go func(d []int) {
			defer wg.Done()
			<-start
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			tx, err := s.DB.Begin(ctx)
			if err != nil {
				answers <- answer{err: err}
				return
			}
			defer tx.Rollback(context.Background())
			result, err := s.ManualDraw(ctx, tx, brand, a, p.ID, p.Version, p.PeriodNo, rules.Draw{Digits: d}, now.Add(-time.Minute), "concurrent draw", points.Metadata{})
			if err == nil {
				err = tx.Commit(ctx)
			}
			answers <- answer{result: result, err: err}
		}(digits)
	}
	close(start)
	wg.Wait()
	close(answers)
	wins, losses := 0, 0
	for got := range answers {
		if got.err == nil {
			wins++
		} else if errors.Is(got.err, ErrVersion) || errors.Is(got.err, ErrState) {
			losses++
		} else {
			t.Fatalf("unexpected concurrent manual draw error=%v", got.err)
		}
	}
	if wins != 1 || losses != 1 {
		t.Fatalf("concurrent manual draw wins=%d losses=%d", wins, losses)
	}
	current, err := scanPeriod(s.DB.QueryRow(context.Background(), `SELECT `+periodFields+` FROM periods WHERE brand_id=$1 AND id=$2`, brand, p.ID))
	if err != nil || current.Status != "drawn" || current.Version != p.Version+1 {
		t.Fatalf("period after concurrent draw=%+v, err=%v", current, err)
	}
	if drawCount(t, s, `SELECT count(*) FROM draw_results WHERE period_id=$1`, p.ID) != 1 || drawCount(t, s, `SELECT count(*) FROM audit_logs WHERE action='draw.lock' AND resource_type='draw_result' AND resource_id IN (SELECT id FROM draw_results WHERE period_id=$1)`, p.ID) != 1 {
		t.Fatal("concurrent manual draw left duplicate result or audit")
	}
}
