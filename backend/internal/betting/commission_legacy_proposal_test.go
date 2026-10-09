package betting

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/authcrypto"
	"github.com/gxfcjkxf/lottery/backend/internal/commissionreview"
	"github.com/gxfcjkxf/lottery/backend/internal/database"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
)

func TestCommissionLegacyProposalAuditsFreshEvidenceAndKeepsFundsAndHistory(t *testing.T) {
	f, pay, target, _ := zeroOriginalManualOnlyFixture(t)
	ctx, db := context.Background(), f.betting.db
	next := createFixtureCorrection(t, f.betting, correctedDigits(1, 2, 2))
	if _, err := f.betting.service.ProcessCorrections(ctx, 20); err != nil {
		t.Fatal(err)
	}
	if _, err := f.betting.service.ProcessSettlements(ctx, 20); err != nil {
		t.Fatal(err)
	}
	_ = next
	advanceCommissionWorker(t, f, 40)
	if err := commissionPaymentCallTx(t, f, func(tx pgx.Tx) error {
		log, err := audit.Append(ctx, tx, audit.Record{BrandID: f.betting.brand, ActorType: "system", Action: "commission.payment.invalidate", ResourceType: "commission_payment", ResourceID: pay.ID, Reason: "reproduce legacy metadata for proposal test", RequestID: ids.New(), Before: map[string]any{"version": pay.Version, "state": pay.State}, After: map[string]any{"version": pay.Version + 1, "state": "stale"}})
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE commission_payments SET state='stale',version=version+1,last_error_code=NULL,last_audit_log_id=$2 WHERE id=$1`, pay.ID, log)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := database.PrepareCommissionHistoryReview(ctx, db); err != nil {
		t.Fatal(err)
	}
	// Real stored roles are required; no forged in-memory grants can authorize.
	actorID := f.betting.version.CreatedBy
	role := ids.New()
	if _, err := db.Exec(ctx, `INSERT INTO roles(id,code,name,brand_id) VALUES($1,$2,'legacy review test',$3)`, role, "legacy-review-"+role, f.betting.brand); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"commission.view.brand", "report_commission.view.brand", "commission_correction.retry.brand", "commission_correction.approve.brand"} {
		if _, err := db.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) VALUES($1,$2)`, role, key); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(ctx, `INSERT INTO admin_brand_scopes(account_id,brand_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, actorID, f.betting.brand); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO admin_account_roles(account_id,role_id) VALUES($1,$2)`, actorID, role); err != nil {
		t.Fatal(err)
	}
	actor, err := (adminsys.Store{DB: db}).Account(ctx, actorID)
	if err != nil {
		t.Fatal(err)
	}
	if actor.SuperAdmin {
		t.Fatal("test requires ordinary real account")
	}
	read := func(zone string) commissionreview.Snapshot {
		t.Helper()
		tx, err := db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if _, err = tx.Exec(ctx, `SELECT set_config('TimeZone',$1,true)`, zone); err != nil {
			t.Fatal(err)
		}
		s, err := commissionreview.SnapshotTx(ctx, tx, f.betting.brand, pay.CycleID)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	source := read("UTC")
	if source.SourceDigest != read("Asia/Manila").SourceDigest {
		t.Fatal("digest depends on connection timezone or observation time")
	}
	before := correctionPlanSnapshot(t, f, pay.ID)
	wallet := commissionWalletBySource(t, f, target.MemberID)
	meta := func() points.Metadata {
		return points.Metadata{ActorType: "admin", ActorID: actor.ID, RequestID: ids.New()}
	}
	in := commissionreview.ProposalInput{CycleID: pay.CycleID, SourceDigest: source.SourceDigest, Reason: "freeze complete legacy facts for explicit review"}
	var proposal commissionreview.Proposal
	bettingTx(t, db, func(tx pgx.Tx) error {
		var err error
		proposal, err = commissionreview.ProposeTx(ctx, tx, actor, f.betting.brand, in, meta())
		return err
	})
	if proposal.Version != 1 || proposal.State != "awaiting_review" || proposal.ExecutionAvailable || proposal.AuditLogID == "" {
		t.Fatalf("proposal=%+v", proposal)
	}
	if source.SourceDigest != read("UTC").SourceDigest {
		t.Fatal("proposal audit invalidated its own financial digest")
	}
	bettingTx(t, db, func(tx pgx.Tx) error {
		_, err := audit.Append(ctx, tx, audit.Record{BrandID: f.betting.brand, ActorType: "admin", ActorID: actor.ID, Action: "commission.cycle.read", ResourceType: "commission_cycle", ResourceID: pay.CycleID, RequestID: ids.New()})
		return err
	})
	if source.SourceDigest != read("UTC").SourceDigest {
		t.Fatal("readonly audit invalidated frozen financial evidence")
	}
	var reviewed commissionreview.Proposal
	bettingTx(t, db, func(tx pgx.Tx) error {
		var err error
		reviewed, err = commissionreview.ReviewTx(ctx, tx, actor, f.betting.brand, proposal.ID, commissionreview.ReviewInput{Version: 1, SourceDigest: source.SourceDigest, Reason: "single-person evidence review, no financial execution"}, meta())
		return err
	})
	if reviewed.Version != 2 || reviewed.State != "reviewed" || reviewed.ExecutionAvailable || reviewed.ReviewedBy == nil || *reviewed.ReviewedBy != actor.ID {
		t.Fatalf("review=%+v", reviewed)
	}
	bettingTx(t, db, func(tx pgx.Tx) error {
		p, err := commissionreview.ReadProposalTx(ctx, tx, actor, f.betting.brand, proposal.ID)
		if err == nil && p.AuditLogID != reviewed.AuditLogID {
			t.Fatal("journal read lost latest audited state")
		}
		return err
	})
	if source.SourceDigest != read("UTC").SourceDigest || before != correctionPlanSnapshot(t, f, pay.ID) || wallet != commissionWalletBySource(t, f, target.MemberID) {
		t.Fatal("proposal/review changed financial evidence")
	}
	assertLegacyProposalCLI(t, f, actor, role, pay.CycleID)
	// A different key cannot review an already-reviewed proposal again.
	if err := commissionPaymentCallTx(t, f, func(tx pgx.Tx) error {
		_, err := commissionreview.ReviewTx(ctx, tx, actor, f.betting.brand, proposal.ID, commissionreview.ReviewInput{Version: 1, SourceDigest: source.SourceDigest, Reason: "duplicate review"}, meta())
		return err
	}); !errors.Is(err, commissionreview.ErrProposalState) {
		t.Fatalf("duplicate review=%v", err)
	}
	// Freeze a second proposal, then actual old-library registration changes
	// history without changing the awarded amount or current calculation.
	var drifted commissionreview.Proposal
	bettingTx(t, db, func(tx pgx.Tx) error {
		var err error
		drifted, err = commissionreview.ProposeTx(ctx, tx, actor, f.betting.brand, in, meta())
		return err
	})
	if _, err := f.service.ProcessPayments(ctx, 20); err != nil {
		t.Fatal(err)
	}
	drift := read("UTC")
	if drift.SourceDigest == source.SourceDigest || drift.Report.Analysis.Summary.ActualNetPoints != source.Report.Analysis.Summary.ActualNetPoints {
		t.Fatal("same-net new payment history was not bound into digest")
	}
	if err := commissionPaymentCallTx(t, f, func(tx pgx.Tx) error {
		_, err := commissionreview.ReviewTx(ctx, tx, actor, f.betting.brand, drifted.ID, commissionreview.ReviewInput{Version: 1, SourceDigest: source.SourceDigest, Reason: "reject changed source"}, meta())
		return err
	}); !errors.Is(err, commissionreview.ErrSourceChanged) {
		t.Fatalf("stale source approved: %v", err)
	}
	if _, err := db.Exec(ctx, `DELETE FROM role_permissions WHERE role_id=$1 AND permission_key='commission_correction.retry.brand'`, role); err != nil {
		t.Fatal(err)
	}
	if err := commissionPaymentCallTx(t, f, func(tx pgx.Tx) error {
		_, err := commissionreview.ProposeTx(ctx, tx, actor, f.betting.brand, commissionreview.ProposalInput{CycleID: pay.CycleID, SourceDigest: drift.SourceDigest, Reason: "old in-memory grant must not suffice"}, meta())
		return err
	}); !errors.Is(err, commissionreview.ErrDenied) {
		t.Fatalf("revoked grant reused=%v", err)
	}
	forged := actor
	forged.SuperAdmin = false
	forged.Roles = []access.Role{{BrandID: f.betting.brand, Permissions: []access.Permission{{Resource: "commission_correction", Action: "retry", Scope: access.ScopeBrand}}}}
	if err := commissionPaymentCallTx(t, f, func(tx pgx.Tx) error {
		_, err := commissionreview.ProposeTx(ctx, tx, forged, f.betting.brand, in, meta())
		return err
	}); !errors.Is(err, commissionreview.ErrDenied) {
		t.Fatalf("forged context accepted=%v", err)
	}
	if err := database.Migrate(ctx, db); err == nil || !strings.Contains(err.Error(), "stale commission payment has actual money") {
		t.Fatalf("proposal bypassed upgrade refusal=%v", err)
	}
}

func assertLegacyProposalCLI(t *testing.T, f commissionBatchFixture, actor access.Account, role, cycle string) {
	t.Helper()
	ctx := context.Background()
	db := f.betting.db
	bin := filepath.Join(t.TempDir(), "platform")
	build := exec.Command("go", "build", "-o", bin, "./cmd/platform")
	build.Dir = "../.."
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("owned CLI build: %v %s", err, output)
	}
	var schema string
	if err := db.QueryRow(ctx, `SELECT current_schema()`).Scan(&schema); err != nil {
		t.Fatal(err)
	}
	dsn, err := url.Parse(os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	query := dsn.Query()
	query.Set("search_path", schema)
	dsn.RawQuery = query.Encode()
	// A generated test-only server-format token bound to a real stored admin
	// session; no production credential or authentication bypass in the CLI.
	token, err := authcrypto.NewSessionToken()
	if err != nil {
		t.Fatal(err)
	}
	hash := authcrypto.DigestSessionToken(token)
	sessionID := ids.New()
	if _, err := db.Exec(ctx, `INSERT INTO sessions(id,token_hash,admin_id,expires_at) VALUES($1,$2,$3,clock_timestamp()+interval '1 hour')`, sessionID, hex.EncodeToString(hash[:]), actor.ID); err != nil {
		t.Fatal(err)
	}
	env := append(os.Environ(), "APP_ENV=test", "DATABASE_URL="+dsn.String(), "DATABASE_READ_URL=", "DATABASE_READ_URLS=", "AUTH_KEY_FILE=", "AUTH_KEY="+base64.RawStdEncoding.EncodeToString(bytes.Repeat([]byte{27}, 32)), "LOTTERY_COMMISSION_RECOVERY_BEARER="+token)
	run := func(args []string, digest, key, reason string, wantOK bool) []byte {
		t.Helper()
		cmdCtx, cancel := context.WithTimeout(ctx, 35*time.Second)
		defer cancel()
		command := exec.CommandContext(cmdCtx, bin, args...)
		command.Env = append(env, "LOTTERY_COMMISSION_RECOVERY_DIGEST="+digest, "LOTTERY_COMMISSION_RECOVERY_KEY="+key, "LOTTERY_COMMISSION_RECOVERY_REASON="+reason)
		out, err := command.CombinedOutput()
		if strings.Contains(string(out), token) {
			t.Fatal("CLI exposed test session")
		}
		if (err == nil) != wantOK {
			t.Fatalf("CLI unexpected result err=%v output=%s", err, out)
		}
		return out
	}
	var source commissionreview.Snapshot
	if err := json.Unmarshal(run([]string{"commission-recovery-source", f.betting.brand, cycle}, "", "", "", true), &source); err != nil {
		t.Fatal(err)
	}
	args := []string{"commission-recovery-propose", f.betting.brand, cycle, "--confirm=record-only-review"}
	first := run(args, source.SourceDigest, "legacy-cli-proposal-1", "record CLI frozen source", true)
	if again := run(args, source.SourceDigest, "legacy-cli-proposal-1", "record CLI frozen source", true); !bytes.Equal(first, again) {
		t.Fatal("original immutable proposal receipt was not replayed")
	}
	var proposal commissionreview.Proposal
	if err := json.Unmarshal(first, &proposal); err != nil {
		t.Fatal(err)
	}
	reviewArgs := []string{"commission-recovery-review", f.betting.brand, proposal.ID, "1", "--confirm=record-only-review"}
	reviewed := run(reviewArgs, source.SourceDigest, "legacy-cli-review-1", "record-only review", true)
	if again := run(reviewArgs, source.SourceDigest, "legacy-cli-review-1", "record-only review", true); !bytes.Equal(reviewed, again) {
		t.Fatal("review receipt changed on replay")
	}
	wrongTarget := append([]string(nil), reviewArgs...)
	wrongTarget[2] = ids.New()
	run(wrongTarget, source.SourceDigest, "legacy-cli-review-1", "record-only review", false)
	var current commissionreview.Proposal
	if err := json.Unmarshal(run([]string{"commission-recovery-get", f.betting.brand, proposal.ID}, "", "", "", true), &current); err != nil {
		t.Fatal(err)
	}
	if current.Version != 2 || current.ExecutionAvailable {
		t.Fatalf("current record became execution approval: %+v", current)
	}
	// Leave the session valid and revoke only the write grant, proving cached
	// responses also revalidate permissions, not merely account/session status.
	if _, err := db.Exec(ctx, `DELETE FROM role_permissions WHERE role_id=$1 AND permission_key='commission_correction.retry.brand'`, role); err != nil {
		t.Fatal(err)
	}
	run(args, source.SourceDigest, "legacy-cli-proposal-1", "record CLI frozen source", false)
	if _, err := db.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) VALUES($1,'commission_correction.retry.brand')`, role); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `UPDATE sessions SET revoked_at=clock_timestamp() WHERE id=$1`, sessionID); err != nil {
		t.Fatal(err)
	}
	run(reviewArgs, source.SourceDigest, "legacy-cli-review-1", "record-only review", false)
	var count int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE brand_id=$1 AND resource_id=$2 AND action LIKE 'commission.history_recovery.%'`, f.betting.brand, proposal.ID).Scan(&count); err != nil || count != 2 {
		t.Fatalf("CLI duplicated journal: %d %v", count, err)
	}
}
