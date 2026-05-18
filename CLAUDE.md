# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

<!-- AUTO-MANAGED: project-description -->
## Overview

**Claude Agent SDK for Go** - Unofficial Go SDK for Claude Code CLI integration. Provides programmatic interaction through `Query()` (one-shot) and `Client` (streaming) APIs with 100% Python SDK parity.

- **Module**: `github.com/severity1/claude-agent-sdk-go`
- **Package**: `claudecode`
- **Go Version**: 1.18+

<!-- END AUTO-MANAGED -->

<!-- AUTO-MANAGED: build-commands -->
## Build & Development Commands

```bash
# Build and test
go build ./...                    # Build all packages
go test ./...                     # Run all tests
go test -race ./...               # Race condition detection
go test -cover ./...              # Coverage analysis
make test-cover                   # Tests with coverage + HTML report

# Specific test patterns
go test -v -run TestClient        # Run client tests (verbose)
go test -count=3 -run TestClient  # Run tests multiple times for consistency
make bench                        # Run benchmarks

# Code quality (run before commits)
gofmt -s -w .                     # Format code (CI uses `gofmt -s`; plain `go fmt ./...` omits -s and fails CI)
go vet ./...                      # Static analysis
golangci-lint run                 # Comprehensive linting
gocyclo -over 15 .                # Cyclomatic complexity check

# Makefile targets (recommended)
make check                        # Run all checks (fmt, vet, lint, cyclo)
make cyclo                        # Show complex functions (threshold: 15)
make cyclo-check                  # Fail if complexity exceeds threshold (CI)
make fmt-check                    # Verify code formatting
make security                     # Run security vulnerability checks
make sdk-test                     # Test SDK as consumer would use it
make release-check                # Pre-release validation
make ci                           # Run full CI pipeline locally
```

<!-- END AUTO-MANAGED -->

<!-- AUTO-MANAGED: architecture -->
## Architecture

```
.
├── client.go              # Client interface and WithClient/WithClientTransport context managers
├── query.go               # Query API (one-shot operations)
├── errors.go              # Structured error types
├── transport.go           # Transport interface abstraction
├── types.go               # Re-exports of all internal types and constants for external consumers
├── options.go             # Options types and functional options
├── options_bench_test.go  # Options performance benchmarks
├── internal/
│   ├── cli/               # CLI discovery and command building
│   ├── control/           # Bidirectional control protocol (hooks, permissions, MCP)
│   ├── parser/            # JSON message parsing with speculative parsing
│   ├── shared/            # Shared types (Message, ContentBlock interfaces)
│   └── subprocess/        # Subprocess management and protocol adapter
├── examples/              # Usage examples (numbered by complexity)
└── docs/
    ├── architecture/      # Detailed architecture documentation
    └── tracking/          # Python SDK parity tracking (PR replay tracker)
```

**Data Flow**:
1. `Query()`/`Client` -> `Transport` interface -> `subprocess.Transport` -> Claude CLI
2. CLI stdout -> `parser.Parser` -> `shared.Message` types -> User code
3. Control protocol: `control.Protocol` <-> CLI (hooks, permissions, MCP)

**Documentation**: See ARCHITECTURE.md and CONTRIBUTING.md for comprehensive details on design patterns, interfaces, data flow, and contribution guidelines.

<!-- END AUTO-MANAGED -->

<!-- AUTO-MANAGED: conventions -->
## Code Conventions

- **Idiomatic Go**: Use `gofmt` formatting, standard naming conventions
- **Interface-driven**: All message types implement `Message`, all content blocks implement `ContentBlock`
- **Error handling**: Use `fmt.Errorf` with `%w` verb for wrapping, include contextual information
- **Context-first**: All blocking functions accept `context.Context` as first parameter
- **JSON handling**: Custom `UnmarshalJSON` for union types, discriminate on `"type"` field
- **Cyclomatic complexity**: Keep functions under complexity 15 (measured by gocyclo); use `//nolint:gocyclo` on functions that legitimately exceed the threshold (large table-driven test functions, complex orchestration); extract helpers (e.g. `buildToolsListResult` from `routeMcpMethod`) to keep dispatch switches within budget rather than suppressing with nolint; higher complexity acceptable for table-driven tests, examples, orchestration code, method dispatch
- **Naming patterns**: Interfaces describe behavior, implementations use concrete names, options use `WithXxx()`, errors use `XxxError` suffix
- **No unnecessary exports**: Keep identifiers unexported unless needed by external consumers

<!-- END AUTO-MANAGED -->

<!-- AUTO-MANAGED: patterns -->
## Detected Patterns

- **Transport interface**: Central abstraction for CLI communication; use `MockTransport` for tests
- **Process cleanup**: SIGTERM -> wait 5 seconds -> SIGKILL pattern
- **Buffer protection**: 1MB limit to prevent memory exhaustion; `parser.NewWithSize(n)` allows configurable override
- **Environment variables**: Set `CLAUDE_CODE_ENTRYPOINT` to identify SDK to CLI
- **Table-driven tests**: Use for complex scenarios with multiple test cases
- **Functional options**: `WithXxx()` pattern for configuration
- **Benchmark tests**: Use `var sink any` to prevent dead code elimination, always call `b.ReportAllocs()` and `b.ResetTimer()`
- **tool_use_result metadata**: `UserMessage.ToolUseResult` carries rich edit info (filePath, structuredPatch, diffs); check with `HasToolUseResult()` before accessing via `GetToolUseResult()`
- **parent_tool_use_id placement**: parsed from top-level JSON data (not nested `message` object) for `UserMessage`, `AssistantMessage`, and `StreamEvent`; identifies messages produced inside a subagent (Agent/Task tool)
- **AssistantMessage error field**: `AssistantMessage.Error` is `*AssistantMessageError` parsed from top-level `data["error"]` (not nested `data["message"]["error"]`); CLI wire format is `{"type":"assistant","error":"rate_limit","message":{...}}`; use `HasError()` to check presence, `IsRateLimited()` for rate limit specifically; `AssistantMessageError` constants (all Python SDK parity): `rate_limit`, `billing_error`, `server_error`, `authentication_failed`, `invalid_request`, `unknown`
- **Unified streaming transport (Python SDK PR #468)**: `subprocess.New(cliPath, options, entrypoint)` is the single constructor; both `Query()` and `Client` route through streaming mode (`--input-format stream-json`) and run the initialize handshake unconditionally so the agents map can travel on the initialize control request instead of the deprecated `--agents` CLI flag. The prompt for `Query()` is written to stdin as a user-message JSON line (`{"type":"user","session_id":"","message":{"role":"user","content":...},"parent_tool_use_id":null}`) after `Connect()` returns. Wire-shape pins (no `omitempty`, key always emitted): `StreamMessage.SessionID` and `StreamMessage.ParentToolUseID` always emit `"session_id":""` and `"parent_tool_use_id":null` when unset; `InitializeRequest.Hooks` always emits `"hooks":null` when no hooks are registered (matches Python `"hooks": hooks_config if hooks_config else None`). `Transport.EndInput(ctx)` closes the stdin write side only (idempotent) and is invoked by `queryIterator.start()` immediately when no bidirectional channel is needed, or after the first `ResultMessage` when hooks/CanUseTool/SDK-MCP/file-checkpointing keep the control protocol alive. The needs-bidirectional decision lives in `needsBidirectionalStdin()` in `query.go`. The Go gate is deliberately wider than Python's (Python: `sdk_mcp_servers` OR `hooks`; Go: those + `CanUseTool` + `EnableFileCheckpointing`) as an idiomatic-Go divergence - Go has no preflight rejection for `Query(str)` + `CanUseTool`, and `EnableFileCheckpointing` legitimately requires the control protocol because `RewindFiles()` rides over it. `agentsToMap` stripping is intentionally stricter than Python (`if v is not None`): description/prompt always emit, empty Tools and empty Model are dropped - Go's zero-value-as-unset semantics can't round-trip Python's preserve-empty-list/string behavior until Phase 2 #19 (Python PR #684) lands nullable AgentDefinition fields.
- **Init error routing**: `subprocess.routeInitError()` detects error `ResultMessage` arriving before transport is connected and calls `protocol.HandleControlInitErr()` to unblock `SendControlRequest()` via `initErrChan`. `setupControlProtocol()` additionally spawns a watcher goroutine that calls `HandleControlInitErr()` when the CLI exits before responding to initialize (`stdoutDone` channel closed by `handleStdout`); the watcher captures channel/protocol pointers locally to avoid races on reconnect. `HandleControlInitErr()` reads `p.initialized` under lock and is a no-op after `Initialize()` succeeds, preventing the post-init watcher from poisoning `initErrChan` for later `SendControlRequest` calls (e.g. `SetModel`/`GetMcpStatus` on a long-lived client).
- **Control protocol delegation**: `SetModel()`, `SetPermissionMode()`, `RewindFiles()`, `GetMcpStatus()` guard with `t.connected` before delegating to `t.protocol`; nil protocol guard uses descriptive error `"internal error: transport connected but control protocol is nil"`. (No more `closeStdin` gate - every connection runs control protocol now.)
- **MCP config serialization**: `generateMcpConfigFile()` strips Go `Instance` field from SDK servers and propagates `AlwaysLoad` explicitly (not via struct json tags) when building CLI config
- **Permission suggestions**: `ToolPermissionContext.Suggestions []PermissionUpdate` carries CLI-provided permission suggestions to `CanUseTool` callbacks; `PermissionUpdate.Type` is one of `addRules`, `replaceRules`, `removeRules`, `setMode`, `addDirectories`, `removeDirectories`
- **Test mock helpers**: `newClientMockTransport()` / `newQueryMockTransport()` with functional options (`WithQueryAssistantResponse`, `WithQueryMultipleMessages`); `QueryWithTransport()` for transport-injected query tests
- **TestMain cross-platform mock CLI**: `internal/subprocess/mock_cli_test.go` and `internal/cli/mock_cli_test.go` each define a `TestMain` that checks a package-scoped env var (`CLAUDE_SDK_TEST_MOCK_MODE` / `CLAUDE_SDK_TEST_CLI_MOCK_MODE`) and re-enters the compiled test binary as a mock CLI process before `m.Run()`. This is the Go analog of Python's `sys.executable -c "..."` and matches the `os/exec` stdlib `TestHelperProcess` idiom. Replaces per-platform `.bat`/`.sh` fixtures. Test helpers (e.g. `newTransportMockCLI(t)`) set the env var via `t.Setenv` (auto-cleaned). `t.Setenv` is incompatible with `t.Parallel()` at the same scope - keep test functions using these helpers sequential. Race-mode parent contexts need ~30s budget because spawning the Go test binary is slower than bash.
- **Constructor functions**: `NewGetMcpStatusRequest()` follows `NewPermissionResultAllow/Deny` pattern - constructor sets required `Subtype` field (`SubtypeGetMcpStatus = "mcp_status"`, not `"get_mcp_status"`); use constructors for control request types with fixed subtype values
- **McpServerConfigType constants**: `McpServerConfigTypeStdio/SSE/HTTP/SDK/ClaudeAI` re-exported in root `types.go` alongside `McpServerConnectionStatus` constants; discriminate `McpServerStatusConfig.Type` field
- **McpServerStatus conditional fields**: `ServerInfo` non-nil only when `Status == McpServerConnectionStatusConnected`; `Error` non-nil only when `Status == McpServerConnectionStatusFailed`; `Tools` populated only when connected
- **streamErrChan fan-in**: `ClientImpl.streamErrChan chan error` (buffered, size 1) receives errors from `QueryStream` goroutine; passed directly to `clientIterator` alongside transport `errChan` and `streamErrChan`; `Next()` selects on both so callers see all stream errors without spawning goroutines
- **prepareOptions()**: applies defaults before validating - auto-configures `PermissionPromptToolName = "stdio"` when `CanUseTool` callback is set; renamed from `validateOptions()` to reflect dual role
- **ReceiveResponse() disconnected behavior**: returns non-nil `clientIterator` with closed `msgChan` and empty `errChan` (never nil) when called while disconnected; callers can always range over the result safely
- **PostToolUseFailureHookInput**: distinct from `PostToolUseHookInput`; fields: `ToolUseID string`, `Error string`, `IsInterrupt *bool json:"is_interrupt,omitempty"`; `IsInterrupt` nil means key absent in JSON (Python `NotRequired[bool]` semantics); `PostToolUseFailureHookSpecificOutput` is structurally identical to `PostToolUseHookSpecificOutput` (only `HookEventName` literal differs), both have `AdditionalContext *string` (omitempty); hook event count is now 10 (Notification, SubagentStart, PermissionRequest added in Python SDK PR #545); `_SubagentContextMixin` fields (`agent_id`/`agent_type`) deferred to Phase2 item #13 (Python PR #628), where the mixin is introduced and applied to PostToolUseFailureHookInput; HookEvent const block order matches Python SDK: PreToolUse, PostToolUse, PostToolUseFailure, UserPromptSubmit, Stop, SubagentStop, PreCompact, Notification, SubagentStart, PermissionRequest
- **WithHook() generic API**: use `WithHook(eventName, toolFilter, callback)` for hook events that have no convenience helper (e.g. `PostToolUseFailure`, `Notification`, `SubagentStart`, `PermissionRequest`); convenience helpers `WithPreToolUseHook` / `WithPostToolUseHook` exist only for PreToolUse and PostToolUse; see `examples/12_hooks` Example 4 (PostToolUseFailure) and Example 5 (Notification) for canonical usage
- **HookSpecificOutput types**: `PostToolUseHookSpecificOutput`, `PostToolUseFailureHookSpecificOutput` both require `HookEventName` field set to the event name string (json tag `hookEventName`); `AdditionalContext *string` (omitempty) is the field for injecting context Claude will read on the next turn; use a local `ptrTo[T any]` helper (`func ptrTo[T any](v T) *T { return &v }`) in examples for constructing `*string` values; `PermissionRequestHookSpecificOutput.Decision map[string]any` is the only output field without `omitempty` - it is required (mirrors Python required dict)
- **PostToolUseFailure IsInterrupt nil-check**: always guard `failInput.IsInterrupt != nil && *failInput.IsInterrupt` before treating as interrupt; nil means the JSON key was absent (not false); skip recovery context injection when true to respect user stop intent
- **SubagentStopHookInput agent fields**: `AgentID`, `AgentTranscriptPath`, `AgentType` added as flat required string fields in Python SDK PR #545 (NOT via `_SubagentContextMixin`); the mixin is a separate construct that lands in Python PR #628 / Phase 2 item #13 and is applied to the four tool-lifecycle inputs at that time
- **getAnySlice helper**: mirrors `getMap` in `internal/control/hooks.go`; returns nil when key absent (preserving Python `NotRequired` semantics); use when Python field type is `list[Any]` with `NotRequired` semantics (e.g. `PermissionRequestHookInput.PermissionSuggestions`)
- **UpdatedMCPToolOutput casing**: Go field name uses Go-idiomatic acronym casing (`UpdatedMCPToolOutput`) while wire tag preserves Python camelCase exactly (`json:"updatedMCPToolOutput,omitempty"`); apply this same pattern for any future fields with acronyms (MCP, HTTP, SSE)
- **NotificationHookInput fields**: `NotificationType string`, `Message string`, `Title *string` (omitempty); `Title` nil maps to Python `NotRequired[str]` absent state - always nil-guard before dereferencing; Notification hooks return empty `HookJSONOutput{}` (observation only, no HookSpecificOutput); `PermissionRequestHookInput.PermissionSuggestions` is `[]any` (mirrors Python `list[Any]`) with same nil-means-absent semantics; `PermissionRequestHookInput` intentionally has no `agent_id`/`agent_type` - Python PR #545 did not apply the mixin to it; those fields land with Phase 2 item #13
- **MCP tool annotations (Phase1 #5, Python PR #551)**: Two distinct annotation types - `shared.ToolAnnotations` (authoring side, MCP-spec Hint suffix: `ReadOnlyHint`/`DestructiveHint`/`IdempotentHint`/`OpenWorldHint` + `Title`, all `*T` omitempty) vs `control.McpToolAnnotations` (CLI status response side, Hint suffix stripped: `ReadOnly`/`Destructive`/`OpenWorld`); mirrors Python `mcp.types.ToolAnnotations` vs `McpToolAnnotations` TypedDict split; `ToolOption` functional option pattern extends `NewTool()` with variadic `...ToolOption` (backward compatible); consumers use `NewTool(..., WithToolAnnotations(&ToolAnnotations{...}))`; `annotationsToMap` uses `json.Marshal+Unmarshal` round-trip to honor omitempty; `buildToolsListResult` is a standalone helper (not inlined in `routeMcpMethod` switch) to keep dispatch under gocyclo budget; `tools/list` wire behavior: `nil` pointer omits `"annotations"` key entirely; `&ToolAnnotations{}` (non-nil, all fields unset) emits `"annotations": {}` (key present, empty map); camelCase wire keys: `title`, `readOnlyHint`, `destructiveHint`, `idempotentHint`, `openWorldHint`

<!-- END AUTO-MANAGED -->

<!-- AUTO-MANAGED: git-insights -->
## Git Insights

- Conventional commit messages: `feat:`, `fix:`, `docs:`, `test:`, `refactor:`, `chore:`
- Issue references in commits: `(Issue #N)` or `(#N)`, use `Closes #N` in PR body
- PR-based workflow with CI checks
- Recent focus: Phase1 item #6 - agents via initialize + unified streaming (Python PR #468): `subprocess.New()` single constructor; both `Query()` and `Client` use streaming mode + unconditional initialize handshake; agents travel on initialize control request; `EndInput()` closes stdin write side; review-feedback fixes: `StreamMessage.SessionID`/`ParentToolUseID` lack `omitempty` (wire always emits both keys), `HandleControlInitErr()` is no-op after successful Initialize (race guard for long-lived clients); Phase1 item #5 - MCP tool annotations (Python PR #551): `shared.ToolAnnotations` struct (authoring side) + `ToolOption`/`WithToolAnnotations` on `NewTool()`; `annotationsToMap` json round-trip; `buildToolsListResult` extracted from `routeMcpMethod` (gocyclo refactor, removed nolint); Phase1 item #4 - 3 new hook events + missing fields (Python PR #545): Notification, SubagentStart, PermissionRequest event types; hook count 7 -> 10
- Benchmark organization: Table-driven benchmarks across all core modules (options, parser, shared, control, cli)
- Makefile integration: All code quality checks (fmt, vet, lint, cyclo) unified under `make check`
- Python SDK parity tracking: `docs/tracking/README.md` tracks all Python SDK PRs to port (snapshot through Apr 12, 2026); organized into 4 chronological phases (Phase 1: Jan 26-Feb 20, Phase 2: Mar 3-Mar 16, Phase 3: Mar 20-Mar 30, Phase 4: Mar 31-Apr 8); Phase 1 items done: #1 GetMcpStatus (Go PR #124, Python PR #516), #2 PostToolUseFailure hook (Go PR #125, Python PR #535), #3 AssistantMessage error field fix (Go PR #124 squash, Python PR #506), #4 3 new hook events + missing fields (Go PR #128, Python PR #545), #5 MCP tool annotations (Python PR #551, Go PR #133), #6 Agents via initialize + unified streaming (Python PR #468, bundles Go issue #113 Query→streaming migration); next pending: Phase1 item #7; Phase3 item #35 done (Go PR #114, Python PR #749); post-snapshot PRs (merged after Apr 12, 2026) tracked in `docs/tracking/post-snapshot.md` using "P" row prefix (P1, P2, ...) with lettered deferred-scope sublists (a/b/c/...)

<!-- END AUTO-MANAGED -->

<!-- AUTO-MANAGED: best-practices -->
## Best Practices

- **TDD approach**: Write failing tests first, implement to make them pass
- **Test file organization**: Test functions first, then mocks, then helpers
- **Helper functions**: Always call `t.Helper()` in test utilities
- **Thread safety**: All mocks must be thread-safe with proper mutex usage
- **Self-contained tests**: Each test file has its own helpers to avoid dependencies
- **Benchmark organization**: Use table-driven benchmarks with realistic scenarios, measure allocations with `b.ReportAllocs()`
- **t.Fatal() + return**: Always follow `t.Fatal()` with `return` in subtests to prevent staticcheck SA5011 nil pointer dereference warnings (staticcheck does not track that t.Fatal() stops execution)
- **Cross-platform mock CLI via TestMain**: use `TestMain` + `os.Args[0]` re-entrancy (not `.bat`/`.sh` fixtures) for packages that need a mock subprocess; dispatch on a package-scoped env var set with `t.Setenv`; never use Windows-only skips to work around fixture limitations - the TestMain pattern works identically on all platforms

<!-- END AUTO-MANAGED -->

<!-- MANUAL -->
## Custom Notes

Add project-specific notes here. This section is never auto-modified.

<!-- END MANUAL -->
