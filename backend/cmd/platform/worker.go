package main

import (
	"context"
	"github.com/gxfcjkxf/lottery/backend/internal/betting"
	"github.com/gxfcjkxf/lottery/backend/internal/drawfeed"
	"github.com/gxfcjkxf/lottery/backend/internal/rulebook"
	"github.com/jackc/pgx/v5/pgxpool"
	"log/slog"
	"time"
)

func runPeriodWorker(ctx context.Context, db *pgxpool.Pool, logger *slog.Logger) error {
	store := rulebook.Store{DB: db}
	// Draw I/O has its own bounded loop; slow future adapters must not delay the
	// one-second period clock. Exactly one collector loop lives per worker.
	drawCtx, stopDraw := context.WithCancel(ctx)
	drawDone := make(chan struct{})
	go func() { defer close(drawDone); runDrawWorker(drawCtx, store, logger) }()
	defer func() { stopDraw(); <-drawDone }()
	refundCtx, stopRefund := context.WithCancel(ctx)
	refundDone := make(chan struct{})
	go func() { defer close(refundDone); runCancellationWorker(refundCtx, betting.Service{DB: db}, logger) }()
	defer func() { stopRefund(); <-refundDone }()
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

func runCancellationWorker(ctx context.Context, service betting.Service, logger *slog.Logger) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run, cancel := context.WithTimeout(ctx, 10*time.Second)
			n, err := service.ProcessCancellations(run, 20)
			cancel()
			if err != nil && ctx.Err() == nil {
				logger.Error("period refunds failed", "error", err, "committed_targets", n)
			} else if n > 0 {
				logger.Info("period refunds committed", "targets", n)
			}
		}
	}
}

func runDrawWorker(ctx context.Context, store rulebook.Store, logger *slog.Logger) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	resolver := drawfeed.DefaultResolver(5 * time.Second)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run, cancel := context.WithTimeout(ctx, 10*time.Second)
			n, err := store.CollectDraws(run, resolver)
			cancel()
			if err != nil && ctx.Err() == nil {
				logger.Error("draw collection failed", "error", err, "committed_results", n)
			} else if n > 0 {
				logger.Info("draw results locked", "count", n)
			}
		}
	}
}
