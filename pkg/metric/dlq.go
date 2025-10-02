package metric

import (
	"github.com/prometheus/client_golang/prometheus"
)

var _ DLQ = (*dlqMetrics)(nil)

type dlqMetrics struct {
	messagesSent *prometheus.CounterVec
	retryCount   *prometheus.HistogramVec
	errors       *prometheus.CounterVec
}

func newDLQMetrics(registry *promRegistry) *dlqMetrics {
	sent := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "dlq_messages_sent_total",
			Help: "Total number of messages sent to the Dead Letter Queue",
		},
		[]string{"dlq_topic", "original_topic"},
	)

	retryHist := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "dlq_retry_count",
			Help:    "Distribution of retry counts before message was sent to DLQ",
			Buckets: []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 15, 20},
		},
		[]string{"original_topic"},
	)

	errors := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "dlq_errors_total",
			Help: "Total number of errors related to DLQ operations (e.g. send failure)",
		},
		[]string{"dlq_topic", "reason"},
	)

	registry.registry.MustRegister(sent, retryHist, errors)

	return &dlqMetrics{
		messagesSent: sent,
		retryCount:   retryHist,
		errors:       errors,
	}
}

func (m *dlqMetrics) DLSent(dlqTopic string, originalTopic string, retryCount int) {
	m.messagesSent.WithLabelValues(dlqTopic, originalTopic).Add(1)
	m.DLRetryCount(originalTopic, retryCount)
}

func (m *dlqMetrics) DLRetryCount(originalTopic string, retryCount int) {
	m.retryCount.WithLabelValues(originalTopic).Observe(float64(retryCount))
}

func (m *dlqMetrics) DLError(dlqTopic string, reason string) {
	m.errors.WithLabelValues(dlqTopic, reason).Add(1)
}
