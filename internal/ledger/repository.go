package ledger

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) CreateTransaction(ctx context.Context, req CreateTransactionRequest, idempotencyKey string) (Transaction, error) {
	existing, found, err := r.getTransactionByIdempotencyKey(ctx, req.UserID, idempotencyKey)
	if err != nil {
		return Transaction{}, err
	}
	if found {
		return existing, nil
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Transaction{}, err
	}
	defer tx.Rollback(ctx)

	created, err := r.createTransactionTx(ctx, tx, req)
	if err != nil {
		return Transaction{}, err
	}

	responseBody, err := json.Marshal(created)
	if err != nil {
		return Transaction{}, err
	}

	_, err = tx.Exec(
		ctx,
		`
		INSERT INTO idempotency_keys (
			user_id,
			idempotency_key,
			response_status_code,
			response_body
		)
		VALUES ($1, $2, $3, $4)
		`,
		req.UserID,
		idempotencyKey,
		httpStatusCreated,
		responseBody,
	)
	if err != nil {
		if isUniqueViolation(err) {
			existing, found, getErr := r.getTransactionByIdempotencyKey(ctx, req.UserID, idempotencyKey)
			if getErr != nil {
				return Transaction{}, getErr
			}
			if found {
				return existing, nil
			}
		}

		return Transaction{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return Transaction{}, err
	}

	return created, nil
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

func (r *Repository) getTransactionByIdempotencyKey(ctx context.Context, userID int64, idempotencyKey string) (Transaction, bool, error) {
	var responseBody []byte

	err := r.db.QueryRow(
		ctx,
		`
		SELECT response_body
		FROM idempotency_keys
		WHERE user_id = $1 AND idempotency_key = $2
		`,
		userID,
		idempotencyKey,
	).Scan(&responseBody)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Transaction{}, false, nil
		}
		return Transaction{}, false, err
	}

	var tx Transaction
	if err := json.Unmarshal(responseBody, &tx); err != nil {
		return Transaction{}, false, err
	}

	return tx, true, nil
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

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

const httpStatusCreated = 201
