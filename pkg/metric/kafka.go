package metric

import (
	"github.com/prometheus/client_golang/prometheus"
)

var _ Kafka = (*kafkaMetrics)(nil)

type kafkaMetrics struct {
	messagesProcessed *prometheus.CounterVec
	messagesFailed    *prometheus.CounterVec
	consumerGroupLag  *prometheus.GaugeVec
}

func newKafkaMetrics(registry *promRegistry) *kafkaMetrics {
	processed := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kafka_messages_processed_total",
			Help: "Total number of processed Kafka messages",
		},
		[]string{"topic", "partition"},
	)

	failed := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kafka_messages_failed_total",
			Help: "Total number of failed Kafka messages",
		},
		[]string{"topic", "partition", "reason"},
	)

	lag := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "kafka_consumer_group_lag",
			Help: "Consumer group lag for Kafka partitions",
		},
		[]string{"topic", "partition"},
	)

	registry.registry.MustRegister(processed, failed, lag)

	return &kafkaMetrics{
		messagesProcessed: processed,
		messagesFailed:    failed,
		consumerGroupLag:  lag,
	}
}

func (m *kafkaMetrics) MessageProcessed(topic string, partition int) {
	m.messagesProcessed.WithLabelValues(topic, partitionString(partition)).Add(1)
}

func (m *kafkaMetrics) MessageFailed(topic string, partition int, reason string) {
	m.messagesFailed.WithLabelValues(topic, partitionString(partition), reason).Add(1)
}

func (m *kafkaMetrics) ConsumerGroupLag(topic string, partition int, lag int64) {
	m.consumerGroupLag.WithLabelValues(topic, partitionString(partition)).Set(float64(lag))
}

func partitionString(partition int) string {
	if partition == -1 {
		return "all"
	}
	return string(rune(partition + '0'))
}
