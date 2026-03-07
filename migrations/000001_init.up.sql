CREATE TABLE categories (
                            id BIGSERIAL PRIMARY KEY,
                            user_id BIGINT NOT NULL,
                            name TEXT NOT NULL,
                            created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
                            UNIQUE (user_id, name)
);

CREATE TABLE transactions (
                              id BIGSERIAL PRIMARY KEY,
                              user_id BIGINT NOT NULL,
                              type TEXT NOT NULL CHECK (type IN ('income', 'expense', 'transfer')),
                              amount NUMERIC(14,2) NOT NULL CHECK (amount >= 0),
                              currency CHAR(3) NOT NULL,
                              category_id BIGINT REFERENCES categories(id) ON DELETE SET NULL,
                              merchant TEXT,
                              occurred_at TIMESTAMPTZ NOT NULL,
                              created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_transactions_user_id ON transactions(user_id);
CREATE INDEX idx_transactions_category_id ON transactions(category_id);
CREATE INDEX idx_transactions_occurred_at ON transactions(occurred_at);

CREATE TABLE idempotency_keys (
                                  id BIGSERIAL PRIMARY KEY,
                                  user_id BIGINT NOT NULL,
                                  idempotency_key TEXT NOT NULL,
                                  request_hash TEXT,
                                  response_status_code INT NOT NULL,
                                  response_body JSONB NOT NULL,
                                  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
                                  UNIQUE (user_id, idempotency_key)
);

CREATE TABLE outbox_events (
                               event_id UUID PRIMARY KEY,
                               event_type TEXT NOT NULL,
                               payload JSONB NOT NULL,
                               status TEXT NOT NULL CHECK (status IN ('pending', 'sent', 'failed')),
                               created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_outbox_events_status_created_at ON outbox_events(status, created_at);

CREATE TABLE budgets (
                         id BIGSERIAL PRIMARY KEY,
                         user_id BIGINT NOT NULL,
                         category_id BIGINT NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
                         period_type TEXT NOT NULL CHECK (period_type IN ('month', 'week')),
                         period_start DATE NOT NULL,
                         limit_amount NUMERIC(14,2) NOT NULL CHECK (limit_amount >= 0),
                         created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
                         UNIQUE (user_id, category_id, period_type, period_start)
);

CREATE TABLE budget_stats (
                              id BIGSERIAL PRIMARY KEY,
                              user_id BIGINT NOT NULL,
                              category_id BIGINT NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
                              period_type TEXT NOT NULL CHECK (period_type IN ('month', 'week')),
                              period_start DATE NOT NULL,
                              spent_amount NUMERIC(14,2) NOT NULL DEFAULT 0 CHECK (spent_amount >= 0),
                              remaining_amount NUMERIC(14,2) NOT NULL DEFAULT 0,
                              updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
                              UNIQUE (user_id, category_id, period_type, period_start)
);

CREATE TABLE processed_events (
                                  event_id UUID PRIMARY KEY,
                                  processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE notifications (
                               id BIGSERIAL PRIMARY KEY,
                               user_id BIGINT NOT NULL,
                               type TEXT NOT NULL,
                               payload JSONB NOT NULL,
                               created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_notifications_user_id_created_at ON notifications(user_id, created_at);