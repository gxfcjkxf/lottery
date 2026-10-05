package identity

import (
	"context"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"strings"
	"testing"
)

func TestChallengeSingleUseAndScope(t *testing.T) {
	p := testdb.New(t)
	s, _ := New(p)
	e, _ := mutation.New(p, make([]byte, 32))
	ctx := context.Background()
	a := "0199a000-0000-7000-8000-000000000001"
	b := "0199a000-0000-7000-8000-000000000002"
	c, err := s.Challenge(ctx, e, a, "telegram")
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := s.Consume(ctx, e, b, "telegram", c.ID, c.Nonce); ok || err != nil {
		t.Fatal("cross-brand challenge", err)
	}
	if ok, err := s.Consume(ctx, e, a, "telegram", c.ID, c.Nonce); !ok || err != nil {
		t.Fatal("valid challenge rejected", err)
	}
	if ok, err := s.Consume(ctx, e, a, "telegram", c.ID, c.Nonce); ok || err != nil {
		t.Fatal("replay challenge", err)
	}
	c, err = s.Challenge(ctx, e, a, "captcha")
	if err != nil || !strings.Contains(c.SVG, "<svg") {
		t.Fatal(err)
	}
	if ok, err := s.Consume(ctx, e, a, "captcha", c.ID, "wrong"); ok || err != nil {
		t.Fatal("wrong challenge accepted", err)
	}
	var used bool
	p.QueryRow(ctx, "SELECT used_at IS NOT NULL FROM auth_challenges WHERE id=$1", c.ID).Scan(&used)
	if !used {
		t.Fatal("failed challenge reusable")
	}
	if _, err = s.Consume(ctx, e, a, "captcha", strings.Repeat("x", 36), "x"); err != nil {
		t.Fatal("malformed ID creates SQL error")
	}
}
