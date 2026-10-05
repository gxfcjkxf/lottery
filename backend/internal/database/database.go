package database

import (
	"context"
	"fmt"
	"github.com/gxfcjkxf/lottery/backend/internal/config"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

type Pools struct{ Primary, Replica *pgxpool.Pool }

func Open(ctx context.Context, c config.Config) (*Pools, error) {
	primary, err := open(ctx, c.DatabaseURL, c.DBMaxConns)
	if err != nil {
		return nil, fmt.Errorf("primary database unavailable")
	}
	p := &Pools{Primary: primary, Replica: primary}
	if c.DatabaseReadURL != "" {
		p.Replica, err = open(ctx, c.DatabaseReadURL, c.DBMaxConns)
		if err != nil {
			primary.Close()
			return nil, fmt.Errorf("read database unavailable")
		}
	}
	return p, nil
}
func open(ctx context.Context, url string, max int32) (*pgxpool.Pool, error) {
	c, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, err
	}
	c.MaxConns = max
	c.ConnConfig.ConnectTimeout = 5 * time.Second
	c.ConnConfig.RuntimeParams["timezone"] = "UTC"
	p, err := pgxpool.NewWithConfig(ctx, c)
	if err != nil {
		return nil, err
	}
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err = p.Ping(pingCtx); err != nil {
		p.Close()
		return nil, err
	}
	return p, nil
}
func (p *Pools) Close() {
	if p.Replica != p.Primary {
		p.Replica.Close()
	}
	p.Primary.Close()
}
