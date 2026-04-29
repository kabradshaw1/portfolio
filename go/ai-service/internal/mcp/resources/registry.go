// Package resources provides an MCP Resource registry. Resources are
// read-only URIs the client can list and fetch (e.g. catalog://categories,
// runbook://how-this-portfolio-works). Each Resource is responsible for
// its own data fetching and content shape; the registry is just a thin
// router from URI to Resource.
package resources

import (
	"context"
	"errors"
	"strings"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/kabradshaw1/portfolio/go/ai-service/internal/metrics"
)

// ErrResourceNotFound is returned by Registry.Read when no resource is
// registered for the given URI.
var ErrResourceNotFound = errors.New("resource not found")

// Resource is one MCP-addressable read-only document.
type Resource interface {
	// URI is the canonical address (scheme://path) the LLM uses to reference
	// this resource. Must be stable across server restarts.
	URI() string
	// Name is a short human-readable label (shown in MCP clients).
	Name() string
	// Description explains what the resource contains.
	Description() string
	// MIMEType is the response media type, e.g. "application/json" or "text/markdown".
	MIMEType() string
	// Read fetches the current content. Implementations should treat ctx as the
	// request lifetime and avoid background work that outlives it.
	Read(ctx context.Context) (Content, error)
}

// Content is the output of Resource.Read. Text-only for v1; binary content
// (e.g. image bytes) is intentionally out of scope.
type Content struct {
	URI      string
	MIMEType string
	Text     string
}

// Registry is a thread-safe in-memory router from URI to Resource.
type Registry struct {
	mu        sync.RWMutex
	resources map[string]Resource
	catalog   CatalogClient
}

// NewRegistry constructs an empty Registry.
func NewRegistry() *Registry {
	return &Registry{resources: make(map[string]Resource)}
}

// WithCatalogClient enables templated catalog://product/{id} reads without
// enumerating every product in resources/list.
func (r *Registry) WithCatalogClient(c CatalogClient) *Registry {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.catalog = c
	return r
}

// HasCatalogClient reports whether templated catalog resources are enabled.
func (r *Registry) HasCatalogClient() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.catalog != nil
}

// Register adds (or replaces) a Resource by its URI. Concurrency-safe.
func (r *Registry) Register(res Resource) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.resources[res.URI()] = res
}

// List returns a snapshot of all registered resources. Order is not guaranteed.
func (r *Registry) List() []Resource {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Resource, 0, len(r.resources))
	for _, v := range r.resources {
		out = append(out, v)
	}
	return out
}

// Read looks up the Resource for uri and returns its current Content.
// Returns ErrResourceNotFound if no Resource is registered.
func (r *Registry) Read(ctx context.Context, uri string) (Content, error) {
	ctx, span := otel.Tracer("ai-service/mcp").Start(ctx, "mcp.resource.read",
		trace.WithAttributes(attribute.String("mcp.resource.uri", uri)))
	defer span.End()

	content, err := r.read(ctx, uri)
	result := "ok"
	if err != nil {
		result = "error"
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	metrics.MCPResourcesReadTotal.WithLabelValues(uri, result).Inc()
	return content, err
}

func (r *Registry) read(ctx context.Context, uri string) (Content, error) {
	r.mu.RLock()
	res, ok := r.resources[uri]
	catalog := r.catalog
	r.mu.RUnlock()
	if ok {
		return res.Read(ctx)
	}
	if id, ok := matchProductURI(uri); ok && catalog != nil {
		return NewProductResource(catalog, id).Read(ctx)
	}
	return Content{}, ErrResourceNotFound
}

func matchProductURI(uri string) (string, bool) {
	id, ok := strings.CutPrefix(uri, productURIPrefix)
	return id, ok && id != ""
}
