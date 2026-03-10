package outbox

type Event struct {
	EventID   string
	EventType string
	Payload   []byte
	Status    string
}

const StatusPending = "pending"
