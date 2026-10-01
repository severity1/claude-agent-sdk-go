# Feature Parity: Go SDK vs Python SDK

This document compares the public API of the Go Agent SDK with the Python Agent SDK. It shows what the Go SDK covers today and what is still missing.

---

## Executive Summary

**Status: partial parity. Work continues row by row.**

The Go SDK (`github.com/severity1/claude-agent-sdk-go`) covers the core Python SDK (`claude-agent-sdk`) surface: one-shot queries, the streaming client, hooks, permission callbacks, in-process MCP servers, programmatic agents, and session listing. The Python SDK continues to add features, and some of them are not in Go yet.

Parity is tracked one Python PR at a time:

- [docs/tracking/README.md](tracking/README.md): Python PRs merged Jan 6 to Apr 12, 2026 (Phases 1-4).
- [docs/tracking/post-snapshot.md](tracking/post-snapshot.md): Python PRs merged after Apr 12, 2026.

Each row in those files has a Go status (`done`, `partial`, `pending`, or `n/a`). This document gives the overview. The tracker rows are the source of truth.

Counts below compare Python SDK main (db750b2, Sep 30, 2026) with Go SDK main (a5535fb, Oct 1, 2026).

| Category | Python SDK | Go SDK | Notes |
|:---------|:-----------|:-------|:------|
| Client methods | 15 | 15 | 11 Python methods have a Go equivalent; 4 are pending; Go adds `QueryStream`, `GetStreamIssues`, `GetStreamStats` |
| Hook events | 10 | 10 | All 10 events ported |
| Message types (top level) | 7 | 6 (+`RawControlMessage`) | `ConversationResetMessage` pending; Python also has 6 typed `SystemMessage` subclasses that Go does not have |
| Content block types | 6 | 4 | `ServerToolUseBlock` and `ServerToolResultBlock` pending |
| Error types | 7 | 6 (+`BaseError`) | `ResultError` pending |
| Permission modes | 6 | 4 | `dontAsk` and `auto` pending |
| Option fields | 49 (`ClaudeAgentOptions`) | 38 (`Options`), 61 `With*` constructors | See the option table for the missing fields |
| Sandbox config | 3 types | 3 types | 4 `SandboxNetworkConfig` fields pending |

---

## Functions

| Python SDK | Go SDK | Status |
|:-----------|:-------|:-------|
| `query(prompt, options)` | `Query(ctx, prompt, opts...)` | PARITY |
| `tool(name, desc, schema, annotations=None)` | `NewTool(name, desc, schema, handler, opts...)` with `WithToolAnnotations(...)` | PARITY |
| `create_sdk_mcp_server(name, version, tools)` | `CreateSDKMcpServer(name, version, tools...)` | PARITY |
| `list_sessions(...)` | `ListSessions(opts...)` | PARITY |
| `get_session_messages(session_id, ...)` | `GetSessionMessages(sessionID, opts...)` | PARITY |
| `get_session_info(session_id, ...)` | `GetSessionInfo(sessionID, opts...)` | PARITY |
| `rename_session`, `tag_session` | - | PENDING (README #16, #17) |
| `delete_session`, `fork_session` | - | PENDING (README #32, post-snapshot P2) |
| `list_subagents`, `get_subagent_messages` | - | PENDING (post-snapshot P4) |
| `SessionStore` helpers (`*_from_store`, `*_via_store`, `import_session_to_store`) | - | PENDING (post-snapshot P6, P12) |

### Go SDK Additional Functions

| Function | Description |
|:---------|:------------|
| `QueryWithTransport()` | Query with custom transport (testing) |
| `NewClient()` | Create new Client |
| `NewClientWithTransport()` | Create Client with custom transport |
| `WithClient()` | Resource management helper (like `async with`) |
| `WithClientTransport()` | Resource management with custom transport |

---

## Classes / Client Interface

### Python: `ClaudeSDKClient`

| Method | Go Equivalent | Status |
|:-------|:--------------|:-------|
| `__init__(options)` | `NewClient(opts...)` | PARITY (functional options) |
| `connect(prompt)` | `Connect(ctx, prompt...)` | PARITY |
| `query(prompt, session_id)` | `Query(ctx, prompt)` / `QueryWithSession(ctx, prompt, sessionID)` | PARITY (split into two methods) |
| `receive_messages()` | `ReceiveMessages(ctx)` | PARITY (returns channel) |
| `receive_response()` | `ReceiveResponse(ctx)` | PARITY (returns MessageIterator) |
| `interrupt()` | `Interrupt(ctx)` | PARITY |
| `set_permission_mode(mode)` | `SetPermissionMode(ctx, mode)` | PARITY |
| `set_model(model)` | `SetModel(ctx, model)` | PARITY |
| `rewind_files(uuid)` | `RewindFiles(ctx, messageUUID)` | PARITY |
| `get_mcp_status()` | `GetMcpStatus(ctx)` | PARITY |
| `get_server_info()` | `GetServerInfo(ctx)` | PARITY |
| `disconnect()` | `Disconnect()` | PARITY |
| `reconnect_mcp_server(name)` | - | PENDING (README #10) |
| `toggle_mcp_server(name, enabled)` | - | PENDING (README #10) |
| `stop_task(task_id)` | - | PENDING (README #10, Go issue #143) |
| `get_context_usage()` | - | PENDING (README #39) |
| `async with` context manager | `WithClient()` helper | PARITY (Go-idiomatic resource management) |

### Go SDK Additional Methods

| Method | Description |
|:-------|:------------|
| `QueryStream(ctx, messages)` | Send messages from a channel |
| `GetStreamIssues()` | Get validation issues from stream |
| `GetStreamStats()` | Get stream statistics |

---

## Configuration Options

### ClaudeAgentOptions Mapping

| Python Option | Go Option Constructor | Status |
|:--------------|:---------------------|:-------|
| `allowed_tools` | `WithAllowedTools(tools...)` | PARITY |
| `disallowed_tools` | `WithDisallowedTools(tools...)` | PARITY |
| `tools` | `WithTools(tools...)` | PARITY |
| - | `WithToolsPreset(preset)` | GO EXTRA |
| - | `WithClaudeCodeTools()` | GO EXTRA |
| `system_prompt` (string) | `WithSystemPrompt(prompt)` | PARITY |
| `system_prompt` (preset, file, custom) | - | PENDING (README #22, #48; post-snapshot P45) |
| - | `WithAppendSystemPrompt(prompt)` | GO EXTRA |
| `model` | `WithModel(model)` | PARITY |
| `fallback_model` | `WithFallbackModel(model)` | PARITY |
| `effort` | `WithEffort(effort)` | PARITY |
| `thinking` | - | PENDING (README #7) |
| `max_turns` | `WithMaxTurns(turns)` | PARITY |
| `max_budget_usd` | `WithMaxBudgetUSD(budget)` | PARITY |
| `max_thinking_tokens` | `WithMaxThinkingTokens(tokens)` | PARITY |
| `task_budget` | - | PENDING (README #33) |
| `permission_mode` | `WithPermissionMode(mode)` | PARITY |
| `permission_prompt_tool_name` | `WithPermissionPromptToolName(toolName)` | PARITY |
| `continue_conversation` | `WithContinueConversation(bool)` | PARITY |
| `resume` | `WithResume(sessionID)` | PARITY |
| `session_id` | - | PENDING (README #37) |
| `resume_session_at`, `resume_drops_turn` | - | PENDING (post-snapshot P40) |
| `fork_session` | `WithForkSession(fork)` | PARITY |
| `cwd` | `WithCwd(cwd)` | PARITY |
| `add_dirs` | `WithAddDirs(dirs...)` | PARITY |
| `mcp_servers` | `WithMcpServers(servers)` | PARITY |
| - | `WithSdkMcpServer(name, server)` | GO EXTRA |
| `strict_mcp_config` | - | PENDING (post-snapshot P16) |
| `settings` | `WithSettings(settings)` | PARITY |
| `setting_sources` | `WithSettingSources(sources...)` | PARTIAL (nil and empty list both send an empty value; post-snapshot P5) |
| `skills` | `WithSkills(skills)`, `WithSkillsAll()`, `WithSkillsList(names...)`, `WithSkillsDisabled()` | PARTIAL (post-snapshot P1) |
| `env` | `WithEnv(env)` | PARITY |
| - | `WithEnvVar(key, value)` | GO EXTRA |
| `extra_args` | `WithExtraArgs(args)` | PARITY |
| `cli_path` | `WithCLIPath(path)` | PARITY |
| `max_buffer_size` | `WithMaxBufferSize(size)` | PARITY |
| `stderr` | `WithStderrCallback(callback)` | PARITY |
| `debug_stderr` (deprecated) | `WithDebugWriter(w)` | PARITY |
| - | `WithDebugStderr()` | GO EXTRA |
| - | `WithDebugDisabled()` | GO EXTRA |
| `can_use_tool` | `WithCanUseTool(callback)` | PARITY |
| `hooks` | `WithHooks(hooks)` | PARITY |
| - | `WithHook(event, matcher, callback)` | GO EXTRA |
| - | `WithPreToolUseHook(matcher, callback)` | GO EXTRA |
| - | `WithPostToolUseHook(matcher, callback)` | GO EXTRA |
| `include_hook_events` | - | PENDING (post-snapshot P22) |
| `forward_subagent_text` | - | PENDING (post-snapshot P42) |
| `verbatim_prompts` | - | PENDING (post-snapshot P46) |
| `user` | `WithUser(user)` | PARITY |
| `include_partial_messages` | `WithIncludePartialMessages(include)` | PARITY |
| - | `WithPartialStreaming()` | GO EXTRA |
| `enable_file_checkpointing` | `WithEnableFileCheckpointing(enable)` | PARITY |
| - | `WithFileCheckpointing()` | GO EXTRA |
| `agents` | `WithAgents(agents)` | PARITY |
| - | `WithAgent(name, agent)` | GO EXTRA |
| `plugins` | `WithPlugins(plugins)` | PARITY |
| - | `WithPlugin(plugin)` | GO EXTRA |
| - | `WithLocalPlugin(path)` | GO EXTRA |
| `sandbox` | `WithSandbox(sandbox)` | PARITY |
| - | `WithSandboxEnabled(enabled)` | GO EXTRA |
| - | `WithAutoAllowBashIfSandboxed(autoAllow)` | GO EXTRA |
| - | `WithSandboxExcludedCommands(commands...)` | GO EXTRA |
| - | `WithSandboxNetwork(network)` | GO EXTRA |
| `output_format` | `WithOutputFormat(format)` | PARITY |
| - | `WithJSONSchema(schema)` | GO EXTRA |
| `betas` | `WithBetas(betas...)` | PARITY |
| `session_store`, `session_store_flush`, `load_timeout_ms` | - | PENDING (post-snapshot P6, P14) |

---

## Message Types

| Python SDK | Go SDK | Status |
|:-----------|:-------|:-------|
| `Message` (union) | `Message` interface | PARITY |
| `UserMessage` | `UserMessage` struct | PARITY |
| `AssistantMessage` | `AssistantMessage` struct | PARITY |
| `SystemMessage` | `SystemMessage` struct | PARITY |
| `ResultMessage` | `ResultMessage` struct | PARITY (some newer fields pending, for example `stop_reason`, `api_error_status`, `terminal_reason`, `model_usage`) |
| `StreamEvent` | `StreamEvent` struct | PARITY |
| `RateLimitEvent` | `RateLimitEventMessage` struct | PARTIAL (README #15) |
| `ConversationResetMessage` | - | PENDING (post-snapshot P37) |
| `TaskStartedMessage`, `TaskProgressMessage`, `TaskNotificationMessage` | - | PENDING (README #11) |
| `TaskUpdatedMessage` | - | PENDING (post-snapshot P27) |
| `HookEventMessage` | - | PENDING (post-snapshot P22) |
| `MirrorErrorMessage` | - | PENDING (post-snapshot P6) |
| - | `RawControlMessage` struct | GO EXTRA |

### Message Type Constants

| Python | Go | Status |
|:-------|:---|:-------|
| `"user"` | `MessageTypeUser` | PARITY |
| `"assistant"` | `MessageTypeAssistant` | PARITY |
| `"system"` | `MessageTypeSystem` | PARITY |
| `"result"` | `MessageTypeResult` | PARITY |
| `"stream_event"` | `MessageTypeStreamEvent` | PARITY |
| `"rate_limit_event"` | `MessageTypeRateLimitEvent` | PARITY |
| - | `MessageTypeControlRequest` | GO EXTRA |
| - | `MessageTypeControlResponse` | GO EXTRA |

---

## Content Block Types

| Python SDK | Go SDK | Status |
|:-----------|:-------|:-------|
| `ContentBlock` (union) | `ContentBlock` interface | PARITY |
| `TextBlock` | `TextBlock` struct | PARITY |
| `ThinkingBlock` | `ThinkingBlock` struct | PARITY |
| `ToolUseBlock` | `ToolUseBlock` struct | PARITY |
| `ToolResultBlock` | `ToolResultBlock` struct | PARITY |
| `ServerToolUseBlock` | - | PENDING (post-snapshot P8) |
| `ServerToolResultBlock` | - | PENDING (post-snapshot P8) |

### Content Block Type Constants

| Python | Go | Status |
|:-------|:---|:-------|
| `"text"` | `ContentBlockTypeText` | PARITY |
| `"thinking"` | `ContentBlockTypeThinking` | PARITY |
| `"tool_use"` | `ContentBlockTypeToolUse` | PARITY |
| `"tool_result"` | `ContentBlockTypeToolResult` | PARITY |
| `"server_tool_use"`, `"advisor_tool_result"` | - | PENDING (post-snapshot P8) |

---

## Error Types

| Python SDK | Go SDK | Status |
|:-----------|:-------|:-------|
| `ClaudeSDKError` | `SDKError` interface + `BaseError` | PARITY |
| `CLIConnectionError` | `ConnectionError` | PARITY |
| `CLINotFoundError` | `CLINotFoundError` | PARITY |
| `ProcessError` | `ProcessError` | PARITY |
| `ResultError` | - | PENDING (post-snapshot P44) |
| `CLIJSONDecodeError` | `JSONDecodeError` | PARITY |
| `MessageParseError` | `MessageParseError` | PARITY |

### AssistantMessageError Types

| Python | Go | Status |
|:-------|:---|:-------|
| `"authentication_failed"` | `AssistantMessageErrorAuthFailed` | PARITY |
| `"billing_error"` | `AssistantMessageErrorBilling` | PARITY |
| `"rate_limit"` | `AssistantMessageErrorRateLimit` | PARITY |
| `"invalid_request"` | `AssistantMessageErrorInvalidRequest` | PARITY |
| `"server_error"` | `AssistantMessageErrorServer` | PARITY |
| `"unknown"` | `AssistantMessageErrorUnknown` | PARITY |

### Go-Specific Error Type Helpers

Go SDK provides idiomatic helper functions following the `os.IsNotExist` pattern. These work with wrapped errors (using `errors.As` internally).

| Function | Description | Status |
|:---------|:------------|:-------|
| `IsConnectionError(err)` | Check if error is ConnectionError | GO-NATIVE |
| `IsCLINotFoundError(err)` | Check if error is CLINotFoundError | GO-NATIVE |
| `IsProcessError(err)` | Check if error is ProcessError | GO-NATIVE |
| `IsJSONDecodeError(err)` | Check if error is JSONDecodeError | GO-NATIVE |
| `IsMessageParseError(err)` | Check if error is MessageParseError | GO-NATIVE |
| `AsConnectionError(err)` | Extract *ConnectionError or nil | GO-NATIVE |
| `AsCLINotFoundError(err)` | Extract *CLINotFoundError or nil | GO-NATIVE |
| `AsProcessError(err)` | Extract *ProcessError or nil | GO-NATIVE |
| `AsJSONDecodeError(err)` | Extract *JSONDecodeError or nil | GO-NATIVE |
| `AsMessageParseError(err)` | Extract *MessageParseError or nil | GO-NATIVE |

**Note**: Python uses `isinstance()` for error type checking. Go SDK provides these helpers as a more idiomatic alternative to manual type assertions.

---

## Hook Types

### Hook Events

| Python | Go | Status |
|:-------|:---|:-------|
| `"PreToolUse"` | `HookEventPreToolUse` | PARITY |
| `"PostToolUse"` | `HookEventPostToolUse` | PARITY |
| `"PostToolUseFailure"` | `HookEventPostToolUseFailure` | PARITY |
| `"UserPromptSubmit"` | `HookEventUserPromptSubmit` | PARITY |
| `"Stop"` | `HookEventStop` | PARITY |
| `"SubagentStop"` | `HookEventSubagentStop` | PARITY |
| `"PreCompact"` | `HookEventPreCompact` | PARITY |
| `"Notification"` | `HookEventNotification` | PARITY |
| `"SubagentStart"` | `HookEventSubagentStart` | PARITY |
| `"PermissionRequest"` | `HookEventPermissionRequest` | PARITY |

### Hook Types

| Python SDK | Go SDK | Status |
|:-----------|:-------|:-------|
| `HookEvent` | `HookEvent` type | PARITY |
| `HookCallback` | `HookCallback` type | PARITY |
| `HookContext` | `HookContext` struct | PARITY |
| `HookMatcher` | `HookMatcher` struct | PARITY |
| `HookJSONOutput` | `HookJSONOutput` struct | PARITY |
| `AsyncHookJSONOutput` | `AsyncHookJSONOutput` struct | PARITY |

### Hook Input Types

| Python | Go | Status |
|:-------|:---|:-------|
| `BaseHookInput` | `BaseHookInput` | PARITY |
| `PreToolUseHookInput` | `PreToolUseHookInput` | PARITY (`agent_id`/`agent_type` pending, README #13) |
| `PostToolUseHookInput` | `PostToolUseHookInput` | PARITY (`agent_id`/`agent_type` pending, README #13) |
| `PostToolUseFailureHookInput` | `PostToolUseFailureHookInput` | PARITY (`agent_id`/`agent_type` pending, README #13) |
| `UserPromptSubmitHookInput` | `UserPromptSubmitHookInput` | PARITY |
| `StopHookInput` | `StopHookInput` | PARITY |
| `SubagentStopHookInput` | `SubagentStopHookInput` | PARITY |
| `PreCompactHookInput` | `PreCompactHookInput` | PARITY |
| `NotificationHookInput` | `NotificationHookInput` | PARITY |
| `SubagentStartHookInput` | `SubagentStartHookInput` | PARITY |
| `PermissionRequestHookInput` | `PermissionRequestHookInput` | PARITY (`agent_id`/`agent_type` pending, README #13) |

### Hook Output Types

| Python | Go | Status |
|:-------|:---|:-------|
| `PreToolUseHookSpecificOutput` | `PreToolUseHookSpecificOutput` | PARITY |
| `PostToolUseHookSpecificOutput` | `PostToolUseHookSpecificOutput` | PARITY (`updatedToolOutput` pending, post-snapshot P18) |
| `PostToolUseFailureHookSpecificOutput` | `PostToolUseFailureHookSpecificOutput` | PARITY |
| `UserPromptSubmitHookSpecificOutput` | `UserPromptSubmitHookSpecificOutput` | PARITY |
| `NotificationHookSpecificOutput` | `NotificationHookSpecificOutput` | PARITY |
| `SubagentStartHookSpecificOutput` | `SubagentStartHookSpecificOutput` | PARITY |
| `PermissionRequestHookSpecificOutput` | `PermissionRequestHookSpecificOutput` | PARITY |
| `SessionStartHookSpecificOutput` | - | NOT PORTED (Python has no `SessionStart` hook event; no tracker row) |

---

## MCP Types

| Python SDK | Go SDK | Status |
|:-----------|:-------|:-------|
| `SdkMcpTool` | `McpTool` struct | PARITY |
| `mcp.types.ToolAnnotations` | `ToolAnnotations` struct | PARITY |
| `McpServerConfig` (union) | `McpServerConfig` interface | PARITY |
| `McpStdioServerConfig` | `McpStdioServerConfig` | PARITY |
| `McpSSEServerConfig` | `McpSSEServerConfig` | PARITY |
| `McpHttpServerConfig` | `McpHTTPServerConfig` | PARITY |
| `McpSdkServerConfig` | `McpSdkServerConfig` | PARITY |
| `McpClaudeAIProxyServerConfig` | - | NOT PORTED (Go has only the `McpServerConfigTypeClaudeAI` status constant) |
| `McpStatusResponse` | `McpStatusResponse` | PARITY |
| `McpServerStatus` | `McpServerStatus` | PARITY |
| `McpServerInfo` | `McpServerInfo` | PARITY |
| `McpToolInfo` | `McpToolInfo` | PARITY |
| `McpToolAnnotations` | `McpToolAnnotations` | PARITY |
| `McpServerConnectionStatus` | `McpServerConnectionStatus` + 5 constants | PARITY |

### Go SDK MCP Extras

| Type | Description |
|:-----|:------------|
| `SdkMcpServer` struct | In-process server implementation |
| `McpToolHandler` | Function type for tool handlers |
| `ToolOption` | Functional option for `NewTool` (for example `WithToolAnnotations`) |
| `McpToolResult` | Result from tool execution |
| `McpContent` | Content in tool result |
| `McpToolDefinition` | Tool definition for listing |

---

## Permission Types

| Python | Go | Status |
|:-------|:---|:-------|
| `CanUseTool` | `CanUseToolCallback` | PARITY |
| `ToolPermissionContext` | `ToolPermissionContext` | PARTIAL (`tool_use_id`, `agent_id`, `decision_reason` and display fields pending; README #38, post-snapshot P19) |
| `PermissionResult` | `PermissionResult` interface | PARITY |
| `PermissionResultAllow` | `PermissionResultAllow` struct | PARITY |
| `PermissionResultDeny` | `PermissionResultDeny` struct | PARITY |
| `PermissionUpdate` | `PermissionUpdate` struct | PARITY |
| `PermissionRuleValue` | `PermissionRuleValue` struct | PARITY |

### Permission Modes

| Python | Go | Status |
|:-------|:---|:-------|
| `"default"` | `PermissionModeDefault` | PARITY |
| `"acceptEdits"` | `PermissionModeAcceptEdits` | PARITY |
| `"plan"` | `PermissionModePlan` | PARITY |
| `"bypassPermissions"` | `PermissionModeBypassPermissions` | PARITY |
| `"dontAsk"` | - | PENDING (README #25) |
| `"auto"` | - | PENDING (README #46) |

---

## Sandbox Configuration

| Python SDK | Go SDK | Status |
|:-----------|:-------|:-------|
| `SandboxSettings` | `SandboxSettings` struct | PARITY |
| `SandboxNetworkConfig` | `SandboxNetworkConfig` struct | PARTIAL (see fields) |
| `SandboxIgnoreViolations` | `SandboxIgnoreViolations` struct | PARITY |

### SandboxSettings Fields

| Python | Go | Status |
|:-------|:---|:-------|
| `enabled` | `Enabled` | PARITY |
| `autoAllowBashIfSandboxed` | `AutoAllowBashIfSandboxed` | PARITY |
| `excludedCommands` | `ExcludedCommands` | PARITY |
| `allowUnsandboxedCommands` | `AllowUnsandboxedCommands` | PARITY |
| `network` | `Network` | PARITY |
| `ignoreViolations` | `IgnoreViolations` | PARITY |
| `enableWeakerNestedSandbox` | `EnableWeakerNestedSandbox` | PARITY |

### SandboxNetworkConfig Fields

| Python | Go | Status |
|:-------|:---|:-------|
| `allowLocalBinding` | `AllowLocalBinding` | PARITY |
| `allowUnixSockets` | `AllowUnixSockets` | PARITY |
| `allowAllUnixSockets` | `AllowAllUnixSockets` | PARITY |
| `httpProxyPort` | `HTTPProxyPort` | PARITY |
| `socksProxyPort` | `SOCKSProxyPort` | PARITY |
| `allowedDomains`, `deniedDomains`, `allowManagedDomainsOnly`, `allowMachLookup` | - | PENDING (post-snapshot P13) |

---

## Advanced Features

| Feature | Python SDK | Go SDK | Status |
|:--------|:-----------|:-------|:-------|
| Streaming responses | `async for message in query()` | `MessageIterator.Next(ctx)` | PARITY |
| Partial message streaming | `include_partial_messages=True` | `WithPartialStreaming()` | PARITY |
| Session options | `resume`, `fork_session` | `WithResume()`, `WithForkSession()` | PARITY |
| Session listing | `list_sessions()`, `get_session_messages()`, `get_session_info()` | `ListSessions()`, `GetSessionMessages()`, `GetSessionInfo()` | PARITY |
| Session store | `session_store` | - | PENDING (post-snapshot P6) |
| File checkpointing | `enable_file_checkpointing` | `WithFileCheckpointing()` | PARITY |
| File rewinding | `rewind_files(uuid)` | `RewindFiles(ctx, uuid)` | PARITY |
| Interrupt support | `interrupt()` | `Interrupt(ctx)` | PARITY |
| Runtime model and permission mode | `set_model()`, `set_permission_mode()` | `SetModel()`, `SetPermissionMode()` | PARITY |
| MCP server status | `get_mcp_status()` | `GetMcpStatus()` | PARITY |
| Structured output | `output_format` | `WithOutputFormat()`, `WithJSONSchema()` | PARITY |
| Custom agents | `agents` (sent on the initialize request) | `WithAgents()`, `WithAgent()` (sent on the initialize request) | PARITY |
| Skills | `skills` | `WithSkills()` and helpers | PARTIAL (post-snapshot P1) |
| Plugins | `plugins` | `WithPlugins()`, `WithLocalPlugin()` | PARITY |
| Beta features | `betas` | `WithBetas()` | PARITY |

### Go SDK Advanced Extras

| Feature | Description |
|:--------|:------------|
| `Transport` interface | Custom transport for testing |
| `StreamValidator` | Stream validation and diagnostics |
| `GetStreamIssues()` | Get validation issues |
| `GetStreamStats()` | Get stream statistics |
| `QueryStream()` | Send a channel of messages to the client |

---

## Other Types

### Agent Types

| Python | Go | Status |
|:-------|:---|:-------|
| `AgentDefinition` dataclass | `AgentDefinition` struct | PARTIAL (Go has `Description`, `Prompt`, `Tools`, `Model`; newer Python fields pending, README #19, #36, #44) |
| `model` alias `"sonnet"` | `AgentModelSonnet` | PARITY |
| `model` alias `"opus"` | `AgentModelOpus` | PARITY |
| `model` alias `"haiku"` | `AgentModelHaiku` | PARITY |
| `model` alias `"inherit"` | `AgentModelInherit` | PARITY |

### Effort Levels

| Python | Go | Status |
|:-------|:---|:-------|
| `"low"` | `EffortLow` | PARITY |
| `"medium"` | `EffortMedium` | PARITY |
| `"high"` | `EffortHigh` | PARITY |
| `"xhigh"` | `EffortXHigh` | PARITY |
| `"max"` | `EffortMax` | PARITY |

### Plugin Types

| Python | Go | Status |
|:-------|:---|:-------|
| `SdkPluginConfig` TypedDict | `SdkPluginConfig` struct | PARITY |
| `"local"` | `SdkPluginTypeLocal` | PARITY |

### Setting Sources

| Python | Go | Status |
|:-------|:---|:-------|
| `"user"` | `SettingSourceUser` | PARITY |
| `"project"` | `SettingSourceProject` | PARITY |
| `"local"` | `SettingSourceLocal` | PARITY |

### Beta Features

| Python | Go | Status |
|:-------|:---|:-------|
| `"context-1m-2025-08-07"` | `SdkBetaContext1M` | PARITY |

---

## Migration Guide for Python SDK Users

### Key Differences

1. **Context-First Pattern**: Go functions accept `context.Context` as the first parameter for cancellation and timeouts.

2. **Functional Options**: Instead of a single options object, Go uses the functional options pattern with `With*()` functions.

3. **Interfaces vs Classes**: Go uses interfaces (`Client`, `Message`, `ContentBlock`) instead of classes.

4. **Error Handling**: Go uses explicit error returns instead of exceptions.

5. **Resource Management**: Use `WithClient()` instead of `async with` for automatic resource cleanup.

### Code Comparison

**Python:**
```python
from claude_agent_sdk import query, ClaudeAgentOptions

options = ClaudeAgentOptions(
    system_prompt="You are an expert",
    allowed_tools=["Read", "Write"],
    permission_mode="acceptEdits"
)

async for message in query(prompt="Hello", options=options):
    if isinstance(message, AssistantMessage):
        print(message.content)
```

**Go:**
```go
import "github.com/severity1/claude-agent-sdk-go"

iterator, err := claudecode.Query(ctx, "Hello",
    claudecode.WithSystemPrompt("You are an expert"),
    claudecode.WithAllowedTools("Read", "Write"),
    claudecode.WithPermissionMode(claudecode.PermissionModeAcceptEdits),
)
if err != nil {
    return err
}
defer iterator.Close()

for {
    message, err := iterator.Next(ctx)
    if errors.Is(err, claudecode.ErrNoMoreMessages) {
        break
    }
    if assistant, ok := message.(*claudecode.AssistantMessage); ok {
        // Process content
    }
}
```

### Client Usage Comparison

**Python:**
```python
async with ClaudeSDKClient(options) as client:
    await client.query("Hello")
    async for msg in client.receive_response():
        print(msg)
```

**Go:**
```go
err := claudecode.WithClient(ctx, func(client claudecode.Client) error {
    if err := client.Query(ctx, "Hello"); err != nil {
        return err
    }
    for msg := range client.ReceiveMessages(ctx) {
        // Process message
    }
    return nil
}, opts...)
```

---

## Summary

The Go SDK covers the core Python SDK features and adds Go-idiomatic design:

- **Functional options pattern** for flexible configuration
- **Context-first design** for proper cancellation and timeouts
- **Interface-based design** for testability
- **Additional diagnostic features** (StreamValidator, GetStreamIssues, GetStreamStats)
- **Custom transport support** for testing

Some Python SDK features are not in Go yet. See [docs/tracking/README.md](tracking/README.md) and [docs/tracking/post-snapshot.md](tracking/post-snapshot.md) for the full list and the status of each item.
