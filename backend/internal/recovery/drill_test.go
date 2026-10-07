//go:build recovery && !windows

package recovery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/database"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/migrations"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type drillNode struct {
	bin, root, data, name string
	port                  int
	started               bool
}

func (n *drillNode) command(t *testing.T, program string, args ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, filepath.Join(n.bin, program), args...)
	cmd.Env = drillCommandEnvironment(os.Environ())
	raw, err := cmd.CombinedOutput()
	// Commands operate only on owned, synthetic clusters. No original DSN or keys
	// enter their arguments; preserve diagnostics locally, never in the report.
	log, openErr := os.OpenFile(filepath.Join(n.root, n.name+"-commands.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if openErr != nil {
		t.Fatal(openErr)
	}
	_, writeErr := log.Write(raw)
	closeErr := log.Close()
	if writeErr != nil || closeErr != nil {
		t.Fatal("could not preserve drill diagnostics")
	}
	if err != nil {
		t.Fatalf("%s %s failed; inspect owned drill command log: %v", n.name, program, err)
	}
}

func (n *drillNode) start(t *testing.T, upstream int) {
	t.Helper()
	opts := fmt.Sprintf("-p %d -h 127.0.0.1 -k %s -c wal_level=replica -c max_wal_senders=8 -c max_replication_slots=4 -c hot_standby=on -c wal_keep_size=64MB", n.port, n.root)
	if upstream != 0 {
		opts += fmt.Sprintf(` -c "primary_conninfo=host=127.0.0.1 port=%d user=lottery_drill application_name=%s"`, upstream, n.name)
	}
	n.command(t, "pg_ctl", "-D", n.data, "-l", filepath.Join(n.root, n.name+"-server.log"), "-w", "-t", "20", "-o", opts, "start")
	n.started = true
}

func (n *drillNode) stop(t *testing.T) {
	t.Helper()
	if !n.started {
		return
	}
	n.command(t, "pg_ctl", "-D", n.data, "-m", "fast", "-w", "-t", "20", "stop")
	n.started = false
}

func (n *drillNode) pool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	cfg, err := pgxpool.ParseConfig(fmt.Sprintf("postgres://lottery_drill@127.0.0.1:%d/postgres?sslmode=disable", n.port))
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConns = 4
	cfg.ConnConfig.ConnectTimeout = 3 * time.Second
	cfg.ConnConfig.RuntimeParams["timezone"] = "UTC"
	p, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	if err = p.Ping(context.Background()); err != nil {
		t.Fatalf("owned node %s is unavailable", n.name)
	}
	return p
}

func waitDrill(t *testing.T, condition func() bool) {
	t.Helper()
	until := time.Now().Add(20 * time.Second)
	for time.Now().Before(until) {
		if condition() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("owned drill condition did not become true within 20 seconds")
}

func sameSnapshot(t *testing.T, p *pgxpool.Pool, want Snapshot) bool {
	t.Helper()
	got, err := SnapshotSchema(context.Background(), p, "public")
	return err == nil && got.SHA256 == want.SHA256 && got.RowCount == want.RowCount && len(got.Tables) == len(want.Tables)
}

func snapshotDrill(t *testing.T, p *pgxpool.Pool) Snapshot {
	t.Helper()
	s, err := SnapshotSchema(context.Background(), p, "public")
	if err != nil {
		t.Fatal("cannot snapshot drill schema", err)
	}
	return s
}

func setupDrill(t *testing.T) (string, string, int, *pgxpool.Pool) {
	t.Helper()
	if os.Getenv("LOTTERY_RECOVERY_CONFIRM_ISOLATED") != "yes" {
		t.Fatal("explicit LOTTERY_RECOVERY_CONFIRM_ISOLATED=yes required")
	}
	root, bin := os.Getenv("LOTTERY_RECOVERY_ROOT"), os.Getenv("LOTTERY_POSTGRES_BIN")
	if err := drillPaths(root, bin); err != nil {
		t.Fatal(err)
	}
	children, err := os.ReadDir(root)
	if err != nil || len(children) != 0 {
		t.Fatal("owned drill root must already exist and be completely empty")
	}
	for _, name := range []string{"initdb", "pg_ctl", "postgres", "pg_basebackup", "pg_dump", "pg_restore"} {
		info, err := os.Stat(filepath.Join(bin, name))
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
			t.Fatalf("missing executable PostgreSQL tool: %s", name)
		}
		raw, err := exec.Command(filepath.Join(bin, name), "--version").CombinedOutput()
		if err != nil || !strings.Contains(string(raw), " 17.") {
			t.Fatal("all owned drill tools must be PostgreSQL 17")
		}
	}
	base, err := drillBasePort(os.Getenv("LOTTERY_RECOVERY_BASE_PORT"))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		port := base + i
		listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err != nil {
			t.Fatal("owned drill ports must be unused")
		}
		listener.Close()
	}
	// This baseline is strictly read-only. Refuse remote hosts and any owned port.
	cfg, err := originalReadConfig(os.Getenv("LOTTERY_RECOVERY_ORIGINAL_DATABASE_URL"), base)
	if err != nil {
		t.Fatal(err)
	}
	p, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal("cannot open read-only baseline")
	}
	t.Cleanup(p.Close)
	marker, err := os.OpenFile(filepath.Join(root, "owned-isolated-drill"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if err = marker.Close(); err != nil {
		t.Fatal(err)
	}
	return root, bin, base, p
}

const drillBrand = "0199a000-0000-7000-8000-000000000001"

func postDrillPoints(t *testing.T, p *pgxpool.Pool, member string, n int64) {
	t.Helper()
	ctx := context.Background()
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	var delta points.Balance
	delta[2][0] = points.Amount(n)
	_, err = (points.Store{DB: p}).Post(ctx, tx, points.Change{BrandID: drillBrand, MemberID: member, EntryType: "adjustment", ReferenceType: "recovery_drill", OperationKey: "recovery-" + ids.New(), Reason: "synthetic isolated recovery funding", ActorType: "system", RequestID: ids.New(), Delta: delta, Allocation: []points.Allocation{{Source: "gift", State: "available", Points: points.Amount(n)}}})
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

func seedDrill(t *testing.T, p *pgxpool.Pool) string {
	t.Helper()
	ctx := context.Background()
	if err := database.Migrate(ctx, p); err != nil {
		t.Fatal(err)
	}
	if err := database.Seed(ctx, p, "test"); err != nil {
		t.Fatal(err)
	}
	user, member, account := ids.New(), ids.New(), ids.New()
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO global_users(id,username,password_hash) VALUES($1,'recovery_synthetic','not-a-login-hash')`, []any{user}},
		{`INSERT INTO brand_members(id,brand_id,global_user_id,join_method,privacy_policy_version,service_terms_version) VALUES($1,$2,$3,'domain','drill','drill')`, []any{member, drillBrand, user}},
		{`INSERT INTO point_accounts(id,brand_id,brand_member_id) VALUES($1,$2,$3)`, []any{account, drillBrand, member}},
		{`INSERT INTO point_buckets(brand_id,account_id,source,state) SELECT $1,$2,s,t FROM unnest(ARRAY['recharge','winning','gift','commission'])s CROSS JOIN unnest(ARRAY['available','manual_frozen','system_frozen','withdrawal'])t ON CONFLICT DO NOTHING`, []any{drillBrand, account}},
	} {
		if _, err = tx.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	postDrillPoints(t, p, member, 123)
	return member
}

func verifyRestoreGuards(t *testing.T, p *pgxpool.Pool) bool {
	t.Helper()
	ctx := context.Background()
	for _, q := range []string{`UPDATE point_ledger_entries SET reason='forbidden rewrite'`, `DELETE FROM point_ledger_entries`, `UPDATE audit_logs SET reason='forbidden rewrite'`, `DELETE FROM audit_logs`} {
		_, err := p.Exec(ctx, q)
		var pe *pgconn.PgError
		if !errors.As(err, &pe) || pe.Code != "P0001" {
			t.Fatal("restored immutable ledger/audit guard did not reject mutation")
		}
	}
	return true
}

func digestFile(t *testing.T, path string) (string, int64) {
	t.Helper()
	hash, n, err := FileDigest(path)
	if err != nil {
		t.Fatal(err)
	}
	return hash, n
}

func TestReplicationPromotionAndBackupRestore(t *testing.T) {
	root, bin, base, original := setupDrill(t)
	ctx := context.Background()
	originalBefore := snapshotDrill(t, original)
	nodes := make([]*drillNode, 4)
	for i, name := range []string{"primary", "replica1", "replica2", "restore"} {
		n := &drillNode{bin: bin, root: root, data: filepath.Join(root, name), name: name, port: base + i}
		nodes[i] = n
		t.Cleanup(func() {
			// pg_ctl can time out after starting a detached server. Check the
			// data directory we exclusively created, not just its success flag.
			status := exec.Command(filepath.Join(n.bin, "pg_ctl"), "-D", n.data, "status")
			status.Env = drillCommandEnvironment(os.Environ())
			if n.started || status.Run() == nil {
				n.started = true
				n.stop(t)
			}
		})
	}
	primary, replica1, replica2, restorer := nodes[0], nodes[1], nodes[2], nodes[3]
	primary.command(t, "initdb", "-D", primary.data, "-U", "lottery_drill", "-A", "trust", "--locale=C", "--encoding=UTF8")
	primary.start(t, 0)
	mainDB := primary.pool(t)
	member := seedDrill(t, mainDB)
	baseline := snapshotDrill(t, mainDB)
	names, err := fs.Glob(migrations.Files, "*.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	evidence := Evidence{Replicas: 2, BaselineTableCount: int64(len(baseline.Tables)), BaselineRowCount: baseline.RowCount, MigrationCount: baseline.MigrationCount, ExpectedMigrationCount: int64(len(names))}
	var recovery bool
	if err := mainDB.QueryRow(ctx, `SELECT pg_is_in_recovery()`).Scan(&recovery); err != nil || recovery {
		t.Fatal("initial primary is not primary")
	}
	evidence.InitialPrimaryWritable = true // Real ledger Post above has committed.
	readers := make([]*pgxpool.Pool, 2)
	for i, n := range []*drillNode{replica1, replica2} {
		n.command(t, "pg_basebackup", "-D", n.data, "-h", "127.0.0.1", "-p", strconv.Itoa(primary.port), "-U", "lottery_drill", "-X", "stream", "-R", "-c", "fast")
		n.start(t, primary.port)
		readers[i] = n.pool(t)
		waitDrill(t, func() bool { return sameSnapshot(t, readers[i], baseline) })
		if err := readers[i].QueryRow(ctx, `SELECT pg_is_in_recovery()`).Scan(&recovery); err != nil || !recovery {
			t.Fatal("replica is not in recovery")
		}
		_, err := readers[i].Exec(ctx, `CREATE TABLE forbidden_replica_write(n integer)`)
		var pe *pgconn.PgError
		if !errors.As(err, &pe) || pe.Code != "25006" {
			t.Fatal("replica accepted a write or returned a different error")
		}
		evidence.ReplicaReadOnlyRejects++
	}
	waitDrill(t, func() bool {
		var n int64
		err := mainDB.QueryRow(ctx, `SELECT count(*) FROM pg_stat_replication WHERE state='streaming'`).Scan(&n)
		evidence.StreamingReplicas = n
		return err == nil && n == 2
	})
	// Prove streaming changes after both base copies are already running.
	catchupStart := time.Now()
	postDrillPoints(t, mainDB, member, 7)
	baseline = snapshotDrill(t, mainDB)
	evidence.BaselineTableCount = int64(len(baseline.Tables))
	evidence.BaselineRowCount = baseline.RowCount
	for _, reader := range readers {
		waitDrill(t, func() bool { return sameSnapshot(t, reader, baseline) })
		evidence.ReplicaDigestMatches++
	}
	evidence.ReplicaCatchupSeconds = time.Since(catchupStart).Seconds()
	backup := filepath.Join(root, "business.dump")
	backupFile, err := os.OpenFile(backup, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if err = backupFile.Close(); err != nil {
		t.Fatal(err)
	}
	primary.command(t, "pg_dump", "-h", "127.0.0.1", "-p", strconv.Itoa(primary.port), "-U", "lottery_drill", "-d", "postgres", "-n", "public", "-Fc", "--no-owner", "--no-acl", "-f", backup)
	evidence.BackupSHA256, evidence.BackupBytes = digestFile(t, backup)
	primary.stop(t)
	status := exec.Command(filepath.Join(bin, "pg_ctl"), "-D", primary.data, "status").Run()
	pingCtx, cancel := context.WithTimeout(ctx, time.Second)
	pingErr := mainDB.Ping(pingCtx)
	cancel()
	if status == nil || pingErr == nil {
		t.Fatal("old primary was not fenced by an actual shutdown")
	}
	evidence.PrimaryStoppedBeforePromotion = true
	promotionStart := time.Now()
	replica1.command(t, "pg_ctl", "-D", replica1.data, "-w", "-t", "15", "promote")
	waitDrill(t, func() bool {
		err := readers[0].QueryRow(ctx, `SELECT pg_is_in_recovery()`).Scan(&recovery)
		return err == nil && !recovery
	})
	evidence.PromotionSeconds = time.Since(promotionStart).Seconds()
	evidence.PromotedDigestMatches = sameSnapshot(t, readers[0], baseline)
	postDrillPoints(t, readers[0], member, 50)
	wallet, err := (points.Store{DB: readers[0]}).Read(ctx, drillBrand, member)
	if err != nil || wallet.GiftPoints != 180 {
		t.Fatal("promoted ledger write is not visible")
	}
	evidence.PromotedWritable = true
	evidence.PromotedExtraWriteVisible = true
	promoted := snapshotDrill(t, readers[0])
	readers[1].Close()
	replica2.stop(t)
	replica2.start(t, replica1.port)
	readers[1] = replica2.pool(t)
	waitDrill(t, func() bool { return sameSnapshot(t, readers[1], promoted) })
	evidence.RemainingReplicaReattached = true
	evidence.RemainingReplicaDigestMatches = true
	restoreStart := time.Now()
	restorer.command(t, "initdb", "-D", restorer.data, "-U", "lottery_drill", "-A", "trust", "--locale=C", "--encoding=UTF8")
	restorer.start(t, 0)
	restoredDB := restorer.pool(t)
	if err = VerifyBackup(backup, evidence.BackupSHA256, evidence.BackupBytes); err != nil {
		t.Fatal(err)
	}
	restorer.command(t, "pg_restore", "-h", "127.0.0.1", "-p", strconv.Itoa(restorer.port), "-U", "lottery_drill", "-d", "postgres", "--single-transaction", "--exit-on-error", "--clean", "--if-exists", "--no-owner", "--no-acl", backup)
	restored := snapshotDrill(t, restoredDB)
	evidence.RestoreSeconds = time.Since(restoreStart).Seconds()
	evidence.RestoredDigestMatches = restored.SHA256 == baseline.SHA256
	evidence.RestoredTableCount = int64(len(restored.Tables))
	evidence.RestoredRowCount = restored.RowCount
	evidence.RestoreMigrationCount = restored.MigrationCount
	evidence.RestoredImmutableGuardsVerified = verifyRestoreGuards(t, restoredDB)
	restoredWallet, err := (points.Store{DB: restoredDB}).Read(ctx, drillBrand, member)
	if err != nil || restoredWallet.GiftPoints != 130 {
		t.Fatal("restored wallet is not the verified backup cutoff state")
	}
	if !sameSnapshot(t, restoredDB, baseline) {
		t.Fatal("failed immutable writes changed restored data")
	}
	if err = database.Migrate(ctx, restoredDB); err != nil {
		t.Fatal("restored migration checksums do not verify", err)
	}
	mainDB.Close()
	readers[0].Close()
	readers[1].Close()
	restoredDB.Close()
	restorer.stop(t)
	replica2.stop(t)
	replica1.stop(t)
	originalAfter := snapshotDrill(t, original)
	evidence.OriginalDatabaseUntouched = originalBefore.SHA256 == originalAfter.SHA256
	if err = Validate(evidence); err != nil {
		t.Fatal(err)
	}
	reportPath := os.Getenv("LOTTERY_RECOVERY_REPORT")
	if !filepath.IsAbs(reportPath) {
		t.Fatal("new absolute evidence report path required")
	}
	f, err := os.OpenFile(reportPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal(err)
	}
	var serverVersion string
	// Preserve the observed version from an owned node's startup metadata. Nodes
	// are stopped now; the --version command does not create or connect a server.
	version, versionErr := exec.Command(filepath.Join(bin, "postgres"), "--version").Output()
	if versionErr != nil {
		t.Fatal(versionErr)
	}
	serverVersion = strings.TrimSpace(string(version))
	artifact := map[string]any{"schema_version": 1, "scope": "isolated loopback PostgreSQL physical replication, manual fenced promotion, logical public-schema snapshot restoration; synthetic ledger/audit; no automatic endpoint failover, secrets backup or WAL archive/PITR", "postgres_version": serverVersion, "evidence": evidence, "baseline": baseline, "promoted": promoted, "restored": restored, "original_digest_before": originalBefore.SHA256, "original_digest_after": originalAfter.SHA256, "owned_nodes_stopped": true, "auth_keys_restored": false, "pitr_verified": false, "application_endpoint_failover_verified": false}
	artifact["recorded_at_utc"] = time.Now().UTC()
	artifact["go_version"] = runtime.Version()
	artifact["os"] = runtime.GOOS
	artifact["arch"] = runtime.GOARCH
	artifact["logical_cpus"] = runtime.NumCPU()
	encoder := json.NewEncoder(f)
	encoder.SetIndent("", "  ")
	err = encoder.Encode(artifact)
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		t.Fatal("cannot persist recovery evidence")
	}
	t.Logf("replicas=%d tables=%d rows=%d backup_bytes=%d catchup_seconds=%.3f promotion_seconds=%.3f restore_seconds=%.3f", evidence.Replicas, evidence.BaselineTableCount, evidence.BaselineRowCount, evidence.BackupBytes, evidence.ReplicaCatchupSeconds, evidence.PromotionSeconds, evidence.RestoreSeconds)
}
