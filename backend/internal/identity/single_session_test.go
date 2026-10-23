package identity

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestSingleSessionConcurrentLoginAndRollback(t *testing.T) {
	for _, kind := range []string{"user", "brand", "platform"} {
		t.Run(kind, func(t *testing.T) {
			db := testdb.New(t)
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			s, err := New(db)
			if err != nil {
				t.Fatal(err)
			}
			second, err := New(db)
			if err != nil {
				t.Fatal(err)
			}
			password := "single-session-password"
			hash, err := s.PasswordHash(ctx, password)
			if err != nil {
				t.Fatal(err)
			}
			id, username := ids.New(), "single_"+kind
			var old string
			if kind == "user" {
				f := newUserSessionFixture(t, ctx, db)
				id, old = f.user, f.token
				_, err = db.Exec(ctx, `UPDATE global_users SET username=$2,password_hash=$3 WHERE id=$1`, id, username, hash)
			} else {
				_, err = db.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash,is_super_admin) VALUES($1,$2,$3,$4)`, id, username, hash, kind == "platform")
			}
			if err != nil {
				t.Fatal(err)
			}
			login := func(store *Store, pass string, commit bool) (string, error) {
				tx, e := db.Begin(ctx)
				if e != nil {
					return "", e
				}
				defer tx.Rollback(ctx)
				var r mutation.Result
				input := LoginInput{Identifier: username, Password: pass}
				if kind == "user" {
					r, e = store.Login(ctx, tx, operatorTestBrand, input, Metadata{})
				} else {
					r, e = store.AdminLogin(ctx, tx, operatorTestBrand, input, Metadata{}, kind == "platform")
				}
				if e != nil {
					return "", e
				}
				if r.Status != 200 {
					if r.Error != nil && r.Error.Code == "AUTH_INVALID_CREDENTIALS" {
						return "", nil
					}
					return "", errors.New(string(r.Data))
				}
				var auth AdminAuthentication
				if e = json.Unmarshal(r.Data, &auth); e != nil {
					return "", e
				}
				if commit {
					e = tx.Commit(ctx)
				}
				return auth.AccessToken, e
			}
			authenticate := func(token string) error {
				if kind == "user" {
					_, e := s.Authenticate(ctx, operatorTestBrand, token)
					return e
				}
				_, e := s.AdminAuthenticate(ctx, token)
				return e
			}
			if old == "" {
				old, err = login(s, password, true)
				if err != nil {
					t.Fatal(err)
				}
			}
			if _, err = login(s, "wrong-password", true); err != nil {
				t.Fatal(err)
			}
			if err = authenticate(old); err != nil {
				t.Fatalf("failed login revoked old session: %v", err)
			}
			rolled, err := login(s, password, false)
			if err != nil {
				t.Fatal(err)
			}
			if err = authenticate(old); err != nil {
				t.Fatalf("rollback revoked old session: %v", err)
			}
			if err = authenticate(rolled); !errors.Is(err, ErrSession) {
				t.Fatalf("rolled-back token valid: %v", err)
			}
			const count = 4
			tokens := make([]string, count)
			errs := make([]error, count)
			start := make(chan struct{})
			var wg sync.WaitGroup
			for i := range count {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					<-start
					store := s
					if i%2 == 1 {
						store = second
					}
					tokens[i], errs[i] = login(store, password, true)
				}(i)
			}
			close(start)
			wg.Wait()
			active := 0
			for i, token := range tokens {
				if errs[i] != nil {
					t.Fatal(errs[i])
				}
				if e := authenticate(token); e == nil {
					active++
				} else if !errors.Is(e, ErrSession) {
					t.Fatal(e)
				}
			}
			if active != 1 {
				t.Fatalf("active concurrent logins=%d", active)
			}
			if err = authenticate(old); !errors.Is(err, ErrSession) {
				t.Fatalf("old session still valid: %v", err)
			}
			replaced, err := s.SessionReplaced(ctx, old, operatorTestBrand, kind != "user")
			if err != nil || !replaced {
				t.Fatalf("replacement reason missing: %v %v", replaced, err)
			}
			var stored int
			if err = db.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE (user_id=$1 OR admin_id=$1) AND revoked_at IS NULL`, id).Scan(&stored); err != nil || stored != 1 {
				t.Fatalf("stored active=%d err=%v", stored, err)
			}
			// The database also rejects writes bypassing the login service.
			if kind == "user" {
				_, err = db.Exec(ctx, `INSERT INTO sessions(id,token_hash,user_id,member_id,brand_id,expires_at) SELECT $1,$2,user_id,member_id,brand_id,expires_at FROM sessions WHERE user_id=$3 AND revoked_at IS NULL`, ids.New(), ids.New(), id)
			} else {
				_, err = db.Exec(ctx, `INSERT INTO sessions(id,token_hash,admin_id,expires_at) VALUES($1,$2,$3,clock_timestamp()+interval '1 hour')`, ids.New(), ids.New(), id)
			}
			if !unique(err) {
				t.Fatalf("unique session constraint not enforced: %v", err)
			}
			tx, err := db.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			if kind == "user" {
				_, err = s.AuthenticateTx(ctx, tx, operatorTestBrand, old)
			} else {
				_, err = s.AdminAuthenticateTx(ctx, tx, old)
			}
			if !errors.Is(err, ErrSession) {
				t.Fatalf("transaction accepted old token: %v", err)
			}
		})
	}
}

// All new session issuance, including Telegram, goes through the same revocation.
func TestSingleSessionReplacementAcrossBrands(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	s, err := New(db)
	if err != nil {
		t.Fatal(err)
	}
	f := newUserSessionFixture(t, ctx, db)
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT id FROM global_users WHERE id=$1 FOR UPDATE`, f.user); err != nil {
		t.Fatal(err)
	}
	member, err := s.joinCode(ctx, tx, operatorTestOtherBrand, f.user, "dev-1", "dev-1", "", "", Metadata{})
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.issue(ctx, tx, View{User: User{ID: f.user}, Member: member}, Metadata{}, "user.login")
	if err != nil || r.Status != 200 {
		t.Fatalf("issue: %+v %v", r, err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Authenticate(ctx, f.brand, f.token); !errors.Is(err, ErrSession) {
		t.Fatalf("old brand session valid: %v", err)
	}
	replaced, err := s.SessionReplaced(ctx, f.token, operatorTestOtherBrand, false)
	if err != nil || replaced {
		t.Fatalf("wrong brand disclosed replacement: %v %v", replaced, err)
	}
}

func TestSingleSessionBaselineRejectsDuplicateActiveSession(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	f := newUserSessionFixture(t, ctx, db)
	duplicate := ids.New()
	_, err := db.Exec(ctx, `INSERT INTO sessions(id,token_hash,user_id,member_id,brand_id,expires_at) VALUES($1,$2,$3,$4,$5,clock_timestamp()+interval '1 hour')`, duplicate, ids.New(), f.user, f.member, f.brand)
	var sqlError *pgconn.PgError
	if !errors.As(err, &sqlError) || sqlError.Code != "23505" || sqlError.ConstraintName != "sessions_one_active_user" {
		t.Fatalf("baseline accepted duplicate active session: %v", err)
	}
	// Revoked history remains insertable without replacing the active session.
	if _, err = db.Exec(ctx, `INSERT INTO sessions(id,token_hash,user_id,member_id,brand_id,expires_at,revoked_at) VALUES($1,$2,$3,$4,$5,clock_timestamp()+interval '1 hour',clock_timestamp())`, duplicate, ids.New(), f.user, f.member, f.brand); err != nil {
		t.Fatal(err)
	}
	var active int
	if err = db.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE user_id=$1 AND revoked_at IS NULL`, f.user).Scan(&active); err != nil || active != 1 {
		t.Fatalf("active session changed: %d %v", active, err)
	}
	var originalActive bool
	if err = db.QueryRow(ctx, `SELECT revoked_at IS NULL AND NOT replaced_by_login FROM sessions WHERE id=$1`, f.session).Scan(&originalActive); err != nil || !originalActive {
		t.Fatalf("original session changed: %v %v", originalActive, err)
	}
}
