package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ariwalapratham/go-payment-simulator/internal/bank"
	"github.com/ariwalapratham/go-payment-simulator/internal/config"
	"github.com/ariwalapratham/go-payment-simulator/internal/database"
	"github.com/ariwalapratham/go-payment-simulator/internal/handler"
	"github.com/ariwalapratham/go-payment-simulator/internal/logger"
	"github.com/ariwalapratham/go-payment-simulator/internal/middleware"
	"github.com/ariwalapratham/go-payment-simulator/internal/repository"
	"github.com/ariwalapratham/go-payment-simulator/internal/server"
	"github.com/ariwalapratham/go-payment-simulator/internal/service"
	"github.com/ariwalapratham/go-payment-simulator/internal/worker"
)

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		panic(err)
	}

	loggerService := logger.NewLoggerService(cfg.Observability)
	defer loggerService.Shutdown()
	log := logger.NewLoggerWithService(cfg.Observability, loggerService)

	ctx := context.Background()
	if err := database.Migrate(ctx, &log, cfg); err != nil {
		log.Fatal().Err(err).Msg("migrate failed")
	}

	srv, err := server.New(cfg, &log, loggerService)
	if err != nil {
		log.Fatal().Err(err).Msg("server init failed")
	}

	gw, err := bank.NewGateway(cfg.Bank.Provider, cfg.Bank.Simulator)
	if err != nil {
		log.Fatal().Err(err).Msg("bank gateway failed")
	}

	repo := repository.NewPaymentRepository(srv.DB.Pool)
	processor, err := service.NewPaymentProcessor(
		repo,
		gw,
		service.RetryConfig{
			MaxAttempts: cfg.Worker.MaxAttempts,
			BaseDelay:   time.Duration(cfg.Worker.BaseDelayMS) * time.Millisecond,
			MaxDelay:    time.Duration(cfg.Worker.MaxDelayMS) * time.Millisecond,
			Jitter:      cfg.Worker.RetryJitter(),
		},
		time.Duration(cfg.Bank.CallTimeout)*time.Second,
		time.Duration(cfg.Worker.LeaseSeconds)*time.Second,
		srv.Logger,
		nil,
	)
	if err != nil {
		log.Fatal().Err(err).Msg("payment processor failed")
	}
	payWorker := worker.NewPaymentWorker(
		processor,
		cfg.Worker.PoolSize,
		time.Duration(cfg.Worker.PollIntervalMS)*time.Millisecond,
		srv.Logger,
	)

	mw := middleware.NewMiddlewares(srv)
	payments := handler.NewPaymentHandler(service.NewPaymentService(repo), srv.Logger)
	refunds := handler.NewRefundHandler(service.NewRefundService(repo), srv.Logger)
	router := handler.NewRouter(srv, mw, payments, refunds)
	srv.SetupHTTPServer(router)

	workerCtx, stopWorkers := context.WithCancel(context.Background())
	payWorker.Start(workerCtx)

	go func() {
		if err := srv.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal().Err(err).Msg("server failed")
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	stopWorkers()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.ShutdownHTTP(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("http shutdown failed")
	}

	done := make(chan struct{})
	go func() {
		payWorker.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-shutdownCtx.Done():
		log.Error().Msg("worker shutdown timed out")
	}

	if err := srv.CloseDB(); err != nil {
		log.Error().Err(err).Msg("database close failed")
	}
}
