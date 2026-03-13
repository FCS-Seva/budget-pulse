package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"budgetpulse/internal/budget"
	"budgetpulse/internal/config"
	"budgetpulse/internal/ledger"
	"budgetpulse/internal/platform/logging"
	"budgetpulse/internal/platform/metrics"
	natsclient "budgetpulse/internal/platform/nats"
	"budgetpulse/internal/platform/postgres"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg := config.Load()
	logger := logging.NewLogger()

	db, err := postgres.NewPool(ctx, cfg.PostgresURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	nc, err := natsclient.New(cfg.NATSURL)
	if err != nil {
		log.Fatal(err)
	}
	defer nc.Close()

	ledgerRepo := ledger.NewRepository(db)
	ledgerService := ledger.NewService(ledgerRepo)
	ledgerHandler := ledger.NewHandler(ledgerService)

	budgetRepo := budget.NewRepository(db)
	budgetService := budget.NewService(budgetRepo)
	budgetHandler := budget.NewHandler(budgetService)

	reg := prometheus.NewRegistry()
	reg.MustRegister(prometheus.NewGoCollector(), prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
	httpMetrics := metrics.NewHTTPMiddleware(reg)

	appMux := http.NewServeMux()

	appMux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		pingCtx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		if err := db.Ping(pingCtx); err != nil {
			http.Error(w, "postgres unavailable", http.StatusServiceUnavailable)
			return
		}

		if !nc.Conn.IsConnected() {
			http.Error(w, "nats unavailable", http.StatusServiceUnavailable)
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	ledgerHandler.RegisterRoutes(appMux)
	budgetHandler.RegisterRoutes(appMux)
	appMux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))

	handler := httpMetrics.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		appMux.ServeHTTP(w, r)
		logger.Info(
			"http request completed",
			"method", r.Method,
			"path", r.URL.Path,
			"query", r.URL.RawQuery,
			"remote_addr", r.RemoteAddr,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	}))

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		<-ctx.Done()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_ = server.Shutdown(shutdownCtx)
	}()

	logger.Info("api started", "addr", cfg.HTTPAddr)

	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}

	logger.Info("api stopped")
}
