package shared

import (
	"encoding/json"
)

// Message type constants
const (
	MessageTypeUser      = "user"
	MessageTypeAssistant = "assistant"
	MessageTypeSystem    = "system"
	MessageTypeResult    = "result"

	// Control protocol message types
	MessageTypeControlRequest  = "control_request"
	MessageTypeControlResponse = "control_response"

	// Partial message streaming type
	MessageTypeStreamEvent = "stream_event"
)

// Content block type constants
const (
	ContentBlockTypeText       = "text"
	ContentBlockTypeThinking   = "thinking"
	ContentBlockTypeToolUse    = "tool_use"
	ContentBlockTypeToolResult = "tool_result"
)

// AssistantMessageError represents error types in assistant messages.
type AssistantMessageError string

// AssistantMessageError constants for error type identification.
// Matches Python SDK Literal values from types.py (AssistantMessageError):
// authentication_failed, billing_error, rate_limit, invalid_request,
// server_error, unknown.
const (
	AssistantMessageErrorAuthFailed     AssistantMessageError = "authentication_failed"
	AssistantMessageErrorBilling        AssistantMessageError = "billing_error"
	AssistantMessageErrorRateLimit      AssistantMessageError = "rate_limit"
	AssistantMessageErrorInvalidRequest AssistantMessageError = "invalid_request"
	AssistantMessageErrorServer         AssistantMessageError = "server_error"
	AssistantMessageErrorUnknown        AssistantMessageError = "unknown"
)

// Message represents any message type in the Claude Code protocol.
type Message interface {
	Type() string
}

// ContentBlock represents any content block within a message.
type ContentBlock interface {
	BlockType() string
}

// UserMessage represents a message from the user.
type UserMessage struct {
	MessageType     string         `json:"type"`
	Content         interface{}    `json:"content"` // string or []ContentBlock
	UUID            *string        `json:"uuid,omitempty"`
	ParentToolUseID *string        `json:"parent_tool_use_id,omitempty"`
	ToolUseResult   map[string]any `json:"tool_use_result,omitempty"`
}

// Type returns the message type for UserMessage.
func (m *UserMessage) Type() string {
	return MessageTypeUser
}

// GetUUID returns the UUID or empty string if nil.
func (m *UserMessage) GetUUID() string {
	if m.UUID != nil {
		return *m.UUID
	}
	return ""
}

// GetParentToolUseID returns the parent tool use ID or empty string if nil.
func (m *UserMessage) GetParentToolUseID() string {
	if m.ParentToolUseID != nil {
		return *m.ParentToolUseID
	}
	return ""
}

// GetToolUseResult returns the tool use result metadata or nil if not present.
func (m *UserMessage) GetToolUseResult() map[string]any {
	return m.ToolUseResult
}

// HasToolUseResult returns true if tool use result metadata is present and non-empty.
func (m *UserMessage) HasToolUseResult() bool {
	return len(m.ToolUseResult) > 0
}

// MarshalJSON implements custom JSON marshaling for UserMessage
func (m *UserMessage) MarshalJSON() ([]byte, error) {
	type userMessage UserMessage
	temp := struct {
		Type string `json:"type"`
		*userMessage
	}{
		Type:        MessageTypeUser,
		userMessage: (*userMessage)(m),
	}
	return json.Marshal(temp)
}

// AssistantMessage represents a message from the assistant.
type AssistantMessage struct {
	MessageType     string                 `json:"type"`
	Content         []ContentBlock         `json:"content"`
	Model           string                 `json:"model"`
	Error           *AssistantMessageError `json:"error,omitempty"`
	ParentToolUseID *string                `json:"parent_tool_use_id,omitempty"`
}

// Type returns the message type for AssistantMessage.
func (m *AssistantMessage) Type() string {
	return MessageTypeAssistant
}

// GetParentToolUseID returns the parent tool use ID or empty string if nil.
// Set by the CLI on assistant messages produced inside a subagent (Agent/Task tool)
// to identify the orchestrator tool_use_id that spawned the subagent.
func (m *AssistantMessage) GetParentToolUseID() string {
	if m.ParentToolUseID != nil {
		return *m.ParentToolUseID
	}
	return ""
}

// HasError returns true if the message contains an error.
func (m *AssistantMessage) HasError() bool {
	return m.Error != nil
}

// GetError returns the error type or empty string if nil.
func (m *AssistantMessage) GetError() AssistantMessageError {
	if m.Error != nil {
		return *m.Error
	}
	return ""
}

// IsRateLimited returns true if the error is a rate limit error.
func (m *AssistantMessage) IsRateLimited() bool {
	return m.Error != nil && *m.Error == AssistantMessageErrorRateLimit
}

// MarshalJSON implements custom JSON marshaling for AssistantMessage
func (m *AssistantMessage) MarshalJSON() ([]byte, error) {
	type assistantMessage AssistantMessage
	temp := struct {
		Type string `json:"type"`
		*assistantMessage
	}{
		Type:             MessageTypeAssistant,
		assistantMessage: (*assistantMessage)(m),
	}
	return json.Marshal(temp)
}

// SystemMessage represents a system message.
type SystemMessage struct {
	MessageType string         `json:"type"`
	Subtype     string         `json:"subtype"`
	Data        map[string]any `json:"-"` // Preserve all original data
}

// Type returns the message type for SystemMessage.
func (m *SystemMessage) Type() string {
	return MessageTypeSystem
}

// MarshalJSON implements custom JSON marshaling for SystemMessage
func (m *SystemMessage) MarshalJSON() ([]byte, error) {
	data := make(map[string]any)
	for k, v := range m.Data {
		data[k] = v
	}
	data["type"] = MessageTypeSystem
	data["subtype"] = m.Subtype
	return json.Marshal(data)
}

// ResultMessage represents the final result of a conversation turn.
type ResultMessage struct {
	MessageType      string          `json:"type"`
	Subtype          string          `json:"subtype"`
	DurationMs       int             `json:"duration_ms"`
	DurationAPIMs    int             `json:"duration_api_ms"`
	IsError          bool            `json:"is_error"`
	Errors           []string        `json:"errors,omitempty"`
	NumTurns         int             `json:"num_turns"`
	SessionID        string          `json:"session_id"`
	TotalCostUSD     *float64        `json:"total_cost_usd,omitempty"`
	Usage            *map[string]any `json:"usage,omitempty"`
	Result           *string         `json:"result,omitempty"`
	StructuredOutput any             `json:"structured_output,omitempty"`
}

// Type returns the message type for ResultMessage.
func (m *ResultMessage) Type() string {
	return MessageTypeResult
}

// MarshalJSON implements custom JSON marshaling for ResultMessage
func (m *ResultMessage) MarshalJSON() ([]byte, error) {
	type resultMessage ResultMessage
	temp := struct {
		Type string `json:"type"`
		*resultMessage
	}{
		Type:          MessageTypeResult,
		resultMessage: (*resultMessage)(m),
	}
	return json.Marshal(temp)
}

// TextBlock represents text content.
type TextBlock struct {
	MessageType string `json:"type"`
	Text        string `json:"text"`
}

// BlockType returns the content block type for TextBlock.
func (b *TextBlock) BlockType() string {
	return ContentBlockTypeText
}

// ThinkingBlock represents thinking content with signature.
type ThinkingBlock struct {
	MessageType string `json:"type"`
	Thinking    string `json:"thinking"`
	Signature   string `json:"signature"`
}

// BlockType returns the content block type for ThinkingBlock.
func (b *ThinkingBlock) BlockType() string {
	return ContentBlockTypeThinking
}

// ToolUseBlock represents a tool use request.
type ToolUseBlock struct {
	MessageType string         `json:"type"`
	ToolUseID   string         `json:"tool_use_id"`
	Name        string         `json:"name"`
	Input       map[string]any `json:"input"`
}

// BlockType returns the content block type for ToolUseBlock.
func (b *ToolUseBlock) BlockType() string {
	return ContentBlockTypeToolUse
}

// ToolResultBlock represents the result of a tool use.
type ToolResultBlock struct {
	MessageType string      `json:"type"`
	ToolUseID   string      `json:"tool_use_id"`
	Content     interface{} `json:"content"` // string or structured data
	IsError     *bool       `json:"is_error,omitempty"`
}

// BlockType returns the content block type for ToolResultBlock.
func (b *ToolResultBlock) BlockType() string {
	return ContentBlockTypeToolResult
}

// RawControlMessage wraps raw control protocol messages for passthrough to the control handler.
// Control messages are not parsed into typed structs by the parser - they are routed directly
// to the control protocol handler which performs its own parsing.
type RawControlMessage struct {
	MessageType string
	Data        map[string]any
}

// Type returns the message type for RawControlMessage.
func (m *RawControlMessage) Type() string {
	return m.MessageType
}

// Stream event type constants for Event["type"] discrimination.
// Use these when type-switching on StreamEvent.Event to handle different event types.
const (
	StreamEventTypeContentBlockStart = "content_block_start"
	StreamEventTypeContentBlockDelta = "content_block_delta"
	StreamEventTypeContentBlockStop  = "content_block_stop"
	StreamEventTypeMessageStart      = "message_start"
	StreamEventTypeMessageDelta      = "message_delta"
	StreamEventTypeMessageStop       = "message_stop"
)

// StreamEvent represents a partial message update during streaming.
// Emitted when IncludePartialMessages is enabled in Options.
//
// The Event field contains varying structure depending on event type:
//   - content_block_start: {"type": "content_block_start", "index": <int>, "content_block": {...}}
//   - content_block_delta: {"type": "content_block_delta", "index": <int>, "delta": {...}}
//   - content_block_stop: {"type": "content_block_stop", "index": <int>}
//   - message_start: {"type": "message_start", "message": {...}}
//   - message_delta: {"type": "message_delta", "delta": {...}, "usage": {...}}
//   - message_stop: {"type": "message_stop"}
//
// Consumer code should type-switch on Event["type"] to handle different event types:
//
//	switch event.Event["type"] {
//	case shared.StreamEventTypeContentBlockDelta:
//	    // Handle content delta
//	case shared.StreamEventTypeMessageStop:
//	    // Handle message completion
//	}
type StreamEvent struct {
	UUID            string         `json:"uuid"`
	SessionID       string         `json:"session_id"`
	Event           map[string]any `json:"event"`
	ParentToolUseID *string        `json:"parent_tool_use_id,omitempty"`
}

// Type returns the message type for StreamEvent.
func (m *StreamEvent) Type() string {
	return MessageTypeStreamEvent
}
