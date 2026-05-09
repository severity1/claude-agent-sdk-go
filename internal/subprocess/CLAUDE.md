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
1. `Connect()`: Spawn CLI subprocess with configured arguments
2. `SendMessage()`: Write JSON messages to stdin
3. `handleStdout()`: Read stdout, parse JSON, route messages (io.go)
4. Control messages: Route to `control.Protocol.HandleIncomingMessage()`
5. `Close()`: SIGTERM -> wait 5s -> SIGKILL (process.go)

<!-- END AUTO-MANAGED -->

<!-- AUTO-MANAGED: conventions -->
## Module-Specific Conventions

- Graceful shutdown: SIGTERM with 5s grace period before SIGKILL
- Message routing: Distinguish control vs regular messages by type
- Protocol adapter: Bridges subprocess stdin to `control.Transport` interface
- Resource cleanup: Always close stdin before waiting for process exit
- Init error routing: `routeInitError()` in io.go detects error `ResultMessage` before `t.connected` is set and calls `protocol.HandleControlInitErr()`; `formatInitError()` builds error string with priority: `Errors` slice > `Result` field > `Subtype` fallback
- Protocol options wiring: `buildProtocolOptions()` in config.go appends `control.WithOptions(t.options)` so agents flow through the Initialize handshake instead of via CLI flags
- GetMcpStatus delegation: `Transport.GetMcpStatus()` in config.go delegates to `protocol.GetMcpStatus()`; only available in streaming mode (closeStdin == false)
- MCP config cleanup: `cleanup()` in process.go calls `mcpConfigFile.Close()` then `os.Remove()`; Close() returns `os.ErrClosed` benignly because the file was already closed in `generateMcpConfigFile` after write/sync (retain *os.File only for Name()/Remove)
- ensureProtocolInitialized lazy-init: `Transport.ensureProtocolInitialized(ctx)` in transport.go performs control protocol handshake on demand; returns error immediately for one-shot mode ("control protocol not available in one-shot mode") or nil protocol ("not started"); used by GetMcpStatus so callers don't need up-front feature flags to trigger init
- ReceiveMessages disconnected behavior: returns two closed channels immediately when transport is not connected; callers can range or select on them safely; multiple calls return distinct channel instances all exhibiting the same closed behavior
- noopControlTransport test helper: adapter_test.go defines an inline `noopControlTransport` (Write captures bytes into a slice, Read returns a never-sending channel that Close() closes once, Close is idempotent via select) to drive `control.Protocol` in unit tests without spawning a real subprocess
- TestCanUseToolAdapter_PreservesToolUseIDAndAgentID: regression test in adapter_test.go locks the `buildProtocolOptions()` -> `control.Protocol` bridge; verifies that `ToolUseID` and `AgentID` on `ToolPermissionContext` survive the `any`-typed `Options.CanUseTool` adapter hop intact
- TestGetMcpStatus_LazyInitSuccessPath: transport_test.go exercises the full `ensureProtocolInitialized` -> `protocol.Initialize` -> `GetMcpStatus` code path using `newTransportMockCLIWithControlProtocol`

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
