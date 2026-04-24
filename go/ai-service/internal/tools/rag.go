package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/kabradshaw1/portfolio/go/ai-service/internal/tools/clients"
)

// ragAPI is the subset of the RAG HTTP client the RAG tools use.
// Kept as an interface so tests can swap in a fake.
type ragAPI interface {
	Search(ctx context.Context, query, collection string, limit int) ([]clients.SearchResult, error)
	Ask(ctx context.Context, question, collection string) (clients.AskAnswer, error)
	ListCollections(ctx context.Context) ([]clients.Collection, error)
}

// -------- search_documents --------

type searchDocumentsTool struct {
	api ragAPI
}

func NewSearchDocumentsTool(api ragAPI) Tool { return &searchDocumentsTool{api: api} }

func (t *searchDocumentsTool) Name() string { return "search_documents" }
func (t *searchDocumentsTool) Description() string {
	return "Semantic search over ingested documents. Returns ranked chunks with source file and page. Optional collection name and limit (default 5, max 20)."
}
func (t *searchDocumentsTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"properties":{
			"query":{"type":"string","description":"Free-text search query."},
			"collection":{"type":"string","description":"Optional collection name to search within."},
			"limit":{"type":"integer","description":"Max results to return (default 5, cap 20)."}
		},
		"required":["query"]
	}`)
}

type searchDocumentsArgs struct {
	Query      string `json:"query"`
	Collection string `json:"collection"`
	Limit      int    `json:"limit"`
}

const maxDocSearchResults = 20

func (t *searchDocumentsTool) Call(ctx context.Context, args json.RawMessage, userID string) (Result, error) {
	start := time.Now()
	var a searchDocumentsArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return Result{}, fmt.Errorf("search_documents: bad args: %w", err)
	}
	if a.Query == "" {
		return Result{}, errors.New("search_documents: query is required")
	}
	limit := a.Limit
	if limit <= 0 {
		limit = 5
	}
	if limit > maxDocSearchResults {
		limit = maxDocSearchResults
	}

	results, err := t.api.Search(ctx, a.Query, a.Collection, limit)
	if err != nil {
		slog.WarnContext(ctx, "tool search_documents failed", "tool", "search_documents", "query", truncate(a.Query, 200), "error", err.Error())
		return Result{}, fmt.Errorf("search_documents: %w", err)
	}

	out := make([]map[string]any, 0, len(results))
	for _, r := range results {
		out = append(out, map[string]any{
			"text":        r.Text,
			"filename":    r.Filename,
			"page_number": r.PageNumber,
			"score":       r.Score,
		})
	}
	slog.InfoContext(ctx, "tool search_documents executed", "tool", "search_documents", "query", truncate(a.Query, 200), "collection", a.Collection, "result_count", len(out), "duration_ms", time.Since(start).Milliseconds())
	return Result{
		Content: out,
		Display: map[string]any{"kind": "search_results", "results": out},
	}, nil
}

// -------- ask_document --------

type askDocumentTool struct {
	api ragAPI
}

func NewAskDocumentTool(api ragAPI) Tool { return &askDocumentTool{api: api} }

func (t *askDocumentTool) Name() string { return "ask_document" }
func (t *askDocumentTool) Description() string {
	return "Ask a natural-language question against ingested documents. Returns a generated answer with source citations."
}
func (t *askDocumentTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"properties":{
			"question":{"type":"string","description":"The question to answer using document context."},
			"collection":{"type":"string","description":"Optional collection name to query within."}
		},
		"required":["question"]
	}`)
}

type askDocumentArgs struct {
	Question   string `json:"question"`
	Collection string `json:"collection"`
}

func (t *askDocumentTool) Call(ctx context.Context, args json.RawMessage, userID string) (Result, error) {
	start := time.Now()
	var a askDocumentArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return Result{}, fmt.Errorf("ask_document: bad args: %w", err)
	}
	if a.Question == "" {
		return Result{}, errors.New("ask_document: question is required")
	}

	ans, err := t.api.Ask(ctx, a.Question, a.Collection)
	if err != nil {
		slog.WarnContext(ctx, "tool ask_document failed", "tool", "ask_document", "question", truncate(a.Question, 200), "error", err.Error())
		return Result{}, fmt.Errorf("ask_document: %w", err)
	}

	sources := make([]map[string]any, 0, len(ans.Sources))
	for _, s := range ans.Sources {
		sources = append(sources, map[string]any{
			"file": s.File,
			"page": s.Page,
		})
	}
	content := map[string]any{
		"answer":  ans.Answer,
		"sources": sources,
	}
	slog.InfoContext(ctx, "tool ask_document executed", "tool", "ask_document", "question", truncate(a.Question, 200), "collection", a.Collection, "duration_ms", time.Since(start).Milliseconds())
	return Result{
		Content: content,
		Display: map[string]any{"kind": "rag_answer", "answer": ans.Answer, "sources": sources},
	}, nil
}

// -------- list_collections --------

type listCollectionsTool struct {
	api ragAPI
}

func NewListCollectionsTool(api ragAPI) Tool { return &listCollectionsTool{api: api} }

func (t *listCollectionsTool) Name() string { return "list_collections" }
func (t *listCollectionsTool) Description() string {
	return "List all document collections available in the vector store, with their document counts."
}
func (t *listCollectionsTool) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{}}`)
}

func (t *listCollectionsTool) Call(ctx context.Context, args json.RawMessage, userID string) (Result, error) {
	start := time.Now()
	cols, err := t.api.ListCollections(ctx)
	if err != nil {
		slog.WarnContext(ctx, "tool list_collections failed", "tool", "list_collections", "error", err.Error())
		return Result{}, fmt.Errorf("list_collections: %w", err)
	}

	out := make([]map[string]any, 0, len(cols))
	for _, c := range cols {
		out = append(out, map[string]any{
			"name":        c.Name,
			"point_count": c.PointCount,
		})
	}
	slog.InfoContext(ctx, "tool list_collections executed", "tool", "list_collections", "collection_count", len(out), "duration_ms", time.Since(start).Milliseconds())
	return Result{
		Content: out,
		Display: map[string]any{"kind": "collections_list", "collections": out},
	}, nil
}

// truncate returns a string truncated to max length with ellipsis if needed.
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
