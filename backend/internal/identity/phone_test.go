package identity

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"

 "github.com/jackc/pgx/v5"
)

func TestPhoneRegistrationAndLoginWithOptionalCountryCode(t *testing.T) {
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
			if err := tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
		}
		return result
	}
	for _, phone := range []string{"09171234567", "13455676549", "+8613455676549"} {
		t.Run(phone, func(t *testing.T) {
			input := RegisterInput{Phone: phone, Password: "phone-test-password", Privacy: "dev-1", Terms: "dev-1"}
			registered := run(func(tx pgx.Tx) (mutation.Result, error) {
				return s.Register(ctx, tx, operatorTestBrand, input, Metadata{})
			})
			if registered.Status != 201 {
				t.Fatalf("registration: %+v", registered)
			}
			login := run(func(tx pgx.Tx) (mutation.Result, error) {
				return s.Login(ctx, tx, operatorTestBrand, LoginInput{Identifier: phone, Password: input.Password}, Metadata{})
			})
			var authenticated Authentication
			if login.Status != 200 || json.Unmarshal(login.Data, &authenticated) != nil || authenticated.User.Phone != phone {
				t.Fatalf("login did not preserve phone: %+v", login)
			}
			duplicate := run(func(tx pgx.Tx) (mutation.Result, error) {
				return s.Register(ctx, tx, operatorTestBrand, input, Metadata{})
			})
			if duplicate.Status != 409 || duplicate.Error == nil || duplicate.Error.Code != "IDENTITY_EXISTS" {
				t.Fatalf("duplicate phone accepted: %+v", duplicate)
			}
		})
	}
}
