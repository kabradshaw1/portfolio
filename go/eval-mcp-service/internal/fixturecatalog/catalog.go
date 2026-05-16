package fixturecatalog

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	maxNameLen   = 100
	maxItems     = 100
	maxQueryLen  = 2000
	maxAnswerLen = 5000
)

var datasetNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

type Catalog struct {
	roots []string
}

type Fixture struct {
	ID                string       `json:"id"`
	Name              string       `json:"name"`
	Path              string       `json:"path"`
	DocumentRoot      string       `json:"document_root"`
	ItemCount         int          `json:"item_count"`
	ReferencedSources []string     `json:"referenced_sources"`
	Valid             bool         `json:"valid"`
	Errors            []string     `json:"errors,omitempty"`
	Items             []GoldenItem `json:"-"`
}

type GoldenItem struct {
	Query           string   `json:"query"`
	ExpectedAnswer  string   `json:"expected_answer"`
	ExpectedSources []string `json:"expected_sources"`
}

type fixtureFile struct {
	Name  string       `json:"name"`
	Items []GoldenItem `json:"items"`
}

func New(roots []string) *Catalog {
	cleaned := make([]string, 0, len(roots))
	for _, root := range roots {
		if strings.TrimSpace(root) != "" {
			cleaned = append(cleaned, root)
		}
	}
	return &Catalog{roots: cleaned}
}

func (c *Catalog) List() ([]Fixture, error) {
	var fixtures []Fixture
	for _, root := range c.roots {
		matches, err := filepath.Glob(filepath.Join(root, "*.json"))
		if err != nil {
			return nil, err
		}
		sort.Strings(matches)
		for _, path := range matches {
			fixture, err := c.Load(path)
			if err != nil {
				fixtures = append(fixtures, Fixture{
					ID:           filepath.Base(path),
					Path:         path,
					DocumentRoot: root,
					Valid:        false,
					Errors:       []string{err.Error()},
				})
				continue
			}
			fixtures = append(fixtures, fixture)
		}
	}
	sort.Slice(fixtures, func(i, j int) bool { return fixtures[i].ID < fixtures[j].ID })
	return fixtures, nil
}

func (c *Catalog) Load(idOrPath string) (Fixture, error) {
	path, root, err := c.resolve(idOrPath)
	if err != nil {
		return Fixture{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Fixture{}, fmt.Errorf("read fixture %q: %w", path, err)
	}
	var raw fixtureFile
	if err := json.Unmarshal(data, &raw); err != nil {
		return Fixture{}, fmt.Errorf("parse fixture %q: %w", path, err)
	}
	fixture := Fixture{
		ID:           filepath.Base(path),
		Name:         raw.Name,
		Path:         path,
		DocumentRoot: root,
		ItemCount:    len(raw.Items),
		Items:        raw.Items,
	}
	fixture.Errors = validateFixture(raw, root)
	fixture.ReferencedSources = referencedSources(raw.Items)
	fixture.Valid = len(fixture.Errors) == 0
	if !fixture.Valid {
		return fixture, fmt.Errorf("invalid fixture %q: %s", path, strings.Join(fixture.Errors, "; "))
	}
	return fixture, nil
}

func (c *Catalog) resolve(idOrPath string) (string, string, error) {
	for _, root := range c.roots {
		candidate := idOrPath
		if !filepath.IsAbs(candidate) {
			candidate = filepath.Join(root, idOrPath)
		}
		absRoot, err := filepath.Abs(root)
		if err != nil {
			return "", "", err
		}
		absCandidate, err := filepath.Abs(candidate)
		if err != nil {
			return "", "", err
		}
		rel, err := filepath.Rel(absRoot, absCandidate)
		if err != nil || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." || filepath.IsAbs(rel) {
			continue
		}
		if _, err := os.Stat(absCandidate); err == nil {
			return absCandidate, absRoot, nil
		}
	}
	return "", "", fmt.Errorf("fixture %q not found", idOrPath)
}

func validateFixture(raw fixtureFile, root string) []string {
	var errs []string
	if raw.Name == "" || len(raw.Name) > maxNameLen || !datasetNamePattern.MatchString(raw.Name) {
		errs = append(errs, "name must match ^[a-zA-Z0-9_-]+$ and be 1-100 characters")
	}
	if len(raw.Items) == 0 || len(raw.Items) > maxItems {
		errs = append(errs, "items must contain 1-100 entries")
	}
	for i, item := range raw.Items {
		if strings.TrimSpace(item.Query) == "" || len(item.Query) > maxQueryLen {
			errs = append(errs, fmt.Sprintf("items[%d].query must be 1-2000 characters", i))
		}
		if strings.TrimSpace(item.ExpectedAnswer) == "" || len(item.ExpectedAnswer) > maxAnswerLen {
			errs = append(errs, fmt.Sprintf("items[%d].expected_answer must be 1-5000 characters", i))
		}
		for _, source := range item.ExpectedSources {
			if err := validateSource(root, source); err != nil {
				errs = append(errs, fmt.Sprintf("items[%d].expected_sources %q: %v", i, source, err))
			}
		}
	}
	return errs
}

func validateSource(root, source string) error {
	if source == "" || filepath.IsAbs(source) {
		return fmt.Errorf("must be a relative path")
	}
	ext := strings.ToLower(filepath.Ext(source))
	if ext != ".pdf" && ext != ".md" {
		return fmt.Errorf("must reference a .pdf or .md file")
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	absSource, err := filepath.Abs(filepath.Join(root, source))
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(absRoot, absSource)
	if err != nil || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." || filepath.IsAbs(rel) {
		return fmt.Errorf("must stay under document root")
	}
	if _, err := os.Stat(absSource); err != nil {
		return err
	}
	return nil
}

func referencedSources(items []GoldenItem) []string {
	seen := map[string]struct{}{}
	for _, item := range items {
		for _, source := range item.ExpectedSources {
			seen[source] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for source := range seen {
		out = append(out, source)
	}
	sort.Strings(out)
	return out
}
