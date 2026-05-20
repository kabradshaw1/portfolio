package management

import (
	"context"
	"testing"
	"time"

	"github.com/kabradshaw1/portfolio/go/observability-mcp-service/internal/history"
)

type fakeRunner struct{ result ActionResult }

func (f fakeRunner) Run(context.Context, Action) ActionResult { return f.result }

type fakeStore struct {
	inputs []history.ManagementActionInput
	events []history.Event
}

func (f *fakeStore) RecordManagementAction(_ context.Context, in history.ManagementActionInput) (history.Event, error) {
	f.inputs = append(f.inputs, in)
	return history.Event{ID: int64(len(f.inputs)), Type: history.EventManagementActionCompleted, Summary: in.Summary}, nil
}

func (f *fakeStore) ListManagementActions(context.Context, history.ManagementActionFilter) ([]history.Event, error) {
	return f.events, nil
}

func TestServicePreviewBlocksDisabledPolicy(t *testing.T) {
	catalog := mustCatalog(t, []Action{{ID: "a", Title: "A", Description: "desc", RiskTier: RiskLowRiskMutation, ScriptPath: "scripts/ops/a.sh", Timeout: time.Second, TimeoutText: "1s"}})
	service := NewService(catalog, Policy{}, fakeRunner{}, nil)
	result := service.Preview(context.Background(), ActionRequest{ActionID: "a"})
	if result.Status != StatusBlocked || result.Decision != DecisionBlock {
		t.Fatalf("result = %+v", result)
	}
}

func TestServiceExecuteRunsAllowedActionAndRecordsHistory(t *testing.T) {
	catalog := mustCatalog(t, []Action{{ID: "a", Title: "A", Description: "desc", RiskTier: RiskLowRiskMutation, ScriptPath: "scripts/ops/a.sh", Timeout: time.Second, TimeoutText: "1s"}})
	store := &fakeStore{}
	service := NewService(catalog, Policy{ActionsEnabled: true}, fakeRunner{result: ActionResult{Status: StatusSucceeded, Stdout: "ok"}}, store)
	result := service.Execute(context.Background(), ActionRequest{ActionID: "a", IncidentKey: "inc", IncidentTitle: "Incident"})
	if result.Status != StatusSucceeded || result.Decision != DecisionAllow {
		t.Fatalf("result = %+v", result)
	}
	if len(store.inputs) != 1 || store.inputs[0].ActionID != "a" {
		t.Fatalf("history inputs = %+v", store.inputs)
	}
	if len(result.HistoryEventIDs) != 1 {
		t.Fatalf("history ids = %+v", result.HistoryEventIDs)
	}
}

func TestServiceExecuteRequiresTitleForNewIncident(t *testing.T) {
	catalog := mustCatalog(t, []Action{{ID: "a", Title: "A", Description: "desc", RiskTier: RiskLowRiskMutation, ScriptPath: "scripts/ops/a.sh", Timeout: time.Second, TimeoutText: "1s"}})
	service := NewService(catalog, Policy{ActionsEnabled: true}, fakeRunner{result: ActionResult{Status: StatusSucceeded}}, &fakeStore{})
	result := service.Execute(context.Background(), ActionRequest{ActionID: "a", IncidentKey: "inc"})
	if result.Status != StatusBlocked || result.PolicyReason != "incident_title is required when incident_key creates action history" {
		t.Fatalf("result = %+v", result)
	}
}

func mustCatalog(t *testing.T, actions []Action) Catalog {
	t.Helper()
	catalog, err := NewCatalog(actions)
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}
