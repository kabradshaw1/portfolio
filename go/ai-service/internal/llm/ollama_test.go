package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kabradshaw1/portfolio/go/pkg/resilience"
)

func TestOllamaClient_Chat_FinalText(t *testing.T) {
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &gotBody); err != nil {
			t.Fatalf("bad request body: %v", err)
		}
		_, _ = w.Write([]byte(`{
			"model":"qwen2.5",
			"message":{"role":"assistant","content":"hello there"},
			"done":true
		}`))
	}))
	defer server.Close()

	client := NewOllamaClient(server.URL, "qwen2.5", resilience.NewBreaker(resilience.BreakerConfig{Name: "test"}))
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
	if gotBody["model"] != "qwen2.5" {
		t.Errorf("expected model qwen2.5, got %v", gotBody["model"])
	}
	if gotBody["stream"] != false {
		t.Errorf("expected stream=false, got %v", gotBody["stream"])
	}
}

func TestOllamaClient_Chat_ToolCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"model":"qwen2.5",
			"message":{
				"role":"assistant",
				"content":"",
				"tool_calls":[
					{"function":{"name":"search_products","arguments":{"query":"jacket","max_price":150}}}
				]
			},
			"done":true
		}`))
	}))
	defer server.Close()

	client := NewOllamaClient(server.URL, "qwen2.5", resilience.NewBreaker(resilience.BreakerConfig{Name: "test"}))
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
	if resp.ToolCalls[0].ID == "" {
		t.Errorf("expected a generated tool call id")
	}
}

func TestOllamaClient_Chat_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewOllamaClient(server.URL, "qwen2.5", resilience.NewBreaker(resilience.BreakerConfig{Name: "test"}))
	_, err := client.Chat(context.Background(), []Message{{Role: RoleUser, Content: "hi"}}, nil)
	if err == nil {
		t.Fatal("expected error on 500, got nil")
	}
}
