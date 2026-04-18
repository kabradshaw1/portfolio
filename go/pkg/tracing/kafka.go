package tracing

import (
	"context"

	"github.com/segmentio/kafka-go"
	"go.opentelemetry.io/otel"
)

// kafkaHeaderCarrier adapts a *[]kafka.Header to the TextMapCarrier interface
// so OTel can inject/extract trace context into Kafka message headers.
type kafkaHeaderCarrier struct {
	headers *[]kafka.Header
}

func (c *kafkaHeaderCarrier) Get(key string) string {
	for _, h := range *c.headers {
		if h.Key == key {
			return string(h.Value)
		}
	}
	return ""
}

func (c *kafkaHeaderCarrier) Set(key, value string) {
	// Overwrite if key exists.
	for i, h := range *c.headers {
		if h.Key == key {
			(*c.headers)[i].Value = []byte(value)
			return
		}
	}
	*c.headers = append(*c.headers, kafka.Header{Key: key, Value: []byte(value)})
}

func (c *kafkaHeaderCarrier) Keys() []string {
	keys := make([]string, len(*c.headers))
	for i, h := range *c.headers {
		keys[i] = h.Key
	}
	return keys
}

// InjectKafka writes W3C trace context into Kafka message headers.
func InjectKafka(ctx context.Context, headers *[]kafka.Header) {
	otel.GetTextMapPropagator().Inject(ctx, &kafkaHeaderCarrier{headers: headers})
}

// ExtractKafka reads W3C trace context from Kafka message headers into a new context.
func ExtractKafka(ctx context.Context, headers []kafka.Header) context.Context {
	return otel.GetTextMapPropagator().Extract(ctx, &kafkaHeaderCarrier{headers: &headers})
}
