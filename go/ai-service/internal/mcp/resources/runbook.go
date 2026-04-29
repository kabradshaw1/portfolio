package resources

import (
	"context"
	"errors"
	"os"
	"sync"
)

// fileBackedResource is a small private helper for file-loaded resources
// that read once per process via sync.Once. Used by runbook:// and schema://.
type fileBackedResource struct {
	uri         string
	name        string
	description string
	mime        string
	path        string

	once    sync.Once
	content string
	loadErr error
}

func newFileBackedResource(uri, name, description, mime, path string) (*fileBackedResource, error) {
	if path == "" {
		return nil, errors.New(uri + ": path required")
	}
	return &fileBackedResource{
		uri: uri, name: name, description: description, mime: mime, path: path,
	}, nil
}

func (r *fileBackedResource) URI() string         { return r.uri }
func (r *fileBackedResource) Name() string        { return r.name }
func (r *fileBackedResource) Description() string { return r.description }
func (r *fileBackedResource) MIMEType() string    { return r.mime }

func (r *fileBackedResource) Read(_ context.Context) (Content, error) {
	r.once.Do(func() {
		b, err := os.ReadFile(r.path)
		if err != nil {
			r.loadErr = err
			return
		}
		r.content = string(b)
	})
	if r.loadErr != nil {
		return Content{}, r.loadErr
	}
	return Content{URI: r.uri, MIMEType: r.mime, Text: r.content}, nil
}

// NewRunbookResource returns runbook://how-this-portfolio-works backed by
// the markdown file at path. The content is loaded once per process.
func NewRunbookResource(path string) (Resource, error) {
	return newFileBackedResource(
		"runbook://how-this-portfolio-works",
		"How this portfolio works",
		"Architectural narrative of the portfolio system.",
		"text/markdown",
		path,
	)
}
