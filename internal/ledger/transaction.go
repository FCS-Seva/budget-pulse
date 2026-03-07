package ledger

import (
	"strconv"
	"strings"
	"time"
)

type Transaction struct {
	ID         int64     `json:"id"`
	UserID     int64     `json:"user_id"`
	Type       string    `json:"type"`
	Amount     string    `json:"amount"`
	Currency   string    `json:"currency"`
	CategoryID *int64    `json:"category_id,omitempty"`
	Merchant   *string   `json:"merchant,omitempty"`
	OccurredAt time.Time `json:"occurred_at"`
	CreatedAt  time.Time `json:"created_at"`
}

type CreateTransactionRequest struct {
	UserID     int64     `json:"user_id"`
	Type       string    `json:"type"`
	Amount     string    `json:"amount"`
	Currency   string    `json:"currency"`
	CategoryID *int64    `json:"category_id,omitempty"`
	Merchant   *string   `json:"merchant,omitempty"`
	OccurredAt time.Time `json:"occurred_at"`
}

type ValidationError struct {
	Message string
}

func (e ValidationError) Error() string {
	return e.Message
}

func validateCreateTransactionRequest(req CreateTransactionRequest) error {
	if req.UserID <= 0 {
		return ValidationError{Message: "user_id must be greater than 0"}
	}

	if req.Type != "income" && req.Type != "expense" && req.Type != "transfer" {
		return ValidationError{Message: "type must be one of: income, expense, transfer"}
	}

	amount, err := strconv.ParseFloat(req.Amount, 64)
	if err != nil {
		return ValidationError{Message: "amount must be a valid number"}
	}

	if amount <= 0 {
		return ValidationError{Message: "amount must be greater than 0"}
	}

	req.Currency = strings.TrimSpace(req.Currency)
	if len(req.Currency) != 3 {
		return ValidationError{Message: "currency must be 3 letters"}
	}

	if req.OccurredAt.IsZero() {
		return ValidationError{Message: "occurred_at is required"}
	}

	if req.CategoryID != nil && *req.CategoryID <= 0 {
		return ValidationError{Message: "category_id must be greater than 0"}
	}

	return nil
}
