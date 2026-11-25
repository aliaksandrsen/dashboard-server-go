package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aliaksandrsen/dashboard-server-go/internal/config"
	"github.com/aliaksandrsen/dashboard-server-go/internal/health"
	"github.com/aliaksandrsen/dashboard-server-go/internal/httputil/middleware"
	"github.com/aliaksandrsen/dashboard-server-go/internal/jobs"
	"github.com/aliaksandrsen/dashboard-server-go/internal/logger"
	"github.com/aliaksandrsen/dashboard-server-go/internal/orders"
	"github.com/aliaksandrsen/dashboard-server-go/internal/platform/cache"
	"github.com/aliaksandrsen/dashboard-server-go/internal/platform/origin"
	"github.com/aliaksandrsen/dashboard-server-go/internal/platform/postgres"
	"github.com/aliaksandrsen/dashboard-server-go/internal/products"
	"github.com/aliaksandrsen/dashboard-server-go/internal/repository"
	"github.com/aliaksandrsen/dashboard-server-go/internal/server"
	"github.com/aliaksandrsen/dashboard-server-go/internal/stripe"
	"github.com/aliaksandrsen/dashboard-server-go/internal/users"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	log, closeLog, err := logger.New(string(cfg.NodeEnv), logger.DefaultLogFilePath, os.Stdout)
	if err != nil {
		return fmt.Errorf("init logger: %w", err)
	}
	defer func() { _ = closeLog() }()

	initCtx, cancelInit := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelInit()

	pool, err := postgres.NewPool(initCtx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect postgres: %w", err)
	}
	defer pool.Close()

	cacheStore, err := cache.NewRedisStore(cfg.EffectiveRedisURL())
	if err != nil {
		return fmt.Errorf("init redis cache: %w", err)
	}
	defer func() { _ = cacheStore.Close() }()

	usersRepo := repository.NewUsersRepository(pool)
	productsRepo := repository.NewProductsRepository(pool)
	ordersRepo := repository.NewOrdersRepository(pool)

	stripeSvc := stripe.NewService(cfg.StripeSecretKey, string(cfg.NodeEnv), cfg.FrontendURL, log)
	usersSvc := users.NewService(usersRepo, log)
	productsSvc := products.NewService(productsRepo, stripeSvc, log)
	ordersSvc := orders.NewService(ordersRepo, log)

	syncSvc, err := jobs.NewSyncService(
		ordersRepo,
		usersRepo,
		stripeSvc,
		cfg.StripeSyncCron,
		cfg.StripeSyncTimezone,
		log,
	)
	if err != nil {
		return fmt.Errorf("init sync service: %w", err)
	}
	if err := syncSvc.Start(); err != nil {
		return fmt.Errorf("start cron scheduler: %w", err)
	}

	rateLimiter := middleware.NewRateLimiter(
		time.Duration(cfg.ThrottleTTLMS)*time.Millisecond,
		cfg.ThrottleLimit,
		log,
	)
	defer rateLimiter.Close()

	router := server.NewRouter(server.Dependencies{
		AllowedOrigins: origin.GetAllowedOrigins(cfg.FrontendURL),
		RateLimiter:    rateLimiter,
		Logger:         log,
		Health:         health.NewHandler(pool, cacheStore, log),
		Users:          users.NewHandler(usersSvc, log),
		Products:       products.NewHandler(productsSvc, cacheStore, log),
		Orders:         orders.NewHandler(ordersSvc, log),
		Stripe:         stripe.NewHandler(stripeSvc, log),
		Jobs:           jobs.NewHandler(syncSvc, log),
	})

	httpServer := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("DashboardServer listening", "port", cfg.Port, "env", cfg.NodeEnv)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	sigCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	select {
	case err := <-errCh:
		return err
	case <-sigCtx.Done():
		log.Info("Shutting down gracefully...")
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Error("HTTP server shutdown error", "error", err)
	}
	if err := syncSvc.Stop(shutdownCtx); err != nil {
		log.Error("Cron scheduler shutdown error", "error", err)
	}

	return nil
}
