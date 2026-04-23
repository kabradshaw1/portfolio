package buildinfo

import (
	"log/slog"
	"runtime"
)

// Set via -ldflags at build time.
var (
	Version = "dev"
	GitSHA  = "unknown"
)

// Log emits a structured log line with build metadata. Call once at startup.
func Log() {
	slog.Info("service started",
		"version", Version,
		"gitSHA", GitSHA,
		"goVersion", runtime.Version(),
	)
}
