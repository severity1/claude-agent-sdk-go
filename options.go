package claudecode

import (
	"context"
	"io"
	"log"
	"os"

	"github.com/severity1/claude-agent-sdk-go/internal/control"
	"github.com/severity1/claude-agent-sdk-go/internal/shared"
)

// Options contains configuration for Claude Code CLI interactions.
type Options = shared.Options

// PermissionMode defines the permission handling mode.
type PermissionMode = shared.PermissionMode

// McpServerType defines the type of MCP server.
type McpServerType = shared.McpServerType

// McpServerConfig represents an MCP server configuration.
type McpServerConfig = shared.McpServerConfig

// McpStdioServerConfig represents a stdio MCP server configuration.
type McpStdioServerConfig = shared.McpStdioServerConfig

// McpSSEServerConfig represents an SSE MCP server configuration.
type McpSSEServerConfig = shared.McpSSEServerConfig

// McpHTTPServerConfig represents an HTTP MCP server configuration.
type McpHTTPServerConfig = shared.McpHTTPServerConfig

// SdkBeta represents a beta feature identifier.
type SdkBeta = shared.SdkBeta

// ToolsPreset represents a preset tools configuration.
type ToolsPreset = shared.ToolsPreset

// SettingSource represents a settings source location.
type SettingSource = shared.SettingSource

// SandboxSettings configures sandbox behavior for bash command execution.
type SandboxSettings = shared.SandboxSettings

// SandboxNetworkConfig configures network access within sandbox.
type SandboxNetworkConfig = shared.SandboxNetworkConfig

// SandboxIgnoreViolations specifies patterns to ignore during sandbox violations.
type SandboxIgnoreViolations = shared.SandboxIgnoreViolations

// SdkPluginType represents the type of SDK plugin.
type SdkPluginType = shared.SdkPluginType

// SdkPluginConfig represents a plugin configuration.
type SdkPluginConfig = shared.SdkPluginConfig

// OutputFormat specifies the format for structured output.
type OutputFormat = shared.OutputFormat

// ThinkingConfig configures the model's extended thinking behavior.
type ThinkingConfig = shared.ThinkingConfig

// ThinkingConfigAdaptive lets the model decide thinking budget adaptively.
type ThinkingConfigAdaptive = shared.ThinkingConfigAdaptive

// ThinkingConfigEnabled enables thinking with an explicit token budget.
type ThinkingConfigEnabled = shared.ThinkingConfigEnabled

// ThinkingConfigDisabled disables thinking explicitly.
type ThinkingConfigDisabled = shared.ThinkingConfigDisabled

// =============================================================================
// Permission Callback Types (Issue #8)
// =============================================================================

// CanUseToolCallback is invoked when CLI requests permission to use a tool.
// The callback receives tool name, input parameters, and permission context.
// Return PermissionResultAllow to permit, PermissionResultDeny to deny.
// The callback must be thread-safe as it may be invoked concurrently.
type CanUseToolCallback = control.CanUseToolCallback

// PermissionResult is the interface for permission callback results.
// Implementations are PermissionResultAllow and PermissionResultDeny.
type PermissionResult = control.PermissionResult

// PermissionResultAllow permits tool execution with optional modifications.
// Use NewPermissionResultAllow() to create with proper defaults.
type PermissionResultAllow = control.PermissionResultAllow

// PermissionResultDeny prevents tool execution.
// Use NewPermissionResultDeny(message) to create with proper defaults.
type PermissionResultDeny = control.PermissionResultDeny

// ToolPermissionContext provides context for permission callbacks.
// Contains suggestions from CLI for permission decisions.
type ToolPermissionContext = control.ToolPermissionContext

// PermissionUpdate represents a dynamic permission rule update.
type PermissionUpdate = control.PermissionUpdate

// PermissionRuleValue represents a permission rule.
type PermissionRuleValue = control.PermissionRuleValue

// PermissionUpdateType specifies the type of permission update.
type PermissionUpdateType = control.PermissionUpdateType

// Re-export constants
const (
	PermissionModeDefault           = shared.PermissionModeDefault
	PermissionModeAcceptEdits       = shared.PermissionModeAcceptEdits
	PermissionModePlan              = shared.PermissionModePlan
	PermissionModeBypassPermissions = shared.PermissionModeBypassPermissions
	McpServerTypeStdio              = shared.McpServerTypeStdio
	McpServerTypeSSE                = shared.McpServerTypeSSE
	McpServerTypeHTTP               = shared.McpServerTypeHTTP
	SdkBetaContext1M                = shared.SdkBetaContext1M
	SettingSourceUser               = shared.SettingSourceUser
	SettingSourceProject            = shared.SettingSourceProject
	SettingSourceLocal              = shared.SettingSourceLocal
	SdkPluginTypeLocal              = shared.SdkPluginTypeLocal
)

// Permission update type constants
const (
	PermissionUpdateTypeAddRules          = control.PermissionUpdateTypeAddRules
	PermissionUpdateTypeReplaceRules      = control.PermissionUpdateTypeReplaceRules
	PermissionUpdateTypeRemoveRules       = control.PermissionUpdateTypeRemoveRules
	PermissionUpdateTypeSetMode           = control.PermissionUpdateTypeSetMode
	PermissionUpdateTypeAddDirectories    = control.PermissionUpdateTypeAddDirectories
	PermissionUpdateTypeRemoveDirectories = control.PermissionUpdateTypeRemoveDirectories
)

// Option configures Options using the functional options pattern.
type Option func(*Options)

// WithAllowedTools sets the allowed tools list.
func WithAllowedTools(tools ...string) Option {
	return func(o *Options) {
		o.AllowedTools = tools
	}
}

// WithDisallowedTools sets the disallowed tools list.
func WithDisallowedTools(tools ...string) Option {
	return func(o *Options) {
		o.DisallowedTools = tools
	}
}

// WithTools sets available tools as a list of tool names.
func WithTools(tools ...string) Option {
	return func(o *Options) {
		o.Tools = tools
	}
}

// WithToolsPreset sets tools to a preset configuration.
func WithToolsPreset(preset string) Option {
	return func(o *Options) {
		o.Tools = ToolsPreset{
			Type:   "preset",
			Preset: preset,
		}
	}
}

// WithClaudeCodeTools sets tools to the claude_code preset.
func WithClaudeCodeTools() Option {
	return WithToolsPreset("claude_code")
}

// WithSystemPrompt sets the system prompt.
func WithSystemPrompt(prompt string) Option {
	return func(o *Options) {
		o.SystemPrompt = &prompt
	}
}

// WithAppendSystemPrompt sets the append system prompt.
func WithAppendSystemPrompt(prompt string) Option {
	return func(o *Options) {
		o.AppendSystemPrompt = &prompt
	}
}

// WithModel sets the model to use.
func WithModel(model string) Option {
	return func(o *Options) {
		o.Model = &model
	}
}

// WithFallbackModel sets the fallback model when primary model is unavailable.
func WithFallbackModel(model string) Option {
	return func(o *Options) {
		o.FallbackModel = &model
	}
}

// WithMaxBudgetUSD sets the maximum budget in USD for API usage.
func WithMaxBudgetUSD(budget float64) Option {
	return func(o *Options) {
		o.MaxBudgetUSD = &budget
	}
}

// WithUser sets the user identifier for tracking and billing.
func WithUser(user string) Option {
	return func(o *Options) {
		o.User = &user
	}
}

// WithMaxBufferSize sets the maximum buffer size for CLI output.
func WithMaxBufferSize(size int) Option {
	return func(o *Options) {
		o.MaxBufferSize = &size
	}
}

// WithMaxThinkingTokens sets the maximum thinking tokens.
//
// Deprecated: Use WithThinkingBudget instead for new code.
func WithMaxThinkingTokens(tokens int) Option {
	return func(o *Options) {
		o.MaxThinkingTokens = tokens
	}
}

// WithThinking sets the thinking configuration.
// Use WithThinkingAdaptive(), WithThinkingBudget(), or WithThinkingDisabled() for convenience.
func WithThinking(config ThinkingConfig) Option {
	return func(o *Options) {
		o.Thinking = config
	}
}

// WithThinkingAdaptive enables adaptive thinking mode where the model decides its budget.
func WithThinkingAdaptive() Option {
	return WithThinking(ThinkingConfigAdaptive{})
}

// WithThinkingBudget enables thinking with an explicit token budget.
// This replaces WithMaxThinkingTokens for new code.
func WithThinkingBudget(tokens int) Option {
	return WithThinking(ThinkingConfigEnabled{BudgetTokens: tokens})
}

// WithThinkingDisabled disables extended thinking explicitly.
func WithThinkingDisabled() Option {
	return WithThinking(ThinkingConfigDisabled{})
}

// WithEffort sets the model's reasoning effort level.
// Valid values: "low", "medium", "high", "max".
func WithEffort(effort string) Option {
	return func(o *Options) {
		o.Effort = &effort
	}
}

// WithPermissionMode sets the permission mode.
func WithPermissionMode(mode PermissionMode) Option {
	return func(o *Options) {
		o.PermissionMode = &mode
	}
}

// WithPermissionPromptToolName sets the permission prompt tool name.
func WithPermissionPromptToolName(toolName string) Option {
	return func(o *Options) {
		o.PermissionPromptToolName = &toolName
	}
}

// WithContinueConversation enables conversation continuation.
func WithContinueConversation(continueConversation bool) Option {
	return func(o *Options) {
		o.ContinueConversation = continueConversation
	}
}

// WithResume sets the session ID to resume.
func WithResume(sessionID string) Option {
	return func(o *Options) {
		o.Resume = &sessionID
	}
}

// WithCwd sets the working directory.
func WithCwd(cwd string) Option {
	return func(o *Options) {
		o.Cwd = &cwd
	}
}

// WithAddDirs adds directories to the context.
func WithAddDirs(dirs ...string) Option {
	return func(o *Options) {
		o.AddDirs = dirs
	}
}

// WithMcpServers sets the MCP server configurations.
func WithMcpServers(servers map[string]McpServerConfig) Option {
	return func(o *Options) {
		o.McpServers = servers
	}
}

// WithSdkMcpServer adds an in-process SDK MCP server by name.
// This is a convenience method for adding SDK MCP servers created with CreateSDKMcpServer.
// Multiple calls accumulate servers.
//
// Example:
//
//	calculator := claudecode.CreateSDKMcpServer("calculator", "1.0.0", addTool, sqrtTool)
//	client := claudecode.NewClient(
//	    claudecode.WithSdkMcpServer("calc", calculator),
//	    claudecode.WithAllowedTools("mcp__calc__add", "mcp__calc__sqrt"),
//	)
func WithSdkMcpServer(name string, server *McpSdkServerConfig) Option {
	return func(o *Options) {
		if o.McpServers == nil {
			o.McpServers = make(map[string]McpServerConfig)
		}
		o.McpServers[name] = server
	}
}

// WithMaxTurns sets the maximum number of conversation turns.
func WithMaxTurns(turns int) Option {
	return func(o *Options) {
		o.MaxTurns = turns
	}
}

// WithSettings sets the settings file path or JSON string.
func WithSettings(settings string) Option {
	return func(o *Options) {
		o.Settings = &settings
	}
}

// WithForkSession enables forking to a new session ID when resuming.
// When true, resumed sessions fork to a new session ID rather than
// continuing the previous session.
func WithForkSession(fork bool) Option {
	return func(o *Options) {
		o.ForkSession = fork
	}
}

// WithSettingSources sets which settings sources to load.
// Valid sources are SettingSourceUser, SettingSourceProject, and SettingSourceLocal.
func WithSettingSources(sources ...SettingSource) Option {
	return func(o *Options) {
		o.SettingSources = sources
	}
}

// WithExtraArgs sets arbitrary CLI flags via ExtraArgs.
func WithExtraArgs(args map[string]*string) Option {
	return func(o *Options) {
		o.ExtraArgs = args
	}
}

// WithCLIPath sets a custom CLI path.
func WithCLIPath(path string) Option {
	return func(o *Options) {
		o.CLIPath = &path
	}
}

// WithEnv sets environment variables for the subprocess.
// Multiple calls to WithEnv or WithEnvVar merge the values.
// Later calls override earlier ones for the same key.
func WithEnv(env map[string]string) Option {
	return func(o *Options) {
		if o.ExtraEnv == nil {
			o.ExtraEnv = make(map[string]string)
		}
		// Merge pattern - idiomatic Go
		for k, v := range env {
			o.ExtraEnv[k] = v
		}
	}
}

// WithEnvVar sets a single environment variable for the subprocess.
// This is a convenience method for setting individual variables.
func WithEnvVar(key, value string) Option {
	return func(o *Options) {
		if o.ExtraEnv == nil {
			o.ExtraEnv = make(map[string]string)
		}
		o.ExtraEnv[key] = value
	}
}

// WithBetas sets the SDK beta features to enable.
// See https://docs.anthropic.com/en/api/beta-headers
func WithBetas(betas ...SdkBeta) Option {
	return func(o *Options) {
		o.Betas = betas
	}
}

// WithSandbox sets the sandbox settings for bash command isolation.
func WithSandbox(sandbox *SandboxSettings) Option {
	return func(o *Options) {
		o.Sandbox = sandbox
	}
}

// WithSandboxEnabled enables or disables sandbox.
// If sandbox settings don't exist, they are initialized.
func WithSandboxEnabled(enabled bool) Option {
	return func(o *Options) {
		if o.Sandbox == nil {
			o.Sandbox = &SandboxSettings{}
		}
		o.Sandbox.Enabled = enabled
	}
}

// WithAutoAllowBashIfSandboxed sets whether to auto-approve bash when sandboxed.
// If sandbox settings don't exist, they are initialized.
func WithAutoAllowBashIfSandboxed(autoAllow bool) Option {
	return func(o *Options) {
		if o.Sandbox == nil {
			o.Sandbox = &SandboxSettings{}
		}
		o.Sandbox.AutoAllowBashIfSandboxed = autoAllow
	}
}

// WithSandboxExcludedCommands sets commands that always bypass sandbox.
// If sandbox settings don't exist, they are initialized.
func WithSandboxExcludedCommands(commands ...string) Option {
	return func(o *Options) {
		if o.Sandbox == nil {
			o.Sandbox = &SandboxSettings{}
		}
		o.Sandbox.ExcludedCommands = commands
	}
}

// WithSandboxNetwork sets the network configuration for sandbox.
// If sandbox settings don't exist, they are initialized.
func WithSandboxNetwork(network *SandboxNetworkConfig) Option {
	return func(o *Options) {
		if o.Sandbox == nil {
			o.Sandbox = &SandboxSettings{}
		}
		o.Sandbox.Network = network
	}
}

// WithPlugins sets the plugin configurations.
// This replaces any previously configured plugins.
func WithPlugins(plugins []SdkPluginConfig) Option {
	return func(o *Options) {
		o.Plugins = plugins
	}
}

// WithPlugin appends a single plugin configuration.
// Multiple calls accumulate plugins.
func WithPlugin(plugin SdkPluginConfig) Option {
	return func(o *Options) {
		o.Plugins = append(o.Plugins, plugin)
	}
}

// WithLocalPlugin appends a local plugin by path.
// This is a convenience method for the common case of local plugins.
func WithLocalPlugin(path string) Option {
	return func(o *Options) {
		o.Plugins = append(o.Plugins, SdkPluginConfig{
			Type: SdkPluginTypeLocal,
			Path: path,
		})
	}
}

// WithAgents sets the programmatic agent definitions.
// This replaces any existing agents.
func WithAgents(agents map[string]AgentDefinition) Option {
	return func(o *Options) {
		o.Agents = agents
	}
}

// WithAgent adds or updates a single agent definition.
// Multiple calls merge agents (later calls override same-name agents).
func WithAgent(name string, agent AgentDefinition) Option {
	return func(o *Options) {
		if o.Agents == nil {
			o.Agents = make(map[string]AgentDefinition)
		}
		o.Agents[name] = agent
	}
}

// NewOptions creates Options with default values using functional options pattern.
func NewOptions(opts ...Option) *Options {
	// Create options with defaults from shared package
	options := shared.NewOptions()

	// Apply functional options
	for _, opt := range opts {
		opt(options)
	}

	return options
}

// WithDebugWriter sets the writer for CLI debug output.
// If not set, stderr is isolated to a temporary file (default behavior).
// Common values: os.Stderr, io.Discard, or a custom io.Writer like bytes.Buffer.
func WithDebugWriter(w io.Writer) Option {
	return func(o *Options) {
		o.DebugWriter = w
	}
}

// WithDebugStderr redirects CLI debug output to os.Stderr.
// This is useful for seeing debug output in real-time during development.
func WithDebugStderr() Option {
	return WithDebugWriter(os.Stderr)
}

// WithDebugDisabled discards all CLI debug output.
// This is more explicit than the default nil behavior but has the same effect.
func WithDebugDisabled() Option {
	return WithDebugWriter(io.Discard)
}

// WithStderrCallback sets a callback for receiving CLI stderr output.
// The callback is invoked for each non-empty line of stderr output.
// Lines are stripped of trailing whitespace before being passed to the callback.
// This takes precedence over WithDebugWriter if both are set.
// Callback panics are silently recovered to prevent crashing the SDK.
// Matches Python SDK's stderr callback behavior.
func WithStderrCallback(callback func(string)) Option {
	return func(o *Options) {
		o.StderrCallback = callback
	}
}

// OutputFormatJSONSchema creates an OutputFormat for JSON schema constraints.
func OutputFormatJSONSchema(schema map[string]any) *OutputFormat {
	return &OutputFormat{
		Type:   OutputFormatTypeJSONSchema,
		Schema: schema,
	}
}

// WithOutputFormat sets the output format for structured responses.
func WithOutputFormat(format *OutputFormat) Option {
	return func(o *Options) {
		o.OutputFormat = format
	}
}

// WithJSONSchema is a convenience function that sets a JSON schema output format.
// This is equivalent to WithOutputFormat(OutputFormatJSONSchema(schema)).
func WithJSONSchema(schema map[string]any) Option {
	return func(o *Options) {
		if schema == nil {
			o.OutputFormat = nil
			return
		}
		o.OutputFormat = OutputFormatJSONSchema(schema)
	}
}

// WithIncludePartialMessages enables streaming of partial message updates.
// When true, StreamEvent messages are emitted during response generation,
// providing real-time progress as the model generates content.
func WithIncludePartialMessages(include bool) Option {
	return func(o *Options) {
		o.IncludePartialMessages = include
	}
}

// WithPartialStreaming is a convenience function that enables partial message streaming.
// Equivalent to WithIncludePartialMessages(true).
func WithPartialStreaming() Option {
	return WithIncludePartialMessages(true)
}

// =============================================================================
// File Checkpointing Options (Issue #32)
// =============================================================================

// WithEnableFileCheckpointing enables or disables file checkpointing.
// When enabled, file changes are tracked during the session and can be
// rewound to their state at any user message using Client.RewindFiles().
// Matches Python SDK's enable_file_checkpointing option.
func WithEnableFileCheckpointing(enable bool) Option {
	return func(o *Options) {
		o.EnableFileCheckpointing = enable
	}
}

// WithFileCheckpointing enables file checkpointing.
// Equivalent to WithEnableFileCheckpointing(true).
// This is the recommended convenience function for enabling file checkpointing.
func WithFileCheckpointing() Option {
	return WithEnableFileCheckpointing(true)
}

// =============================================================================
// Permission Callback Constructors and Options (Issue #8)
// =============================================================================

// NewPermissionResultAllow creates an Allow result with proper defaults.
// Use this to permit tool execution.
//
// Example:
//
//	return claudecode.NewPermissionResultAllow(), nil
var NewPermissionResultAllow = control.NewPermissionResultAllow

// NewPermissionResultDeny creates a Deny result with proper defaults.
// Use this to deny tool execution with a reason message.
//
// Example:
//
//	return claudecode.NewPermissionResultDeny("Only Read tool is allowed"), nil
var NewPermissionResultDeny = control.NewPermissionResultDeny

// WithCanUseTool sets the permission callback for tool usage requests.
// The callback is invoked when Claude CLI requests permission to use a tool.
// It receives the tool name, input parameters, and context for decision-making.
//
// Example - Allow all Read tool calls, deny others:
//
//	client := claudecode.NewClient(
//	    claudecode.WithCanUseTool(func(
//	        ctx context.Context,
//	        toolName string,
//	        input map[string]any,
//	        permCtx claudecode.ToolPermissionContext,
//	    ) (claudecode.PermissionResult, error) {
//	        if toolName == "Read" {
//	            return claudecode.NewPermissionResultAllow(), nil
//	        }
//	        return claudecode.NewPermissionResultDeny("Only Read tool is allowed"), nil
//	    }),
//	)
//
// The callback must be thread-safe as it may be invoked concurrently.
// If no callback is set, all tool requests are denied (secure default).
// Matches Python SDK's can_use_tool callback behavior.
func WithCanUseTool(callback CanUseToolCallback) Option {
	return func(o *Options) {
		// Handle nil callback explicitly
		if callback == nil {
			o.CanUseTool = nil
			return
		}
		// Store a wrapper that converts between control types and any types
		// to bridge the type boundary between shared.Options and control package
		o.CanUseTool = func(
			ctx context.Context,
			toolName string,
			input map[string]any,
			permCtx any,
		) (any, error) {
			// Convert permCtx back to strongly-typed ToolPermissionContext.
			// If the cast fails it means a caller populated Options.CanUseTool
			// with a non-standard wrapper (bypassing WithCanUseTool). Fall back
			// to an empty context so the callback still runs, but log so the
			// misuse is at least observable in practice.
			tpc, ok := permCtx.(control.ToolPermissionContext)
			if !ok {
				log.Printf("claude-sdk: WithCanUseTool received permCtx of type %T; expected control.ToolPermissionContext - using empty context", permCtx)
				tpc = control.ToolPermissionContext{}
			}
			return callback(ctx, toolName, input, tpc)
		}
	}
}

// =============================================================================
// Hook Types (Issue #9)
// =============================================================================

// HookEvent represents lifecycle events that can trigger hooks.
type HookEvent = control.HookEvent

// Hook event constants matching Python SDK exactly.
const (
	// HookEventPreToolUse is triggered before a tool is executed.
	HookEventPreToolUse = control.HookEventPreToolUse
	// HookEventPostToolUse is triggered after a tool is executed.
	HookEventPostToolUse = control.HookEventPostToolUse
	// HookEventUserPromptSubmit is triggered when a user submits a prompt.
	HookEventUserPromptSubmit = control.HookEventUserPromptSubmit
	// HookEventStop is triggered when the session is stopping.
	HookEventStop = control.HookEventStop
	// HookEventSubagentStop is triggered when a subagent is stopping.
	HookEventSubagentStop = control.HookEventSubagentStop
	// HookEventPreCompact is triggered before context compaction.
	HookEventPreCompact = control.HookEventPreCompact
	// HookEventPostToolUseFailure is triggered after a tool execution fails.
	HookEventPostToolUseFailure = control.HookEventPostToolUseFailure
	// HookEventNotification is triggered when a notification is sent.
	HookEventNotification = control.HookEventNotification
	// HookEventSubagentStart is triggered when a subagent starts.
	HookEventSubagentStart = control.HookEventSubagentStart
	// HookEventPermissionRequest is triggered when a permission request is made.
	HookEventPermissionRequest = control.HookEventPermissionRequest
)

// HookCallback is the function signature for hook callbacks.
type HookCallback = control.HookCallback

// HookMatcher defines which hooks to trigger for a given pattern.
type HookMatcher = control.HookMatcher

// HookContext provides context information for hook callbacks.
type HookContext = control.HookContext

// HookJSONOutput is the synchronous hook output structure.
type HookJSONOutput = control.HookJSONOutput

// AsyncHookJSONOutput indicates the hook will respond asynchronously.
type AsyncHookJSONOutput = control.AsyncHookJSONOutput

// BaseHookInput and related types represent hook event inputs.
type (
	// BaseHookInput contains common fields present across all hook events.
	BaseHookInput = control.BaseHookInput
	// PreToolUseHookInput is the input for PreToolUse hook events.
	PreToolUseHookInput = control.PreToolUseHookInput
	// PostToolUseHookInput is the input for PostToolUse hook events.
	PostToolUseHookInput = control.PostToolUseHookInput
	// PostToolUseFailureHookInput is the input for PostToolUseFailure hook events.
	PostToolUseFailureHookInput = control.PostToolUseFailureHookInput
	// UserPromptSubmitHookInput is the input for UserPromptSubmit hook events.
	UserPromptSubmitHookInput = control.UserPromptSubmitHookInput
	// StopHookInput is the input for Stop hook events.
	StopHookInput = control.StopHookInput
	// SubagentStopHookInput is the input for SubagentStop hook events.
	SubagentStopHookInput = control.SubagentStopHookInput
	// PreCompactHookInput is the input for PreCompact hook events.
	PreCompactHookInput = control.PreCompactHookInput
	// NotificationHookInput is the input for Notification hook events.
	NotificationHookInput = control.NotificationHookInput
	// SubagentStartHookInput is the input for SubagentStart hook events.
	SubagentStartHookInput = control.SubagentStartHookInput
	// PermissionRequestHookInput is the input for PermissionRequest hook events.
	PermissionRequestHookInput = control.PermissionRequestHookInput
)

// PreToolUseHookSpecificOutput and related types contain hook-specific output fields.
type (
	// PreToolUseHookSpecificOutput contains PreToolUse-specific output fields.
	PreToolUseHookSpecificOutput = control.PreToolUseHookSpecificOutput
	// PostToolUseHookSpecificOutput contains PostToolUse-specific output fields.
	PostToolUseHookSpecificOutput = control.PostToolUseHookSpecificOutput
	// PostToolUseFailureHookSpecificOutput contains PostToolUseFailure-specific output fields.
	PostToolUseFailureHookSpecificOutput = control.PostToolUseFailureHookSpecificOutput
	// UserPromptSubmitHookSpecificOutput contains UserPromptSubmit-specific output fields.
	UserPromptSubmitHookSpecificOutput = control.UserPromptSubmitHookSpecificOutput
	// NotificationHookSpecificOutput contains Notification-specific output fields.
	NotificationHookSpecificOutput = control.NotificationHookSpecificOutput
	// SubagentStartHookSpecificOutput contains SubagentStart-specific output fields.
	SubagentStartHookSpecificOutput = control.SubagentStartHookSpecificOutput
	// PermissionRequestHookSpecificOutput contains PermissionRequest-specific output fields.
	PermissionRequestHookSpecificOutput = control.PermissionRequestHookSpecificOutput
)

// =============================================================================
// Hook Options (Issue #9)
// =============================================================================

// WithHooks sets the complete hook configuration for lifecycle events.
// This replaces any previously configured hooks.
//
// Example - Configure multiple hooks:
//
//	client := claudecode.NewClient(
//	    claudecode.WithHooks(map[claudecode.HookEvent][]claudecode.HookMatcher{
//	        claudecode.HookEventPreToolUse: {
//	            {Matcher: "Bash", Hooks: []claudecode.HookCallback{myCallback}},
//	        },
//	    }),
//	)
func WithHooks(hooks map[HookEvent][]HookMatcher) Option {
	return func(o *Options) {
		o.Hooks = hooks
	}
}

// WithHook adds a single hook callback for a specific event and tool pattern.
// Multiple calls accumulate hooks for the same event.
// Pass empty string for matcher to match all tools.
//
// Example - Add a PreToolUse hook for Bash commands:
//
//	client := claudecode.NewClient(
//	    claudecode.WithHook(claudecode.HookEventPreToolUse, "Bash", myCallback),
//	)
func WithHook(event HookEvent, matcher string, callback HookCallback) Option {
	return func(o *Options) {
		if o.Hooks == nil {
			o.Hooks = make(map[HookEvent][]HookMatcher)
		}
		hooks, ok := o.Hooks.(map[HookEvent][]HookMatcher)
		if !ok {
			// If not the expected type, initialize fresh
			hooks = make(map[HookEvent][]HookMatcher)
			o.Hooks = hooks
		}
		hooks[event] = append(hooks[event], HookMatcher{
			Matcher: matcher,
			Hooks:   []HookCallback{callback},
		})
	}
}

// WithPreToolUseHook is a convenience function to add a PreToolUse hook.
// Pass empty string for matcher to match all tools.
//
// Example:
//
//	client := claudecode.NewClient(
//	    claudecode.WithPreToolUseHook("Bash", func(ctx context.Context, input any, toolUseID *string, hookCtx claudecode.HookContext) (claudecode.HookJSONOutput, error) {
//	        // Log the bash command
//	        return claudecode.HookJSONOutput{}, nil
//	    }),
//	)
func WithPreToolUseHook(matcher string, callback HookCallback) Option {
	return WithHook(HookEventPreToolUse, matcher, callback)
}

// WithPostToolUseHook is a convenience function to add a PostToolUse hook.
// Pass empty string for matcher to match all tools.
func WithPostToolUseHook(matcher string, callback HookCallback) Option {
	return WithHook(HookEventPostToolUse, matcher, callback)
}
