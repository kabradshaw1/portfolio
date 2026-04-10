package llm

import "fmt"

// NewClient creates an LLM Client based on the provider name.
// Supported providers: "ollama", "openai", "anthropic".
func NewClient(provider, baseURL, model, apiKey string) (Client, error) {
	switch provider {
	case "ollama":
		return NewOllamaClient(baseURL, model), nil
	case "openai":
		return NewOpenAIClient(baseURL, model, apiKey), nil
	case "anthropic":
		return NewAnthropicClient(model, apiKey), nil
	default:
		return nil, fmt.Errorf("unknown LLM provider: %q (use ollama, openai, or anthropic)", provider)
	}
}
