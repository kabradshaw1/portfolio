package tools

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kabradshaw1/portfolio/go/ai-service/internal/jwtctx"
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/llm"
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/tools/clients"
)

type fakeOrdersAPI struct {
	listOut []clients.Order
	listErr error
	getOut  clients.Order
	getErr  error
	seenJWT string
	seenID  string
}

func (f *fakeOrdersAPI) ListOrders(ctx context.Context, jwt string) ([]clients.Order, error) {
	f.seenJWT = jwt
	return f.listOut, f.listErr
}

func (f *fakeOrdersAPI) GetOrder(ctx context.Context, jwt, id string) (clients.Order, error) {
	f.seenJWT = jwt
	f.seenID = id
	return f.getOut, f.getErr
}

func ctxWithJWT(jwt string) context.Context {
	return jwtctx.WithJWT(context.Background(), jwt)
}

func TestListOrdersTool_BoundedAndForwardsJWT(t *testing.T) {
	fake := &fakeOrdersAPI{listOut: make([]clients.Order, 50)}
	for i := range fake.listOut {
		fake.listOut[i] = clients.Order{
			ID:        "o" + string(rune('a'+i%26)),
			Status:    "paid",
			Total:     100 + i,
			CreatedAt: time.Now(),
		}
	}
	tool := NewListOrdersTool(fake)

	res, err := tool.Call(ctxWithJWT("tok"), json.RawMessage(`{}`), "user-1")
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if fake.seenJWT != "tok" {
		t.Errorf("jwt forwarded = %q", fake.seenJWT)
	}
	items := res.Content.([]map[string]any)
	if len(items) > 20 {
		t.Errorf("expected bound of 20, got %d", len(items))
	}
}

func TestListOrdersTool_RequiresUserID(t *testing.T) {
	tool := NewListOrdersTool(&fakeOrdersAPI{})
	_, err := tool.Call(ctxWithJWT("tok"), json.RawMessage(`{}`), "")
	if err == nil {
		t.Fatal("expected error when userID empty")
	}
}

func TestGetOrderTool_Success(t *testing.T) {
	fake := &fakeOrdersAPI{getOut: clients.Order{ID: "order-1", Status: "paid", Total: 12999}}
	tool := NewGetOrderTool(fake)

	res, err := tool.Call(ctxWithJWT("tok"), json.RawMessage(`{"order_id":"order-1"}`), "user-1")
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if fake.seenID != "order-1" || fake.seenJWT != "tok" {
		t.Errorf("seen id=%q jwt=%q", fake.seenID, fake.seenJWT)
	}
	m := res.Content.(map[string]any)
	if m["id"] != "order-1" || m["status"] != "paid" {
		t.Errorf("content = %+v", m)
	}
}

func TestGetOrderTool_MissingID(t *testing.T) {
	tool := NewGetOrderTool(&fakeOrdersAPI{})
	_, err := tool.Call(ctxWithJWT("tok"), json.RawMessage(`{}`), "user-1")
	if err == nil {
		t.Fatal("expected error for missing order_id")
	}
}

func TestGetOrderTool_RequiresUserID(t *testing.T) {
	tool := NewGetOrderTool(&fakeOrdersAPI{})
	_, err := tool.Call(ctxWithJWT("tok"), json.RawMessage(`{"order_id":"x"}`), "")
	if err == nil {
		t.Fatal("expected error for empty userID")
	}
}

type summarizerLLM struct {
	reply   string
	err     error
	seenMsg []llm.Message
	delay   time.Duration
}

func (f *summarizerLLM) Chat(ctx context.Context, msgs []llm.Message, tools []llm.ToolSchema) (llm.ChatResponse, error) {
	f.seenMsg = msgs
	if f.delay > 0 {
		time.Sleep(f.delay)
	}
	if f.err != nil {
		return llm.ChatResponse{}, f.err
	}
	return llm.ChatResponse{Content: f.reply}, nil
}

type recordedSubLLMCall struct {
	tool string
	dur  time.Duration
}

type fakeSubLLMRecorder struct {
	calls []recordedSubLLMCall
}

func (f *fakeSubLLMRecorder) RecordToolSubLLM(tool string, dur time.Duration) {
	f.calls = append(f.calls, recordedSubLLMCall{tool: tool, dur: dur})
}

type capturedLog struct {
	message string
	attrs   map[string]slog.Value
}

type captureSlogHandler struct {
	mu      sync.Mutex
	records []capturedLog
}

func (h *captureSlogHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return true
}

func (h *captureSlogHandler) Handle(ctx context.Context, r slog.Record) error {
	attrs := make(map[string]slog.Value, r.NumAttrs())
	r.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value
		return true
	})

	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, capturedLog{message: r.Message, attrs: attrs})
	return nil
}

func (h *captureSlogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return h
}

func (h *captureSlogHandler) WithGroup(name string) slog.Handler {
	return h
}

func captureSlog(t *testing.T) *captureSlogHandler {
	t.Helper()
	handler := &captureSlogHandler{}
	previous := slog.Default()
	slog.SetDefault(slog.New(handler))
	t.Cleanup(func() {
		slog.SetDefault(previous)
	})
	return handler
}

func (h *captureSlogHandler) findMessage(message string) (capturedLog, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, record := range h.records {
		if record.message == message {
			return record, true
		}
	}
	return capturedLog{}, false
}

func requireLogAttr(t *testing.T, record capturedLog, key string) slog.Value {
	t.Helper()
	value, ok := record.attrs[key]
	if !ok {
		t.Fatalf("expected log attr %q in %+v", key, record.attrs)
	}
	return value
}

func TestSummarizeOrdersTool_Success(t *testing.T) {
	fakeAPI := &fakeOrdersAPI{listOut: []clients.Order{
		{ID: "o1", Status: "paid", Total: 12999, CreatedAt: time.Now().Add(-48 * time.Hour)},
		{ID: "o2", Status: "paid", Total: 8900, CreatedAt: time.Now().Add(-24 * time.Hour)},
	}}
	fakeLLM := &summarizerLLM{reply: "You placed 2 orders totaling $218.99.", delay: time.Millisecond}
	recorder := &fakeSubLLMRecorder{}
	logs := captureSlog(t)
	tool := NewSummarizeOrdersToolWithRecorder(fakeAPI, fakeLLM, recorder)

	res, err := tool.Call(ctxWithJWT("tok"), json.RawMessage(`{}`), "user-1")
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	m := res.Content.(map[string]any)
	if m["summary"] != "You placed 2 orders totaling $218.99." {
		t.Errorf("summary = %+v", m)
	}
	if len(fakeLLM.seenMsg) == 0 {
		t.Error("expected at least one message sent to sub-LLM")
	}
	if len(recorder.calls) != 1 {
		t.Fatalf("metric calls = %d, want 1", len(recorder.calls))
	}
	if recorder.calls[0].tool != "summarize_orders" {
		t.Errorf("metric tool = %q", recorder.calls[0].tool)
	}
	if recorder.calls[0].dur <= 0 {
		t.Errorf("metric duration = %s, want positive", recorder.calls[0].dur)
	}
	record, ok := logs.findMessage("tool result")
	if !ok {
		t.Fatal("expected tool result log")
	}
	requireLogAttr(t, record, "duration_ms")
	requireLogAttr(t, record, "sub_llm_duration_ms")
}

func TestSummarizeOrdersTool_SubLLMErrorRecordsDuration(t *testing.T) {
	fakeAPI := &fakeOrdersAPI{listOut: []clients.Order{
		{ID: "o1", Status: "paid", Total: 12999, CreatedAt: time.Now().Add(-48 * time.Hour)},
	}}
	fakeLLM := &summarizerLLM{err: errors.New("ollama timeout"), delay: time.Millisecond}
	recorder := &fakeSubLLMRecorder{}
	logs := captureSlog(t)
	tool := NewSummarizeOrdersToolWithRecorder(fakeAPI, fakeLLM, recorder)

	_, err := tool.Call(ctxWithJWT("tok"), json.RawMessage(`{}`), "user-1")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "summarize_orders: sub-llm: ollama timeout") {
		t.Fatalf("error = %q", err.Error())
	}
	if len(recorder.calls) != 1 {
		t.Fatalf("metric calls = %d, want 1", len(recorder.calls))
	}
	if recorder.calls[0].tool != "summarize_orders" {
		t.Errorf("metric tool = %q", recorder.calls[0].tool)
	}
	if recorder.calls[0].dur <= 0 {
		t.Errorf("metric duration = %s, want positive", recorder.calls[0].dur)
	}
	record, ok := logs.findMessage("tool error")
	if !ok {
		t.Fatal("expected tool error log")
	}
	requireLogAttr(t, record, "error")
	requireLogAttr(t, record, "sub_llm_duration_ms")
}

func TestSummarizeOrdersTool_NoOrders(t *testing.T) {
	fakeAPI := &fakeOrdersAPI{listOut: nil}
	fakeLLM := &summarizerLLM{reply: "should not be called"}
	tool := NewSummarizeOrdersTool(fakeAPI, fakeLLM)

	res, err := tool.Call(ctxWithJWT("tok"), json.RawMessage(`{}`), "user-1")
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	m := res.Content.(map[string]any)
	if m["summary"] != "You have no orders yet." {
		t.Errorf("summary = %+v", m)
	}
	if fakeLLM.seenMsg != nil {
		t.Error("expected sub-LLM to be skipped on empty order list")
	}
}

func TestSummarizeOrdersTool_NoOrdersSkipsSubLLMMetric(t *testing.T) {
	fakeAPI := &fakeOrdersAPI{listOut: nil}
	fakeLLM := &summarizerLLM{reply: "should not be called"}
	recorder := &fakeSubLLMRecorder{}
	logs := captureSlog(t)
	tool := NewSummarizeOrdersToolWithRecorder(fakeAPI, fakeLLM, recorder)

	_, err := tool.Call(ctxWithJWT("tok"), json.RawMessage(`{}`), "user-1")
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if fakeLLM.seenMsg != nil {
		t.Error("expected sub-LLM to be skipped on empty order list")
	}
	if len(recorder.calls) != 0 {
		t.Fatalf("metric calls = %d, want 0", len(recorder.calls))
	}
	if record, ok := logs.findMessage("tool result"); ok {
		if _, exists := record.attrs["sub_llm_duration_ms"]; exists {
			t.Fatalf("empty-order log included sub_llm_duration_ms: %+v", record.attrs)
		}
	}
}

func TestSummarizeOrdersTool_RequiresUserID(t *testing.T) {
	tool := NewSummarizeOrdersTool(&fakeOrdersAPI{}, &summarizerLLM{})
	_, err := tool.Call(ctxWithJWT("tok"), json.RawMessage(`{}`), "")
	if err == nil {
		t.Fatal("expected error")
	}
}
