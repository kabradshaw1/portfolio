package corpusfixture

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var (
	fixtureIDPattern  = regexp.MustCompile(`^[a-z0-9_]{1,80}$`)
	collectionPattern = regexp.MustCompile(`^eval_[a-z0-9_]+$`)
)

type Catalog struct {
	roots []string
}

type Fixture struct {
	ID                       string     `json:"id"`
	Name                     string     `json:"name"`
	Description              string     `json:"description"`
	Path                     string     `json:"path"`
	Root                     string     `json:"root"`
	Documents                []Document `json:"documents"`
	DocumentCount            int        `json:"document_count"`
	SourceHash               string     `json:"source_hash"`
	DefaultCollection        string     `json:"default_collection"`
	ExpectedCollectionPrefix string     `json:"expected_collection_prefix"`
	Notes                    string     `json:"notes,omitempty"`
}

type Document struct {
	Path   string `json:"path"`
	Abs    string `json:"-"`
	SHA256 string `json:"sha256"`
}

type fixtureFile struct {
	ID                       string   `json:"id"`
	Name                     string   `json:"name"`
	Description              string   `json:"description"`
	Documents                []string `json:"documents"`
	ExpectedCollectionPrefix string   `json:"expected_collection_prefix"`
	Notes                    string   `json:"notes"`
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
	var out []Fixture
	for _, root := range c.roots {
		matches, err := filepath.Glob(filepath.Join(root, "*.corpus.json"))
		if err != nil {
			return nil, err
		}
		sort.Strings(matches)
		for _, path := range matches {
			fixture, err := c.Load(path)
			if err != nil {
				return nil, err
			}
			out = append(out, fixture)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (c *Catalog) Load(idOrPath string) (Fixture, error) {
	path, root, err := c.resolve(idOrPath)
	if err != nil {
		return Fixture{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Fixture{}, err
	}
	var raw fixtureFile
	if err := json.Unmarshal(data, &raw); err != nil {
		return Fixture{}, err
	}
	if !fixtureIDPattern.MatchString(raw.ID) {
		return Fixture{}, fmt.Errorf("fixture id must match %s", fixtureIDPattern.String())
	}
	if raw.Name == "" || raw.Description == "" {
		return Fixture{}, fmt.Errorf("name and description are required")
	}
	if len(raw.Documents) == 0 {
		return Fixture{}, fmt.Errorf("documents are required")
	}
	if !collectionPattern.MatchString(raw.ExpectedCollectionPrefix) {
		return Fixture{}, fmt.Errorf("expected_collection_prefix must match %s", collectionPattern.String())
	}
	docs := make([]Document, 0, len(raw.Documents))
	for _, docPath := range raw.Documents {
		doc, err := resolveDocument(root, docPath)
		if err != nil {
			return Fixture{}, err
		}
		docs = append(docs, doc)
	}
	sourceHash, err := computeHash(raw, docs)
	if err != nil {
		return Fixture{}, err
	}
	return Fixture{
		ID:                       raw.ID,
		Name:                     raw.Name,
		Description:              raw.Description,
		Path:                     path,
		Root:                     root,
		Documents:                docs,
		DocumentCount:            len(docs),
		SourceHash:               sourceHash,
		DefaultCollection:        raw.ExpectedCollectionPrefix + "_" + sourceHash[:8],
		ExpectedCollectionPrefix: raw.ExpectedCollectionPrefix,
		Notes:                    raw.Notes,
	}, nil
}

func (c *Catalog) resolve(idOrPath string) (string, string, error) {
	for _, root := range c.roots {
		candidate := idOrPath
		if !filepath.IsAbs(candidate) && filepath.Ext(candidate) == "" {
			candidate = filepath.Join(root, idOrPath+".corpus.json")
		} else if !filepath.IsAbs(candidate) {
			candidate = filepath.Join(root, candidate)
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
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			continue
		}
		if _, err := os.Stat(absCandidate); err == nil {
			return absCandidate, absRoot, nil
		}
	}
	return "", "", fmt.Errorf("corpus fixture %q not found", idOrPath)
}

func resolveDocument(root, docPath string) (Document, error) {
	if docPath == "" || filepath.IsAbs(docPath) {
		return Document{}, fmt.Errorf("document %q must be relative", docPath)
	}
	if strings.ToLower(filepath.Ext(docPath)) != ".pdf" {
		return Document{}, fmt.Errorf("document %q must be a PDF", docPath)
	}
	absDoc, err := filepath.Abs(filepath.Join(root, docPath))
	if err != nil {
		return Document{}, err
	}
	rel, err := filepath.Rel(root, absDoc)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return Document{}, fmt.Errorf("document %q must stay under fixture root", docPath)
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return Document{}, err
	}
	realDoc, err := filepath.EvalSymlinks(absDoc)
	if err != nil {
		return Document{}, err
	}
	realRel, err := filepath.Rel(realRoot, realDoc)
	if err != nil || realRel == ".." || strings.HasPrefix(realRel, ".."+string(filepath.Separator)) || filepath.IsAbs(realRel) {
		return Document{}, fmt.Errorf("document %q must stay under fixture root", docPath)
	}
	data, err := os.ReadFile(realDoc)
	if err != nil {
		return Document{}, err
	}
	sum := sha256.Sum256(data)
	return Document{Path: filepath.ToSlash(docPath), Abs: realDoc, SHA256: hex.EncodeToString(sum[:])}, nil
}

func computeHash(raw fixtureFile, docs []Document) (string, error) {
	h := sha256.New()
	h.Write([]byte(raw.ID + "\n" + raw.Name + "\n" + raw.Description + "\n" + raw.ExpectedCollectionPrefix + "\n"))
	for _, doc := range docs {
		h.Write([]byte(doc.Path + "\n" + doc.SHA256 + "\n"))
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
