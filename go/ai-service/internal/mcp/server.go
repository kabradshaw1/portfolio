// Package mcp provides adapters between the ai-service's tools.Registry and
// the MCP protocol, using the official modelcontextprotocol/go-sdk.
package mcp

import (
	"context"
	"encoding/json"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kabradshaw1/portfolio/go/ai-service/internal/jwtctx"
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/mcp/prompts"
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/mcp/resources"
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/tools"
)

// Defaults holds fallback auth values for environments without per-request
// auth (stdio mode). In HTTP mode these are empty — the middleware injects
// per-request values instead.
type Defaults struct {
	UserID string // from AI_SERVICE_TOKEN JWT sub claim
	JWT    string // raw AI_SERVICE_TOKEN value, forwarded to ecommerce-service
}

type Option func(*serverOptions)

type serverOptions struct {
	resources *resources.Registry
	prompts   *prompts.Registry
}

// WithResources registers MCP Resources on the server.
func WithResources(reg *resources.Registry) Option {
	return func(opts *serverOptions) {
		opts.resources = reg
	}
}

// WithPrompts registers server-provided MCP Prompts on the server.
func WithPrompts(reg *prompts.Registry) Option {
	return func(opts *serverOptions) {
		opts.prompts = reg
	}
}

// NewServer creates an MCP server that exposes every tool in reg, plus optional
// Resource and Prompt registries.
func NewServer(reg tools.Registry, defaults Defaults, opts ...Option) *sdkmcp.Server {
	options := serverOptions{}
	for _, opt := range opts {
		opt(&options)
	}

	srv := sdkmcp.NewServer(&sdkmcp.Implementation{
		Name:    "ai-service",
		Version: "1.1.0",
	}, nil)

	for _, t := range reg.All() {
		registerTool(srv, t, defaults)
	}
	if options.resources != nil {
		registerResources(srv, options.resources, defaults)
	}
	if options.prompts != nil {
		registerPrompts(srv, options.prompts)
	}
	return srv
}

func registerTool(srv *sdkmcp.Server, t tools.Tool, defaults Defaults) {
	srv.AddTool(
		&sdkmcp.Tool{
			Name:        t.Name(),
			Description: t.Description(),
			InputSchema: t.Schema(),
		},
		makeHandler(t, defaults),
	)
}

func makeHandler(t tools.Tool, defaults Defaults) sdkmcp.ToolHandler {
	return func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		// req.Params.Arguments is already json.RawMessage from the SDK.
		args := req.Params.Arguments
		if args == nil {
			args = json.RawMessage("{}")
		}

		userID := UserIDFromContext(ctx)
		if userID == "" && defaults.UserID != "" {
			userID = defaults.UserID
			ctx = jwtctx.WithJWT(ctx, defaults.JWT)
		}

		result, err := t.Call(ctx, args, userID)
		if err != nil {
			return errorResult(err.Error()), nil
		}

		content, err := json.Marshal(result.Content)
		if err != nil {
			return errorResult("result not serializable: " + err.Error()), nil
		}

		return &sdkmcp.CallToolResult{
			Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: string(content)}},
		}, nil
	}
}

func errorResult(msg string) *sdkmcp.CallToolResult {
	return &sdkmcp.CallToolResult{
		Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: msg}},
		IsError: true,
	}
}

// --- context helpers ---

type contextKey string

const userIDCtxKey contextKey = "mcp_user_id"

// WithUserID returns a context carrying the authenticated user's ID.
func WithUserID(ctx context.Context, uid string) context.Context {
	return context.WithValue(ctx, userIDCtxKey, uid)
}

// UserIDFromContext extracts the user ID set by WithUserID.
func UserIDFromContext(ctx context.Context) string {
	uid, _ := ctx.Value(userIDCtxKey).(string)
	return uid
}
