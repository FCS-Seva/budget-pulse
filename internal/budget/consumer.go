package budget

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"budgetpulse/internal/outbox"

	"github.com/nats-io/nats.go/jetstream"
)

type Consumer struct {
	repo   *Repository
	js     jetstream.JetStream
	logger *slog.Logger
}

func NewConsumer(repo *Repository, js jetstream.JetStream, logger *slog.Logger) *Consumer {
	return &Consumer{
		repo:   repo,
		js:     js,
		logger: logger,
	}
}

func (c *Consumer) Run(ctx context.Context) error {
	consumer, err := c.js.CreateOrUpdateConsumer(ctx, outbox.StreamName, jetstream.ConsumerConfig{
		Durable:       "budget-consumer",
		AckPolicy:     jetstream.AckExplicitPolicy,
		FilterSubject: outbox.SubjectTransactionCreated,
	})
	if err != nil {
		return err
	}

	c.logger.Info("budget consumer started", "durable", "budget-consumer", "subject", outbox.SubjectTransactionCreated)

	cc, err := consumer.Consume(func(msg jetstream.Msg) {
		processCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		var event TransactionCreatedEvent
		if err := json.Unmarshal(msg.Data(), &event); err != nil {
			c.logger.Error("budget consumer decode failed", "error", err)
			_ = msg.Ack()
			return
		}

		processed, err := c.repo.ApplyTransactionCreated(processCtx, event)
		if err != nil {
			c.logger.Error(
				"budget consumer apply failed",
				"event_id", event.EventID,
				"transaction_id", event.TransactionID,
				"user_id", event.UserID,
				"error", err,
			)
			_ = msg.Nak()
			return
		}

		c.logger.Info(
			"budget event processed",
			"event_id", event.EventID,
			"transaction_id", event.TransactionID,
			"user_id", event.UserID,
			"processed", processed,
			"type", event.Type,
			"category_id", event.CategoryID,
		)

		_ = msg.Ack()
	})
	if err != nil {
		return err
	}
	defer cc.Stop()

	<-ctx.Done()
	c.logger.Info("budget consumer stopped")
	return nil
}
