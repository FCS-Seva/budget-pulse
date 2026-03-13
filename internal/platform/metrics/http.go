package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type HTTPMiddleware struct {
	requestsTotal      *prometheus.CounterVec
	errorsTotal        *prometheus.CounterVec
	requestDurationSec *prometheus.HistogramVec
}

func NewHTTPMiddleware(reg prometheus.Registerer) *HTTPMiddleware {
	m := &HTTPMiddleware{
		requestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "requests_total",
				Help: "Total number of HTTP requests",
			},
			[]string{"method", "path", "status"},
		),
		errorsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "errors_total",
				Help: "Total number of HTTP error responses",
			},
			[]string{"method", "path", "status"},
		),
		requestDurationSec: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "request_duration_seconds",
				Help:    "HTTP request duration in seconds",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"method", "path", "status"},
		),
	}

	reg.MustRegister(m.requestsTotal, m.errorsTotal, m.requestDurationSec)

	return m
}

func (m *HTTPMiddleware) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		rec := &statusRecorder{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		next.ServeHTTP(rec, r)

		status := strconv.Itoa(rec.statusCode)
		path := r.URL.Path
		method := r.Method
		duration := time.Since(start).Seconds()

		m.requestsTotal.WithLabelValues(method, path, status).Inc()
		if rec.statusCode >= 400 {
			m.errorsTotal.WithLabelValues(method, path, status).Inc()
		}
		m.requestDurationSec.WithLabelValues(method, path, status).Observe(duration)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (r *statusRecorder) WriteHeader(statusCode int) {
	r.statusCode = statusCode
	r.ResponseWriter.WriteHeader(statusCode)
}
