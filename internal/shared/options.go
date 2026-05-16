package shared

import (
	"context"
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

// SkillsAll is the sentinel string value for Options.Skills that enables every
// discovered Skill. Mirrors the Python SDK's skills="all" value.
const SkillsAll = "all"

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

// OutputFormat specifies the format for structured output.
// Wire format: {"type": "json_schema", "schema": {...}}
type OutputFormat struct {
	Type   string         `json:"type"`   // Always "json_schema"
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
	MaxThinkingTokens  int     `json:"max_thinking_tokens,omitempty"`

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

	// Skills controls which filesystem-discovered Skills are exposed to the model.
	// Accepts the string "all" (SkillsAll) to enable every discovered Skill, a
	// []string of Skill names to enable only those, or an empty []string{} to
	// disable all. When non-nil and SettingSources is unset, SettingSources
	// defaults to [user, project] so the CLI discovers installed Skills.
	// Matches the Python SDK's skills option (see _apply_skills_defaults).
	Skills any `json:"skills,omitempty"`

	// Partial Message Streaming
	IncludePartialMessages bool `json:"include_partial_messages,omitempty"`

	// EnableFileCheckpointing enables file change tracking for rewind support.
	// When enabled, files can be rewound to their state at any user message
	// using Client.RewindFiles().
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
	StderrCallback func(string) `json:"-"` // Not serialized

	// CanUseTool is invoked when CLI requests permission to use a tool.
	// The callback receives the tool name, input parameters, and permission context.
	// Return PermissionResultAllow to permit, PermissionResultDeny to deny.
	// If nil, all tool requests are denied (secure default).
	// Callback panics are recovered to prevent crashing the SDK.
	// Note: The actual types are defined in internal/control to avoid import cycles.
	// Use the claudecode package's WithCanUseTool option for type-safe configuration.
	CanUseTool func(
		ctx context.Context,
		toolName string,
		input map[string]any,
		permCtx any, // Actually control.ToolPermissionContext
	) (any, error) `json:"-"` // Not serialized

	// Hooks contains lifecycle event hook registrations.
	// The actual type is map[control.HookEvent][]control.HookMatcher.
	// Stored as any to avoid import cycles with internal/control package.
	// Use the claudecode package's WithHook option for type-safe configuration.
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

// ToolAnnotations carries optional MCP-spec behavioral hints attached by
// a tool author when defining an SDK MCP tool. Sent to the CLI as part of
// the JSONRPC tools/list response under the "annotations" key.
//
// All fields are pointers so unset fields are omitted from the wire format.
// See MCP spec:
// https://modelcontextprotocol.io/specification/2025-03-26/server/tools#tool
//
// This is the authoring counterpart to McpToolAnnotations in the control
// package, which describes annotations as reported back by the CLI in
// GetMcpStatus responses. The two are kept separate because the CLI strips
// the "Hint" suffix on status responses, so the wire field sets differ.
type ToolAnnotations struct {
	Title           *string `json:"title,omitempty"`
	ReadOnlyHint    *bool   `json:"readOnlyHint,omitempty"`
	DestructiveHint *bool   `json:"destructiveHint,omitempty"`
	IdempotentHint  *bool   `json:"idempotentHint,omitempty"`
	OpenWorldHint   *bool   `json:"openWorldHint,omitempty"`
}

// McpToolDefinition describes a tool exposed by an MCP server.
type McpToolDefinition struct {
	Name        string           `json:"name"`
	Description string           `json:"description"`
	InputSchema map[string]any   `json:"inputSchema"`
	Annotations *ToolAnnotations `json:"annotations,omitempty"`
}

// McpToolResult represents the result of a tool call.
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
