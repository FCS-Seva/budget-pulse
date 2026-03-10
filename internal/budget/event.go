package budget

import "time"

type TransactionCreatedEvent struct {
	EventID       string    `json:"event_id"`
	TransactionID int64     `json:"transaction_id"`
	UserID        int64     `json:"user_id"`
	Type          string    `json:"type"`
	Amount        string    `json:"amount"`
	Currency      string    `json:"currency"`
	CategoryID    *int64    `json:"category_id,omitempty"`
	Merchant      *string   `json:"merchant,omitempty"`
	OccurredAt    time.Time `json:"occurred_at"`
	CreatedAt     time.Time `json:"created_at"`
}
