package budget

import (
	"strconv"
	"time"
)

type Budget struct {
	ID          int64     `json:"id"`
	UserID      int64     `json:"user_id"`
	CategoryID  int64     `json:"category_id"`
	PeriodType  string    `json:"period_type"`
	PeriodStart time.Time `json:"period_start"`
	LimitAmount string    `json:"limit_amount"`
	CreatedAt   time.Time `json:"created_at"`
}

type CreateBudgetRequest struct {
	UserID      int64     `json:"user_id"`
	CategoryID  int64     `json:"category_id"`
	PeriodStart time.Time `json:"period_start"`
	LimitAmount string    `json:"limit_amount"`
}

type BudgetStatus struct {
	LimitAmount     string `json:"limit_amount"`
	SpentAmount     string `json:"spent_amount"`
	RemainingAmount string `json:"remaining_amount"`
	Exceeded        bool   `json:"exceeded"`
}

type ValidationError struct {
	Message string
}

func (e ValidationError) Error() string {
	return e.Message
}

type ConflictError struct {
	Message string
}

func (e ConflictError) Error() string {
	return e.Message
}

type NotFoundError struct {
	Message string
}

func (e NotFoundError) Error() string {
	return e.Message
}

func validateCreateBudgetRequest(req CreateBudgetRequest) error {
	if req.UserID <= 0 {
		return ValidationError{Message: "user_id must be greater than 0"}
	}

	if req.CategoryID <= 0 {
		return ValidationError{Message: "category_id must be greater than 0"}
	}

	if req.PeriodStart.IsZero() {
		return ValidationError{Message: "period_start is required"}
	}

	amount, err := strconv.ParseFloat(req.LimitAmount, 64)
	if err != nil {
		return ValidationError{Message: "limit_amount must be a valid number"}
	}

	if amount <= 0 {
		return ValidationError{Message: "limit_amount must be greater than 0"}
	}

	return nil
}
