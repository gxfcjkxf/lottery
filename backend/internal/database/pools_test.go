package database_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/config"
	"github.com/gxfcjkxf/lottery/backend/internal/database"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"github.com/jackc/pgx/v5"
)

func TestOptionalReadNodeCannotBlockPrimaryStartup(t *testing.T) {
	p := testdb.New(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	before := time.Now()
	pools, e := database.Open(ctx, config.Config{DatabaseURL: p.Config().ConnString(), DatabaseReadURLs: []string{"postgres://unreachable@127.0.0.1:1/missing?sslmode=disable"}, DBMaxConns: 1})
	if e != nil {
		t.Fatal(e)
	}
	defer pools.Close()
	if time.Since(before) > time.Second {
		t.Fatal("optional node delayed startup")
	}
	if len(pools.Replicas) != 1 {
		t.Fatal("configured replica pool missing")
	}
	tx, e := pools.Primary.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	callbackRan := false
	_, source, e := database.NewHistoryRouter(pools.Replicas).Read(ctx, tx, database.HistoryAudit, func(tx pgx.Tx) (any, error) {
		callbackRan = true
		var n int
		err := tx.QueryRow(ctx, `SELECT 42`).Scan(&n)
		return n, err
	})
	if e == nil || callbackRan || source.Reason != "replica_unavailable" {
		t.Fatal("unavailable configured replica did not return an explicit error", source, e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
}
func TestReadConfigurationErrorsNeverExposeCredentials(t *testing.T) {
	p := testdb.New(t)
	for _, urls := range [][]string{{"postgres://secret-pass:%broken"}, make([]string, 9)} {
		_, e := database.Open(context.Background(), config.Config{DatabaseURL: p.Config().ConnString(), DatabaseReadURLs: urls, DBMaxConns: 1})
		if e == nil || strings.Contains(e.Error(), "secret-pass") {
			t.Fatal("invalid optional node did not fail safely")
		}
	}
}
