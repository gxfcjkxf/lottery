package identity

import (
	"context"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/authcrypto"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
)

func TestDevelopmentAdminLoginIsPlatformOnlyAndOptIn(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	hash, err := authcrypto.HashDevelopmentAdminPassword()
	if err != nil {
		t.Fatal(err)
	}
	adminID := ids.New()
	if _, err = db.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash,is_super_admin) VALUES($1,'admin',$2,true)`, adminID, hash); err != nil {
		t.Fatal(err)
	}

	login := func(store *Store, platform bool) int {
		t.Helper()
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		result, err := store.AdminLogin(ctx, tx, "0199a000-0000-7000-8000-000000000001", LoginInput{Identifier: " ADMIN ", Password: "admin123"}, Metadata{RequestID: ids.New()}, platform)
		if err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		return result.Status
	}

	development, err := New(db)
	if err != nil {
		t.Fatal(err)
	}
	development.Development = true
	if status := login(development, true); status != 200 {
		t.Fatalf("development platform login status=%d, want 200", status)
	}
	if status := login(development, false); status != 401 {
		t.Fatalf("development brand-entry login status=%d, want 401", status)
	}
	production, err := New(db)
	if err != nil {
		t.Fatal(err)
	}
	if production.Development {
		t.Fatal("development mode must default to false")
	}
	if status := login(production, true); status != 401 {
		t.Fatalf("default-mode platform login status=%d, want 401", status)
	}

	var sessions int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE admin_id=$1`, adminID).Scan(&sessions); err != nil || sessions != 1 {
		t.Fatalf("unexpected development admin sessions=%d err=%v", sessions, err)
	}
}
