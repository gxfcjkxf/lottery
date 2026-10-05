package database

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Seed only installs reproducible development brands; it never creates passwords.
func Seed(ctx context.Context, pool *pgxpool.Pool, environment string) error {
	if environment != "development" && environment != "test" {
		return fmt.Errorf("seed is restricted to development/test")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	statements := []string{
		`INSERT INTO brands(id,code,name,status,theme) VALUES
  ('0199a000-0000-7000-8000-000000000001','aurora','Aurora','active','{"primary_color":"#27634a","accent_color":"#d7b56d"}'),
  ('0199a000-0000-7000-8000-000000000002','harbor','Harbor','active','{"primary_color":"#274c75","accent_color":"#80becc"}')
  ON CONFLICT DO NOTHING`,
		`INSERT INTO brand_domains(id,brand_id,domain,is_primary) VALUES
   ('0199a000-0000-7000-8000-000000000101','0199a000-0000-7000-8000-000000000001','localhost',true),
   ('0199a000-0000-7000-8000-000000000102','0199a000-0000-7000-8000-000000000001','127.0.0.1',false),
   ('0199a000-0000-7000-8000-000000000103','0199a000-0000-7000-8000-000000000002','harbor.localhost',true)
   ON CONFLICT DO NOTHING`,
		`INSERT INTO platform_domains(domain) VALUES('localhost'),('127.0.0.1') ON CONFLICT DO NOTHING`,
	}
	for _, q := range statements {
		if _, err = tx.Exec(ctx, q); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
