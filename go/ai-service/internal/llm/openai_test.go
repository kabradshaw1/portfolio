package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAIClient_Chat_FinalText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-key" {
			t.Fatalf("unexpected auth header: %s", auth)
		}
		_, _ = w.Write([]byte(`{
			"choices":[{"message":{"role":"assistant","content":"hello there"}}],
			"usage":{"prompt_tokens":10,"completion_tokens":5}
		}`))
	}))
	defer server.Close()

	client := NewOpenAIClient(server.URL, "gpt-test", "test-key")
	resp, err := client.Chat(context.Background(),
		[]Message{{Role: RoleUser, Content: "hi"}},
		nil,
	)
	if err != nil {
		t.Fatalf("Chat returned error: %v", err)
	}
	if resp.Content != "hello there" {
		t.Errorf("expected content 'hello there', got %q", resp.Content)
	}
	if len(resp.ToolCalls) != 0 {
		t.Errorf("expected no tool calls, got %d", len(resp.ToolCalls))
	}
	if resp.PromptEvalCount != 10 {
		t.Errorf("expected 10 prompt tokens, got %d", resp.PromptEvalCount)
	}
	if resp.EvalCount != 5 {
		t.Errorf("expected 5 completion tokens, got %d", resp.EvalCount)
	}
}

func TestOpenAIClient_Chat_ToolCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"choices":[{
				"message":{
					"role":"assistant",
					"content":"",
					"tool_calls":[{
						"id":"call_123",
						"type":"function",
						"function":{"name":"search_products","arguments":"{\"query\":\"jacket\"}"}
					}]
				}
			}],
			"usage":{"prompt_tokens":15,"completion_tokens":8}
		}`))
	}))
	defer server.Close()

	client := NewOpenAIClient(server.URL, "gpt-test", "test-key")
	resp, err := client.Chat(context.Background(),
		[]Message{{Role: RoleUser, Content: "find a jacket"}},
		[]ToolSchema{{Name: "search_products", Description: "", Parameters: json.RawMessage(`{}`)}},
	)
	if err != nil {
		t.Fatalf("Chat returned error: %v", err)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Name != "search_products" {
		t.Errorf("expected name search_products, got %q", resp.ToolCalls[0].Name)
	}
	if !strings.Contains(string(resp.ToolCalls[0].Args), "jacket") {
		t.Errorf("expected args to contain 'jacket', got %s", resp.ToolCalls[0].Args)
	}
	if resp.ToolCalls[0].ID != "call_123" {
		t.Errorf("expected id call_123, got %q", resp.ToolCalls[0].ID)
	}
}

func TestOpenAIClient_Chat_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewOpenAIClient(server.URL, "gpt-test", "test-key")
	_, err := client.Chat(context.Background(), []Message{{Role: RoleUser, Content: "hi"}}, nil)
	if err == nil {
		t.Fatal("expected error on 500, got nil")
	}
}
