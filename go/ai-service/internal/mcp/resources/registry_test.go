package resources

import (
	"context"
	"errors"
	"testing"
)

type fakeResource struct {
	uri  string
	body string
	err  error
}

func (f fakeResource) URI() string         { return f.uri }
func (f fakeResource) Name() string        { return "fake" }
func (f fakeResource) Description() string { return "fake" }
func (f fakeResource) MIMEType() string    { return "text/plain" }
func (f fakeResource) Read(ctx context.Context) (Content, error) {
	if f.err != nil {
		return Content{}, f.err
	}
	return Content{URI: f.uri, MIMEType: "text/plain", Text: f.body}, nil
}

func TestRegistryListAndRead(t *testing.T) {
	r := NewRegistry()
	r.Register(fakeResource{uri: "fake://a", body: "hello"})
	r.Register(fakeResource{uri: "fake://b", body: "world"})

	list := r.List()
	if len(list) != 2 {
		t.Fatalf("expected 2, got %d", len(list))
	}

	got, err := r.Read(context.Background(), "fake://a")
	if err != nil {
		t.Fatal(err)
	}
	if got.Text != "hello" {
		t.Fatalf("got %s", got.Text)
	}
}

func TestRegistryReadUnknownURIReturnsError(t *testing.T) {
	r := NewRegistry()
	_, err := r.Read(context.Background(), "fake://missing")
	if !errors.Is(err, ErrResourceNotFound) {
		t.Fatalf("want ErrResourceNotFound, got %v", err)
	}
}

func TestRegistryRegisterIsThreadSafe(t *testing.T) {
	r := NewRegistry()
	done := make(chan struct{}, 16)
	for i := range 16 {
		uri := "fake://" + string(rune('a'+i))
		go func() {
			r.Register(fakeResource{uri: uri})
			done <- struct{}{}
		}()
	}
	for range 16 {
		<-done
	}
	if len(r.List()) != 16 {
		t.Fatalf("expected 16 entries, got %d", len(r.List()))
	}
}
