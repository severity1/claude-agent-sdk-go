# Module: control

<!-- AUTO-MANAGED: module-description -->
## Purpose

Bidirectional control protocol for Claude CLI communication. Manages request/response correlation, permission callbacks, lifecycle hooks, and SDK MCP server integration.

<!-- END AUTO-MANAGED -->

<!-- AUTO-MANAGED: architecture -->
## Module Architecture

```
control/
├── protocol.go            # Protocol struct, Initialize, SendControlRequest, message routing
├── hooks.go               # Hook callback handling, input parsing, registration
├── mcp.go                 # MCP JSONRPC message routing, method dispatch
├── permissions.go         # Permission callback handling, response building
├── types.go               # Request/Response types, Initialize handshake
├── types_hook.go          # Hook event types, HookMatcher, HookCallback
├── protocol_test.go       # Protocol unit tests
├── protocol_bench_test.go # Performance benchmarks
├── hooks_test.go          # Hook system tests
├── mcp_test.go            # MCP server tests
└── types_hook_test.go     # Hook type tests
```

**Protocol Flow**:
1. `Initialize()`: Handshake with CLI, negotiate capabilities
2. `SendControlRequest()`: Send JSON-RPC style requests with correlation IDs
3. `HandleIncomingMessage()`: Route responses to pending requests
4. Hook/Permission callbacks: Invoked on tool use events (hooks.go, permissions.go)
5. MCP messages: Route to SDK MCP servers (mcp.go)

<!-- END AUTO-MANAGED -->

<!-- AUTO-MANAGED: conventions -->
## Module-Specific Conventions

- Request correlation: Use unique request IDs for response matching
- Thread safety: All state access protected by mutex
- Timeout handling: Default 60s init timeout, configurable via `WithInitTimeout`
- Hook registration: `RegisterHook()` returns callback ID for later removal
- Init error channel: `initErrChan chan error` (buffered, size 1) in Protocol struct; `HandleControlInitErr()` sends non-blocking to unblock `SendControlRequest()` when CLI fails before handshake (e.g., invalid session ID)
- Constructor pattern: `NewGetMcpStatusRequest()` sets `Subtype: SubtypeGetMcpStatus`; follows same pattern as `NewPermissionResultAllow/Deny`; use constructors for request types with fixed subtype values
- SubtypeGetMcpStatus = `"mcp_status"` (wire value from Python SDK query.py); included in parity table in `testSubtypeConstants`
- McpServerConfigType constants: `McpServerConfigTypeStdio/SSE/HTTP/SDK/ClaudeAI` discriminate `McpServerStatusConfig.Type`
- McpServerStatus conditional fields: `ServerInfo` non-nil only when connected; `Error` non-nil only when failed; `Tools` populated only when connected
- Hook event count: 7 as of Python SDK PR #535 (`HookEventPostToolUseFailure = "PostToolUseFailure"` added); Phase1 item #4 (PR #545) adds 3 more events (`Notification`, `SubagentStart`, `PermissionRequest`) and missing fields: `tool_use_id` on PreToolUse/PostToolUse inputs; `agent_id`/`agent_transcript_path`/`agent_type` as flat required fields on SubagentStopHookInput (not via mixin); `additionalContext` on PreToolUseHookSpecificOutput; `updatedMCPToolOutput` on PostToolUseHookSpecificOutput
- PostToolUseFailureHookInput fields: `ToolUseID string`, `Error string`, `IsInterrupt *bool json:"is_interrupt,omitempty"`; nil `IsInterrupt` maps to key absent in JSON (Python `NotRequired[bool]`); `PostToolUseFailureHookSpecificOutput` is structurally identical to `PostToolUseHookSpecificOutput` (only `HookEventName` literal differs), both have `AdditionalContext *string` (omitempty); `_SubagentContextMixin` fields (`agent_id`/`agent_type`) deferred to Phase2 item #13 (Python PR #628), where the mixin is introduced and applied to PostToolUseFailureHookInput; HookEvent const block order in types_hook.go matches Python SDK: PreToolUse, PostToolUse, PostToolUseFailure, UserPromptSubmit, Stop, SubagentStop, PreCompact

<!-- END AUTO-MANAGED -->

<!-- AUTO-MANAGED: dependencies -->
## Key Dependencies

- `control.Transport`: Interface for stdin/stdout communication (implemented by subprocess)
- `crypto/rand`: Generate unique request IDs
- `sync`: Mutex for thread-safe state management

<!-- END AUTO-MANAGED -->

<!-- MANUAL -->
## Notes

<!-- END MANUAL -->
