package shared

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
)

const (
	// DefaultMaxThinkingTokens is the default maximum number of thinking tokens.
	DefaultMaxThinkingTokens = 8000
)

// PermissionMode represents the different permission handling modes.
type PermissionMode string

const (
	// PermissionModeDefault is the standard permission handling mode.
	PermissionModeDefault PermissionMode = "default"
	// PermissionModeAcceptEdits automatically accepts all edit permissions.
	PermissionModeAcceptEdits PermissionMode = "acceptEdits"
	// PermissionModePlan enables plan mode for task execution.
	PermissionModePlan PermissionMode = "plan"
	// PermissionModeBypassPermissions bypasses all permission checks.
	PermissionModeBypassPermissions PermissionMode = "bypassPermissions"
)

// SdkBeta represents a beta feature identifier.
// See https://docs.anthropic.com/en/api/beta-headers
type SdkBeta string

const (
	// SdkBetaContext1M enables the 1M context window beta feature.
	SdkBetaContext1M SdkBeta = "context-1m-2025-08-07"
)

// ToolsPreset represents a preset tools configuration.
type ToolsPreset struct {
	Type   string `json:"type"`   // Always "preset"
	Preset string `json:"preset"` // e.g., "claude_code"
}

// SettingSource represents a settings source location.
type SettingSource string

const (
	// SettingSourceUser loads user-level settings.
	SettingSourceUser SettingSource = "user"
	// SettingSourceProject loads project-level settings.
	SettingSourceProject SettingSource = "project"
	// SettingSourceLocal loads local/workspace-level settings.
	SettingSourceLocal SettingSource = "local"
)

// SandboxNetworkConfig configures network access within sandbox.
type SandboxNetworkConfig struct {
	// AllowUnixSockets specifies Unix socket paths accessible in sandbox.
	AllowUnixSockets []string `json:"allowUnixSockets,omitempty"`
	// AllowAllUnixSockets allows all Unix sockets (less secure).
	AllowAllUnixSockets bool `json:"allowAllUnixSockets,omitempty"`
	// AllowLocalBinding allows binding to localhost ports (macOS only).
	AllowLocalBinding bool `json:"allowLocalBinding,omitempty"`
	// HTTPProxyPort is the HTTP proxy port if using custom proxy.
	HTTPProxyPort *int `json:"httpProxyPort,omitempty"`
	// SOCKSProxyPort is the SOCKS5 proxy port if using custom proxy.
	SOCKSProxyPort *int `json:"socksProxyPort,omitempty"`
}

// SandboxIgnoreViolations specifies patterns to ignore during sandbox violations.
type SandboxIgnoreViolations struct {
	// File paths for which violations should be ignored.
	File []string `json:"file,omitempty"`
	// Network hosts for which violations should be ignored.
	Network []string `json:"network,omitempty"`
}

// SandboxSettings configures sandbox behavior for bash command execution.
type SandboxSettings struct {
	// Enabled enables bash sandboxing (macOS/Linux only).
	Enabled bool `json:"enabled,omitempty"`
	// AutoAllowBashIfSandboxed auto-approves bash when sandboxed.
	AutoAllowBashIfSandboxed bool `json:"autoAllowBashIfSandboxed,omitempty"`
	// ExcludedCommands are commands that always bypass sandbox automatically.
	ExcludedCommands []string `json:"excludedCommands,omitempty"`
	// AllowUnsandboxedCommands allows commands to bypass sandbox.
	AllowUnsandboxedCommands bool `json:"allowUnsandboxedCommands,omitempty"`
	// Network configures network access in sandbox.
	Network *SandboxNetworkConfig `json:"network,omitempty"`
	// IgnoreViolations configures which violations to ignore.
	IgnoreViolations *SandboxIgnoreViolations `json:"ignoreViolations,omitempty"`
	// EnableWeakerNestedSandbox for unprivileged Docker (Linux only).
	EnableWeakerNestedSandbox bool `json:"enableWeakerNestedSandbox,omitempty"`
}

// SdkPluginType represents the type of SDK plugin.
type SdkPluginType string

const (
	// SdkPluginTypeLocal represents a local plugin loaded from the filesystem.
	SdkPluginTypeLocal SdkPluginType = "local"
)

// SdkPluginConfig represents a plugin configuration.
type SdkPluginConfig struct {
	// Type is the plugin type (currently only "local" is supported).
	Type SdkPluginType `json:"type"`
	// Path is the filesystem path to the plugin directory.
	Path string `json:"path"`
}

// OutputFormatTypeJSONSchema is the only currently-supported value for
// OutputFormat.Type. Matches the Messages API structured-output wire contract.
const OutputFormatTypeJSONSchema = "json_schema"

// OutputFormat specifies the format for structured output.
// Matches the Messages API structure: {"type": "json_schema", "schema": {...}}
type OutputFormat struct {
	Type   string         `json:"type"`   // Always OutputFormatTypeJSONSchema
	Schema map[string]any `json:"schema"` // JSON Schema definition
}

// AgentModel represents the model to use for an agent.
type AgentModel string

const (
	// AgentModelSonnet specifies Claude Sonnet model for the agent.
	AgentModelSonnet AgentModel = "sonnet"
	// AgentModelOpus specifies Claude Opus model for the agent.
	AgentModelOpus AgentModel = "opus"
	// AgentModelHaiku specifies Claude Haiku model for the agent.
	AgentModelHaiku AgentModel = "haiku"
	// AgentModelInherit specifies the agent should inherit the parent's model.
	AgentModelInherit AgentModel = "inherit"
)

// AgentDefinition defines a programmatic subagent.
type AgentDefinition struct {
	// Description is a brief description of the agent's purpose.
	Description string `json:"description"`

	// Prompt is the agent's system prompt.
	Prompt string `json:"prompt"`

	// Tools is an optional list of tools available to the agent.
	Tools []string `json:"tools,omitempty"`

	// Model specifies which model the agent should use.
	Model AgentModel `json:"model,omitempty"`
}

// ThinkingConfig configures the model's extended thinking behavior.
// Use one of: ThinkingConfigAdaptive, ThinkingConfigEnabled, ThinkingConfigDisabled.
// Go idiom: unexported marker method seals the interface (prevents external implementations).
type ThinkingConfig interface {
	thinkingConfig() // unexported - seals the union
}

// Thinking config wire discriminator values. Match Python SDK
// ThinkingConfig{Adaptive,Enabled,Disabled}.type literals.
const (
	thinkingConfigTypeAdaptive = "adaptive"
	thinkingConfigTypeEnabled  = "enabled"
	thinkingConfigTypeDisabled = "disabled"
)

// ThinkingConfigAdaptive lets the model decide its thinking budget adaptively.
type ThinkingConfigAdaptive struct{}

func (ThinkingConfigAdaptive) thinkingConfig() {}

// MarshalJSON emits the Python-SDK-compatible discriminator so this variant
// roundtrips correctly if ever serialized (e.g., in future control-protocol
// payloads that carry thinking config on the wire).
func (ThinkingConfigAdaptive) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type string `json:"type"`
	}{Type: thinkingConfigTypeAdaptive})
}

// ThinkingConfigEnabled enables thinking with an explicit token budget.
type ThinkingConfigEnabled struct {
	// BudgetTokens is the maximum number of thinking tokens.
	BudgetTokens int `json:"budget_tokens"`
}

func (ThinkingConfigEnabled) thinkingConfig() {}

// MarshalJSON emits the Python-SDK-compatible discriminator alongside
// budget_tokens so this variant roundtrips correctly on the wire.
func (t ThinkingConfigEnabled) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type         string `json:"type"`
		BudgetTokens int    `json:"budget_tokens"`
	}{Type: thinkingConfigTypeEnabled, BudgetTokens: t.BudgetTokens})
}

// ThinkingConfigDisabled disables extended thinking explicitly.
type ThinkingConfigDisabled struct{}

func (ThinkingConfigDisabled) thinkingConfig() {}

// MarshalJSON emits the Python-SDK-compatible discriminator so this variant
// roundtrips correctly if ever serialized.
func (ThinkingConfigDisabled) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type string `json:"type"`
	}{Type: thinkingConfigTypeDisabled})
}

// Options configures the Claude Agent SDK behavior.
type Options struct {
	// Tool Control
	AllowedTools    []string `json:"allowed_tools,omitempty"`
	DisallowedTools []string `json:"disallowed_tools,omitempty"`

	// Tools configures available tools.
	// Can be []string (list of tool names) or ToolsPreset (preset configuration).
	Tools any `json:"tools,omitempty"`

	// Beta Features
	Betas []SdkBeta `json:"betas,omitempty"`

	// System Prompts & Model
	SystemPrompt       *string `json:"system_prompt,omitempty"`
	AppendSystemPrompt *string `json:"append_system_prompt,omitempty"`
	Model              *string `json:"model,omitempty"`
	FallbackModel      *string `json:"fallback_model,omitempty"`
	// MaxThinkingTokens is the legacy thinking-budget knob.
	//
	// Deprecated: Use Thinking (ThinkingConfig) instead. When Thinking is set
	// it takes precedence over MaxThinkingTokens.
	MaxThinkingTokens int `json:"max_thinking_tokens,omitempty"`

	// Thinking configures the model's extended thinking behavior.
	// When set, takes precedence over MaxThinkingTokens.
	// Use ThinkingConfigAdaptive, ThinkingConfigEnabled, or ThinkingConfigDisabled.
	Thinking ThinkingConfig `json:"-"` // Converted to CLI flags, not JSON-serialized

	// Effort sets the model's reasoning effort level.
	// Valid values: "low", "medium", "high", "max".
	Effort *string `json:"effort,omitempty"`

	// Budget & Billing
	MaxBudgetUSD *float64 `json:"max_budget_usd,omitempty"`
	User         *string  `json:"user,omitempty"`

	// Buffer Configuration (internal)
	MaxBufferSize *int `json:"max_buffer_size,omitempty"`

	// Permission & Safety System
	PermissionMode           *PermissionMode `json:"permission_mode,omitempty"`
	PermissionPromptToolName *string         `json:"permission_prompt_tool_name,omitempty"`

	// Session & State Management
	ContinueConversation bool            `json:"continue_conversation,omitempty"`
	Resume               *string         `json:"resume,omitempty"`
	MaxTurns             int             `json:"max_turns,omitempty"`
	Settings             *string         `json:"settings,omitempty"`
	ForkSession          bool            `json:"fork_session,omitempty"`
	SettingSources       []SettingSource `json:"setting_sources,omitempty"`

	// Partial Message Streaming
	IncludePartialMessages bool `json:"include_partial_messages,omitempty"`

	// File Checkpointing (Issue #32)
	// EnableFileCheckpointing enables file change tracking for rewind support.
	// When enabled, files can be rewound to their state at any user message
	// using Client.RewindFiles(). Matches Python SDK's enable_file_checkpointing.
	EnableFileCheckpointing bool `json:"enable_file_checkpointing,omitempty"`

	// Agent Definitions
	Agents map[string]AgentDefinition `json:"agents,omitempty"`

	// File System & Context
	Cwd     *string  `json:"cwd,omitempty"`
	AddDirs []string `json:"add_dirs,omitempty"`

	// MCP Integration
	McpServers map[string]McpServerConfig `json:"mcp_servers,omitempty"`

	// Sandbox Configuration
	Sandbox *SandboxSettings `json:"sandbox,omitempty"`

	// Plugin Configurations
	Plugins []SdkPluginConfig `json:"plugins,omitempty"`

	// Extensibility
	ExtraArgs map[string]*string `json:"extra_args,omitempty"`

	// ExtraEnv specifies additional environment variables for the subprocess.
	// These are merged with the system environment variables.
	ExtraEnv map[string]string `json:"extra_env,omitempty"`

	// OutputFormat specifies structured output format with JSON schema.
	// When set, Claude's response will conform to the provided schema.
	OutputFormat *OutputFormat `json:"output_format,omitempty"`

	// CLI Path (for testing and custom installations)
	CLIPath *string `json:"cli_path,omitempty"`

	// DebugWriter specifies where to write debug output from the CLI subprocess.
	// If nil (default), stderr is isolated to a temporary file to prevent deadlocks.
	// Common values: os.Stderr, io.Discard, or a custom io.Writer.
	DebugWriter io.Writer `json:"-"` // Not serialized

	// StderrCallback receives CLI stderr output line-by-line.
	// If set, takes precedence over DebugWriter for stderr handling.
	// Each line is stripped of trailing whitespace and empty lines are skipped.
	// Callback panics are silently recovered to prevent crashing the SDK.
	// Matches Python SDK's stderr callback behavior.
	StderrCallback func(string) `json:"-"` // Not serialized

	// CanUseTool is invoked when CLI requests permission to use a tool.
	// The callback receives the tool name, input parameters, and permission context.
	// Return PermissionResultAllow to permit, PermissionResultDeny to deny.
	// If nil, all tool requests are denied (secure default).
	// Callback panics are recovered to prevent crashing the SDK.
	// Matches Python SDK's can_use_tool callback behavior.
	//
	// WARNING: This field is typed as any to avoid an import cycle between
	// shared and internal/control. Do not set it directly - use the
	// claudecode package's WithCanUseTool option, which wraps the callback
	// with the required any<->control.ToolPermissionContext conversion. A
	// direct assignment that returns a value not assertable to
	// control.PermissionResult is treated as a bug and surfaced loudly
	// rather than silently denying.
	CanUseTool func(
		ctx context.Context,
		toolName string,
		input map[string]any,
		permCtx any, // Actually control.ToolPermissionContext
	) (any, error) `json:"-"` // Not serialized

	// Hooks contains lifecycle event hook registrations.
	// The actual type is map[control.HookEvent][]control.HookMatcher.
	// Stored as any to avoid import cycles with internal/control package.
	//
	// WARNING: Do not set this field directly - use the claudecode package's
	// WithHook / WithHooks options. A direct assignment whose underlying
	// type does not match map[control.HookEvent][]control.HookMatcher is
	// silently ignored at transport wire-up time, which will make hook
	// callbacks appear to be registered but never fire.
	Hooks any `json:"-"` // Not serialized
}

// McpServerType represents the type of MCP server.
type McpServerType string

const (
	// McpServerTypeStdio represents a stdio-based MCP server.
	McpServerTypeStdio McpServerType = "stdio"
	// McpServerTypeSSE represents a Server-Sent Events MCP server.
	McpServerTypeSSE McpServerType = "sse"
	// McpServerTypeHTTP represents an HTTP-based MCP server.
	McpServerTypeHTTP McpServerType = "http"
)

// McpServerConfig represents MCP server configuration.
type McpServerConfig interface {
	GetType() McpServerType
}

// McpStdioServerConfig configures an MCP stdio server.
type McpStdioServerConfig struct {
	Type    McpServerType     `json:"type"`
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	// AlwaysLoad, when true, opts the server out of tool-search deferral so
	// all of its tools are always available without a ToolSearch round-trip.
	// Requires Claude Code CLI 2.1.121 or later.
	AlwaysLoad bool `json:"alwaysLoad,omitempty"`
}

// GetType returns the server type for McpStdioServerConfig.
func (c *McpStdioServerConfig) GetType() McpServerType {
	return McpServerTypeStdio
}

// McpSSEServerConfig configures an MCP Server-Sent Events server.
type McpSSEServerConfig struct {
	Type    McpServerType     `json:"type"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
	// AlwaysLoad, when true, opts the server out of tool-search deferral so
	// all of its tools are always available without a ToolSearch round-trip.
	// Requires Claude Code CLI 2.1.121 or later.
	AlwaysLoad bool `json:"alwaysLoad,omitempty"`
}

// GetType returns the server type for McpSSEServerConfig.
func (c *McpSSEServerConfig) GetType() McpServerType {
	return McpServerTypeSSE
}

// McpHTTPServerConfig configures an MCP HTTP server.
type McpHTTPServerConfig struct {
	Type    McpServerType     `json:"type"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
	// AlwaysLoad, when true, opts the server out of tool-search deferral so
	// all of its tools are always available without a ToolSearch round-trip.
	// Requires Claude Code CLI 2.1.121 or later.
	AlwaysLoad bool `json:"alwaysLoad,omitempty"`
}

// GetType returns the server type for McpHTTPServerConfig.
func (c *McpHTTPServerConfig) GetType() McpServerType {
	return McpServerTypeHTTP
}

// McpServerTypeSdk represents an in-process SDK MCP server.
const McpServerTypeSdk McpServerType = "sdk"

// McpServer is the interface for in-process SDK MCP servers.
// Implementations must be thread-safe as methods may be called concurrently.
type McpServer interface {
	// Name returns the server name.
	Name() string
	// Version returns the server version.
	Version() string
	// ListTools returns the available tools.
	ListTools(ctx context.Context) ([]McpToolDefinition, error)
	// CallTool executes a tool by name with the given arguments.
	CallTool(ctx context.Context, name string, args map[string]any) (*McpToolResult, error)
}

// McpSdkServerConfig configures an in-process SDK MCP server.
// The Instance field contains the actual server implementation and is
// excluded from JSON serialization (not sent to CLI).
type McpSdkServerConfig struct {
	Type     McpServerType `json:"type"`
	Name     string        `json:"name"`
	Instance McpServer     `json:"-"` // Excluded from CLI serialization
	// AlwaysLoad, when true, opts the server out of tool-search deferral so
	// all of its tools are always available without a ToolSearch round-trip.
	// Requires Claude Code CLI 2.1.121 or later.
	AlwaysLoad bool `json:"alwaysLoad,omitempty"`
}

// GetType returns the server type for McpSdkServerConfig.
func (c *McpSdkServerConfig) GetType() McpServerType {
	return McpServerTypeSdk
}

// McpToolAnnotations describes tool behavior hints for MCP tools.
// Used in McpToolDefinition for SDK MCP server tool definitions.
type McpToolAnnotations struct {
	ReadOnly    *bool `json:"readOnly,omitempty"`
	Destructive *bool `json:"destructive,omitempty"`
	OpenWorld   *bool `json:"openWorld,omitempty"`
}

// McpToolDefinition describes a tool exposed by an MCP server.
type McpToolDefinition struct {
	Name        string              `json:"name"`
	Description string              `json:"description"`
	InputSchema map[string]any      `json:"inputSchema"`
	Annotations *McpToolAnnotations `json:"annotations,omitempty"`
}

// McpToolResult represents the result of a tool call.
// Matches Python SDK's tool result structure for 100% parity.
type McpToolResult struct {
	Content []McpContent `json:"content"`
	IsError bool         `json:"isError,omitempty"`
}

// McpContent represents content returned by a tool.
// Supports both text and image content types.
type McpContent struct {
	Type     string `json:"type"` // "text" or "image"
	Text     string `json:"text,omitempty"`
	Data     string `json:"data,omitempty"`     // base64 for images
	MimeType string `json:"mimeType,omitempty"` // for images
}

// Validate checks the options for valid values and constraints.
func (o *Options) Validate() error {
	// Validate MaxThinkingTokens
	if o.MaxThinkingTokens < 0 {
		return fmt.Errorf("MaxThinkingTokens must be non-negative, got %d", o.MaxThinkingTokens)
	}

	// Validate ThinkingConfigEnabled.BudgetTokens when present.
	if enabled, ok := o.Thinking.(ThinkingConfigEnabled); ok && enabled.BudgetTokens < 0 {
		return fmt.Errorf("ThinkingConfigEnabled.BudgetTokens must be non-negative, got %d", enabled.BudgetTokens)
	}

	// Validate MaxTurns
	if o.MaxTurns < 0 {
		return fmt.Errorf("MaxTurns must be non-negative, got %d", o.MaxTurns)
	}

	// Validate tool conflicts (same tool in both allowed and disallowed)
	allowedSet := make(map[string]bool)
	for _, tool := range o.AllowedTools {
		allowedSet[tool] = true
	}

	for _, tool := range o.DisallowedTools {
		if allowedSet[tool] {
			return fmt.Errorf("tool '%s' cannot be in both AllowedTools and DisallowedTools", tool)
		}
	}

	// Validate OutputFormat.Type when set. The only currently-supported wire
	// value is "json_schema"; empty is permitted for callers that leave the
	// zero value (OutputFormat unset).
	if o.OutputFormat != nil && o.OutputFormat.Type != "" && o.OutputFormat.Type != OutputFormatTypeJSONSchema {
		return fmt.Errorf("OutputFormat.Type must be %q, got %q", OutputFormatTypeJSONSchema, o.OutputFormat.Type)
	}

	// AgentDefinition.Model is intentionally not validated beyond string type:
	// Python's AgentDefinition documents Model as "alias or full model ID" and
	// performs no validation, so rejecting strings the Python SDK accepts
	// would break parity and prevent callers from pinning specific versions
	// (e.g. "claude-opus-4-7"). The CLI is the source of truth for which
	// model IDs resolve; let it surface unknown values.

	return nil
}

// NewOptions creates Options with default values.
func NewOptions() *Options {
	return &Options{
		AllowedTools:      []string{},
		DisallowedTools:   []string{},
		Betas:             []SdkBeta{},
		MaxThinkingTokens: DefaultMaxThinkingTokens,
		AddDirs:           []string{},
		McpServers:        make(map[string]McpServerConfig),
		Plugins:           []SdkPluginConfig{},
		ExtraArgs:         make(map[string]*string),
		ExtraEnv:          make(map[string]string),
		SettingSources:    []SettingSource{},
	}
}
