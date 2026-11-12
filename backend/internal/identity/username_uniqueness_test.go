package identity

import (
	"context"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"github.com/jackc/pgx/v5"
)

func TestUsernameUniquenessAcrossBrandsAndCreationPaths(t *testing.T) {
	p := testdb.New(t)
	ctx := context.Background()
	s, err := New(p)
	if err != nil {
		t.Fatal(err)
	}
	engine, err := mutation.New(p, make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	admin := operatorTestAdmin(t, ctx, p)
	meta := Metadata{RequestID: "username-uniqueness", Domain: "localhost"}
	run := func(brand, key string, fn func(context.Context, pgx.Tx) (mutation.Result, error)) mutation.Result {
		t.Helper()
		result, err := engine.Execute(ctx, brand, "username-test", "create", key, engine.Fingerprint(key), fn)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	registered := run(operatorTestBrand, "first-registration", func(ctx context.Context, tx pgx.Tx) (mutation.Result, error) {
		return s.Register(ctx, tx, operatorTestBrand, RegisterInput{Username: "TEST", Password: "test-password", }, meta)
	})
	if registered.Status != 201 {
		t.Fatalf("register: %+v", registered)
	}
	for _, brand := range []string{operatorTestBrand, operatorTestOtherBrand} {
		for _, name := range []string{"TEST", "test", "TeSt"} {
			for _, path := range []string{"register", "operator"} {
				key := brand + name + path
				result := run(brand, key, func(ctx context.Context, tx pgx.Tx) (mutation.Result, error) {
					if path == "register" {
						return s.Register(ctx, tx, brand, RegisterInput{Username: name, Password: "other-password", }, meta)
					}
					return s.OperatorCreate(ctx, tx, brand, admin, OperatorInput{Username: name, Password: "other-password"}, meta)
				})
				if result.Status != 409 || result.Error == nil || result.Error.Code != "IDENTITY_EXISTS" {
					t.Fatalf("%s %s %s: %+v", brand, name, path, result)
				}
			}
		}
	}
	var users, members int
	if err := p.QueryRow(ctx, `SELECT (SELECT count(*) FROM global_users WHERE username='test'),(SELECT count(*) FROM brand_members)`).Scan(&users, &members); err != nil || users != 1 || members != 1 {
		t.Fatalf("users=%d members=%d err=%v", users, members, err)
	}
}
