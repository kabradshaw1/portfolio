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

type PrometheusClient struct {
	baseURL string
	http    *http.Client
}

func NewPrometheus(baseURL string, httpClient *http.Client) *PrometheusClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &PrometheusClient{baseURL: strings.TrimRight(baseURL, "/"), http: httpClient}
}

func (c *PrometheusClient) Query(ctx context.Context, query string) ([]MetricSample, error) {
	values := url.Values{"query": []string{query}}
	var resp prometheusResponse
	if err := c.get(ctx, "/api/v1/query", values, &resp); err != nil {
		return nil, err
	}
	return parsePrometheusResult(resp.Data.Result)
}

func (c *PrometheusClient) QueryRange(ctx context.Context, query string, start, end time.Time, step time.Duration) ([]MetricSample, error) {
	values := url.Values{
		"query": []string{query},
		"start": []string{strconv.FormatInt(start.Unix(), 10)},
		"end":   []string{strconv.FormatInt(end.Unix(), 10)},
		"step":  []string{strconv.FormatFloat(step.Seconds(), 'f', -1, 64)},
	}
	var resp prometheusResponse
	if err := c.get(ctx, "/api/v1/query_range", values, &resp); err != nil {
		return nil, err
	}
	return parsePrometheusResult(resp.Data.Result)
}

func (c *PrometheusClient) get(ctx context.Context, path string, values url.Values, out *prometheusResponse) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path+"?"+values.Encode(), nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("prometheus HTTP status %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode prometheus response: %w", err)
	}
	if out.Status != "success" {
		if out.Error != "" {
			return fmt.Errorf("prometheus query failed: %s", out.Error)
		}
		return fmt.Errorf("prometheus query failed with status %q", out.Status)
	}
	return nil
}

type prometheusResponse struct {
	Status string `json:"status"`
	Error  string `json:"error"`
	Data   struct {
		Result []prometheusResult `json:"result"`
	} `json:"data"`
}

type prometheusResult struct {
	Metric map[string]string `json:"metric"`
	Value  []any             `json:"value"`
	Values [][]any           `json:"values"`
}

func parsePrometheusResult(results []prometheusResult) ([]MetricSample, error) {
	var samples []MetricSample
	for _, result := range results {
		if len(result.Value) > 0 {
			sample, err := parsePrometheusSample(result.Metric, result.Value)
			if err != nil {
				return nil, err
			}
			samples = append(samples, sample)
		}
		for _, value := range result.Values {
			sample, err := parsePrometheusSample(result.Metric, value)
			if err != nil {
				return nil, err
			}
			samples = append(samples, sample)
		}
	}
	return samples, nil
}

func parsePrometheusSample(metric map[string]string, raw []any) (MetricSample, error) {
	if len(raw) != 2 {
		return MetricSample{}, fmt.Errorf("prometheus sample has %d values", len(raw))
	}
	ts, err := number(raw[0])
	if err != nil {
		return MetricSample{}, fmt.Errorf("parse prometheus timestamp: %w", err)
	}
	valueString, ok := raw[1].(string)
	if !ok {
		return MetricSample{}, fmt.Errorf("prometheus value is %T", raw[1])
	}
	value, err := strconv.ParseFloat(valueString, 64)
	if err != nil {
		return MetricSample{}, fmt.Errorf("parse prometheus value: %w", err)
	}
	return MetricSample{Metric: metric, Value: value, Time: time.Unix(int64(ts), int64((ts-float64(int64(ts)))*1e9)).UTC()}, nil
}

func number(value any) (float64, error) {
	switch v := value.(type) {
	case float64:
		return v, nil
	case string:
		return strconv.ParseFloat(v, 64)
	default:
		return 0, fmt.Errorf("unsupported number type %T", value)
	}
}
