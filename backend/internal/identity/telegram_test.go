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
	if _, err := p.Exec(ctx, `UPDATE brands SET auth_inherit_platform=false,auth_config=auth_config||'{"telegram_enabled":true,"telegram_client_id":"12345"}'::jsonb`); err != nil {
		t.Fatal(err)
	}
	input := TelegramInput{}
	claims := telegramauth.Claims{ID: "987654321", Subject: "verified-subject", Name: "Example", Username: "Example_User"}
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
	if a.User.Username != "example_user" || a.User.Telegram != "@Example_User" || a.Member.DisplayName != "Example" {
		t.Fatalf("Telegram profile not populated: %#v", a)
	}
	claims.Username = "Changed_User"
	second := run(other, "telegram-second", "", claims)
	var b Authentication
	json.Unmarshal(second.Data, &b)
	if b.User.Username != "example_user" || b.User.Telegram != "@Example_User" {
		t.Fatalf("Existing profile overwritten: %#v", b.User)
	}
	if b.User.ID != a.User.ID || b.Member.ID == a.Member.ID {
		t.Fatal("Telegram identity not global/brand membership not isolated")
	}
	if r := run(brand, "telegram-rebind", a.User.ID, telegramauth.Claims{ID: "987654322", Subject: "other"}); r.Status != 409 || r.Error.Code != "TELEGRAM_BINDING_IMMUTABLE" {
		t.Fatal("binding replaced", r)
	}
	// Previously registered Telegram users receive missing account/contact fields on login.
	if _, err := p.Exec(ctx, "UPDATE global_users SET username=NULL,telegram=NULL WHERE id=$1", a.User.ID); err != nil {
		t.Fatal(err)
	}
	backfill := run(brand, "telegram-backfill", "", claims)
	var filled Authentication
	if backfill.Status != 200 || json.Unmarshal(backfill.Data, &filled) != nil || filled.User.Username != "changed_user" || filled.User.Telegram != "@Changed_User" {
		t.Fatal("missing profile not filled", backfill)
	}
	collision := run(brand, "telegram-collision", "", telegramauth.Claims{ID: "111222333", Username: "Changed_User"})
	var prefixed Authentication
	if collision.Status != 200 || json.Unmarshal(collision.Data, &prefixed) != nil || prefixed.User.Username != "tg_changed_user" || prefixed.User.Telegram != "@Changed_User" {
		t.Fatal("username collision should use tg_ prefix", collision)
	}
	prefixedAgain := run(brand, "telegram-prefixed-repeat", "", telegramauth.Claims{ID: "111222333", Username: "Changed_User"})
	var repeated Authentication
	if prefixedAgain.Status != 200 || json.Unmarshal(prefixedAgain.Data, &repeated) != nil || repeated.User.ID != prefixed.User.ID || repeated.User.Username != "tg_changed_user" {
		t.Fatal("prefixed account should remain stable", prefixedAgain)
	}
	doubleCollision := run(brand, "telegram-double-collision", "", telegramauth.Claims{ID: "777888999", Username: "Changed_User"})
	if doubleCollision.Status != 409 || doubleCollision.Error.Code != "IDENTITY_EXISTS" {
		t.Fatal("occupied prefixed username must be rejected", doubleCollision)
	}
	var count int
	p.QueryRow(ctx, "SELECT count(*) FROM global_users").Scan(&count)
	if count != 2 {
		t.Fatal("duplicate Telegram identity", count)
	}
	withoutUsername := run(brand, "telegram-no-username", "", telegramauth.Claims{ID: "444555666"})
	var empty Authentication
	if withoutUsername.Status != 200 || json.Unmarshal(withoutUsername.Data, &empty) != nil || empty.User.Username != "444555666" || empty.User.Telegram != "" || empty.Member.DisplayName != "" {
		t.Fatal("Telegram ID must identify an account without a username", withoutUsername)
	}
	repeat := run(brand, "telegram-id-repeat", "", telegramauth.Claims{ID: "444555666", Username: "Later_Name"})
	var repeatAuth Authentication
	if repeat.Status != 200 || json.Unmarshal(repeat.Data, &repeatAuth) != nil || repeatAuth.User.Username != "444555666" || repeatAuth.User.Telegram != "@Later_Name" {
		t.Fatal("Telegram ID account name must remain stable", repeat)
	}

	// Missing names on existing Telegram identities are repaired on the next login.
	if _, err := p.Exec(ctx, "UPDATE global_users SET username=NULL,username_set_at=NULL WHERE id=$1", empty.User.ID); err != nil {
		t.Fatal(err)
	}
	repaired := run(brand, "telegram-no-username-repair", "", telegramauth.Claims{ID: "444555666"})
	var repairedAuth Authentication
	if repaired.Status != 200 || json.Unmarshal(repaired.Data, &repairedAuth) != nil || repairedAuth.User.Username != "444555666" || repairedAuth.User.ID != empty.User.ID {
		t.Fatal("existing Telegram identity must use its ID", repaired)
	}
}
