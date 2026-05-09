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
- Buffer limit: 1MB max (`MaxBufferSize`) to prevent memory exhaustion
- Speculative parsing: Match Python SDK behavior for streaming JSON
- Type discrimination: Use `"type"` field to determine message type
- `tool_use_result` extraction: `parseUserMessage` reads top-level `tool_use_result` map and passes it into `UserMessage.ToolUseResult`
- Control message routing: `control_request` and `control_response` types return `&shared.RawControlMessage{MessageType, Data}` - bypasses user-facing message stream, handled by control protocol layer
- Stream event handling: `stream_event` type dispatched to `parseStreamEventMessage`
- Forward-compat: unknown message types return `&shared.RawMessage{MessageType, Data}` instead of an error - new CLI versions can add types without breaking older SDK versions
- parseResultRequiredFields helper: required fields of ResultMessage (subtype, duration_ms, duration_api_ms, is_error, num_turns, session_id) extracted into a standalone function to keep `parseResultMessage` cyclomatic complexity under the gocyclo threshold as optional fields grow
- AssistantMessage error constants: `parseAssistantMessage` uses typed `shared.AssistantMessageError` constants, never raw strings; error objects without a "type" field collapse directly to `AssistantMessageErrorUnknown` (not marshaled to JSON string); callers always compare against typed constants via `HasError()`/`IsRateLimited()`

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
