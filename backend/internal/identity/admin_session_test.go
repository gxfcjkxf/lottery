package identity

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
)

func TestAdminAuthenticateTxUsesWallClockAfterTransactionStarts(t *testing.T) {
	db := testdb.New(t)
	s, err := New(db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	admin, token := ids.New(), strings.Repeat("a", 43)
	if _, err = db.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,$2,'test-only')`, admin, "expiring_admin_"+admin); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `INSERT INTO sessions(id,token_hash,admin_id,expires_at) VALUES($1,$2,$3,clock_timestamp()+interval '500 milliseconds')`, ids.New(), tokenHash(token), admin); err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if got, e := s.AdminAuthenticateTx(ctx, tx, token); e != nil || got != admin {
		t.Fatalf("initial authentication=%s err=%v", got, e)
	}
	if _, err = tx.Exec(ctx, `SELECT pg_sleep(0.7)`); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AdminAuthenticateTx(ctx, tx, token); !errors.Is(err, ErrSession) {
		t.Fatalf("transaction-start time admitted expired session: %v", err)
	}
	if _, err = s.AdminAuthenticateTx(ctx, nil, token); !errors.Is(err, ErrSession) {
		t.Fatalf("nil transaction=%v", err)
	}
}
