// Package tlsconfig provides mTLS configuration helpers for Go services.
// It loads TLS certificates from a directory (ca.crt, tls.crt, tls.key),
// supports hot-reload via fsnotify file watching, and produces tls.Config
// for servers and gRPC TransportCredentials for clients.
package tlsconfig

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
	"google.golang.org/grpc/credentials"
)

const watchDebounce = 500 * time.Millisecond // coalesce rapid file events from cert rotation

// ServerTLS loads certs from certDir and returns a *tls.Config for mTLS servers.
// Uses GetCertificate with an atomic pointer for hot reload via Watch.
func ServerTLS(certDir string) (*tls.Config, error) {
	certPtr, caPool, err := Load(certDir)
	if err != nil {
		return nil, fmt.Errorf("server tls: %w", err)
	}

	return &tls.Config{
		GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
			return certPtr.Load(), nil
		},
		ClientCAs:  caPool,
		ClientAuth: tls.RequireAndVerifyClientCert,
		MinVersion: tls.VersionTLS13,
	}, nil
}

// ClientTLS loads certs from certDir and returns gRPC TransportCredentials for mTLS clients.
func ClientTLS(certDir string) (credentials.TransportCredentials, error) {
	certPtr, caPool, err := Load(certDir)
	if err != nil {
		return nil, fmt.Errorf("client tls: %w", err)
	}

	cfg := &tls.Config{
		GetClientCertificate: func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
			return certPtr.Load(), nil
		},
		RootCAs:    caPool,
		MinVersion: tls.VersionTLS13,
	}

	return credentials.NewTLS(cfg), nil
}

// Load reads tls.crt, tls.key, ca.crt from certDir and returns an atomic
// pointer to the certificate and a CA pool. The atomic pointer enables
// lock-free hot reload when paired with Watch.
func Load(certDir string) (*atomic.Pointer[tls.Certificate], *x509.CertPool, error) {
	certFile := filepath.Join(certDir, "tls.crt")
	keyFile := filepath.Join(certDir, "tls.key")
	caFile := filepath.Join(certDir, "ca.crt")

	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, nil, fmt.Errorf("load cert/key: %w", err)
	}

	caData, err := os.ReadFile(caFile)
	if err != nil {
		return nil, nil, fmt.Errorf("read ca cert: %w", err)
	}

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caData) {
		return nil, nil, fmt.Errorf("failed to parse CA cert")
	}

	var ptr atomic.Pointer[tls.Certificate]
	ptr.Store(&cert)

	return &ptr, caPool, nil
}

// Watch monitors certDir for file changes and reloads the cert into certPtr.
// It debounces with a 500ms timer to coalesce rapid writes from cert-manager
// rotation. Returns a stop function that shuts down the watcher goroutine.
func Watch(certDir string, certPtr *atomic.Pointer[tls.Certificate]) (stop func(), err error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("create watcher: %w", err)
	}

	if err := watcher.Add(certDir); err != nil {
		watcher.Close()
		return nil, fmt.Errorf("watch %s: %w", certDir, err)
	}

	done := make(chan struct{})

	go func() {
		defer watcher.Close()

		var debounce *time.Timer

		for {
			select {
			case <-done:
				if debounce != nil {
					debounce.Stop()
				}
				return
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}

				if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
					if debounce != nil {
						debounce.Stop()
					}

					debounce = time.AfterFunc(watchDebounce, func() {
						reload(certDir, certPtr)
					})
				}
			case watchErr, ok := <-watcher.Errors:
				if !ok {
					return
				}

				slog.Error("tls watcher error", "error", watchErr)
			}
		}
	}()

	return func() { close(done) }, nil
}

func reload(certDir string, certPtr *atomic.Pointer[tls.Certificate]) {
	certFile := filepath.Join(certDir, "tls.crt")
	keyFile := filepath.Join(certDir, "tls.key")

	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		slog.Error("tls reload failed", "error", err)
		return
	}

	certPtr.Store(&cert)
	slog.Info("tls certificate reloaded", "certDir", certDir)
}
