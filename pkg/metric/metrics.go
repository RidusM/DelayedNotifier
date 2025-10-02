package metric

import (
	"net/http"
	"time"
)

type (
	Factory interface {
		HTTP() HTTP
		Transaction() Transaction
		Cache() Cache
		Kafka() Kafka
		DLQ() DLQ
		Handler() http.Handler
	}

	HTTP interface {
		Request(method, path string, status int, duration time.Duration)
		SlowRequest(method, path string, status int, duration time.Duration)
	}

	Transaction interface {
		ObserveDuration(operation string, duration time.Duration)
		IncrementRetries(operation string)
		IncrementFailures(operation string)
	}

	Cache interface {
		Hit(cacheType string)
		Miss(cacheType string)
		Eviction(cacheType string, reason string)
		Size(cacheType string, size int)
	}

	Kafka interface {
		MessageProcessed(topic string, partition int)
		MessageFailed(topic string, partition int, reason string)
		ConsumerGroupLag(topic string, partition int, lag int64)
	}

	DLQ interface {
		DLSent(topic string, originalTopic string, retryCount int)
		DLError(topic string, reason string)
		DLRetryCount(originalTopic string, retryCount int)
	}
)
