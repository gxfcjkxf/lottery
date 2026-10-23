package agency

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestModeChangeRequiresCompleteRatiosAndStartsAtBoundary(t *testing.T) {
	s, a := fixture(t)
	ctx := context.Background()
	p := enable(t, s, a)
	caps := p.Config
	caps.TurnoverRatioCap = "0.03"
	if err := transaction(t, s, func(tx pgx.Tx) error {
		var err error
		p, err = s.SavePolicy(ctx, tx, brandA, a, PolicyInput{Version: p.Version, Config: caps}, metadata(a))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	root := create(t, s, a, p, nil, "0.08")
	child := create(t, s, a, p, &root, "0.04")
	leaf := create(t, s, a, p, &child, "0.02")
	input := ModeChangeInput{PolicyVersion: p.Version, Mode: "turnover", Items: []ModeRatioInput{{root.ID, root.Version, "0.03"}, {child.ID, child.Version, "0.02"}, {leaf.ID, leaf.Version, "0.01"}}}
	schedule := func(in ModeChangeInput) (ModeChangeReceipt, error) {
		var out ModeChangeReceipt
		err := transaction(t, s, func(tx pgx.Tx) error {
			var err error
			out, err = s.ScheduleModeChange(ctx, tx, brandA, a, root.ID, in, metadata(a))
			return err
		})
		return out, err
	}
	if _, err := schedule(input); !errors.Is(err, ErrCalendarRequired) {
		t.Fatal("missing calendar", err)
	}
	// Use an actual near-future weekly boundary; database time remains authoritative.
	boundary := time.Now().UTC().Add(5 * time.Second).Truncate(time.Second)
	weekday := int(boundary.Weekday())
	financial := map[string]any{"enabled": false, "calendar": map[string]any{"timezone": "UTC", "cycle": "weekly", "boundary_time": boundary.Format("15:04:05"), "weekday": weekday, "month_day": nil, "short_month": ""}, "payout_mode": "manual"}
	err := transaction(t, s, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE brand_commission_policies SET version=version+1,config=$2 WHERE brand_id=$1`, brandA, mustJSON(financial))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	aboveCap := input
	aboveCap.Items = append([]ModeRatioInput{}, input.Items...)
	aboveCap.Items[0].Ratio = "0.04"
	if _, err = schedule(aboveCap); !errors.Is(err, ErrLimit) {
		t.Fatal("target mode cap ignored", err)
	}
	missing := input
	missing.Items = missing.Items[:2]
	if _, err = schedule(missing); !errors.Is(err, ErrVersion) {
		t.Fatal("missing child", err)
	}
	invalid := input
	invalid.Items = append([]ModeRatioInput{}, input.Items...)
	invalid.Items[1].Ratio = "0.04"
	if _, err = schedule(invalid); !errors.Is(err, ErrLimit) {
		t.Fatal("new child exceeds new parent", err)
	}
	duplicate := input
	duplicate.Items = append([]ModeRatioInput{}, input.Items...)
	duplicate.Items[2] = duplicate.Items[1]
	if _, err = schedule(duplicate); !errors.Is(err, ErrInvalid) {
		t.Fatal("duplicate", err)
	}
	cross := input
	cross.Items = append([]ModeRatioInput{}, input.Items...)
	cross.Items[2].AgentID = brandB
	if _, err = schedule(cross); !errors.Is(err, ErrVersion) {
		t.Fatal("foreign target", err)
	}
	receipt, err := schedule(input)
	if err != nil {
		t.Fatal(err)
	}
	if !receipt.EffectiveAt.Equal(boundary) {
		t.Fatal(receipt.EffectiveAt, boundary)
	}
	before, err := s.Node(ctx, brandA, root.ID)
	if err != nil || before.EffectiveMode != "loss" || before.Config.Ratio != "0.08" || before.PendingChange == nil {
		t.Fatal(before, err)
	}
	for _, instant := range []time.Time{boundary.Add(-time.Microsecond), boundary} {
		var mode, ratio string
		err = s.DB.QueryRow(ctx, `SELECT agent_effective_mode_at_path($1,path,NULL,NULL,'loss',$3),config->>'ratio' FROM agent_nodes_at($1,$3) WHERE id=$2`, brandA, leaf.ID, instant).Scan(&mode, &ratio)
		expectedMode, expectedRatio := "loss", "0.02"
		if instant.Equal(boundary) {
			expectedMode, expectedRatio = "turnover", "0.01"
		}
		if err != nil || mode != expectedMode || ratio != expectedRatio {
			t.Fatal(instant, mode, ratio, err)
		}
	}
	if _, err = schedule(input); !errors.Is(err, ErrModePending) {
		t.Fatal("pending change duplicated", err)
	}
	cfg := child.Config
	cfg.Ratio = "0.03"
	_, err = updateNode(t, s, a, p, child, cfg)
	if !errors.Is(err, ErrModePending) {
		t.Fatal("pending child edited", err)
	}
	// Wait only for this test's real boundary, then prove that materialization
	// keeps the same revisions and ratios already used by timestamp projection.
	if remaining := time.Until(boundary); remaining > 0 {
		time.Sleep(remaining + 20*time.Millisecond)
	}
	active, err := s.Node(ctx, brandA, leaf.ID)
	if err != nil || active.EffectiveMode != "turnover" || active.Config.Ratio != "0.01" || active.Version != leaf.Version+1 || active.PendingChange != nil {
		t.Fatal(active, err)
	}
	err = transaction(t, s, func(tx pgx.Tx) error { _, err := s.LockPolicy(ctx, tx, brandA); return err })
	if err != nil {
		t.Fatal("materialization", err)
	}
	var applied bool
	var posted int
	err = s.DB.QueryRow(ctx, `SELECT applied_at IS NOT NULL FROM agent_mode_changes WHERE id=$1`, receipt.ID).Scan(&applied)
	if err != nil || !applied {
		t.Fatal(applied, err)
	}
	if err = s.DB.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries`).Scan(&posted); err != nil || posted != 0 {
		t.Fatal("mode change moved money", posted, err)
	}
}
