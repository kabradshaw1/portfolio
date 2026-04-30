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

var numberedHeading = regexp.MustCompile(`^#+\s+\d+\.\s+(.+)$`)

// Question is one imported study prompt from the interview-prep material.
type Question struct {
	SourcePath     string
	Topic          string
	Prompt         string
	ExpectedAnswer string
	IsFollowUp     bool
	Priority       int
}

// ParseDir parses every markdown file directly under root in lexical order.
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
	var answerLines []string
	mode := ""

	flush := func() {
		if current == nil {
			return
		}
		current.ExpectedAnswer = cleanAnswer(answerLines)
		if current.Prompt != "" {
			questions = append(questions, *current)
		}
		current = nil
		answerLines = nil
		mode = ""
	}

	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "# ") {
			topic = strings.TrimSpace(strings.TrimPrefix(line, "# "))
			continue
		}

		if prompt, ok := questionHeading(line); ok {
			flush()
			current = &Question{
				SourcePath: path,
				Topic:      topic,
				Prompt:     prompt,
				Priority:   priorityForPath(path),
			}
			continue
		}

		switch {
		case line == "Fast answer:":
			mode = "answer"
		case line == "Follow-ups:":
			flush()
			mode = "followups"
		case mode == "answer" && current != nil:
			answerLines = append(answerLines, raw)
		case mode == "followups" && strings.HasPrefix(line, "- "):
			prompt := strings.TrimSpace(strings.TrimPrefix(line, "- "))
			if prompt != "" {
				questions = append(questions, Question{
					SourcePath: path,
					Topic:      topic,
					Prompt:     prompt,
					IsFollowUp: true,
					Priority:   priorityForPath(path),
				})
			}
		}
	}
	flush()

	return questions, nil
}

func questionHeading(line string) (string, bool) {
	matches := numberedHeading.FindStringSubmatch(line)
	if len(matches) != 2 {
		return "", false
	}
	return strings.TrimSpace(matches[1]), true
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

func priorityForPath(path string) int {
	base := filepath.Base(path)
	if strings.HasPrefix(base, "02-") || strings.HasPrefix(base, "03-") ||
		strings.HasPrefix(base, "04-") || strings.HasPrefix(base, "05-") ||
		strings.HasPrefix(base, "09-") {
		return 10
	}
	return 0
}
