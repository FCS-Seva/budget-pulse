package budgetpulse_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"budgetpulse/internal/budget"
	"budgetpulse/internal/ledger"
	"budgetpulse/internal/outbox"
	natsclient "budgetpulse/internal/platform/nats"
	"budgetpulse/internal/platform/postgres"

	"github.com/golang-migrate/migrate/v4"
	postgresmigrate "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/nats-io/nats.go/jetstream"
	tc "github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestMVPFlow_CreateBudget_CreateExpense_UpdatesBudgetStats(t *testing.T) {
	ctx := context.Background()

	postgresURL := startPostgresContainer(t, ctx)
	natsURL := startNATSContainer(t, ctx)

	applyMigrations(t, postgresURL)

	db, err := postgres.NewPool(ctx, postgresURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	nc, err := natsclient.New(natsURL)
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()

	_, err = nc.JetStream.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:     outbox.StreamName,
		Subjects: []string{outbox.SubjectTransactionCreated},
	})
	if err != nil {
		t.Fatal(err)
	}

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	workerCtx, workerCancel := context.WithCancel(ctx)
	defer workerCancel()

	outboxRepo := outbox.NewRepository(db)
	publisher := outbox.NewPublisher(outboxRepo, nc.JetStream, logger, 50*time.Millisecond)

	budgetRepo := budget.NewRepository(db)
	budgetConsumer := budget.NewConsumer(budgetRepo, nc.JetStream, logger)

	publisherErrCh := make(chan error, 1)
	consumerErrCh := make(chan error, 1)

	go func() {
		publisherErrCh <- publisher.Run(workerCtx)
	}()

	go func() {
		consumerErrCh <- budgetConsumer.Run(workerCtx)
	}()

	server := newTestAPIServer(db)
	defer server.Close()

	categoryID := insertCategory(t, ctx, db, 1, "Food")

	createBudget(t, server.URL, categoryID)
	createExpenseTransaction(t, server.URL, categoryID)

	assertOutboxEventCreated(t, ctx, db)

	waitForBudgetStats(t, ctx, db, 1, categoryID, time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), "250.00", "750.00")

	select {
	case err := <-publisherErrCh:
		if err != nil {
			t.Fatal(err)
		}
	default:
	}

	select {
	case err := <-consumerErrCh:
		if err != nil {
			t.Fatal(err)
		}
	default:
	}
}

func newTestAPIServer(db *pgxpool.Pool) *httptest.Server {
	ledgerRepo := ledger.NewRepository(db)
	ledgerService := ledger.NewService(ledgerRepo)
	ledgerHandler := ledger.NewHandler(ledgerService)

	budgetRepo := budget.NewRepository(db)
	budgetService := budget.NewService(budgetRepo)
	budgetHandler := budget.NewHandler(budgetService)

	mux := http.NewServeMux()
	ledgerHandler.RegisterRoutes(mux)
	budgetHandler.RegisterRoutes(mux)

	return httptest.NewServer(mux)
}

func createBudget(t *testing.T, baseURL string, categoryID int64) {
	body := fmt.Sprintf(`{
		"user_id": 1,
		"category_id": %d,
		"period_start": "2026-03-15T10:00:00Z",
		"limit_amount": "1000.00"
	}`, categoryID)

	resp := doJSONRequest(t, http.MethodPost, baseURL+"/budgets", body, nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", resp.StatusCode)
	}
}

func createExpenseTransaction(t *testing.T, baseURL string, categoryID int64) {
	body := fmt.Sprintf(`{
		"user_id": 1,
		"type": "expense",
		"amount": "250.00",
		"currency": "USD",
		"category_id": %d,
		"merchant": "Starbucks",
		"occurred_at": "2026-03-10T12:00:00Z"
	}`, categoryID)

	resp := doJSONRequest(t, http.MethodPost, baseURL+"/transactions", body, map[string]string{
		"Idempotency-Key": "tx-int-0001",
	})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", resp.StatusCode)
	}
}

func doJSONRequest(t *testing.T, method, url, body string, headers map[string]string) *http.Response {
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}

	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}

	return resp
}

func insertCategory(t *testing.T, ctx context.Context, db *pgxpool.Pool, userID int64, name string) int64 {
	var categoryID int64

	err := db.QueryRow(
		ctx,
		`
		INSERT INTO categories (user_id, name)
		VALUES ($1, $2)
		RETURNING id
		`,
		userID,
		name,
	).Scan(&categoryID)
	if err != nil {
		t.Fatal(err)
	}

	return categoryID
}

func assertOutboxEventCreated(t *testing.T, ctx context.Context, db *pgxpool.Pool) {
	var count int

	err := db.QueryRow(
		ctx,
		`
		SELECT COUNT(*)
		FROM outbox_events
		WHERE event_type = 'transaction.created'
		`,
	).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}

	if count != 1 {
		t.Fatalf("expected 1 outbox event, got %d", count)
	}
}

func waitForBudgetStats(
	t *testing.T,
	ctx context.Context,
	db *pgxpool.Pool,
	userID int64,
	categoryID int64,
	periodStart time.Time,
	wantSpent string,
	wantRemaining string,
) {
	deadline := time.Now().Add(10 * time.Second)

	for time.Now().Before(deadline) {
		var spent string
		var remaining string

		err := db.QueryRow(
			ctx,
			`
			SELECT spent_amount::text, remaining_amount::text
			FROM budget_stats
			WHERE user_id = $1
			  AND category_id = $2
			  AND period_type = 'month'
			  AND period_start = $3
			`,
			userID,
			categoryID,
			periodStart,
		).Scan(&spent, &remaining)

		if err == nil && spent == wantSpent && remaining == wantRemaining {
			return
		}

		time.Sleep(100 * time.Millisecond)
	}

	t.Fatalf("budget_stats was not updated to spent=%s remaining=%s", wantSpent, wantRemaining)
}

func applyMigrations(t *testing.T, postgresURL string) {
	sqlDB, err := sql.Open("pgx", postgresURL)
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()

	driver, err := postgresmigrate.WithInstance(sqlDB, &postgresmigrate.Config{})
	if err != nil {
		t.Fatal(err)
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	sourceURL := "file://" + filepath.Join(wd, "migrations")

	m, err := migrate.NewWithDatabaseInstance(sourceURL, "postgres", driver)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	err = m.Up()
	if err != nil && !errors.Is(err, migrate.ErrNoChange) {
		t.Fatal(err)
	}
}

func startPostgresContainer(t *testing.T, ctx context.Context) string {
	container, err := tc.GenericContainer(ctx, tc.GenericContainerRequest{
		Started: true,
		ContainerRequest: tc.ContainerRequest{
			Image:        "postgres:16-alpine",
			ExposedPorts: []string{"5432/tcp"},
			Env: map[string]string{
				"POSTGRES_DB":       "budgetpulse",
				"POSTGRES_USER":     "budgetpulse",
				"POSTGRES_PASSWORD": "budgetpulse",
			},
			WaitingFor: wait.ForListeningPort("5432/tcp").WithStartupTimeout(30 * time.Second),
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		_ = container.Terminate(context.Background())
	})

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatal(err)
	}

	port, err := container.MappedPort(ctx, "5432/tcp")
	if err != nil {
		t.Fatal(err)
	}

	postgresURL := fmt.Sprintf(
		"postgres://budgetpulse:budgetpulse@%s:%s/budgetpulse?sslmode=disable",
		host,
		port.Port(),
	)

	waitForPostgres(t, ctx, postgresURL)

	return postgresURL
}

func waitForPostgres(t *testing.T, ctx context.Context, postgresURL string) {
	deadline := time.Now().Add(30 * time.Second)

	for time.Now().Before(deadline) {
		db, err := postgres.NewPool(ctx, postgresURL)
		if err == nil {
			db.Close()
			return
		}

		time.Sleep(200 * time.Millisecond)
	}

	t.Fatal("postgres did not become ready in time")
}

func startNATSContainer(t *testing.T, ctx context.Context) string {
	container, err := tc.GenericContainer(ctx, tc.GenericContainerRequest{
		Started: true,
		ContainerRequest: tc.ContainerRequest{
			Image:        "nats:2.10-alpine",
			ExposedPorts: []string{"4222/tcp"},
			Cmd:          []string{"-js", "-sd", "/data", "-m", "8222"},
			WaitingFor:   wait.ForListeningPort("4222/tcp").WithStartupTimeout(30 * time.Second),
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		_ = container.Terminate(context.Background())
	})

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatal(err)
	}

	port, err := container.MappedPort(ctx, "4222/tcp")
	if err != nil {
		t.Fatal(err)
	}

	natsURL := fmt.Sprintf("nats://%s:%s", host, port.Port())

	waitForNATS(t, natsURL)

	return natsURL
}

func waitForNATS(t *testing.T, natsURL string) {
	deadline := time.Now().Add(30 * time.Second)

	for time.Now().Before(deadline) {
		nc, err := natsclient.New(natsURL)
		if err == nil {
			nc.Close()
			return
		}

		time.Sleep(200 * time.Millisecond)
	}

	t.Fatal("nats did not become ready in time")
}
