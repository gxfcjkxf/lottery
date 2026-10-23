package betting

import (
	"context"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/agency"
	"github.com/gxfcjkxf/lottery/backend/internal/commission"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
)

func TestBetModeChangeSnapshotsUsePlacementCycleWithoutWorker(t *testing.T) {
	f := newCommissionBatchFixtureWithWindow(t, 9*time.Second, 11*time.Second)
	ctx := context.Background()
	agents := agency.Service{DB: f.betting.db}
	policy, err := agents.Policy(ctx, f.betting.brand)
	if err != nil {
		t.Fatal(err)
	}
	var change agency.ModeChangeReceipt
	bettingTx(t, f.betting.db, func(tx pgx.Tx) error {
		var e error
		change, e = agents.ScheduleModeChange(ctx, tx, f.betting.brand, f.actor, f.node.ID, agency.ModeChangeInput{PolicyVersion: policy.Version, Mode: "turnover", Items: []agency.ModeRatioInput{{AgentID: f.node.ID, Version: f.node.Version, Ratio: "0.02"}}}, points.Metadata{RequestID: ids.New()})
		return e
	})
	before, err := placeBettingOrder(t, f.betting, f.betting.input, "mode-change-before")
	if err != nil {
		t.Fatal(err)
	}
	waitCommissionBoundary(t, change.EffectiveAt)
	after, err := placeBettingOrder(t, f.betting, f.betting.input, "mode-change-after")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ id, mode, ratio, version string }{{before.ID, "loss", "0.3", "1"}, {after.ID, "turnover", "0.02", "2"}} {
		var mode, ratio, version, attributionRatio string
		err = f.betting.db.QueryRow(ctx, `SELECT commission_rule_snapshot->'agent_path'->0->>'effective_mode',commission_rule_snapshot->'agent_path'->0->'config'->>'ratio',commission_rule_snapshot->'agent_path'->0->>'version',attribution_snapshot->'agent_configs_at_bet'->0->'config'->>'ratio' FROM bet_orders WHERE id=$1`, tc.id).Scan(&mode, &ratio, &version, &attributionRatio)
		if err != nil || mode != tc.mode || ratio != tc.ratio || version != tc.version || attributionRatio != tc.ratio {
			t.Fatal(tc, mode, ratio, version, attributionRatio, err)
		}
	}
	// No activation worker or configuration write ran between the two bets.
	var rawMode string
	if err = f.betting.db.QueryRow(ctx, `SELECT config->>'mode' FROM agent_nodes WHERE id=$1`, f.node.ID).Scan(&rawMode); err != nil || rawMode != "loss" {
		t.Fatal(rawMode, err)
	}
	// Editing the financial policy after the boundary materializes the line
	// first, so an expired reservation never leaves the calendar locked.
	current, err := f.service.Policy(ctx, f.betting.brand)
	if err != nil {
		t.Fatal(err)
	}
	cfg := current.Config
	cfg.PayoutMode = commission.PayoutAutomatic
	bettingTx(t, f.betting.db, func(tx pgx.Tx) error {
		_, err := f.service.Update(ctx, tx, f.betting.brand, f.actor, commission.PolicyInput{Version: current.Version, Config: cfg}, points.Metadata{ActorType: "admin", ActorID: f.actor.ID, RequestID: ids.New()})
		return err
	})
	if err = f.betting.db.QueryRow(ctx, `SELECT config->>'mode' FROM agent_nodes WHERE id=$1`, f.node.ID).Scan(&rawMode); err != nil || rawMode != "turnover" {
		t.Fatal(rawMode, err)
	}

}
