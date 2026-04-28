package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLI_NoArgs(t *testing.T) {
	cmd := exec.Command(buildBinary(t))
	if err := cmd.Run(); err == nil {
		t.Fatal("expected non-zero exit")
	} else if exitCode(err) != 2 {
		t.Fatalf("exit code: got %d want 2", exitCode(err))
	}
}

func TestCLI_CleanFile_ExitsZero(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	clean := filepath.Join(dir, "001_clean.up.sql")
	if err := os.WriteFile(clean, []byte("CREATE TABLE x (id int);\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, clean)
	if err := cmd.Run(); err != nil {
		t.Fatalf("expected clean run, got %v", err)
	}
}

func TestCLI_ReportsViolations_ExitOne(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	bad := filepath.Join(dir, "002_bad.up.sql")
	src := []byte("CREATE INDEX idx_x ON existing_table (a);\n")
	if err := os.WriteFile(bad, src, 0o600); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd := exec.Command(bin, bad)
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected non-zero exit")
	}
	if exitCode(err) != 1 {
		t.Fatalf("exit code: got %d want 1; stderr=%s", exitCode(err), stderr.String())
	}
	if !strings.Contains(stderr.String(), "MIG001") {
		t.Errorf("expected MIG001 in stderr, got %q", stderr.String())
	}
}

func TestCLI_ParseError_ExitTwo(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	broken := filepath.Join(dir, "003_broken.up.sql")
	if err := os.WriteFile(broken, []byte("CRATE TABBLE bad;"), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, broken)
	err := cmd.Run()
	if exitCode(err) != 2 {
		t.Fatalf("exit code: got %d want 2", exitCode(err))
	}
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	if e, ok := err.(*exec.ExitError); ok {
		return e.ExitCode()
	}
	return -1
}

func buildBinary(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "migration-lint")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("build failed: %v", err)
	}
	return bin
}
