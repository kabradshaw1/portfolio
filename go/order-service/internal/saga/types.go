package saga

import "time"

// Saga step constants — stored in orders.saga_step column.
const (
	StepCreated              = "CREATED"
	StepItemsReserved        = "ITEMS_RESERVED"
	StepStockValidated       = "STOCK_VALIDATED"
	StepPaymentCreated       = "PAYMENT_CREATED"
	StepPaymentConfirmed     = "PAYMENT_CONFIRMED"
	StepCompleted            = "COMPLETED"
	StepCompensating         = "COMPENSATING"
	StepCompensationComplete = "COMPENSATION_COMPLETE"
	StepFailed               = "FAILED"
)

// TerminalSteps are saga steps that indicate the saga is finished.
var TerminalSteps = []string{StepCompleted, StepCompensationComplete, StepFailed}

// Command is a saga command published to RabbitMQ.
type Command struct {
	Command   string        `json:"command"`
	OrderID   string        `json:"order_id"`
	UserID    string        `json:"user_id"`
	Items     []CommandItem `json:"items,omitempty"`
	TraceID   string        `json:"trace_id"`
	Timestamp time.Time     `json:"timestamp"`
}

// CommandItem identifies a product and quantity in a saga command.
type CommandItem struct {
	ProductID string `json:"product_id"`
	Quantity  int    `json:"quantity"`
}

// Event is a saga event reply consumed from RabbitMQ.
type Event struct {
	Event     string    `json:"event"`
	OrderID   string    `json:"order_id"`
	UserID    string    `json:"user_id"`
	Success   bool      `json:"success"`
	Error     string    `json:"error,omitempty"`
	TraceID   string    `json:"trace_id"`
	Timestamp time.Time `json:"timestamp"`
}

// Commands
const (
	CmdReserveItems  = "reserve.items"
	CmdReleaseItems  = "release.items"
	CmdClearCart     = "clear.cart"
	CmdCreatePayment = "create.payment"
)

// Events
const (
	EvtItemsReserved    = "items.reserved"
	EvtItemsReleased    = "items.released"
	EvtCartCleared      = "cart.cleared"
	EvtPaymentConfirmed = "payment.confirmed"
	EvtPaymentFailed    = "payment.failed"
)
