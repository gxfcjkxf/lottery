package identity

import (
	"context"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/authcrypto"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
)

func TestAdminPasswordLoginEnforcesEntryInAllModes(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	hash, err := authcrypto.HashPassword("admin123")
	if err != nil {
		t.Fatal(err)
	}
	adminID := ids.New()
	if _, err = db.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash,is_super_admin) VALUES($1,'admin',$2,true)`, adminID, hash); err != nil {
		t.Fatal(err)
	}

	login := func(store *Store, platform bool, password string) mutation.Result {
		t.Helper()
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		result, err := store.AdminLogin(ctx, tx, "0199a000-0000-7000-8000-000000000001", LoginInput{Identifier: " ADMIN ", Password: password}, Metadata{RequestID: ids.New()}, platform)
		if err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		return result
	}

	development, err := New(db)
	if err != nil {
		t.Fatal(err)
	}
	development.Development = true
	production, err := New(db)
	if err != nil {
		t.Fatal(err)
	}
	if production.Development {
		t.Fatal("development mode must default to false")
	}
	for _, mode := range []struct {
		name  string
		store *Store
	}{{"development", development}, {"default", production}} {
		t.Run(mode.name, func(t *testing.T) {
			if result := login(mode.store, true, "admin123"); result.Status != 200 {
				t.Fatalf("platform password login=%+v, want 200", result)
			}
			if result := login(mode.store, false, "admin123"); result.Status != 403 || result.Error == nil || result.Error.Code != "ADMIN_ENTRY_MISMATCH" {
				t.Fatalf("brand-entry login=%+v, want 403 ADMIN_ENTRY_MISMATCH", result)
			}
			if result := login(mode.store, true, "wrong-password"); result.Status != 401 || result.Error == nil || result.Error.Code != "AUTH_INVALID_CREDENTIALS" {
				t.Fatalf("wrong-password login=%+v, want 401 AUTH_INVALID_CREDENTIALS", result)
			}
		})
	}

	var sessions, active, replaced int
	if err := db.QueryRow(ctx, `SELECT count(*), count(*) FILTER (WHERE revoked_at IS NULL), count(*) FILTER (WHERE revoked_at IS NOT NULL AND replaced_by_login) FROM sessions WHERE admin_id=$1`, adminID).Scan(&sessions, &active, &replaced); err != nil || sessions != 2 || active != 1 || replaced != 1 {
		t.Fatalf("unexpected admin sessions: total=%d active=%d replaced=%d err=%v", sessions, active, replaced, err)
	}
}
