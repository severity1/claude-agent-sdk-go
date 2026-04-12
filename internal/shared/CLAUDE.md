# Module: shared

<!-- AUTO-MANAGED: module-description -->
## Purpose

Shared types used across the SDK. Defines the `Message` and `ContentBlock` interfaces, concrete message types, error types, options, and streaming utilities.

<!-- END AUTO-MANAGED -->

<!-- AUTO-MANAGED: architecture -->
## Module Architecture

```
shared/
├── message.go             # Message interface, UserMessage, AssistantMessage, ResultMessage
├── message_test.go        # Message type tests
├── message_bench_test.go  # Message benchmarks
├── errors.go              # CLINotFoundError, ConnectionError, etc.
├── errors_test.go         # Error type tests
├── errors_helpers_test.go # Error helper tests
├── options.go             # Options struct, functional options
├── options_test.go        # Options tests
├── stream.go              # StreamIssue, StreamStats
├── stream_test.go         # Stream tests
└── validator.go           # Input validation
```

**Type Hierarchy**:
- `Message` interface: `Type() string`
- `ContentBlock` interface: `BlockType() string`
- Concrete types: `UserMessage`, `AssistantMessage`, `SystemMessage`, `ResultMessage`
- Content blocks: `TextBlock`, `ThinkingBlock`, `ToolUseBlock`, `ToolResultBlock`

**UserMessage fields**:
- `Content`: string or `[]ContentBlock`
- `UUID`, `ParentToolUseID`: optional string pointers
- `ToolUseResult map[string]any`: rich edit metadata (filePath, structuredPatch, diffs); use `HasToolUseResult()` / `GetToolUseResult()`

<!-- END AUTO-MANAGED -->

<!-- AUTO-MANAGED: conventions -->
## Module-Specific Conventions

- Interface-driven polymorphism: All message types implement `Message`
- Custom JSON unmarshaling: Use `json.RawMessage` for delayed parsing
- Type discrimination: Switch on `"type"` field for union types
- Error wrapping: Use `%w` verb for error chain support

<!-- END AUTO-MANAGED -->

<!-- AUTO-MANAGED: dependencies -->
## Key Dependencies

- `encoding/json`: JSON serialization/deserialization
- Standard library only (no external dependencies)

<!-- END AUTO-MANAGED -->

<!-- MANUAL -->
## Notes

<!-- END MANUAL -->
