package outbox

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) PublishNextPending(ctx context.Context, publish func(context.Context, Event) error) (bool, error) {
	dbtx, err := r.db.Begin(ctx)
	if err != nil {
		return false, err
	}

	done := false
	defer func() {
		if !done {
			_ = dbtx.Rollback(ctx)
		}
	}()

	event, found, err := r.lockNextPendingEvent(ctx, dbtx)
	if err != nil {
		return false, err
	}
	if !found {
		done = true
		_ = dbtx.Rollback(ctx)
		return false, nil
	}

	if err := publish(ctx, event); err != nil {
		return false, err
	}

	if err := r.markSentTx(ctx, dbtx, event.EventID); err != nil {
		return false, err
	}

	if err := dbtx.Commit(ctx); err != nil {
		return false, err
	}

	done = true
	return true, nil
}

func (r *Repository) lockNextPendingEvent(ctx context.Context, dbtx pgx.Tx) (Event, bool, error) {
	var event Event

	err := dbtx.QueryRow(
		ctx,
		`
		SELECT
			event_id,
			event_type,
			payload,
			status
		FROM outbox_events
		WHERE status = $1
		ORDER BY created_at
		LIMIT 1
		FOR UPDATE SKIP LOCKED
		`,
		StatusPending,
	).Scan(
		&event.EventID,
		&event.EventType,
		&event.Payload,
		&event.Status,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Event{}, false, nil
		}
		return Event{}, false, err
	}

	return event, true, nil
}

func (r *Repository) markSentTx(ctx context.Context, dbtx pgx.Tx, eventID string) error {
	_, err := dbtx.Exec(
		ctx,
		`
		UPDATE outbox_events
		SET status = $2
		WHERE event_id = $1
		`,
		eventID,
		StatusSent,
	)

	return err
}
