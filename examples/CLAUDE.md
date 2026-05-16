# Module: examples

<!-- AUTO-MANAGED: module-description -->
## Purpose

Working examples demonstrating SDK usage patterns. Examples are numbered by complexity (01-20) from beginner to advanced, covering Query API, Client API, tools, MCP integration, and production patterns.

<!-- END AUTO-MANAGED -->

<!-- AUTO-MANAGED: architecture -->
## Module Architecture

```
examples/
├── 01_quickstart/           # Basic Query API usage
├── 02_client_streaming/     # Real-time streaming responses
├── 03_client_multi_turn/    # Multi-turn conversations
├── 04_query_with_tools/     # File operations with Query API
├── 05_client_with_tools/    # Interactive file workflows; tool_use_result metadata, SetPermissionMode
├── 06_query_with_mcp/       # MCP server integration (Query)
├── 07_client_with_mcp/      # MCP server integration (Client)
├── 08_client_advanced/      # Error handling, model switching
├── 09_context_manager/      # WithClient pattern
├── 10_session_management/   # Session isolation
├── 11_permission_callback/  # Tool permission control
├── 12_hooks/                # Lifecycle hooks; PreToolUse logging, command blocking, PostToolUse context injection, PostToolUseFailure recovery via WithHook(), Notification observation via WithHook()
├── 13_file_checkpointing/   # File rewind capabilities
├── 14_sdk_mcp_server/       # In-process custom tools; 3 sub-examples: Calculator, Text Processor, Annotated Tool (circle_area with ReadOnlyHint/IdempotentHint/OpenWorldHint via WithToolAnnotations)
├── 15_programmatic_subagents/ # Agent definitions
├── 16_structured_output/    # JSON schema constraints
├── 17_plugins/              # Plugin configuration
├── 18_sandbox_security/     # Command isolation
├── 19_partial_streaming/    # Real-time delta updates
├── 20_debugging_and_diagnostics/ # Debug output, health monitoring
└── README.md                # Example documentation
```

<!-- END AUTO-MANAGED -->

<!-- AUTO-MANAGED: conventions -->
## Module-Specific Conventions

- Each example is self-contained in its own directory
- All examples have a `main.go` with runnable code
- Run with `go run main.go` from the example directory
- Prerequisites noted in README.md (e.g., MCP servers need `uvx`)
- Use `WithClient` + `client.Query` + `client.ReceiveMessages(ctx)` as the standard streaming pattern
- Use `WithHook(eventName, toolFilter, callback)` for hook events without convenience helpers (PostToolUseFailure, Notification, SubagentStart, PermissionRequest); `WithPreToolUseHook` / `WithPostToolUseHook` are the only convenience helpers
- Use a local `ptrTo[T any]` helper (`func ptrTo[T any](v T) *T { return &v }`) for constructing pointer fields; defined in examples 12 (for `AdditionalContext *string`) and 14 (for `ToolAnnotations` pointer fields like `ReadOnlyHint *bool`)
- Use `toolUseID *string` parameter for correlating pre/post hook calls (e.g., timing via a map keyed by tool use ID); always guard with `if toolUseID == nil` before dereferencing

<!-- END AUTO-MANAGED -->

<!-- AUTO-MANAGED: dependencies -->
## Key Dependencies

- Root `claudecode` package
- Claude CLI installed (`npm install -g @anthropic-ai/claude-code`)
- Go 1.18+
- Optional: `uvx` for MCP server examples

<!-- END AUTO-MANAGED -->

<!-- MANUAL -->
## Notes

<!-- END MANUAL -->
