// Package prompts provides an MCP Prompt registry. Prompts are parameterized
// templates the server publishes via prompts/list and prompts/get; clients
// surface them as suggestion chips or slash commands. Each Prompt owns its
// Render logic; the registry is just a thin router from name to Prompt.
package prompts

import (
	"context"
	"errors"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/kabradshaw1/portfolio/go/ai-service/internal/metrics"
)

// ErrPromptNotFound is returned by Registry.Get when no prompt is registered
// under the given name.
var ErrPromptNotFound = errors.New("prompt not found")

// Argument describes one parameter the LLM client may pass to Prompt.Render.
type Argument struct {
	Name        string
	Description string
	Required    bool
}

// Message is one rendered turn in a prompt — Role is "user", "assistant",
// or "system" per the MCP spec.
type Message struct {
	Role string
	Text string
}

// Rendered is the result of Prompt.Render — a description plus an ordered
// sequence of Messages the client can splice into its conversation.
type Rendered struct {
	Description string
	Messages    []Message
}

// Prompt is one parameterized template.
type Prompt interface {
	Name() string
	Description() string
	Arguments() []Argument
	Render(ctx context.Context, args map[string]string) (Rendered, error)
}

// Registry is a thread-safe in-memory router from prompt name to Prompt.
type Registry struct {
	mu      sync.RWMutex
	prompts map[string]Prompt
}

// NewRegistry constructs an empty Registry.
func NewRegistry() *Registry {
	return &Registry{prompts: make(map[string]Prompt)}
}

// Register adds (or replaces) a Prompt by its Name. Concurrency-safe.
func (r *Registry) Register(p Prompt) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.prompts[p.Name()] = p
}

// List returns a snapshot of all registered prompts. Order is not guaranteed.
func (r *Registry) List() []Prompt {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Prompt, 0, len(r.prompts))
	for _, p := range r.prompts {
		out = append(out, p)
	}
	return out
}

// Get looks up the Prompt by name and renders it with args. Returns
// ErrPromptNotFound if no prompt is registered under name.
func (r *Registry) Get(ctx context.Context, name string, args map[string]string) (Rendered, error) {
	ctx, span := otel.Tracer("ai-service/mcp").Start(ctx, "mcp.prompt.get",
		trace.WithAttributes(attribute.String("mcp.prompt.name", name)))
	defer span.End()

	rendered, err := r.get(ctx, name, args)
	result := "ok"
	if err != nil {
		result = "error"
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	metrics.MCPPromptsGetTotal.WithLabelValues(name, result).Inc()
	return rendered, err
}

func (r *Registry) get(ctx context.Context, name string, args map[string]string) (Rendered, error) {
	r.mu.RLock()
	p, ok := r.prompts[name]
	r.mu.RUnlock()
	if !ok {
		return Rendered{}, ErrPromptNotFound
	}
	return p.Render(ctx, args)
}
