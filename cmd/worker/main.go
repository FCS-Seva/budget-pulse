package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"budgetpulse/internal/config"
	natsclient "budgetpulse/internal/platform/nats"
	"budgetpulse/internal/platform/postgres"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg := config.Load()

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

	log.Print("worker started")

	<-ctx.Done()

	log.Print("worker stopped")
}
