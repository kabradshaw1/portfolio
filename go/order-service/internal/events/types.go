package events

const (
	TopicOrderEvents = "ecommerce.order-events"

	OrderCreated          = "order.created"
	OrderReserved         = "order.reserved"
	OrderPaymentInitiated = "order.payment_initiated"
	OrderPaymentCompleted = "order.payment_completed"
	OrderCompleted        = "order.completed"
	OrderFailed           = "order.failed"
	OrderCancelled        = "order.cancelled"

	CurrentVersion = 1
)

// OrderCreatedData is the payload for order.created events.
type OrderCreatedData struct {
	UserID     string          `json:"userID"`
	TotalCents int             `json:"totalCents"`
	Currency   string          `json:"currency"`
	Items      []OrderItemData `json:"items"`
}

// OrderItemData describes a single line item in an order event.
type OrderItemData struct {
	ProductID  string `json:"productID"`
	Quantity   int    `json:"quantity"`
	PriceCents int    `json:"priceCents"`
}

// OrderReservedData is the payload for order.reserved events.
type OrderReservedData struct {
	ReservedItems []string `json:"reservedItems"`
}

// OrderPaymentInitiatedData is the payload for order.payment_initiated events.
type OrderPaymentInitiatedData struct {
	CheckoutURL     string `json:"checkoutURL"`
	PaymentProvider string `json:"paymentProvider"`
}

// OrderPaymentCompletedData is the payload for order.payment_completed events.
type OrderPaymentCompletedData struct {
	PaymentID   string `json:"paymentID,omitempty"`
	AmountCents int    `json:"amountCents"`
}

// OrderCompletedData is the payload for order.completed events.
type OrderCompletedData struct {
	CompletedAt string `json:"completedAt"`
}

// OrderFailedData is the payload for order.failed events.
type OrderFailedData struct {
	FailureReason string `json:"failureReason"`
	FailedStep    string `json:"failedStep"`
}

// OrderCancelledData is the payload for order.cancelled events.
type OrderCancelledData struct {
	CancelReason string `json:"cancelReason"`
	RefundStatus string `json:"refundStatus"`
}
