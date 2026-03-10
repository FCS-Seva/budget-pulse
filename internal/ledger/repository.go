package ledger

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"budgetpulse/internal/outbox"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

type idempotencyRecord struct {
	RequestHash  string
	ResponseBody []byte
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) CreateTransaction(ctx context.Context, req CreateTransactionRequest, idempotencyKey, requestHash string) (Transaction, bool, error) {
	rec, found, err := r.getTransactionByIdempotencyKey(ctx, req.UserID, idempotencyKey)
	if err != nil {
		return Transaction{}, false, err
	}

	if found {
		if rec.RequestHash != requestHash {
			return Transaction{}, false, ConflictError{Message: "Idempotency-Key is already used with different payload"}
		}

		tx, err := decodeStoredTransaction(rec.ResponseBody)
		if err != nil {
			return Transaction{}, false, err
		}

		return tx, true, nil
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Transaction{}, false, err
	}
	defer tx.Rollback(ctx)

	if req.CategoryID != nil {
		ok, err := r.categoryExistsForUserTx(ctx, tx, req.UserID, *req.CategoryID)
		if err != nil {
			return Transaction{}, false, err
		}
		if !ok {
			return Transaction{}, false, ValidationError{Message: "category_id does not belong to user"}
		}
	}

	created, err := r.createTransactionTx(ctx, tx, req)
	if err != nil {
		return Transaction{}, false, err
	}

	event, err := newTransactionCreatedEvent(created)
	if err != nil {
		return Transaction{}, false, err
	}

	if err := r.insertOutboxEventTx(ctx, tx, event); err != nil {
		return Transaction{}, false, err
	}

	responseBody, err := json.Marshal(created)
	if err != nil {
		return Transaction{}, false, err
	}

	err = r.saveIdempotencyKeyTx(
		ctx,
		tx,
		req.UserID,
		idempotencyKey,
		requestHash,
		httpStatusCreated,
		responseBody,
	)
	if err != nil {
		if isUniqueViolation(err) {
			rec, found, getErr := r.getTransactionByIdempotencyKey(ctx, req.UserID, idempotencyKey)
			if getErr != nil {
				return Transaction{}, false, getErr
			}
			if found {
				if rec.RequestHash != requestHash {
					return Transaction{}, false, ConflictError{Message: "Idempotency-Key is already used with different payload"}
				}

				tx, err := decodeStoredTransaction(rec.ResponseBody)
				if err != nil {
					return Transaction{}, false, err
				}

				return tx, true, nil
			}
		}

		return Transaction{}, false, err
	}

	if err := tx.Commit(ctx); err != nil {
		return Transaction{}, false, err
	}

	return created, false, nil
}

func (r *Repository) createTransactionTx(ctx context.Context, tx pgx.Tx, req CreateTransactionRequest) (Transaction, error) {
	var result Transaction
	var categoryID pgtype.Int8
	var merchant pgtype.Text

	query := `
		INSERT INTO transactions (
			user_id,
			type,
			amount,
			currency,
			category_id,
			merchant,
			occurred_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING
			id,
			user_id,
			type,
			amount::text,
			currency,
			category_id,
			merchant,
			occurred_at,
			created_at
	`

	var categoryValue any
	if req.CategoryID != nil {
		categoryValue = *req.CategoryID
	}

	var merchantValue any
	if req.Merchant != nil {
		trimmed := strings.TrimSpace(*req.Merchant)
		if trimmed != "" {
			merchantValue = trimmed
		}
	}

	err := tx.QueryRow(
		ctx,
		query,
		req.UserID,
		req.Type,
		req.Amount,
		strings.ToUpper(req.Currency),
		categoryValue,
		merchantValue,
		req.OccurredAt,
	).Scan(
		&result.ID,
		&result.UserID,
		&result.Type,
		&result.Amount,
		&result.Currency,
		&categoryID,
		&merchant,
		&result.OccurredAt,
		&result.CreatedAt,
	)
	if err != nil {
		if isForeignKeyViolation(err) {
			return Transaction{}, ValidationError{Message: "invalid category_id"}
		}
		return Transaction{}, err
	}

	if categoryID.Valid {
		value := categoryID.Int64
		result.CategoryID = &value
	}

	if merchant.Valid {
		value := merchant.String
		result.Merchant = &value
	}

	return result, nil
}

func (r *Repository) insertOutboxEventTx(ctx context.Context, tx pgx.Tx, event outbox.Event) error {
	_, err := tx.Exec(
		ctx,
		`
		INSERT INTO outbox_events (
			event_id,
			event_type,
			payload,
			status
		)
		VALUES ($1, $2, $3, $4)
		`,
		event.EventID,
		event.EventType,
		event.Payload,
		event.Status,
	)

	return err
}

func (r *Repository) saveIdempotencyKeyTx(ctx context.Context, tx pgx.Tx, userID int64, idempotencyKey, requestHash string, responseStatusCode int, responseBody []byte) error {
	_, err := tx.Exec(
		ctx,
		`
		INSERT INTO idempotency_keys (
			user_id,
			idempotency_key,
			request_hash,
			response_status_code,
			response_body
		)
		VALUES ($1, $2, $3, $4, $5)
		`,
		userID,
		idempotencyKey,
		requestHash,
		responseStatusCode,
		responseBody,
	)

	return err
}

func (r *Repository) getTransactionByIdempotencyKey(ctx context.Context, userID int64, idempotencyKey string) (idempotencyRecord, bool, error) {
	var rec idempotencyRecord

	err := r.db.QueryRow(
		ctx,
		`
		SELECT request_hash, response_body
		FROM idempotency_keys
		WHERE user_id = $1 AND idempotency_key = $2
		`,
		userID,
		idempotencyKey,
	).Scan(&rec.RequestHash, &rec.ResponseBody)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return idempotencyRecord{}, false, nil
		}
		return idempotencyRecord{}, false, err
	}

	return rec, true, nil
}

func (r *Repository) ListTransactions(ctx context.Context, userID int64) ([]Transaction, error) {
	query := `
		SELECT
			id,
			user_id,
			type,
			amount::text,
			currency,
			category_id,
			merchant,
			occurred_at,
			created_at
		FROM transactions
		WHERE user_id = $1
		ORDER BY id DESC
		LIMIT 100
	`

	rows, err := r.db.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]Transaction, 0)
	for rows.Next() {
		var tx Transaction
		var categoryID pgtype.Int8
		var merchant pgtype.Text

		err := rows.Scan(
			&tx.ID,
			&tx.UserID,
			&tx.Type,
			&tx.Amount,
			&tx.Currency,
			&categoryID,
			&merchant,
			&tx.OccurredAt,
			&tx.CreatedAt,
		)
		if err != nil {
			return nil, err
		}

		if categoryID.Valid {
			value := categoryID.Int64
			tx.CategoryID = &value
		}

		if merchant.Valid {
			value := merchant.String
			tx.Merchant = &value
		}

		result = append(result, tx)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

func (r *Repository) CreateCategory(ctx context.Context, req CreateCategoryRequest) (Category, error) {
	var category Category

	query := `
		INSERT INTO categories (user_id, name)
		VALUES ($1, $2)
		RETURNING id, user_id, name, created_at
	`

	err := r.db.QueryRow(
		ctx,
		query,
		req.UserID,
		strings.TrimSpace(req.Name),
	).Scan(
		&category.ID,
		&category.UserID,
		&category.Name,
		&category.CreatedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return Category{}, ConflictError{Message: "category already exists"}
		}
		return Category{}, err
	}

	return category, nil
}

func (r *Repository) ListCategories(ctx context.Context, userID int64) ([]Category, error) {
	query := `
		SELECT
			id,
			user_id,
			name,
			created_at
		FROM categories
		WHERE user_id = $1
		ORDER BY id DESC
	`

	rows, err := r.db.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]Category, 0)
	for rows.Next() {
		var category Category

		err := rows.Scan(
			&category.ID,
			&category.UserID,
			&category.Name,
			&category.CreatedAt,
		)
		if err != nil {
			return nil, err
		}

		result = append(result, category)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

func (r *Repository) categoryExistsForUserTx(ctx context.Context, tx pgx.Tx, userID, categoryID int64) (bool, error) {
	var exists bool

	err := tx.QueryRow(
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

func isForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503"
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func decodeStoredTransaction(body []byte) (Transaction, error) {
	var tx Transaction
	if err := json.Unmarshal(body, &tx); err != nil {
		return Transaction{}, err
	}
	return tx, nil
}

const httpStatusCreated = 201
