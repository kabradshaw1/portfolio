package tools

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestToolSchemasExposeDescribedObjectContracts(t *testing.T) {
	tests := []struct {
		name             string
		tool             Tool
		required         []string
		described        []string
		arrayStringItems []string
		authScoped       bool
	}{
		{
			name:      "search_products",
			tool:      NewSearchProductsTool(&fakeEcommerce{}),
			required:  []string{"query"},
			described: []string{"query", "limit"},
		},
		{
			name:      "get_product",
			tool:      NewGetProductTool(&fakeEcommerce{}),
			required:  []string{"product_id"},
			described: []string{"product_id"},
		},
		{
			name:      "check_inventory",
			tool:      NewCheckInventoryTool(&fakeEcommerce{}),
			required:  []string{"product_id"},
			described: []string{"product_id"},
		},
		{
			name:       "list_orders",
			tool:       NewListOrdersTool(&fakeOrdersAPI{}),
			described:  []string{"limit"},
			authScoped: true,
		},
		{
			name:       "get_order",
			tool:       NewGetOrderTool(&fakeOrdersAPI{}),
			required:   []string{"order_id"},
			described:  []string{"order_id"},
			authScoped: true,
		},
		{
			name:       "summarize_orders",
			tool:       NewSummarizeOrdersTool(&fakeOrdersAPI{}, &summarizerLLM{}),
			described:  []string{"period"},
			authScoped: true,
		},
		{
			name:       "view_cart",
			tool:       NewViewCartTool(&fakeCartAPI{}),
			authScoped: true,
		},
		{
			name:       "add_to_cart",
			tool:       NewAddToCartTool(&fakeCartAPI{}),
			required:   []string{"product_id", "qty"},
			described:  []string{"product_id", "qty"},
			authScoped: true,
		},
		{
			name:             "initiate_return",
			tool:             NewInitiateReturnTool(&fakeReturnsAPI{}),
			required:         []string{"order_id", "item_ids", "reason"},
			described:        []string{"order_id", "item_ids", "reason"},
			arrayStringItems: []string{"item_ids"},
			authScoped:       true,
		},
		{
			name:      "search_documents",
			tool:      NewSearchDocumentsTool(&fakeRAG{}),
			required:  []string{"query"},
			described: []string{"query", "collection", "limit"},
		},
		{
			name:      "ask_document",
			tool:      NewAskDocumentTool(&fakeRAG{}),
			required:  []string{"question"},
			described: []string{"question", "collection"},
		},
		{
			name: "list_collections",
			tool: NewListCollectionsTool(&fakeRAG{}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema := decodeToolSchema(t, tt.tool.Schema())
			if got := schema["type"]; got != "object" {
				t.Fatalf("schema type = %v, want object", got)
			}
			props := schemaProperties(t, schema)
			if tt.authScoped {
				if _, ok := props["user_id"]; ok {
					t.Fatalf("authenticated-user tool must not advertise user_id")
				}
			}
			for _, field := range tt.required {
				if !schemaRequires(schema, field) {
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

func TestToolDescriptionsIncludeUseAndDoNotUseGuidance(t *testing.T) {
	tests := []struct {
		name string
		tool Tool
	}{
		{name: "search_products", tool: NewSearchProductsTool(&fakeEcommerce{})},
		{name: "get_product", tool: NewGetProductTool(&fakeEcommerce{})},
		{name: "check_inventory", tool: NewCheckInventoryTool(&fakeEcommerce{})},
		{name: "list_orders", tool: NewListOrdersTool(&fakeOrdersAPI{})},
		{name: "get_order", tool: NewGetOrderTool(&fakeOrdersAPI{})},
		{name: "summarize_orders", tool: NewSummarizeOrdersTool(&fakeOrdersAPI{}, &summarizerLLM{})},
		{name: "view_cart", tool: NewViewCartTool(&fakeCartAPI{})},
		{name: "add_to_cart", tool: NewAddToCartTool(&fakeCartAPI{})},
		{name: "initiate_return", tool: NewInitiateReturnTool(&fakeReturnsAPI{})},
		{name: "search_documents", tool: NewSearchDocumentsTool(&fakeRAG{})},
		{name: "ask_document", tool: NewAskDocumentTool(&fakeRAG{})},
		{name: "list_collections", tool: NewListCollectionsTool(&fakeRAG{})},
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

func decodeToolSchema(t *testing.T, raw json.RawMessage) map[string]any {
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

func schemaProperties(t *testing.T, schema map[string]any) map[string]any {
	t.Helper()
	raw, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema properties must be an object")
	}
	return raw
}

func schemaRequires(schema map[string]any, field string) bool {
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
