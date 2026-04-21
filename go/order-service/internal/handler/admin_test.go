package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kabradshaw1/portfolio/go/order-service/internal/saga"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
)

type mockDLQ struct {
	messages  []saga.DLQMessage
	replayErr error
}

func (m *mockDLQ) List(limit int) ([]saga.DLQMessage, error) {
	if limit > len(m.messages) {
		return m.messages, nil
	}
	return m.messages[:limit], nil
}

func (m *mockDLQ) Replay(index int) (*saga.DLQMessage, error) {
	if m.replayErr != nil {
		return nil, m.replayErr
	}
	if index >= len(m.messages) {
		return nil, fmt.Errorf("index %d out of range", index)
	}
	return &m.messages[index], nil
}

func setupAdminRouter(dlq DLQLister) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(apperror.ErrorHandler())
	h := NewAdminHandler(dlq)
	admin := r.Group("/admin")
	{
		admin.GET("/dlq/messages", h.ListDLQ)
		admin.POST("/dlq/replay", h.ReplayDLQ)
	}
	return r
}

func TestListDLQ_Empty(t *testing.T) {
	router := setupAdminRouter(&mockDLQ{})

	req, _ := http.NewRequest("GET", "/admin/dlq/messages", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Messages []saga.DLQMessage `json:"messages"`
		Count    int               `json:"count"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Count != 0 {
		t.Errorf("expected count=0, got %d", resp.Count)
	}
}

func TestListDLQ_WithMessages(t *testing.T) {
	dlq := &mockDLQ{
		messages: []saga.DLQMessage{
			{Index: 0, RoutingKey: "saga.cart.commands", Exchange: "ecommerce.saga", Timestamp: time.Now(), Body: json.RawMessage(`{"command":"reserve.items"}`)},
		},
	}
	router := setupAdminRouter(dlq)

	req, _ := http.NewRequest("GET", "/admin/dlq/messages?limit=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Count int `json:"count"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Count != 1 {
		t.Errorf("expected count=1, got %d", resp.Count)
	}
}

func TestReplayDLQ_Success(t *testing.T) {
	dlq := &mockDLQ{
		messages: []saga.DLQMessage{
			{Index: 0, RoutingKey: "saga.cart.commands", Exchange: "ecommerce.saga", Body: json.RawMessage(`{}`)},
		},
	}
	router := setupAdminRouter(dlq)

	body := `{"index": 0}`
	req, _ := http.NewRequest("POST", "/admin/dlq/replay", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestReplayDLQ_NotFound(t *testing.T) {
	dlq := &mockDLQ{replayErr: fmt.Errorf("index 5 not found")}
	router := setupAdminRouter(dlq)

	body := `{"index": 5}`
	req, _ := http.NewRequest("POST", "/admin/dlq/replay", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestReplayDLQ_InvalidBody(t *testing.T) {
	router := setupAdminRouter(&mockDLQ{})

	req, _ := http.NewRequest("POST", "/admin/dlq/replay", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}
