package identity

import (
	"context"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"strconv"
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
	if err != nil || c.Puzzle == nil || !strings.HasPrefix(c.Puzzle.Image, "data:image/jpeg;base64,") {
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

func TestPuzzleAnswerToleranceExpiryAndScope(t *testing.T) {
	p := testdb.New(t)
	s, _ := New(p)
	e, _ := mutation.New(p, make([]byte, 32))
	ctx := context.Background()
	const brand = "0199a000-0000-7000-8000-000000000001"
	const other = "0199a000-0000-7000-8000-000000000002"
	for _, offset := range []int{-5, -4, 0, 4, 5} {
		c, err := s.Challenge(ctx, e, brand, "captcha")
		if err != nil {
			t.Fatal(err)
		}
		var hash string
		if err := p.QueryRow(ctx, "SELECT proof_hash FROM auth_challenges WHERE id=$1", c.ID).Scan(&hash); err != nil {
			t.Fatal(err)
		}
		target := -1
		for x := 0; x <= 300; x++ {
			if e.Fingerprint(c.ID+":"+strconv.Itoa(x)) == hash {
				target = x
				break
			}
		}
		if target < 0 {
			t.Fatal("no stored target")
		}
		answer := strconv.Itoa(target + offset)
		if ok, err := s.Consume(ctx, e, other, "captcha", c.ID, answer); ok || err != nil {
			t.Fatal("cross-brand", err)
		}
		ok, err := s.Consume(ctx, e, brand, "captcha", c.ID, answer)
		if err != nil || ok != (offset >= -4 && offset <= 4) {
			t.Fatalf("offset %d: %v %v", offset, ok, err)
		}
		if ok, err := s.Consume(ctx, e, brand, "captcha", c.ID, strconv.Itoa(target)); ok || err != nil {
			t.Fatal("replay", err)
		}
	}
	c, err := s.Challenge(ctx, e, brand, "captcha")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.Exec(ctx, "UPDATE auth_challenges SET expires_at=now()-interval '1 second' WHERE id=$1", c.ID); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.Consume(ctx, e, brand, "captcha", c.ID, "100"); ok || err != nil {
		t.Fatal("expired", err)
	}
}
