package content

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseFileExtractsBaseQuestionAndFollowUps(t *testing.T) {
	data := []byte(`# Go Language Fundamentals Rehearsal

Intro text.

### 3. How do maps work under concurrency?

Fast answer:

> A Go map is not safe for concurrent writes, or concurrent read/write access,
> without synchronization. I use sync.RWMutex or sharding and verify with race tests.

Follow-ups:

- When is sync.Map appropriate?
- How do you shard a map?
`)

	questions, err := ParseFile("02-go-language-fundamentals.md", data)
	if err != nil {
		t.Fatalf("ParseFile returned error: %v", err)
	}

	if len(questions) != 3 {
		t.Fatalf("expected 3 questions, got %d: %#v", len(questions), questions)
	}

	base := questions[0]
	if base.Topic != "Go Language Fundamentals Rehearsal" {
		t.Fatalf("expected topic from h1, got %q", base.Topic)
	}
	if base.Prompt != "How do maps work under concurrency?" {
		t.Fatalf("unexpected base prompt: %q", base.Prompt)
	}
	if base.ExpectedAnswer == "" {
		t.Fatal("expected base question answer to be captured")
	}
	if base.IsFollowUp {
		t.Fatal("base question marked as follow-up")
	}
	if base.Priority != 10 {
		t.Fatalf("expected high priority for 02 file, got %d", base.Priority)
	}
	if base.Tier != 1 {
		t.Fatalf("expected curated base question to be tier 1, got %d", base.Tier)
	}
	if base.Category != "golang" {
		t.Fatalf("expected Go category, got %q", base.Category)
	}
	if base.Kind != "qa" {
		t.Fatalf("expected qa kind, got %q", base.Kind)
	}

	if questions[1].Prompt != "When is sync.Map appropriate?" || !questions[1].IsFollowUp {
		t.Fatalf("unexpected first follow-up: %#v", questions[1])
	}
	if questions[1].Tier != 2 {
		t.Fatalf("expected follow-up to be tier 2, got %d", questions[1].Tier)
	}
	if questions[1].ExpectedAnswer != "" {
		t.Fatalf("follow-up should not have expected answer, got %q", questions[1].ExpectedAnswer)
	}
	if questions[2].Prompt != "How do you shard a map?" || !questions[2].IsFollowUp {
		t.Fatalf("unexpected second follow-up: %#v", questions[2])
	}
}

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

func TestParseDirReadsMarkdownFilesInSortedOrder(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"b.md": "# Topic B\n\n### 2. Second?\n\nFast answer:\n\n> B answer.\n",
		"a.md": "# Topic A\n\n### 1. First?\n\nFast answer:\n\n> A answer.\n",
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
		t.Fatalf("expected 2 questions, got %d", len(questions))
	}
	if questions[0].SourcePath != filepath.Join(dir, "a.md") {
		t.Fatalf("expected sorted a.md first, got %q", questions[0].SourcePath)
	}
	if questions[1].SourcePath != filepath.Join(dir, "b.md") {
		t.Fatalf("expected sorted b.md second, got %q", questions[1].SourcePath)
	}
}
