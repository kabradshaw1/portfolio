package pagination

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

type cursorPayload struct {
	Value string `json:"v"`
	ID    string `json:"id"`
}

func EncodeCursor(sortValue string, id uuid.UUID) string {
	payload := cursorPayload{Value: sortValue, ID: id.String()}
	data, _ := json.Marshal(payload)
	return base64.URLEncoding.EncodeToString(data)
}

func DecodeCursor(cursor string) (string, uuid.UUID, error) {
	if cursor == "" {
		return "", uuid.Nil, fmt.Errorf("cursor is empty")
	}
	data, err := base64.URLEncoding.DecodeString(cursor)
	if err != nil {
		return "", uuid.Nil, fmt.Errorf("invalid cursor encoding: %w", err)
	}
	var payload cursorPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return "", uuid.Nil, fmt.Errorf("invalid cursor format: %w", err)
	}
	id, err := uuid.Parse(payload.ID)
	if err != nil {
		return "", uuid.Nil, fmt.Errorf("invalid cursor ID: %w", err)
	}
	return payload.Value, id, nil
}
