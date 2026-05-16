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
- Hook event count: 10 as of Python SDK PR #545 (added `Notification`, `SubagentStart`, `PermissionRequest` to the 7 from PR #535); const block order in types_hook.go: PreToolUse, PostToolUse, PostToolUseFailure, UserPromptSubmit, Stop, SubagentStop, PreCompact, Notification, SubagentStart, PermissionRequest
- New hook input structs (PR #545): `NotificationHookInput` (Message, Title *string omitempty, NotificationType); `SubagentStartHookInput` (AgentID, AgentType); `PermissionRequestHookInput` (ToolName, ToolInput map[string]any, PermissionSuggestions []any omitempty) - intentionally no agent_id/agent_type until Phase2 item #13 (Python PR #628)
- New hook output structs (PR #545): `NotificationHookSpecificOutput`, `SubagentStartHookSpecificOutput` (both: HookEventName + AdditionalContext *string omitempty); `PermissionRequestHookSpecificOutput` (HookEventName + Decision map[string]any - required, no omitempty)
- Missing fields added (PR #545): `ToolUseID string` on PreToolUseHookInput and PostToolUseHookInput; `AgentID`, `AgentTranscriptPath`, `AgentType` as flat required string fields on SubagentStopHookInput (NOT via mixin - mixin is a separate construct landing in PR #628); `AdditionalContext *string` on PreToolUseHookSpecificOutput; `UpdatedMCPToolOutput any` (Go acronym casing, wire tag `updatedMCPToolOutput`) on PostToolUseHookSpecificOutput
- getAnySlice helper in hooks.go: mirrors getMap; returns nil when key absent (Python NotRequired semantics); use for []any typed fields
- PostToolUseFailureHookInput fields: `ToolUseID string`, `Error string`, `IsInterrupt *bool json:"is_interrupt,omitempty"`; nil `IsInterrupt` maps to key absent in JSON (Python `NotRequired[bool]`); `PostToolUseFailureHookSpecificOutput` is structurally identical to `PostToolUseHookSpecificOutput` (only `HookEventName` literal differs), both have `AdditionalContext *string` (omitempty); `_SubagentContextMixin` fields (`agent_id`/`agent_type`) still deferred to Phase2 item #13 (Python PR #628)
- `routeMcpMethod` in mcp.go dispatches JSONRPC methods; the `tools/list` branch delegates to `buildToolsListResult` (standalone helper, not inlined) to keep dispatch under the gocyclo budget - no `//nolint:gocyclo` needed on `routeMcpMethod`; `tools/list` wire behavior: `nil` Annotations omits the `"annotations"` key entirely; `&ToolAnnotations{}` (non-nil, all pointer fields unset) emits `"annotations": {}` (key present, empty map); camelCase wire keys verified by `TestMcpToolsListResponseAnnotationsAllFields`: `title`, `readOnlyHint`, `destructiveHint`, `idempotentHint`, `openWorldHint`; test assertions on `routeMcpMethod` results must use ok-pattern guards (`v, ok := result["result"].(map[string]any); if !ok { t.Fatal(...) }`) rather than chained type assertions to satisfy staticcheck SA5011

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
