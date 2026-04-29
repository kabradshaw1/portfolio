package mcp

import (
	"context"
	"errors"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kabradshaw1/portfolio/go/ai-service/internal/jwtctx"
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/mcp/resources"
)

func registerResources(srv *sdkmcp.Server, reg *resources.Registry, defaults Defaults) {
	handler := makeResourceHandler(reg, defaults)
	for _, res := range reg.List() {
		srv.AddResource(
			&sdkmcp.Resource{
				URI:         res.URI(),
				Name:        res.Name(),
				Description: res.Description(),
				MIMEType:    res.MIMEType(),
			},
			handler,
		)
	}
	if reg.HasCatalogClient() {
		srv.AddResourceTemplate(
			&sdkmcp.ResourceTemplate{
				URITemplate: "catalog://product/{id}",
				Name:        "Product detail",
				Description: "Single product detail by product id.",
				MIMEType:    "application/json",
			},
			handler,
		)
	}
}

func makeResourceHandler(reg *resources.Registry, defaults Defaults) sdkmcp.ResourceHandler {
	return func(ctx context.Context, req *sdkmcp.ReadResourceRequest) (*sdkmcp.ReadResourceResult, error) {
		if jwtctx.UserID(ctx) == "" && defaults.UserID != "" {
			ctx = jwtctx.WithUserID(ctx, defaults.UserID)
			ctx = jwtctx.WithJWT(ctx, defaults.JWT)
		}
		content, err := reg.Read(ctx, req.Params.URI)
		if err != nil {
			if errors.Is(err, resources.ErrResourceNotFound) {
				return nil, sdkmcp.ResourceNotFoundError(req.Params.URI)
			}
			return nil, err
		}
		return &sdkmcp.ReadResourceResult{
			Contents: []*sdkmcp.ResourceContents{{
				URI:      content.URI,
				MIMEType: content.MIMEType,
				Text:     content.Text,
			}},
		}, nil
	}
}
