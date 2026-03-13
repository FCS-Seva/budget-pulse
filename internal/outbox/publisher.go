package outbox

import (
	"context"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

type Publisher struct {
	repo         *Repository
	js           jetstream.JetStream
	logger       *slog.Logger
	pollInterval time.Duration
}

func NewPublisher(repo *Repository, js jetstream.JetStream, logger *slog.Logger, pollInterval time.Duration) *Publisher {
	return &Publisher{
		repo:         repo,
		js:           js,
		logger:       logger,
		pollInterval: pollInterval,
	}
}

func (p *Publisher) Run(ctx context.Context) error {
	if err := p.ensureStream(ctx); err != nil {
		return err
	}

	p.logger.Info("outbox publisher started", "stream", StreamName, "subject", SubjectTransactionCreated, "poll_interval", p.pollInterval.String())

	ticker := time.NewTicker(p.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			p.logger.Info("outbox publisher stopped")
			return nil
		case <-ticker.C:
			for {
				processed, err := p.repo.PublishNextPending(ctx, p.publish)
				if err != nil {
					p.logger.Error("outbox publish failed", "error", err)
					break
				}
				if !processed {
					break
				}
			}
		}
	}
}

func (p *Publisher) ensureStream(ctx context.Context) error {
	_, err := p.js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:     StreamName,
		Subjects: []string{SubjectTransactionCreated},
	})

	return err
}

func (p *Publisher) publish(ctx context.Context, event Event) error {
	_, err := p.js.Publish(ctx, event.EventType, event.Payload)
	if err != nil {
		return err
	}

	p.logger.Info("outbox event published", "event_id", event.EventID, "event_type", event.EventType, "status", event.Status)

	return nil
}
