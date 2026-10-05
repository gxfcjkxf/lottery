package rulebook

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/drawfeed"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/jackc/pgx/v5"
)

type drawAdapterFunc func(context.Context, drawfeed.Source, drawfeed.Request) (drawfeed.Candidate, error)

func (f drawAdapterFunc) Fetch(ctx context.Context, source drawfeed.Source, request drawfeed.Request) (drawfeed.Candidate, error) {
	return f(ctx, source, request)
}

func prepareDrawCollector(t *testing.T, configCount int) (Store, access.Account, Game, []drawfeed.SourceConfig, Period) {
	t.Helper()
	s, author, _, game, _, _ := fixture(t)
	for i := range author.Roles {
		author.Roles[i].Permissions = append(author.Roles[i].Permissions,
			access.Permission{Resource: "draw_source", Action: "write", Scope: access.ScopeBrand},
			access.Permission{Resource: "draw", Action: "manual_create", Scope: access.ScopeBrand},
		)
	}
	configs := make([]drawfeed.SourceConfig, configCount)
	for i := range configs {
		configs[i] = drawfeed.SourceConfig{ID: ids.New(), Name: "source", Type: "api", Priority: i + 1, Enabled: true, Endpoint: "https://feeds.example.com/draw"}
	}
	var set SourceSet
	transact(t, s.DB, func(tx pgx.Tx) error {
		var err error
		set, err = s.WriteSources(context.Background(), tx, brand, author, game.ID, game.Version, configs, "configure draw collector", points.Metadata{})
		return err
	})
	game, err := scanGame(s.DB.QueryRow(context.Background(), `SELECT `+gameFields+` FROM games WHERE id=$1`, game.ID))
	if err != nil {
		t.Fatal(err)
	}
	var now time.Time
	if err := s.DB.QueryRow(context.Background(), `SELECT clock_timestamp()`).Scan(&now); err != nil {
		t.Fatal(err)
	}
	period, err := scanPeriod(s.DB.QueryRow(context.Background(), `INSERT INTO periods
		(id,brand_id,game_id,period_no,sequence,bet_start_at,bet_end_at,draw_at,status)
		VALUES($1,$2,$3,$4,1,$5,$6,$7,'waiting_draw') RETURNING `+periodFields,
		ids.New(), brand, game.ID, "collector-"+ids.New()[:8], now.Add(-3*time.Minute), now.Add(-2*time.Minute), now.Add(-time.Minute)))
	if err != nil {
		t.Fatal(err)
	}
	_ = set
	return s, author, game, configs, period
}

func drawCandidate(t *testing.T, s Store, period Period, draw rules.Draw) drawfeed.Candidate {
	t.Helper()
	var now time.Time
	if err := s.DB.QueryRow(context.Background(), `SELECT clock_timestamp()`).Scan(&now); err != nil {
		t.Fatal(err)
	}
	return drawfeed.Candidate{PeriodNo: period.PeriodNo, Draw: draw, DrawnAt: now.Add(-time.Second)}
}

func resolverFor(adapter drawfeed.Adapter) drawfeed.Resolver {
	return drawfeed.Resolver{Adapters: map[string]drawfeed.Adapter{"api": adapter}, Timeout: 2 * time.Second}
}

func TestCollectDrawsConcurrentClaimsOnlyFetchOnce(t *testing.T) {
	s, _, _, _, period := prepareDrawCollector(t, 1)
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	resolver := resolverFor(drawAdapterFunc(func(ctx context.Context, _ drawfeed.Source, request drawfeed.Request) (drawfeed.Candidate, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		select {
		case <-release:
		case <-ctx.Done():
			return drawfeed.Candidate{}, ctx.Err()
		}
		return drawCandidate(t, s, period, rules.Draw{Digits: []int{1, 2, 3}}), nil
	}))
	results := make(chan error, 2)
	for range 2 {
		go func() {
			_, err := s.CollectDraws(context.Background(), resolver)
			results <- err
		}()
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("fetch did not start")
	}
	close(release)
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("fetches=%d, want exactly one", got)
	}
	var count int
	if err := s.DB.QueryRow(context.Background(), `SELECT count(*) FROM draw_results WHERE period_id=$1`, period.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("result count=%d err=%v", count, err)
	}
}

func TestCollectDrawsManualSelectionWinsInflightAndStopsFallback(t *testing.T) {
	s, author, game, configs, period := prepareDrawCollector(t, 2)
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	resolver := drawfeed.Resolver{Timeout: 2 * time.Second, Adapters: map[string]drawfeed.Adapter{"api": drawAdapterFunc(func(ctx context.Context, _ drawfeed.Source, _ drawfeed.Request) (drawfeed.Candidate, error) {
		calls.Add(1)
		close(started)
		select {
		case <-release:
		case <-ctx.Done():
			return drawfeed.Candidate{}, ctx.Err()
		}
		return drawCandidate(t, s, period, rules.Draw{Digits: []int{1, 2, 3}}), nil
	})}}
	resultCh := make(chan error, 1)
	go func() { _, err := s.CollectDraws(context.Background(), resolver); resultCh <- err }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("fetch did not start")
	}
	manualCandidate := drawCandidate(t, s, period, rules.Draw{Digits: []int{4, 5, 6}})
	transact(t, s.DB, func(tx pgx.Tx) error {
		_, err := s.ManualDraw(context.Background(), tx, brand, author, period.ID, period.Version, period.PeriodNo, manualCandidate.Draw, manualCandidate.DrawnAt, "manual draw wins", points.Metadata{})
		return err
	})
	close(release)
	if err := <-resultCh; err != nil {
		t.Fatal(err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("source fetches=%d, want fallback stopped after manual result", got)
	}
	var kind string
	if err := s.DB.QueryRow(context.Background(), `SELECT kind FROM draw_results WHERE period_id=$1`, period.ID).Scan(&kind); err != nil || kind != "manual" {
		t.Fatalf("persisted result kind=%q err=%v", kind, err)
	}
	var status string
	if err := s.DB.QueryRow(context.Background(), `SELECT status FROM draw_attempt_batches WHERE period_id=$1`, period.ID).Scan(&status); err != nil || status != "discarded" {
		t.Fatalf("attempt batch status=%q err=%v", status, err)
	}
	_ = game
	_ = configs
}

func TestCollectDrawsDiscardsReplacedConfiguration(t *testing.T) {
	s, author, game, configs, period := prepareDrawCollector(t, 2)
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	resolver := drawfeed.Resolver{Timeout: 2 * time.Second, Adapters: map[string]drawfeed.Adapter{"api": drawAdapterFunc(func(ctx context.Context, _ drawfeed.Source, _ drawfeed.Request) (drawfeed.Candidate, error) {
		calls.Add(1)
		close(started)
		select {
		case <-release:
		case <-ctx.Done():
			return drawfeed.Candidate{}, ctx.Err()
		}
		return drawCandidate(t, s, period, rules.Draw{Digits: []int{1, 2, 3}}), nil
	})}}
	resultCh := make(chan error, 1)
	go func() { _, err := s.CollectDraws(context.Background(), resolver); resultCh <- err }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("fetch did not start")
	}
	current, err := scanGame(s.DB.QueryRow(context.Background(), `SELECT `+gameFields+` FROM games WHERE id=$1`, game.ID))
	if err != nil {
		t.Fatal(err)
	}
	replacement := append([]drawfeed.SourceConfig(nil), configs...)
	replacement[0].Name = "replacement"
	transact(t, s.DB, func(tx pgx.Tx) error {
		_, err := s.WriteSources(context.Background(), tx, brand, author, game.ID, current.Version, replacement, "replace during fetch", points.Metadata{})
		return err
	})
	close(release)
	if err := <-resultCh; err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("fetches=%d, want fallback stopped after source set replacement", calls.Load())
	}
	var status string
	if err := s.DB.QueryRow(context.Background(), `SELECT status FROM draw_attempt_batches WHERE period_id=$1`, period.ID).Scan(&status); err != nil || status != "discarded" {
		t.Fatalf("attempt status=%q err=%v", status, err)
	}
}

func TestCollectDrawsValidationFailureBackoffAndImmutableEvidence(t *testing.T) {
	t.Run("no data backs off", func(t *testing.T) {
		s, _, _, _, period := prepareDrawCollector(t, 1)
		resolver := resolverFor(drawAdapterFunc(func(context.Context, drawfeed.Source, drawfeed.Request) (drawfeed.Candidate, error) {
			return drawfeed.Candidate{}, drawfeed.ErrNoData
		}))
		if count, err := s.CollectDraws(context.Background(), resolver); err != nil || count != 0 {
			t.Fatalf("CollectDraws count=%d err=%v, want persisted no_data outcome", count, err)
		}
		var status string
		var next *time.Time
		if err := s.DB.QueryRow(context.Background(), `SELECT b.status,p.draw_next_poll_at FROM draw_attempt_batches b JOIN periods p ON p.id=b.period_id WHERE b.period_id=$1`, period.ID).Scan(&status, &next); err != nil || status != "no_data" || next == nil {
			t.Fatalf("attempt status=%q next_poll=%v err=%v", status, next, err)
		}
		if count, err := s.CollectDraws(context.Background(), resolver); err != nil || count != 0 {
			t.Fatalf("immediate retry count=%d err=%v", count, err)
		}
	})
	t.Run("invalid timestamp falls back to valid source", func(t *testing.T) {
		s, _, _, _, current := prepareDrawCollector(t, 2)
		resolver := drawfeed.Resolver{Timeout: 2 * time.Second, Adapters: map[string]drawfeed.Adapter{"api": drawAdapterFunc(func(_ context.Context, source drawfeed.Source, _ drawfeed.Request) (drawfeed.Candidate, error) {
			candidate := drawCandidate(t, s, current, rules.Draw{Digits: []int{1, 2, 3}})
			if source.Priority == 1 {
				candidate.DrawnAt = current.DrawAt.Add(-time.Second)
			}
			return candidate, nil
		})}}
		if count, err := s.CollectDraws(context.Background(), resolver); err != nil || count != 1 {
			t.Fatalf("CollectDraws count=%d err=%v", count, err)
		}
		var firstCode, secondCode, status string
		if err := s.DB.QueryRow(context.Background(), `SELECT status,attempts->0->>'code',attempts->1->>'code' FROM draw_attempt_batches WHERE period_id=$1`, current.ID).Scan(&status, &firstCode, &secondCode); err != nil || status != "accepted" || firstCode != "invalid_candidate" || secondCode != "ok" {
			t.Fatalf("status=%q codes=(%q,%q) err=%v", status, firstCode, secondCode, err)
		}
	})
	t.Run("previous draw is rejected", func(t *testing.T) {
		s, author, game, _, current := prepareDrawCollector(t, 1)
		previous, err := scanPeriod(s.DB.QueryRow(context.Background(), `INSERT INTO periods
			(id,brand_id,game_id,period_no,sequence,bet_start_at,bet_end_at,draw_at,status)
			VALUES($1,$2,$3,'collector-previous',2,$4,$5,$6,'waiting_draw') RETURNING `+periodFields,
			ids.New(), brand, game.ID, current.DrawAt.Add(-time.Hour-2*time.Minute), current.DrawAt.Add(-time.Hour-time.Minute), current.DrawAt.Add(-time.Hour)))
		if err != nil {
			t.Fatal(err)
		}
		previousCandidate := drawCandidate(t, s, previous, rules.Draw{Digits: []int{1, 2, 3}})
		transact(t, s.DB, func(tx pgx.Tx) error {
			_, err := s.ManualDraw(context.Background(), tx, brand, author, previous.ID, previous.Version, previous.PeriodNo, previousCandidate.Draw, previousCandidate.DrawnAt, "seed previous", points.Metadata{})
			return err
		})
		resolver := resolverFor(drawAdapterFunc(func(context.Context, drawfeed.Source, drawfeed.Request) (drawfeed.Candidate, error) {
			return drawfeed.Candidate{PeriodNo: current.PeriodNo, Draw: rules.Draw{Digits: []int{1, 2, 3}}, DrawnAt: current.DrawAt.Add(time.Second)}, nil
		}))
		if _, err := s.CollectDraws(context.Background(), resolver); err != nil {
			t.Fatalf("classified abnormal feed outcome leaked as operational error: %v", err)
		}
		var status, code string
		if err := s.DB.QueryRow(context.Background(), `SELECT status,attempts->0->>'code' FROM draw_attempt_batches WHERE period_id=$1`, current.ID).Scan(&status, &code); err != nil || status != "failed" || code != "invalid_candidate" {
			t.Fatalf("attempt status=%q code=%q err=%v", status, code, err)
		}
		var count int
		if err := s.DB.QueryRow(context.Background(), `SELECT count(*) FROM draw_results WHERE period_id=$1`, current.ID).Scan(&count); err != nil || count != 0 {
			t.Fatalf("result count=%d err=%v", count, err)
		}
	})
	t.Run("accepted evidence is immutable", func(t *testing.T) {
		s, _, _, _, period := prepareDrawCollector(t, 1)
		resolver := resolverFor(drawAdapterFunc(func(context.Context, drawfeed.Source, drawfeed.Request) (drawfeed.Candidate, error) {
			return drawCandidate(t, s, period, rules.Draw{Digits: []int{7, 8, 9}}), nil
		}))
		if count, err := s.CollectDraws(context.Background(), resolver); err != nil || count != 1 {
			t.Fatalf("CollectDraws count=%d err=%v", count, err)
		}
		for _, query := range []string{
			`UPDATE draw_results SET result_hash=result_hash WHERE period_id=$1`,
			`UPDATE draw_attempt_batches SET status=status WHERE period_id=$1`,
		} {
			if _, err := s.DB.Exec(context.Background(), query, period.ID); err == nil {
				t.Fatalf("immutable update unexpectedly succeeded: %s", query)
			}
		}
	})
}
