# Module: subprocess

<!-- AUTO-MANAGED: module-description -->
## Purpose

Subprocess management and transport layer. Spawns Claude CLI process, manages stdin/stdout communication, and implements the `Transport` interface for message passing.

<!-- END AUTO-MANAGED -->

<!-- AUTO-MANAGED: architecture -->
## Module Architecture

```
subprocess/
├── transport.go          # Transport struct, Connect, lifecycle orchestration
├── io.go                 # Stdout/stderr handling, message parsing
├── process.go            # Process termination, cleanup
├── config.go             # MCP config, environment, protocol options
├── transport_test.go     # Transport lifecycle and core tests
├── io_test.go            # I/O and stderr callback tests
├── process_test.go       # Process termination tests
├── config_test.go        # Environment and MCP config tests
├── protocol_adapter.go   # ProtocolAdapter for control.Transport interface
└── protocol_adapter_test.go # Adapter tests
```

**Transport Flow**:
1. `Connect()`: Spawn CLI subprocess with streaming-mode arguments and run the initialize handshake unconditionally (Python SDK PR #468 parity).
2. `SendMessage()`: Write JSON messages to stdin
3. `EndInput()`: Close the stdin write side only (stdout still drained). Mirrors Python `transport.end_input()` and is used by Query to signal end-of-input after the prompt write or first ResultMessage.
4. `handleStdout()`: Read stdout, parse JSON, route messages (io.go)
5. Control messages: Route to `control.Protocol.HandleIncomingMessage()`
6. `Close()`: SIGTERM -> wait 5s -> SIGKILL (process.go)

<!-- END AUTO-MANAGED -->

<!-- AUTO-MANAGED: conventions -->
## Module-Specific Conventions

- Graceful shutdown: SIGTERM with 5s grace period before SIGKILL
- Message routing: Distinguish control vs regular messages by type
- Protocol adapter: Bridges subprocess stdin to `control.Transport` interface
- Resource cleanup: Always close stdin before waiting for process exit
- Unified streaming: `New(cliPath, options, entrypoint)` is the single constructor; no `closeStdin` parameter, no `NewWithPrompt`. Prompts are written to stdin via `SendMessage` after `Connect` returns, and `EndInput(ctx)` closes the stdin write side (idempotent) when no more input will come.
- Init error routing: `routeInitError()` in io.go detects error `ResultMessage` before `t.connected` is set and calls `protocol.HandleControlInitErr()`. A separate stdout-close watcher in `setupControlProtocol` also calls `HandleControlInitErr` when the CLI exits before responding to initialize, so a dying CLI doesn't strand Initialize on its timeout. `formatInitError()` builds error string with priority: `Errors` slice > `Result` field > `Subtype` fallback.
- Stdout-done channel: `stdoutDone chan struct{}` is allocated per Connect and closed by `handleStdout` on exit. The watcher goroutine and `handleStdout` capture the channel/protocol pointer into locals before reading to avoid races with a subsequent reconnect.
- Nil protocol guard: `GetMcpStatus()` (and other control delegation methods) return descriptive error `"internal error: transport connected but control protocol is nil"` when `t.protocol == nil` after connected check
- `buildProtocolOptions()` is split into per-feature helpers (`canUseToolAdapter`, `hooksProtocolOption`, `sdkMcpServersProtocolOption`) to stay under gocyclo 15. The `agents` wiring uses `control.WithAgents(agentsToMap(...))`; `agentsToMap` converts `shared.AgentDefinition` to `map[string]any` at the package boundary so `control` stays free of any `shared` dependency.

<!-- END AUTO-MANAGED -->

<!-- AUTO-MANAGED: dependencies -->
## Key Dependencies

- `internal/parser`: JSON message parsing
- `internal/control`: Control protocol for hooks/permissions
- `os/exec`: Subprocess management
- `bufio`: Line-by-line stdout reading

<!-- END AUTO-MANAGED -->

<!-- MANUAL -->
## Notes

<!-- END MANUAL -->
