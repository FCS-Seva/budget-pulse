package budget

import (
	"encoding/json"
	"time"
)

const NotificationTypeBudgetExceeded = "budget.exceeded"

type BudgetExceededNotificationPayload struct {
	UserID          int64     `json:"user_id"`
	CategoryID      int64     `json:"category_id"`
	PeriodType      string    `json:"period_type"`
	PeriodStart     time.Time `json:"period_start"`
	LimitAmount     string    `json:"limit_amount"`
	SpentAmount     string    `json:"spent_amount"`
	RemainingAmount string    `json:"remaining_amount"`
	EventID         string    `json:"event_id"`
}

func newBudgetExceededNotificationPayload(
	event TransactionCreatedEvent,
	periodStart time.Time,
	limitAmount string,
	spentAmount string,
	remainingAmount string,
) ([]byte, error) {
	payload := BudgetExceededNotificationPayload{
		UserID:          event.UserID,
		CategoryID:      *event.CategoryID,
		PeriodType:      "month",
		PeriodStart:     periodStart,
		LimitAmount:     limitAmount,
		SpentAmount:     spentAmount,
		RemainingAmount: remainingAmount,
		EventID:         event.EventID,
	}

	return json.Marshal(payload)
}
