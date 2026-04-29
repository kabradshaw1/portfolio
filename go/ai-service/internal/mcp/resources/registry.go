// Package resources provides an MCP Resource registry. Resources are
// read-only URIs the client can list and fetch (e.g. catalog://categories,
// runbook://how-this-portfolio-works). Each Resource is responsible for
// its own data fetching and content shape; the registry is just a thin
// router from URI to Resource.
package resources

import (
	"context"
	"errors"
	"sync"
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
}

// NewRegistry constructs an empty Registry.
func NewRegistry() *Registry {
	return &Registry{resources: make(map[string]Resource)}
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
	r.mu.RLock()
	res, ok := r.resources[uri]
	r.mu.RUnlock()
	if !ok {
		return Content{}, ErrResourceNotFound
	}
	return res.Read(ctx)
}
