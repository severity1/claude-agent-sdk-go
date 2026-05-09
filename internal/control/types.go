// Package control provides the SDK control protocol for bidirectional communication with Claude CLI.
// This package enables features like tool permission callbacks, hook callbacks, and MCP message routing.
package control

import (
	"context"
	"encoding/json"

	"github.com/severity1/claude-agent-sdk-go/internal/shared"
)

// Permission behavior wire-format constants (sent to the CLI as the
// discriminator for PermissionResult variants). Always sourced from these
// constants — the Behavior field on the structs is informational and ignored
// during marshaling.
const (
	permissionBehaviorAllow = "allow"
	permissionBehaviorDeny  = "deny"
)

// McpToolAnnotations describes tool behavior hints.
// Type alias from shared to avoid duplication across packages.
type McpToolAnnotations = shared.McpToolAnnotations

// Message type constants for control protocol discrimination.
// Aliased from shared so the wire-format string lives in one place.
const (
	// MessageTypeControlRequest is sent TO the CLI to request an action.
	MessageTypeControlRequest = shared.MessageTypeControlRequest
	// MessageTypeControlResponse is received FROM the CLI as a response.
	MessageTypeControlResponse = shared.MessageTypeControlResponse
)

// Request subtype constants matching Python SDK for 100% parity.
const (
	// SubtypeInterrupt requests interruption of current operation.
	SubtypeInterrupt = "interrupt"
	// SubtypeCanUseTool requests permission to use a tool.
	SubtypeCanUseTool = "can_use_tool"
	// SubtypeInitialize performs the control protocol handshake.
	SubtypeInitialize = "initialize"
	// SubtypeSetPermissionMode changes the permission mode at runtime.
	SubtypeSetPermissionMode = "set_permission_mode"
	// SubtypeSetModel changes the AI model at runtime.
	SubtypeSetModel = "set_model"
	// SubtypeHookCallback invokes a registered hook callback.
	SubtypeHookCallback = "hook_callback"
	// SubtypeMcpMessage routes an MCP message to an SDK MCP server.
	SubtypeMcpMessage = "mcp_message"
	// SubtypeRewindFiles requests file rewind to a specific user message state.
	SubtypeRewindFiles = "rewind_files"
	// SubtypeGetMcpStatus requests the current status of all MCP servers.
	SubtypeGetMcpStatus = "get_mcp_status"
)

// Response subtype constants for control responses.
const (
	// ResponseSubtypeSuccess indicates the request succeeded.
	ResponseSubtypeSuccess = "success"
	// ResponseSubtypeError indicates the request failed.
	ResponseSubtypeError = "error"
)

// SDKControlRequest represents a control request sent TO the CLI.
// This is the envelope that wraps all control request types.
type SDKControlRequest struct {
	// Type is always MessageTypeControlRequest.
	Type string `json:"type"`
	// RequestID is a unique identifier for request/response correlation.
	// Format: req_{counter}_{random_hex}
	RequestID string `json:"request_id"`
	// Request contains the actual request payload (InterruptRequest, InitializeRequest, etc.).
	Request any `json:"request"`
}

// SDKControlResponse represents a control response received FROM the CLI.
// This is the envelope that wraps all control response types.
type SDKControlResponse struct {
	// Type is always MessageTypeControlResponse.
	Type string `json:"type"`
	// Response contains the actual response data.
	Response Response `json:"response"`
}

// Response is the inner response structure within SDKControlResponse.
type Response struct {
	// Subtype is either ResponseSubtypeSuccess or ResponseSubtypeError.
	Subtype string `json:"subtype"`
	// RequestID matches the request that this response is for.
	RequestID string `json:"request_id"`
	// Response contains the response data (only for success).
	Response any `json:"response,omitempty"`
	// Error contains the error message (only for error).
	Error string `json:"error,omitempty"`
}

// InterruptRequest requests interruption of the current operation.
type InterruptRequest struct {
	// Subtype is always SubtypeInterrupt.
	Subtype string `json:"subtype"`
}

// InitializeRequest performs the control protocol handshake.
// This must be sent before any other control requests in streaming mode.
type InitializeRequest struct {
	// Subtype is always SubtypeInitialize.
	Subtype string `json:"subtype"`
	// Hooks contains hook registrations keyed by event type.
	// Format: {"PreToolUse": [...], "PostToolUse": [...]}
	Hooks map[string][]HookMatcherConfig `json:"hooks,omitempty"`
	// Agents contains programmatic subagent definitions keyed by agent name.
	// Format: {"name": {"description": "...", "prompt": "...", "tools": [...], "model": "..."}}
	Agents map[string]map[string]any `json:"agents,omitempty"`
}

// InitializeResponse contains the CLI's response to initialization.
type InitializeResponse struct {
	// SupportedCommands lists the control commands supported by this CLI version.
	SupportedCommands []string `json:"supported_commands,omitempty"`
}

// SetPermissionModeRequest changes the permission mode at runtime.
type SetPermissionModeRequest struct {
	// Subtype is always SubtypeSetPermissionMode.
	Subtype string `json:"subtype"`
	// Mode is the new permission mode to set.
	Mode string `json:"mode"`
}

// SetModelRequest changes the AI model at runtime.
// This matches Python SDK's set_model() behavior exactly.
type SetModelRequest struct {
	// Subtype is always SubtypeSetModel.
	Subtype string `json:"subtype"`
	// Model is the new model to use. Use nil to reset to default.
	// Examples: "claude-sonnet-4-5", "claude-opus-4-1-20250805"
	Model *string `json:"model"`
}

// RewindFilesRequest requests rewinding files to a specific user message state.
// Matches Python SDK's SDKControlRewindFilesRequest structure.
type RewindFilesRequest struct {
	// Subtype is always SubtypeRewindFiles ("rewind_files").
	Subtype string `json:"subtype"`
	// UserMessageID is the UUID of the user message to rewind to.
	// This should be obtained from UserMessage.UUID received during the session.
	UserMessageID string `json:"user_message_id"`
}

// =============================================================================
// MCP Status Types (Python SDK PR #516)
// =============================================================================

// McpServerConnectionStatus represents the connection state of an MCP server.
type McpServerConnectionStatus string

const (
	// McpServerConnectionStatusConnected indicates the server is connected.
	McpServerConnectionStatusConnected McpServerConnectionStatus = "connected"
	// McpServerConnectionStatusFailed indicates the server connection failed.
	McpServerConnectionStatusFailed McpServerConnectionStatus = "failed"
	// McpServerConnectionStatusNeedsAuth indicates the server requires authentication.
	McpServerConnectionStatusNeedsAuth McpServerConnectionStatus = "needs-auth"
	// McpServerConnectionStatusPending indicates the server connection is pending.
	McpServerConnectionStatusPending McpServerConnectionStatus = "pending"
	// McpServerConnectionStatusDisabled indicates the server is disabled.
	McpServerConnectionStatusDisabled McpServerConnectionStatus = "disabled"
)

// McpServerInfo contains version info about a connected MCP server.
type McpServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// McpToolInfo describes a tool exposed by an MCP server (for status reporting).
// NOTE: This is distinct from shared.McpToolDefinition (used for SDK MCP server tool definitions).
// This type is used in McpServerStatus.Tools to report available tools from connected servers.
type McpToolInfo struct {
	Name        string              `json:"name"`
	Description *string             `json:"description,omitempty"`
	Annotations *McpToolAnnotations `json:"annotations,omitempty"`
}

// McpServerStatus represents the status of a connected MCP server.
//
// Config captures the server's wire-format configuration (URL for
// HTTP/SSE servers, type/name for SDK servers, claudeai-proxy details).
// It mirrors Python SDK's McpServerStatusConfig union as a raw map; a
// typed union may be introduced in a later parity phase.
type McpServerStatus struct {
	Name       string                    `json:"name"`
	Status     McpServerConnectionStatus `json:"status"`
	ServerInfo *McpServerInfo            `json:"serverInfo,omitempty"`
	Error      *string                   `json:"error,omitempty"`
	Config     map[string]any            `json:"config,omitempty"`
	Scope      *string                   `json:"scope,omitempty"`
	Tools      []McpToolInfo             `json:"tools,omitempty"`
}

// McpStatusResponse is the response payload from a get_mcp_status request.
type McpStatusResponse struct {
	McpServers []McpServerStatus `json:"mcpServers"`
}

// GetMcpStatusRequest requests the current status of all MCP servers.
type GetMcpStatusRequest struct {
	// Subtype is always SubtypeGetMcpStatus.
	Subtype string `json:"subtype"`
}

// =============================================================================
// Permission Callback Types (Issue #8)
// =============================================================================

// PermissionUpdateType specifies the type of permission update.
// Matches Python SDK's Literal type exactly for 100% parity.
type PermissionUpdateType string

const (
	// PermissionUpdateTypeAddRules adds new permission rules.
	PermissionUpdateTypeAddRules PermissionUpdateType = "addRules"
	// PermissionUpdateTypeReplaceRules replaces all permission rules.
	PermissionUpdateTypeReplaceRules PermissionUpdateType = "replaceRules"
	// PermissionUpdateTypeRemoveRules removes specified permission rules.
	PermissionUpdateTypeRemoveRules PermissionUpdateType = "removeRules"
	// PermissionUpdateTypeSetMode sets the permission mode.
	PermissionUpdateTypeSetMode PermissionUpdateType = "setMode"
	// PermissionUpdateTypeAddDirectories adds directories to allowed list.
	PermissionUpdateTypeAddDirectories PermissionUpdateType = "addDirectories"
	// PermissionUpdateTypeRemoveDirectories removes directories from allowed list.
	PermissionUpdateTypeRemoveDirectories PermissionUpdateType = "removeDirectories"
)

// PermissionRuleValue represents a permission rule.
// JSON tags use camelCase to match CLI protocol.
type PermissionRuleValue struct {
	// ToolName is the name of the tool this rule applies to.
	ToolName string `json:"toolName"`
	// RuleContent is the optional rule content (e.g., path pattern).
	RuleContent *string `json:"ruleContent,omitempty"`
}

// PermissionUpdate represents a dynamic permission rule update.
// Matches Python SDK's PermissionUpdate dataclass.
type PermissionUpdate struct {
	// Type is the kind of permission update.
	Type PermissionUpdateType `json:"type"`
	// Rules are the permission rules to add/replace/remove.
	Rules []PermissionRuleValue `json:"rules,omitempty"`
	// Behavior is the permission behavior (allow/deny).
	Behavior *string `json:"behavior,omitempty"`
	// Mode is the permission mode to set.
	Mode *string `json:"mode,omitempty"`
	// Directories are the directories to add/remove.
	Directories []string `json:"directories,omitempty"`
	// Destination specifies where the update applies (session/user/project).
	Destination *string `json:"destination,omitempty"`
}

// ToolPermissionContext provides context for permission callbacks.
// Matches Python SDK's ToolPermissionContext dataclass.
type ToolPermissionContext struct {
	// Signal is reserved for future abort signal support (currently unused).
	Signal any `json:"-"`
	// Suggestions contains permission suggestions from CLI.
	Suggestions []PermissionUpdate `json:"suggestions,omitempty"`
	// ToolUseID identifies which tool call triggered this permission request.
	// Distinct tool_use_ids let callbacks disambiguate multiple concurrent
	// tool calls within the same assistant message. Nil when the CLI did not
	// supply one (older CLI versions).
	ToolUseID *string `json:"tool_use_id,omitempty"`
	// AgentID identifies the subagent that triggered the permission request,
	// or nil when the main agent triggered it.
	AgentID *string `json:"agent_id,omitempty"`
}

// PermissionResult is the interface for permission callback results.
// Go idiom: unexported marker method for sealed interface pattern.
type PermissionResult interface {
	permissionResult() // Marker method - unexported, lowercase
}

// PermissionResultAllow permits tool execution with optional modifications.
// Behavior is informational and always serialized as "allow" regardless of
// field value (see MarshalJSON), so callers cannot accidentally invert the
// discriminator by mutating the field.
type PermissionResultAllow struct {
	// Behavior is always "allow".
	Behavior string `json:"-"`
	// UpdatedInput contains the modified tool input (optional).
	UpdatedInput map[string]any `json:"updatedInput,omitempty"`
	// UpdatedPermissions contains dynamic permission updates (optional).
	UpdatedPermissions []PermissionUpdate `json:"updatedPermissions,omitempty"`
}

// permissionResult implements PermissionResult marker interface.
func (PermissionResultAllow) permissionResult() {}

// MarshalJSON forces behavior:"allow" on the wire regardless of struct state.
func (a PermissionResultAllow) MarshalJSON() ([]byte, error) {
	type alias PermissionResultAllow
	return json.Marshal(struct {
		Behavior string `json:"behavior"`
		alias
	}{Behavior: permissionBehaviorAllow, alias: alias(a)})
}

// NewPermissionResultAllow creates an Allow result with proper defaults.
// Go idiom: constructor functions for types with required fields.
func NewPermissionResultAllow() PermissionResultAllow {
	return PermissionResultAllow{Behavior: permissionBehaviorAllow}
}

// PermissionResultDeny prevents tool execution.
// Behavior is informational and always serialized as "deny" regardless of
// field value (see MarshalJSON).
type PermissionResultDeny struct {
	// Behavior is always "deny".
	Behavior string `json:"-"`
	// Message is the reason for denial.
	Message string `json:"message,omitempty"`
	// Interrupt indicates whether to interrupt the session.
	Interrupt bool `json:"interrupt,omitempty"`
}

// permissionResult implements PermissionResult marker interface.
func (PermissionResultDeny) permissionResult() {}

// MarshalJSON forces behavior:"deny" on the wire regardless of struct state.
func (d PermissionResultDeny) MarshalJSON() ([]byte, error) {
	type alias PermissionResultDeny
	return json.Marshal(struct {
		Behavior string `json:"behavior"`
		alias
	}{Behavior: permissionBehaviorDeny, alias: alias(d)})
}

// NewPermissionResultDeny creates a Deny result with proper defaults.
func NewPermissionResultDeny(message string) PermissionResultDeny {
	return PermissionResultDeny{Behavior: permissionBehaviorDeny, Message: message}
}

// CanUseToolCallback is invoked when CLI requests permission to use a tool.
// Go idiom: context.Context as first parameter, (result, error) return.
// The callback must be thread-safe as it may be invoked concurrently.
//
// Parameters:
//   - ctx: Context for cancellation and timeouts
//   - toolName: Name of the tool being requested (e.g., "Read", "Write", "Bash")
//   - input: Tool input parameters as a map
//   - permCtx: Context with permission suggestions from CLI
//
// Returns:
//   - PermissionResult: Either PermissionResultAllow or PermissionResultDeny
//   - error: Non-nil if the callback encounters an error
type CanUseToolCallback func(
	ctx context.Context,
	toolName string,
	input map[string]any,
	permCtx ToolPermissionContext,
) (PermissionResult, error)

// =============================================================================
// MCP Server Types (Issue #7)
// =============================================================================

// Type aliases for MCP types from shared package.
// Using type aliases (not type definitions) ensures interface compatibility:
// - shared.McpServer and control.McpServer are the SAME type
// - This allows transport to pass shared.McpServer to control.WithSdkMcpServers()
type (
	// McpServer is the interface for in-process SDK MCP servers.
	McpServer = shared.McpServer
	// McpToolDefinition describes a tool exposed by an MCP server.
	McpToolDefinition = shared.McpToolDefinition
	// McpToolResult represents the result of a tool call.
	McpToolResult = shared.McpToolResult
	// McpContent represents content returned by a tool.
	McpContent = shared.McpContent
)
