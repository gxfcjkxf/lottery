//go:build browserfixture

package main

import "testing"

func TestFixtureRequiresOwnedLoopbackDatabaseAndExplicitTestConfirmation(t *testing.T) {
	good := "postgres://lottery_test@127.0.0.1:55432/lottery_withdrawal_ui_s8?sslmode=disable"
	if err := safeFixtureURL(good, "test", "owned_synthetic_database"); err != nil {
		t.Fatal(err)
	}
	if err := safeFixtureURL("postgres://lottery_test@127.0.0.1:55432/lottery_withdrawal_ui_s8_followup?sslmode=disable", "test", "owned_synthetic_database"); err != nil {
		t.Fatal(err)
	}
	if err := safeFixtureURL("postgres://lottery_test@127.0.0.1:55432/lottery_withdrawal_ui_s8_verified?sslmode=disable", "test", "owned_synthetic_database"); err != nil {
		t.Fatal(err)
	}
	for _, v := range []struct{ url, env, confirm string }{
		{"postgres://lottery_test@127.0.0.1:55432/postgres?sslmode=disable", "test", "owned_synthetic_database"},
		{good, "development", "owned_synthetic_database"}, {good, "test", ""},
		{"postgres://lottery_test@example.com:55432/lottery_withdrawal_ui_s8?sslmode=disable", "test", "owned_synthetic_database"},
		{good + "&search_path=public", "test", "owned_synthetic_database"},
		{"postgres://lottery_test@127.0.0.1:55432/lottery_withdrawal_ui_s8_customer?sslmode=disable", "test", "owned_synthetic_database"},
	} {
		if safeFixtureURL(v.url, v.env, v.confirm) == nil {
			t.Fatalf("unsafe fixture target accepted %q", v.url)
		}
	}
}
