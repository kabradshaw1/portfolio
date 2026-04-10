package tracing

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

// amqpCarrier adapts a map[string]interface{} (AMQP headers) to the
// TextMapCarrier interface so OTel can inject/extract trace context.
type amqpCarrier map[string]interface{}

func (c amqpCarrier) Get(key string) string {
	v, _ := c[key].(string)
	return v
}

func (c amqpCarrier) Set(key, value string) {
	c[key] = value
}

func (c amqpCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	return keys
}

// InjectAMQP writes W3C trace context into AMQP headers.
func InjectAMQP(ctx context.Context, headers map[string]interface{}) {
	otel.GetTextMapPropagator().Inject(ctx, amqpCarrier(headers))
}

// ExtractAMQP reads W3C trace context from AMQP headers into a new context.
func ExtractAMQP(ctx context.Context, headers map[string]interface{}) context.Context {
	return otel.GetTextMapPropagator().Extract(ctx, amqpCarrier(headers))
}

// Propagator returns the global text map propagator for use in tests.
func Propagator() propagation.TextMapPropagator {
	return otel.GetTextMapPropagator()
}
