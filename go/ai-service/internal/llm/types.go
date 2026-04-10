package llm

import (
	"encoding/json"
	"time"
)

// Role is the sender of a chat message.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Message is one entry in the chat history sent to the LLM.
type Message struct {
	Role       Role       `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"` // tool name for role=tool
}

// ToolCall is a single tool invocation requested by the model.
type ToolCall struct {
	ID   string          `json:"id"`
	Name string          `json:"name"`
	Args json.RawMessage `json:"arguments"`
}

// ChatResponse is what the model returns for one Chat call.
type ChatResponse struct {
	Content          string     // final text if no tool calls
	ToolCalls        []ToolCall // non-empty when the model wants to call tools
	PromptEvalCount  int        // prompt tokens (from Ollama metadata)
	EvalCount        int        // completion tokens (from Ollama metadata)
	EvalDurationNs   int        // model eval duration in nanoseconds
	RequestDuration  time.Duration // wall-clock request time
}

// ToolSchema is the JSON-Schema-shaped description of a tool we advertise
// to the LLM on every turn.
type ToolSchema struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"` // JSON Schema object
}

// ToolResultMessage builds a role="tool" message from a tool's JSON-serializable result.
func ToolResultMessage(callID, toolName string, content any) (Message, error) {
	body, err := json.Marshal(content)
	if err != nil {
		return Message{}, err
	}
	return Message{
		Role:       RoleTool,
		ToolCallID: callID,
		Name:       toolName,
		Content:    string(body),
	}, nil
}
