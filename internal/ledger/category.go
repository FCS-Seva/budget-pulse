package ledger

import (
	"strings"
	"time"
)

type Category struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type CreateCategoryRequest struct {
	UserID int64  `json:"user_id"`
	Name   string `json:"name"`
}

func validateCreateCategoryRequest(req CreateCategoryRequest) error {
	if req.UserID <= 0 {
		return ValidationError{Message: "user_id must be greater than 0"}
	}

	if strings.TrimSpace(req.Name) == "" {
		return ValidationError{Message: "name is required"}
	}

	return nil
}
