//go:build integration

package testutil

import (
	"context"
	"testing"

	tckafka "github.com/testcontainers/testcontainers-go/modules/kafka"
)

// Infra holds live connections to Kafka spun up by testcontainers.
type Infra struct {
	KafkaBrokers []string

	kafkaContainer *tckafka.KafkaContainer
}

// SetupInfra starts a Kafka container and returns connection details.
func SetupInfra(ctx context.Context, t testing.TB) *Infra {
	t.Helper()

	kafkaC, err := tckafka.Run(ctx, "confluentinc/confluent-local:7.6.0")
	if err != nil {
		t.Fatalf("start kafka container: %v", err)
	}

	brokers, err := kafkaC.Brokers(ctx)
	if err != nil {
		t.Fatalf("kafka brokers: %v", err)
	}

	infra := &Infra{
		KafkaBrokers:   brokers,
		kafkaContainer: kafkaC,
	}

	t.Cleanup(func() {
		cleanCtx := context.Background()
		if err := kafkaC.Terminate(cleanCtx); err != nil {
			t.Logf("terminate kafka container: %v", err)
		}
	})

	return infra
}

// Teardown terminates the Kafka container. For use in TestMain.
func (i *Infra) Teardown() {
	ctx := context.Background()
	_ = i.kafkaContainer.Terminate(ctx)
}
