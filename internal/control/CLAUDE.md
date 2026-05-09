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
- getMap helper: returns `make(map[string]any)` (empty map, never nil) when key absent or wrong type; matches Python SDK behavior where hook inputs always receive a dict for fields like tool_input; prevents nil-map assignment panics in callbacks that mutate the returned map
- getSlice helper: returns `nil` (not an empty slice) when key absent or wrong type; asymmetric with getMap on purpose - nil slices are range-safe and `len()`-safe in Go so no panic risk, and callers that check `len(...) > 0` behave correctly; used for `PermissionRequestHookInput.PermissionSuggestions`
- Agents-in-initialize: `WithOptions()` ProtocolOption stores `shared.Options` on Protocol; `Initialize()` reads `options.Agents` and builds `InitializeRequest.Agents` map (description/prompt/tools/model per agent)
- GetMcpStatus: sends `GetMcpStatusRequest{SubtypeGetMcpStatus}` with 5s timeout; response re-marshaled from `map[string]any` to typed `*McpStatusResponse{McpServers []McpServerStatus}`
- McpToolInfo vs McpToolDefinition: `McpToolInfo` (types.go) used only in status responses; `shared.McpToolDefinition` used for SDK MCP server tool definitions - not interchangeable
- RewindFiles: `RewindFilesRequest{SubtypeRewindFiles, UserMessageID}` rewinds file state to a specific user message; obtain UserMessageID from `UserMessage.UUID` received during session
- Permission secure-deny default: `handleCanUseToolRequest` returns `NewPermissionResultDeny(...)` when no `CanUseToolCallback` is registered; never permits by default
- parsePermissionSuggestions forward-compat: silently skips unrecognized items in `permission_suggestions` array to tolerate new CLI fields without breaking older SDK versions
- ToolPermissionContext ToolUseID/AgentID: `handleCanUseToolRequest` parses `tool_use_id` and `agent_id` from the request map (Python PR #754); stored as `*string` on `ToolPermissionContext`; `ToolUseID` lets callbacks disambiguate concurrent tool calls; `AgentID` identifies the originating subagent; both nil when the CLI does not send them (older CLI versions)
- PermissionResultAllow/Deny MarshalJSON: hard-codes `"behavior"` discriminator on wire regardless of struct field value - caller cannot accidentally invert the discriminator
- UpdatedMCPToolOutput foot-gun: `PostToolUseHookSpecificOutput.UpdatedMCPToolOutput` is typed `any` with `omitempty`; `omitempty` only elides nil for `any` - zero-value scalars (`""`, `0`, `false`) are still serialized; leave field nil to omit entirely
- sendHookResponse struct-tag serialization: passes `HookJSONOutput` directly to `json.Marshal`; relies on struct tags and `omitempty` so new fields on the struct automatically flow to the wire without requiring manual map assembly updates
- buildHooksConfig deferred unlock: uses `defer p.hookCallbacksMu.Unlock()` immediately after acquiring the lock so any future early return cannot leak the mutex
- HookCallback godoc: types_hook.go documents a complete event-to-input-type mapping table; HookSpecificOutput type in the returned `HookJSONOutput` must match the event type

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
