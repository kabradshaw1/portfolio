package content

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var (
	numberedHeading = regexp.MustCompile(`^#+\s+\d+\.\s+(.+)$`)
	exerciseHeading = regexp.MustCompile(`^#+\s+Exercise\s+\d+:\s+(.+)$`)
	scenarioHeading = regexp.MustCompile(`^#+\s+Scenario(?:\s+\d+)?:\s+(.+)$`)
)

// Question is one imported coding exercise prompt from the interview-prep material.
type Question struct {
	SourcePath     string
	Topic          string
	Category       string
	Kind           string
	Prompt         string
	ExpectedAnswer string
	IsFollowUp     bool
	ParentPrompt   string
	RepoAnchors    []RepoAnchor
	Priority       int
	Tier           int
}

type RepoAnchor struct {
	Path string
	Note string
}

// ParseDir parses coding exercise markdown files under root.
func ParseDir(root string) ([]Question, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("read material dir: %w", err)
	}

	var paths []string
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		paths = append(paths, filepath.Join(root, entry.Name()))
	}
	sort.Strings(paths)

	var questions []Question
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read material file %s: %w", path, err)
		}
		parsed, err := ParseFile(path, data)
		if err != nil {
			return nil, err
		}
		questions = append(questions, parsed...)
	}
	return questions, nil
}

// ParseFile extracts base questions, fast answers, and follow-up prompts.
func ParseFile(path string, data []byte) ([]Question, error) {
	lines := strings.Split(string(bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))), "\n")

	var questions []Question
	var topic string
	var current *Question
	var lastBasePrompt string
	var lastBaseAnchors []RepoAnchor
	var promptLines []string
	var answerLines []string
	var anchorLines []string
	mode := ""

	flush := func() {
		if current == nil {
			return
		}
		if prompt := cleanPrompt(promptLines); prompt != "" {
			current.Prompt = prompt
			current.Tier = tierForQuestion(current.SourcePath, current.Prompt, current.IsFollowUp)
		}
		current.ExpectedAnswer = cleanAnswer(answerLines)
		var anchors []RepoAnchor
		for _, raw := range anchorLines {
			if anchor, ok := cleanAnchorLine(raw); ok {
				anchors = append(anchors, anchor)
			}
		}
		if len(anchors) == 0 && current.IsFollowUp {
			anchors = cloneAnchors(lastBaseAnchors)
		}
		current.RepoAnchors = anchors
		if current.Prompt != "" {
			questions = append(questions, *current)
			if !current.IsFollowUp {
				lastBasePrompt = current.Prompt
				lastBaseAnchors = cloneAnchors(current.RepoAnchors)
			}
		}
		current = nil
		promptLines = nil
		answerLines = nil
		anchorLines = nil
		mode = ""
	}

	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "# ") {
			topic = strings.TrimSpace(strings.TrimPrefix(line, "# "))
			continue
		}

		if strings.HasPrefix(line, "#### Follow-up: ") {
			flush()
			prompt := strings.TrimSpace(strings.TrimPrefix(line, "#### Follow-up: "))
			current = &Question{
				SourcePath:   path,
				Topic:        topic,
				Category:     categoryForPath(path),
				Kind:         kindForQuestion(path, true),
				Prompt:       prompt,
				IsFollowUp:   true,
				ParentPrompt: lastBasePrompt,
				Priority:     priorityForPath(path),
				Tier:         tierForQuestion(path, prompt, true),
			}
			continue
		}

		if prompt, ok := questionHeading(line); ok {
			flush()
			current = &Question{
				SourcePath: path,
				Topic:      topic,
				Category:   categoryForPath(path),
				Kind:       kindForQuestion(path, false),
				Prompt:     prompt,
				Priority:   priorityForPath(path),
				Tier:       tierForQuestion(path, prompt, false),
			}
			continue
		}

		switch {
		case line == "Repo anchors:":
			mode = "anchors"
		case line == "Prompt:":
			mode = "prompt"
		case line == "Fast answer:" || line == "Fast design:" || line == "Expected discussion:" || line == "What to say while coding:":
			if prompt := cleanPrompt(promptLines); current != nil && prompt != "" {
				current.Prompt = prompt
				current.Tier = tierForQuestion(current.SourcePath, current.Prompt, current.IsFollowUp)
				promptLines = nil
			}
			mode = "answer"
		case mode == "prompt" && current != nil:
			promptLines = append(promptLines, raw)
		case mode == "anchors" && current != nil && strings.HasPrefix(line, "- "):
			anchorLines = append(anchorLines, raw)
		case line == "Follow-ups:":
			flush()
			mode = "followups"
		case mode == "answer" && current != nil:
			answerLines = append(answerLines, raw)
		case mode == "followups" && strings.HasPrefix(line, "- "):
			prompt := strings.TrimSpace(strings.TrimPrefix(line, "- "))
			if prompt != "" {
				questions = append(questions, Question{
					SourcePath:   path,
					Topic:        topic,
					Category:     categoryForPath(path),
					Kind:         kindForQuestion(path, true),
					Prompt:       prompt,
					IsFollowUp:   true,
					ParentPrompt: lastBasePrompt,
					RepoAnchors:  cloneAnchors(lastBaseAnchors),
					Priority:     priorityForPath(path),
					Tier:         tierForQuestion(path, prompt, true),
				})
			}
		}
	}
	flush()

	return questions, nil
}

func tierForQuestion(path, prompt string, followUp bool) int {
	if followUp {
		return 2
	}
	name := filepath.Base(path)
	if tierOneQuestions[prompt] {
		return 1
	}
	if name == "08-coding-exercises.md" {
		return 1
	}
	if strings.Contains(prompt, "Scenario") || strings.Contains(prompt, "Exercise") {
		return 2
	}
	return 3
}

func kindForQuestion(path string, followUp bool) string {
	if followUp {
		return "qa"
	}
	return "coding_exercise"
}

func categoryForPath(path string) string {
	switch filepath.Base(path) {
	case "01-portfolio-recall-matrix.md":
		return "portfolio"
	case "02-go-language-fundamentals.md":
		return "golang"
	case "03-rest-api-gateway-questions.md":
		return "api"
	case "04-third-party-integrations.md":
		return "integrations"
	case "05-distributed-systems-scalability.md":
		return "distributed"
	case "06-ai-agent-systems.md":
		return "ai"
	case "07-database-observability-security.md":
		return "db_observability_security"
	case "08-coding-exercises.md":
		return "coding"
	case "09-go-performance-and-concurrency-drills.md":
		return "performance_concurrency"
	case "10-mock-interview-drills.md":
		return "mock_interview"
	default:
		return "general"
	}
}

var tierOneQuestions = map[string]bool{
	"Tell me about a Go backend system you built.":                    true,
	"Tell me about a distributed workflow you designed.":              true,
	"Tell me about your AI agent experience.":                         true,
	"How do you handle third-party API failures?":                     true,
	"How do you debug high latency across services?":                  true,
	"How do maps work under concurrency?":                             true,
	"How should errors be handled in Go?":                             true,
	"How do you use `context.Context`?":                               true,
	"Goroutines versus channels versus mutexes?":                      true,
	"How do you write tests in Go?":                                   true,
	"How do you structure a Go service?":                              true,
	"What makes a Go API production-grade?":                           true,
	"How do you design a good REST API?":                              true,
	"What belongs in API gateway middleware?":                         true,
	"How do you make POST requests safe under retries?":               true,
	"How do you design rate limiting?":                                true,
	"How should API errors be shaped?":                                true,
	"What is the role of an API gateway in microservices?":            true,
	"How do you stream responses from an API?":                        true,
	"How do you design a robust third-party API client?":              true,
	"Which third-party API errors should you retry?":                  true,
	"How do idempotency keys apply to external APIs?":                 true,
	"How do you handle webhooks safely?":                              true,
	"How do you prevent provider outages from cascading?":             true,
	"How do context timeouts and HTTP client timeouts work together?": true,
	"How do you handle external API rate limits?":                     true,
	"How do you handle transactions across multiple microservices?":   true,
	"What is eventual consistency, and when is it acceptable?":        true,
	"How do you make message consumers reliable?":                     true,
	"What is the outbox pattern?":                                     true,
	"How do retries and idempotency work together?":                   true,
	"How do circuit breakers help distributed systems?":               true,
	"How do you design for backpressure?":                             true,
	"How do you debug high latency in a distributed system?":          true,
	"How do you design graceful shutdown?":                            true,
	"How does a tool-calling agent work?":                             true,
	"How do you prevent runaway agent loops?":                         true,
	"What is a tool registry?":                                        true,
	"How do you integrate RAG into an agent?":                         true,
	"How do you stream agent responses?":                              true,
	"How do you handle tool errors?":                                  true,
	"What guardrails belong in an AI gateway?":                        true,
	"How do you observe an agent in production?":                      true,
}

func questionHeading(line string) (string, bool) {
	matches := numberedHeading.FindStringSubmatch(line)
	if len(matches) == 2 {
		return strings.TrimSpace(matches[1]), true
	}
	matches = exerciseHeading.FindStringSubmatch(line)
	if len(matches) == 2 {
		return strings.TrimSpace(matches[1]), true
	}
	matches = scenarioHeading.FindStringSubmatch(line)
	if len(matches) == 2 {
		return "Scenario: " + strings.TrimSpace(matches[1]), true
	}
	if strings.HasPrefix(line, "### ") {
		prompt := strings.TrimSpace(strings.TrimPrefix(line, "### "))
		if looksLikePrompt(prompt) {
			return prompt, true
		}
	}
	return "", false
}

func looksLikePrompt(prompt string) bool {
	return strings.HasSuffix(prompt, "?") ||
		strings.HasPrefix(prompt, "Tell me ") ||
		strings.HasPrefix(prompt, "How ") ||
		strings.HasPrefix(prompt, "What ") ||
		strings.HasPrefix(prompt, "Which ") ||
		strings.HasPrefix(prompt, "Why ")
}

func cleanAnswer(lines []string) string {
	var cleaned []string
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			if len(cleaned) > 0 && cleaned[len(cleaned)-1] != "" {
				cleaned = append(cleaned, "")
			}
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, ">"))
		if line != "" {
			cleaned = append(cleaned, line)
		}
	}
	return strings.TrimSpace(strings.Join(cleaned, "\n"))
}

func cleanPrompt(lines []string) string {
	var fields []string
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		line = strings.TrimSpace(strings.TrimPrefix(line, ">"))
		if line != "" {
			fields = append(fields, strings.Fields(line)...)
		}
	}
	return strings.Join(fields, " ")
}

func cleanAnchorLine(line string) (RepoAnchor, bool) {
	line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "- "))
	if line == "" || !strings.HasPrefix(line, "`") {
		return RepoAnchor{}, false
	}
	rest := strings.TrimPrefix(line, "`")
	parts := strings.SplitN(rest, "`", 2)
	if len(parts) != 2 {
		return RepoAnchor{}, false
	}
	path := strings.TrimSpace(parts[0])
	note := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(parts[1]), "-"))
	if path == "" {
		return RepoAnchor{}, false
	}
	return RepoAnchor{Path: path, Note: note}, true
}

func cloneAnchors(in []RepoAnchor) []RepoAnchor {
	if len(in) == 0 {
		return nil
	}
	out := make([]RepoAnchor, len(in))
	copy(out, in)
	return out
}

func priorityForPath(path string) int {
	base := filepath.Base(path)
	if strings.HasPrefix(base, "02-") || strings.HasPrefix(base, "03-") ||
		strings.HasPrefix(base, "04-") || strings.HasPrefix(base, "05-") ||
		strings.HasPrefix(base, "09-") {
		return 10
	}
	return 0
}
