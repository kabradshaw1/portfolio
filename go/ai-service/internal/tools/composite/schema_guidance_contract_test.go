package composite

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCompositeToolSchemasExposeDescribedObjectContracts(t *testing.T) {
	tests := []struct {
		name string
		tool interface {
			Description() string
			Schema() json.RawMessage
		}
		required         []string
		described        []string
		arrayStringItems []string
		authScoped       bool
		requiresNoUser   bool
	}{
		{
			name:             "compare_products",
			tool:             NewCompareProductsTool(fakeProductCatalog{}, fakeEmbeddings{}),
			required:         []string{"product_ids"},
			described:        []string{"product_ids"},
			arrayStringItems: []string{"product_ids"},
		},
		{
			name:           "recommend_with_rationale",
			tool:           NewRecommendWithRationaleTool(fakeUserHistory{}, fakeNeighborSearch{}),
			described:      []string{"category"},
			authScoped:     true,
			requiresNoUser: true,
		},
		{
			name:       "investigate_my_order",
			tool:       NewInvestigateMyOrderTool(EvidenceFetcher{}),
			required:   []string{"order_id"},
			described:  []string{"order_id"},
			authScoped: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema := decodeCompositeSchema(t, tt.tool.Schema())
			if got := schema["type"]; got != "object" {
				t.Fatalf("schema type = %v, want object", got)
			}
			props := compositeSchemaProperties(t, schema)
			if tt.authScoped {
				if _, ok := props["user_id"]; ok {
					t.Fatalf("authenticated-user tool must not advertise user_id")
				}
			}
			if tt.requiresNoUser && compositeSchemaRequires(schema, "user_id") {
				t.Fatalf("authenticated-user tool must not require user_id")
			}
			for _, field := range tt.required {
				if !compositeSchemaRequires(schema, field) {
					t.Fatalf("schema required fields missing %q", field)
				}
			}
			for _, field := range tt.described {
				prop, ok := props[field].(map[string]any)
				if !ok {
					t.Fatalf("schema properties missing %q", field)
				}
				desc, _ := prop["description"].(string)
				if strings.TrimSpace(desc) == "" {
					t.Fatalf("property %q must include a description", field)
				}
			}
			for _, field := range tt.arrayStringItems {
				prop, ok := props[field].(map[string]any)
				if !ok {
					t.Fatalf("schema properties missing %q", field)
				}
				items, ok := prop["items"].(map[string]any)
				if !ok {
					t.Fatalf("array property %q must include items schema", field)
				}
				if got := items["type"]; got != "string" {
					t.Fatalf("array property %q items type = %v, want string", field, got)
				}
			}
		})
	}
}

func TestCompositeToolDescriptionsIncludeUseAndDoNotUseGuidance(t *testing.T) {
	tests := []struct {
		name string
		tool interface{ Description() string }
	}{
		{name: "compare_products", tool: NewCompareProductsTool(fakeProductCatalog{}, fakeEmbeddings{})},
		{name: "recommend_with_rationale", tool: NewRecommendWithRationaleTool(fakeUserHistory{}, fakeNeighborSearch{})},
		{name: "investigate_my_order", tool: NewInvestigateMyOrderTool(EvidenceFetcher{})},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			desc := strings.ToLower(tt.tool.Description())
			if !strings.Contains(desc, "use when") {
				t.Fatalf("description must include Use when guidance: %q", tt.tool.Description())
			}
			if !strings.Contains(desc, "do not use") {
				t.Fatalf("description must include Do not use guidance: %q", tt.tool.Description())
			}
		})
	}
}

func decodeCompositeSchema(t *testing.T, raw json.RawMessage) map[string]any {
	t.Helper()
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("schema is invalid JSON: %v", err)
	}
	if schema == nil {
		t.Fatalf("schema must be a JSON object")
	}
	return schema
}

func compositeSchemaProperties(t *testing.T, schema map[string]any) map[string]any {
	t.Helper()
	raw, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema properties must be an object")
	}
	return raw
}

func compositeSchemaRequires(schema map[string]any, field string) bool {
	raw, ok := schema["required"].([]any)
	if !ok {
		return false
	}
	for _, v := range raw {
		if v == field {
			return true
		}
	}
	return false
}
