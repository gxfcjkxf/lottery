package main

import (
	"context"
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/config"
	"github.com/gxfcjkxf/lottery/backend/internal/database"
	"github.com/gxfcjkxf/lottery/backend/internal/httpapi"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/gxfcjkxf/lottery/backend/internal/observability"
	"github.com/gxfcjkxf/lottery/backend/internal/opsmetrics"
	"github.com/gxfcjkxf/lottery/backend/internal/telegramauth"
	"github.com/gxfcjkxf/lottery/backend/internal/tenant"
	"github.com/gxfcjkxf/lottery/backend/internal/withdrawal"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata" // Schedule validation must also work in minimal runtime images.
)

var buildVersion = "development"

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)
	if err := run(logger); err != nil {
		logger.Error("platform stopped", "error", err.Error())
		os.Exit(1)
	}
}
func run(logger *slog.Logger) error {
	if len(os.Args) == 3 && os.Args[1] == "generate-auth-key" {
		return generateKey(os.Args[2])
	}
	if len(os.Args) == 3 && os.Args[1] == "generate-metrics-token" {
		return generateMetricsToken(os.Args[2])
	}
	c, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pools, err := database.Open(ctx, c)
	if err != nil {
		return err
	}
	defer pools.Close()
	command := "serve"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}
	switch command {
	case "migrate":
		return database.Migrate(ctx, pools.Primary)
	case "seed":
		if err := database.Seed(ctx, pools.Primary, c.Environment); err != nil {
			return err
		}
		return seedDevelopmentAdmin(ctx, pools.Primary, c.Environment)
	case "create-admin":
		return createAdmin(ctx, pools.Primary)
	case "check":
		return database.CheckMigrations(ctx, pools.Primary)
	case "worker":
	case "serve":
	default:
		return errors.New("usage: platform serve|worker|migrate|check|seed|create-admin|generate-auth-key <file>|generate-metrics-token <file>")
	}
	// Refuse anything other than the current schema before accepting requests or
	// starting business workers. This check never upgrades or repairs metadata.
	checkCtx, checkCancel := context.WithTimeout(ctx, 5*time.Second)
	err = database.CheckMigrations(checkCtx, pools.Primary)
	checkCancel()
	if err != nil {
		return errors.New("database schema unavailable or incompatible; initialize the current schema explicitly")
	}
	// Finish API dependency validation before any operational listener can
	// advertise readiness. Workers intentionally do not need the API auth key.
	var engine *mutation.Engine
	var users *identity.Store
	if command == "serve" {
		key, keyErr := loadKey(c)
		if keyErr != nil {
			return keyErr
		}
		engine, err = mutation.New(pools.Primary, key)
		if err != nil {
			return err
		}
		users, err = identity.New(pools.Primary)
		if err != nil {
			return err
		}
		users.Development = c.Environment == "development"
	}
	service := "api"
	if command == "worker" {
		service = "worker"
	}
	options, err := observability.LoadEnv(service, c.Environment, buildVersion)
	if err != nil {
		return err
	}
	telemetry, err := observability.New(ctx, options)
	if err != nil {
		return err
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = telemetry.Shutdown(closeCtx)
	}()
	ctx, cancelRun := context.WithCancel(ctx)
	defer cancelRun()
	ctx = observability.WithRuntime(ctx, telemetry)
	ready := func(check context.Context) error {
		if err := pools.Primary.Ping(check); err != nil {
			return errors.New("database unavailable")
		}
		return database.CheckMigrations(check, pools.Primary)
	}
	if options.MetricsAddr != "" {
		collector := opsmetrics.New(pools.Primary, pools.Replicas)
		if err := telemetry.Registry().Register(collector); err != nil {
			return errors.New("cannot register operations metrics")
		}
		collector.Start(ctx)
	}
	stopMetrics, err := telemetry.Start(ctx, ready)
	if err != nil {
		return err
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = stopMetrics(closeCtx)
	}()
	metricsFailure := make(chan error, 1)
	go func() {
		select {
		case e := <-telemetry.Errors():
			if e != nil {
				metricsFailure <- errors.New("operations listener stopped unexpectedly")
				cancelRun()
			}
		case <-ctx.Done():
		}
	}()
	if command == "worker" {
		err = runPeriodWorker(ctx, pools.Primary, logger)
		select {
		case e := <-metricsFailure:
			return e
		default:
			return err
		}
	}
	server := &http.Server{Addr: c.HTTPAddr, Handler: httpapi.New(httpapi.Dependencies{Brands: tenant.Store{DB: pools.Primary}, Ready: ready, Logger: logger, Identity: users, Mutations: engine, Admins: adminsys.Store{DB: pools.Primary}, WithdrawalEligibility: withdrawal.TurnoverChecker{}, SecureCookies: c.Environment == "production", Telegram: telegramauth.Verifier{Keys: telegramauth.NewRemoteKeys()}, TrustedProxies: c.TrustedProxies, HistoryReads: database.NewHistoryRouter(pools.Replicas), Telemetry: telemetry}), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 16}
	stopped := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = server.Shutdown(shutdown)
		case <-stopped:
		}
	}()
	logger.Info("API listening", "address", c.HTTPAddr, "environment", c.Environment)
	err = server.ListenAndServe()
	close(stopped)
	select {
	case e := <-metricsFailure:
		return e
	default:
	}
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
