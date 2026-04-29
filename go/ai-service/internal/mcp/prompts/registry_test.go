package prompts

import (
	"context"
	"errors"
	"testing"
)

type fakePrompt struct{ name string }

func (f fakePrompt) Name() string          { return f.name }
func (f fakePrompt) Description() string   { return "fake" }
func (f fakePrompt) Arguments() []Argument { return nil }
func (f fakePrompt) Render(ctx context.Context, args map[string]string) (Rendered, error) {
	return Rendered{Messages: []Message{{Role: "user", Text: "rendered:" + f.name}}}, nil
}

func TestPromptRegistryListAndGet(t *testing.T) {
	r := NewRegistry()
	r.Register(fakePrompt{name: "a"})
	r.Register(fakePrompt{name: "b"})

	if len(r.List()) != 2 {
		t.Fatalf("expected 2, got %d", len(r.List()))
	}

	got, err := r.Get(context.Background(), "a", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Messages[0].Text != "rendered:a" {
		t.Fatalf("got %v", got)
	}
}

func TestPromptRegistryGetUnknown(t *testing.T) {
	r := NewRegistry()
	_, err := r.Get(context.Background(), "missing", nil)
	if !errors.Is(err, ErrPromptNotFound) {
		t.Fatalf("got %v", err)
	}
}

func TestPromptRegistryRegisterIsThreadSafe(t *testing.T) {
	r := NewRegistry()
	done := make(chan struct{}, 16)
	for i := range 16 {
		name := "p" + string(rune('a'+i))
		go func() {
			r.Register(fakePrompt{name: name})
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
