//go:build browserfixture

package main

import (
	"context"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/commission"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	validFixtureDSN      = "postgres://lottery_test:local_test_password@127.0.0.1:55432/lottery_commission_ui_desktop_s16?sslmode=disable"
	verifiedDesktopDSN   = "postgres://lottery_test:local_test_password@127.0.0.1:55432/lottery_commission_ui_desktop_s16_verified?sslmode=disable"
	verifiedMobileDSN    = "postgres://lottery_test:local_test_password@127.0.0.1:55432/lottery_commission_ui_mobile_s16_verified?sslmode=disable"
	finalDesktopDSN      = "postgres://lottery_test:local_test_password@127.0.0.1:55432/lottery_commission_ui_desktop_s16_final?sslmode=disable"
	finalMobileDSN       = "postgres://lottery_test:local_test_password@127.0.0.1:55432/lottery_commission_ui_mobile_s16_final?sslmode=disable"
	correctionDesktopDSN = "postgres://lottery_test:local_test_password@127.0.0.1:55432/lottery_commission_correction_desktop_s27?sslmode=disable"
	correctionMobileDSN  = "postgres://lottery_test:local_test_password@127.0.0.1:55432/lottery_commission_correction_mobile_s27?sslmode=disable"
	validAdminPass       = "owned-fixture-admin-password"
	validUserPass        = "owned-fixture-user-password"
)

func TestFixtureGuardAcceptsOnlyNamedLocalSyntheticDatabases(t *testing.T) {
	if err := safeFixtureURL(validFixtureDSN, "test", fixtureAck, validAdminPass, validUserPass, true); err != nil {
		t.Fatal(err)
	}
	second := "postgresql://lottery_test:local_test_password@localhost:5432/lottery_commission_ui_mobile_s16?sslmode=disable"
	if err := safeFixtureURL(second, "test", fixtureAck, validAdminPass, validUserPass, true); err != nil {
		t.Fatalf("second explicitly owned local database rejected: %v", err)
	}
	for _, dsn := range []string{verifiedDesktopDSN, verifiedMobileDSN, finalDesktopDSN, finalMobileDSN, correctionDesktopDSN, correctionMobileDSN,
		"postgres://lottery_test:local_test_password@127.0.0.1:55432/lottery_commission_payment_desktop_s17?sslmode=disable",
		"postgres://lottery_test:local_test_password@127.0.0.1:55432/lottery_commission_payment_mobile_s17?sslmode=disable",
		"postgres://lottery_test:local_test_password@127.0.0.1:55432/lottery_commission_payment_desktop_s17_verified?sslmode=disable",
		"postgres://lottery_test:local_test_password@127.0.0.1:55432/lottery_commission_payment_mobile_s17_verified?sslmode=disable",
		"postgres://lottery_test:local_test_password@127.0.0.1:55432/lottery_commission_adjustment_desktop_s18?sslmode=disable",
		"postgres://lottery_test:local_test_password@127.0.0.1:55432/lottery_commission_adjustment_mobile_s18?sslmode=disable",
		"postgres://lottery_test:local_test_password@127.0.0.1:55432/lottery_commission_adjustment_desktop_s18_verified?sslmode=disable",
		"postgres://lottery_test:local_test_password@127.0.0.1:55432/lottery_commission_adjustment_mobile_s18_verified?sslmode=disable",
		"postgres://lottery_test:local_test_password@127.0.0.1:55432/lottery_commission_report_desktop_s19?sslmode=disable",
		"postgres://lottery_test:local_test_password@127.0.0.1:55432/lottery_commission_report_mobile_s19?sslmode=disable"} {
		if err := safeFixtureURL(dsn, "test", fixtureAck, validAdminPass, validUserPass, true); err != nil {
			t.Errorf("fresh verification database rejected: %v", err)
		}
	}
	if err := safeFixtureURL(correctionDesktopDSN, "test", fixtureAck, validAdminPass, validUserPass, true); err != nil {
		t.Fatalf("exact newly approved correction database rejected: %v", err)
	}
	if err := safeFixtureURL(correctionMobileDSN, "test", fixtureAck, validAdminPass, validUserPass, true); err != nil {
		t.Fatalf("exact newly approved mobile correction database rejected: %v", err)
	}
	unsafe := []struct {
		name, dsn, environment, confirmation, adminPassword, userPassword string
	}{
		{"environment", validFixtureDSN, "production", fixtureAck, validAdminPass, validUserPass},
		{"confirmation", validFixtureDSN, "test", "", validAdminPass, validUserPass},
		{"other database", "postgres://lottery_test:pw@127.0.0.1:55432/lottery_test?sslmode=disable", "test", fixtureAck, validAdminPass, validUserPass},
		{"unapproved correction database", "postgres://lottery_test:pw@127.0.0.1:55432/lottery_commission_correction_unapproved_s27?sslmode=disable", "test", fixtureAck, validAdminPass, validUserPass},
		{"correction database suffix", "postgres://lottery_test:pw@127.0.0.1:55432/lottery_commission_correction_desktop_s27_suffix?sslmode=disable", "test", fixtureAck, validAdminPass, validUserPass},
		{"remote host", "postgres://lottery_test:pw@example.com:55432/lottery_commission_ui_desktop_s16?sslmode=disable", "test", fixtureAck, validAdminPass, validUserPass},
		{"unapproved port", "postgres://lottery_test:pw@127.0.0.1:5433/lottery_commission_ui_desktop_s16?sslmode=disable", "test", fixtureAck, validAdminPass, validUserPass},
		{"wrong user", "postgres://other:pw@127.0.0.1:55432/lottery_commission_ui_desktop_s16?sslmode=disable", "test", fixtureAck, validAdminPass, validUserPass},
		{"no database password", "postgres://lottery_test@127.0.0.1:55432/lottery_commission_ui_desktop_s16?sslmode=disable", "test", fixtureAck, validAdminPass, validUserPass},
		{"fragment", validFixtureDSN + "#ignored", "test", fixtureAck, validAdminPass, validUserPass},
		{"connection override", validFixtureDSN + "&search_path=public", "test", fixtureAck, validAdminPass, validUserPass},
		{"duplicate sslmode", validFixtureDSN + "&sslmode=disable", "test", fixtureAck, validAdminPass, validUserPass},
		{"ssl enabled", "postgres://lottery_test:pw@127.0.0.1:55432/lottery_commission_ui_desktop_s16?sslmode=require", "test", fixtureAck, validAdminPass, validUserPass},
		{"short admin secret", validFixtureDSN, "test", fixtureAck, "short", validUserPass},
		{"short user secret", validFixtureDSN, "test", fixtureAck, validAdminPass, "short"},
	}
	for _, test := range unsafe {
		t.Run(test.name, func(t *testing.T) {
			if safeFixtureURL(test.dsn, test.environment, test.confirmation, test.adminPassword, test.userPassword, true) == nil {
				t.Fatal("unsafe fixture target accepted")
			}
		})
	}
}

func TestNonInitCommandsStillRequireOwnedDatabaseButNotFixturePasswords(t *testing.T) {
	if err := safeFixtureURL(validFixtureDSN, "test", fixtureAck, "", "", false); err != nil {
		t.Fatal(err)
	}
	if err := safeFixtureURL("postgres://lottery_test:pw@127.0.0.1:55432/lottery_test?sslmode=disable", "test", fixtureAck, "", "", false); err == nil {
		t.Fatal("non-init command accepted an unowned database")
	}
}

func TestEconomicFingerprintCanonicalizesOrderedFinancialJSON(t *testing.T) {
	first, err := hashCanonicalJSON([]byte(`{"accounts":[],"ledger":[{"id":"e1","points":20}]}`))
	if err != nil {
		t.Fatal(err)
	}
	reordered, err := hashCanonicalJSON([]byte(" { \"ledger\" : [ { \"points\" : 20, \"id\" : \"e1\" } ], \"accounts\" : [] } "))
	if err != nil {
		t.Fatal(err)
	}
	changed, err := hashCanonicalJSON([]byte(`{"accounts":[],"ledger":[{"id":"e1","points":19}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if first != reordered {
		t.Fatal("equivalent JSON produced different economic fingerprints")
	}
	if first == changed {
		t.Fatal("financial ledger change did not change the economic fingerprint")
	}
	if _, err = hashCanonicalJSON([]byte(`{} {}`)); err == nil {
		t.Fatal("multiple JSON values accepted as fingerprint input")
	}
}

func TestFixtureInitializesAllThreeActualCommissionWorkflows(t *testing.T) {
	db := testdb.New(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	out, err := initialize(ctx, db, validAdminPass, validUserPass)
	if err != nil {
		t.Fatal(err)
	}
	if err = validateInitOut(ctx, db, out); err != nil {
		t.Fatal(err)
	}
	before, err := verify(ctx, db, out.AdminID)
	if err != nil {
		t.Fatal(err)
	}
	if before["commission_ledger_entries"] != int64(0) {
		t.Fatalf("fixture posted commission money: %+v", before)
	}
	if _, err = initialize(ctx, db, validAdminPass, validUserPass); err == nil {
		t.Fatal("fixture initialization overwrote occupied identities")
	}
}

func TestFixtureMigrationAdmissionRejectsStaleOrDamagedHistoryBeforeWrites(t *testing.T) {
	for _, mode := range []string{"historical", "checksum"} {
		t.Run(mode, func(t *testing.T) {
			var db *pgxpool.Pool
			if mode == "historical" {
				db = testdb.NewAtVersion(t, 53)
			} else {
				db = testdb.New(t)
				if _, err := db.Exec(context.Background(), `UPDATE schema_migrations SET checksum='damaged' WHERE name=(SELECT min(name) FROM schema_migrations)`); err != nil {
					t.Fatal(err)
				}
			}
			fingerprint := func() string {
				t.Helper()
				var result string
				if err := db.QueryRow(context.Background(), `SELECT jsonb_build_object(
				 'migrations',(SELECT jsonb_agg(to_jsonb(m) ORDER BY m.name) FROM schema_migrations m),
				 'admins',(SELECT jsonb_agg(to_jsonb(a) ORDER BY a.id) FROM admin_accounts a),
				 'members',(SELECT jsonb_agg(to_jsonb(m) ORDER BY m.id) FROM brand_members m),
				 'accounts',(SELECT jsonb_agg(to_jsonb(a) ORDER BY a.id) FROM point_accounts a),
				 'buckets',(SELECT jsonb_agg(to_jsonb(b) ORDER BY b.account_id,b.source,b.state) FROM point_buckets b),
				 'ledger',(SELECT jsonb_agg(to_jsonb(l) ORDER BY l.id) FROM point_ledger_entries l),
				 'audits',(SELECT jsonb_agg(to_jsonb(a) ORDER BY a.id) FROM audit_logs a))::text`).Scan(&result); err != nil {
					t.Fatal(err)
				}
				return result
			}
			before := fingerprint()
			if _, err := initialize(context.Background(), db, validAdminPass, validUserPass); err == nil {
				t.Fatal("fixture initialized against stale or damaged migration history")
			}
			if err := requireLatestMigration(context.Background(), db); err == nil {
				t.Fatal("existing-fixture command admitted stale or damaged migration history")
			}
			if after := fingerprint(); after != before {
				t.Fatalf("%s migration rejection changed fixture identity, funds, audits or metadata", mode)
			}
		})
	}
}

func TestCorrectionLedgerEntryCountBeforeExecution(t *testing.T) {
	count, err := correctionLedgerEntryCount(context.Background(), nil, nil)
	if err != nil || count != 0 {
		t.Fatalf("ledger count before execution = %d, %v; want 0, nil", count, err)
	}
}

func TestVerifiedCorrectionExecutionTargetAllowsUnappliedAndChecksApplied(t *testing.T) {
	planTarget := commission.CorrectionPlanTarget{ID: "plan-target", BrandID: fixtureBrand, PlanID: "plan", MemberID: "member", DeltaPoints: points.Amount(-1)}
	if target, err := verifiedCorrectionExecutionTarget("0", "execution", planTarget, nil); err != nil || target != nil {
		t.Fatalf("unapplied correction target = %+v, %v; want nil, nil", target, err)
	}
	ledgerID, auditID := "ledger", "audit"
	targetValue := commission.CorrectionExecutionTarget{
		ID: "execution-target", BrandID: fixtureBrand, ExecutionID: "execution", PlanTargetID: planTarget.ID,
		MemberID: planTarget.MemberID, DeltaPoints: planTarget.DeltaPoints, State: commission.CorrectionExecutionTargetApplied,
		LedgerEntryID: &ledgerID, AuditLogID: &auditID,
	}
	target, err := verifiedCorrectionExecutionTarget("1", "execution", planTarget, []commission.CorrectionExecutionTarget{targetValue})
	if err != nil || target == nil || target.ID != targetValue.ID {
		t.Fatalf("applied correction target = %+v, %v; want matching target", target, err)
	}
	targetValue.MemberID = "other-member"
	if _, err = verifiedCorrectionExecutionTarget("1", "execution", planTarget, []commission.CorrectionExecutionTarget{targetValue}); err == nil {
		t.Fatal("applied correction target with mismatched beneficiary was accepted")
	}
}
