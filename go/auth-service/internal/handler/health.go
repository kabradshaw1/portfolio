package handler

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/gin-gonic/gin"
)

const readinessTimeout = 2 * time.Second

type dbPinger interface {
	Ping(ctx context.Context) error
}

type dnsLookupFunc func(ctx context.Context, host string) ([]net.IPAddr, error)

type HealthHandler struct {
	db           dbPinger
	oauthHosts   []string
	lookupIPAddr dnsLookupFunc
}

func NewHealthHandler(db dbPinger, oauthHosts []string) *HealthHandler {
	return &HealthHandler{
		db:           db,
		oauthHosts:   oauthHosts,
		lookupIPAddr: net.DefaultResolver.LookupIPAddr,
	}
}

func (h *HealthHandler) Health(c *gin.Context) {
	if err := h.db.Ping(c.Request.Context()); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unhealthy", "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "healthy"})
}

func (h *HealthHandler) Ready(c *gin.Context) {
	checks := gin.H{}
	ready := true

	if err := h.db.Ping(c.Request.Context()); err != nil {
		checks["postgres"] = "unhealthy"
		ready = false
	} else {
		checks["postgres"] = "healthy"
	}

	dnsCtx, cancel := context.WithTimeout(c.Request.Context(), readinessTimeout)
	defer cancel()

	dnsChecks := gin.H{}
	for _, host := range h.oauthHosts {
		if host == "" {
			continue
		}
		if _, err := h.lookupIPAddr(dnsCtx, host); err != nil {
			dnsChecks[host] = "unhealthy"
			ready = false
			continue
		}
		dnsChecks[host] = "healthy"
	}
	checks["oauth_dns"] = dnsChecks

	status := http.StatusOK
	bodyStatus := "ready"
	if !ready {
		status = http.StatusServiceUnavailable
		bodyStatus = "not_ready"
	}

	c.JSON(status, gin.H{"status": bodyStatus, "checks": checks})
}

func OAuthDependencyHosts(tokenURL, userinfoURL string) []string {
	hostsByName := make(map[string]struct{}, 2)
	hosts := make([]string, 0, 2)
	for _, rawURL := range []string{tokenURL, userinfoURL} {
		parsed, err := url.Parse(rawURL)
		if err != nil || parsed.Hostname() == "" {
			continue
		}
		host := parsed.Hostname()
		if _, ok := hostsByName[host]; ok {
			continue
		}
		hostsByName[host] = struct{}{}
		hosts = append(hosts, host)
	}
	return hosts
}
