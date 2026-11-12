package identity

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"github.com/jackc/pgx/v5"
)

func TestOptionalProfileContacts(t *testing.T) {
	p := testdb.New(t)
	s, err := New(p)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	run := func(fn func(pgx.Tx) (mutation.Result, error)) mutation.Result {
		t.Helper()
		tx, err := p.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		result, err := fn(tx)
		if err != nil {
			t.Fatal(err)
		}
		if result.Status < 400 {
			if err = tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
		}
		return result
	}
	registered := run(func(tx pgx.Tx) (mutation.Result, error) {
		return s.Register(ctx, tx, operatorTestBrand, RegisterInput{Username: "contact_member", Password: "contact-password", }, Metadata{})
	})
	var auth Authentication
	if registered.Status != 201 || json.Unmarshal(registered.Data, &auth) != nil {
		t.Fatalf("register without phone: %+v", registered)
	}
	for _, in := range []ProfileInput{{Wechat: " wechat_member "}, {Telegram: "@contact_member"}, {Whatsapp: "+6581234567"}} {
		result := run(func(tx pgx.Tx) (mutation.Result, error) {
			return s.FillProfile(ctx, tx, operatorTestBrand, auth.User.ID, in, Metadata{})
		})
		if result.Status != 200 {
			t.Fatalf("contact-only update: %+v", result)
		}
	}
	session, err := s.Authenticate(ctx, operatorTestBrand, auth.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	if session.User.Phone != "" || session.User.Wechat != "wechat_member" || session.User.Telegram != "@contact_member" || session.User.Whatsapp != "+6581234567" || session.User.TelegramID != "" {
		t.Fatalf("contacts not preserved or login binding changed: %+v", session.User)
	}
	result := run(func(tx pgx.Tx) (mutation.Result, error) {
		return s.FillProfile(ctx, tx, operatorTestBrand, auth.User.ID, ProfileInput{Wechat: "overwrite"}, Metadata{})
	})
	if result.Status != 409 {
		t.Fatalf("set contact overwritten: %+v", result)
	}
	login := run(func(tx pgx.Tx) (mutation.Result, error) {
		return s.Login(ctx, tx, operatorTestBrand, LoginInput{Identifier: "contact_member", Password: "contact-password"}, Metadata{})
	})
	if login.Status != 200 || json.Unmarshal(login.Data, &auth) != nil || auth.User.Wechat != "wechat_member" {
		t.Fatalf("login lost contacts: %+v", login)
	}
}
