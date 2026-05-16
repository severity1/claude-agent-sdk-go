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
go fmt ./...                      # Format code
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
- **Cyclomatic complexity**: Keep functions under complexity 15 (measured by gocyclo); use `//nolint:gocyclo` on large table-driven test functions that legitimately exceed the threshold; higher complexity acceptable for table-driven tests, examples, orchestration code
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
- **Init error routing**: `subprocess.routeInitError()` detects error `ResultMessage` arriving before transport is connected and calls `protocol.HandleControlInitErr()` to unblock `SendControlRequest()` via `initErrChan`
- **Control protocol delegation**: `SetModel()`, `SetPermissionMode()`, `RewindFiles()`, `GetMcpStatus()` all guard with `t.connected && !t.closeStdin` before delegating to `t.protocol`; nil protocol guard uses descriptive error `"internal error: transport connected but control protocol is nil"`
- **MCP config serialization**: `generateMcpConfigFile()` strips Go `Instance` field from SDK servers and propagates `AlwaysLoad` explicitly (not via struct json tags) when building CLI config
- **Permission suggestions**: `ToolPermissionContext.Suggestions []PermissionUpdate` carries CLI-provided permission suggestions to `CanUseTool` callbacks; `PermissionUpdate.Type` is one of `addRules`, `replaceRules`, `removeRules`, `setMode`, `addDirectories`, `removeDirectories`
- **Test mock helpers**: `newClientMockTransport()` / `newQueryMockTransport()` with functional options (`WithQueryAssistantResponse`, `WithQueryMultipleMessages`); `QueryWithTransport()` for transport-injected query tests
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

<!-- END AUTO-MANAGED -->

<!-- AUTO-MANAGED: git-insights -->
## Git Insights

- Conventional commit messages: `feat:`, `fix:`, `docs:`, `test:`, `refactor:`, `chore:`
- Issue references in commits: `(Issue #N)` or `(#N)`, use `Closes #N` in PR body
- PR-based workflow with CI checks
- Recent focus: Phase1 item #4 - 3 new hook events + missing fields (Python PR #545): Notification, SubagentStart, PermissionRequest event types; tool_use_id on PreToolUse/PostToolUse inputs; agent fields on SubagentStop; additionalContext on PreToolUseHookSpecificOutput; updatedMCPToolOutput on PostToolUseHookSpecificOutput; hook count 7 -> 10; PostToolUseFailure hook event (Python PR #535, Phase1 item #2); AssistantMessage error object form (Python PR #506, Go PR #124 squash); GetMcpStatus() control protocol method (Python PR #516, Go PR #124)
- Benchmark organization: Table-driven benchmarks across all core modules (options, parser, shared, control, cli)
- Makefile integration: All code quality checks (fmt, vet, lint, cyclo) unified under `make check`
- Python SDK parity tracking: `docs/tracking/README.md` tracks all Python SDK PRs to port (snapshot through Apr 12, 2026); organized into 4 chronological phases (Phase 1: Jan 26-Feb 20, Phase 2: Mar 3-Mar 16, Phase 3: Mar 20-Mar 30, Phase 4: Mar 31-Apr 8); Phase 1 items done: #1 GetMcpStatus (Go PR #124, Python PR #516), #2 PostToolUseFailure hook (Go PR #125, Python PR #535), #3 AssistantMessage error field fix (Go PR #124 squash, Python PR #506), #4 3 new hook events + missing fields (Go PR #128, Python PR #545); next pending: Phase1 item #5 (Python PR #551, MCP tool annotations); Phase3 item #35 done (Go PR #114, Python PR #749); post-snapshot PRs (merged after Apr 12, 2026) tracked in `docs/tracking/post-snapshot.md` using "P" row prefix (P1, P2, ...) with lettered deferred-scope sublists (a/b/c/...)

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

<!-- END AUTO-MANAGED -->

<!-- MANUAL -->
## Custom Notes

Add project-specific notes here. This section is never auto-modified.

<!-- END MANUAL -->
