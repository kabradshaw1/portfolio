package productclient

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/kabradshaw1/portfolio/go/cart-service/internal/metrics"
	"github.com/kabradshaw1/portfolio/go/cart-service/internal/model"
	pb "github.com/kabradshaw1/portfolio/go/product-service/pb/product/v1"
)

type GRPCClient struct {
	client pb.ProductServiceClient
	conn   *grpc.ClientConn
}

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

func (c *GRPCClient) ValidateProduct(ctx context.Context, productID uuid.UUID) error {
	_, err := c.client.GetProduct(ctx, &pb.GetProductRequest{
		Id: productID.String(),
	})
	if err != nil {
		metrics.ProductValidation.WithLabelValues("not_found").Inc()
		return fmt.Errorf("product not found: %w", err)
	}
	return nil
}

func (c *GRPCClient) EnrichCartItems(ctx context.Context, items []model.CartItem) []model.CartItem {
	for i := range items {
		resp, err := c.client.GetProduct(ctx, &pb.GetProductRequest{
			Id: items[i].ProductID.String(),
		})
		if err != nil {
			slog.Warn("failed to enrich cart item", "productID", items[i].ProductID, "error", err)
			continue
		}
		items[i].ProductName = resp.Name
		items[i].ProductPrice = int(resp.Price)
		items[i].ProductImage = resp.ImageUrl
	}
	return items
}

func (c *GRPCClient) Close() error {
	return c.conn.Close()
}
