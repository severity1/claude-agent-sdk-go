// Package control provides hook types for lifecycle event handling.
// This file implements 100% feature parity with the Python SDK hooks system.
package control

import "context"

// =============================================================================
// Hook Event Types (Python SDK: types.py:163-170)
// =============================================================================

// HookEvent represents lifecycle events that can trigger hooks.
// Matches Python SDK's HookEvent Literal type exactly.
type HookEvent string

const (
	// HookEventPreToolUse is triggered before a tool is executed.
	HookEventPreToolUse HookEvent = "PreToolUse"
	// HookEventPostToolUse is triggered after a tool is executed.
	HookEventPostToolUse HookEvent = "PostToolUse"
	// HookEventUserPromptSubmit is triggered when a user submits a prompt.
	HookEventUserPromptSubmit HookEvent = "UserPromptSubmit"
	// HookEventStop is triggered when the session is stopping.
	HookEventStop HookEvent = "Stop"
	// HookEventSubagentStop is triggered when a subagent is stopping.
	HookEventSubagentStop HookEvent = "SubagentStop"
	// HookEventPreCompact is triggered before context compaction.
	HookEventPreCompact HookEvent = "PreCompact"
	// HookEventPostToolUseFailure is triggered after a tool execution fails.
	HookEventPostToolUseFailure HookEvent = "PostToolUseFailure"
	// HookEventNotification is triggered when a notification is sent.
	HookEventNotification HookEvent = "Notification"
	// HookEventSubagentStart is triggered when a subagent starts.
	HookEventSubagentStart HookEvent = "SubagentStart"
	// HookEventPermissionRequest is triggered when a permission request is made.
	HookEventPermissionRequest HookEvent = "PermissionRequest"
)

// =============================================================================
// Hook Input Types (Python SDK: types.py:174-237)
// =============================================================================

// BaseHookInput contains common fields present across all hook events.
// Matches Python SDK's BaseHookInput TypedDict.
type BaseHookInput struct {
	// SessionID is the unique identifier for the session.
	SessionID string `json:"session_id"`
	// TranscriptPath is the path to the transcript file.
	TranscriptPath string `json:"transcript_path"`
	// Cwd is the current working directory.
	Cwd string `json:"cwd"`
	// PermissionMode is the current permission mode (optional).
	PermissionMode string `json:"permission_mode,omitempty"`
}

// PreToolUseHookInput is the input for PreToolUse hook events.
// Matches Python SDK's PreToolUseHookInput TypedDict.
type PreToolUseHookInput struct {
	BaseHookInput
	// HookEventName is always "PreToolUse".
	HookEventName string `json:"hook_event_name"`
	// ToolName is the name of the tool being executed.
	ToolName string `json:"tool_name"`
	// ToolInput contains the tool's input parameters.
	ToolInput map[string]any `json:"tool_input"`
	// ToolUseID is the unique identifier for this tool use.
	// Required per Python SDK PreToolUseHookInput.
	ToolUseID string `json:"tool_use_id"`
	// AgentID identifies the agent making the tool use (optional).
	AgentID *string `json:"agent_id,omitempty"`
	// AgentType identifies the type of agent (optional).
	AgentType *string `json:"agent_type,omitempty"`
}

// PostToolUseHookInput is the input for PostToolUse hook events.
// Matches Python SDK's PostToolUseHookInput TypedDict.
type PostToolUseHookInput struct {
	BaseHookInput
	// HookEventName is always "PostToolUse".
	HookEventName string `json:"hook_event_name"`
	// ToolName is the name of the tool that was executed.
	ToolName string `json:"tool_name"`
	// ToolInput contains the tool's input parameters.
	ToolInput map[string]any `json:"tool_input"`
	// ToolResponse contains the tool's output.
	ToolResponse any `json:"tool_response"`
	// ToolUseID is the unique identifier for this tool use.
	// Required per Python SDK PostToolUseHookInput.
	ToolUseID string `json:"tool_use_id"`
	// AgentID identifies the agent making the tool use (optional).
	AgentID *string `json:"agent_id,omitempty"`
	// AgentType identifies the type of agent (optional).
	AgentType *string `json:"agent_type,omitempty"`
}

// UserPromptSubmitHookInput is the input for UserPromptSubmit hook events.
// Matches Python SDK's UserPromptSubmitHookInput TypedDict.
type UserPromptSubmitHookInput struct {
	BaseHookInput
	// HookEventName is always "UserPromptSubmit".
	HookEventName string `json:"hook_event_name"`
	// Prompt is the user's submitted prompt.
	Prompt string `json:"prompt"`
}

// StopHookInput is the input for Stop hook events.
// Matches Python SDK's StopHookInput TypedDict.
type StopHookInput struct {
	BaseHookInput
	// HookEventName is always "Stop".
	HookEventName string `json:"hook_event_name"`
	// StopHookActive indicates if the stop hook is currently active.
	StopHookActive bool `json:"stop_hook_active"`
}

// SubagentStopHookInput is the input for SubagentStop hook events.
// Matches Python SDK's SubagentStopHookInput TypedDict.
type SubagentStopHookInput struct {
	BaseHookInput
	// HookEventName is always "SubagentStop".
	HookEventName string `json:"hook_event_name"`
	// StopHookActive indicates if the stop hook is currently active.
	StopHookActive bool `json:"stop_hook_active"`
	// AgentID identifies the subagent.
	AgentID string `json:"agent_id"`
	// AgentTranscriptPath is the path to the subagent's transcript file.
	AgentTranscriptPath string `json:"agent_transcript_path"`
	// AgentType identifies the type of subagent.
	AgentType string `json:"agent_type"`
}

// PreCompactHookInput is the input for PreCompact hook events.
// Matches Python SDK's PreCompactHookInput TypedDict.
type PreCompactHookInput struct {
	BaseHookInput
	// HookEventName is always "PreCompact".
	HookEventName string `json:"hook_event_name"`
	// Trigger is either "manual" or "auto".
	Trigger string `json:"trigger"`
	// CustomInstructions contains custom compaction instructions (optional).
	CustomInstructions *string `json:"custom_instructions,omitempty"`
}

// PostToolUseFailureHookInput is the input for PostToolUseFailure hook events.
// Matches Python SDK's PostToolUseFailureHookInput TypedDict (PR #535).
type PostToolUseFailureHookInput struct {
	BaseHookInput
	// HookEventName is always "PostToolUseFailure".
	HookEventName string `json:"hook_event_name"`
	// ToolName is the name of the tool that failed.
	ToolName string `json:"tool_name"`
	// ToolInput contains the tool's input parameters.
	ToolInput map[string]any `json:"tool_input"`
	// ToolUseID is the unique identifier for this tool use.
	// Required per Python SDK PostToolUseFailureHookInput.
	ToolUseID string `json:"tool_use_id"`
	// Error is the error message from the failed tool execution.
	Error string `json:"error"`
	// IsInterrupt indicates if the failure was due to an interrupt (optional).
	IsInterrupt *bool `json:"is_interrupt,omitempty"`
	// AgentID identifies the agent making the tool use (optional).
	AgentID *string `json:"agent_id,omitempty"`
	// AgentType identifies the type of agent (optional).
	AgentType *string `json:"agent_type,omitempty"`
}

// NotificationHookInput is the input for Notification hook events.
// Matches Python SDK's NotificationHookInput TypedDict (PR #545).
type NotificationHookInput struct {
	BaseHookInput
	// HookEventName is always "Notification".
	HookEventName string `json:"hook_event_name"`
	// Message is the notification message.
	Message string `json:"message"`
	// Title is the notification title (optional).
	Title *string `json:"title,omitempty"`
	// NotificationType is the type of notification.
	NotificationType string `json:"notification_type"`
}

// SubagentStartHookInput is the input for SubagentStart hook events.
// Matches Python SDK's SubagentStartHookInput TypedDict (PR #545).
type SubagentStartHookInput struct {
	BaseHookInput
	// HookEventName is always "SubagentStart".
	HookEventName string `json:"hook_event_name"`
	// AgentID identifies the subagent that is starting.
	AgentID string `json:"agent_id"`
	// AgentType identifies the type of subagent.
	AgentType string `json:"agent_type"`
}

// PermissionRequestHookInput is the input for PermissionRequest hook events.
// Matches Python SDK's PermissionRequestHookInput TypedDict (PR #545).
type PermissionRequestHookInput struct {
	BaseHookInput
	// HookEventName is always "PermissionRequest".
	HookEventName string `json:"hook_event_name"`
	// ToolName is the name of the tool requesting permission.
	ToolName string `json:"tool_name"`
	// ToolInput contains the tool's input parameters.
	ToolInput map[string]any `json:"tool_input"`
	// PermissionSuggestions are the suggested permission options (optional).
	PermissionSuggestions []any `json:"permission_suggestions,omitempty"`
	// AgentID identifies the agent making the request (optional).
	AgentID *string `json:"agent_id,omitempty"`
	// AgentType identifies the type of agent (optional).
	AgentType *string `json:"agent_type,omitempty"`
}

// =============================================================================
// Hook-Specific Output Types (Python SDK: types.py:241-276)
// =============================================================================

// PreToolUseHookSpecificOutput contains PreToolUse-specific output fields.
// Matches Python SDK's PreToolUseHookSpecificOutput TypedDict.
type PreToolUseHookSpecificOutput struct {
	// HookEventName is always "PreToolUse".
	HookEventName string `json:"hookEventName"`
	// PermissionDecision is "allow", "deny", or "ask".
	PermissionDecision *string `json:"permissionDecision,omitempty"`
	// PermissionDecisionReason explains the decision.
	PermissionDecisionReason *string `json:"permissionDecisionReason,omitempty"`
	// UpdatedInput contains modified tool input (optional).
	UpdatedInput map[string]any `json:"updatedInput,omitempty"`
	// AdditionalContext provides extra context for Claude.
	AdditionalContext *string `json:"additionalContext,omitempty"`
}

// PostToolUseHookSpecificOutput contains PostToolUse-specific output fields.
// Matches Python SDK's PostToolUseHookSpecificOutput TypedDict.
type PostToolUseHookSpecificOutput struct {
	// HookEventName is always "PostToolUse".
	HookEventName string `json:"hookEventName"`
	// AdditionalContext provides extra context for Claude.
	AdditionalContext *string `json:"additionalContext,omitempty"`
	// UpdatedMCPToolOutput rewrites MCP tool output sent to Claude. Accepts a
	// string or structured MCP content payload. Leave nil to omit; zero-value
	// scalars ("", 0, false) are serialized (omitempty only drops nil for any).
	UpdatedMCPToolOutput any `json:"updatedMCPToolOutput,omitempty"`
}

// UserPromptSubmitHookSpecificOutput contains UserPromptSubmit-specific output fields.
// Matches Python SDK's UserPromptSubmitHookSpecificOutput TypedDict.
type UserPromptSubmitHookSpecificOutput struct {
	// HookEventName is always "UserPromptSubmit".
	HookEventName string `json:"hookEventName"`
	// AdditionalContext provides extra context for Claude.
	AdditionalContext *string `json:"additionalContext,omitempty"`
}

// PostToolUseFailureHookSpecificOutput contains PostToolUseFailure-specific output fields.
// Matches Python SDK's PostToolUseFailureHookSpecificOutput TypedDict (PR #535).
type PostToolUseFailureHookSpecificOutput struct {
	// HookEventName is always "PostToolUseFailure".
	HookEventName string `json:"hookEventName"`
	// AdditionalContext provides extra context for Claude.
	AdditionalContext *string `json:"additionalContext,omitempty"`
}

// NotificationHookSpecificOutput contains Notification-specific output fields.
// Matches Python SDK's NotificationHookSpecificOutput TypedDict (PR #545).
type NotificationHookSpecificOutput struct {
	// HookEventName is always "Notification".
	HookEventName string `json:"hookEventName"`
	// AdditionalContext provides extra context for Claude.
	AdditionalContext *string `json:"additionalContext,omitempty"`
}

// SubagentStartHookSpecificOutput contains SubagentStart-specific output fields.
// Matches Python SDK's SubagentStartHookSpecificOutput TypedDict (PR #545).
type SubagentStartHookSpecificOutput struct {
	// HookEventName is always "SubagentStart".
	HookEventName string `json:"hookEventName"`
	// AdditionalContext provides extra context for Claude.
	AdditionalContext *string `json:"additionalContext,omitempty"`
}

// PermissionRequestHookSpecificOutput contains PermissionRequest-specific output fields.
// Matches Python SDK's PermissionRequestHookSpecificOutput TypedDict (PR #545).
type PermissionRequestHookSpecificOutput struct {
	// HookEventName is always "PermissionRequest".
	HookEventName string `json:"hookEventName"`
	// Decision is the permission decision payload sent back to the CLI
	// (e.g., {"behavior": "allow", "updatedInput": {...}}). Required by Python
	// SDK; opaque map here for forward compatibility with future decision shapes.
	Decision map[string]any `json:"decision"`
}

// =============================================================================
// Hook Output Types (Python SDK: types.py:286-345)
// =============================================================================

// HookJSONOutput is the synchronous hook output structure.
// Matches Python SDK's SyncHookJSONOutput TypedDict.
// Note: Go can use "continue" and "async" directly (not keywords like in Python).
type HookJSONOutput struct {
	// Continue indicates whether Claude should proceed (default: true).
	// Python SDK uses continue_ to avoid keyword conflict.
	Continue *bool `json:"continue,omitempty"`
	// SuppressOutput hides stdout from transcript mode.
	SuppressOutput *bool `json:"suppressOutput,omitempty"`
	// StopReason is the message shown when Continue is false.
	StopReason *string `json:"stopReason,omitempty"`

	// Decision can be "block" to indicate blocking behavior.
	Decision *string `json:"decision,omitempty"`
	// SystemMessage is a warning message displayed to the user.
	SystemMessage *string `json:"systemMessage,omitempty"`
	// Reason is feedback for Claude about the decision.
	Reason *string `json:"reason,omitempty"`

	// HookSpecificOutput contains event-specific output fields.
	HookSpecificOutput any `json:"hookSpecificOutput,omitempty"`
}

// AsyncHookJSONOutput indicates the hook will respond asynchronously.
// Matches Python SDK's AsyncHookJSONOutput TypedDict.
type AsyncHookJSONOutput struct {
	// Async must be true for async hook output.
	// Python SDK uses async_ to avoid keyword conflict.
	Async bool `json:"async"`
	// AsyncTimeout is the timeout in milliseconds for the async operation.
	AsyncTimeout int `json:"asyncTimeout,omitempty"`
}

// =============================================================================
// Hook Context (Python SDK: types.py:348-355)
// =============================================================================

// HookContext provides context information for hook callbacks.
// Matches Python SDK's HookContext TypedDict.
type HookContext struct {
	// Signal is reserved for future abort signal support.
	// Currently always holds the parent context for cancellation.
	Signal context.Context `json:"-"`
}

// =============================================================================
// Hook Callback Type (Python SDK: types.py:358-365)
// =============================================================================

// HookCallback is the function signature for hook callbacks.
// Go idiom: context.Context as first parameter, (result, error) return.
// Python SDK uses async callback; Go uses synchronous with context for cancellation.
//
// Parameters:
//
//   - ctx: Context for cancellation and timeouts
//
//   - input: Hook input - typed as any because the concrete input struct
//     depends on which event fired. The callback must type-switch on the
//     event it was registered for:
//
//     HookEventPreToolUse           -> *PreToolUseHookInput
//     HookEventPostToolUse          -> *PostToolUseHookInput
//     HookEventPostToolUseFailure   -> *PostToolUseFailureHookInput
//     HookEventUserPromptSubmit     -> *UserPromptSubmitHookInput
//     HookEventStop                 -> *StopHookInput
//     HookEventSubagentStop         -> *SubagentStopHookInput
//     HookEventPreCompact           -> *PreCompactHookInput
//     HookEventNotification         -> *NotificationHookInput
//     HookEventSubagentStart        -> *SubagentStartHookInput
//     HookEventPermissionRequest    -> *PermissionRequestHookInput
//
//     Unknown event types pass the raw input map[string]any (forward compat).
//
//   - toolUseID: Optional tool use identifier (only for tool-related hooks)
//
//   - hookCtx: Hook context with signal support
//
// Returns:
//   - HookJSONOutput: The hook's response. When HookSpecificOutput is set,
//     it must be the event-specific output type matching the event
//     (PreToolUseHookSpecificOutput, PostToolUseHookSpecificOutput, etc.);
//     there is no compile-time guard against mismatched types.
//   - error: Non-nil if the callback encounters an error
type HookCallback func(
	ctx context.Context,
	input any,
	toolUseID *string,
	hookCtx HookContext,
) (HookJSONOutput, error)

// =============================================================================
// Hook Matcher (Python SDK: types.py:369-383)
// =============================================================================

// HookMatcher defines which hooks to trigger for a given pattern.
// Matches Python SDK's HookMatcher dataclass.
type HookMatcher struct {
	// Matcher is a tool name pattern (e.g., "Bash", "Write|Edit|MultiEdit").
	// Empty string matches all tools (Python SDK: None).
	Matcher string `json:"matcher"`

	// Hooks are the callbacks to execute when the pattern matches.
	// Not serialized to JSON.
	Hooks []HookCallback `json:"-"`

	// Timeout is the maximum time in seconds for all hooks in this matcher.
	// Default is 60 seconds (Python SDK default).
	Timeout *float64 `json:"timeout,omitempty"`
}

// =============================================================================
// Hook Registration Types (for initialize request)
// =============================================================================

// HookMatcherConfig is the serializable format for the initialize request.
// This is what gets sent to the CLI during initialization.
type HookMatcherConfig struct {
	// Matcher is a tool name pattern.
	Matcher string `json:"matcher"`
	// HookCallbackIDs are the generated callback IDs for this matcher.
	HookCallbackIDs []string `json:"hookCallbackIds"`
	// Timeout is the maximum time in seconds.
	Timeout *float64 `json:"timeout,omitempty"`
}
