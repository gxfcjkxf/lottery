package main

import (
	"context"
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/betting"
	"github.com/gxfcjkxf/lottery/backend/internal/brandops"
	"github.com/gxfcjkxf/lottery/backend/internal/commission"
	"github.com/gxfcjkxf/lottery/backend/internal/drawfeed"
	"github.com/gxfcjkxf/lottery/backend/internal/notification"
	"github.com/gxfcjkxf/lottery/backend/internal/observability"
	"github.com/gxfcjkxf/lottery/backend/internal/platformlottery"
	"github.com/gxfcjkxf/lottery/backend/internal/reconciliation"
	"github.com/gxfcjkxf/lottery/backend/internal/reportarchive"
	"github.com/gxfcjkxf/lottery/backend/internal/rulebook"
	"github.com/jackc/pgx/v5/pgxpool"
	"log/slog"
	"time"
)

// Observation does not change retries, locks, task state or committed counts.
// A panic is still fatal to this worker; it is never converted to success.
func observedWork(ctx context.Context, component string, work func(context.Context) (int, error)) (n int, err error) {
	ctx, finish := observability.BeginWork(ctx, component)
	defer func() {
		if value := recover(); value != nil {
			finish(n, errors.New("worker panic"))
			panic(value)
		}
		finish(n, err)
	}()
	return work(ctx)
}

func runPeriodWorker(ctx context.Context, db *pgxpool.Pool, logger *slog.Logger) error {
	store := rulebook.Store{DB: db}
	sources, e := platformlottery.LoadAutomationSources()
	if e != nil {
		return e
	}
	autoCtx, stopAuto := context.WithCancel(ctx)
	autoDone := make(chan struct{})
	go func() {
		defer close(autoDone)
		runSharedAutomation(autoCtx, platformlottery.Service{DB: db}, sources, logger)
	}()
	defer func() { stopAuto(); <-autoDone }()
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
	markSixCtx, stopMarkSix := context.WithCancel(ctx)
	markSixDone := make(chan struct{})
	go func() { defer close(markSixDone); runMarkSixWorker(markSixCtx, betting.Service{DB: db}, logger) }()
	defer func() { stopMarkSix(); <-markSixDone }()
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
	archiveCtx, stopArchive := context.WithCancel(ctx)
	archiveDone := make(chan struct{})
	go func() {
		defer close(archiveDone)
		runReportArchiveWorker(archiveCtx, reportarchive.Service{DB: db}, logger)
	}()
	defer func() { stopArchive(); <-archiveDone }()
	commissionCtx, stopCommission := context.WithCancel(ctx)
	commissionDone := make(chan struct{})
	go func() {
		defer close(commissionDone)
		runCommissionWorker(commissionCtx, commission.Service{DB: db}, logger)
	}()
	defer func() { stopCommission(); <-commissionDone }()
	pauseCtx, stopPause := context.WithCancel(ctx)
	pauseDone := make(chan struct{})
	go func() {
		defer close(pauseDone)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			run, cancel := context.WithTimeout(pauseCtx, 10*time.Second)
			_, err := (brandops.Service{DB: db}).Tick(run)
			cancel()
			if err != nil && pauseCtx.Err() == nil {
				logger.Error("brand pause transition failed", "error", err)
			}
			select {
			case <-pauseCtx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	defer func() { stopPause(); <-pauseDone }()
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	fill := time.NewTicker(time.Minute)
	defer fill.Stop()
	fillCalendar := func() {
		run, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		n, e := observedWork(run, "calendar", store.FillCalendar)
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
			n, e := observedWork(run, "period_tick", store.Tick)
			cancel()
			if e != nil && ctx.Err() == nil {
				logger.Error("period transition failed", "error", e, "committed_transitions", n)
			} else if n > 0 {
				logger.Info("period transitions committed", "count", n)
			}
		}
	}
}

// Archive reporting never shares the one-second betting clock. Policies are
// opt-in; independent bounded transactions allow multiple workers safely.
func runReportArchiveWorker(ctx context.Context, s reportarchive.Service, logger *slog.Logger) {
	discovery := time.NewTicker(time.Minute)
	processing := time.NewTicker(time.Second)
	defer discovery.Stop()
	defer processing.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-discovery.C:
			run, cancel := context.WithTimeout(ctx, 30*time.Second)
			n, err := observedWork(run, "archive_discovery", func(ctx context.Context) (int, error) { return s.DiscoverAutomatic(ctx, 20) })
			cancel()
			if err != nil && ctx.Err() == nil {
				logger.Error("automatic archive discovery failed", "committed_tasks", n)
			}
		case <-processing.C:
			run, cancel := context.WithTimeout(ctx, 10*time.Second)
			n, err := observedWork(run, "archive_processing", func(ctx context.Context) (int, error) { return s.ProcessAutomatic(ctx, 20) })
			cancel()
			if err != nil && ctx.Err() == nil {
				logger.Error("automatic archive processing failed", "committed_tasks", n)
			}
		}
	}
}

func runCommissionWorker(ctx context.Context, service commission.Service, logger *slog.Logger) {
	paymentCtx, stopPayment := context.WithCancel(ctx)
	paymentDone := make(chan struct{})
	go func() { defer close(paymentDone); runCommissionPaymentWorker(paymentCtx, service, logger) }()
	defer func() { stopPayment(); <-paymentDone }()
	planCtx, stopPlans := context.WithCancel(ctx)
	planDone := make(chan struct{})
	go func() { defer close(planDone); runCommissionCorrectionPlanWorker(planCtx, service, logger) }()
	defer func() { stopPlans(); <-planDone }()
	executionCtx, stopExecutions := context.WithCancel(ctx)
	executionDone := make(chan struct{})
	go func() {
		defer close(executionDone)
		runCommissionCorrectionExecutionWorker(executionCtx, service, logger)
	}()
	defer func() { stopExecutions(); <-executionDone }()
	discoveryCtx, stopDiscovery := context.WithCancel(ctx)
	discoveryDone := make(chan struct{})
	go func() {
		defer close(discoveryDone)
		runCommissionDiscoveryWorker(discoveryCtx, service, logger)
	}()
	defer func() { stopDiscovery(); <-discoveryDone }()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run, cancel := context.WithTimeout(ctx, 10*time.Second)
			n, err := observedWork(run, "commission_cycle", func(ctx context.Context) (int, error) { return service.ProcessCycles(ctx, 20) })
			cancel()
			if err != nil && ctx.Err() == nil {
				logger.Error("commission cycle processing failed", "committed_steps", n)
			}
		}
	}
}

func runCommissionPaymentWorker(ctx context.Context, service commission.Service, logger *slog.Logger) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run, cancel := context.WithTimeout(ctx, 10*time.Second)
			n, err := observedWork(run, "commission_payment", func(ctx context.Context) (int, error) { return service.ProcessPayments(ctx, 20) })
			cancel()
			if err != nil && ctx.Err() == nil {
				logger.Error("commission payment processing failed", "committed_steps", n)
			}
		}
	}
}

func runCommissionCorrectionPlanWorker(ctx context.Context, service commission.Service, logger *slog.Logger) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run, cancel := context.WithTimeout(ctx, 10*time.Second)
			n, err := observedWork(run, "commission_correction_plan", func(ctx context.Context) (int, error) { return service.ProcessCorrectionPlans(ctx, 20) })
			cancel()
			if err != nil && ctx.Err() == nil {
				logger.Error("commission correction plan preparation failed", "committed_steps", n)
			}
		}
	}
}

func runCommissionCorrectionExecutionWorker(ctx context.Context, service commission.Service, logger *slog.Logger) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run, cancel := context.WithTimeout(ctx, 10*time.Second)
			n, err := observedWork(run, "commission_correction_execution", func(ctx context.Context) (int, error) { return service.ProcessCorrectionExecutions(ctx, 20) })
			cancel()
			if err != nil && ctx.Err() == nil {
				logger.Error("commission correction execution failed", "committed_steps", n)
			}
		}
	}
}

func runCommissionDiscoveryWorker(ctx context.Context, service commission.Service, logger *slog.Logger) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run, cancel := context.WithTimeout(ctx, 5*time.Second)
			n, err := observedWork(run, "commission_discovery", func(ctx context.Context) (int, error) { return service.ProcessDiscovery(ctx, 100) })
			cancel()
			if err != nil && ctx.Err() == nil {
				logger.Error("commission discovery processing failed", "committed_records", n)
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
			n, e := observedWork(run, "reconciliation", func(ctx context.Context) (int, error) { return service.Process(ctx, 20) })
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
			n, e := observedWork(run, "draw_correction", func(ctx context.Context) (int, error) { return service.ProcessCorrections(ctx, 20) })
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
			n, e := observedWork(run, "settlement", func(ctx context.Context) (int, error) {
				legacyCount, legacyErr := service.ProcessSettlements(ctx, 20)
				configuredCount, configuredErr := service.ProcessConfiguredSettlements(ctx, 20)
				return legacyCount + configuredCount, errors.Join(legacyErr, configuredErr)
			})
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
			n, err := observedWork(run, "notification", func(ctx context.Context) (int, error) { return service.Process(ctx, 100) })
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
			n, err := observedWork(run, "period_refund", func(ctx context.Context) (int, error) { return service.ProcessCancellations(ctx, 20) })
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
			n, err := observedWork(run, "draw_collection", func(ctx context.Context) (int, error) { return store.CollectDraws(ctx, resolver) })
			cancel()
			if err != nil && ctx.Err() == nil {
				logger.Error("draw collection failed", "error", err, "committed_results", n)
			} else if n > 0 {
				logger.Info("draw results locked", "count", n)
			}
		}
	}
}

func runMarkSixWorker(ctx context.Context, service betting.Service, logger *slog.Logger) {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			run, cancel := context.WithTimeout(ctx, 10*time.Second)
			_, canadaErr := service.ProcessCanadaSettlements(run, 20)
			if canadaErr != nil {
				logger.Error("Canada settlement failed", "error", canadaErr)
			}
			n, e := observedWork(run, "mark_six_settlement", func(ctx context.Context) (int, error) { return service.ProcessMarkSixSettlements(ctx, 20) })
			cancel()
			if e != nil && ctx.Err() == nil {
				logger.Error("Mark Six settlement failed", "error", e, "committed_orders", n)
			}
		}
	}
}

func runSharedAutomation(ctx context.Context, s platformlottery.Service, sources platformlottery.AutomationSources, logger *slog.Logger) {
	collectCtx, stop := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		for {
			select {
			case <-collectCtx.Done():
				return
			case <-tick.C:
				run, cancel := context.WithTimeout(collectCtx, 40*time.Second)
				_, e := s.CollectAutomaticDraws(run, sources)
				cancel()
				if e != nil && collectCtx.Err() == nil {
					logger.Error("shared draw collection failed", "error", e)
				}
			}
		}
	}()
	defer func() { stop(); <-done }()
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		run, cancel := context.WithTimeout(ctx, 10*time.Second)
		_, e := s.GenerateAutomaticPeriods(run)
		cancel()
		if e != nil && ctx.Err() == nil {
			logger.Error("shared period generation failed", "error", e)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
