package resources

import (
	"context"
	"fmt"

	"github.com/kabradshaw1/portfolio/go/ai-service/internal/jwtctx"
)

const (
	uriUserOrders = "user://orders"
	uriUserCart   = "user://cart"
)

// UserClient fetches a single authenticated user's order/cart data. The data
// is returned as opaque JSON text — the resources do not re-marshal it.
type UserClient interface {
	Orders(ctx context.Context, userID string) (string, error)
	Cart(ctx context.Context, userID string) (string, error)
}

// userOrdersResource implements user://orders.
type userOrdersResource struct{ c UserClient }

// NewUserOrdersResource returns the user://orders resource. Reads scope to
// the JWT subject in the request context; anonymous reads return
// ErrResourceNotFound rather than another user's data.
func NewUserOrdersResource(c UserClient) Resource { return userOrdersResource{c: c} }
func (r userOrdersResource) URI() string          { return uriUserOrders }
func (r userOrdersResource) Name() string         { return "Your orders" }
func (r userOrdersResource) Description() string  { return "Order history for the authenticated user." }
func (r userOrdersResource) MIMEType() string     { return mimeJSON }
func (r userOrdersResource) Read(ctx context.Context) (Content, error) {
	uid := jwtctx.UserID(ctx)
	if uid == "" {
		return Content{}, ErrResourceNotFound
	}
	body, err := r.c.Orders(ctx, uid)
	if err != nil {
		return Content{}, fmt.Errorf("%s: %w", uriUserOrders, err)
	}
	return Content{URI: r.URI(), MIMEType: r.MIMEType(), Text: body}, nil
}

// userCartResource implements user://cart.
type userCartResource struct{ c UserClient }

// NewUserCartResource returns the user://cart resource. JWT-scoped like
// userOrdersResource.
func NewUserCartResource(c UserClient) Resource { return userCartResource{c: c} }
func (r userCartResource) URI() string          { return uriUserCart }
func (r userCartResource) Name() string         { return "Your cart" }
func (r userCartResource) Description() string  { return "Current cart for the authenticated user." }
func (r userCartResource) MIMEType() string     { return mimeJSON }
func (r userCartResource) Read(ctx context.Context) (Content, error) {
	uid := jwtctx.UserID(ctx)
	if uid == "" {
		return Content{}, ErrResourceNotFound
	}
	body, err := r.c.Cart(ctx, uid)
	if err != nil {
		return Content{}, fmt.Errorf("%s: %w", uriUserCart, err)
	}
	return Content{URI: r.URI(), MIMEType: r.MIMEType(), Text: body}, nil
}
