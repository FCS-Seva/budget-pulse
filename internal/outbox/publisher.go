package outbox

import (
	"context"
	"log"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

type Publisher struct {
	repo         *Repository
	js           jetstream.JetStream
	pollInterval time.Duration
}

func NewPublisher(repo *Repository, js jetstream.JetStream, pollInterval time.Duration) *Publisher {
	return &Publisher{
		repo:         repo,
		js:           js,
		pollInterval: pollInterval,
	}
}

func (p *Publisher) Run(ctx context.Context) error {
	if err := p.ensureStream(ctx); err != nil {
		return err
	}

	ticker := time.NewTicker(p.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			for {
				processed, err := p.repo.PublishNextPending(ctx, p.publish)
				if err != nil {
					log.Printf("outbox publish failed: %v", err)
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
	return err
}
