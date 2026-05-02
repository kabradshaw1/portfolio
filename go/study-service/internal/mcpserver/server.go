package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kabradshaw1/portfolio/go/study-service/internal/store"
	"github.com/kabradshaw1/portfolio/go/study-service/internal/study"
)

const workflowResourceURI = "study://micro1/workflow"

type StudyService interface {
	ImportMaterial(context.Context) (study.ImportResult, error)
	ListTopics(context.Context) ([]store.Topic, error)
	GetNextQuestion(context.Context, store.QuestionFilter) (store.Question, error)
	SubmitAnswer(context.Context, int64, string) (study.AnswerReview, error)
	RecordFeedback(context.Context, study.FeedbackInput) error
	ProgressSummary(context.Context) (store.ProgressSummary, error)
	UpdateExpectedAnswer(context.Context, int64, string) error
}

type studySession struct {
	StudySet     string                `json:"study_set"`
	Tier         int                   `json:"tier,omitempty"`
	Category     string                `json:"category,omitempty"`
	Instructions string                `json:"instructions"`
	Topics       []store.Topic         `json:"topics"`
	Progress     store.ProgressSummary `json:"progress"`
	NextQuestion store.Question        `json:"next_question"`
}

type studyTurn struct {
	Review                   study.AnswerReview `json:"review"`
	NextQuestion             store.Question     `json:"next_question"`
	Tier                     int                `json:"tier,omitempty"`
	Category                 string             `json:"category,omitempty"`
	PreviousFeedbackRecorded bool               `json:"previous_feedback_recorded"`
}

func New(service StudyService) *sdkmcp.Server {
	srv := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "study-service", Version: "0.1.0"}, nil)
	addPrompt(srv, "study_micro1", "Study Micro1 Go Developer", "Start a Micro1 Go developer interview-practice session.", studyMicro1PromptHandler())
	addResource(srv, workflowResourceURI, "Micro1 Study Workflow", "Workflow instructions for agent-led Micro1 study sessions.", workflowResourceHandler())
	addTool(srv, "start_study_session", "Start a Micro1 study session with workflow instructions, progress, topics, and the next question.", startStudySessionSchema(), startStudySessionHandler(service))
	addTool(srv, "import_material", "Import or refresh study questions from markdown.", emptySchema(), importMaterialHandler(service))
	addTool(srv, "list_topics", "List study topics and progress counts.", emptySchema(), listTopicsHandler(service))
	addTool(srv, "get_next_question", "Return the next unseen or weak study question.", nextQuestionSchema(), nextQuestionHandler(service))
	addTool(srv, "submit_answer", "Store an answer attempt and return expected answer data.", submitAnswerSchema(), submitAnswerHandler(service))
	addTool(srv, "submit_answer_and_prepare_next", "Store the current answer, optionally record previous feedback, and return expected answer data plus the next question.", submitAnswerAndPrepareNextSchema(), submitAnswerAndPrepareNextHandler(service))
	addTool(srv, "record_feedback", "Store critique and score for an answer attempt.", recordFeedbackSchema(), recordFeedbackHandler(service))
	addTool(srv, "get_progress_summary", "Return recent attempts and weak-topic summary.", emptySchema(), progressSummaryHandler(service))
	addTool(srv, "add_or_update_expected_answer", "Save an expected answer for a question.", updateExpectedAnswerSchema(), updateExpectedAnswerHandler(service))
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

func startStudySessionHandler(service StudyService) sdkmcp.ToolHandler {
	type input struct {
		StudySet string `json:"study_set"`
		Tier     int    `json:"tier,omitempty"`
		Category string `json:"category,omitempty"`
	}
	return func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		in := input{StudySet: "micro1"}
		if req != nil && req.Params != nil && len(req.Params.Arguments) > 0 {
			if err := decodeArgs(req, &in); err != nil {
				return toolError(err.Error()), nil
			}
		}
		if strings.TrimSpace(in.StudySet) == "" {
			in.StudySet = "micro1"
		}
		if in.StudySet != "micro1" {
			return toolError("study_set must be micro1"), nil
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
		return jsonResult(studySession{
			StudySet:     in.StudySet,
			Tier:         in.Tier,
			Category:     filter.Category,
			Instructions: studyWorkflowInstructions(),
			Topics:       topics,
			Progress:     progress,
			NextQuestion: nextQuestion,
		}), nil
	}
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

func submitAnswerAndPrepareNextHandler(service StudyService) sdkmcp.ToolHandler {
	type input struct {
		QuestionID       int64                `json:"question_id"`
		Answer           string               `json:"answer"`
		Tier             int                  `json:"tier,omitempty"`
		Category         string               `json:"category,omitempty"`
		PreviousFeedback *study.FeedbackInput `json:"previous_feedback,omitempty"`
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
		return jsonResult(studyTurn{
			Review:                   review,
			NextQuestion:             nextQuestion,
			Tier:                     in.Tier,
			Category:                 category,
			PreviousFeedbackRecorded: previousFeedbackRecorded,
		}), nil
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

func studyMicro1PromptHandler() sdkmcp.PromptHandler {
	return func(context.Context, *sdkmcp.GetPromptRequest) (*sdkmcp.GetPromptResult, error) {
		return &sdkmcp.GetPromptResult{
			Description: "Start a Micro1 Go developer interview-practice session.",
			Messages: []*sdkmcp.PromptMessage{{
				Role:    sdkmcp.Role("user"),
				Content: &sdkmcp.TextContent{Text: "Study tier 1 for micro1. Call start_study_session with study_set=micro1, tier=1 unless the user requested a different tier, and category when the user names a category such as golang, api, distributed, or coding. Use the returned workflow instructions and next_question, then continue one question at a time within that tier and category."},
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
				Text:     studyWorkflowInstructions(),
			}},
		}, nil
	}
}

func studyWorkflowInstructions() string {
	return strings.TrimSpace(`
# Micro1 Study Workflow

When the user asks to study for micro1, use this MCP server as the source of durable study state.

1. Call start_study_session with study_set=micro1.
2. Respect the requested tier and requested category. Pass both values to start_study_session, get_next_question, and submit_answer_and_prepare_next whenever the user asked for them.
3. If next_question.kind is "qa", ask next_question.prompt exactly enough to quiz the user. Do not reveal expected_answer before the user answers.
4. If next_question.kind is "coding_exercise", present next_question.prompt as a coding task. Tell the user to implement it in the repo/workspace and respond when ready for review. Do not ask for a prose answer. When the user is ready, inspect the relevant files and tests, summarize what you found as the answer, and submit that review summary through submit_answer_and_prepare_next.
5. For coding_exercise feedback, grade correctness against the prompt, concurrency safety, idiomatic Go, edge cases, test quality, and simplicity. Mention the files reviewed.
6. After the user answers a qa question, or after you review a coding_exercise implementation, call exactly one MCP tool: submit_answer_and_prepare_next with question_id, answer, tier, category, and any previous_feedback payload you prepared for the prior answer.
7. Do not call record_feedback or get_next_question separately in the same turn.
8. Compare review.answer to review.expected_answer. Point out missing or inaccurate parts clearly.
9. Respond in this teaching-first style:
   Score: X/3
   Explanation: explain the concept thoroughly, including why the expected answer is correct, key distinctions, tradeoffs, and concrete examples when helpful. Include a small Go code snippet when it would make the concept clearer. Do not force code when the idea is better explained in prose. Do not compress this section just because the next question is included.
   Interview answer: provide a polished answer the user could say in an interview.
   Minimum answer, only when useful: provide a short memorization-friendly fallback answer.
   Memory hook, only when useful: provide a compact phrase that helps the user remember the idea.
10. Decide concise missing_points, inaccurate_points, suggested_answer, and the 0-3 score for the current answer, but do not call record_feedback yet.
11. Keep that feedback payload in context and send it as previous_feedback on the next submit_answer_and_prepare_next call.
12. Ask the next qa prompt in the same response as the feedback so the user never has to ask for the next question. For the next coding_exercise prompt, present it as a repo implementation task and wait for the user to write code.

Use list_topics and get_progress_summary when the user asks about coverage, weak areas, or progress.
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

func startStudySessionSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"study_set":{"type":"string","enum":["micro1"],"default":"micro1"},"tier":{"type":"integer","minimum":1,"maximum":3},"category":{"type":"string"}},"additionalProperties":false}`)
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
