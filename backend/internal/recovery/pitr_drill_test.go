//go:build recovery && !windows

package recovery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestNamedRestorePointPITR(t *testing.T) {
	root, bin, base, original := setupDrill(t)
	ctx := context.Background()
	originalBefore := snapshotDrill(t, original)
	archive := filepath.Join(root, "pitr-archive")
	if err := os.Mkdir(archive, 0700); err != nil {
		t.Fatal(err)
	}
	archiveInfo, err := os.Stat(archive)
	if err != nil || archiveInfo.Mode().Perm() != 0700 {
		t.Fatal("owned WAL archive directory must have mode 0700")
	}
	archiveCommand := filepath.Join(root, "pitr-archive-command")
	archiveScript := "#!/bin/sh\nset -eu\nsrc=$1\nname=$2\ndst='" + archive + "/'\"$name\"\nif [ -e \"$dst\" ]; then cmp -s \"$src\" \"$dst\"; exit $?; fi\nif (set -C; cat \"$src\" > \"$dst\") 2>/dev/null; then exit 0; fi\ncmp -s \"$src\" \"$dst\"\n"
	if err := os.WriteFile(archiveCommand, []byte(archiveScript), 0700); err != nil {
		t.Fatal(err)
	}

	source := &drillNode{bin: bin, root: root, data: filepath.Join(root, "pitr-source"), name: "pitr-source", port: base}
	restorer := &drillNode{bin: bin, root: root, data: filepath.Join(root, "pitr-restorer"), name: "pitr-restorer", port: base + 1}
	for _, node := range []*drillNode{source, restorer} {
		n := node
		t.Cleanup(func() {
			status := exec.Command(filepath.Join(n.bin, "pg_ctl"), "-D", n.data, "status")
			status.Env = drillCommandEnvironment(os.Environ())
			if n.started || status.Run() == nil {
				n.started = true
				n.stop(t)
			}
		})
	}

	source.command(t, "initdb", "-D", source.data, "-U", "lottery_drill", "-A", "trust", "--locale=C", "--encoding=UTF8")
	conf, err := os.OpenFile(filepath.Join(source.data, "postgresql.conf"), os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := fmt.Fprintf(conf, "\narchive_mode = on\narchive_command = '%s \"%%p\" \"%%f\"'\n", archiveCommand)
	closeErr := conf.Close()
	if writeErr != nil || closeErr != nil {
		t.Fatal("could not configure owned WAL archiving")
	}
	source.start(t, 0)
	sourceDB := source.pool(t)
	member := seedDrill(t, sourceDB)
	baseline := snapshotDrill(t, sourceDB)
	// The physical backup is deliberately taken before the named target. Recovery
	// must replay archived WAL from this base to the restore point.
	basebackup := filepath.Join(root, "pitr-basebackup")
	source.command(t, "pg_basebackup", "-D", basebackup, "-h", "127.0.0.1", "-p", strconv.Itoa(source.port), "-U", "lottery_drill", "-X", "stream", "-c", "fast", "-F", "plain")
	basebackupBeforeTarget := true
	beforeTargetWallet, err := (points.Store{DB: sourceDB}).Read(ctx, drillBrand, member)
	if err != nil {
		t.Fatal("cannot read synthetic baseline wallet", err)
	}
	postDrillPoints(t, sourceDB, member, 7)
	target := snapshotDrill(t, sourceDB)
	var targetLSN string
	if err := sourceDB.QueryRow(ctx, `SELECT pg_create_restore_point('lottery_pitr_target')::text`).Scan(&targetLSN); err != nil {
		t.Fatal("could not create named PITR target", err)
	}
	targetWallet, err := (points.Store{DB: sourceDB}).Read(ctx, drillBrand, member)
	if err != nil {
		t.Fatal("cannot read target wallet", err)
	}
	postDrillPoints(t, sourceDB, member, 50)
	postTarget := snapshotDrill(t, sourceDB)
	postTargetWallet, err := (points.Store{DB: sourceDB}).Read(ctx, drillBrand, member)
	if err != nil {
		t.Fatal(err)
	}
	var walFile string
	if err := sourceDB.QueryRow(ctx, `SELECT pg_walfile_name($1::pg_lsn)`, targetLSN).Scan(&walFile); err != nil {
		t.Fatal("could not identify target WAL segment", err)
	}
	if _, err := sourceDB.Exec(ctx, `SELECT pg_switch_wal()`); err != nil {
		t.Fatal("could not switch source WAL", err)
	}
	archivedPath := filepath.Join(archive, walFile)
	archiveDone := filepath.Join(source.data, "pg_wal", "archive_status", walFile+".done")
	waitDrill(t, func() bool {
		_, archiveErr := os.Stat(archivedPath)
		_, doneErr := os.Stat(archiveDone)
		return archiveErr == nil && doneErr == nil
	})
	archiveHash, archiveBytes := digestFile(t, archivedPath)
	sourceHash, sourceBytes := digestFile(t, filepath.Join(source.data, "pg_wal", walFile))
	if archiveHash != sourceHash || archiveBytes != sourceBytes {
		t.Fatal("archived target WAL differs from the source WAL segment")
	}
	beforeTargetGift := int64(beforeTargetWallet.GiftPoints)
	targetGift := int64(targetWallet.GiftPoints)
	postTargetGift := int64(postTargetWallet.GiftPoints)
	if beforeTargetGift != 123 || targetGift != 130 || postTargetGift != 180 {
		t.Fatalf("synthetic wallet values differ from expected checkpoints: before=%d target=%d post-target=%d", beforeTargetGift, targetGift, postTargetGift)
	}

	source.stop(t)
	status := exec.Command(filepath.Join(bin, "pg_ctl"), "-D", source.data, "status")
	status.Env = drillCommandEnvironment(os.Environ())
	pingCtx, cancel := context.WithTimeout(ctx, time.Second)
	pingErr := sourceDB.Ping(pingCtx)
	cancel()
	primaryStopped := status.Run() != nil && pingErr != nil
	if !primaryStopped {
		t.Fatal("source was not stopped and connection-fenced before recovery")
	}

	restoreConf, err := os.OpenFile(filepath.Join(basebackup, "postgresql.auto.conf"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	restoreCommand := "cp \"" + archive + "/%f\" \"%p\""
	_, writeErr = fmt.Fprintf(restoreConf, "\narchive_mode = off\nrestore_command = '%s'\nrecovery_target_name = 'lottery_pitr_target'\nrecovery_target_action = 'pause'\n", restoreCommand)
	closeErr = restoreConf.Close()
	if writeErr != nil || closeErr != nil {
		t.Fatal("could not configure owned PITR recovery")
	}
	recoverySignal, err := os.OpenFile(filepath.Join(basebackup, "recovery.signal"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if err = recoverySignal.Close(); err != nil {
		t.Fatal(err)
	}
	restorer.data = basebackup
	restoreStarted := time.Now()
	restorer.start(t, 0)
	recoveredDB := restorer.pool(t)
	var recovering, paused bool
	waitDrill(t, func() bool {
		err := recoveredDB.QueryRow(ctx, `SELECT pg_is_in_recovery(), pg_is_wal_replay_paused()`).Scan(&recovering, &paused)
		return err == nil && recovering && paused
	})
	var replayLSN string
	if err := recoveredDB.QueryRow(ctx, `SELECT pg_last_wal_replay_lsn()::text`).Scan(&replayLSN); err != nil || replayLSN != targetLSN {
		t.Fatalf("recovery paused at unexpected LSN: got %q want %q", replayLSN, targetLSN)
	}
	var readonly bool
	if err := recoveredDB.QueryRow(ctx, `SELECT current_setting('transaction_read_only')::boolean`).Scan(&readonly); err != nil || !readonly {
		t.Fatal("recovered database was not read-only at the PITR target")
	}
	_, writeError := recoveredDB.Exec(ctx, `CREATE TABLE forbidden_pitr_write(value integer)`)
	var pgError *pgconn.PgError
	if !errors.As(writeError, &pgError) || pgError.Code != "25006" {
		t.Fatal("paused PITR target did not reject an actual write")
	}
	recovered := snapshotDrill(t, recoveredDB)
	if recovered.SHA256 != target.SHA256 {
		t.Fatal("restored whole public schema does not match the target snapshot")
	}
	var recoveredGift int64
	if err := recoveredDB.QueryRow(ctx, `SELECT b.points FROM point_buckets b JOIN point_accounts a ON a.id=b.account_id AND a.brand_id=b.brand_id WHERE a.brand_id=$1 AND a.brand_member_id=$2 AND b.source='gift' AND b.state='available'`, drillBrand, member).Scan(&recoveredGift); err != nil || recoveredGift != 130 {
		t.Fatalf("paused target gift balance: got %d want 130, error: %v", recoveredGift, err)
	}
	restorer.command(t, "pg_ctl", "-D", restorer.data, "-w", "-t", "20", "promote")
	waitDrill(t, func() bool {
		return recoveredDB.QueryRow(ctx, `SELECT pg_is_in_recovery()`).Scan(&recovering) == nil && !recovering
	})
	recoveredWallet, err := (points.Store{DB: recoveredDB}).Read(ctx, drillBrand, member)
	if err != nil || int64(recoveredWallet.GiftPoints) != recoveredGift {
		t.Fatalf("promoted wallet does not match the paused target balance: %v", err)
	}
	writeTx, err := recoveredDB.Begin(ctx)
	if err != nil {
		t.Fatal("promoted database did not accept a write transaction", err)
	}
	if _, err = writeTx.Exec(ctx, `CREATE TEMP TABLE pitr_write_probe(value integer)`); err == nil {
		_, err = writeTx.Exec(ctx, `INSERT INTO pitr_write_probe VALUES (1)`)
	}
	rollbackErr := writeTx.Rollback(ctx)
	if err != nil || rollbackErr != nil {
		t.Fatal("promoted database failed a rolled-back temporary write")
	}
	restoredWritable := err == nil && rollbackErr == nil
	restoreSeconds := time.Since(restoreStarted).Seconds()
	guards := verifyRestoreGuards(t, recoveredDB)
	if !guards || !sameSnapshot(t, recoveredDB, target) {
		t.Fatal("promoted restore did not retain target data and immutable guards")
	}

	sourceDB.Close()
	recoveredDB.Close()
	restorer.stop(t)
	// Confirm both owned processes are stopped before reading the original again.
	ownedNodesStopped := true
	for _, node := range []*drillNode{source, restorer} {
		check := exec.Command(filepath.Join(bin, "pg_ctl"), "-D", node.data, "status")
		check.Env = drillCommandEnvironment(os.Environ())
		if check.Run() == nil {
			ownedNodesStopped = false
			t.Fatalf("owned node %s is still running", node.name)
		}
	}
	originalAfter := snapshotDrill(t, original)
	originalUntouched := originalBefore.SHA256 == originalAfter.SHA256
	if !originalUntouched {
		t.Fatal("original database changed during isolated PITR drill")
	}
	version, err := exec.Command(filepath.Join(bin, "postgres"), "--version").Output()
	if err != nil {
		t.Fatal("could not read PostgreSQL version", err)
	}
	reportPath := os.Getenv("LOTTERY_RECOVERY_REPORT")
	if !filepath.IsAbs(reportPath) || filepath.Clean(reportPath) != reportPath {
		t.Fatal("new absolute PITR evidence report path required")
	}
	archiveVerified := archiveHash == sourceHash && archiveBytes == sourceBytes
	postTargetExcluded := postTarget.SHA256 != recovered.SHA256
	wholeTableMatch := recovered.SHA256 == target.SHA256
	pitrVerified := basebackupBeforeTarget && archiveVerified && primaryStopped && readonly && restoredWritable && guards && wholeTableMatch && postTargetExcluded && originalUntouched && ownedNodesStopped
	report := map[string]any{
		"schema_version": 1, "profile": "named_restore_point_pitr", "scope": "isolated loopback PostgreSQL 17 named restore point PITR using an owned physical base backup and WAL archive; synthetic ledger only",
		"go_version": runtime.Version(), "postgres_version": strings.TrimSpace(string(version)), "restore_point_name": "lottery_pitr_target", "restore_point_lsn": targetLSN,
		"baseline": baseline, "target": target, "post_target": postTarget, "restored": recovered,
		"basebackup_before_restore_point": basebackupBeforeTarget, "target_wal_archived": archiveVerified, "archive_checksum_verified": archiveVerified,
		"archive_files":             []map[string]any{{"file": walFile, "bytes": archiveBytes, "sha256": archiveHash}},
		"before_target_gift_points": beforeTargetGift, "target_gift_points": targetGift, "post_target_primary_gift_points": postTargetGift, "recovered_gift_points": int64(recoveredWallet.GiftPoints),
		"post_target_write_excluded": postTargetExcluded, "whole_table_target_digest_matches": wholeTableMatch,
		"primary_stopped_before_recovery": primaryStopped, "recovered_readonly_at_target": readonly, "recovered_promoted_writable": restoredWritable, "restored_immutable_guards_verified": guards,
		"original_database_untouched": originalUntouched, "original_digest_before": originalBefore.SHA256, "original_digest_after": originalAfter.SHA256,
		"owned_nodes_stopped": ownedNodesStopped, "pitr_verified": pitrVerified, "automatic_failover_verified": false, "production_disaster_recovery_accepted": false, "auth_keys_restored": false,
		"restore_seconds": restoreSeconds, "recorded_at_utc": time.Now().UTC().Format(time.RFC3339),
	}
	f, err := os.OpenFile(reportPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal("could not exclusively create PITR evidence report", err)
	}
	if err := f.Chmod(0600); err != nil {
		_ = f.Close()
		t.Fatal("could not set PITR evidence report mode")
	}
	encoder := json.NewEncoder(f)
	encoder.SetIndent("", "  ")
	encodeErr := encoder.Encode(report)
	closeErr = f.Close()
	if encodeErr != nil || closeErr != nil {
		t.Fatal("could not persist PITR evidence report")
	}
}
