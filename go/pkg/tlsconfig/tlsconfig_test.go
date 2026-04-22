package tlsconfig_test

import (
	"crypto/tls"
	"testing"
	"time"

	"github.com/kabradshaw1/portfolio/go/pkg/tlsconfig"
)

func TestServerTLS_LoadsCerts(t *testing.T) {
	dir := t.TempDir()
	generateTestCerts(t, dir)

	cfg, err := tlsconfig.ServerTLS(dir)
	if err != nil {
		t.Fatalf("ServerTLS: %v", err)
	}

	if cfg.ClientAuth != tls.RequireAndVerifyClientCert {
		t.Fatalf("expected RequireAndVerifyClientCert, got %v", cfg.ClientAuth)
	}

	cert, err := cfg.GetCertificate(nil)
	if err != nil {
		t.Fatalf("GetCertificate: %v", err)
	}

	if cert == nil {
		t.Fatal("GetCertificate returned nil")
	}
}

func TestClientTLS_LoadsCerts(t *testing.T) {
	dir := t.TempDir()
	generateTestCerts(t, dir)

	creds, err := tlsconfig.ClientTLS(dir)
	if err != nil {
		t.Fatalf("ClientTLS: %v", err)
	}

	if creds == nil {
		t.Fatal("ClientTLS returned nil credentials")
	}

	if info := creds.Info(); info.SecurityProtocol != "tls" {
		t.Fatalf("expected 'tls', got %q", info.SecurityProtocol)
	}
}

func TestServerTLS_MissingDir_ReturnsError(t *testing.T) {
	_, err := tlsconfig.ServerTLS("/nonexistent/path")
	if err == nil {
		t.Fatal("expected error for missing cert dir")
	}
}

func TestClientTLS_MissingDir_ReturnsError(t *testing.T) {
	_, err := tlsconfig.ClientTLS("/nonexistent/path")
	if err == nil {
		t.Fatal("expected error for missing cert dir")
	}
}

func TestWatch_ReloadsOnChange(t *testing.T) {
	dir := t.TempDir()
	generateTestCerts(t, dir)

	certPtr, _, err := tlsconfig.Load(dir)
	if err != nil {
		t.Fatal(err)
	}

	originalCert := certPtr.Load()

	stop, err := tlsconfig.Watch(dir, certPtr)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	defer stop()

	// Overwrite certs with new ones
	generateTestCerts(t, dir)

	// Wait for watcher debounce (500ms) + margin
	time.Sleep(1 * time.Second)

	newCert := certPtr.Load()
	if newCert == originalCert {
		t.Fatal("expected cert pointer to change after file write")
	}
}
