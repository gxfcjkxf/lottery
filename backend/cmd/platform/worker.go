package main

import (
	"context"
	"github.com/gxfcjkxf/lottery/backend/internal/betting"
	"github.com/gxfcjkxf/lottery/backend/internal/commission"
	"github.com/gxfcjkxf/lottery/backend/internal/drawfeed"
	"github.com/gxfcjkxf/lottery/backend/internal/notification"
	"github.com/gxfcjkxf/lottery/backend/internal/reconciliation"
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
	settleCtx, stopSettle := context.WithCancel(ctx)
	settleDone := make(chan struct{})
	go func() { defer close(settleDone); runSettlementWorker(settleCtx, betting.Service{DB: db}, logger) }()
	defer func() { stopSettle(); <-settleDone }()
	correctionCtx, stopCorrection := context.WithCancel(ctx)
	correctionDone := make(chan struct{})
	go func() {
		defer close(correctionDone)
		runCorrectionWorker(correctionCtx, betting.Service{DB: db}, logger)
	}()
	defer func() { stopCorrection(); <-correctionDone }()
	inboxCtx, stopInbox := context.WithCancel(ctx)
	inboxDone := make(chan struct{})
	go func() { defer close(inboxDone); runNotificationWorker(inboxCtx, notification.Service{DB: db}, logger) }()
	defer func() { stopInbox(); <-inboxDone }()
	reconcileCtx, stopReconcile := context.WithCancel(ctx)
	reconcileDone := make(chan struct{})
	go func() {
		defer close(reconcileDone)
		runReconciliationWorker(reconcileCtx, reconciliation.Service{DB: db}, logger)
	}()
	defer func() { stopReconcile(); <-reconcileDone }()
	commissionCtx, stopCommission := context.WithCancel(ctx)
	commissionDone := make(chan struct{})
	go func() {
		defer close(commissionDone)
		runCommissionWorker(commissionCtx, commission.Service{DB: db}, logger)
	}()
	defer func() { stopCommission(); <-commissionDone }()
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

func runCommissionWorker(ctx context.Context, service commission.Service, logger *slog.Logger) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run, cancel := context.WithTimeout(ctx, 10*time.Second)
			n, err := service.ProcessCycles(run, 20)
			cancel()
			if err != nil && ctx.Err() == nil {
				logger.Error("commission cycle processing failed", "committed_steps", n)
			}
		}
	}
}

func runReconciliationWorker(ctx context.Context, service reconciliation.Service, logger *slog.Logger) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run, cancel := context.WithTimeout(ctx, 10*time.Second)
			n, e := service.Process(run, 20)
			cancel()
			if e != nil && ctx.Err() == nil {
				logger.Error("wallet reconciliation processing failed", "committed_steps", n)
			}
		}
	}
}

func runCorrectionWorker(ctx context.Context, service betting.Service, logger *slog.Logger) {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			run, cancel := context.WithTimeout(ctx, 10*time.Second)
			n, e := service.ProcessCorrections(run, 20)
			cancel()
			if e != nil && ctx.Err() == nil {
				logger.Error("draw correction processing failed", "error", e, "committed_steps", n)
			}
		}
	}
}
func runSettlementWorker(ctx context.Context, service betting.Service, logger *slog.Logger) {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			run, cancel := context.WithTimeout(ctx, 10*time.Second)
			n, e := service.ProcessSettlements(run, 20)
			cancel()
			if e != nil && ctx.Err() == nil {
				logger.Error("settlement processing failed", "error", e, "committed_steps", n)
			}
		}
	}
}

func runNotificationWorker(ctx context.Context, service notification.Service, logger *slog.Logger) {
	// A 20/s polling cap would necessarily lag behind the stated 500 bets/s
	// target. This is a bounded burst allowance, not a throughput guarantee;
	// real delivery latency and scaling are still subject to S7 load tests.
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run, cancel := context.WithTimeout(ctx, 10*time.Second)
			n, err := service.Process(run, 100)
			cancel()
			if err != nil && ctx.Err() == nil {
				logger.Error("in-app notification processing failed", "error", err, "committed_events", n)
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
