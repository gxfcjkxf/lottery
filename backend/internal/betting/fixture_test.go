package betting

import (
	"context"
	"encoding/hex"
	"fmt"
	"github.com/gxfcjkxf/lottery/backend/internal/authcrypto"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"testing"
	"time"
)

const storeTestBrand = "0199a000-0000-7000-8000-000000000001"
const storeTestOtherBrand = "0199a000-0000-7000-8000-000000000002"

type bettingFixture struct {
	db                            *pgxpool.Pool
	service                       Service
	user                          identity.Session
	token, brand, member, adminID string
}

func bettingTx(t *testing.T, db interface {
	Begin(context.Context) (pgx.Tx, error)
}, fn func(pgx.Tx) error) {
	t.Helper()
	ctx := context.Background()
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if err = fn(tx); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

// Period admission is judged by PostgreSQL's clock, including in test fixtures.
func bettingDatabaseNow(t *testing.T, db *pgxpool.Pool) time.Time {
	t.Helper()
	var now time.Time
	if err := db.QueryRow(context.Background(), `SELECT clock_timestamp()`).Scan(&now); err != nil {
		t.Fatal(err)
	}
	return now.UTC()
}

func newBettingFixture(t *testing.T, brand string) bettingFixture {
	ctx := context.Background()
	db := testdb.New(t)
	f := bettingFixture{db: db, service: Service{DB: db}, brand: brand}
	creatorID, reviewerID, userID, memberID := ids.New(), ids.New(), ids.New(), ids.New()
	for i, id := range []string{creatorID, reviewerID} {
		if _, err := db.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,$2,'test-only')`, id, fmt.Sprintf("bet_rule_%s_%d", id[24:], i)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(ctx, `INSERT INTO global_users(id,username,password_hash) VALUES($1,$2,'test-only')`, userID, "bet_user_"+userID[24:]); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO brand_members(id,brand_id,global_user_id,join_method) VALUES($1,$2,$3,'domain')`, memberID, brand, userID); err != nil {
		t.Fatal(err)
	}
	accountID := ids.New()
	if _, err := db.Exec(ctx, `INSERT INTO point_accounts(id,brand_id,brand_member_id) VALUES($1,$2,$3)`, accountID, brand, memberID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO point_buckets(brand_id,account_id,source,state) SELECT $1,$2,s,t FROM unnest(ARRAY['recharge','winning','gift','commission'])s CROSS JOIN unnest(ARRAY['available','manual_frozen','system_frozen','withdrawal'])t ON CONFLICT DO NOTHING`, brand, accountID); err != nil {
		t.Fatal(err)
	}
	token, err := authcrypto.NewSessionToken()
	if err != nil {
		t.Fatal(err)
	}
	digest := authcrypto.DigestSessionToken(token)
	if _, err := db.Exec(ctx, `INSERT INTO sessions(id,token_hash,user_id,member_id,brand_id,expires_at) VALUES($1,$2,$3,$4,$5,clock_timestamp()+interval '1 hour')`, ids.New(), hex.EncodeToString(digest[:]), userID, memberID, brand); err != nil {
		t.Fatal(err)
	}
	// This fixture revalidates an already issued token, not a password login.
	// AuthenticateTx uses the real transaction and session/member locks; it
	// does not need New's intentionally expensive unknown-login password hash.
	identityStore := &identity.Store{DB: db}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	f.user, err = identityStore.AuthenticateTx(ctx, tx, brand, token)
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	f.member = memberID
	f.token = token

	f.adminID = creatorID
	return f
}
func fundBettingWallet(t *testing.T, f bettingFixture, amounts ...points.Amount) {
	t.Helper()
	ctx := context.Background()
	ps := points.Store{DB: f.db}
	for i, amount := range amounts {
		if amount == 0 {
			continue
		}
		var delta points.Balance
		delta[i][0] = amount
		allocation := []points.Allocation{{Source: []string{"recharge", "winning", "gift", "commission"}[i], State: "available", Points: amount}}
		tx, err := f.db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		_, err = ps.Post(ctx, tx, points.Change{BrandID: f.brand, MemberID: f.member, EntryType: "adjustment", ReferenceType: "test_fund", OperationKey: "bet-fund-" + ids.New(), Reason: "betting fixture funding", ActorType: "system", RequestID: ids.New(), Delta: delta, Allocation: allocation})
		if err != nil {
			_ = tx.Rollback(ctx)
			t.Fatal(err)
		}
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
}

func walletBySource(t *testing.T, f bettingFixture) points.Balance {
	t.Helper()
	wallet, err := (points.Store{DB: f.db}).Read(context.Background(), f.brand, f.member)
	if err != nil {
		t.Fatal(err)
	}
	return wallet.BySource
}
