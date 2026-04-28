package composite

import "strconv"

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

// ComputeVerdict reduces an EvidenceBundle to a Verdict.
// The mapping encodes the customer-facing interpretation of the saga state machine:
// completed orders return "ok", in-flight ones return "retrying" with a saga-step
// specific message, and failed orders surface a contact-support next action.
func ComputeVerdict(b EvidenceBundle) Verdict {
	v := Verdict{
		Evidence: Evidence{
			TraceID:  b.Order.TraceID,
			SagaStep: b.Saga.Step,
			Partial:  b.Partial,
			LastLogs: b.Logs,
		},
	}

	switch b.Order.Status {
	case "completed":
		v.Stage = "completed"
		v.Status = "ok"
		v.CustomerMessage = "Your order is complete."
		v.TechnicalDetail = "saga=" + b.Saga.Step
		v.NextAction = "none"
	case "processing":
		v.Stage = b.Saga.Step
		v.Status = "retrying"
		switch b.Saga.Step {
		case "payment_captured":
			v.CustomerMessage = "Your payment cleared and we're handing off to the warehouse."
		case "warehouse_pending":
			v.CustomerMessage = "Warehouse hasn't acknowledged yet — we're retrying."
		default:
			v.CustomerMessage = "Your order is in progress."
		}
		v.TechnicalDetail = "saga=" + b.Saga.Step
		v.NextAction = "wait"
	case "failed":
		v.Stage = "failed"
		v.Status = "failed"
		v.CustomerMessage = "Your order didn't go through. Please contact support."
		v.TechnicalDetail = "saga=" + b.Saga.Step + " retries=" + strconv.Itoa(b.Saga.Retries)
		v.NextAction = "contact_support"
	default:
		v.Stage = b.Order.Status
		v.Status = "stalled"
		v.CustomerMessage = "Your order is in an unusual state."
		v.NextAction = "contact_support"
	}
	return v
}
