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

// ServerToolUseBlock is a call to a tool that the API runs on the server side.
type ServerToolUseBlock = shared.ServerToolUseBlock

// ServerToolResultBlock is the result of a server-side tool call.
type ServerToolResultBlock = shared.ServerToolResultBlock

// ServerToolName names a tool that the API runs on the server side.
type ServerToolName = shared.ServerToolName

// StreamMessage represents a message in the streaming protocol.
type StreamMessage = shared.StreamMessage

// RateLimitEventMessage is a session heartbeat carrying rate-limit window
// state. Emitted on essentially every CLI session even when nothing is
// constrained — see IsAllowed for the quick "all good" check.
type RateLimitEventMessage = shared.RateLimitEventMessage

// RateLimitInfo is the window state carried by RateLimitEventMessage.
type RateLimitInfo = shared.RateLimitInfo

// ConversationResetMessage reports that the session's conversation was
// replaced without ending the connection, for example after /clear.
type ConversationResetMessage = shared.ConversationResetMessage

// TaskStartedMessage is the typed form of a task_started system message,
// returned by SystemMessage.AsTaskStarted. Its TaskID is the ID StopTask takes.
type TaskStartedMessage = shared.TaskStartedMessage

// TaskProgressMessage is the typed form of a task_progress system message,
// returned by SystemMessage.AsTaskProgress.
type TaskProgressMessage = shared.TaskProgressMessage

// TaskNotificationMessage is the typed form of a task_notification system
// message, returned by SystemMessage.AsTaskNotification.
type TaskNotificationMessage = shared.TaskNotificationMessage

// TaskUpdatedMessage is the typed form of a task_updated system message,
// returned by SystemMessage.AsTaskUpdated.
type TaskUpdatedMessage = shared.TaskUpdatedMessage

// TaskUsage is the usage reported in task_progress and task_notification messages.
type TaskUsage = shared.TaskUsage

// TaskNotificationStatus is the status of a TaskNotificationMessage.
type TaskNotificationStatus = shared.TaskNotificationStatus

// TaskUpdatedStatus is the status reported inside a TaskUpdatedMessage patch.
type TaskUpdatedStatus = shared.TaskUpdatedStatus

// IsTerminalTaskStatus reports whether a TaskNotificationMessage or
// TaskUpdatedMessage status means the task has finished.
var IsTerminalTaskStatus = shared.IsTerminalTaskStatus

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

	// Session heartbeat carrying rate-limit window state.
	MessageTypeRateLimitEvent = shared.MessageTypeRateLimitEvent

	// Conversation replaced mid-session.
	MessageTypeConversationReset = shared.MessageTypeConversationReset
)

// Re-export server tool names.
const (
	ServerToolNameAdvisor                 = shared.ServerToolNameAdvisor
	ServerToolNameWebSearch               = shared.ServerToolNameWebSearch
	ServerToolNameWebFetch                = shared.ServerToolNameWebFetch
	ServerToolNameCodeExecution           = shared.ServerToolNameCodeExecution
	ServerToolNameBashCodeExecution       = shared.ServerToolNameBashCodeExecution
	ServerToolNameTextEditorCodeExecution = shared.ServerToolNameTextEditorCodeExecution
	ServerToolNameToolSearchToolRegex     = shared.ServerToolNameToolSearchToolRegex
	ServerToolNameToolSearchToolBM25      = shared.ServerToolNameToolSearchToolBM25
)

// Re-export task lifecycle system message subtypes and statuses.
const (
	SystemSubtypeTaskStarted      = shared.SystemSubtypeTaskStarted
	SystemSubtypeTaskProgress     = shared.SystemSubtypeTaskProgress
	SystemSubtypeTaskNotification = shared.SystemSubtypeTaskNotification
	SystemSubtypeTaskUpdated      = shared.SystemSubtypeTaskUpdated

	TaskNotificationStatusCompleted = shared.TaskNotificationStatusCompleted
	TaskNotificationStatusFailed    = shared.TaskNotificationStatusFailed
	TaskNotificationStatusStopped   = shared.TaskNotificationStatusStopped

	TaskUpdatedStatusPending   = shared.TaskUpdatedStatusPending
	TaskUpdatedStatusRunning   = shared.TaskUpdatedStatusRunning
	TaskUpdatedStatusPaused    = shared.TaskUpdatedStatusPaused
	TaskUpdatedStatusCompleted = shared.TaskUpdatedStatusCompleted
	TaskUpdatedStatusFailed    = shared.TaskUpdatedStatusFailed
	TaskUpdatedStatusKilled    = shared.TaskUpdatedStatusKilled
)

// Rate-limit window status constants.
const (
	RateLimitStatusAllowed        = shared.RateLimitStatusAllowed
	RateLimitStatusAllowedWarning = shared.RateLimitStatusAllowedWarning
	RateLimitStatusRejected       = shared.RateLimitStatusRejected

	RateLimitTypeFiveHour       = shared.RateLimitTypeFiveHour
	RateLimitTypeSevenDay       = shared.RateLimitTypeSevenDay
	RateLimitTypeSevenDayOpus   = shared.RateLimitTypeSevenDayOpus
	RateLimitTypeSevenDaySonnet = shared.RateLimitTypeSevenDaySonnet
	RateLimitTypeOverage        = shared.RateLimitTypeOverage
)

// Re-export content block type constants
const (
	ContentBlockTypeText       = shared.ContentBlockTypeText
	ContentBlockTypeThinking   = shared.ContentBlockTypeThinking
	ContentBlockTypeToolUse    = shared.ContentBlockTypeToolUse
	ContentBlockTypeToolResult = shared.ContentBlockTypeToolResult

	ContentBlockTypeServerToolUse     = shared.ContentBlockTypeServerToolUse
	ContentBlockTypeAdvisorToolResult = shared.ContentBlockTypeAdvisorToolResult
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
//
// A Transport can also report the exit of its CLI process by implementing
// Done() <-chan struct{} and Err() error with the semantics of Client.Done
// and Client.Err. The subprocess transport does; for one that does not,
// Client.Done closes on Disconnect.
//
// A Transport can also keep the CLI's initialize response by implementing
// InitializationResult() map[string]interface{}; Client.GetServerInfo returns
// it. The subprocess transport does.
type Transport interface {
	// Connect starts the CLI. ctx bounds only the connect step; the CLI
	// runs until Close.
	Connect(ctx context.Context) error
	SendMessage(ctx context.Context, message StreamMessage) error
	// EndInput signals end-of-input by closing the write side of the
	// transport (stdin for subprocess transports). The receive direction
	// stays open until the CLI closes its end. Idempotent. The context is
	// accepted for symmetry with the other methods; implementations are
	// not required to honor cancellation since closing a pipe is a fast
	// non-cancellable syscall.
	EndInput(ctx context.Context) error
	ReceiveMessages(ctx context.Context) (<-chan Message, <-chan error)
	Interrupt(ctx context.Context) error
	// SetModel changes the AI model during streaming session.
	SetModel(ctx context.Context, model *string) error
	// SetPermissionMode changes the permission mode during streaming session.
	SetPermissionMode(ctx context.Context, mode PermissionMode) error
	// RewindFiles reverts tracked files to their state at a specific user message.
	// Requires file checkpointing to be enabled and control protocol initialized.
	RewindFiles(ctx context.Context, userMessageID string) error
	// GetMcpStatus returns the connection status of all configured MCP servers.
	GetMcpStatus(ctx context.Context) (*McpStatusResponse, error)
	// StopTask stops a single running task by the task_id from its
	// task_started system message.
	StopTask(ctx context.Context, taskID string) error
	// ReconnectMcpServer reconnects a disconnected or failed MCP server.
	ReconnectMcpServer(ctx context.Context, serverName string) error
	// ToggleMcpServer enables or disables an MCP server.
	ToggleMcpServer(ctx context.Context, serverName string, enabled bool) error
	Close() error
	GetValidator() *StreamValidator
}

// RawControlMessage wraps raw control protocol messages for passthrough.
type RawControlMessage = shared.RawControlMessage

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

// StopTaskRequest to stop a single running task via control protocol.
type StopTaskRequest = control.StopTaskRequest

// GetMcpStatusRequest to query MCP server status via control protocol.
type GetMcpStatusRequest = control.GetMcpStatusRequest

// McpServerConnectionStatus represents the connection state of an MCP server.
type McpServerConnectionStatus = control.McpServerConnectionStatus

// Re-export MCP server connection status constants
const (
	McpServerConnectionStatusConnected = control.McpServerConnectionStatusConnected
	McpServerConnectionStatusFailed    = control.McpServerConnectionStatusFailed
	McpServerConnectionStatusNeedsAuth = control.McpServerConnectionStatusNeedsAuth
	McpServerConnectionStatusPending   = control.McpServerConnectionStatusPending
	McpServerConnectionStatusDisabled  = control.McpServerConnectionStatusDisabled
)

// Re-export MCP server config type constants for McpServerStatusConfig.Type.
const (
	McpServerConfigTypeStdio    = control.McpServerConfigTypeStdio
	McpServerConfigTypeSSE      = control.McpServerConfigTypeSSE
	McpServerConfigTypeHTTP     = control.McpServerConfigTypeHTTP
	McpServerConfigTypeSDK      = control.McpServerConfigTypeSDK
	McpServerConfigTypeClaudeAI = control.McpServerConfigTypeClaudeAI
)

// McpServerInfo contains version information about a connected MCP server.
type McpServerInfo = control.McpServerInfo

// McpToolAnnotations describes behavioral hints for an MCP tool.
type McpToolAnnotations = control.McpToolAnnotations

// McpToolInfo describes a tool exposed by an MCP server.
type McpToolInfo = control.McpToolInfo

// McpServerStatusConfig covers all MCP server config variants, discriminated by Type.
type McpServerStatusConfig = control.McpServerStatusConfig

// McpServerStatus contains the full status of a single MCP server.
type McpServerStatus = control.McpServerStatus

// McpStatusResponse is the response payload for a GetMcpStatus request.
type McpStatusResponse = control.McpStatusResponse

// ControlProtocol manages bidirectional control communication with CLI.
type ControlProtocol = control.Protocol

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
	SubtypeStopTask          = control.SubtypeStopTask

	// Control response subtypes
	ResponseSubtypeSuccess = control.ResponseSubtypeSuccess
	ResponseSubtypeError   = control.ResponseSubtypeError
)
