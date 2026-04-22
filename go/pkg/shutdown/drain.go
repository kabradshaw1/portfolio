package shutdown

import (
	"context"
	"log/slog"
	"time"

	"google.golang.org/grpc"
)

// DrainHTTP returns a shutdown hook that gracefully drains an HTTP server.
// It stops accepting new connections and waits for in-flight requests to
// complete, up to the context deadline.
func DrainHTTP(name string, srv interface{ Shutdown(ctx context.Context) error }) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		slog.Info("draining HTTP server", "name", name)
		return srv.Shutdown(ctx)
	}
}

// DrainGRPC returns a shutdown hook that gracefully drains a gRPC server.
// It stops accepting new RPCs and waits for active RPCs to finish. Falls
// back to hard Stop() if the context deadline expires.
func DrainGRPC(name string, srv *grpc.Server) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		slog.Info("draining gRPC server", "name", name)
		done := make(chan struct{})
		go func() {
			srv.GracefulStop()
			close(done)
		}()
		select {
		case <-done:
			return nil
		case <-ctx.Done():
			slog.Warn("gRPC drain timeout, forcing stop", "name", name)
			srv.Stop()
			return ctx.Err()
		}
	}
}

// WaitForInflight returns a shutdown hook that polls idle() until it
// returns true (no in-flight work) or the context expires. Use this to
// let saga handlers and Kafka consumers finish processing their current
// message before closing connections.
func WaitForInflight(name string, idle func() bool, pollInterval time.Duration) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		slog.Info("waiting for in-flight work to complete", "name", name)
		for {
			if idle() {
				slog.Info("in-flight work complete", "name", name)
				return nil
			}
			select {
			case <-ctx.Done():
				slog.Warn("in-flight wait timeout", "name", name)
				return ctx.Err()
			case <-time.After(pollInterval):
			}
		}
	}
}
