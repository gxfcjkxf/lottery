package identity

import (
	"context"
	"encoding/json"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/gxfcjkxf/lottery/backend/internal/telegramauth"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"github.com/jackc/pgx/v5"
	"testing"
)

func TestTelegramGlobalIdentityAndImmutableBinding(t *testing.T) {
	p := testdb.New(t)
	s, _ := New(p)
	e, _ := mutation.New(p, make([]byte, 32))
	ctx := context.Background()
	brand := "0199a000-0000-7000-8000-000000000001"
	other := "0199a000-0000-7000-8000-000000000002"
	if _, err := p.Exec(ctx, `UPDATE brands SET auth_config=auth_config||'{"telegram_enabled":true,"telegram_client_id":"12345"}'::jsonb`); err != nil {
		t.Fatal(err)
	}
	input := TelegramInput{Privacy: "dev-1", Terms: "dev-1"}
	claims := telegramauth.Claims{ID: "987654321", Subject: "verified-subject", Name: "Example"}
	meta := Metadata{RequestID: "test-telegram", Domain: "localhost"}
	run := func(brand, key, bind string, claims telegramauth.Claims) mutation.Result {
		r, err := e.Execute(ctx, brand, "telegram-test", "tg", key, e.Fingerprint(key), func(ctx context.Context, tx pgx.Tx) (mutation.Result, error) {
			return s.Telegram(ctx, tx, brand, claims, input, bind, meta)
		})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	first := run(brand, "telegram-first", "", claims)
	if first.Status != 200 {
		t.Fatal(first)
	}
	var a Authentication
	json.Unmarshal(first.Data, &a)
	second := run(other, "telegram-second", "", claims)
	var b Authentication
	json.Unmarshal(second.Data, &b)
	if b.User.ID != a.User.ID || b.Member.ID == a.Member.ID {
		t.Fatal("Telegram identity not global/brand membership not isolated")
	}
	if r := run(brand, "telegram-rebind", a.User.ID, telegramauth.Claims{ID: "987654322", Subject: "other"}); r.Status != 409 || r.Error.Code != "TELEGRAM_BINDING_IMMUTABLE" {
		t.Fatal("binding replaced", r)
	}
	var count int
	p.QueryRow(ctx, "SELECT count(*) FROM global_users").Scan(&count)
	if count != 1 {
		t.Fatal("duplicate Telegram identity", count)
	}
}
