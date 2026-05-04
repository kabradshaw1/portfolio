package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kabradshaw1/portfolio/go/study-service/internal/store"
	"github.com/kabradshaw1/portfolio/go/study-service/internal/study"
)

type StudyService interface {
	ImportMaterial(context.Context) (study.ImportResult, error)
	ListTopics(context.Context) ([]store.Topic, error)
	GetNextQuestion(context.Context) (store.Question, error)
	SubmitAnswer(context.Context, int64, string) (study.AnswerReview, error)
	RecordFeedback(context.Context, study.FeedbackInput) error
	ProgressSummary(context.Context) (store.ProgressSummary, error)
	UpdateExpectedAnswer(context.Context, int64, string) error
}

func New(service StudyService) *sdkmcp.Server {
	srv := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "study-service", Version: "0.1.0"}, nil)
	addTool(srv, "import_material", "Import or refresh study questions from markdown.", emptySchema(), importMaterialHandler(service))
	addTool(srv, "list_topics", "List study topics and progress counts.", emptySchema(), listTopicsHandler(service))
	addTool(srv, "get_next_question", "Return the next unseen or weak study question.", emptySchema(), nextQuestionHandler(service))
	addTool(srv, "submit_answer", "Store an answer attempt and return expected answer data.", submitAnswerSchema(), submitAnswerHandler(service))
	addTool(srv, "record_feedback", "Store critique and score for an answer attempt.", recordFeedbackSchema(), recordFeedbackHandler(service))
	addTool(srv, "get_progress_summary", "Return recent attempts and weak-topic summary.", emptySchema(), progressSummaryHandler(service))
	addTool(srv, "add_or_update_expected_answer", "Save an expected answer for a question.", updateExpectedAnswerSchema(), updateExpectedAnswerHandler(service))
	return srv
}

func addTool(srv *sdkmcp.Server, name, description string, schema json.RawMessage, handler sdkmcp.ToolHandler) {
	srv.AddTool(&sdkmcp.Tool{Name: name, Description: description, InputSchema: schema}, handler)
}

func importMaterialHandler(service StudyService) sdkmcp.ToolHandler {
	return func(ctx context.Context, _ *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		result, err := service.ImportMaterial(ctx)
		return resultOrError(result, err), nil
	}
}

func listTopicsHandler(service StudyService) sdkmcp.ToolHandler {
	return func(ctx context.Context, _ *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		result, err := service.ListTopics(ctx)
		return resultOrError(result, err), nil
	}
}

func nextQuestionHandler(service StudyService) sdkmcp.ToolHandler {
	return func(ctx context.Context, _ *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		result, err := service.GetNextQuestion(ctx)
		return resultOrError(result, err), nil
	}
}

func submitAnswerHandler(service StudyService) sdkmcp.ToolHandler {
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

func recordFeedbackHandler(service StudyService) sdkmcp.ToolHandler {
	return func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		var in study.FeedbackInput
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

func progressSummaryHandler(service StudyService) sdkmcp.ToolHandler {
	return func(ctx context.Context, _ *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		result, err := service.ProgressSummary(ctx)
		return resultOrError(result, err), nil
	}
}

func updateExpectedAnswerHandler(service StudyService) sdkmcp.ToolHandler {
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

func emptySchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","additionalProperties":false}`)
}

func submitAnswerSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"question_id":{"type":"integer"},"answer":{"type":"string"}},"required":["question_id","answer"],"additionalProperties":false}`)
}

func recordFeedbackSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"attempt_id":{"type":"integer"},"score":{"type":"integer","minimum":0,"maximum":3},"missing_points":{"type":"string"},"inaccurate_points":{"type":"string"},"suggested_answer":{"type":"string"}},"required":["attempt_id","score"],"additionalProperties":false}`)
}

func updateExpectedAnswerSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"question_id":{"type":"integer"},"answer":{"type":"string"}},"required":["question_id","answer"],"additionalProperties":false}`)
}
