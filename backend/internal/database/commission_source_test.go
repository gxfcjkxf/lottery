package database_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"sort"
	"strings"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/database"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"github.com/gxfcjkxf/lottery/backend/migrations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const upgradeBrand = "0199a000-0000-7000-8000-000000000001"

func upgradeWallet(t *testing.T, db *pgxpool.Pool, legacy bool) (string, string) {
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
	if err := db.QueryRow(ctx, `SELECT count(*) FROM point_buckets WHERE account_id=$1`, account).Scan(&count); err != nil || count != map[bool]int{true: 12, false: 16}[legacy] {
		t.Fatal("wrong initialized source count", count, err)
	}
	return member, account
}

func TestCommissionSnapshotNormalizerIsClosedAndNewWritesRequireSixteen(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	member, account := upgradeWallet(t, db, false)
	for _, test := range []struct {
		sql  string
		want bool
	}{
		{`valid_point_snapshot(point_zero_snapshot(),false,false)`, true},
		{`valid_point_snapshot(point_zero_snapshot()-'commission',false,false)`, false},
		{`valid_point_snapshot(point_zero_snapshot()-'commission',false,true)`, true},
		{`normalize_point_snapshot(point_zero_snapshot()-'commission')=point_zero_snapshot()`, true},
		{`normalize_point_snapshot(point_zero_snapshot()-'gift') IS NULL`, true},
		{`normalize_point_snapshot(jsonb_set(point_zero_snapshot(),'{commission}','{}')) IS NULL`, true},
		{`valid_point_snapshot(point_zero_snapshot()||' {"unknown":{}}'::jsonb,true,true)`, false},
		{`valid_point_snapshot(jsonb_set(point_zero_snapshot(),'{commission,available}','"-0"'),true,false)`, false},
		{`valid_point_snapshot(jsonb_set(point_zero_snapshot(),'{commission,available}','"9223372036854775808"'),true,false)`, false},
		{`valid_point_snapshot(jsonb_set(point_zero_snapshot(),'{commission,available}','"-9223372036854775808"'),true,false)`, true},
		{`valid_point_snapshot(jsonb_set(jsonb_set(point_zero_snapshot(),'{commission,available}','"9223372036854775807"'),'{gift,available}','"1"'),false,false)`, false},
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

func TestCommissionUpgradePreservesActualLegacyLedgerHashReplayAndReversal(t *testing.T) {
	base := testdb.New(t)
	ctx := context.Background()
	schema := "legacy_" + strings.ReplaceAll(ids.New(), "-", "")
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err := base.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		t.Fatal(err)
	}
	config := base.Config().Copy()
	config.ConnConfig.RuntimeParams["search_path"] = schema
	db, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Close()
		if _, err := base.Exec(ctx, "DROP SCHEMA "+quoted+" CASCADE"); err != nil {
			t.Error(err)
		}
	})
	// Apply the real previously committed schema, not a new schema with its
	// protections disabled. Only this randomly named, owned schema is changed.
	if _, err = db.Exec(ctx, `CREATE TABLE schema_migrations(name text PRIMARY KEY,checksum text NOT NULL,applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		t.Fatal(err)
	}
	names, _ := fs.Glob(migrations.Files, "*.up.sql")
	sort.Strings(names)
	for _, name := range names {
		if name >= "0046" {
			break
		}
		body, err := migrations.Files.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(ctx, string(body)); err != nil {
			t.Fatal(name, err)
		}
		h := sha256.Sum256(body)
		if _, err = db.Exec(ctx, `INSERT INTO schema_migrations(name,checksum) VALUES($1,$2)`, name, hex.EncodeToString(h[:])); err != nil {
			t.Fatal(err)
		}
	}
	if err = database.Seed(ctx, db, "test"); err != nil {
		t.Fatal(err)
	}
	member, account := upgradeWallet(t, db, true)
	entryID, operationKey := ids.New(), "legacy-source-credit:"+ids.New()
	oldDelta := oldBalance{Recharge: oldBucket{Available: 11}}
	oldBefore := oldBalance{}
	allocation := []points.Allocation{{Source: "recharge", State: "available", Points: 11}}
	c := oldChange{BrandID: upgradeBrand, MemberID: member, EntryType: "adjustment", ReferenceType: "source_upgrade_test", OperationKey: operationKey, Reason: "legacy source credit", ActorType: "system", Delta: oldDelta, Allocation: allocation}
	raw, _ := json.Marshal(c)
	h := sha256.Sum256(raw)
	requestHash := hex.EncodeToString(h[:])
	b, _ := json.Marshal(oldBefore)
	d, _ := json.Marshal(oldDelta)
	alloc, _ := json.Marshal(allocation)
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `INSERT INTO point_ledger_entries(id,brand_id,account_id,member_id,version,entry_type,reference_type,operation_key,before_snapshot,delta_snapshot,after_snapshot,source_allocation,reason,actor_type,request_id,request_hash)
 VALUES($1,$2,$3,$4,1,'adjustment','source_upgrade_test',$5,$6,$7,$7,$8,'legacy source credit','system','legacy-request',$9)`, entryID, upgradeBrand, account, member, operationKey, b, d, alloc, requestHash); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE point_buckets SET points=11 WHERE account_id=$1 AND source='recharge' AND state='available'`, account); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE point_accounts SET version=1 WHERE id=$1`, account); err != nil {
		t.Fatal(err)
	}
	if _, err = audit.Append(ctx, tx, audit.Record{BrandID: upgradeBrand, ActorType: "system", Action: "points.adjustment", ResourceType: "point_account", ResourceID: account, Reason: "legacy source credit", RequestID: "legacy-request", Before: oldBefore, After: map[string]any{"ledger_entry_id": entryID, "version": 1, "balance": oldDelta}}); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	fingerprint := func() string {
		var value string
		if err := db.QueryRow(ctx, `SELECT jsonb_build_object('ledger',(SELECT jsonb_agg(to_jsonb(l) ORDER BY id) FROM point_ledger_entries l),'audits',(SELECT jsonb_agg(to_jsonb(a) ORDER BY id) FROM audit_logs a),'policies',(SELECT jsonb_agg(to_jsonb(p) ORDER BY brand_id) FROM brand_withdrawal_policies p))::text`).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	before := fingerprint()
	if err = database.Migrate(ctx, db); err != nil {
		t.Fatal("actual legacy migration failed", err)
	}
	if fingerprint() != before {
		t.Fatal("source migration changed economic history, audit or default financial authorization")
	}
	s := points.Store{DB: db}
	w, err := s.Read(ctx, upgradeBrand, member)
	if err != nil || w.Version != 1 || w.RechargePoints != 11 || w.CommissionPoints != 0 {
		t.Fatal(w, err)
	}
	var delta points.Balance
	delta[0][0] = 11
	tx, err = db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	replay, err := s.Post(ctx, tx, points.Change{BrandID: upgradeBrand, MemberID: member, EntryType: c.EntryType, ReferenceType: c.ReferenceType, OperationKey: operationKey, Reason: c.Reason, ActorType: "system", RequestID: "new-request-id", Delta: delta, Allocation: allocation})
	if err != nil || replay.ID != entryID {
		t.Fatal("old operation failed exact hash replay", replay, err)
	}
	if err = tx.Commit(ctx); err != nil || fingerprint() != before {
		t.Fatal("legacy replay appended or rewrote evidence", err)
	}
	preview, err := s.PreviewRepair(ctx, upgradeBrand, member)
	if err != nil || !preview.Consistent || preview.Repairable {
		t.Fatal("intact old ledger no longer reconciles", preview, err)
	}
	tx, err = db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = s.Reverse(ctx, tx, upgradeBrand, member, entryID, "legacy-source-refund:"+ids.New(), "original source reversal after upgrade", points.Metadata{ActorType: "system", RequestID: ids.New()}); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal("legacy reversal did not commit sixteen-bucket proof", err)
	}
	w, err = s.Read(ctx, upgradeBrand, member)
	if err != nil || w.Version != 2 || w.AvailablePoints != 0 || w.CommissionPoints != 0 {
		t.Fatal("legacy reversal changed the original source", w, err)
	}
}
