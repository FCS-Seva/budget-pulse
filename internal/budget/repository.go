package budget

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) CreateBudget(ctx context.Context, req CreateBudgetRequest) (Budget, error) {
	dbtx, err := r.db.Begin(ctx)
	if err != nil {
		return Budget{}, err
	}

	done := false
	defer func() {
		if !done {
			_ = dbtx.Rollback(ctx)
		}
	}()

	ok, err := r.categoryExistsForUserTx(ctx, dbtx, req.UserID, req.CategoryID)
	if err != nil {
		return Budget{}, err
	}
	if !ok {
		return Budget{}, ValidationError{Message: "category_id does not belong to user"}
	}

	var budget Budget
	err = dbtx.QueryRow(
		ctx,
		`
		INSERT INTO budgets (
			user_id,
			category_id,
			period_type,
			period_start,
			limit_amount
		)
		VALUES ($1, $2, 'month', $3, $4)
		RETURNING
			id,
			user_id,
			category_id,
			period_type,
			period_start,
			limit_amount::text,
			created_at
		`,
		req.UserID,
		req.CategoryID,
		req.PeriodStart,
		req.LimitAmount,
	).Scan(
		&budget.ID,
		&budget.UserID,
		&budget.CategoryID,
		&budget.PeriodType,
		&budget.PeriodStart,
		&budget.LimitAmount,
		&budget.CreatedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return Budget{}, ConflictError{Message: "budget already exists for this category and month"}
		}
		return Budget{}, err
	}

	if err := dbtx.Commit(ctx); err != nil {
		return Budget{}, err
	}

	done = true
	return budget, nil
}

func (r *Repository) GetBudgetStatus(ctx context.Context, userID, categoryID int64, periodStart time.Time) (BudgetStatus, error) {
	var status BudgetStatus

	err := r.db.QueryRow(
		ctx,
		`
		SELECT
			b.limit_amount::text,
			COALESCE(bs.spent_amount, 0)::text,
			COALESCE(bs.remaining_amount, b.limit_amount)::text,
			COALESCE(bs.spent_amount, 0) > b.limit_amount
		FROM budgets b
		LEFT JOIN budget_stats bs
			ON bs.user_id = b.user_id
			AND bs.category_id = b.category_id
			AND bs.period_type = b.period_type
			AND bs.period_start = b.period_start
		WHERE b.user_id = $1
		  AND b.category_id = $2
		  AND b.period_type = 'month'
		  AND b.period_start = $3
		`,
		userID,
		categoryID,
		periodStart,
	).Scan(
		&status.LimitAmount,
		&status.SpentAmount,
		&status.RemainingAmount,
		&status.Exceeded,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BudgetStatus{}, NotFoundError{Message: "budget not found"}
		}
		return BudgetStatus{}, err
	}

	return status, nil
}

func (r *Repository) ApplyTransactionCreated(ctx context.Context, event TransactionCreatedEvent) (bool, error) {
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

	inserted, err := r.insertProcessedEventTx(ctx, dbtx, event.EventID)
	if err != nil {
		return false, err
	}
	if !inserted {
		done = true
		_ = dbtx.Rollback(ctx)
		return false, nil
	}

	if event.Type != "expense" || event.CategoryID == nil {
		if err := dbtx.Commit(ctx); err != nil {
			return false, err
		}
		done = true
		return true, nil
	}

	periodStart := monthStart(event.OccurredAt)

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
			if err := dbtx.Commit(ctx); err != nil {
				return false, err
			}
			done = true
			return true, nil
		}
		return false, err
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
		return false, err
	}

	if err := dbtx.Commit(ctx); err != nil {
		return false, err
	}

	done = true
	return true, nil
}

func (r *Repository) insertProcessedEventTx(ctx context.Context, dbtx pgx.Tx, eventID string) (bool, error) {
	var insertedEventID string

	err := dbtx.QueryRow(
		ctx,
		`
		INSERT INTO processed_events (event_id)
		VALUES ($1)
		ON CONFLICT DO NOTHING
		RETURNING event_id
		`,
		eventID,
	).Scan(&insertedEventID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

func (r *Repository) categoryExistsForUserTx(ctx context.Context, dbtx pgx.Tx, userID, categoryID int64) (bool, error) {
	var exists bool

	err := dbtx.QueryRow(
		ctx,
		`
		SELECT EXISTS(
			SELECT 1
			FROM categories
			WHERE id = $1 AND user_id = $2
		)
		`,
		categoryID,
		userID,
	).Scan(&exists)
	if err != nil {
		return false, err
	}

	return exists, nil
}

func monthStart(t time.Time) time.Time {
	utc := t.UTC()
	return time.Date(utc.Year(), utc.Month(), 1, 0, 0, 0, 0, time.UTC)
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
