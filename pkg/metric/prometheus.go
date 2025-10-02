package metric

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var _ Factory = (*prometheusFactory)(nil)

type prometheusFactory struct {
	registry    *promRegistry
	http        *httpMetrics
	transaction *transactionMetrics
	cache       *cacheMetrics
	kafka       *kafkaMetrics
	dlq         *dlqMetrics
}

func NewFactory() Factory {
	registry := newPromRegistry()

	return &prometheusFactory{
		registry:    registry,
		http:        newHTTPMetrics(registry),
		transaction: newTransactionMetrics(registry),
		cache:       newCacheMetrics(registry),
		kafka:       newKafkaMetrics(registry),
		dlq:         newDLQMetrics(registry),
	}
}

func (f *prometheusFactory) HTTP() HTTP {
	return f.http
}

func (f *prometheusFactory) Transaction() Transaction {
	return f.transaction
}

func (f *prometheusFactory) Cache() Cache {
	return f.cache
}

func (f *prometheusFactory) Kafka() Kafka {
	return f.kafka
}

func (f *prometheusFactory) DLQ() DLQ {
	return f.dlq
}

func (f *prometheusFactory) Handler() http.Handler {
	return promhttp.HandlerFor(f.registry.registry,
		promhttp.HandlerOpts{
			EnableOpenMetrics: true,
		})
}

type promRegistry struct {
	registry *prometheus.Registry
}

func newPromRegistry() *promRegistry {
	reg := prometheus.NewRegistry()

	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	return &promRegistry{registry: reg}
}
