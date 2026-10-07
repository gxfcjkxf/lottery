package recovery

import (
	"context"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"github.com/jackc/pgx/v5"
)

func TestSnapshotDetectsSameCountDifferentContentAndRejectsMissingSchema(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	var schema string
	if err := db.QueryRow(ctx, `SELECT current_schema()`).Scan(&schema); err != nil {
		t.Fatal(err)
	}
	before, err := SnapshotSchema(ctx, db, schema)
	if err != nil {
		t.Fatal(err)
	}
	same, err := SnapshotSchema(ctx, db, schema)
	if err != nil || same.SHA256 != before.SHA256 {
		t.Fatal("unchanged snapshot differs", err)
	}
	if _, err = db.Exec(ctx, `UPDATE brands SET name=name||' changed' WHERE code='aurora'`); err != nil {
		t.Fatal(err)
	}
	after, err := SnapshotSchema(ctx, db, schema)
	if err != nil {
		t.Fatal(err)
	}
	if after.RowCount != before.RowCount || len(after.Tables) != len(before.Tables) || after.SHA256 == before.SHA256 || after.Tables["brands"].SHA256 == before.Tables["brands"].SHA256 {
		t.Fatal("same counts concealed changed content")
	}
	if _, err = SnapshotSchema(ctx, db, "missing' ; SELECT 1 --"); err == nil {
		t.Fatal("missing schema accepted")
	}
	if after.MigrationCount <= 0 || after.MigrationCount != before.MigrationCount {
		t.Fatal("invalid migration evidence")
	}
}

func TestStoredValidationSurvivesEmptyRestoreSearchPath(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	var schema string
	if err := db.QueryRow(ctx, `SELECT current_schema()`).Scan(&schema); err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SET LOCAL search_path=''`); err != nil {
		t.Fatal(err)
	}
	var valid bool
	query := `SELECT ` + pgx.Identifier{schema, "valid_agent_config"}.Sanitize() + `('{"enabled":false,"max_depth":5,"ratio_cap":"0","mode":"loss","cycle":"monthly"}'::jsonb,true)`
	if err = tx.QueryRow(ctx, query).Scan(&valid); err != nil || !valid {
		t.Fatalf("legitimate policy rejected under pg_restore search_path: valid=%v err=%v", valid, err)
	}
	query = `SELECT ` + pgx.Identifier{schema, "valid_positive_withdrawal_multiple"}.Sanitize() + `('"0.000001"'::jsonb)`
	if err = tx.QueryRow(ctx, query).Scan(&valid); err != nil || !valid {
		t.Fatalf("positive withdrawal N rejected under restore path: valid=%v err=%v", valid, err)
	}
	query = `SELECT ` + pgx.Identifier{schema, "valid_positive_withdrawal_multiple"}.Sanitize() + `('"0"'::jsonb)`
	if err = tx.QueryRow(ctx, query).Scan(&valid); err != nil || valid {
		t.Fatalf("zero withdrawal N accepted under restore path: valid=%v err=%v", valid, err)
	}
	var unpinned int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname=$1 AND NOT EXISTS(SELECT 1 FROM unnest(p.proconfig) c WHERE c LIKE 'search_path=%')`, schema).Scan(&unpinned); err != nil || unpinned != 0 {
		t.Fatalf("schema functions without pinned path=%d err=%v", unpinned, err)
	}
}

func TestStoredValidatorIgnoresCallerTemporaryShadow(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	var schema string
	if err := db.QueryRow(ctx, `SELECT current_schema()`).Scan(&schema); err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	for _, query := range []string{
		`CREATE TEMP TABLE recovery_shadow_marker(n integer)`,
		`CREATE FUNCTION pg_temp.valid_agent_ratio(v text) RETURNS boolean LANGUAGE sql IMMUTABLE AS 'SELECT false'`,
		`CREATE FUNCTION pg_temp.valid_withdrawal_multiple(v jsonb) RETURNS boolean LANGUAGE sql IMMUTABLE AS 'SELECT false'`,
		`SET LOCAL search_path = pg_temp, ` + pgx.Identifier{schema}.Sanitize() + `, pg_catalog`,
	} {
		if _, err = tx.Exec(ctx, query); err != nil {
			t.Fatal(err)
		}
	}
	var valid bool
	query := `SELECT ` + pgx.Identifier{schema, "valid_agent_config"}.Sanitize() + `('{"enabled":false,"max_depth":5,"ratio_cap":"0","mode":"loss","cycle":"monthly"}'::jsonb,true)`
	if err = tx.QueryRow(ctx, query).Scan(&valid); err != nil || !valid {
		t.Fatalf("caller shadow changed stored validator: valid=%v err=%v", valid, err)
	}
	query = `SELECT ` + pgx.Identifier{schema, "valid_positive_withdrawal_multiple"}.Sanitize() + `('"2.5"'::jsonb)`
	if err = tx.QueryRow(ctx, query).Scan(&valid); err != nil || !valid {
		t.Fatalf("caller shadow changed positive withdrawal validator: valid=%v err=%v", valid, err)
	}
}
