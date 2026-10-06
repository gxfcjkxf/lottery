package database

import (
	"context"
	"fmt"
	"github.com/gxfcjkxf/lottery/backend/internal/config"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

type Pools struct {
	Primary, Replica *pgxpool.Pool // Replica is the legacy first-read-node alias.
	Replicas         []*pgxpool.Pool
}

func Open(ctx context.Context, c config.Config) (*Pools, error) {
	primary, err := open(ctx, c.DatabaseURL, c.DBMaxConns)
	if err != nil {
		return nil, fmt.Errorf("primary database unavailable")
	}
	p := &Pools{Primary: primary, Replica: primary}
	urls := append([]string(nil), c.DatabaseReadURLs...)
	if c.DatabaseReadURL != "" {
		if len(urls) != 0 {
			primary.Close()
			return nil, fmt.Errorf("conflicting read database configuration")
		}
		urls = append(urls, c.DatabaseReadURL)
	}
	if len(urls) > 8 {
		primary.Close()
		return nil, fmt.Errorf("too many read databases")
	}
	for _, url := range urls {
		replica, err := newPool(ctx, url, c.DBMaxConns)
		if err != nil {
			p.Close()
			return nil, fmt.Errorf("invalid read database configuration")
		}
		p.Replicas = append(p.Replicas, replica)
	}
	if len(p.Replicas) > 0 {
		p.Replica = p.Replicas[0]
	}
	return p, nil
}
func newPool(ctx context.Context, url string, max int32) (*pgxpool.Pool, error) {
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
	return p, nil
}
func open(ctx context.Context, url string, max int32) (*pgxpool.Pool, error) {
	p, err := newPool(ctx, url, max)
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
	seen := map[*pgxpool.Pool]bool{p.Primary: true}
	for _, replica := range append(append([]*pgxpool.Pool(nil), p.Replicas...), p.Replica) {
		if replica != nil && !seen[replica] {
			replica.Close()
			seen[replica] = true
		}
	}
	if p.Primary != nil {
		p.Primary.Close()
	}
}
