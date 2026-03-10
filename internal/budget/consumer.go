package budget

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"budgetpulse/internal/outbox"

	"github.com/nats-io/nats.go/jetstream"
)

type Consumer struct {
	repo *Repository
	js   jetstream.JetStream
}

func NewConsumer(repo *Repository, js jetstream.JetStream) *Consumer {
	return &Consumer{
		repo: repo,
		js:   js,
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

	cc, err := consumer.Consume(func(msg jetstream.Msg) {
		processCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		var event TransactionCreatedEvent
		if err := json.Unmarshal(msg.Data(), &event); err != nil {
			log.Printf("budget consumer decode failed: %v", err)
			_ = msg.Ack()
			return
		}

		_, err := c.repo.ApplyTransactionCreated(processCtx, event)
		if err != nil {
			log.Printf("budget consumer apply failed: %v", err)
			_ = msg.Nak()
			return
		}

		_ = msg.Ack()
	})
	if err != nil {
		return err
	}
	defer cc.Stop()

	<-ctx.Done()
	return nil
}
