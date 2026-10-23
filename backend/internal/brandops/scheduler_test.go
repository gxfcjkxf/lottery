package brandops

import (
	"context"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
)

func TestScheduledPauseResumeAndManualCancellation(t *testing.T) {
	ctx := context.Background()
	p := testdb.New(t)
	adminID := ids.New()
	if _, err := p.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,$2,'unused')`, adminID, "schedule_"+adminID[:8]); err != nil {
		t.Fatal(err)
	}
	actor := access.Account{ID: adminID, Type: access.AccountAdmin, Roles: []access.Role{{Permissions: []access.Permission{{Resource: "brand_operation", Action: "write", Scope: access.ScopePlatform}}}}}
	svc := Service{DB: p}
	meta := points.Metadata{ActorType: "admin", ActorID: adminID, RequestID: "schedule-test"}
	apply := func(in Input) Record {
		t.Helper()
		tx, err := p.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		out, err := svc.Update(ctx, tx, brandID, actor, in, meta)
		if err != nil {
			t.Fatal(err)
		}
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		return out
	}
	start := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	end := start.Add(time.Hour)
	planned := apply(Input{Version: 1, Status: "paused", PauseStartsAt: &start, PauseEndsAt: &end})
	if planned.Status != "active" || planned.PauseStartsAt == nil {
		t.Fatalf("future pause applied early: %+v", planned)
	}
	if changed, err := svc.tickOne(ctx, start.Add(-time.Second)); err != nil || changed {
		t.Fatalf("ran early: %v %v", changed, err)
	}
	if changed, err := svc.tickOne(ctx, start); err != nil || !changed {
		t.Fatalf("did not pause: %v %v", changed, err)
	}
	paused, err := svc.Read(ctx, brandID)
	if err != nil || paused.Status != "paused" || paused.Version != 3 {
		t.Fatalf("pause: %+v %v", paused, err)
	}
	if changed, err := svc.tickOne(ctx, start); err != nil || changed {
		t.Fatalf("repeated pause: %v %v", changed, err)
	}
	if changed, err := svc.tickOne(ctx, end); err != nil || !changed {
		t.Fatalf("did not resume: %v %v", changed, err)
	}
	resumed, err := svc.Read(ctx, brandID)
	if err != nil || resumed.Status != "active" || resumed.PauseStartsAt != nil || resumed.Version != 4 {
		t.Fatalf("resume: %+v %v", resumed, err)
	}
	var automatic int
	if err = p.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE brand_id=$1 AND action='brand_operation.update' AND actor_type='system' AND actor_id IS NULL AND reason=''`, brandID).Scan(&automatic); err != nil || automatic != 2 {
		t.Fatalf("system audit: %d %v", automatic, err)
	}
	planned = apply(Input{Version: 4, Status: "paused", Reason: "second plan", PauseStartsAt: &start, PauseEndsAt: &end})
	stopped := apply(Input{Version: planned.Version, Status: "disabled", Reason: "stop operations"})
	if stopped.Status != "disabled" || stopped.PauseStartsAt != nil {
		t.Fatal(stopped)
	}
	if changed, err := svc.tickOne(ctx, end.Add(time.Hour)); err != nil || changed {
		t.Fatalf("stop revived by stale plan: %v %v", changed, err)
	}
	restarted := apply(Input{Version: stopped.Version, Status: "active", Reason: "restart operations"})
	if restarted.Status != "active" {
		t.Fatal(restarted)
	}
	planned = apply(Input{Version: restarted.Version, Status: "paused", Reason: "cancel this plan", PauseStartsAt: &start, PauseEndsAt: &end})
	cancelled := apply(Input{Version: planned.Version, Status: "active", Reason: "cancel scheduled maintenance"})
	if cancelled.PauseStartsAt != nil {
		t.Fatal(cancelled)
	}
	if changed, err := svc.tickOne(ctx, start); err != nil || changed {
		t.Fatalf("cancelled plan ran: %v %v", changed, err)
	}
	// A delayed worker clears an entirely expired window without briefly pausing.
	planned = apply(Input{Version: cancelled.Version, Status: "paused", Reason: "expired window", PauseStartsAt: &start, PauseEndsAt: &end})
	if changed, err := svc.tickOne(ctx, end.Add(time.Second)); err != nil || !changed {
		t.Fatalf("expired plan: %v %v", changed, err)
	}
	current, err := svc.Read(ctx, brandID)
	if err != nil || current.Status != "active" || current.PauseStartsAt != nil {
		t.Fatalf("expired plan paused: %+v %v", current, err)
	}
	history, err := svc.History(ctx, brandID, 100, 0)
	if err != nil || history[0].Reason != "expired window" {
		t.Fatalf("history: %+v %v", history, err)
	}
}
