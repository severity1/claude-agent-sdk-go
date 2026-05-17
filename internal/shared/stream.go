package shared

import "context"

// StreamMessage represents messages sent to the CLI for streaming communication.
// SessionID and ParentToolUseID are always emitted (without omitempty) so the
// wire shape matches the TypeScript and Python SDKs: a user-message line
// always carries `"session_id":""` and `"parent_tool_use_id":null` when those
// are unset.
type StreamMessage struct {
	Type            string                 `json:"type"`
	Message         interface{}            `json:"message,omitempty"`
	ParentToolUseID *string                `json:"parent_tool_use_id"`
	SessionID       string                 `json:"session_id"`
	RequestID       string                 `json:"request_id,omitempty"`
	Request         map[string]interface{} `json:"request,omitempty"`
	Response        map[string]interface{} `json:"response,omitempty"`
}

// MessageIterator provides an iterator pattern for streaming messages.
type MessageIterator interface {
	Next(ctx context.Context) (Message, error)
	Close() error
}
