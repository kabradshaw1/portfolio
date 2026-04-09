package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"github.com/kabradshaw1/portfolio/go/ai-service/internal/agent"
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/llm"
)

type fakeRunner struct {
	events []agent.Event
	err    error
	onRun  func(ctx context.Context, turn agent.Turn)
}

func (f *fakeRunner) Run(ctx context.Context, turn agent.Turn, emit func(agent.Event)) error {
	if f.onRun != nil {
		f.onRun(ctx, turn)
	}
	for _, e := range f.events {
		emit(e)
	}
	return f.err
}

func TestChatHandler_StreamsEventsAsSSE(t *testing.T) {
	gin.SetMode(gin.TestMode)
	runner := &fakeRunner{events: []agent.Event{
		{ToolCall: &agent.ToolCallEvent{Name: "search_products", Args: json.RawMessage(`{"query":"jacket"}`)}},
		{ToolResult: &agent.ToolResultEvent{Name: "search_products", Display: map[string]any{"kind": "product_list"}}},
		{Final: &agent.FinalEvent{Text: "Here are some jackets."}},
	}}
	r := gin.New()
	RegisterChatRoutes(r, runner, "")

	body := strings.NewReader(`{"messages":[{"role":"user","content":"find a jacket"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/chat", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Errorf("Content-Type = %q, expected text/event-stream", ct)
	}
	out := w.Body.String()
	for _, want := range []string{
		"event: tool_call",
		`"name":"search_products"`,
		"event: tool_result",
		"event: final",
		`"text":"Here are some jackets."`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("response missing %q:\n%s", want, out)
		}
	}
}

func TestChatHandler_BadBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterChatRoutes(r, &fakeRunner{}, "")
	req := httptest.NewRequest(http.MethodPost, "/chat", strings.NewReader(`not json`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestChatHandler_AcceptsValidJWT(t *testing.T) {
	gin.SetMode(gin.TestMode)
	secret := "dev-secret-key-at-least-32-characters-long"
	tok := mintChatTestToken(t, secret, "user-abc", time.Hour)

	var capturedTurn agent.Turn
	var capturedJWT string
	runner := &fakeRunner{events: []agent.Event{{Final: &agent.FinalEvent{Text: "ok"}}}}
	runner.onRun = func(ctx context.Context, turn agent.Turn) {
		capturedTurn = turn
		capturedJWT = JWTFromContext(ctx)
	}

	r := gin.New()
	RegisterChatRoutes(r, runner, secret)
	req := httptest.NewRequest(http.MethodPost, "/chat",
		strings.NewReader(`{"messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	if capturedTurn.UserID != "user-abc" {
		t.Errorf("turn.UserID = %q", capturedTurn.UserID)
	}
	if capturedJWT != tok {
		t.Errorf("ctx jwt = %q", capturedJWT)
	}
}

func TestChatHandler_RejectsInvalidJWT(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterChatRoutes(r, &fakeRunner{}, "dev-secret-key-at-least-32-characters-long")

	req := httptest.NewRequest(http.MethodPost, "/chat",
		strings.NewReader(`{"messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer not-a-real-token")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// mintChatTestToken mirrors the helper in auth/jwt_test.go.
func mintChatTestToken(t *testing.T, secret, sub string, d time.Duration) string {
	t.Helper()
	claims := jwt.MapClaims{"sub": sub, "exp": time.Now().Add(d).Unix()}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, err := tok.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return s
}

var _ = llm.RoleUser // keep import for future tests
