# Module: parser

<!-- AUTO-MANAGED: module-description -->
## Purpose

JSON message parsing with speculative parsing and buffer management. Handles streaming JSON output from Claude CLI, including partial messages and embedded newlines.

<!-- END AUTO-MANAGED -->

<!-- AUTO-MANAGED: architecture -->
## Module Architecture

```
parser/
├── json.go            # Parser struct, ProcessLine, speculative parsing
├── json_test.go       # Parser tests
└── json_bench_test.go # Performance benchmarks
```

**Parsing Strategy**:
1. Accumulate input in buffer
2. Attempt JSON parse (speculative)
3. On success: return message, clear buffer
4. On failure: continue accumulating (incomplete JSON)
5. Buffer overflow protection at 1MB

<!-- END AUTO-MANAGED -->

<!-- AUTO-MANAGED: conventions -->
## Module-Specific Conventions

- Thread safety: Mutex protects buffer access
- Buffer limit: 1MB max (`MaxBufferSize`) to prevent memory exhaustion; use `NewWithSize(n)` to override
- Speculative parsing: Match Python SDK behavior for streaming JSON
- Type discrimination: Use `"type"` field to determine message type
- `tool_use_result` extraction: `parseUserMessage` reads top-level `tool_use_result` map and passes it into `UserMessage.ToolUseResult`
- `parent_tool_use_id` extraction: read from top-level data (not nested `message`) for `UserMessage`, `AssistantMessage`, and `StreamEvent` - matches Python SDK placement
- `AssistantMessage.Error` field: parsed from top-level `data["error"]` (not nested `data["message"]["error"]`); CLI wire format is `{"type":"assistant","error":"rate_limit","message":{...}}`; check with `HasError()` / `IsRateLimited()`
- `ToolResultBlock.IsError`: optional field parsed as `*bool` (nil when absent)

<!-- END AUTO-MANAGED -->

<!-- AUTO-MANAGED: dependencies -->
## Key Dependencies

- `internal/shared`: Message types for parsed output
- `encoding/json`: JSON parsing
- `strings`: Buffer management via `strings.Builder`

<!-- END AUTO-MANAGED -->

<!-- MANUAL -->
## Notes

<!-- END MANUAL -->
