package identity

import (
	"context"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"testing"
)

func TestRateLimitPersistsAcrossInstancesAndExpires(t *testing.T) {
	p := testdb.New(t)
	one, _ := New(p)
	two, _ := New(p)
	e, _ := mutation.New(p, make([]byte, 32))
	ctx := context.Background()
	brand := "0199a000-0000-7000-8000-000000000001"
	meta := Metadata{IP: "192.0.2.10", RequestID: "test-limit"}
	for i := 0; i < 10; i++ {
		ok, err := one.AllowAttempt(ctx, e, brand, "alice", meta)
		if err != nil || !ok {
			t.Fatal(i, err)
		}
	}
	if ok, err := two.AllowAttempt(ctx, e, brand, "alice", meta); err != nil || ok {
		t.Fatal("second process bypassed account limit", err)
	}
	var count int
	p.QueryRow(ctx, "SELECT count(*) FROM audit_logs WHERE action='auth.rate_limit'").Scan(&count)
	if count != 1 {
		t.Fatal("risk event missing", count)
	}
	p.Exec(ctx, "UPDATE auth_rate_limits SET expires_at=now()-interval '1 minute'")
	if ok, err := two.AllowAttempt(ctx, e, brand, "alice", meta); err != nil || !ok {
		t.Fatal("expired window didn't recover", err)
	}
}

func TestUnidentifiedAuthTrafficDoesNotShareAnAccountBucket(t *testing.T) {
	p := testdb.New(t)
	s, _ := New(p)
	e, _ := mutation.New(p, make([]byte, 32))
	ctx := context.Background()
	brand := "0199a000-0000-7000-8000-000000000001"
	for i := 0; i < 30; i++ {
		allowed, err := s.AllowAttempt(ctx, e, brand, "", Metadata{IP: "192.0.2.20"})
		if err != nil || !allowed {
			t.Fatalf("IP-only auth request %d wrongly consumed a shared account bucket: %v", i+1, err)
		}
	}
	if allowed, err := s.AllowAttempt(ctx, e, brand, "", Metadata{IP: "192.0.2.20"}); err != nil || allowed {
		t.Fatalf("IP quota not enforced: %v", err)
	}
	if allowed, err := s.AllowAttempt(ctx, e, brand, "", Metadata{IP: "192.0.2.21"}); err != nil || !allowed {
		t.Fatalf("one IP blocked unrelated Telegram/captcha traffic: %v", err)
	}
}

func TestVerifiedTelegramAccountQuotaIsIndependentAndGlobal(t *testing.T) {
	p := testdb.New(t)
	s, _ := New(p)
	e, _ := mutation.New(p, make([]byte, 32))
	ctx := context.Background()
	brandA := "0199a000-0000-7000-8000-000000000001"
	brandB := "0199a000-0000-7000-8000-000000000002"
	meta := Metadata{IP: "192.0.2.30"}
	for i := 0; i < 10; i++ {
		if allowed, err := s.AllowAccountAttempt(ctx, e, brandA, "telegram:111", meta); err != nil || !allowed {
			t.Fatal(i, err)
		}
	}
	if allowed, err := s.AllowAccountAttempt(ctx, e, brandB, "telegram:111", meta); err != nil || allowed {
		t.Fatalf("same verified identity bypassed its quota across brands: %v", err)
	}
	if allowed, err := s.AllowAccountAttempt(ctx, e, brandA, "telegram:222", meta); err != nil || !allowed {
		t.Fatalf("one identity consumed another's quota: %v", err)
	}
}
