package management

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunnerExecutesCatalogScriptAndBoundsOutput(t *testing.T) {
	repo := t.TempDir()
	script := filepath.Join(repo, "scripts/ops/test.sh")
	if err := os.MkdirAll(filepath.Dir(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(script, []byte("#!/usr/bin/env bash\nprintf 'abcdef'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	runner := Runner{RepoRoot: repo, MaxOutputBytes: 4, MaxTimeout: time.Second}
	result := runner.Run(context.Background(), Action{ID: "test", ScriptPath: "scripts/ops/test.sh", Timeout: time.Second})
	if result.Status != StatusSucceeded {
		t.Fatalf("status = %q stderr=%q", result.Status, result.Stderr)
	}
	if result.Stdout != "abcd" || !result.OutputTruncated {
		t.Fatalf("stdout=%q truncated=%t", result.Stdout, result.OutputTruncated)
	}
}

func TestRunnerRejectsUnsafeScriptPath(t *testing.T) {
	runner := Runner{RepoRoot: t.TempDir(), MaxOutputBytes: 1024, MaxTimeout: time.Second}
	result := runner.Run(context.Background(), Action{ID: "bad", ScriptPath: "../bad.sh", Timeout: time.Second})
	if result.Status != StatusFailed || !strings.Contains(result.Stderr, "unsafe script path") {
		t.Fatalf("result = %+v", result)
	}
}

func TestRunnerRedactsSecretLookingOutput(t *testing.T) {
	repo := t.TempDir()
	script := filepath.Join(repo, "scripts/ops/secret.sh")
	if err := os.MkdirAll(filepath.Dir(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(script, []byte("#!/usr/bin/env bash\necho 'token=super-secret-value'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	runner := Runner{RepoRoot: repo, MaxOutputBytes: 1024, MaxTimeout: time.Second}
	result := runner.Run(context.Background(), Action{ID: "secret", ScriptPath: "scripts/ops/secret.sh", Timeout: time.Second})
	if strings.Contains(result.Stdout, "super-secret-value") {
		t.Fatalf("secret was not redacted: %q", result.Stdout)
	}
	if result.RedactionsApplied == 0 {
		t.Fatalf("expected redaction count, got %+v", result)
	}
}

func TestRunnerTimesOut(t *testing.T) {
	repo := t.TempDir()
	script := filepath.Join(repo, "scripts/ops/slow.sh")
	if err := os.MkdirAll(filepath.Dir(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(script, []byte("#!/usr/bin/env bash\nsleep 2\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	runner := Runner{RepoRoot: repo, MaxOutputBytes: 1024, MaxTimeout: time.Second}
	result := runner.Run(context.Background(), Action{ID: "slow", ScriptPath: "scripts/ops/slow.sh", Timeout: 10 * time.Millisecond})
	if result.Status != StatusTimedOut {
		t.Fatalf("status = %q, want timed_out", result.Status)
	}
}
