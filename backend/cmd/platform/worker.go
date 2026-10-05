package main

import (
	"context"
	"github.com/gxfcjkxf/lottery/backend/internal/rulebook"
	"github.com/jackc/pgx/v5/pgxpool"
	"log/slog"
	"time"
)

func runPeriodWorker(ctx context.Context, db *pgxpool.Pool, logger *slog.Logger) error {
	store := rulebook.Store{DB: db}
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	fill := time.NewTicker(time.Minute)
	defer fill.Stop()
	fillCalendar := func() {
		run, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		n, e := store.FillCalendar(run)
		if e != nil && ctx.Err() == nil {
			logger.Error("calendar reservation failed", "error", e, "committed_created", n)
		} else if n > 0 {
			logger.Info("calendar periods reserved", "created", n)
		}
	}
	fillCalendar()
	logger.Info("period worker started", "tick_seconds", 1, "calendar_seconds", 60)
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-fill.C:
			fillCalendar()
		case <-tick.C:
			run, cancel := context.WithTimeout(ctx, 10*time.Second)
			n, e := store.Tick(run)
			cancel()
			if e != nil && ctx.Err() == nil {
				logger.Error("period transition failed", "error", e, "committed_transitions", n)
			} else if n > 0 {
				logger.Info("period transitions committed", "count", n)
			}
		}
	}
}
