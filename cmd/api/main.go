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

	webhookProc, err := service.NewWebhookProcessor(
		repo,
		service.RetryConfig{
			MaxAttempts: cfg.Webhook.MaxAttempts,
			BaseDelay:   time.Duration(cfg.Webhook.BaseDelayMS) * time.Millisecond,
			MaxDelay:    time.Duration(cfg.Webhook.MaxDelayMS) * time.Millisecond,
			Jitter:      cfg.Webhook.RetryJitter(),
		},
		time.Duration(cfg.Webhook.CallTimeoutSec)*time.Second,
		time.Duration(cfg.Webhook.LeaseSeconds)*time.Second,
		srv.Logger,
		nil,
	)
	if err != nil {
		log.Fatal().Err(err).Msg("webhook processor failed")
	}
	webhookWorker := worker.NewWebhookWorker(
		webhookProc,
		cfg.Webhook.PoolSize,
		time.Duration(cfg.Webhook.PollIntervalMS)*time.Millisecond,
		srv.Logger,
	)

	mountHTTP(srv, repo)

	workerCtx, stopWorkers := context.WithCancel(context.Background())
	payWorker.Start(workerCtx)
	webhookWorker.Start(workerCtx)

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
		webhookWorker.Wait()
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

func mountHTTP(srv *server.Server, repo *repository.PaymentRepository) {
	merchantSvc := service.NewMerchantService(repository.NewMerchantRepository(srv.DB.Pool))
	router := handler.NewRouter(
		srv,
		middleware.NewMiddlewares(srv, merchantSvc.PublicIDByAPIKey),
		handler.NewPaymentHandler(service.NewPaymentService(repo), srv.Logger),
		handler.NewRefundHandler(service.NewRefundService(repo), srv.Logger),
		handler.NewAdminMerchantHandler(merchantSvc, srv.Logger),
		handler.NewMerchantHandler(merchantSvc, srv.Logger),
	)
	srv.SetupHTTPServer(router)
}
