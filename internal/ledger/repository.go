package ledger

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) CreateTransaction(ctx context.Context, req CreateTransactionRequest) (Transaction, error) {
	var tx Transaction
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

	err := r.db.QueryRow(
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
		return Transaction{}, err
	}

	if categoryID.Valid {
		value := categoryID.Int64
		tx.CategoryID = &value
	}

	if merchant.Valid {
		value := merchant.String
		tx.Merchant = &value
	}

	return tx, nil
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
