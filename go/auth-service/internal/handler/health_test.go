package handler

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

type fakePinger struct {
	err error
}

func (f fakePinger) Ping(context.Context) error {
	return f.err
}

func TestReady_HealthyWhenDatabaseAndOAuthDNSResolve(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewHealthHandler(fakePinger{}, []string{"oauth2.googleapis.com", "www.googleapis.com"})
	handler.lookupIPAddr = func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("203.0.113.10")}}, nil
	}

	router := gin.New()
	router.GET("/ready", handler.Ready)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d with body %s", http.StatusOK, rec.Code, rec.Body.String())
	}
}

func TestReady_UnhealthyWhenOAuthDNSFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewHealthHandler(fakePinger{}, []string{"oauth2.googleapis.com"})
	handler.lookupIPAddr = func(context.Context, string) ([]net.IPAddr, error) {
		return nil, errors.New("lookup failed")
	}

	router := gin.New()
	router.GET("/ready", handler.Ready)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d with body %s", http.StatusServiceUnavailable, rec.Code, rec.Body.String())
	}
}

func TestOAuthDependencyHosts_DeduplicatesParsedHosts(t *testing.T) {
	hosts := OAuthDependencyHosts(
		"https://oauth2.googleapis.com/token",
		"https://oauth2.googleapis.com/oauth2/v3/userinfo",
	)

	if len(hosts) != 1 || hosts[0] != "oauth2.googleapis.com" {
		t.Fatalf("expected deduped oauth2.googleapis.com host, got %#v", hosts)
	}
}
