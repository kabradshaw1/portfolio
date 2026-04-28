package composite

// Verdict is the structured output of investigate_my_order.
type Verdict struct {
	Stage           string   `json:"stage"`
	Status          string   `json:"status"`
	CustomerMessage string   `json:"customer_message"`
	TechnicalDetail string   `json:"technical_detail"`
	NextAction      string   `json:"next_action"`
	Evidence        Evidence `json:"evidence"`
}

// Evidence records the data sources consulted to produce the verdict.
type Evidence struct {
	TraceID  string   `json:"trace_id"`
	SagaStep string   `json:"saga_step"`
	Partial  bool     `json:"partial_evidence"`
	LastLogs []string `json:"last_logs"`
}
