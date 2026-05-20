package management

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Catalog struct {
	actions map[string]Action
	order   []string
}

func NewCatalog(actions []Action) (Catalog, error) {
	catalog := Catalog{actions: make(map[string]Action, len(actions)), order: make([]string, 0, len(actions))}
	for _, action := range actions {
		if err := validateAction(action); err != nil {
			return Catalog{}, err
		}
		if _, ok := catalog.actions[action.ID]; ok {
			return Catalog{}, fmt.Errorf("duplicate management action id %q", action.ID)
		}
		if action.TimeoutText == "" {
			action.TimeoutText = action.Timeout.String()
		}
		catalog.actions[action.ID] = action
		catalog.order = append(catalog.order, action.ID)
	}
	sort.Strings(catalog.order)
	return catalog, nil
}

func (c Catalog) Get(id string) (Action, bool) {
	action, ok := c.actions[id]
	return action, ok
}

func (c Catalog) List() []Action {
	actions := make([]Action, 0, len(c.order))
	for _, id := range c.order {
		actions = append(actions, c.actions[id])
	}
	return actions
}

func (c Catalog) ValidateScripts(repoRoot string) error {
	root, err := filepath.Abs(repoRoot)
	if err != nil {
		return fmt.Errorf("resolve repo root: %w", err)
	}
	root = filepath.Clean(root)
	for _, action := range c.List() {
		fullPath, err := safeScriptPath(root, action.ScriptPath)
		if err != nil {
			return fmt.Errorf("%s: %w", action.ID, err)
		}
		info, err := os.Stat(fullPath)
		if err != nil {
			return fmt.Errorf("%s: stat script: %w", action.ID, err)
		}
		if info.IsDir() {
			return fmt.Errorf("%s: script path is a directory", action.ID)
		}
	}
	return nil
}

func DefaultCatalog() Catalog {
	catalog, err := NewCatalog([]Action{
		{
			ID:          "reload_grafana_alerting",
			Title:       "Reload Grafana alerting",
			Description: "Restart Grafana so committed alerting provisioning is loaded and verified.",
			RiskTier:    RiskLowRiskMutation,
			ScriptPath:  "scripts/ops/2026-05-09-reload-grafana-alerting.sh",
			Timeout:     2 * time.Minute,
			TimeoutText: "2m0s",
			Idempotent:  true,
			Preflight:   "Script verifies the expected Grafana alerting ConfigMap content before restart.",
			Postflight:  "Script waits for rollout and verifies live Grafana alert expression and active alert count.",
			NextSteps:   "Inspect script output and rerun system health evidence if the action fails.",
		},
		{
			ID:          "run_postgres_backup_verify",
			Title:       "Run Postgres backup verification",
			Description: "Create a manual Job from the committed Postgres backup verification CronJob and wait for completion.",
			RiskTier:    RiskLowRiskMutation,
			ScriptPath:  "scripts/ops/2026-05-15-run-postgres-backup-verify.sh",
			Timeout:     40 * time.Minute,
			TimeoutText: "40m0s",
			Idempotent:  true,
			Preflight:   "Script creates a timestamped Job from the existing CronJob.",
			Postflight:  "Script waits for completion and prints bounded pod logs.",
			NextSteps:   "Review Job logs and investigate backup-verification alerts if the action fails.",
		},
	})
	if err != nil {
		panic(err)
	}
	return catalog
}

func validateAction(action Action) error {
	if strings.TrimSpace(action.ID) == "" {
		return fmt.Errorf("management action id is required")
	}
	if strings.TrimSpace(action.Title) == "" {
		return fmt.Errorf("%s: title is required", action.ID)
	}
	if strings.TrimSpace(action.Description) == "" {
		return fmt.Errorf("%s: description is required", action.ID)
	}
	switch action.RiskTier {
	case RiskDiagnostic, RiskLowRiskMutation, RiskHighRiskMutation:
	default:
		return fmt.Errorf("%s: invalid risk tier %q", action.ID, action.RiskTier)
	}
	if action.Timeout <= 0 {
		return fmt.Errorf("%s: timeout must be positive", action.ID)
	}
	if _, err := safeScriptPath(".", action.ScriptPath); err != nil {
		return fmt.Errorf("%s: %w", action.ID, err)
	}
	return nil
}

func safeScriptPath(repoRoot, scriptPath string) (string, error) {
	if scriptPath == "" {
		return "", fmt.Errorf("script path is required")
	}
	if filepath.IsAbs(scriptPath) {
		return "", fmt.Errorf("script path must be relative")
	}
	cleanScript := filepath.Clean(scriptPath)
	if cleanScript == "." || cleanScript == ".." || strings.HasPrefix(cleanScript, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("script path escapes repo root")
	}
	root := filepath.Clean(repoRoot)
	fullPath := filepath.Clean(filepath.Join(root, cleanScript))
	rel, err := filepath.Rel(root, fullPath)
	if err != nil {
		return "", fmt.Errorf("resolve script path: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("script path escapes repo root")
	}
	return fullPath, nil
}
