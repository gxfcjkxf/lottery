//go:build capacity && !windows

package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/betting"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rulebook"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/jackc/pgx/v5"
)

type capacitySettlementFixture struct {
	Jobs     []betting.SettlementJob
	OrderIDs []string
}

func prepareCapacitySettlement(t *testing.T, f capacityFixture) capacitySettlementFixture {
	t.Helper()
	ctx := context.Background()
	inputs := make(map[string]betting.Input)
	brandIDs := make(map[string]string)
	for _, user := range f.Users {
		if _, exists := inputs[user.Brand]; exists {
			continue
		}
		var brandID string
		if err := f.DB.QueryRow(ctx, `SELECT id::text FROM brands WHERE code=$1`, user.Brand).Scan(&brandID); err != nil {
			t.Fatalf("read brand id for %s: %v", user.Brand, err)
		}
		brandIDs[user.Brand] = brandID
		inputs[user.Brand] = capacityRule(t, f.DB, brandID, "capacity_background", 10*time.Second, 10100*time.Millisecond)
	}

	ordersStarted := time.Now()
	orderIDs := make([]string, len(f.Users))
	orderErrors := make([]error, len(f.Users))
	var orderWG sync.WaitGroup
	limit := make(chan struct{}, f.PoolSize)
	for i, user := range f.Users {
		orderWG.Add(1)
		go func(i int, user capacityUser) {
			defer orderWG.Done()
			limit <- struct{}{}
			defer func() { <-limit }()
			in := inputs[user.Brand]
			previewInput := in
			previewInput.ActorContext = ""
			raw, err := json.Marshal(previewInput)
			if err != nil {
				orderErrors[i] = err
				return
			}
			preview, err := f.request(ctx, user, "/bet-previews", "", raw)
			if err != nil {
				orderErrors[i] = fmt.Errorf("background bet preview %d failed: %w", i, err)
				return
			}
			var previewEnvelope struct {
				Success bool `json:"success"`
				Data    struct {
					ActorContext string `json:"actor_context"`
				} `json:"data"`
			}
			if preview.status != 200 || json.Unmarshal(preview.body, &previewEnvelope) != nil || !previewEnvelope.Success || previewEnvelope.Data.ActorContext == "" {
				orderErrors[i] = fmt.Errorf("background bet preview %d failed with status %d: %s", i, preview.status, preview.body)
				return
			}
			in.ActorContext = previewEnvelope.Data.ActorContext
			body, err := json.Marshal(in)
			if err != nil {
				orderErrors[i] = err
				return
			}
			placed, err := f.request(ctx, user, "/bet-orders", "capacity-background-"+user.Member, body)
			if err != nil {
				orderErrors[i] = fmt.Errorf("background bet order %d failed: %w", i, err)
				return
			}
			var orderEnvelope struct {
				Success bool `json:"success"`
				Data    struct {
					ID     string `json:"id"`
					Member string `json:"brand_member_id"`
					Points string `json:"total_points"`
				} `json:"data"`
			}
			if placed.status != 201 || json.Unmarshal(placed.body, &orderEnvelope) != nil || !orderEnvelope.Success || orderEnvelope.Data.ID == "" || orderEnvelope.Data.Member != user.Member || orderEnvelope.Data.Points != "1" {
				orderErrors[i] = fmt.Errorf("background bet order %d failed with status %d: %s", i, placed.status, placed.body)
				return
			}
			orderIDs[i] = orderEnvelope.Data.ID
		}(i, user)
	}
	orderWG.Wait()
	for _, err := range orderErrors {
		if err != nil {
			t.Fatal(err)
		}
	}
	if time.Since(ordersStarted) > 10*time.Second {
		t.Fatalf("background bet setup took longer than its 10 second betting window: %s", time.Since(ordersStarted))
	}

	store := rulebook.Store{DB: f.DB}
	periods := make(map[string]struct {
		id      string
		version int64
		no      string
		drawAt  time.Time
	})
	for brand, in := range inputs {
		deadline := time.Now().Add(30 * time.Second)
		for {
			if _, err := store.Tick(ctx); err != nil {
				t.Fatalf("advance background period for %s: %v", brand, err)
			}
			var status string
			var period struct {
				id      string
				version int64
				no      string
				drawAt  time.Time
			}
			err := f.DB.QueryRow(ctx, `SELECT id::text,version,period_no,draw_at,status FROM periods WHERE id=$1`, in.PeriodID).Scan(&period.id, &period.version, &period.no, &period.drawAt, &status)
			if err != nil {
				t.Fatalf("read background period for %s: %v", brand, err)
			}
			if status == "waiting_draw" {
				periods[brand] = period
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("background period for %s did not reach waiting_draw within 30 seconds; current state=%q", brand, status)
			}
			time.Sleep(25 * time.Millisecond)
		}
	}

	service := betting.Service{DB: f.DB}
	jobs := make([]betting.SettlementJob, 0, len(inputs))
	for brand, in := range inputs {
		period := periods[brand]
		var creator string
		brandID := brandIDs[brand]
		if err := f.DB.QueryRow(ctx, `SELECT g.created_by::text FROM games g JOIN periods p ON p.game_id=g.id WHERE p.id=$1 AND g.brand_id=$2`, period.id, brandID).Scan(&creator); err != nil {
			t.Fatalf("read background game creator for %s: %v", brand, err)
		}
		actor := access.Account{ID: creator, Type: access.AccountAdmin, BrandIDs: []string{brandID}, Roles: []access.Role{{BrandID: brandID, Permissions: []access.Permission{
			{Resource: "draw", Action: "manual_create", Scope: access.ScopeBrand},
			{Resource: "settlement_policy", Action: "write", Scope: access.ScopeBrand},
			{Resource: "settlement", Action: "run", Scope: access.ScopeBrand},
		}}}}
		meta := points.Metadata{ActorType: "admin", ActorID: creator, RequestID: ids.New()}
		var draw rulebook.DrawResult
		capacityTx(t, f.DB, func(tx pgx.Tx) error {
			var err error
			draw, err = (rulebook.Store{DB: f.DB}).ManualDraw(ctx, tx, brandID, actor, period.id, period.version, period.no, rules.Draw{Digits: []int{1, 2, 1}}, period.drawAt, "capacity settlement background draw", meta)
			return err
		})
		mode := "automatic"
		var policy betting.SettlementPolicy
		capacityTx(t, f.DB, func(tx pgx.Tx) error {
			var err error
			policy, err = service.SaveSettlementPolicy(ctx, tx, brandID, actor, betting.SettlementPolicyInput{Version: 1, Mode: &mode, Reason: "capacity settlement background fixture"}, meta)
			return err
		})
		settlementContext, err := service.PeriodSettlementContext(ctx, brandID, in.PeriodID)
		if err != nil {
			t.Fatalf("read settlement context for %s: %v", brand, err)
		}
		if settlementContext.DrawResultID == nil || *settlementContext.DrawResultID != draw.ID || !settlementContext.CanStart {
			t.Fatalf("background period for %s is not ready for settlement: %+v", brand, settlementContext)
		}
		var job betting.SettlementJob
		capacityTx(t, f.DB, func(tx pgx.Tx) error {
			var err error
			job, err = service.StartSettlement(ctx, tx, brandID, actor, in.PeriodID, betting.SettlementStartInput{Version: settlementContext.PeriodVersion, PolicyVersion: policy.Version, DrawResultID: *settlementContext.DrawResultID, Reason: "capacity settlement background fixture"}, meta)
			return err
		})
		jobs = append(jobs, job)
	}

	var targetTotal int64
	for _, job := range jobs {
		targetTotal += job.TargetCount
	}
	if targetTotal != int64(len(f.Users)) {
		t.Fatalf("background settlement targets=%d, want %d users across brands", targetTotal, len(f.Users))
	}
	return capacitySettlementFixture{Jobs: jobs, OrderIDs: orderIDs}
}

func TestCapacitySettlementFixtureSmoke(t *testing.T) {
	f := newCapacityFixture(t, 4, 2, 20)
	fixture := prepareCapacitySettlement(t, f)
	if len(fixture.Jobs) != 2 || len(fixture.OrderIDs) != 4 {
		t.Fatalf("background fixture has %d jobs and %d orders; want 2 jobs and 4 orders", len(fixture.Jobs), len(fixture.OrderIDs))
	}
	for _, job := range fixture.Jobs {
		if job.State != "processing" || job.PendingCount != job.TargetCount || job.ReadyCount != 0 || job.PaidCount != 0 || job.TargetCount == 0 {
			t.Fatalf("background settlement job was processed or has unexpected targets: %+v", job)
		}
	}
	var prizeCredits int
	if err := f.DB.QueryRow(context.Background(), `SELECT count(*) FROM point_ledger_entries WHERE entry_type='prize'`).Scan(&prizeCredits); err != nil {
		t.Fatal(err)
	}
	if prizeCredits != 0 {
		t.Fatalf("background fixture created %d prize credits before workers started", prizeCredits)
	}
}
