//go:build browserfixture

// report-archive-tasks-fixture prepares and advances an explicitly owned
// synthetic report-archive task. It is excluded from production builds.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/database"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/reportarchive"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	fixtureAck         = "owned_synthetic_database"
	fixtureBrand       = "0199a000-0000-7000-8000-000000000002"
	fixtureAdminPrefix = "archive_tasks_browser_"
	fixtureTrigger     = "report_archive_tasks_fixture_fail_capture"
	fixtureFunction    = "report_archive_tasks_fixture_fail_capture_fn"
)

var exactFixtureDSN = regexp.MustCompile(`^postgres(?:ql)?://lottery_test@(127\.0\.0\.1|localhost):(5432|55432)/lottery_archive_tasks_browser_(desktop|mobile)\?sslmode=disable$`)

type fixtureConfig struct {
	Database      string
	Viewport      string
	AdminUsername string
}

type fixtureOutput struct {
	TaskID       string  `json:"task_id"`
	BrandID      string  `json:"brand_id"`
	TaskVersion  int64   `json:"task_version"`
	State        string  `json:"state"`
	AttemptCount int64   `json:"attempt_count"`
	ArchiveID    *string `json:"archive_id"`
	ArchiveCount string  `json:"archive_count"`
	LedgerCount  string  `json:"ledger_count"`
	TotalPoints  string  `json:"total_points"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "report archive tasks fixture failed:", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) != 2 || (os.Args[1] != "prepare" && os.Args[1] != "advance" && os.Args[1] != "verify") {
		return errors.New("usage: report-archive-tasks-fixture prepare|advance|verify")
	}
	command := os.Args[1]
	cfg, err := readFixtureConfig(os.Getenv("DATABASE_URL"), os.Getenv("APP_ENV"), os.Getenv("REPORT_ARCHIVE_TASKS_FIXTURE_CONFIRM"), os.Getenv("REPORT_ARCHIVE_TASKS_VIEWPORT"), os.Getenv("DATABASE_READ_URL"), os.Getenv("DATABASE_READ_URLS"), os.Getenv("TEST_ARCHIVE_TASKS_ADMIN_USERNAME"))
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	poolConfig, err := pgxpool.ParseConfig(os.Getenv("DATABASE_URL"))
	if err != nil {
		return errors.New("owned fixture database unavailable")
	}
	db, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return errors.New("owned fixture database unavailable")
	}
	defer db.Close()
	if err = db.Ping(ctx); err != nil {
		return errors.New("owned fixture database unavailable")
	}
	if err = requireLatestMigration(ctx, db); err != nil {
		return err
	}
	var out fixtureOutput
	switch command {
	case "prepare":
		out, err = prepare(ctx, db, cfg.AdminUsername, cfg.Viewport)
	case "advance":
		out, err = advance(ctx, db)
	case "verify":
		out, err = verify(ctx, db)
	}
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(out)
}

func readFixtureConfig(raw, environment, confirmation, viewport, readURL, readURLs, adminUsername string) (fixtureConfig, error) {
	var out fixtureConfig
	if environment != "test" || confirmation != fixtureAck || (viewport != "desktop" && viewport != "mobile") {
		return out, errors.New("explicit owned report archive task test database required")
	}
	if readURL != "" || readURLs != "" {
		return out, errors.New("report archive task read database overrides are forbidden")
	}
	match := exactFixtureDSN.FindStringSubmatch(raw)
	if len(match) != 4 || match[3] != viewport {
		return out, errors.New("exact owned report archive task PostgreSQL URL required")
	}
	u, err := url.Parse(raw)
	if err != nil || u.User == nil || u.User.Username() != "lottery_test" || u.User.String() != "lottery_test" || u.Path != "/lottery_archive_tasks_browser_"+viewport || u.RawPath != "" || u.Fragment != "" || u.RawQuery != "sslmode=disable" || u.Opaque != "" {
		return out, errors.New("exact owned report archive task PostgreSQL URL required")
	}
	wantAdmin := fixtureAdminPrefix + viewport
	if adminUsername != wantAdmin {
		return out, errors.New("exact report archive task administrator username required")
	}
	return fixtureConfig{Database: strings.TrimPrefix(u.Path, "/"), Viewport: viewport, AdminUsername: wantAdmin}, nil
}

func requireLatestMigration(ctx context.Context, db *pgxpool.Pool) error {
	if err := database.CheckMigrations(ctx, db); err != nil {
		return errors.New("owned report archive task fixture requires the latest migration")
	}
	return nil
}

func prepare(ctx context.Context, db *pgxpool.Pool, adminUsername, viewport string) (fixtureOutput, error) {
	var out fixtureOutput
	if db == nil || (viewport != "desktop" && viewport != "mobile") || adminUsername != fixtureAdminPrefix+viewport {
		return out, errors.New("owned report archive task administrator required")
	}
	if err := requireLatestMigration(ctx, db); err != nil {
		return out, err
	}
	var tasks, archives, accounts, ledger int64
	err := db.QueryRow(ctx, `SELECT
	 (SELECT count(*) FROM report_archive_automatic_tasks),
	 (SELECT count(*) FROM report_archives),
	 (SELECT count(*) FROM point_accounts),
	 (SELECT count(*) FROM point_ledger_entries)`).Scan(&tasks, &archives, &accounts, &ledger)
	if err != nil || tasks != 0 || archives != 0 || accounts != 0 || ledger != 0 {
		return out, errors.New("report archive task fixture requires empty task, archive and wallet tables")
	}
	var existingPolicy reportarchive.AutomaticPolicy
	adminID, err := activeAdminID(ctx, db, adminUsername)
	if err != nil {
		return out, err
	}
	policyTx, err := db.Begin(ctx)
	if err != nil {
		return out, errors.New("report archive task policy transaction unavailable")
	}
	actor, err := (adminsys.Store{DB: db}).LockAdminAccess(ctx, policyTx, adminID, false)
	if err == nil {
		existingPolicy, err = (reportarchive.Service{DB: db}).AutomaticPolicyTx(ctx, policyTx, fixtureBrand)
	}
	if err != nil || existingPolicy.Version != 1 || existingPolicy.DailyEnabled || existingPolicy.MonthlyEnabled {
		policyTx.Rollback(ctx)
		return out, errors.New("Harbor archive policy is not in its initial disabled state")
	}
	var timezone string
	if err = policyTx.QueryRow(ctx, `SELECT timezone FROM brands WHERE id=$1 AND code='harbor' AND status='active'`, fixtureBrand).Scan(&timezone); err != nil {
		policyTx.Rollback(ctx)
		return out, errors.New("active Harbor fixture brand required")
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		policyTx.Rollback(ctx)
		return out, errors.New("Harbor fixture timezone unavailable")
	}
	period := time.Now().In(location).AddDate(0, 0, -1).Format("2006-01-02")
	_, err = (reportarchive.Service{DB: db}).UpdateAutomaticPolicyTx(ctx, policyTx, fixtureBrand, actor, reportarchive.AutomaticPolicyInput{
		Version: existingPolicy.Version, DailyEnabled: true, DailyStartPeriod: &period,
		Reason: "Enable the owned browser retry fixture from the previous Harbor-local day",
	}, points.Metadata{ActorType: "admin", ActorID: adminID, RequestID: ids.New()})
	if err != nil {
		policyTx.Rollback(ctx)
		return out, errors.New("audited Harbor archive policy setup failed")
	}
	if err = policyTx.Commit(ctx); err != nil {
		return out, errors.New("audited Harbor archive policy commit failed")
	}
	service := reportarchive.Service{DB: db}
	created, err := service.DiscoverAutomatic(ctx, 1)
	if err != nil || created != 1 {
		return out, errors.New("real automatic archive discovery did not create exactly one fixture task")
	}
	task, err := soleFixtureTask(ctx, db)
	if err != nil || task.State != "pending" || task.Version != 1 || task.AttemptCount != 0 {
		return out, errors.New("discovered fixture task is not in its initial pending state")
	}
	if _, err = db.Exec(ctx, `CREATE FUNCTION `+fixtureFunction+`() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='report_archive.create.automatic' THEN RAISE EXCEPTION 'owned fixture capture audit failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER `+fixtureTrigger+` BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION `+fixtureFunction+`() `); err != nil {
		return out, errors.New("temporary report archive task failure injection unavailable")
	}
	processed, processErr := service.ProcessAutomatic(ctx, 1)
	_, dropErr := db.Exec(ctx, `DROP TRIGGER `+fixtureTrigger+` ON audit_logs; DROP FUNCTION `+fixtureFunction+`() `)
	if processErr != nil || dropErr != nil || processed != 0 {
		return out, errors.New("automatic failure fixture did not complete its isolated failure transition")
	}
	task, err = soleFixtureTask(ctx, db)
	if err != nil || task.State != "failed" || task.Version != 2 || task.AttemptCount != 1 || task.ArchiveID != nil {
		return out, errors.New("automatic worker did not persist the expected genuine failed task")
	}
	return verify(ctx, db)
}

func activeAdminID(ctx context.Context, db *pgxpool.Pool, username string) (string, error) {
	var id string
	if err := db.QueryRow(ctx, `SELECT id::text FROM admin_accounts WHERE username=$1 AND status='active'`, username).Scan(&id); err != nil {
		return "", errors.New("owned Harbor archive administrator unavailable")
	}
	return id, nil
}

func soleFixtureTask(ctx context.Context, db *pgxpool.Pool) (reportarchive.AutomaticTask, error) {
	var out reportarchive.AutomaticTask
	var raw []byte
	err := db.QueryRow(ctx, `SELECT jsonb_build_object('id',j.id,'brand_id',j.brand_id,'policy_version',j.policy_version,'window',jsonb_build_object('kind',j.kind,'period_key',j.period_key,'timezone',j.timezone,'from',j.from_at,'to',j.to_at),'state',j.state,'version',j.version,'attempt_count',j.attempt_count,'archive_id',j.archive_id,'last_error_code',j.last_error_code,'creation_audit_log_id',j.creation_audit_log_id,'last_audit_log_id',j.last_audit_log_id,'created_at',j.created_at,'updated_at',j.updated_at) FROM report_archive_automatic_tasks j WHERE j.brand_id=$1 AND (SELECT count(*) FROM report_archive_automatic_tasks WHERE brand_id=$1)=1`, fixtureBrand).Scan(&raw)
	if err != nil {
		return out, errors.New("exactly one Harbor report archive task required")
	}
	if err = json.Unmarshal(raw, &out); err != nil {
		return out, errors.New("Harbor report archive task is unreadable")
	}
	return out, nil
}

func advance(ctx context.Context, db *pgxpool.Pool) (fixtureOutput, error) {
	var out fixtureOutput
	task, err := soleFixtureTask(ctx, db)
	if err != nil || task.State != "pending" {
		return out, errors.New("fixture task must be the sole pending task before explicit advance")
	}
	var pending int64
	if err = db.QueryRow(ctx, `SELECT count(*) FROM report_archive_automatic_tasks WHERE state='pending'`).Scan(&pending); err != nil || pending != 1 {
		return out, errors.New("fixture advance requires exactly one pending task in the owned database")
	}
	processed, err := (reportarchive.Service{DB: db}).ProcessAutomatic(ctx, 20)
	if err != nil || processed != 1 {
		return out, errors.New("explicit automatic archive worker advance failed")
	}
	out, err = verify(ctx, db)
	if err != nil || out.State != "completed" {
		return fixtureOutput{}, errors.New("fixture worker did not complete the retried task")
	}
	return out, nil
}

func verify(ctx context.Context, db *pgxpool.Pool) (fixtureOutput, error) {
	var out fixtureOutput
	task, err := soleFixtureTask(ctx, db)
	if err != nil {
		return out, err
	}
	var archives, ledger, totalPoints string
	err = db.QueryRow(ctx, `SELECT
	 (SELECT count(*)::text FROM report_archives WHERE brand_id=$1),
	 (SELECT count(*)::text FROM point_ledger_entries WHERE brand_id=$1),
	 (SELECT coalesce(sum(points),0)::text FROM point_buckets WHERE brand_id=$1)`, fixtureBrand).Scan(&archives, &ledger, &totalPoints)
	if err != nil {
		return out, errors.New("fixture task verification query failed")
	}
	out = fixtureOutput{TaskID: task.ID, BrandID: task.BrandID, TaskVersion: task.Version, State: task.State,
		AttemptCount: task.AttemptCount, ArchiveID: task.ArchiveID, ArchiveCount: archives,
		LedgerCount: ledger, TotalPoints: totalPoints}
	return out, nil
}
