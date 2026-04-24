package consumer

import (
	"encoding/json"
	"fmt"

	"github.com/kabradshaw1/portfolio/go/order-projector/internal/event"
)

// LatestVersion is the current schema version all events are upgraded to.
const LatestVersion = 2

// upgrader transforms event data from version N to version N+1.
type upgrader func(data map[string]any) map[string]any

// upgradeRegistry maps event types to their version upgraders.
// Each entry maps a source version to the function that upgrades it to the next version.
var upgradeRegistry = map[string]map[int]upgrader{
	"order.created": {
		1: upgradeOrderCreatedV1toV2,
	},
}

// upgradeOrderCreatedV1toV2 adds the currency field defaulting to "USD" if absent.
func upgradeOrderCreatedV1toV2(data map[string]any) map[string]any {
	if _, ok := data["currency"]; !ok {
		data["currency"] = "USD"
	}
	return data
}

// Deserialize parses a Kafka message value into an OrderEvent,
// applying version upgrades to bring the event data to LatestVersion.
func Deserialize(value []byte) (*event.OrderEvent, error) {
	var evt event.OrderEvent
	if err := json.Unmarshal(value, &evt); err != nil {
		return nil, fmt.Errorf("unmarshal event: %w", err)
	}

	// Treat unversioned events as v1.
	if evt.Version == 0 {
		evt.Version = 1
	}

	if evt.Version < LatestVersion {
		upgraded, err := upgradeData(evt.Type, evt.Version, evt.Data)
		if err != nil {
			return nil, fmt.Errorf("upgrade event data: %w", err)
		}
		evt.Data = upgraded
		evt.Version = LatestVersion
	}

	return &evt, nil
}

// upgradeData chains version upgraders from fromVersion up to LatestVersion.
func upgradeData(eventType string, fromVersion int, raw json.RawMessage) (json.RawMessage, error) {
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return raw, fmt.Errorf("unmarshal data for upgrade: %w", err)
	}

	typeUpgraders, ok := upgradeRegistry[eventType]
	if !ok {
		return raw, nil
	}

	for v := fromVersion; v < LatestVersion; v++ {
		fn, ok := typeUpgraders[v]
		if !ok {
			continue
		}
		data = fn(data)
	}

	return json.Marshal(data)
}
