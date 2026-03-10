package ledger

import (
	"encoding/json"
	"time"

	"budgetpulse/internal/outbox"

	"github.com/google/uuid"
)

const EventTypeTransactionCreated = "transaction.created"

type TransactionCreatedEventPayload struct {
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

func newTransactionCreatedEvent(tx Transaction) (outbox.Event, error) {
	eventID := uuid.NewString()

	payload := TransactionCreatedEventPayload{
		EventID:       eventID,
		TransactionID: tx.ID,
		UserID:        tx.UserID,
		Type:          tx.Type,
		Amount:        tx.Amount,
		Currency:      tx.Currency,
		CategoryID:    tx.CategoryID,
		Merchant:      tx.Merchant,
		OccurredAt:    tx.OccurredAt,
		CreatedAt:     tx.CreatedAt,
	}

	rawPayload, err := json.Marshal(payload)
	if err != nil {
		return outbox.Event{}, err
	}

	return outbox.Event{
		EventID:   eventID,
		EventType: EventTypeTransactionCreated,
		Payload:   rawPayload,
		Status:    outbox.StatusPending,
	}, nil
}
