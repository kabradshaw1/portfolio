package observability

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
)

type JaegerClient struct {
	baseURL       string
	http          *http.Client
	maxTraceSpans int
}

func NewJaeger(baseURL string, httpClient *http.Client, maxTraceSpans int) *JaegerClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	if maxTraceSpans <= 0 {
		maxTraceSpans = 100
	}
	return &JaegerClient{baseURL: strings.TrimRight(baseURL, "/"), http: httpClient, maxTraceSpans: maxTraceSpans}
}

func (c *JaegerClient) Trace(ctx context.Context, traceID string) (TraceSummary, error) {
	if strings.TrimSpace(traceID) == "" {
		return TraceSummary{}, fmt.Errorf("trace_id is required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/traces/"+urlPathEscape(traceID), nil)
	if err != nil {
		return TraceSummary{}, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return TraceSummary{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return TraceSummary{}, fmt.Errorf("jaeger HTTP status %d", resp.StatusCode)
	}
	var decoded jaegerResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return TraceSummary{}, fmt.Errorf("decode jaeger response: %w", err)
	}
	if len(decoded.Data) == 0 {
		return TraceSummary{TraceID: traceID}, nil
	}
	trace := decoded.Data[0]
	processes := map[string]string{}
	for id, process := range trace.Processes {
		processes[id] = process.ServiceName
	}
	summary := TraceSummary{TraceID: trace.TraceID, SpanCount: len(trace.Spans)}
	for _, span := range trace.Spans {
		summary.Spans = append(summary.Spans, TraceSpan{
			Service:    processes[span.ProcessID],
			Operation:  span.OperationName,
			DurationMS: span.Duration / 1000,
			Error:      spanHasError(span.Tags),
		})
	}
	sort.Slice(summary.Spans, func(i, j int) bool {
		return summary.Spans[i].DurationMS > summary.Spans[j].DurationMS
	})
	if len(summary.Spans) > c.maxTraceSpans {
		summary.Spans = summary.Spans[:c.maxTraceSpans]
		summary.Truncated = true
	}
	return summary, nil
}

func urlPathEscape(value string) string {
	return strings.ReplaceAll(value, "/", "%2F")
}

func spanHasError(tags []jaegerTag) bool {
	for _, tag := range tags {
		if tag.Key != "error" {
			continue
		}
		switch v := tag.Value.(type) {
		case bool:
			return v
		case string:
			return strings.EqualFold(v, "true")
		}
	}
	return false
}

type jaegerResponse struct {
	Data []struct {
		TraceID   string `json:"traceID"`
		Processes map[string]struct {
			ServiceName string `json:"serviceName"`
		} `json:"processes"`
		Spans []struct {
			OperationName string      `json:"operationName"`
			ProcessID     string      `json:"processID"`
			Duration      int64       `json:"duration"`
			Tags          []jaegerTag `json:"tags"`
		} `json:"spans"`
	} `json:"data"`
}

type jaegerTag struct {
	Key   string `json:"key"`
	Value any    `json:"value"`
}
