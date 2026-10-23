package betting

import (
	"context"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/agency"
	"github.com/gxfcjkxf/lottery/backend/internal/commission"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
)

func TestCommissionBatchSettlesSavedSnapshotAfterPoliciesAreReplaced(t *testing.T) {
	t.Parallel()
	f := newCommissionBatchFixture(t)
	ctx := context.Background()
	// Replace both current policies after the orders were placed. There is no policy
	// history to consult; the worker must use the immutable order configurations.
	bettingTx(t, f.betting.db, func(tx pgx.Tx) error {
		current, err := f.service.Policy(ctx, f.betting.brand)
		if err != nil {
			return err
		}
		_, err = f.service.Update(ctx, tx, f.betting.brand, f.actor, commission.PolicyInput{Version: current.Version, Config: commission.DefaultPolicyConfig()}, points.Metadata{ActorType: "admin", ActorID: f.actor.ID, RequestID: ids.New()})
		return err
	})
	agents := agency.Service{DB: f.betting.db}
	current, err := agents.Policy(ctx, f.betting.brand)
	if err != nil {
		t.Fatal(err)
	}
	next := current.Config
	next.Mode = "turnover"
	bettingTx(t, f.betting.db, func(tx pgx.Tx) error {
		_, err := agents.SavePolicy(ctx, tx, f.betting.brand, f.actor, agency.PolicyInput{Version: current.Version, Config: next}, points.Metadata{RequestID: ids.New()})
		return err
	})
	waitCommissionBoundary(t, f.boundary)
	cycle := createCommissionCycle(t, f)
	eligibilitySettle(t, f.betting)
	advanceCommissionWorker(t, f, 30)
	cycle = readCommissionCycle(t, f, cycle.ID)
	if cycle.State != "ready" || cycle.TotalPoints != "1" || cycle.EarningCount != "1" {
		t.Fatalf("old orders did not settle from their saved rules: %+v", cycle)
	}
	var policyAudits, policyHistory int
	err = f.betting.db.QueryRow(ctx, `SELECT (SELECT count(*) FROM audit_logs WHERE brand_id=$1 AND action IN ('commission.policy.update','agent.policy.update')),(SELECT count(*) FROM agent_config_revisions WHERE brand_id=$1 AND agent_id IS NULL)`, f.betting.brand).Scan(&policyAudits, &policyHistory)
	if err != nil || policyAudits != 0 || policyHistory != 0 {
		t.Fatalf("policy history was recorded: %d %d %v", policyAudits, policyHistory, err)
	}
}
