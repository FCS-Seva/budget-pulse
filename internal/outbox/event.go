package outbox

type Event struct {
	EventID   string
	EventType string
	Payload   []byte
	Status    string
}

const (
	StatusPending             = "pending"
	StatusSent                = "sent"
	StreamName                = "events"
	SubjectTransactionCreated = "transaction.created"
)
