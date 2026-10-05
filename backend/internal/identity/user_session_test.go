package identity

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
)

type userSessionFixture struct {
	brand, user, member, session, token string
}

func newUserSessionFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool) userSessionFixture {
	t.Helper()
	f := userSessionFixture{brand: operatorTestBrand, user: ids.New(), member: ids.New(), session: ids.New(), token: strings.Repeat("t", 43)}
	username := "auth_tx_" + strings.ReplaceAll(f.user, "-", "")
	if _, err := pool.Exec(ctx, `INSERT INTO global_users(id,username) VALUES($1,$2)`, f.user, username); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO brand_members(id,brand_id,global_user_id,join_method,privacy_policy_version,service_terms_version)
		VALUES($1,$2,$3,'domain','test-1','test-1')`, f.member, f.brand, f.user); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO sessions(id,token_hash,user_id,member_id,brand_id,expires_at)
		VALUES($1,$2,$3,$4,$5,clock_timestamp()+interval '1 hour')`, f.session, tokenHash(f.token), f.user, f.member, f.brand); err != nil {
		t.Fatal(err)
	}
	return f
}

func authenticateUserSessionTx(ctx context.Context, s *Store, pool *pgxpool.Pool, brand, token string) (Session, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return Session{}, err
	}
	defer tx.Rollback(ctx)
	got, err := s.AuthenticateTx(ctx, tx, brand, token)
	if err != nil {
		return got, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Session{}, err
	}
	return got, nil
}

func TestAuthenticateTxUserSessionConditionsAndReadOnly(t *testing.T) {
	pool := testdb.New(t)
	s, err := New(pool)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	f := newUserSessionFixture(t, ctx, pool)

	if _, err = pool.Exec(ctx, `UPDATE brands SET status='paused' WHERE id=$1`, f.brand); err != nil {
		t.Fatal(err)
	}
	got, err := authenticateUserSessionTx(ctx, s, pool, f.brand, f.token)
	if err != nil || got.User.ID != f.user || got.Member.ID != f.member || got.Member.Status != "normal" {
		t.Fatalf("paused brand, normal member should authenticate: session=%+v err=%v", got, err)
	}
	if _, err = pool.Exec(ctx, `UPDATE brand_members SET status='frozen' WHERE id=$1`, f.member); err != nil {
		t.Fatal(err)
	}
	got, err = authenticateUserSessionTx(ctx, s, pool, f.brand, f.token)
	if err != nil || got.Member.Status != "frozen" {
		t.Fatalf("frozen member should authenticate: session=%+v err=%v", got, err)
	}

	var hashBefore, hashAfter string
	var revokedBefore, revokedAfter *time.Time
	if err = pool.QueryRow(ctx, `SELECT token_hash,revoked_at FROM sessions WHERE id=$1`, f.session).Scan(&hashBefore, &revokedBefore); err != nil {
		t.Fatal(err)
	}
	if _, err = authenticateUserSessionTx(ctx, s, pool, "not-a-uuid", f.token); !errors.Is(err, ErrSession) {
		t.Fatalf("malformed brand should return ErrSession, got %v", err)
	}
	if _, err = authenticateUserSessionTx(ctx, s, pool, f.brand, "short"); !errors.Is(err, ErrSession) {
		t.Fatalf("malformed token should return ErrSession, got %v", err)
	}
	if _, err = authenticateUserSessionTx(ctx, s, pool, operatorTestOtherBrand, f.token); !errors.Is(err, ErrSession) {
		t.Fatalf("wrong brand should return ErrSession, got %v", err)
	}
	if _, err = pool.Exec(ctx, `UPDATE brand_members SET terms_accepted=false,accepted_at=NULL WHERE id=$1`, f.member); err != nil {
		t.Fatal(err)
	}
	if _, err = authenticateUserSessionTx(ctx, s, pool, f.brand, f.token); !errors.Is(err, ErrSession) {
		t.Fatalf("unaccepted terms should return ErrSession, got %v", err)
	}
	if _, err = pool.Exec(ctx, `UPDATE brand_members SET terms_accepted=true,accepted_at=clock_timestamp(),status='normal' WHERE id=$1`, f.member); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE brands SET status='disabled' WHERE id=$1`, f.brand); err != nil {
		t.Fatal(err)
	}
	if _, err = authenticateUserSessionTx(ctx, s, pool, f.brand, f.token); !errors.Is(err, ErrSession) {
		t.Fatalf("disabled brand should return ErrSession, got %v", err)
	}
	if err = pool.QueryRow(ctx, `SELECT token_hash,revoked_at FROM sessions WHERE id=$1`, f.session).Scan(&hashAfter, &revokedAfter); err != nil {
		t.Fatal(err)
	}
	if hashAfter != hashBefore || (revokedBefore == nil) != (revokedAfter == nil) {
		t.Fatalf("authentication changed session state: hash %q -> %q, revoked %v -> %v", hashBefore, hashAfter, revokedBefore, revokedAfter)
	}
}

func waitUntilBackendBlocked(t *testing.T, ctx context.Context, pool *pgxpool.Pool, pid int) {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		var blocked bool
		if err := pool.QueryRow(ctx, `SELECT cardinality(pg_blocking_pids($1))>0`, pid).Scan(&blocked); err != nil {
			t.Fatal(err)
		}
		if blocked {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("backend %d did not wait on the held member lock", pid)
}

func runAuthenticateWaitingOnMember(t *testing.T, ctx context.Context, s *Store, pool *pgxpool.Pool, brand, token string) (<-chan error, <-chan int) {
	t.Helper()
	result := make(chan error, 1)
	pidCh := make(chan int, 1)
	go func() {
		tx, err := pool.Begin(ctx)
		if err != nil {
			result <- err
			return
		}
		defer tx.Rollback(ctx)
		var pid int
		if err = tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
			result <- err
			return
		}
		pidCh <- pid
		_, err = s.AuthenticateTx(ctx, tx, brand, token)
		result <- err
	}()
	return result, pidCh
}

func TestAuthenticateTxChecksExpiryAfterMemberLockWait(t *testing.T) {
	pool := testdb.New(t)
	s, err := New(pool)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	f := newUserSessionFixture(t, ctx, pool)
	if _, err = pool.Exec(ctx, `UPDATE sessions SET expires_at=clock_timestamp()+interval '600 milliseconds' WHERE id=$1`, f.session); err != nil {
		t.Fatal(err)
	}
	blocker, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback(ctx)
	if _, err = blocker.Exec(ctx, `UPDATE brand_members SET display_name='locked' WHERE id=$1`, f.member); err != nil {
		t.Fatal(err)
	}
	result, pidCh := runAuthenticateWaitingOnMember(t, ctx, s, pool, f.brand, f.token)
	var pid int
	select {
	case pid = <-pidCh:
	case err = <-result:
		t.Fatalf("authentication finished before lock wait: %v", err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	waitUntilBackendBlocked(t, ctx, pool, pid)
	time.Sleep(750 * time.Millisecond)
	if err = blocker.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-result:
		if !errors.Is(err, ErrSession) {
			t.Fatalf("expired session should be rejected after lock wait, got %v", err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}

func TestAuthenticateTxSeesConcurrentRevocationAndMemberStatus(t *testing.T) {
	pool := testdb.New(t)
	s, err := New(pool)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	f := newUserSessionFixture(t, ctx, pool)

	for _, change := range []struct {
		name  string
		query string
		args  []any
	}{
		{name: "member disabled", query: `UPDATE brand_members SET status='disabled' WHERE id=$1`, args: []any{f.member}},
		{name: "session revoked", query: `UPDATE sessions SET revoked_at=clock_timestamp() WHERE id=$1`, args: []any{f.session}},
	} {
		t.Run(change.name, func(t *testing.T) {
			blocker, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer blocker.Rollback(ctx)
			if _, err = blocker.Exec(ctx, `UPDATE brand_members SET display_name=display_name WHERE id=$1`, f.member); err != nil {
				t.Fatal(err)
			}
			result, pidCh := runAuthenticateWaitingOnMember(t, ctx, s, pool, f.brand, f.token)
			var pid int
			select {
			case pid = <-pidCh:
			case err = <-result:
				t.Fatalf("authentication finished before lock wait: %v", err)
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			waitUntilBackendBlocked(t, ctx, pool, pid)
			if _, err = blocker.Exec(ctx, change.query, change.args...); err != nil {
				t.Fatal(err)
			}
			if err = blocker.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			select {
			case err = <-result:
				if !errors.Is(err, ErrSession) {
					t.Fatalf("concurrent change should be rejected, got %v", err)
				}
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			if change.name == "member disabled" {
				if _, err = pool.Exec(ctx, `UPDATE brand_members SET status='normal' WHERE id=$1`, f.member); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
