package compliance

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
)

func TestGatePolicySerializationAndDisabledNoEvidence(t *testing.T) {
	s := Service{DB: testdb.New(t)}
	a := actor(t, s)
	ctx := context.Background()
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	subject := GateSubject{ActorType: "anonymous", RequestID: "gate-serialization"}
	if e = AssessTx(ctx, tx, testBrand, "register", subject); e != nil {
		t.Fatal(e)
	}
	writer, e := s.DB.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer writer.Rollback(ctx)
	var pid int
	if e = writer.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); e != nil {
		t.Fatal(e)
	}
	done := make(chan error, 1)
	go func() {
		cfg := DefaultConfig()
		cfg.IdentityEnabled = true
		_, err := s.Update(ctx, writer, testBrand, a, Input{Version: 1, Config: cfg, Reason: "Serialize compliance activation"}, points.Metadata{ActorType: "admin", ActorID: a.ID, RequestID: "policy-writer"})
		if err == nil {
			err = writer.Commit(ctx)
		}
		done <- err
	}()
	deadline := time.Now().Add(3 * time.Second)
	blocked := false
	for time.Now().Before(deadline) {
		if e = s.DB.QueryRow(ctx, `SELECT cardinality(pg_blocking_pids($1))>0`, pid).Scan(&blocked); e != nil {
			t.Fatal(e)
		}
		if blocked {
			break
		}
		select {
		case e = <-done:
			t.Fatalf("policy update bypassed active gate lock: %v", e)
		default:
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !blocked {
		t.Fatal("writer did not demonstrably wait for gate")
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	if e = <-done; e != nil {
		t.Fatal(e)
	}
	next, e := s.DB.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer next.Rollback(ctx)
	var b *Blocked
	if e = AssessTx(ctx, next, testBrand, "register", subject); !errors.As(e, &b) || b.Record.PolicyVersion != 2 {
		t.Fatal("future admission ignored enabled policy", e)
	}
	var count int
	if e = s.DB.QueryRow(ctx, `SELECT count(*) FROM compliance_gate_rejections`).Scan(&count); e != nil || count != 0 {
		t.Fatal("disabled or uncommitted gate fabricated evidence", count, e)
	}
}
func TestGateHistorySurvivesPolicyChangeAndCannotBeForged(t *testing.T) {
	s := Service{DB: testdb.New(t)}
	a := actor(t, s)
	ctx := context.Background()
	cfg := DefaultConfig()
	cfg.IdentityEnabled = true
	if _, e := update(t, s, a, Input{Version: 1, Config: cfg, Reason: "Enable identity check"}); e != nil {
		t.Fatal(e)
	}
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	var b *Blocked
	e = AssessTx(ctx, tx, testBrand, "register", GateSubject{ActorType: "anonymous", RequestID: "historical-rejection"})
	if !errors.As(e, &b) {
		t.Fatal(e)
	}
	if e = tx.Rollback(ctx); e != nil {
		t.Fatal(e)
	}
	if _, e = update(t, s, a, Input{Version: 2, Config: DefaultConfig(), Reason: "Disable while rejection is pending"}); e != nil {
		t.Fatal(e)
	}
	proof, e := s.DB.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer proof.Rollback(ctx)
	if e = b.Persist(ctx, proof); e != nil {
		t.Fatal(e)
	}
	if e = proof.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	page, e := s.Gates(ctx, testBrand, "registration", 1, 0)
	if e != nil || page.TotalCount != "1" || len(page.Items) != 1 || page.Items[0].PolicyVersion != 2 || page.Items[0].AuditLogID == "" || page.Items[0].ActorID != nil {
		t.Fatal(page, e)
	}
	for _, q := range []string{`UPDATE compliance_gate_rejections SET decision='allow' WHERE id=$1`, `DELETE FROM compliance_gate_rejections WHERE id=$1`} {
		if _, e = s.DB.Exec(ctx, q, b.Record.ID); e == nil {
			t.Fatal("rejection evidence rewritten")
		}
	}
	corrupt := *b
	corrupt.Record.ID = "0199a000-0000-7000-8000-000000000077"
	corrupt.Record.Config = DefaultConfig()
	corrupt.Record.Config.RegionEnabled = true
	corrupt.Record.Config.AllowedCountries = []string{"US"}
	_, corrupt.Record.Checks, _ = Evaluate(corrupt.Record.Config)
	bad, e := s.DB.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer bad.Rollback(ctx)
	if e = corrupt.Persist(ctx, bad); e == nil {
		t.Fatal("forged policy snapshot committed")
	}
}
func TestGateRejectsInvalidContextAndUsesEachEnabledCheck(t *testing.T) {
	s := Service{DB: testdb.New(t)}
	a := actor(t, s)
	ctx := context.Background()
	version := int64(1)
	for _, name := range []string{"age", "region", "identity"} {
		cfg := DefaultConfig()
		switch name {
		case "age":
			n := 21
			cfg.AgeEnabled = true
			cfg.MinimumAge = &n
		case "region":
			cfg.RegionEnabled = true
			cfg.AllowedCountries = []string{"PH"}
		case "identity":
			cfg.IdentityEnabled = true
		}
		if _, e := update(t, s, a, Input{Version: version, Config: cfg, Reason: "Test one enabled check"}); e != nil {
			t.Fatal(e)
		}
		version++
		tx, e := s.DB.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		var b *Blocked
		if e = AssessTx(ctx, tx, testBrand, "register", GateSubject{ActorType: "anonymous", RequestID: "single-check"}); !errors.As(e, &b) {
			t.Fatal(e)
		}
		for _, c := range b.Record.Checks {
			if c.Check == name && (c.Decision != "review" || c.ReasonCode != "ADAPTER_NOT_CONFIGURED") {
				t.Fatal(c)
			}
		}
		tx.Rollback(ctx)
	}
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	for _, bad := range []GateSubject{{ActorType: "anonymous"}, {ActorType: "user", RequestID: "invalid-actor"}, {ActorType: "anonymous", ActorID: a.ID, RequestID: "spoof"}} {
		if e = AssessTx(ctx, tx, testBrand, "register", bad); !errors.Is(e, ErrInvalid) {
			t.Fatal("invalid gate context accepted", e)
		}
	}
	if e = AssessTx(ctx, tx, testBrand, "withdrawal", GateSubject{ActorType: "anonymous", RequestID: "no-workflow"}); !errors.Is(e, ErrInvalid) {
		t.Fatal("invented withdrawal admission", e)
	}
}
