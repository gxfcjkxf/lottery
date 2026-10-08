package database_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/database"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func archiveLegacyTableHashes(t *testing.T, db *pgxpool.Pool, tables []string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, name := range tables {
		var digest string
		if err := db.QueryRow(context.Background(), `SELECT md5(coalesce(jsonb_agg(to_jsonb(r) ORDER BY to_jsonb(r)::text),'[]'::jsonb)::text) FROM `+pgx.Identifier{name}.Sanitize()+` r`).Scan(&digest); err != nil {
			t.Fatal(name, err)
		}
		out[name] = digest
	}
	return out
}

func TestReportArchive0063To0065PreservesEveryLegacyTable(t *testing.T) {
	db := testdb.NewAtVersion(t, 63)
	ctx := context.Background()
	member, _ := upgradeWallet(t, db, false)
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	var delta points.Balance
	delta[0][0] = 37
	if _, err = (points.Store{DB: db}).Post(ctx, tx, points.Change{BrandID: upgradeBrand, MemberID: member, EntryType: "adjustment", ReferenceType: "archive_upgrade", OperationKey: "archive-upgrade:" + ids.New(), Reason: "actual pre-upgrade ledger posting", ActorType: "system", RequestID: ids.New(), Delta: delta, Allocation: []points.Allocation{{Source: "recharge", State: "available", Points: 37}}}); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	rows, err := db.Query(ctx, `SELECT tablename FROM pg_tables WHERE schemaname=current_schema() AND tablename<>'schema_migrations' ORDER BY tablename`)
	if err != nil {
		t.Fatal(err)
	}
	var tables []string
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		tables = append(tables, name)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(tables) < 60 {
		t.Fatal("upgrade table inventory incomplete", len(tables))
	}
	before := archiveLegacyTableHashes(t, db, tables)
	if err = database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if after := archiveLegacyTableHashes(t, db, tables); !reflect.DeepEqual(before, after) {
		t.Fatal("archive migrations rewrote legacy data", before, after)
	}
	var archiveCount, migrationCount int
	if err = db.QueryRow(ctx, `SELECT (SELECT count(*) FROM report_archives),(SELECT count(*) FROM schema_migrations WHERE name IN('0064_report_archive_capture.up.sql','0065_report_archive_versions.up.sql'))`).Scan(&archiveCount, &migrationCount); err != nil || archiveCount != 0 || migrationCount != 2 {
		t.Fatal(archiveCount, migrationCount, err)
	}
	var hardened int
	if err = db.QueryRow(ctx, `SELECT count(*) FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname=current_schema() AND p.proname IN('report_archive_capture','report_archive_period_boundary','guard_report_archive_version') AND p.proconfig=ARRAY['search_path=pg_catalog, '||current_schema()||', pg_temp']`).Scan(&hardened); err != nil || hardened != 3 {
		t.Fatal("archive helpers not pinned", hardened, err)
	}
	if err = database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if after := archiveLegacyTableHashes(t, db, tables); !reflect.DeepEqual(before, after) {
		t.Fatal("repeated upgrade changed legacy rows")
	}
	t.Logf("preserved %d legacy tables, including genuine ledger/bucket/audit records", len(tables))
}
