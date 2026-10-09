package database_test

import (
	"context"

	"encoding/json"
	"errors"

	"strings"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const upgradeBrand = "0199a000-0000-7000-8000-000000000001"

func currentWallet(t *testing.T, db *pgxpool.Pool) (string, string) {
	t.Helper()
	ctx := context.Background()
	user, member, account := ids.New(), ids.New(), ids.New()
	for _, query := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO global_users(id,username) VALUES($1,$2)`, []any{user, "upgrade_" + strings.ReplaceAll(user, "-", "")}},
		{`INSERT INTO brand_members(id,brand_id,global_user_id,join_method,privacy_policy_version,service_terms_version) VALUES($1,$2,$3,'domain','dev-1','dev-1')`, []any{member, upgradeBrand, user}},
		{`INSERT INTO point_accounts(id,brand_id,brand_member_id) VALUES($1,$2,$3)`, []any{account, upgradeBrand, member}},
		{`INSERT INTO point_buckets(brand_id,account_id,source,state) SELECT $1,$2,s,t FROM unnest(ARRAY['recharge','winning','gift']) s CROSS JOIN unnest(ARRAY['available','manual_frozen','system_frozen','withdrawal']) t`, []any{upgradeBrand, account}},
	} {
		if _, err := db.Exec(ctx, query.sql, query.args...); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM point_buckets WHERE account_id=$1`, account).Scan(&count); err != nil || count != 16 {
		t.Fatal("wrong initialized source count", count, err)
	}
	return member, account
}

func TestCommissionSnapshotNormalizerIsClosedAndNewWritesRequireSixteen(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	member, account := currentWallet(t, db)
	for _, test := range []struct {
		sql  string
		want bool
	}{
		{`valid_point_snapshot(point_zero_snapshot(),false)`, true},
		{`valid_point_snapshot(point_zero_snapshot()-'commission',false)`, false},
		{`valid_point_snapshot(point_zero_snapshot()-'commission',false)`, false},
		{`validated_point_snapshot(point_zero_snapshot()-'commission') IS NULL`, true},
		{`validated_point_snapshot(point_zero_snapshot()-'gift') IS NULL`, true},
		{`validated_point_snapshot(jsonb_set(point_zero_snapshot(),'{commission}','{}')) IS NULL`, true},
		{`valid_point_snapshot(point_zero_snapshot()||' {"unknown":{}}'::jsonb,true)`, false},
		{`valid_point_snapshot(jsonb_set(point_zero_snapshot(),'{commission,available}','"-0"'),true)`, false},
		{`valid_point_snapshot(jsonb_set(point_zero_snapshot(),'{commission,available}','"9223372036854775808"'),true)`, false},
		{`valid_point_snapshot(jsonb_set(point_zero_snapshot(),'{commission,available}','"-9223372036854775808"'),true)`, true},
		{`valid_point_snapshot(jsonb_set(jsonb_set(point_zero_snapshot(),'{commission,available}','"9223372036854775807"'),'{gift,available}','"1"'),false)`, false},
	} {
		var valid bool
		if err := db.QueryRow(ctx, "SELECT "+test.sql).Scan(&valid); err != nil || valid != test.want {
			t.Fatal(test.sql, valid, err)
		}
	}
	var zero points.Balance
	var delta points.Balance
	delta[0][0] = 5
	after, _ := zero.Apply(delta)
	beforeJSON, _ := json.Marshal(zero)
	deltaJSON, _ := json.Marshal(delta)
	afterJSON, _ := json.Marshal(after)
	allocation, _ := json.Marshal([]points.Allocation{{Source: "recharge", State: "available", Points: 5}})
	insert := `INSERT INTO point_ledger_entries(id,brand_id,account_id,member_id,version,entry_type,reference_type,operation_key,before_snapshot,delta_snapshot,after_snapshot,source_allocation,reason,actor_type,request_id,request_hash)
 VALUES($1,$2,$3,$4,1,'adjustment','source_upgrade_test',$5,$6,$7,$8,$9,'test strict source evidence','system','source-upgrade-test',repeat('0',64))`
	for _, test := range []struct {
		name string
		edit func([]byte) []byte
	}{
		{"missing commission", func(raw []byte) []byte {
			var object map[string]json.RawMessage
			_ = json.Unmarshal(raw, &object)
			delete(object, "commission")
			result, _ := json.Marshal(object)
			return result
		}},
		{"missing commission state", func(raw []byte) []byte {
			return []byte(strings.Replace(string(raw), `"commission":{"available":"0","manual_frozen":"0","system_frozen":"0","withdrawal":"0"}`, `"commission":{"available":"0"}`, 1))
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			tx, err := db.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			_, err = tx.Exec(ctx, insert, ids.New(), upgradeBrand, account, member, ids.New(), test.edit(beforeJSON), deltaJSON, afterJSON, allocation)
			var pgerr *pgconn.PgError
			if !errors.As(err, &pgerr) || pgerr.ConstraintName != "ledger_four_source_snapshot" {
				t.Fatal("incomplete new snapshot admitted", err)
			}
		})
	}
	// Correct arithmetic alone cannot commit an economic entry while leaving
	// the materialized wallet or its audit witness unapplied.
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, insert, ids.New(), upgradeBrand, account, member, ids.New(), beforeJSON, deltaJSON, afterJSON, allocation); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err == nil {
		t.Fatal("unapplied ledger committed")
	}
	var entries int
	if err = db.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries WHERE account_id=$1`, account).Scan(&entries); err != nil || entries != 0 {
		t.Fatal("rejected projection leaked ledger", entries, err)
	}
	w, err := (points.Store{DB: db}).Read(ctx, upgradeBrand, member)
	if err != nil || w.Version != 0 || w.AvailablePoints != 0 {
		t.Fatal("rejected projection changed wallet", w, err)
	}
}

type oldBucket struct {
	Available points.Amount `json:"available"`
	Manual    points.Amount `json:"manual_frozen"`
	System    points.Amount `json:"system_frozen"`
	Withdraw  points.Amount `json:"withdrawal"`
}
type oldBalance struct {
	Recharge oldBucket `json:"recharge"`
	Winning  oldBucket `json:"winning"`
	Gift     oldBucket `json:"gift"`
}

// Match the pre-upgrade struct order and signed integer JSON exactly, not a
// map sorted into a different request encoding.
type oldChange struct {
	BrandID, MemberID, EntryType, ReferenceType, ReferenceID, OperationKey, Reason, ActorType, ActorID, RequestID, IP string
	Delta                                                                                                             oldBalance
	Allocation                                                                                                        []points.Allocation
	ReversalOf                                                                                                        string
}
