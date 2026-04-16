package http

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/kabradshaw1/portfolio/go/ai-service/internal/agent"
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/auth"
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/guardrails"
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/llm"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
)

// Runner is the subset of *agent.Agent the HTTP handler needs.
type Runner interface {
	Run(ctx context.Context, turn agent.Turn, emit func(agent.Event)) error
}

type chatRequest struct {
	Messages  []llm.Message `json:"messages"`
	SessionID string        `json:"session_id,omitempty"`
}

const maxUserMessageBytes = 4000

// RegisterChatRoutes wires POST /chat onto r.
func RegisterChatRoutes(r *gin.Engine, runner Runner, jwtSecret string, limiter *guardrails.Limiter) {
	r.POST("/chat", guardrails.Middleware(limiter), func(c *gin.Context) {
		var req chatRequest
		body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxUserMessageBytes*4))
		if err != nil {
			_ = c.Error(apperror.BadRequest("READ_BODY_ERROR", "failed to read request body"))
			return
		}
		if err := json.Unmarshal(body, &req); err != nil {
			_ = c.Error(apperror.BadRequest("INVALID_JSON", "invalid json"))
			return
		}
		if len(req.Messages) == 0 {
			_ = c.Error(apperror.BadRequest("MESSAGES_REQUIRED", "messages required"))
			return
		}
		for _, m := range req.Messages {
			if m.Role == llm.RoleUser && len(m.Content) > maxUserMessageBytes {
				_ = c.Error(apperror.BadRequest("MESSAGE_TOO_LONG", "message too long"))
				return
			}
		}

		var userID string
		authHeader := c.GetHeader("Authorization")
		cookieToken, cookieErr := c.Cookie("access_token")
		if authHeader != "" {
			uid, err := auth.ParseBearer(authHeader, jwtSecret)
			if err != nil {
				_ = c.Error(apperror.Unauthorized("INVALID_TOKEN", err.Error()))
				return
			}
			userID = uid
		} else if cookieErr == nil && cookieToken != "" {
			uid, err := auth.ParseBearer("Bearer "+cookieToken, jwtSecret)
			if err != nil {
				_ = c.Error(apperror.Unauthorized("INVALID_TOKEN", err.Error()))
				return
			}
			userID = uid
		}

		// Beyond this point we stream SSE — errors go via emit, not c.Error().
		c.Writer.Header().Set("Content-Type", "text/event-stream")
		c.Writer.Header().Set("Cache-Control", "no-cache")
		c.Writer.Header().Set("Connection", "keep-alive")
		c.Writer.Header().Set("X-Accel-Buffering", "no")
		c.Writer.WriteHeader(http.StatusOK)
		flusher, _ := c.Writer.(http.Flusher)

		emit := func(e agent.Event) {
			name, payload := eventName(e)
			data, _ := json.Marshal(payload)
			_, _ = c.Writer.WriteString("event: " + name + "\n")
			_, _ = c.Writer.WriteString("data: " + string(data) + "\n\n")
			if flusher != nil {
				flusher.Flush()
			}
		}

		ctx := c.Request.Context()
		if authHeader != "" {
			ctx = ContextWithJWT(ctx, strings.TrimPrefix(authHeader, "Bearer "))
		} else if cookieErr == nil && cookieToken != "" {
			ctx = ContextWithJWT(ctx, cookieToken)
		}

		turn := agent.Turn{UserID: userID, Messages: req.Messages}
		if err := runner.Run(ctx, turn, emit); err != nil {
			emit(agent.Event{Error: &agent.ErrorEvent{Reason: err.Error()}})
		}
	})
}

func eventName(e agent.Event) (string, any) {
	switch {
	case e.ToolCall != nil:
		return "tool_call", e.ToolCall
	case e.ToolResult != nil:
		return "tool_result", e.ToolResult
	case e.ToolError != nil:
		return "tool_error", e.ToolError
	case e.Final != nil:
		return "final", e.Final
	case e.Error != nil:
		return "error", e.Error
	default:
		return "unknown", struct{}{}
	}
}
