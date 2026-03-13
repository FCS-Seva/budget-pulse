package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"
	"time"

	"budgetpulse/internal/budget"
	"budgetpulse/internal/config"
	"budgetpulse/internal/outbox"
	"budgetpulse/internal/platform/logging"
	natsclient "budgetpulse/internal/platform/nats"
	"budgetpulse/internal/platform/postgres"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg := config.Load()
	logger := logging.NewLogger()

	db, err := postgres.NewPool(ctx, cfg.PostgresURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	nc, err := natsclient.New(cfg.NATSURL)
	if err != nil {
		log.Fatal(err)
	}
	defer nc.Close()

	outboxRepo := outbox.NewRepository(db)
	publisher := outbox.NewPublisher(outboxRepo, nc.JetStream, logger, time.Second)

	budgetRepo := budget.NewRepository(db)
	budgetConsumer := budget.NewConsumer(budgetRepo, nc.JetStream, logger)

	errCh := make(chan error, 2)

	go func() {
		if err := publisher.Run(ctx); err != nil {
			errCh <- err
		}
	}()

	go func() {
		if err := budgetConsumer.Run(ctx); err != nil {
			errCh <- err
		}
	}()

	logger.Info("worker started")

	select {
	case <-ctx.Done():
	case err := <-errCh:
		log.Fatal(err)
	}

	logger.Info("worker stopped")
}
