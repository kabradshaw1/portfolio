package content

import (
	"os"
	"path/filepath"
	"strings"
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

func TestParseDirExcludesCodingExerciseMaterial(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"02-go-language-fundamentals.md": "# Go Language Fundamentals\n\n### 1. How do maps work under concurrency?\n\nFast answer:\n\n> Use synchronization.\n",
		"08-coding-exercises.md":         "# Timed Coding Exercises\n\n### 1. Worker pool with cancellation\n\nPrompt:\n\n> Write a worker pool.\n\nFast design:\n\n> Use workers and channels.\n",
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
		t.Fatalf("expected exactly 1 QA question, got %d: %#v", len(questions), questions)
	}
	if questions[0].Category == "coding" {
		t.Fatalf("ParseDir imported coding category material: %#v", questions[0])
	}
	if questions[0].Kind == "coding_exercise" {
		t.Fatalf("ParseDir imported coding exercise material: %#v", questions[0])
	}
}

func TestParseDirIgnoresRoleMap(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"00-role-map.md":                "# Role Map\n\n### 1. Should this import?\n\nFast answer:\n\n> No.\n",
		"01-portfolio-recall-matrix.md": "# Portfolio Recall Matrix\n\n### Tell me about a Go backend system you built.\n\nFast answer:\n\n> I built decomposed Go services.\n",
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
		t.Fatalf("expected only portfolio question, got %d: %#v", len(questions), questions)
	}
	if filepath.Base(questions[0].SourcePath) == "00-role-map.md" {
		t.Fatalf("role map imported as QA: %#v", questions[0])
	}
}

func TestParseFileSkipsCodingExercisesSection(t *testing.T) {
	data := []byte(`# REST API And API Gateway Rehearsal

### 1. How do you design a good REST API?

Fast answer:

> Use resource-oriented routes and consistent errors.

## Coding Exercises

### Exercise 1: Idempotent Create Endpoint

Prompt:

> Implement an endpoint.

Fast design:

> Use an idempotency key.
`)

	questions, err := ParseFile("03-rest-api-gateway-questions.md", data)
	if err != nil {
		t.Fatalf("ParseFile returned error: %v", err)
	}
	if len(questions) != 1 {
		t.Fatalf("expected only QA prompt, got %d: %#v", len(questions), questions)
	}
	if strings.Contains(questions[0].Prompt, "Idempotent Create Endpoint") {
		t.Fatalf("coding exercise leaked into QA: %#v", questions[0])
	}
}

func TestParseFileExtractsRepoAnchorsAndFollowUpAnswer(t *testing.T) {
	data := []byte(`# Go Language Fundamentals Rehearsal

### 3. How do maps work under concurrency?

Fast answer:

> A Go map is not safe for concurrent writes without synchronization.

Repo anchors:
- ` + "`" + `go/pkg/resilience` + "`" + ` - Shows shared concurrency-safe helpers.

Follow-ups:

#### Follow-up: When is sync.Map appropriate?

Fast answer:

> sync.Map fits read-heavy shared maps with stable keys or disjoint key ownership.

Repo anchors:
- ` + "`" + `go/analytics-service` + "`" + ` - Shows concurrent consumer state that should stay explicit.
`)

	questions, err := ParseFile("02-go-language-fundamentals.md", data)
	if err != nil {
		t.Fatalf("ParseFile returned error: %v", err)
	}
	if len(questions) != 2 {
		t.Fatalf("expected base plus follow-up, got %d: %#v", len(questions), questions)
	}
	base := questions[0]
	if len(base.RepoAnchors) != 1 || base.RepoAnchors[0].Path != "go/pkg/resilience" {
		t.Fatalf("unexpected base anchors: %#v", base.RepoAnchors)
	}
	follow := questions[1]
	if !follow.IsFollowUp {
		t.Fatalf("expected follow-up: %#v", follow)
	}
	if follow.ParentPrompt != "How do maps work under concurrency?" {
		t.Fatalf("unexpected parent prompt: %q", follow.ParentPrompt)
	}
	if follow.ExpectedAnswer == "" {
		t.Fatalf("expected follow-up answer to be parsed")
	}
	if len(follow.RepoAnchors) != 1 || follow.RepoAnchors[0].Path != "go/analytics-service" {
		t.Fatalf("unexpected follow-up anchors: %#v", follow.RepoAnchors)
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
