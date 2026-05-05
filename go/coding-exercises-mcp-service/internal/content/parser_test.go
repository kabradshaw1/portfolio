package content

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseFileAssignsTierOneToCodingExercisePrompts(t *testing.T) {
	data := []byte(`# Timed Coding Exercises

### 3. Worker pool with cancellation

Prompt:

> Write a worker pool that processes jobs from a channel, stops on context
> cancellation, returns results, and does not leak goroutines.

Fast design:

> Use a parent context, bounded job channel, result channel, and sync.WaitGroup.

Follow-ups:

- Who closes the jobs channel?
`)

	questions, err := ParseFile("08-coding-exercises.md", data)
	if err != nil {
		t.Fatalf("ParseFile returned error: %v", err)
	}
	if len(questions) != 2 {
		t.Fatalf("expected 2 questions, got %d: %#v", len(questions), questions)
	}
	if questions[0].Prompt != "Write a worker pool that processes jobs from a channel, stops on context cancellation, returns results, and does not leak goroutines." {
		t.Fatalf("expected coding exercise prompt body, got %q", questions[0].Prompt)
	}
	if questions[0].Tier != 1 {
		t.Fatalf("expected coding exercise prompt to be tier 1, got %d", questions[0].Tier)
	}
	if questions[0].Category != "coding" {
		t.Fatalf("expected coding category, got %q", questions[0].Category)
	}
	if questions[0].Kind != "coding_exercise" {
		t.Fatalf("expected coding exercise kind, got %q", questions[0].Kind)
	}
	if questions[1].Tier != 2 {
		t.Fatalf("expected coding exercise follow-up to be tier 2, got %d", questions[1].Tier)
	}
	if questions[1].Kind != "qa" {
		t.Fatalf("expected coding follow-up to use qa kind, got %q", questions[1].Kind)
	}
}

func TestParseDirImportsAllMarkdownFilesInSortedOrder(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"09-concurrency.md": "# Concurrency\n\n### 1. Worker pool\n\nPrompt:\n\n> Build a worker pool.\n\nFast design:\n\n> Use workers and cancellation.\n",
		"08-coding-exercises.md": `# Timed

### 1. Retry client

Prompt:

> Build a retry client.

Fast design:

> Use bounded retries.
`,
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatalf("write fixture %s: %v", name, err)
		}
	}

	questions, err := ParseDir(dir)
	if err != nil {
		t.Fatalf("ParseDir returned error: %v", err)
	}
	if len(questions) != 2 {
		t.Fatalf("expected two exercises, got %d: %#v", len(questions), questions)
	}
	if filepath.Base(questions[0].SourcePath) != "08-coding-exercises.md" {
		t.Fatalf("expected lexical order, got %q", questions[0].SourcePath)
	}
}

func TestParseFileExtractsCodingRepoAnchors(t *testing.T) {
	data := []byte(`# Timed Coding Exercises

### 5. Retryable HTTP client

Prompt:

> Build a client that retries retryable errors and respects context cancellation.

Fast design:

> Use context-aware attempts and backoff.

Repo anchors:
- ` + "`go/pkg/resilience`" + ` - Contains retry helpers used by services.
`)

	questions, err := ParseFile("08-coding-exercises.md", data)
	if err != nil {
		t.Fatalf("ParseFile returned error: %v", err)
	}
	if len(questions) != 1 {
		t.Fatalf("expected one exercise, got %d: %#v", len(questions), questions)
	}
	if len(questions[0].RepoAnchors) != 1 || questions[0].RepoAnchors[0].Path != "go/pkg/resilience" {
		t.Fatalf("unexpected anchors: %#v", questions[0].RepoAnchors)
	}
}
