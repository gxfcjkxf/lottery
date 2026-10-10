//go:build browserfixture

package main

import (
	"reflect"
	"testing"
)

func TestFixtureUsersMatchFourIndependentViewports(t *testing.T) {
	want := []string{"qual_user", "qual_mobile_user", "qual_tablet_user", "qual_laptop_user"}
	if !reflect.DeepEqual(fixtureUsernames, want) {
		t.Fatalf("unexpected viewport users: %v", fixtureUsernames)
	}
}

const (
	validFixtureDSN = "postgres://lottery_test:local_test_password@127.0.0.1:55432/lottery_withdrawal_qualification_s9?sslmode=disable"
	validAdminPass  = "fixture-admin-password-2026"
	validUserPass   = "fixture-user-password-2026"
)

func TestFixtureGuardRequiresExactTestDatabaseAndPasswords(t *testing.T) {
	if err := safeFixtureURL(validFixtureDSN, "test", fixtureAck, validAdminPass, validUserPass); err != nil {
		t.Fatal(err)
	}
	unsafe := []struct {
		dsn, environment, confirm, adminPassword, userPassword string
	}{
		{validFixtureDSN, "production", fixtureAck, validAdminPass, validUserPass},
		{"postgres://lottery_test:local_test_password@127.0.0.1:55432/lottery_test?sslmode=disable", "test", fixtureAck, validAdminPass, validUserPass},
		{"postgres://lottery_test@127.0.0.1:55432/lottery_withdrawal_qualification_s9?sslmode=disable", "test", fixtureAck, validAdminPass, validUserPass},
		{validFixtureDSN, "test", fixtureAck, "", validUserPass},
		{validFixtureDSN, "test", fixtureAck, validAdminPass, ""},
		{validFixtureDSN + "&search_path=public", "test", fixtureAck, validAdminPass, validUserPass},
		{"postgres://lottery_test:local_test_password@example.com:55432/lottery_withdrawal_qualification_s9?sslmode=disable", "test", fixtureAck, validAdminPass, validUserPass},
	}
	for _, test := range unsafe {
		if safeFixtureURL(test.dsn, test.environment, test.confirm, test.adminPassword, test.userPassword) == nil {
			t.Fatalf("unsafe fixture target accepted: environment=%q dsn=%q", test.environment, test.dsn)
		}
	}
}
