//go:build integration

package testutil

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// DoRequest fires an HTTP request against the provided gin router and returns
// the recorded response. body may be empty for requests without a payload.
func DoRequest(
	t *testing.T,
	router *gin.Engine,
	method, path, body string,
	headers map[string]string,
) *httptest.ResponseRecorder {
	t.Helper()

	var bodyReader *strings.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	} else {
		bodyReader = strings.NewReader("")
	}

	req, err := http.NewRequest(method, path, bodyReader)
	if err != nil {
		t.Fatalf("create request %s %s: %v", method, path, err)
	}

	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// ParseJSON unmarshals the recorder's response body into target. The test
// fails immediately on any decode error.
func ParseJSON(t *testing.T, w *httptest.ResponseRecorder, target any) {
	t.Helper()

	if err := json.NewDecoder(w.Body).Decode(target); err != nil {
		t.Fatalf("parse response JSON (status=%d body=%q): %v", w.Code, w.Body.String(), err)
	}
}
