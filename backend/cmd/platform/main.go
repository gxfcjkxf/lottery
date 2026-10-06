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
	"github.com/gxfcjkxf/lottery/backend/internal/telegramauth"
	"github.com/gxfcjkxf/lottery/backend/internal/tenant"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata" // Schedule validation must also work in minimal runtime images.
)

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
		return database.Seed(ctx, pools.Primary, c.Environment)
	case "create-admin":
		return createAdmin(ctx, pools.Primary)
	case "worker":
		return runPeriodWorker(ctx, pools.Primary, logger)
	case "serve":
	default:
		return errors.New("usage: platform serve|worker|migrate|seed|create-admin|generate-auth-key <file>")
	}
	key, err := loadKey(c)
	if err != nil {
		return err
	}
	engine, err := mutation.New(pools.Primary, key)
	if err != nil {
		return err
	}
	users, err := identity.New(pools.Primary)
	if err != nil {
		return err
	}
	server := &http.Server{Addr: c.HTTPAddr, Handler: httpapi.New(httpapi.Dependencies{Brands: tenant.Store{DB: pools.Primary}, Ready: pools.Primary.Ping, Logger: logger, Identity: users, Mutations: engine, Admins: adminsys.Store{DB: pools.Primary}, SecureCookies: c.Environment == "production", Telegram: telegramauth.Verifier{Keys: telegramauth.NewRemoteKeys()}, TrustedProxies: c.TrustedProxies, HistoryReads: database.NewHistoryRouter(pools.Replicas)}), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 16}
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
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
