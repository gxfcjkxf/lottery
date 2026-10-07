package betting

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
)

func setWithdrawalMultiple(t *testing.T, f bettingFixture, gameID, source, multiple string) (int64, string) {
	t.Helper()
	ctx := context.Background()
	tx, err := f.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	var version int64
	var config string
	var scopeGame any
	if source == "game" {
		scopeGame = gameID
		if err = tx.QueryRow(ctx, `SELECT version,config::text FROM game_withdrawal_policies WHERE brand_id=$1 AND game_id=$2`, f.brand, gameID).Scan(&version, &config); err != nil {
			t.Fatal(err)
		}
	} else if err = tx.QueryRow(ctx, `SELECT version,config::text FROM brand_withdrawal_policies WHERE brand_id=$1`, f.brand).Scan(&version, &config); err != nil {
		t.Fatal(err)
	}
	if source == "game" {
		_, err = tx.Exec(ctx, `INSERT INTO withdrawal_policy_revisions(brand_id,game_id,version,config,changed_by,reason) VALUES($1,$2,$3,jsonb_set($4::jsonb,'{turnover_multiple}',to_jsonb($5::text)),$6,'snapshot test')`, f.brand, scopeGame, version+1, config, multiple, f.version.CreatedBy)
		if err == nil {
			_, err = tx.Exec(ctx, `UPDATE game_withdrawal_policies SET version=$3,config=jsonb_set(config,'{turnover_multiple}',to_jsonb($4::text)) WHERE brand_id=$1 AND game_id=$2`, f.brand, gameID, version+1, multiple)
		}
	} else {
		_, err = tx.Exec(ctx, `INSERT INTO withdrawal_policy_revisions(brand_id,version,config,changed_by,reason) VALUES($1,$2,jsonb_set($3::jsonb,'{turnover_multiple}',to_jsonb($4::text)),$5,'snapshot test')`, f.brand, version+1, config, multiple, f.version.CreatedBy)
		if err == nil {
			_, err = tx.Exec(ctx, `UPDATE brand_withdrawal_policies SET version=$2,config=jsonb_set(config,'{turnover_multiple}',to_jsonb($3::text)) WHERE brand_id=$1`, f.brand, version+1, multiple)
		}
	}
	if err != nil {
		t.Fatal(err)
	}
	var revisionID string
	if err = tx.QueryRow(ctx, `SELECT id::text FROM withdrawal_policy_revisions WHERE brand_id=$1 AND game_id IS NOT DISTINCT FROM $2::uuid AND version=$3`, f.brand, scopeGame, version+1).Scan(&revisionID); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return version + 1, revisionID
}

type withdrawalSnapshot struct {
	SchemaVersion   int    `json:"schema_version"`
	Multiple        string `json:"multiple"`
	Source          string `json:"source"`
	BrandVersion    string `json:"brand_version"`
	GameVersion     string `json:"game_version"`
	BrandRevisionID string `json:"brand_revision_id"`
	GameRevisionID  string `json:"game_revision_id"`
}

func readWithdrawalSnapshot(t *testing.T, f bettingFixture, orderID string) withdrawalSnapshot {
	t.Helper()
	var snapshot withdrawalSnapshot
	if err := f.db.QueryRow(context.Background(), `SELECT withdrawal_rule_snapshot FROM bet_orders WHERE id=$1`, orderID).Scan(&snapshot); err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func TestBetWithdrawalSnapshotCapturesBrandRevisionAndIsStable(t *testing.T) {
	f := newBettingFixture(t, storeTestBrand)
	fundBettingWallet(t, f, 20)
	first, err := placeBettingOrder(t, f, f.input, "withdraw-snapshot-001")
	if err != nil {
		t.Fatal(err)
	}
	var initialVersion int64
	var initialRevision string
	if err = f.db.QueryRow(context.Background(), `SELECT version,id::text FROM withdrawal_policy_revisions WHERE brand_id=$1 AND game_id IS NULL ORDER BY version LIMIT 1`, f.brand).Scan(&initialVersion, &initialRevision); err != nil {
		t.Fatal(err)
	}
	var defaultGameRevision string
	if err = f.db.QueryRow(context.Background(), `SELECT id::text FROM withdrawal_policy_revisions WHERE brand_id=$1 AND game_id=$2 AND version=1`, f.brand, f.game.ID).Scan(&defaultGameRevision); err != nil {
		t.Fatal(err)
	}
	initialGameVersion, gameOverrideRevision := setWithdrawalMultiple(t, f, f.game.ID, "game", "0.25")
	brandVersion, brandRevision := setWithdrawalMultiple(t, f, f.game.ID, "brand", "2")
	second, err := placeBettingOrder(t, f, f.input, "withdraw-snapshot-002")
	if err != nil {
		t.Fatal(err)
	}
	wantFirst := withdrawalSnapshot{SchemaVersion: 1, Multiple: "1", Source: "brand", BrandVersion: fmt.Sprint(initialVersion), GameVersion: "1", BrandRevisionID: initialRevision, GameRevisionID: defaultGameRevision}
	wantSecond := withdrawalSnapshot{SchemaVersion: 1, Multiple: "0.25", Source: "game", BrandVersion: fmt.Sprint(brandVersion), GameVersion: fmt.Sprint(initialGameVersion), BrandRevisionID: brandRevision, GameRevisionID: gameOverrideRevision}
	if got := readWithdrawalSnapshot(t, f, first.ID); got != wantFirst {
		t.Fatalf("first snapshot=%+v want %+v", got, wantFirst)
	}
	if got := readWithdrawalSnapshot(t, f, second.ID); got != wantSecond {
		t.Fatalf("second snapshot=%+v want %+v", got, wantSecond)
	}
}

func TestBetWithdrawalSnapshotBrandInheritanceAndReplay(t *testing.T) {
	f := newBettingFixture(t, storeTestBrand)
	fundBettingWallet(t, f, 20)
	brandVersion, brandRevision := setWithdrawalMultiple(t, f, f.game.ID, "brand", "2")
	var gameRevision string
	if err := f.db.QueryRow(context.Background(), `SELECT id::text FROM withdrawal_policy_revisions WHERE brand_id=$1 AND game_id=$2 AND version=1`, f.brand, f.game.ID).Scan(&gameRevision); err != nil {
		t.Fatal(err)
	}
	first, err := placeBettingOrder(t, f, f.input, "withdraw-snapshot-replay-1")
	if err != nil {
		t.Fatal(err)
	}
	want := withdrawalSnapshot{SchemaVersion: 1, Multiple: "2", Source: "brand", BrandVersion: fmt.Sprint(brandVersion), GameVersion: "1", BrandRevisionID: brandRevision, GameRevisionID: gameRevision}
	if got := readWithdrawalSnapshot(t, f, first.ID); got != want {
		t.Fatalf("inherited snapshot=%+v want %+v", got, want)
	}
	setWithdrawalMultiple(t, f, f.game.ID, "brand", "3")
	replayed, err := placeBettingOrder(t, f, f.input, "withdraw-snapshot-replay-1")
	if err != nil || replayed.ID != first.ID {
		t.Fatalf("replay=%+v first=%+v err=%v", replayed, first, err)
	}
	if got := readWithdrawalSnapshot(t, f, first.ID); got != want {
		t.Fatalf("replay recaptured snapshot: %+v want %+v", got, want)
	}
}

func TestBetWithdrawalSnapshotCannotBeUpdated(t *testing.T) {
	f := newBettingFixture(t, storeTestBrand)
	fundBettingWallet(t, f, 20)
	o, err := placeBettingOrder(t, f, f.input, "withdraw-snapshot-immut")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.Exec(context.Background(), `UPDATE bet_orders SET withdrawal_rule_snapshot='{}'::jsonb WHERE id=$1`, o.ID); err == nil {
		t.Fatal("snapshot update unexpectedly succeeded")
	}
}

func TestBetWithdrawalSnapshotFunctionsPinSearchPathAndValidateAfterRestore(t *testing.T) {
	f := newBettingFixture(t, storeTestBrand)
	fundBettingWallet(t, f, 20)
	o, err := placeBettingOrder(t, f, f.input, "withdraw-snapshot-searchpath")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	tx, err := f.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	var schema string
	if err = tx.QueryRow(ctx, `SELECT current_schema()`).Scan(&schema); err != nil {
		t.Fatal(err)
	}
	rows, err := tx.Query(ctx, `SELECT p.proname,p.proconfig[1] FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname=$1 AND p.proname=ANY($2)`, schema, []string{"valid_bet_withdrawal_snapshot", "capture_bet_withdrawal_snapshot", "guard_bet_withdrawal_snapshot_update"})
	if err != nil {
		t.Fatal(err)
	}
	paths := map[string]string{}
	for rows.Next() {
		var name string
		var path *string
		if err = rows.Scan(&name, &path); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		if path != nil {
			paths[name] = *path
		}
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		t.Fatal(err)
	}
	rows.Close()
	if len(paths) != 3 {
		t.Fatalf("pinned functions=%v, want all three new functions", paths)
	}
	for name, path := range paths {
		compact := strings.ReplaceAll(path, " ", "")
		want := "search_path=pg_catalog," + schema + ",pg_temp"
		if compact != want {
			t.Errorf("%s search_path=%q want %q", name, path, want)
		}
	}
	var snapshot []byte
	if err = tx.QueryRow(ctx, `SELECT withdrawal_rule_snapshot FROM bet_orders WHERE id=$1`, o.ID).Scan(&snapshot); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `SET LOCAL search_path TO ''`); err != nil {
		t.Fatal(err)
	}
	qualifiedFunction := pgx.Identifier{schema, "valid_bet_withdrawal_snapshot"}.Sanitize()
	var valid bool
	if err = tx.QueryRow(ctx, `SELECT `+qualifiedFunction+`($1::jsonb)`, string(snapshot)).Scan(&valid); err != nil || !valid {
		t.Fatalf("snapshot validation with empty restore search_path valid=%v err=%v", valid, err)
	}
}

func TestBetWithdrawalSnapshotLockBlocksPolicyWriterUntilBetCommit(t *testing.T) {
	f := newBettingFixture(t, storeTestBrand)
	fundBettingWallet(t, f, 20)
	ctx := context.Background()
	tx, err := f.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if err = lockWithdrawalSnapshotPolicies(ctx, tx, f.brand, f.game.ID); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		writer, beginErr := f.db.Begin(ctx)
		if beginErr != nil {
			done <- beginErr
			return
		}
		close(started)
		_, updateErr := writer.Exec(ctx, `INSERT INTO withdrawal_policy_revisions(brand_id,version,config,changed_by,reason) SELECT brand_id,version+1,jsonb_set(config,'{turnover_multiple}','"2"'::jsonb),$2,'lock test' FROM brand_withdrawal_policies WHERE brand_id=$1`, f.brand, f.version.CreatedBy)
		if updateErr == nil {
			_, updateErr = writer.Exec(ctx, `UPDATE brand_withdrawal_policies SET version=version+1,config=jsonb_set(config,'{turnover_multiple}','"2"'::jsonb) WHERE brand_id=$1`, f.brand)
		}
		if updateErr == nil {
			updateErr = writer.Commit(ctx)
		} else {
			_ = writer.Rollback(ctx)
		}
		done <- updateErr
	}()
	<-started
	select {
	case err = <-done:
		t.Fatalf("policy writer passed bet-time SHARE lock: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
}

func TestBetWithdrawalSnapshotRollbackLeavesNoBetOrDebit(t *testing.T) {
	f := newBettingFixture(t, storeTestBrand)
	fundBettingWallet(t, f, 20)
	before := walletBySource(t, f)
	tx, err := f.db.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	o, err := f.service.Place(context.Background(), tx, f.brand, f.user, f.input, "withdraw-snapshot-rollback", points.Metadata{RequestID: ids.New()})
	if err != nil {
		_ = tx.Rollback(context.Background())
		t.Fatal(err)
	}
	if err = tx.Rollback(context.Background()); err != nil {
		t.Fatal(err)
	}
	if after := walletBySource(t, f); after != before {
		t.Fatalf("wallet after rollback=%+v want %+v", after, before)
	}
	var exists bool
	if err = f.db.QueryRow(context.Background(), `SELECT EXISTS(SELECT 1 FROM bet_orders WHERE id=$1)`, o.ID).Scan(&exists); err != nil || exists {
		t.Fatalf("rolled back bet exists=%v err=%v", exists, err)
	}
}
