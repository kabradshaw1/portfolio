package mcp

import (
	"context"
	"errors"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kabradshaw1/portfolio/go/ai-service/internal/mcp/prompts"
)

func registerPrompts(srv *sdkmcp.Server, reg *prompts.Registry) {
	for _, prompt := range reg.List() {
		args := prompt.Arguments()
		sdkArgs := make([]*sdkmcp.PromptArgument, 0, len(args))
		for _, arg := range args {
			sdkArgs = append(sdkArgs, &sdkmcp.PromptArgument{
				Name:        arg.Name,
				Description: arg.Description,
				Required:    arg.Required,
			})
		}

		srv.AddPrompt(
			&sdkmcp.Prompt{
				Name:        prompt.Name(),
				Description: prompt.Description(),
				Arguments:   sdkArgs,
			},
			makePromptHandler(reg),
		)
	}
}

func makePromptHandler(reg *prompts.Registry) sdkmcp.PromptHandler {
	return func(ctx context.Context, req *sdkmcp.GetPromptRequest) (*sdkmcp.GetPromptResult, error) {
		rendered, err := reg.Get(ctx, req.Params.Name, req.Params.Arguments)
		if err != nil {
			if errors.Is(err, prompts.ErrPromptNotFound) {
				return nil, err
			}
			return nil, err
		}

		messages := make([]*sdkmcp.PromptMessage, 0, len(rendered.Messages))
		for _, msg := range rendered.Messages {
			messages = append(messages, &sdkmcp.PromptMessage{
				Role:    sdkmcp.Role(msg.Role),
				Content: &sdkmcp.TextContent{Text: msg.Text},
			})
		}
		return &sdkmcp.GetPromptResult{
			Description: rendered.Description,
			Messages:    messages,
		}, nil
	}
}
