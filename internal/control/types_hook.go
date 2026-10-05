// Package control provides hook types for lifecycle event handling.
package control

import "context"

// HookEvent represents lifecycle events that can trigger hooks.
type HookEvent string

const (
	// HookEventPreToolUse is triggered before a tool is executed.
	HookEventPreToolUse HookEvent = "PreToolUse"
	// HookEventPostToolUse is triggered after a tool is executed.
	HookEventPostToolUse HookEvent = "PostToolUse"
	// HookEventPostToolUseFailure is triggered after a tool execution fails.
	// Distinct from PostToolUse, which fires on success.
	HookEventPostToolUseFailure HookEvent = "PostToolUseFailure"
	// HookEventUserPromptSubmit is triggered when a user submits a prompt.
	HookEventUserPromptSubmit HookEvent = "UserPromptSubmit"
	// HookEventStop is triggered when the session is stopping.
	HookEventStop HookEvent = "Stop"
	// HookEventSubagentStop is triggered when a subagent is stopping.
	HookEventSubagentStop HookEvent = "SubagentStop"
	// HookEventPreCompact is triggered before context compaction.
	HookEventPreCompact HookEvent = "PreCompact"
	// HookEventNotification is triggered when the CLI emits a notification.
	HookEventNotification HookEvent = "Notification"
	// HookEventSubagentStart is triggered when a subagent starts.
	HookEventSubagentStart HookEvent = "SubagentStart"
	// HookEventPermissionRequest is triggered when a permission is requested.
	HookEventPermissionRequest HookEvent = "PermissionRequest"
)

// BaseHookInput contains common fields present across all hook events.
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
type PreToolUseHookInput struct {
	BaseHookInput
	// HookEventName is always "PreToolUse".
	HookEventName string `json:"hook_event_name"`
	// ToolName is the name of the tool being executed.
	ToolName string `json:"tool_name"`
	// ToolInput contains the tool's input parameters.
	ToolInput map[string]any `json:"tool_input"`
	// ToolUseID identifies the tool invocation.
	ToolUseID string `json:"tool_use_id"`
	// AgentID is set only when the hook fires inside a sub-agent.
	AgentID *string `json:"agent_id,omitempty"`
	// AgentType names the agent; also set on the main thread of a session started with --agent.
	AgentType *string `json:"agent_type,omitempty"`
}

// PostToolUseHookInput is the input for PostToolUse hook events.
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
	// ToolUseID identifies the tool invocation.
	ToolUseID string `json:"tool_use_id"`
	// AgentID is set only when the hook fires inside a sub-agent.
	AgentID *string `json:"agent_id,omitempty"`
	// AgentType names the agent; also set on the main thread of a session started with --agent.
	AgentType *string `json:"agent_type,omitempty"`
}

// PostToolUseFailureHookInput is the input for PostToolUseFailure hook events.
type PostToolUseFailureHookInput struct {
	BaseHookInput
	// HookEventName is always "PostToolUseFailure".
	HookEventName string `json:"hook_event_name"`
	// ToolName is the name of the tool that failed.
	ToolName string `json:"tool_name"`
	// ToolInput contains the tool's input parameters.
	ToolInput map[string]any `json:"tool_input"`
	// ToolUseID identifies the failed tool invocation.
	ToolUseID string `json:"tool_use_id"`
	// Error describes the failure.
	Error string `json:"error"`
	// IsInterrupt is true when the failure was caused by user interrupt.
	// Optional; nil means the key was absent from JSON.
	IsInterrupt *bool `json:"is_interrupt,omitempty"`
	// AgentID is set only when the hook fires inside a sub-agent.
	AgentID *string `json:"agent_id,omitempty"`
	// AgentType names the agent; also set on the main thread of a session started with --agent.
	AgentType *string `json:"agent_type,omitempty"`
}

// UserPromptSubmitHookInput is the input for UserPromptSubmit hook events.
type UserPromptSubmitHookInput struct {
	BaseHookInput
	// HookEventName is always "UserPromptSubmit".
	HookEventName string `json:"hook_event_name"`
	// Prompt is the user's submitted prompt.
	Prompt string `json:"prompt"`
}

// StopHookInput is the input for Stop hook events.
type StopHookInput struct {
	BaseHookInput
	// HookEventName is always "Stop".
	HookEventName string `json:"hook_event_name"`
	// StopHookActive indicates if the stop hook is currently active.
	StopHookActive bool `json:"stop_hook_active"`
}

// SubagentStopHookInput is the input for SubagentStop hook events.
type SubagentStopHookInput struct {
	BaseHookInput
	// HookEventName is always "SubagentStop".
	HookEventName string `json:"hook_event_name"`
	// StopHookActive indicates if the stop hook is currently active.
	StopHookActive bool `json:"stop_hook_active"`
	// AgentID is the correlation key for the stopping subagent.
	AgentID string `json:"agent_id"`
	// AgentTranscriptPath is the path to the subagent's transcript.
	AgentTranscriptPath string `json:"agent_transcript_path"`
	// AgentType is the subagent type (e.g. "researcher").
	AgentType string `json:"agent_type"`
}

// PreCompactHookInput is the input for PreCompact hook events.
type PreCompactHookInput struct {
	BaseHookInput
	// HookEventName is always "PreCompact".
	HookEventName string `json:"hook_event_name"`
	// Trigger is either "manual" or "auto".
	Trigger string `json:"trigger"`
	// CustomInstructions contains custom compaction instructions (optional).
	CustomInstructions *string `json:"custom_instructions,omitempty"`
}

// NotificationHookInput is the input for Notification hook events.
type NotificationHookInput struct {
	BaseHookInput
	// HookEventName is always "Notification".
	HookEventName string `json:"hook_event_name"`
	// Message is the notification body.
	Message string `json:"message"`
	// Title is the optional notification title.
	// nil means the key was absent from JSON.
	Title *string `json:"title,omitempty"`
	// NotificationType classifies the notification (e.g. "permission_request").
	NotificationType string `json:"notification_type"`
}

// SubagentStartHookInput is the input for SubagentStart hook events.
type SubagentStartHookInput struct {
	BaseHookInput
	// HookEventName is always "SubagentStart".
	HookEventName string `json:"hook_event_name"`
	// AgentID is the correlation key for the starting subagent.
	AgentID string `json:"agent_id"`
	// AgentType is the subagent type (e.g. "researcher").
	AgentType string `json:"agent_type"`
}

// PermissionRequestHookInput is the input for PermissionRequest hook events.
type PermissionRequestHookInput struct {
	BaseHookInput
	// HookEventName is always "PermissionRequest".
	HookEventName string `json:"hook_event_name"`
	// ToolName is the name of the tool requesting permission.
	ToolName string `json:"tool_name"`
	// ToolInput contains the tool's input parameters.
	ToolInput map[string]any `json:"tool_input"`
	// PermissionSuggestions carries CLI-provided permission suggestions.
	// nil means the key was absent from JSON.
	PermissionSuggestions []any `json:"permission_suggestions,omitempty"`
	// AgentID is set only when the hook fires inside a sub-agent.
	AgentID *string `json:"agent_id,omitempty"`
	// AgentType names the agent; also set on the main thread of a session started with --agent.
	AgentType *string `json:"agent_type,omitempty"`
}

// PreToolUseHookSpecificOutput contains PreToolUse-specific output fields.
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
type PostToolUseHookSpecificOutput struct {
	// HookEventName is always "PostToolUse".
	HookEventName string `json:"hookEventName"`
	// AdditionalContext provides extra context for Claude.
	AdditionalContext *string `json:"additionalContext,omitempty"`
	// UpdatedMCPToolOutput allows the hook to replace the tool output
	// reported back to Claude. Wire tag preserves camelCase (updatedMCPToolOutput).
	UpdatedMCPToolOutput any `json:"updatedMCPToolOutput,omitempty"`
}

// PostToolUseFailureHookSpecificOutput contains PostToolUseFailure-specific output fields.
// Structurally identical to PostToolUseHookSpecificOutput; only the HookEventName literal differs.
type PostToolUseFailureHookSpecificOutput struct {
	// HookEventName is always "PostToolUseFailure".
	HookEventName string `json:"hookEventName"`
	// AdditionalContext provides extra context for Claude.
	AdditionalContext *string `json:"additionalContext,omitempty"`
}

// UserPromptSubmitHookSpecificOutput contains UserPromptSubmit-specific output fields.
type UserPromptSubmitHookSpecificOutput struct {
	// HookEventName is always "UserPromptSubmit".
	HookEventName string `json:"hookEventName"`
	// AdditionalContext provides extra context for Claude.
	AdditionalContext *string `json:"additionalContext,omitempty"`
}

// NotificationHookSpecificOutput contains Notification-specific output fields.
type NotificationHookSpecificOutput struct {
	// HookEventName is always "Notification".
	HookEventName string `json:"hookEventName"`
	// AdditionalContext provides extra context for Claude.
	AdditionalContext *string `json:"additionalContext,omitempty"`
}

// SubagentStartHookSpecificOutput contains SubagentStart-specific output fields.
type SubagentStartHookSpecificOutput struct {
	// HookEventName is always "SubagentStart".
	HookEventName string `json:"hookEventName"`
	// AdditionalContext provides extra context for Claude.
	AdditionalContext *string `json:"additionalContext,omitempty"`
}

// PermissionRequestHookSpecificOutput contains PermissionRequest-specific output fields.
// Decision is required (not omitempty).
type PermissionRequestHookSpecificOutput struct {
	// HookEventName is always "PermissionRequest".
	HookEventName string `json:"hookEventName"`
	// Decision conveys the permission decision back to the CLI.
	Decision map[string]any `json:"decision"`
}

// HookJSONOutput is the synchronous hook output structure.
type HookJSONOutput struct {
	// Continue indicates whether Claude should proceed (default: true).
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
type AsyncHookJSONOutput struct {
	// Async must be true for async hook output.
	Async bool `json:"async"`
	// AsyncTimeout is the timeout in milliseconds for the async operation.
	AsyncTimeout int `json:"asyncTimeout,omitempty"`
}

// HookContext provides context information for hook callbacks.
type HookContext struct {
	// Signal is reserved for future abort signal support.
	// Currently always holds the parent context for cancellation.
	Signal context.Context `json:"-"`
}

// HookCallback is the function signature for hook callbacks.
//
// Parameters:
//   - ctx: Context for cancellation and timeouts
//   - input: Hook input (PreToolUseHookInput, PostToolUseHookInput, etc.)
//   - toolUseID: Optional tool use identifier (only for tool-related hooks)
//   - hookCtx: Hook context with signal support
//
// Returns:
//   - HookJSONOutput: The hook's response
//   - error: Non-nil if the callback encounters an error
type HookCallback func(
	ctx context.Context,
	input any,
	toolUseID *string,
	hookCtx HookContext,
) (HookJSONOutput, error)

// HookMatcher defines which hooks to trigger for a given pattern.
type HookMatcher struct {
	// Matcher is a tool name pattern (e.g., "Bash", "Write|Edit|MultiEdit").
	// Empty string matches all tools.
	Matcher string `json:"matcher"`

	// Hooks are the callbacks to execute when the pattern matches.
	// Not serialized to JSON.
	Hooks []HookCallback `json:"-"`

	// Timeout is the maximum time in seconds for all hooks in this matcher.
	// Default is 60 seconds.
	Timeout *float64 `json:"timeout,omitempty"`
}

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

// HookRegistration represents a hook registration for initialization.
type HookRegistration struct {
	// CallbackID is the unique identifier for this callback.
	CallbackID string `json:"callback_id"`
	// Matcher is the tool name pattern.
	Matcher string `json:"matcher"`
	// Timeout is the maximum time in seconds.
	Timeout *float64 `json:"timeout,omitempty"`
}
