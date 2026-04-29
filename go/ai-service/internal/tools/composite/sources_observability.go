package composite

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// JaegerTraceSource fetches a single trace from Jaeger's query API.
// Empty trace IDs return a zero TraceSummary without making a request —
// orders without an attached trace_id (the common case until OTel propagation
// writes it back to the orders row) just produce empty Spans rather than 404s.
type JaegerTraceSource struct {
	BaseURL string
	HTTP    *http.Client
}

func (j JaegerTraceSource) FetchTrace(ctx context.Context, id string) (TraceSummary, error) {
	if id == "" {
		return TraceSummary{}, nil
	}
	req, err := http.NewRequestWithContext(ctx, "GET", j.BaseURL+"/api/traces/"+id, nil)
	if err != nil {
		return TraceSummary{}, err
	}
	resp, err := j.HTTP.Do(req)
	if err != nil {
		return TraceSummary{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return TraceSummary{}, fmt.Errorf("jaeger: status %d", resp.StatusCode)
	}
	var body struct {
		Data []struct {
			Spans []struct {
				OperationName string `json:"operationName"`
				Duration      int64  `json:"duration"` // microseconds
			} `json:"spans"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return TraceSummary{}, fmt.Errorf("jaeger: decode: %w", err)
	}
	out := TraceSummary{ID: id}
	if len(body.Data) > 0 {
		for _, s := range body.Data[0].Spans {
			out.Spans = append(out.Spans, SpanSummary{
				Name:       s.OperationName,
				DurationMs: s.Duration / 1000,
			})
		}
	}
	return out, nil
}

// LokiLogSource queries Loki's range API for error/warn lines from the listed services.
type LokiLogSource struct {
	BaseURL string
	HTTP    *http.Client
}

func (l LokiLogSource) FetchLogs(ctx context.Context, services []string, fromUnix, toUnix int64) ([]string, error) {
	if len(services) == 0 || fromUnix == 0 || toUnix == 0 {
		return nil, nil
	}
	q := `{service=~"` + servicesAlternation(services) + `"} |~ "(?i)(error|warn|exception)"`
	values := url.Values{
		"query": {q},
		"start": {strconv.FormatInt(fromUnix*1_000_000_000, 10)},
		"end":   {strconv.FormatInt(toUnix*1_000_000_000, 10)},
		"limit": {"50"},
	}
	req, err := http.NewRequestWithContext(ctx, "GET", l.BaseURL+"/loki/api/v1/query_range?"+values.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := l.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("loki: status %d", resp.StatusCode)
	}
	var body struct {
		Data struct {
			Result []struct {
				Values [][2]string `json:"values"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("loki: decode: %w", err)
	}
	var lines []string
	for _, r := range body.Data.Result {
		for _, v := range r.Values {
			lines = append(lines, v[1])
		}
	}
	return lines, nil
}

func servicesAlternation(s []string) string {
	return strings.Join(s, "|")
}

// NopRabbitSource is a placeholder pending the rabbit_events audit table and
// consumer wiring (out of scope for v1.0). It returns no events and no error,
// causing the partial-evidence flag to stay false. When the audit table lands
// (planned follow-up), swap in a Postgres-backed RabbitSource.
type NopRabbitSource struct{}

func (NopRabbitSource) FetchEvents(_ context.Context, _ string) ([]RabbitEvent, error) {
	return nil, nil
}
