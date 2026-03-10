package budget

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) ApplyExpenseTransaction(ctx context.Context, event TransactionCreatedEvent) error {
	if event.CategoryID == nil {
		return nil
	}

	periodStart := monthStart(event.OccurredAt)

	dbtx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}

	done := false
	defer func() {
		if !done {
			_ = dbtx.Rollback(ctx)
		}
	}()

	var limitAmount string
	err = dbtx.QueryRow(
		ctx,
		`
		SELECT limit_amount::text
		FROM budgets
		WHERE user_id = $1
		  AND category_id = $2
		  AND period_type = 'month'
		  AND period_start = $3
		`,
		event.UserID,
		*event.CategoryID,
		periodStart,
	).Scan(&limitAmount)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			done = true
			_ = dbtx.Rollback(ctx)
			return nil
		}
		return err
	}

	_, err = dbtx.Exec(
		ctx,
		`
		INSERT INTO budget_stats (
			user_id,
			category_id,
			period_type,
			period_start,
			spent_amount,
			remaining_amount,
			updated_at
		)
		VALUES ($1, $2, 'month', $3, $4::numeric, $5::numeric - $4::numeric, NOW())
		ON CONFLICT (user_id, category_id, period_type, period_start)
		DO UPDATE SET
			spent_amount = budget_stats.spent_amount + EXCLUDED.spent_amount,
			remaining_amount = $5::numeric - (budget_stats.spent_amount + EXCLUDED.spent_amount),
			updated_at = NOW()
		`,
		event.UserID,
		*event.CategoryID,
		periodStart,
		event.Amount,
		limitAmount,
	)
	if err != nil {
		return err
	}

	if err := dbtx.Commit(ctx); err != nil {
		return err
	}

	done = true
	return nil
}

func monthStart(t time.Time) time.Time {
	utc := t.UTC()
	return time.Date(utc.Year(), utc.Month(), 1, 0, 0, 0, 0, time.UTC)
}
