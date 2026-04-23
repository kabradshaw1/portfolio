package cartclient

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	cart "github.com/kabradshaw1/portfolio/go/cart-service/pb/cart/v1"
	"github.com/kabradshaw1/portfolio/go/order-service/internal/model"
	"github.com/kabradshaw1/portfolio/go/pkg/grpcmetrics"
	pb "github.com/kabradshaw1/portfolio/go/product-service/pb/product/v1"
)

// GRPCClient wraps cart-service and product-service gRPC clients.
// It satisfies the OrderService's CartClient interface.
type GRPCClient struct {
	cartClient    cart.CartServiceClient
	productClient pb.ProductServiceClient
	cartConn      *grpc.ClientConn
	productConn   *grpc.ClientConn
}

// New dials both cart-service and product-service gRPC endpoints.
func New(cartAddr, productAddr string, creds credentials.TransportCredentials) (*GRPCClient, error) {
	cartConn, err := grpc.NewClient(cartAddr,
		grpc.WithTransportCredentials(creds),
		grpc.WithUnaryInterceptor(grpcmetrics.UnaryClientInterceptor("cart-service")),
	)
	if err != nil {
		return nil, fmt.Errorf("connect to cart-service: %w", err)
	}

	productConn, err := grpc.NewClient(productAddr,
		grpc.WithTransportCredentials(creds),
		grpc.WithUnaryInterceptor(grpcmetrics.UnaryClientInterceptor("product-service")),
	)
	if err != nil {
		cartConn.Close()
		return nil, fmt.Errorf("connect to product-service: %w", err)
	}

	return &GRPCClient{
		cartClient:    cart.NewCartServiceClient(cartConn),
		productClient: pb.NewProductServiceClient(productConn),
		cartConn:      cartConn,
		productConn:   productConn,
	}, nil
}

// GetByUser fetches cart items via gRPC and enriches with product prices.
func (c *GRPCClient) GetByUser(ctx context.Context, userID uuid.UUID) ([]model.CartItem, error) {
	resp, err := c.cartClient.GetCart(ctx, &cart.GetCartRequest{
		UserId: userID.String(),
	})
	if err != nil {
		return nil, fmt.Errorf("get cart via gRPC: %w", err)
	}

	items := make([]model.CartItem, len(resp.Items))
	for i, item := range resp.Items {
		pid, _ := uuid.Parse(item.ProductId)
		iid, _ := uuid.Parse(item.Id)
		items[i] = model.CartItem{
			ID:        iid,
			UserID:    userID,
			ProductID: pid,
			Quantity:  int(item.Quantity),
		}
	}

	// Enrich with product prices for order total calculation.
	for i := range items {
		product, err := c.productClient.GetProduct(ctx, &pb.GetProductRequest{
			Id: items[i].ProductID.String(),
		})
		if err != nil {
			return nil, fmt.Errorf("get product price for cart item: %w", err)
		}
		items[i].ProductPrice = int(product.Price)
		items[i].ProductName = product.Name
	}

	return items, nil
}

// ClearCart deletes all cart items for a user via gRPC.
func (c *GRPCClient) ClearCart(ctx context.Context, userID uuid.UUID) error {
	_, err := c.cartClient.ClearCart(ctx, &cart.ClearCartRequest{
		UserId: userID.String(),
	})
	if err != nil {
		return fmt.Errorf("clear cart via gRPC: %w", err)
	}
	return nil
}

// Close shuts down both gRPC connections.
func (c *GRPCClient) Close() error {
	err1 := c.cartConn.Close()
	err2 := c.productConn.Close()
	if err1 != nil {
		return err1
	}
	return err2
}
