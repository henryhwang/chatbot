package types

import "time"

// --- Configuration and Provider ---

type ModelProvider struct {
	Provider string
	UrlBase  string
	APIKey   string
	APIs     map[string]string
	Model    string
}

// --- API Request/Response Structures ---

// Request structure for the chat API (used for both streaming and non-streaming)
type OpenAIRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream,omitempty"` // Set to true for streaming
}

// Standard message structure, now including a timestamp
type Message struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"-"` // Exclude from API JSON, internal use only
}

// --- Structs specifically for STREAMING response handling ---

// Overall structure of a single SSE data line payload
type OpenAIStreamResponse struct {
	Choices []StreamChoice `json:"choices"`
	// Usage *UsageInfo `json:"usage,omitempty"` // Optional: If API provides usage at the end
}

// Structure of a choice within the stream
type StreamChoice struct {
	Delta        Delta   `json:"delta"`                   // The actual changes in this chunk
	FinishReason *string `json:"finish_reason,omitempty"` // e.g., "stop", "length", "tool_calls"
	// Index *int `json:"index,omitempty"` // Usually 0 for chat completions
}

// Structure of the delta (the changes) in a stream chunk
type Delta struct {
	Role      string `json:"role,omitempty"`              // Assistant's role, usually in the first chunk
	Content   string `json:"content,omitempty"`           // Final answer content chunk
	Reasoning string `json:"reasoning_content,omitempty"` // <<< DeepSeek specific reasoning/thinking chunk
	// ToolCalls []*ToolCall `json:"tool_calls,omitempty"` // Standard tool call mechanism (if supported/needed)
}

// --- Optional: Standard Tool Call Structures (If needed in the future) ---
// type ToolCall struct {
//  Index    *int             `json:"index,omitempty"`
//  ID       string           `json:"id,omitempty"`
//  Type     string           `json:"type,omitempty"` // "function"
//  Function ToolCallFunction `json:"function,omitempty"`
// }
// type ToolCallFunction struct {
//  Name      string `json:"name,omitempty"`
//  Arguments string `json:"arguments,omitempty"`
// }
