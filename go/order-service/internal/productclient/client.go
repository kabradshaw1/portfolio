package productclient

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/kabradshaw1/portfolio/go/product-service/pb/product/v1"
)

// GRPCClient wraps the product-service gRPC client, satisfying the
// worker.ProductClient interface so the order processor can call
// product-service over the network instead of using a local repository.
type GRPCClient struct {
	client pb.ProductServiceClient
	conn   *grpc.ClientConn
}

// New dials the product-service gRPC endpoint and returns a ready client.
func New(addr string) (*GRPCClient, error) {
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("connect to product-service: %w", err)
	}
	return &GRPCClient{
		client: pb.NewProductServiceClient(conn),
		conn:   conn,
	}, nil
}

// CheckAvailability calls the product-service CheckAvailability RPC.
func (c *GRPCClient) CheckAvailability(ctx context.Context, productID uuid.UUID, quantity int) (bool, error) {
	resp, err := c.client.CheckAvailability(ctx, &pb.CheckAvailabilityRequest{
		ProductId: productID.String(),
		Quantity:  int32(quantity),
	})
	if err != nil {
		return false, err
	}
	return resp.Available, nil
}

// DecrementStock calls the product-service DecrementStock RPC.
func (c *GRPCClient) DecrementStock(ctx context.Context, productID uuid.UUID, qty int) error {
	_, err := c.client.DecrementStock(ctx, &pb.DecrementStockRequest{
		ProductId: productID.String(),
		Quantity:  int32(qty),
	})
	return err
}

// InvalidateCache calls the product-service InvalidateCache RPC.
func (c *GRPCClient) InvalidateCache(ctx context.Context) error {
	_, err := c.client.InvalidateCache(ctx, &pb.InvalidateCacheRequest{})
	return err
}

// Close shuts down the underlying gRPC connection.
func (c *GRPCClient) Close() error {
	return c.conn.Close()
}
