//go:build browserfixture

package main

import "testing"

func TestFixtureRejectsUnownedDatabasesAndConnectionOverrides(t *testing.T) {
	good := "postgres://lottery_test:synthetic@127.0.0.1:5432/lottery_reconciliation_browser?sslmode=disable"
	if e := safeFixtureURL(good, "test", "owned_synthetic_database"); e != nil {
		t.Fatal(e)
	}
	for _, bad := range []struct{ dsn, environment, confirm string }{
		{good, "production", "owned_synthetic_database"}, {good, "test", ""},
		{"postgres://lottery_test@127.0.0.1:55432/postgres?sslmode=disable", "test", "owned_synthetic_database"},
		{"postgres://lottery_test@other.example:5432/lottery_reconciliation_browser?sslmode=disable", "test", "owned_synthetic_database"},
		{good + "&host=other.example", "test", "owned_synthetic_database"},
		{good + "&options=-csearch_path=public", "test", "owned_synthetic_database"},
		{good + "&sslmode=disable", "test", "owned_synthetic_database"},
	} {
		if e := safeFixtureURL(bad.dsn, bad.environment, bad.confirm); e == nil {
			t.Fatal("unsafe fixture accepted")
		}
	}
}
