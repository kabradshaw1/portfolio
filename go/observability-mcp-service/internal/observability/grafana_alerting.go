package observability

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"time"
)

const maxAlertMetadataEntries = 20

func (c *GrafanaClient) ActiveAlerts(ctx context.Context) ([]AlertInstance, error) {
	var raw []grafanaAlert
	if err := c.getJSON(ctx, "/api/alertmanager/grafana/api/v2/alerts", &raw); err != nil {
		return nil, err
	}
	alerts := make([]AlertInstance, 0, len(raw))
	for _, alert := range raw {
		alerts = append(alerts, alert.toAlertInstance())
	}
	return alerts, nil
}

func (c *GrafanaClient) AlertRules(ctx context.Context) ([]AlertRule, error) {
	var raw []grafanaAlertRule
	if err := c.getJSON(ctx, "/api/v1/provisioning/alert-rules", &raw); err != nil {
		return nil, err
	}
	rules := make([]AlertRule, 0, len(raw))
	for _, rule := range raw {
		rules = append(rules, rule.toAlertRule())
	}
	return rules, nil
}

func (c *GrafanaClient) getJSON(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("grafana HTTP status %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode grafana response: %w", err)
	}
	return nil
}

type grafanaAlert struct {
	Labels       map[string]string `json:"labels"`
	Annotations  map[string]string `json:"annotations"`
	StartsAt     time.Time         `json:"startsAt"`
	EndsAt       time.Time         `json:"endsAt"`
	GeneratorURL string            `json:"generatorURL"`
	DashboardURL string            `json:"dashboardURL"`
	Status       struct {
		State string `json:"state"`
	} `json:"status"`
}

func (a grafanaAlert) toAlertInstance() AlertInstance {
	labels := boundedMap(a.Labels, maxAlertMetadataEntries)
	annotations := boundedMap(a.Annotations, maxAlertMetadataEntries)
	name := labels["alertname"]
	ruleUID := labels["grafana_rule_uid"]
	if ruleUID == "" {
		ruleUID = labels["rule_uid"]
	}
	return AlertInstance{
		Name:         name,
		State:        a.Status.State,
		Labels:       labels,
		Annotations:  annotations,
		StartsAt:     a.StartsAt,
		EndsAt:       a.EndsAt,
		GeneratorURL: a.GeneratorURL,
		DashboardURL: a.DashboardURL,
		RuleUID:      ruleUID,
	}
}

type grafanaAlertRule struct {
	UID        string            `json:"uid"`
	Title      string            `json:"title"`
	FolderUID  string            `json:"folderUID"`
	Namespace  string            `json:"namespace_uid"`
	RuleGroup  string            `json:"ruleGroup"`
	Condition  string            `json:"condition"`
	Labels     map[string]string `json:"labels"`
	Provenance string            `json:"provenance"`
}

func (r grafanaAlertRule) toAlertRule() AlertRule {
	namespace := r.Namespace
	if namespace == "" {
		namespace = r.FolderUID
	}
	return AlertRule{
		UID:        r.UID,
		Title:      r.Title,
		FolderUID:  r.FolderUID,
		Namespace:  namespace,
		RuleGroup:  r.RuleGroup,
		Condition:  r.Condition,
		Labels:     boundedMap(r.Labels, maxAlertMetadataEntries),
		Provenance: r.Provenance,
	}
}

func boundedMap(values map[string]string, limit int) map[string]string {
	if len(values) == 0 || limit <= 0 {
		return nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) > limit {
		keys = keys[:limit]
	}
	bounded := make(map[string]string, len(keys))
	for _, key := range keys {
		bounded[key] = values[key]
	}
	return bounded
}
