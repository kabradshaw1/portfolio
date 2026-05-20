package management

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/kabradshaw1/portfolio/go/observability-mcp-service/internal/history"
)

type RunnerInterface interface {
	Run(context.Context, Action) ActionResult
}

type HistoryStore interface {
	RecordManagementAction(context.Context, history.ManagementActionInput) (history.Event, error)
	ListManagementActions(context.Context, history.ManagementActionFilter) ([]history.Event, error)
}

type Service struct {
	catalog Catalog
	policy  Policy
	runner  RunnerInterface
	history HistoryStore
}

func NewService(catalog Catalog, policy Policy, runner RunnerInterface, store HistoryStore) *Service {
	return &Service{catalog: catalog, policy: policy, runner: runner, history: store}
}

func (s *Service) List() []Action {
	return s.catalog.List()
}

func (s *Service) Preview(ctx context.Context, req ActionRequest) ActionResult {
	action, result, ok := s.prepare(req)
	if !ok {
		return s.record(ctx, req, result)
	}
	decision := s.policy.Evaluate(action)
	result.Decision = decision.Decision
	result.PolicyReason = decision.Reason
	if decision.Decision == DecisionBlock {
		result.Status = StatusBlocked
	} else {
		result.Status = StatusPreviewed
	}
	return s.record(ctx, req, result)
}

func (s *Service) Execute(ctx context.Context, req ActionRequest) ActionResult {
	action, result, ok := s.prepare(req)
	if !ok {
		return s.record(ctx, req, result)
	}
	if req.IncidentKey != "" && req.IncidentTitle == "" {
		result.Status = StatusBlocked
		result.Decision = DecisionBlock
		result.PolicyReason = "incident_title is required when incident_key creates action history"
		return s.record(ctx, req, result)
	}
	decision := s.policy.Evaluate(action)
	result.Decision = decision.Decision
	result.PolicyReason = decision.Reason
	if decision.Decision != DecisionAllow {
		result.Status = StatusBlocked
		if decision.Decision == DecisionPreviewOnly {
			result.Status = StatusPreviewed
		}
		return s.record(ctx, req, result)
	}
	if s.runner == nil {
		result.Status = StatusFailed
		result.PolicyReason = "management action runner is unavailable"
		return s.record(ctx, req, result)
	}
	result = s.runner.Run(ctx, action)
	result.ActionID = action.ID
	result.RiskTier = action.RiskTier
	result.Decision = decision.Decision
	result.PolicyReason = decision.Reason
	result.ScriptPath = action.ScriptPath
	result.IncidentKey = req.IncidentKey
	return s.record(ctx, req, result)
}

func (s *Service) History(ctx context.Context, filter history.ManagementActionFilter) ([]history.Event, error) {
	if s.history == nil {
		return nil, fmt.Errorf("management action history is disabled")
	}
	return s.history.ListManagementActions(ctx, filter)
}

func (s *Service) prepare(req ActionRequest) (Action, ActionResult, bool) {
	action, ok := s.catalog.Get(req.ActionID)
	result := ActionResult{
		ActionID:    req.ActionID,
		IncidentKey: req.IncidentKey,
		StartedAt:   time.Now().UTC(),
	}
	if !ok {
		result.Status = StatusBlocked
		result.Decision = DecisionBlock
		result.PolicyReason = "unknown management action"
		return Action{}, result, false
	}
	result.RiskTier = action.RiskTier
	result.ScriptPath = action.ScriptPath
	return action, result, true
}

func (s *Service) record(ctx context.Context, req ActionRequest, result ActionResult) ActionResult {
	if result.CompletedAt.IsZero() {
		result.CompletedAt = time.Now().UTC()
	}
	if result.DurationMillis == 0 && !result.StartedAt.IsZero() {
		result.DurationMillis = result.CompletedAt.Sub(result.StartedAt).Milliseconds()
	}
	if s.history == nil {
		result.Warning = "management action history is disabled"
		return result
	}
	details, err := json.Marshal(result)
	if err != nil {
		result.Warning = "marshal management action history: " + err.Error()
		return result
	}
	event, err := s.history.RecordManagementAction(ctx, history.ManagementActionInput{
		IncidentKey:   incidentKey(req, result),
		IncidentTitle: incidentTitle(req, result),
		Severity:      req.Severity,
		Service:       req.Service,
		ActionID:      result.ActionID,
		RiskTier:      string(result.RiskTier),
		Decision:      string(result.Decision),
		Status:        string(result.Status),
		Summary:       fmt.Sprintf("%s %s", result.ActionID, result.Status),
		DetailsJSON:   details,
	})
	if err != nil {
		result.Warning = "record management action history: " + err.Error()
		return result
	}
	result.HistoryEventIDs = append(result.HistoryEventIDs, event.ID)
	return result
}

func incidentKey(req ActionRequest, result ActionResult) string {
	if req.IncidentKey != "" {
		return req.IncidentKey
	}
	return "management:" + result.ActionID
}

func incidentTitle(req ActionRequest, result ActionResult) string {
	if req.IncidentTitle != "" {
		return req.IncidentTitle
	}
	if req.IncidentKey == "" {
		return "Management action " + result.ActionID
	}
	return ""
}
