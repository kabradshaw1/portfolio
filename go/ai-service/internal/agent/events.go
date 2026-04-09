package agent

import "encoding/json"

// Event is the sum type emitted by the agent loop.
// Exactly one of the concrete event structs is non-zero.
type Event struct {
	ToolCall   *ToolCallEvent   `json:"tool_call,omitempty"`
	ToolResult *ToolResultEvent `json:"tool_result,omitempty"`
	ToolError  *ToolErrorEvent  `json:"tool_error,omitempty"`
	Final      *FinalEvent      `json:"final,omitempty"`
	Error      *ErrorEvent      `json:"error,omitempty"`
}

type ToolCallEvent struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args"`
}

type ToolResultEvent struct {
	Name    string `json:"name"`
	Display any    `json:"display,omitempty"`
}

type ToolErrorEvent struct {
	Name  string `json:"name"`
	Error string `json:"error"`
}

type FinalEvent struct {
	Text string `json:"text"`
}

type ErrorEvent struct {
	Reason string `json:"reason"`
}
