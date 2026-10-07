//go:build browserfixture

package main

import (
	"context"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"testing"
	"time"
)

const (
	validFixtureDSN    = "postgres://lottery_test:local_test_password@127.0.0.1:55432/lottery_commission_ui_desktop_s16?sslmode=disable"
	verifiedDesktopDSN = "postgres://lottery_test:local_test_password@127.0.0.1:55432/lottery_commission_ui_desktop_s16_verified?sslmode=disable"
	verifiedMobileDSN  = "postgres://lottery_test:local_test_password@127.0.0.1:55432/lottery_commission_ui_mobile_s16_verified?sslmode=disable"
	finalDesktopDSN    = "postgres://lottery_test:local_test_password@127.0.0.1:55432/lottery_commission_ui_desktop_s16_final?sslmode=disable"
	finalMobileDSN     = "postgres://lottery_test:local_test_password@127.0.0.1:55432/lottery_commission_ui_mobile_s16_final?sslmode=disable"
	validAdminPass     = "owned-fixture-admin-password"
	validUserPass      = "owned-fixture-user-password"
)

func TestFixtureGuardAcceptsOnlyNamedLocalSyntheticDatabases(t *testing.T) {
	if err := safeFixtureURL(validFixtureDSN, "test", fixtureAck, validAdminPass, validUserPass, true); err != nil {
		t.Fatal(err)
	}
	second := "postgresql://lottery_test:local_test_password@localhost:5432/lottery_commission_ui_mobile_s16?sslmode=disable"
	if err := safeFixtureURL(second, "test", fixtureAck, validAdminPass, validUserPass, true); err != nil {
		t.Fatalf("second explicitly owned local database rejected: %v", err)
	}
	for _, dsn := range []string{verifiedDesktopDSN, verifiedMobileDSN, finalDesktopDSN, finalMobileDSN} {
		if err := safeFixtureURL(dsn, "test", fixtureAck, validAdminPass, validUserPass, true); err != nil {
			t.Errorf("fresh verification database rejected: %v", err)
		}
	}
	unsafe := []struct {
		name, dsn, environment, confirmation, adminPassword, userPassword string
	}{
		{"environment", validFixtureDSN, "production", fixtureAck, validAdminPass, validUserPass},
		{"confirmation", validFixtureDSN, "test", "", validAdminPass, validUserPass},
		{"other database", "postgres://lottery_test:pw@127.0.0.1:55432/lottery_test?sslmode=disable", "test", fixtureAck, validAdminPass, validUserPass},
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
