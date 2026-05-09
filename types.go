package claudecode

import (
	"context"

	"github.com/severity1/claude-agent-sdk-go/internal/control"
	"github.com/severity1/claude-agent-sdk-go/internal/shared"
)

// Message represents any message type in the conversation.
type Message = shared.Message

// ContentBlock represents a content block within a message.
type ContentBlock = shared.ContentBlock

// UserMessage represents a message from the user.
type UserMessage = shared.UserMessage

// AssistantMessage represents a message from the assistant.
type AssistantMessage = shared.AssistantMessage

// AssistantMessageError represents error types in assistant messages.
type AssistantMessageError = shared.AssistantMessageError

// SystemMessage represents a system prompt message.
type SystemMessage = shared.SystemMessage

// ResultMessage represents a result or status message.
type ResultMessage = shared.ResultMessage

// TextBlock represents a text content block.
type TextBlock = shared.TextBlock

// ThinkingBlock represents a thinking content block.
type ThinkingBlock = shared.ThinkingBlock

// ToolUseBlock represents a tool usage content block.
type ToolUseBlock = shared.ToolUseBlock

// ToolResultBlock represents a tool result content block.
type ToolResultBlock = shared.ToolResultBlock

// StreamMessage represents a message in the streaming protocol.
type StreamMessage = shared.StreamMessage

// MessageIterator provides iteration over messages.
type MessageIterator = shared.MessageIterator

// StreamValidator tracks tool requests and results to detect incomplete streams.
type StreamValidator = shared.StreamValidator

// StreamIssue represents a validation issue found in the stream.
type StreamIssue = shared.StreamIssue

// StreamStats provides statistics about the message stream.
type StreamStats = shared.StreamStats

// Re-export message type constants
const (
	MessageTypeUser      = shared.MessageTypeUser
	MessageTypeAssistant = shared.MessageTypeAssistant
	MessageTypeSystem    = shared.MessageTypeSystem
	MessageTypeResult    = shared.MessageTypeResult

	// Control protocol message types
	MessageTypeControlRequest  = shared.MessageTypeControlRequest
	MessageTypeControlResponse = shared.MessageTypeControlResponse

	// Partial message streaming type
	MessageTypeStreamEvent = shared.MessageTypeStreamEvent
)

// Re-export content block type constants
const (
	ContentBlockTypeText       = shared.ContentBlockTypeText
	ContentBlockTypeThinking   = shared.ContentBlockTypeThinking
	ContentBlockTypeToolUse    = shared.ContentBlockTypeToolUse
	ContentBlockTypeToolResult = shared.ContentBlockTypeToolResult
)

// Re-export stream event type constants for Event["type"] discrimination.
const (
	StreamEventTypeContentBlockStart = shared.StreamEventTypeContentBlockStart
	StreamEventTypeContentBlockDelta = shared.StreamEventTypeContentBlockDelta
	StreamEventTypeContentBlockStop  = shared.StreamEventTypeContentBlockStop
	StreamEventTypeMessageStart      = shared.StreamEventTypeMessageStart
	StreamEventTypeMessageDelta      = shared.StreamEventTypeMessageDelta
	StreamEventTypeMessageStop       = shared.StreamEventTypeMessageStop
)

// Re-export AssistantMessageError constants
const (
	AssistantMessageErrorAuthFailed     = shared.AssistantMessageErrorAuthFailed
	AssistantMessageErrorBilling        = shared.AssistantMessageErrorBilling
	AssistantMessageErrorRateLimit      = shared.AssistantMessageErrorRateLimit
	AssistantMessageErrorInvalidRequest = shared.AssistantMessageErrorInvalidRequest
	AssistantMessageErrorServer         = shared.AssistantMessageErrorServer
	AssistantMessageErrorUnknown        = shared.AssistantMessageErrorUnknown
)

// OutputFormatTypeJSONSchema is the only currently-supported value for
// OutputFormat.Type.
const OutputFormatTypeJSONSchema = shared.OutputFormatTypeJSONSchema

// AgentModel represents the model to use for an agent.
type AgentModel = shared.AgentModel

// AgentDefinition defines a programmatic subagent.
type AgentDefinition = shared.AgentDefinition

// Re-export agent model constants
const (
	AgentModelSonnet  = shared.AgentModelSonnet
	AgentModelOpus    = shared.AgentModelOpus
	AgentModelHaiku   = shared.AgentModelHaiku
	AgentModelInherit = shared.AgentModelInherit
)

// Transport abstracts the communication layer with Claude Code CLI.
// This interface stays in main package because it's used by client code.
type Transport interface {
	Connect(ctx context.Context) error
	SendMessage(ctx context.Context, message StreamMessage) error
	ReceiveMessages(ctx context.Context) (<-chan Message, <-chan error)
	Interrupt(ctx context.Context) error
	// SetModel changes the AI model during streaming session.
	SetModel(ctx context.Context, model *string) error
	// SetPermissionMode changes the permission mode during streaming session.
	SetPermissionMode(ctx context.Context, mode string) error
	// RewindFiles reverts tracked files to their state at a specific user message.
	// Requires file checkpointing to be enabled and control protocol initialized.
	RewindFiles(ctx context.Context, userMessageID string) error
	// GetMcpStatus returns the current status of all connected MCP servers.
	// Only works in streaming mode (after Connect()).
	GetMcpStatus(ctx context.Context) (*McpStatusResponse, error)
	Close() error
	GetValidator() *StreamValidator
}

// RawControlMessage wraps raw control protocol messages for passthrough.
type RawControlMessage = shared.RawControlMessage

// RawMessage is returned for unrecognized message types (forward-compatibility).
type RawMessage = shared.RawMessage

// RawContentBlock is returned for unrecognized content block types (forward-compatibility).
type RawContentBlock = shared.RawContentBlock

// StreamEvent represents a partial message update during streaming.
type StreamEvent = shared.StreamEvent

// Control protocol types for SDK-CLI bidirectional communication.

// SDKControlRequest represents a control request sent to the CLI.
type SDKControlRequest = control.SDKControlRequest

// SDKControlResponse represents a control response received from the CLI.
type SDKControlResponse = control.SDKControlResponse

// ControlResponse is the inner response structure.
type ControlResponse = control.Response

// InitializeRequest for control protocol handshake.
type InitializeRequest = control.InitializeRequest

// InitializeResponse from CLI with supported capabilities.
type InitializeResponse = control.InitializeResponse

// InterruptRequest to interrupt current operation via control protocol.
type InterruptRequest = control.InterruptRequest

// SetPermissionModeRequest to change permission mode via control protocol.
type SetPermissionModeRequest = control.SetPermissionModeRequest

// SetModelRequest to change AI model via control protocol.
type SetModelRequest = control.SetModelRequest

// ControlProtocol manages bidirectional control communication with CLI.
type ControlProtocol = control.Protocol

// McpServerConnectionStatus represents the connection state of an MCP server.
type McpServerConnectionStatus = control.McpServerConnectionStatus

// McpServerInfo contains version info about a connected MCP server.
type McpServerInfo = control.McpServerInfo

// McpToolInfo describes a tool exposed by an MCP server (for status reporting).
type McpToolInfo = control.McpToolInfo

// McpServerStatus represents the status of a connected MCP server.
type McpServerStatus = control.McpServerStatus

// McpStatusResponse is the response from a get_mcp_status request.
type McpStatusResponse = control.McpStatusResponse

// Re-export control protocol subtype constants
const (
	// Control request subtypes
	SubtypeInterrupt         = control.SubtypeInterrupt
	SubtypeCanUseTool        = control.SubtypeCanUseTool
	SubtypeInitialize        = control.SubtypeInitialize
	SubtypeSetPermissionMode = control.SubtypeSetPermissionMode
	SubtypeSetModel          = control.SubtypeSetModel
	SubtypeHookCallback      = control.SubtypeHookCallback
	SubtypeMcpMessage        = control.SubtypeMcpMessage
	SubtypeGetMcpStatus      = control.SubtypeGetMcpStatus
	SubtypeRewindFiles       = control.SubtypeRewindFiles

	// Control response subtypes
	ResponseSubtypeSuccess = control.ResponseSubtypeSuccess
	ResponseSubtypeError   = control.ResponseSubtypeError
)

// Re-export McpServerConnectionStatus constants
const (
	McpServerConnectionStatusConnected = control.McpServerConnectionStatusConnected
	McpServerConnectionStatusFailed    = control.McpServerConnectionStatusFailed
	McpServerConnectionStatusNeedsAuth = control.McpServerConnectionStatusNeedsAuth
	McpServerConnectionStatusPending   = control.McpServerConnectionStatusPending
	McpServerConnectionStatusDisabled  = control.McpServerConnectionStatusDisabled
)
