package observability

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const defaultLogPattern = "(?i)(error|warn|exception)"

type LokiClient struct {
	baseURL string
	http    *http.Client
}

func NewLoki(baseURL string, httpClient *http.Client) *LokiClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &LokiClient{baseURL: strings.TrimRight(baseURL, "/"), http: httpClient}
}

func (c *LokiClient) QueryLogs(ctx context.Context, q LogQuery) ([]LogLine, bool, error) {
	if q.Pattern == "" {
		q.Pattern = defaultLogPattern
	}
	if q.Limit <= 0 {
		q.Limit = 100
	}
	query := fmt.Sprintf(`{service=%q} |~ %q`, q.Service, q.Pattern)
	values := url.Values{
		"query": []string{query},
		"start": []string{strconv.FormatInt(q.Start.UnixNano(), 10)},
		"end":   []string{strconv.FormatInt(q.End.UnixNano(), 10)},
		"limit": []string{strconv.Itoa(q.Limit + 1)},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/loki/api/v1/query_range"+"?"+values.Encode(), nil)
	if err != nil {
		return nil, false, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, false, fmt.Errorf("loki HTTP status %d", resp.StatusCode)
	}
	var decoded lokiResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, false, fmt.Errorf("decode loki response: %w", err)
	}
	if decoded.Status != "success" {
		if decoded.Error != "" {
			return nil, false, fmt.Errorf("loki query failed: %s", decoded.Error)
		}
		return nil, false, fmt.Errorf("loki query failed with status %q", decoded.Status)
	}
	var lines []LogLine
	for _, result := range decoded.Data.Result {
		for _, value := range result.Values {
			if len(value) != 2 {
				return nil, false, fmt.Errorf("loki log entry has %d values", len(value))
			}
			nanos, err := strconv.ParseInt(value[0], 10, 64)
			if err != nil {
				return nil, false, fmt.Errorf("parse loki timestamp: %w", err)
			}
			lines = append(lines, LogLine{Time: time.Unix(0, nanos).UTC(), Labels: result.Stream, Line: value[1]})
		}
	}
	truncated := len(lines) > q.Limit
	if truncated {
		lines = lines[:q.Limit]
	}
	return lines, truncated, nil
}

type lokiResponse struct {
	Status string `json:"status"`
	Error  string `json:"error"`
	Data   struct {
		Result []struct {
			Stream map[string]string `json:"stream"`
			Values [][]string        `json:"values"`
		} `json:"result"`
	} `json:"data"`
}
