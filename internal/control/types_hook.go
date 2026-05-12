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
	// HookEventPostToolUseFailure is triggered after a tool execution fails.
	// Distinct from PostToolUse, which fires on success. Added in Python SDK PR #535.
	HookEventPostToolUseFailure HookEvent = "PostToolUseFailure"
	// HookEventUserPromptSubmit is triggered when a user submits a prompt.
	HookEventUserPromptSubmit HookEvent = "UserPromptSubmit"
	// HookEventStop is triggered when the session is stopping.
	HookEventStop HookEvent = "Stop"
	// HookEventSubagentStop is triggered when a subagent is stopping.
	HookEventSubagentStop HookEvent = "SubagentStop"
	// HookEventPreCompact is triggered before context compaction.
	HookEventPreCompact HookEvent = "PreCompact"
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
}

// PostToolUseFailureHookInput is the input for PostToolUseFailure hook events.
// Matches Python SDK's PostToolUseFailureHookInput TypedDict (PR #535).
// _SubagentContextMixin fields (agent_id/agent_type) deferred to Phase 2 item #13
// (Python PR #628), which is where Python introduces the mixin and applies it to
// PostToolUseFailureHookInput alongside the other three tool-lifecycle inputs.
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
	// Optional; nil maps to Python's NotRequired[bool] absent state.
	IsInterrupt *bool `json:"is_interrupt,omitempty"`
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
}

// PostToolUseHookSpecificOutput contains PostToolUse-specific output fields.
// Matches Python SDK's PostToolUseHookSpecificOutput TypedDict.
type PostToolUseHookSpecificOutput struct {
	// HookEventName is always "PostToolUse".
	HookEventName string `json:"hookEventName"`
	// AdditionalContext provides extra context for Claude.
	AdditionalContext *string `json:"additionalContext,omitempty"`
}

// PostToolUseFailureHookSpecificOutput contains PostToolUseFailure-specific output fields.
// Matches Python SDK's PostToolUseFailureHookSpecificOutput TypedDict (PR #535).
// Structurally identical to PostToolUseHookSpecificOutput; only the HookEventName literal differs.
type PostToolUseFailureHookSpecificOutput struct {
	// HookEventName is always "PostToolUseFailure".
	HookEventName string `json:"hookEventName"`
	// AdditionalContext provides extra context for Claude.
	AdditionalContext *string `json:"additionalContext,omitempty"`
}

// UserPromptSubmitHookSpecificOutput contains UserPromptSubmit-specific output fields.
// Matches Python SDK's UserPromptSubmitHookSpecificOutput TypedDict.
type UserPromptSubmitHookSpecificOutput struct {
	// HookEventName is always "UserPromptSubmit".
	HookEventName string `json:"hookEventName"`
	// AdditionalContext provides extra context for Claude.
	AdditionalContext *string `json:"additionalContext,omitempty"`
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

// HookRegistration represents a hook registration for initialization.
type HookRegistration struct {
	// CallbackID is the unique identifier for this callback.
	CallbackID string `json:"callback_id"`
	// Matcher is the tool name pattern.
	Matcher string `json:"matcher"`
	// Timeout is the maximum time in seconds.
	Timeout *float64 `json:"timeout,omitempty"`
}
