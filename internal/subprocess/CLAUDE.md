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
├── agents_test.go        # agentsToMap stripping and protocol options wiring tests
├── mock_cli_test.go      # TestMain + os.Args[0] mock CLI (cross-platform, CLAUDE_SDK_TEST_MOCK_MODE); modes: default, long_running, should_fail, check_environment, invalid_output, with_control_protocol, with_stderr, init_error
├── protocol_adapter.go   # ProtocolAdapter for control.Transport interface
└── protocol_adapter_test.go # Adapter tests
```

**Transport Flow**:
1. `Connect()`: Spawn CLI subprocess with streaming-mode arguments and run the initialize handshake unconditionally (Python SDK PR #468 parity). On `setupControlProtocol` failure, calls `teardownLocked()` to reap the subprocess before returning the error.
2. `SendMessage()`: Write JSON messages to stdin
3. `EndInput()`: Close the stdin write side only (stdout still drained). Mirrors Python `transport.end_input()` and is used by Query to signal end-of-input after the prompt write or first ResultMessage.
4. `handleStdout()`: Read stdout, parse JSON, route messages (io.go)
5. Control messages: Route to `control.Protocol.HandleIncomingMessage()`
6. `Close()`: SIGTERM -> wait 5s -> SIGKILL (process.go)

<!-- END AUTO-MANAGED -->

<!-- AUTO-MANAGED: conventions -->
## Module-Specific Conventions

- Graceful shutdown: SIGTERM with 5s grace period before SIGKILL; `terminateProcess()` checks `stdoutDone` before signaling - if closed (CLI already exited), calls `t.cmd.Wait()` to reap the process and release the exec.Cmd watchCtx goroutine, then returns nil immediately (mirrors Python `subprocess_cli.py:568`; expected signal exits are not surfaced as errors); avoids a race on Windows where `TerminateProcess` on a self-exited process returns `"Access is denied"`; `isProcessAlreadyFinishedError()` treats that string as a non-error alongside the POSIX equivalents
- Message routing: Distinguish control vs regular messages by type
- Protocol adapter: Bridges subprocess stdin to `control.Transport` interface
- Resource cleanup: Always close stdin before waiting for process exit
- Unified streaming: `New(cliPath, options, entrypoint)` is the single constructor; no `closeStdin` parameter, no `NewWithPrompt`. Prompts are written to stdin via `SendMessage` after `Connect` returns, and `EndInput(ctx)` closes the stdin write side (idempotent) when no more input will come.
- Init error routing: `routeInitError()` in io.go detects error `ResultMessage` before `t.connected` is set and calls `protocol.HandleControlInitErr()`. A separate stdout-close watcher in `setupControlProtocol` also calls `HandleControlInitErr` when the CLI exits before responding to initialize, so a dying CLI doesn't strand Initialize on its timeout. `formatInitError()` builds error string with priority: `Errors` slice > `Result` field > `Subtype` fallback.
- Stdout-done channel: `stdoutDone chan struct{}` is allocated per Connect and closed by `handleStdout` on exit. The watcher goroutine and `handleStdout` capture the channel/protocol pointer into locals before reading to avoid races with a subsequent reconnect.
- Connect failure cleanup: `Connect()` calls `teardownLocked()` when `setupControlProtocol` fails before returning the error. `teardownLocked()` is the unified post-Start cleanup primitive (cancels context, closes protocol, closes stdin, waits goroutines, terminates process, runs cleanup func) shared between `Close()` and the Connect error path. Ensures no subprocess is leaked on a failed connect. `TestTransportConnectFailureTearsDown` + `init_error` mock mode verifies this via `syscall.Signal(0)` probe on the captured PID.
- Nil protocol guard: `GetMcpStatus()` (and other control delegation methods) return descriptive error `"internal error: transport connected but control protocol is nil"` when `t.protocol == nil` after connected check
- `buildProtocolOptions()` is split into per-feature helpers (`canUseToolAdapter`, `hooksProtocolOption`, `sdkMcpServersProtocolOption`) to stay under gocyclo 15. The `agents` wiring uses `control.WithAgents(agentsToMap(...))`; `agentsToMap` converts `shared.AgentDefinition` to `map[string]any` at the package boundary so `control` stays free of any `shared` dependency.
- `agentsToMap` stripping rule (deliberate divergence from Python): `description` and `prompt` always emit; empty `Tools` slice and empty `Model` string are dropped. Python's rule (`if v is not None`) is more permissive - it preserves `tools=[]` and `model=""`. Go is stricter because `AgentDefinition` uses zero-value-as-unset semantics and there is no way for a caller to distinguish "explicit empty" from "unset" with the current field types. Phase 2 #19 (Python PR #684) will introduce nullable optional fields (skills/memory/mcpServers); at that point the per-field strip decision should be re-examined - description/prompt should keep their unconditional treatment.

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

### Cross-platform mock CLI (TestMain + os.Args[0] re-entrancy)

Subprocess tests use the compiled test binary itself as the mock Claude CLI, dispatched on env vars. This is the Go analog of Python's `sys.executable -c "..."` pattern and matches the os/exec stdlib `TestHelperProcess` idiom. It replaces the per-platform `.bat` / shell-script fixtures that previously could not reliably participate in the control protocol on Windows.

- **Entry point**: `mock_cli_test.go` defines `TestMain`. When `CLAUDE_SDK_TEST_MOCK_MODE` is set, the binary runs the mock handler and `os.Exit`s before `m.Run()` is reached. Otherwise normal tests run.
- **Mode protocol**: `CLAUDE_SDK_TEST_MOCK_MODE` selects a handler: `default`, `long_running`, `should_fail`, `check_environment`, `invalid_output`, `with_control_protocol`, `with_stderr`. Modes that handle the streaming flow respond to `-v` first (so `cli.CheckCLIVersion` works), then participate in the unconditional initialize handshake by echoing a `control_response` for every `control_request` line on stdin.
- **Helper API**: `newTransportMockCLI(t)`, `newTransportMockCLIWithOptions(t, opts...)`, `newTransportMockCLIWithControlProtocol(t)`, `newTransportMockCLIWithStderr(t)`. Each calls `t.Setenv` to scope the mode to the test (auto-cleaned at test end) and returns `os.Args[0]`.
- **Parallel-subtest constraint**: `t.Setenv` is incompatible with `t.Parallel()` at the same scope. Sibling subtests under one parent using different modes must run sequentially. None of the existing tests call `t.Parallel()` here; keep it that way.
- **Race-mode timeouts**: spawning the Go test binary is slower than spawning a bash script (especially under `-race`). Parent-test contexts that fan out to multiple connecting subtests need a generous deadline (~30s for 3-6 subtests) so the budget isn't consumed by startup.

<!-- END MANUAL -->
