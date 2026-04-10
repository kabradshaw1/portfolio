package llm

import (
	"testing"

	"github.com/kabradshaw1/portfolio/go/pkg/resilience"
)

func TestNewClient_Ollama(t *testing.T) {
	client, err := NewClient("ollama", "http://localhost:11434", "qwen2.5", "", resilience.NewBreaker(resilience.BreakerConfig{Name: "test"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := client.(*OllamaClient); !ok {
		t.Errorf("expected *OllamaClient, got %T", client)
	}
}

func TestNewClient_OpenAI(t *testing.T) {
	client, err := NewClient("openai", "https://api.groq.com/openai/v1", "llama-3.1-70b", "key", resilience.NewBreaker(resilience.BreakerConfig{Name: "test"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := client.(*OpenAIClient); !ok {
		t.Errorf("expected *OpenAIClient, got %T", client)
	}
}

func TestNewClient_Anthropic(t *testing.T) {
	client, err := NewClient("anthropic", "", "claude-sonnet-4-20250514", "key", resilience.NewBreaker(resilience.BreakerConfig{Name: "test"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := client.(*AnthropicClient); !ok {
		t.Errorf("expected *AnthropicClient, got %T", client)
	}
}

func TestNewClient_Unknown(t *testing.T) {
	_, err := NewClient("unknown", "", "", "", resilience.NewBreaker(resilience.BreakerConfig{Name: "test"}))
	if err == nil {
		t.Fatal("expected error for unknown provider, got nil")
	}
}
