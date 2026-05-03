package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kabradshaw1/portfolio/go/qa-mcp-service/internal/qa"
	"github.com/kabradshaw1/portfolio/go/qa-mcp-service/internal/store"
)

const workflowResourceURI = "qa://workflow"

type QAService interface {
	ImportMaterial(context.Context) (qa.ImportResult, error)
	ListTopics(context.Context) ([]store.Topic, error)
	GetNextQuestion(context.Context, store.QuestionFilter) (store.Question, error)
	SubmitAnswer(context.Context, int64, string) (qa.AnswerReview, error)
	RecordFeedback(context.Context, qa.FeedbackInput) error
	ProgressSummary(context.Context) (store.ProgressSummary, error)
	UpdateExpectedAnswer(context.Context, int64, string) error
}

type qaSession struct {
	Tier         int                   `json:"tier,omitempty"`
	Category     string                `json:"category,omitempty"`
	Instructions string                `json:"instructions"`
	Topics       []store.Topic         `json:"topics"`
	Progress     store.ProgressSummary `json:"progress"`
	NextQuestion store.Question        `json:"next_question"`
}

type qaTurn struct {
	Review                   qa.AnswerReview `json:"review"`
	NextQuestion             store.Question  `json:"next_question"`
	Tier                     int             `json:"tier,omitempty"`
	Category                 string          `json:"category,omitempty"`
	PreviousFeedbackRecorded bool            `json:"previous_feedback_recorded"`
}

func New(service QAService) *sdkmcp.Server {
	srv := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "qa-mcp-service", Version: "0.1.0"}, nil)
	addPrompt(srv, "qa", "QA Interview Practice", "Start a chat-based interview Q&A practice session.", qaPromptHandler())
	addResource(srv, workflowResourceURI, "QA Workflow", "Workflow instructions for agent-led interview Q&A sessions.", workflowResourceHandler())
	addTool(srv, "start_qa_session", "Start a QA session with workflow instructions, progress, topics, and the next question.", startQASessionSchema(), startQASessionHandler(service))
	addTool(srv, "import_qa_material", "Import or refresh QA questions from markdown.", emptySchema(), importMaterialHandler(service))
	addTool(srv, "list_qa_topics", "List QA topics and progress counts.", emptySchema(), listTopicsHandler(service))
	addTool(srv, "get_next_qa_question", "Return the next unseen or weak QA question.", nextQuestionSchema(), nextQuestionHandler(service))
	addTool(srv, "submit_qa_answer", "Store a QA answer attempt and return expected answer data.", submitAnswerSchema(), submitAnswerHandler(service))
	addTool(srv, "submit_qa_answer_and_prepare_next", "Store the current QA answer, optionally record previous feedback, and return expected answer data plus the next question.", submitAnswerAndPrepareNextSchema(), submitAnswerAndPrepareNextHandler(service))
	addTool(srv, "record_qa_feedback", "Store critique and score for a QA answer attempt.", recordFeedbackSchema(), recordFeedbackHandler(service))
	addTool(srv, "get_qa_progress_summary", "Return recent attempts and weak-topic summary.", emptySchema(), progressSummaryHandler(service))
	addTool(srv, "add_or_update_qa_expected_answer", "Save an expected answer for a QA question.", updateExpectedAnswerSchema(), updateExpectedAnswerHandler(service))
	return srv
}

func addPrompt(srv *sdkmcp.Server, name, title, description string, handler sdkmcp.PromptHandler) {
	srv.AddPrompt(&sdkmcp.Prompt{Name: name, Title: title, Description: description}, handler)
}

func addResource(srv *sdkmcp.Server, uri, name, description string, handler sdkmcp.ResourceHandler) {
	srv.AddResource(&sdkmcp.Resource{URI: uri, Name: name, Title: name, Description: description, MIMEType: "text/markdown"}, handler)
}

func addTool(srv *sdkmcp.Server, name, description string, schema json.RawMessage, handler sdkmcp.ToolHandler) {
	srv.AddTool(&sdkmcp.Tool{Name: name, Description: description, InputSchema: schema}, handler)
}

func startQASessionHandler(service QAService) sdkmcp.ToolHandler {
	type input struct {
		Tier     int    `json:"tier,omitempty"`
		Category string `json:"category,omitempty"`
	}
	return func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		var in input
		if req != nil && req.Params != nil && len(req.Params.Arguments) > 0 {
			if err := decodeArgs(req, &in); err != nil {
				return toolError(err.Error()), nil
			}
		}
		if !validTier(in.Tier) {
			return toolError("tier must be 1, 2, or 3"), nil
		}
		topics, err := service.ListTopics(ctx)
		if err != nil {
			return toolError(err.Error()), nil
		}
		progress, err := service.ProgressSummary(ctx)
		if err != nil {
			return toolError(err.Error()), nil
		}
		filter := store.QuestionFilter{Tier: in.Tier, Category: normalizeCategory(in.Category)}
		nextQuestion, err := service.GetNextQuestion(ctx, filter)
		if err != nil {
			return toolError(err.Error()), nil
		}
		return jsonResult(qaSession{
			Tier:         in.Tier,
			Category:     filter.Category,
			Instructions: qaWorkflowInstructions(),
			Topics:       topics,
			Progress:     progress,
			NextQuestion: nextQuestion,
		}), nil
	}
}

func importMaterialHandler(service QAService) sdkmcp.ToolHandler {
	return func(ctx context.Context, _ *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		result, err := service.ImportMaterial(ctx)
		return resultOrError(result, err), nil
	}
}

func listTopicsHandler(service QAService) sdkmcp.ToolHandler {
	return func(ctx context.Context, _ *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		result, err := service.ListTopics(ctx)
		return resultOrError(result, err), nil
	}
}

func nextQuestionHandler(service QAService) sdkmcp.ToolHandler {
	type input struct {
		Tier     int    `json:"tier,omitempty"`
		Category string `json:"category,omitempty"`
	}
	return func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		var in input
		if req != nil && req.Params != nil && len(req.Params.Arguments) > 0 {
			if err := decodeArgs(req, &in); err != nil {
				return toolError(err.Error()), nil
			}
		}
		if !validTier(in.Tier) {
			return toolError("tier must be 1, 2, or 3"), nil
		}
		result, err := service.GetNextQuestion(ctx, store.QuestionFilter{Tier: in.Tier, Category: normalizeCategory(in.Category)})
		return resultOrError(result, err), nil
	}
}

func submitAnswerHandler(service QAService) sdkmcp.ToolHandler {
	type input struct {
		QuestionID int64  `json:"question_id"`
		Answer     string `json:"answer"`
	}
	return func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		var in input
		if err := decodeArgs(req, &in); err != nil {
			return toolError(err.Error()), nil
		}
		result, err := service.SubmitAnswer(ctx, in.QuestionID, in.Answer)
		return resultOrError(result, err), nil
	}
}

func submitAnswerAndPrepareNextHandler(service QAService) sdkmcp.ToolHandler {
	type input struct {
		QuestionID       int64             `json:"question_id"`
		Answer           string            `json:"answer"`
		Tier             int               `json:"tier,omitempty"`
		Category         string            `json:"category,omitempty"`
		PreviousFeedback *qa.FeedbackInput `json:"previous_feedback,omitempty"`
	}
	return func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		var in input
		if err := decodeArgs(req, &in); err != nil {
			return toolError(err.Error()), nil
		}
		if !validTier(in.Tier) {
			return toolError("tier must be 1, 2, or 3"), nil
		}

		previousFeedbackRecorded := false
		if in.PreviousFeedback != nil {
			if in.PreviousFeedback.Score < 0 || in.PreviousFeedback.Score > 3 {
				return toolError("previous_feedback.score must be between 0 and 3"), nil
			}
			if err := service.RecordFeedback(ctx, *in.PreviousFeedback); err != nil {
				return toolError(err.Error()), nil
			}
			previousFeedbackRecorded = true
		}

		review, err := service.SubmitAnswer(ctx, in.QuestionID, in.Answer)
		if err != nil {
			return toolError(err.Error()), nil
		}
		category := normalizeCategory(in.Category)
		nextQuestion, err := service.GetNextQuestion(ctx, store.QuestionFilter{Tier: in.Tier, Category: category})
		if err != nil {
			return toolError(err.Error()), nil
		}
		return jsonResult(qaTurn{
			Review:                   review,
			NextQuestion:             nextQuestion,
			Tier:                     in.Tier,
			Category:                 category,
			PreviousFeedbackRecorded: previousFeedbackRecorded,
		}), nil
	}
}

func recordFeedbackHandler(service QAService) sdkmcp.ToolHandler {
	return func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		var in qa.FeedbackInput
		if err := decodeArgs(req, &in); err != nil {
			return toolError(err.Error()), nil
		}
		if in.Score < 0 || in.Score > 3 {
			return toolError("score must be between 0 and 3"), nil
		}
		if err := service.RecordFeedback(ctx, in); err != nil {
			return toolError(err.Error()), nil
		}
		return jsonResult(map[string]bool{"ok": true}), nil
	}
}

func progressSummaryHandler(service QAService) sdkmcp.ToolHandler {
	return func(ctx context.Context, _ *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		result, err := service.ProgressSummary(ctx)
		return resultOrError(result, err), nil
	}
}

func updateExpectedAnswerHandler(service QAService) sdkmcp.ToolHandler {
	type input struct {
		QuestionID int64  `json:"question_id"`
		Answer     string `json:"answer"`
	}
	return func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		var in input
		if err := decodeArgs(req, &in); err != nil {
			return toolError(err.Error()), nil
		}
		if err := service.UpdateExpectedAnswer(ctx, in.QuestionID, in.Answer); err != nil {
			return toolError(err.Error()), nil
		}
		return jsonResult(map[string]bool{"ok": true}), nil
	}
}

func qaPromptHandler() sdkmcp.PromptHandler {
	return func(context.Context, *sdkmcp.GetPromptRequest) (*sdkmcp.GetPromptResult, error) {
		return &sdkmcp.GetPromptResult{
			Description: "Start a chat-based interview Q&A practice session.",
			Messages: []*sdkmcp.PromptMessage{{
				Role:    sdkmcp.Role("user"),
				Content: &sdkmcp.TextContent{Text: "Start QA tier 1. Call start_qa_session with tier=1 unless the user requested a different tier, and pass category when the user names a category such as golang, api, distributed, ai, integrations, db_observability_security, performance_concurrency, portfolio, or mock_interview. Use the returned workflow instructions and next_question, then continue one question at a time within that tier and category."},
			}},
		}, nil
	}
}

func workflowResourceHandler() sdkmcp.ResourceHandler {
	return func(context.Context, *sdkmcp.ReadResourceRequest) (*sdkmcp.ReadResourceResult, error) {
		return &sdkmcp.ReadResourceResult{
			Contents: []*sdkmcp.ResourceContents{{
				URI:      workflowResourceURI,
				MIMEType: "text/markdown",
				Text:     qaWorkflowInstructions(),
			}},
		}, nil
	}
}

func qaWorkflowInstructions() string {
	return strings.TrimSpace(`
# QA Workflow

When the user asks for interview Q&A practice, use this MCP server as the source of durable QA state.

1. Call start_qa_session.
2. Respect the requested tier and requested category. Pass both values to start_qa_session, get_next_qa_question, and submit_qa_answer_and_prepare_next whenever the user asked for them.
3. Ask next_question.prompt exactly enough to quiz the user. Do not reveal expected_answer before the user answers.
4. After the user answers a QA question, call exactly one MCP tool: submit_qa_answer_and_prepare_next with question_id, answer, tier, category, and any previous_feedback payload you prepared for the prior answer.
5. Do not call record_qa_feedback or get_next_qa_question separately in the same turn.
6. Compare review.answer to review.expected_answer. Point out missing or inaccurate parts clearly.
7. Respond in this teaching-first style:
   Score: X/3
   Explanation: explain the concept thoroughly, including why the expected answer is correct, key distinctions, tradeoffs, and concrete examples when helpful. Include a small Go code snippet when it would make the concept clearer. Do not force code when the idea is better explained in prose. Do not compress this section just because the next question is included.
   Interview answer: provide a polished answer the user could say in an interview.
   Minimum answer, only when useful: provide a short memorization-friendly fallback answer.
   Memory hook, only when useful: provide a compact phrase that helps the user remember the idea.
8. Decide concise missing_points, inaccurate_points, suggested_answer, and the 0-3 score for the current answer, but do not call record_qa_feedback yet.
9. Keep that feedback payload in context and send it as previous_feedback on the next submit_qa_answer_and_prepare_next call.
10. Ask the next QA prompt in the same response as the feedback so the user never has to ask for the next question.

Use list_qa_topics and get_qa_progress_summary when the user asks about coverage, weak areas, or progress.
`)
}

func decodeArgs(req *sdkmcp.CallToolRequest, out any) error {
	if req == nil || req.Params == nil || len(req.Params.Arguments) == 0 {
		return fmt.Errorf("arguments are required")
	}
	if err := json.Unmarshal(req.Params.Arguments, out); err != nil {
		return fmt.Errorf("invalid arguments: %w", err)
	}
	return nil
}

func resultOrError(result any, err error) *sdkmcp.CallToolResult {
	if err != nil {
		return toolError(err.Error())
	}
	return jsonResult(result)
}

func jsonResult(result any) *sdkmcp.CallToolResult {
	data, err := json.Marshal(result)
	if err != nil {
		return toolError("result not serializable: " + err.Error())
	}
	return &sdkmcp.CallToolResult{
		Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: string(data)}},
	}
}

func toolError(message string) *sdkmcp.CallToolResult {
	return &sdkmcp.CallToolResult{
		Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: message}},
		IsError: true,
	}
}

func validTier(tier int) bool {
	return tier >= 0 && tier <= 3
}

func normalizeCategory(category string) string {
	return strings.ToLower(strings.TrimSpace(category))
}

func emptySchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","additionalProperties":false}`)
}

func startQASessionSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"tier":{"type":"integer","minimum":1,"maximum":3},"category":{"type":"string"}},"additionalProperties":false}`)
}

func nextQuestionSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"tier":{"type":"integer","minimum":1,"maximum":3},"category":{"type":"string"}},"additionalProperties":false}`)
}

func submitAnswerSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"question_id":{"type":"integer"},"answer":{"type":"string"}},"required":["question_id","answer"],"additionalProperties":false}`)
}

func submitAnswerAndPrepareNextSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"question_id":{"type":"integer"},"answer":{"type":"string"},"tier":{"type":"integer","minimum":1,"maximum":3},"category":{"type":"string"},"previous_feedback":{"type":"object","properties":{"attempt_id":{"type":"integer"},"score":{"type":"integer","minimum":0,"maximum":3},"missing_points":{"type":"string"},"inaccurate_points":{"type":"string"},"suggested_answer":{"type":"string"}},"required":["attempt_id","score"],"additionalProperties":false}},"required":["question_id","answer"],"additionalProperties":false}`)
}

func recordFeedbackSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"attempt_id":{"type":"integer"},"score":{"type":"integer","minimum":0,"maximum":3},"missing_points":{"type":"string"},"inaccurate_points":{"type":"string"},"suggested_answer":{"type":"string"}},"required":["attempt_id","score"],"additionalProperties":false}`)
}

func updateExpectedAnswerSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"question_id":{"type":"integer"},"answer":{"type":"string"}},"required":["question_id","answer"],"additionalProperties":false}`)
}
