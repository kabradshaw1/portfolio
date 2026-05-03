package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kabradshaw1/portfolio/go/coding-exercises-mcp-service/internal/coding"
	"github.com/kabradshaw1/portfolio/go/coding-exercises-mcp-service/internal/store"
)

const workflowResourceURI = "coding-exercises://workflow"

type CodingService interface {
	ImportMaterial(context.Context) (coding.ImportResult, error)
	ListTopics(context.Context) ([]store.Topic, error)
	GetNextQuestion(context.Context, store.QuestionFilter) (store.Question, error)
	SubmitAnswer(context.Context, int64, string) (coding.AnswerReview, error)
	RecordFeedback(context.Context, coding.FeedbackInput) error
	ProgressSummary(context.Context) (store.ProgressSummary, error)
	UpdateExpectedAnswer(context.Context, int64, string) error
}

type codingSession struct {
	Tier         int                   `json:"tier,omitempty"`
	Category     string                `json:"category,omitempty"`
	Instructions string                `json:"instructions"`
	Topics       []store.Topic         `json:"topics"`
	Progress     store.ProgressSummary `json:"progress"`
	NextExercise store.Question        `json:"next_exercise"`
}

type codingTurn struct {
	Review                   coding.AnswerReview `json:"review"`
	NextExercise             store.Question      `json:"next_exercise"`
	Tier                     int                 `json:"tier,omitempty"`
	Category                 string              `json:"category,omitempty"`
	PreviousFeedbackRecorded bool                `json:"previous_feedback_recorded"`
}

func New(service CodingService) *sdkmcp.Server {
	srv := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "coding-exercises-mcp-service", Version: "0.1.0"}, nil)
	addPrompt(srv, "coding_exercises", "Coding Exercises", "Start a coding exercise review session.", codingExercisesPromptHandler())
	addResource(srv, workflowResourceURI, "Coding Exercises Workflow", "Workflow instructions for agent-led coding exercise review sessions.", workflowResourceHandler())
	addTool(srv, "start_coding_exercise_session", "Start a coding exercise session with workflow instructions, progress, topics, and the next exercise.", startCodingExerciseSessionSchema(), startCodingExerciseSessionHandler(service))
	addTool(srv, "import_coding_exercise_material", "Import or refresh coding exercises from markdown.", emptySchema(), importMaterialHandler(service))
	addTool(srv, "list_coding_exercise_topics", "List coding exercise topics and progress counts.", emptySchema(), listTopicsHandler(service))
	addTool(srv, "get_next_coding_exercise", "Return the next unseen or weak coding exercise.", nextQuestionSchema(), nextQuestionHandler(service))
	addTool(srv, "submit_coding_review", "Store a coding review summary and return expected design data.", submitCodingReviewSchema(), submitCodingReviewHandler(service))
	addTool(srv, "submit_coding_review_and_prepare_next", "Store the current coding review, optionally record previous feedback, and return expected design data plus the next exercise.", submitCodingReviewAndPrepareNextSchema(), submitCodingReviewAndPrepareNextHandler(service))
	addTool(srv, "record_coding_review_feedback", "Store critique and score for a coding review attempt.", recordFeedbackSchema(), recordFeedbackHandler(service))
	addTool(srv, "get_coding_exercise_progress_summary", "Return recent attempts and weak-topic summary.", emptySchema(), progressSummaryHandler(service))
	addTool(srv, "add_or_update_coding_exercise_expected_design", "Save an expected design for a coding exercise.", updateExpectedDesignSchema(), updateExpectedDesignHandler(service))
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

func startCodingExerciseSessionHandler(service CodingService) sdkmcp.ToolHandler {
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
		return jsonResult(codingSession{
			Tier:         in.Tier,
			Category:     filter.Category,
			Instructions: codingWorkflowInstructions(),
			Topics:       topics,
			Progress:     progress,
			NextExercise: nextQuestion,
		}), nil
	}
}

func importMaterialHandler(service CodingService) sdkmcp.ToolHandler {
	return func(ctx context.Context, _ *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		result, err := service.ImportMaterial(ctx)
		return resultOrError(result, err), nil
	}
}

func listTopicsHandler(service CodingService) sdkmcp.ToolHandler {
	return func(ctx context.Context, _ *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		result, err := service.ListTopics(ctx)
		return resultOrError(result, err), nil
	}
}

func nextQuestionHandler(service CodingService) sdkmcp.ToolHandler {
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

func submitCodingReviewHandler(service CodingService) sdkmcp.ToolHandler {
	type input struct {
		QuestionID    int64  `json:"question_id"`
		ReviewSummary string `json:"review_summary"`
	}
	return func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		var in input
		if err := decodeArgs(req, &in); err != nil {
			return toolError(err.Error()), nil
		}
		result, err := service.SubmitAnswer(ctx, in.QuestionID, in.ReviewSummary)
		return resultOrError(result, err), nil
	}
}

func submitCodingReviewAndPrepareNextHandler(service CodingService) sdkmcp.ToolHandler {
	type input struct {
		QuestionID       int64                 `json:"question_id"`
		ReviewSummary    string                `json:"review_summary"`
		Tier             int                   `json:"tier,omitempty"`
		Category         string                `json:"category,omitempty"`
		PreviousFeedback *coding.FeedbackInput `json:"previous_feedback,omitempty"`
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

		review, err := service.SubmitAnswer(ctx, in.QuestionID, in.ReviewSummary)
		if err != nil {
			return toolError(err.Error()), nil
		}
		category := normalizeCategory(in.Category)
		nextQuestion, err := service.GetNextQuestion(ctx, store.QuestionFilter{Tier: in.Tier, Category: category})
		if err != nil {
			return toolError(err.Error()), nil
		}
		return jsonResult(codingTurn{
			Review:                   review,
			NextExercise:             nextQuestion,
			Tier:                     in.Tier,
			Category:                 category,
			PreviousFeedbackRecorded: previousFeedbackRecorded,
		}), nil
	}
}

func recordFeedbackHandler(service CodingService) sdkmcp.ToolHandler {
	return func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		var in coding.FeedbackInput
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

func progressSummaryHandler(service CodingService) sdkmcp.ToolHandler {
	return func(ctx context.Context, _ *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		result, err := service.ProgressSummary(ctx)
		return resultOrError(result, err), nil
	}
}

func updateExpectedDesignHandler(service CodingService) sdkmcp.ToolHandler {
	type input struct {
		QuestionID     int64  `json:"question_id"`
		ExpectedDesign string `json:"expected_design"`
	}
	return func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		var in input
		if err := decodeArgs(req, &in); err != nil {
			return toolError(err.Error()), nil
		}
		if err := service.UpdateExpectedAnswer(ctx, in.QuestionID, in.ExpectedDesign); err != nil {
			return toolError(err.Error()), nil
		}
		return jsonResult(map[string]bool{"ok": true}), nil
	}
}

func codingExercisesPromptHandler() sdkmcp.PromptHandler {
	return func(context.Context, *sdkmcp.GetPromptRequest) (*sdkmcp.GetPromptResult, error) {
		return &sdkmcp.GetPromptResult{
			Description: "Start a coding exercise review session.",
			Messages: []*sdkmcp.PromptMessage{{
				Role:    sdkmcp.Role("user"),
				Content: &sdkmcp.TextContent{Text: "Start coding exercises. Call start_coding_exercise_session with tier=1 unless the user requested a different tier, and category=coding unless the user requested a different category. Use the returned workflow instructions and next_exercise, then continue one exercise at a time within that tier and category."},
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
				Text:     codingWorkflowInstructions(),
			}},
		}, nil
	}
}

func codingWorkflowInstructions() string {
	return strings.TrimSpace(`
# Coding Exercises Workflow

When the user asks for coding exercises, use this MCP server as the source of durable coding exercise state.

1. Call start_coding_exercise_session.
2. Respect the requested tier and requested category. Pass both values to start_coding_exercise_session, get_next_coding_exercise, and submit_coding_review_and_prepare_next whenever the user asked for them.
3. If next_exercise.kind is "coding_exercise", present next_exercise.prompt as an implementation task. Tell the user to implement it in the repo or workspace and respond when ready for review. Do not ask for a prose answer in chat.
4. When the user is ready, inspect the relevant files and tests before calling submit_coding_review_and_prepare_next. The review_summary must mention the files reviewed, tests run, correctness against the prompt, concurrency safety, idiomatic Go, edge cases, test quality, and simplicity.
5. If next_exercise.kind is "qa", ask next_exercise.prompt exactly enough to quiz the user. Do not reveal expected_answer before the user answers.
6. After reviewing a coding_exercise implementation, call exactly one MCP tool: submit_coding_review_and_prepare_next with question_id, review_summary, tier, category, and any previous_feedback payload you prepared for the prior review.
7. Do not call record_coding_review_feedback or get_next_coding_exercise separately in the same turn.
8. Compare review.review_summary to review.expected_answer. Point out missing or inaccurate implementation details clearly.
9. Decide concise missing_points, inaccurate_points, suggested_answer, and the 0-3 score for the current review, but do not call record_coding_review_feedback yet.
10. Keep that feedback payload in context and send it as previous_feedback on the next submit_coding_review_and_prepare_next call.
11. Present the next coding_exercise prompt as a repo implementation task and wait for the user to write code.

Use list_coding_exercise_topics and get_coding_exercise_progress_summary when the user asks about coverage, weak areas, or progress.
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

func startCodingExerciseSessionSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"tier":{"type":"integer","minimum":1,"maximum":3},"category":{"type":"string"}},"additionalProperties":false}`)
}

func nextQuestionSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"tier":{"type":"integer","minimum":1,"maximum":3},"category":{"type":"string"}},"additionalProperties":false}`)
}

func submitCodingReviewSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"question_id":{"type":"integer"},"review_summary":{"type":"string"}},"required":["question_id","review_summary"],"additionalProperties":false}`)
}

func submitCodingReviewAndPrepareNextSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"question_id":{"type":"integer"},"review_summary":{"type":"string"},"tier":{"type":"integer","minimum":1,"maximum":3},"category":{"type":"string"},"previous_feedback":{"type":"object","properties":{"attempt_id":{"type":"integer"},"score":{"type":"integer","minimum":0,"maximum":3},"missing_points":{"type":"string"},"inaccurate_points":{"type":"string"},"suggested_answer":{"type":"string"}},"required":["attempt_id","score"],"additionalProperties":false}},"required":["question_id","review_summary"],"additionalProperties":false}`)
}

func recordFeedbackSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"attempt_id":{"type":"integer"},"score":{"type":"integer","minimum":0,"maximum":3},"missing_points":{"type":"string"},"inaccurate_points":{"type":"string"},"suggested_answer":{"type":"string"}},"required":["attempt_id","score"],"additionalProperties":false}`)
}

func updateExpectedDesignSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"question_id":{"type":"integer"},"expected_design":{"type":"string"}},"required":["question_id","expected_design"],"additionalProperties":false}`)
}
