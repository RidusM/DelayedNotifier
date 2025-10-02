package metric

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	HTTPStatusBadRequest          = 400
	HTTPStatusInternalServerError = 500
)

var _ HTTP = (*httpMetrics)(nil)

type httpMetrics struct {
	requestCounter     *prometheus.CounterVec
	slowRequestCounter *prometheus.CounterVec
	durationHistogram  *prometheus.HistogramVec
}

func newHTTPMetrics(registry *promRegistry) *httpMetrics {
	counter := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests by method, path and status",
		},
		[]string{"method", "path", "status"},
	)

	slowCounter := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_slow_requests_total",
			Help: "Total number of slow HTTP requests by method, path and status",
		},
		[]string{"method", "path", "status"},
	)

	duration := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request duration in seconds",
			Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0},
		},
		[]string{"method", "path", "status"},
	)

	registry.registry.MustRegister(counter, slowCounter, duration)

	return &httpMetrics{
		requestCounter:     counter,
		slowRequestCounter: slowCounter,
		durationHistogram:  duration,
	}
}

func (m *httpMetrics) Request(
	method, path string,
	status int,
	duration time.Duration,
) {
	statusClass := "2xx"
	if status >= HTTPStatusBadRequest {
		statusClass = "4xx"
	} else if status >= HTTPStatusInternalServerError {
		statusClass = "5xx"
	}
	m.requestCounter.WithLabelValues(method, path, statusClass).Add(1)
	m.durationHistogram.WithLabelValues(method, path, statusClass).Observe(duration.Seconds())
}

func (m *httpMetrics) SlowRequest(
	method, path string,
	status int,
	duration time.Duration,
) {
	statusClass := "2xx"
	if status >= HTTPStatusBadRequest {
		statusClass = "4xx"
	} else if status >= HTTPStatusInternalServerError {
		statusClass = "5xx"
	}

	m.slowRequestCounter.WithLabelValues(method, path, statusClass).Add(1)
	m.durationHistogram.WithLabelValues(method, path, statusClass).Observe(duration.Seconds())
}
