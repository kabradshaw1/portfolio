package tools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/kabradshaw1/portfolio/go/ai-service/internal/tools/clients"
)

type fakeRAG struct {
	searchResults  []clients.SearchResult
	searchErr      error
	askAnswer      clients.AskAnswer
	askErr         error
	collections    []clients.Collection
	collectionsErr error
}

func (f *fakeRAG) Search(ctx context.Context, query, collection string, limit int) ([]clients.SearchResult, error) {
	return f.searchResults, f.searchErr
}

func (f *fakeRAG) Ask(ctx context.Context, question, collection string) (clients.AskAnswer, error) {
	return f.askAnswer, f.askErr
}

func (f *fakeRAG) ListCollections(ctx context.Context) ([]clients.Collection, error) {
	return f.collections, f.collectionsErr
}

func TestSearchDocumentsTool_Success(t *testing.T) {
	fake := &fakeRAG{searchResults: []clients.SearchResult{
		{Text: "Kubernetes is...", Filename: "k8s.pdf", PageNumber: 1, Score: 0.95},
		{Text: "Pods are...", Filename: "k8s.pdf", PageNumber: 3, Score: 0.82},
	}}
	tool := NewSearchDocumentsTool(fake)

	if tool.Name() != "search_documents" {
		t.Fatalf("expected name search_documents, got %s", tool.Name())
	}

	res, err := tool.Call(context.Background(), json.RawMessage(`{"query":"kubernetes"}`), "")
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	items, ok := res.Content.([]map[string]any)
	if !ok {
		t.Fatalf("expected []map content, got %T", res.Content)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 results, got %d", len(items))
	}
	if items[0]["text"] != "Kubernetes is..." {
		t.Errorf("unexpected text: %v", items[0]["text"])
	}
}

func TestSearchDocumentsTool_MissingQuery(t *testing.T) {
	tool := NewSearchDocumentsTool(&fakeRAG{})
	_, err := tool.Call(context.Background(), json.RawMessage(`{}`), "")
	if err == nil {
		t.Fatal("expected error for missing query")
	}
}

func TestSearchDocumentsTool_APIError(t *testing.T) {
	fake := &fakeRAG{searchErr: errors.New("connection refused")}
	tool := NewSearchDocumentsTool(fake)
	_, err := tool.Call(context.Background(), json.RawMessage(`{"query":"test"}`), "")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestAskDocumentTool_Success(t *testing.T) {
	fake := &fakeRAG{askAnswer: clients.AskAnswer{
		Answer:  "Kubernetes is a container orchestration platform.",
		Sources: []clients.AskSource{{File: "k8s.pdf", Page: 1}},
	}}
	tool := NewAskDocumentTool(fake)

	if tool.Name() != "ask_document" {
		t.Fatalf("expected name ask_document, got %s", tool.Name())
	}

	res, err := tool.Call(context.Background(), json.RawMessage(`{"question":"what is kubernetes"}`), "")
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	m, ok := res.Content.(map[string]any)
	if !ok {
		t.Fatalf("expected map content, got %T", res.Content)
	}
	if m["answer"] != "Kubernetes is a container orchestration platform." {
		t.Errorf("unexpected answer: %v", m["answer"])
	}
}

func TestAskDocumentTool_MissingQuestion(t *testing.T) {
	tool := NewAskDocumentTool(&fakeRAG{})
	_, err := tool.Call(context.Background(), json.RawMessage(`{}`), "")
	if err == nil {
		t.Fatal("expected error for missing question")
	}
}

func TestListCollectionsTool_Success(t *testing.T) {
	fake := &fakeRAG{collections: []clients.Collection{
		{Name: "documents", PointCount: 150},
		{Name: "debug-myproject", PointCount: 42},
	}}
	tool := NewListCollectionsTool(fake)

	if tool.Name() != "list_collections" {
		t.Fatalf("expected name list_collections, got %s", tool.Name())
	}

	res, err := tool.Call(context.Background(), json.RawMessage(`{}`), "")
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	items, ok := res.Content.([]map[string]any)
	if !ok {
		t.Fatalf("expected []map content, got %T", res.Content)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 collections, got %d", len(items))
	}
	if items[0]["name"] != "documents" {
		t.Errorf("unexpected name: %v", items[0]["name"])
	}
}
