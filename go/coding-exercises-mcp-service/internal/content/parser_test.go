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

func TestParseDirIncludesOnlyCodingExerciseMaterial(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"02-go-language-fundamentals.md": "# Go\n\n### 1. Maps?\n\nFast answer:\n\n> Use synchronization.\n",
		"08-coding-exercises.md": `# Timed Coding Exercises

### 1. Worker pool

Prompt:

> Build a worker pool.

Fast design:

> Use goroutines and a wait group.
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
	if len(questions) != 1 {
		t.Fatalf("expected only the coding exercise question, got %d: %#v", len(questions), questions)
	}
	if questions[0].Kind != "coding_exercise" {
		t.Fatalf("expected coding exercise kind, got %q", questions[0].Kind)
	}
	if questions[0].Category != "coding" {
		t.Fatalf("expected coding category, got %q", questions[0].Category)
	}
}
