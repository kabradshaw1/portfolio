package llm

import (
	"fmt"

	gobreaker "github.com/sony/gobreaker/v2"
)

// NewClient creates an LLM Client based on the provider name.
// Supported providers: "ollama", "openai", "anthropic".
// The breaker is applied to ollama; other providers ignore it for now.
func NewClient(provider, baseURL, model, apiKey string, breaker *gobreaker.CircuitBreaker[any]) (Client, error) {
	switch provider {
	case "ollama":
		return NewOllamaClient(baseURL, model, breaker), nil
	case "openai":
		return NewOpenAIClient(baseURL, model, apiKey), nil
	case "anthropic":
		return NewAnthropicClient(model, apiKey), nil
	default:
		return nil, fmt.Errorf("unknown LLM provider: %q (use ollama, openai, or anthropic)", provider)
	}
}
