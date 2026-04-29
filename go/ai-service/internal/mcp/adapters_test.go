package mcp

import (
	"context"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kabradshaw1/portfolio/go/ai-service/internal/mcp/prompts"
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/mcp/resources"
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/tools"
)

func TestNewServerRegistersResourcesAndPrompts(t *testing.T) {
	resReg := resources.NewRegistry()
	resReg.Register(stubResource{uri: "stub://thing"})

	promptReg := prompts.NewRegistry()
	promptReg.Register(stubPrompt{name: "stub-prompt"})

	srv := NewServer(tools.NewMemRegistry(), Defaults{}, WithResources(resReg), WithPrompts(promptReg))
	session, cleanup := connectInProcess(t, srv)
	defer cleanup()

	ctx := context.Background()
	resourceList, err := session.ListResources(ctx, nil)
	if err != nil {
		t.Fatalf("list resources: %v", err)
	}
	if len(resourceList.Resources) != 1 || resourceList.Resources[0].URI != "stub://thing" {
		t.Fatalf("unexpected resources: %+v", resourceList.Resources)
	}

	resourceRead, err := session.ReadResource(ctx, &sdkmcp.ReadResourceParams{URI: "stub://thing"})
	if err != nil {
		t.Fatalf("read resource: %v", err)
	}
	if len(resourceRead.Contents) != 1 || resourceRead.Contents[0].Text != "hi" {
		t.Fatalf("unexpected resource content: %+v", resourceRead.Contents)
	}

	promptList, err := session.ListPrompts(ctx, nil)
	if err != nil {
		t.Fatalf("list prompts: %v", err)
	}
	if len(promptList.Prompts) != 1 || promptList.Prompts[0].Name != "stub-prompt" {
		t.Fatalf("unexpected prompts: %+v", promptList.Prompts)
	}

	promptGot, err := session.GetPrompt(ctx, &sdkmcp.GetPromptParams{Name: "stub-prompt"})
	if err != nil {
		t.Fatalf("get prompt: %v", err)
	}
	if len(promptGot.Messages) != 1 {
		t.Fatalf("unexpected prompt messages: %+v", promptGot.Messages)
	}
	content, ok := promptGot.Messages[0].Content.(*sdkmcp.TextContent)
	if !ok || content.Text != "hi" {
		t.Fatalf("unexpected prompt content: %+v", promptGot.Messages[0].Content)
	}
}

type stubResource struct{ uri string }

func (s stubResource) URI() string         { return s.uri }
func (s stubResource) Name() string        { return "stub" }
func (s stubResource) Description() string { return "stub resource" }
func (s stubResource) MIMEType() string    { return "text/plain" }
func (s stubResource) Read(context.Context) (resources.Content, error) {
	return resources.Content{URI: s.uri, MIMEType: "text/plain", Text: "hi"}, nil
}

type stubPrompt struct{ name string }

func (s stubPrompt) Name() string                  { return s.name }
func (s stubPrompt) Description() string           { return "stub prompt" }
func (s stubPrompt) Arguments() []prompts.Argument { return nil }
func (s stubPrompt) Render(context.Context, map[string]string) (prompts.Rendered, error) {
	return prompts.Rendered{
		Description: "stub render",
		Messages:    []prompts.Message{{Role: "user", Text: "hi"}},
	}, nil
}
