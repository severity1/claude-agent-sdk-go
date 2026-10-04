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
├── config.go             # MCP config, environment, protocol options (skills, agents)
├── transport_test.go     # Transport lifecycle and core tests
├── io_test.go            # I/O and stderr callback tests
├── process_test.go       # Process termination tests
├── config_test.go        # Environment and MCP config tests
├── agents_test.go        # agentsToMap stripping and protocol options wiring tests
├── lifecycle_test.go     # Wait ownership, fast-exit output drain, init race, connect cancellation tests
├── mock_cli_test.go      # TestMain + os.Args[0] mock CLI (cross-platform, CLAUDE_SDK_TEST_MOCK_MODE); modes: default, long_running, should_fail, check_environment, invalid_output, with_control_protocol, with_stderr, init_error, burst_exit, exit_before_init, stdout_closed_alive, hang_init, early_error_result, two_permission_requests, exit_nonzero, error_result_exit, server_info, exit_clean, orphan_stdout, hold_stdout, ignore_sigterm, slow_exit_after_eof, stop_reading, fixed_size_line, close_orphan_stdout, orphan_stdout_and_stderr, orphan_holder
├── shutdown_test.go      # Graceful shutdown order and timing tests (event log via CLAUDE_SDK_TEST_MOCK_EVENT_LOG)
├── protocol_adapter.go   # ProtocolAdapter for control.Transport interface
└── protocol_adapter_test.go # Adapter tests
```

**Transport Flow**:
1. `Connect()`: Spawn CLI subprocess with streaming-mode arguments and run the initialize handshake unconditionally (Python SDK PR #468 parity). On `setupControlProtocol` failure, calls `teardownLocked()` to reap the subprocess before returning the error.
2. `SendMessage()`: Write JSON messages to stdin
3. `EndInput()`: Close the stdin write side only (stdout still drained). Mirrors Python `transport.end_input()` and is used by Query to signal end-of-input after the prompt write or first ResultMessage.
4. `handleStdout()`: Read stdout, parse JSON, route messages (io.go)
5. Control messages: Route to `control.Protocol.HandleIncomingMessageAsync()` (incoming control requests run on their own goroutine so a slow callback cannot stall `handleStdout()`)
6. `Close()`: SIGTERM -> wait 5s -> SIGKILL (process.go)

<!-- END AUTO-MANAGED -->

<!-- AUTO-MANAGED: conventions -->
## Module-Specific Conventions

- Wait ownership: `startProcessWaiter()` runs right after `cmd.Start()` and is the only `cmd.Wait()` caller; it sets `t.exit` (`*processExit{done, err}`, `exitReason(cmd.ProcessState, waitErr)`, nil state gives a `ConnectionError`) under `exitMu` and then closes `processDone`; `Done()/Err()` read `t.exit` under `exitMu`, never `t.mu`, and cleanup clears it. `terminateProcess()` returns at once if `processDone` is closed, else SIGTERM, then 5s (or ctx done), then SIGKILL, always waiting on `processDone`. stdout EOF is not proof of exit. `isProcessAlreadyFinishedError()` also treats the Windows `"Access is denied"` as a non-error
- Child output pipes: stdout and the stderr callback use `os.Pipe()` via `newChildOutputPipe()`, not `cmd.StdoutPipe()`/`StderrPipe()`, because `cmd.Wait()` closes those readers on exit and drops buffered output. The parent closes `childPipeEnds` after `Start()`. Readers drain to EOF (Python reads stdout to EOF, then calls `process.wait()`)
- Stderr callback: delivers every line already read, also after ctx cancel (Python never drops a read line); the loop ends on EOF or when cleanup closes the pipe
- Message routing: Distinguish control vs regular messages by type
- Protocol adapter: Bridges subprocess stdin to `control.Transport` interface
- Resource cleanup: Always close stdin before waiting for process exit
- Unified streaming: `New(cliPath, options, entrypoint)` is the single constructor; no `closeStdin` parameter, no `NewWithPrompt`. Prompts are written to stdin via `SendMessage` after `Connect` returns, and `EndInput(ctx)` closes the stdin write side (idempotent) when no more input will come.
- Init error routing: `Connect()` builds the control protocol before `go t.handleStdout(t.protocol)`, so the goroutine never reads `t.protocol` (which `teardownLocked` clears). `routeInitError(protocol, msg)` is a free function that detects an error `ResultMessage` while `!protocol.IsInitialized()` and calls `protocol.HandleControlInitErr()`. A separate stdout-close watcher in `setupControlProtocol` also calls `HandleControlInitErr` when the CLI exits before responding to initialize, so a dying CLI doesn't strand Initialize on its timeout. `formatInitError()` builds error string with priority: `Errors` slice > `Result` field > `Subtype` fallback.
- Stdout-done channel: `stdoutDone chan struct{}` is allocated per Connect and closed by `handleStdout` at stdout EOF (`signalStdoutDone`), before it waits for process exit. The watcher goroutine and `handleStdout` capture the channel/protocol pointer into locals before reading to avoid races with a subsequent reconnect.
- Exit reporting: `handleStdout(protocol, childProcess{cmd, done})` calls `childProcess.exitError(ctx, lastErrorResult)` after EOF; non-zero exit yields `*shared.ProcessError` built by `processExitError` ("Claude Code process exited unexpectedly (<state>)", or "Claude Code returned an error result: ..." after an error `ResultMessage`), clean exit or cancelled ctx yields nil. All terminal errors (exit error, stdout scanner error, buffer-size `*JSONDecodeError`) go through `endStreamWithError`, which first calls `protocol.FailPendingRequests(err)` so a control request in flight fails with the same error instead of timing out (`TestStreamEndFailsPendingInterrupt`, mock modes `exit_on_interrupt` and `overflow_on_interrupt`). Split into `processStdoutLine` + `sendStreamError` for gocyclo. `Transport.Done()` returns `processDone` (closed channel before Connect/after Close) and `Transport.Err()` returns `exitReason(ProcessState)` once it closes (`*shared.ProcessError` for non-zero, `*shared.ConnectionError` for exit 0), nil while running, "transport not connected" with no process; `ClientImpl` reads both through its `processWatcher` interface. `SendMessage` returns `*shared.ConnectionError`, `*shared.ConnectionError("cannot write to terminated CLI process", exitReason)` after `processDone` closes; `IsConnected()` is false after exit (`processExitedLocked`). The error is sent only after the last message is queued in `msgChan`, and `msgChan` closes after it (consumers in the root package rely on this order). Tests: `TestTransportReportsCLIExit`, `TestTransportSendsExitErrorAfterLastMessage`, `TestTransportCloseReportsNoProcessError`; helper `answerInitialize` in the mock.
- Connect failure cleanup: `Connect()` calls `teardownLocked()` when `setupControlProtocol` fails before returning the error. `teardownLocked()` is the unified post-Start cleanup primitive (cancels context, closes protocol, closes stdin, waits goroutines, terminates process, runs cleanup func) shared between `Close()` and the Connect error path. Ensures no subprocess is leaked on a failed connect. `TestTransportConnectFailureTearsDown` + `init_error` mock mode verifies this via `syscall.Signal(0)` probe on the captured PID.
- Batch CLI refusal: `Connect()` calls `cli.RejectWindowsBatchCLI(runtime.GOOS, t.cliPath)` right after the already-connected check, before the version probe spawns the CLI (BatBadBut, Python PR #1127)
- Environment build: `buildEnvironment()` drops the inherited `CLAUDECODE` variable, sets `CLAUDE_CODE_ENTRYPOINT`, applies `ExtraEnv`, then sets `CLAUDE_AGENT_SDK_VERSION=shared.SDKVersion` last so `ExtraEnv` cannot override it. `ExtraEnv` can set `CLAUDECODE` again.
- Stdout line limit: `handleStdout` sizes the scanner buffer to limit+1 (the newline uses one byte), so a line of exactly the limit passes. `bufio.ErrTooLong` becomes `parser.NewBufferOverflowError(limit, err)` through `sendStreamError`. Test: mock mode `fixed_size_line` (4096 bytes, `mockFixedLineLen`).
- Skills on initialize: `skillsProtocolOption()` returns `control.WithSkills(skills)` only when `options.Skills` is `[]string`. Other values (nil, `SkillsAll`) send no filter.
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
- **Mode protocol**: `CLAUDE_SDK_TEST_MOCK_MODE` selects a handler: `default`, `long_running`, `should_fail`, `check_environment`, `invalid_output`, `with_control_protocol`, `with_stderr`, `init_error`, `burst_exit`, `exit_before_init`, `stdout_closed_alive`, `hang_init`, `early_error_result`, `two_permission_requests`, `exit_nonzero`, `error_result_exit`, `server_info` (answers initialize with `mockInitializeResponse`)., `exit_clean`, `orphan_stdout` (re-runs the test binary as a `hold_stdout` descendant that inherits stdout, then exits 3, so stdout stays open after the CLI is gone). Modes that handle the streaming flow respond to `-v` first (so `cli.CheckCLIVersion` works), then participate in the unconditional initialize handshake by echoing a `control_response` for every `control_request` line on stdin.
- **Helper API**: `newTransportMockCLI(t)`, `newTransportMockCLIWithOptions(t, opts...)`, `newTransportMockCLIWithControlProtocol(t)`, `newTransportMockCLIWithStderr(t)`, `newTransportMockCLIMode(t, mode)`. `connectWithWatchdog(ctx, t, transport, limit)` fails the test if `Connect` hangs. Each calls `t.Setenv` to scope the mode to the test (auto-cleaned at test end) and returns `os.Args[0]`.
- **Parallel-subtest constraint**: `t.Setenv` is incompatible with `t.Parallel()` at the same scope. Sibling subtests under one parent using different modes must run sequentially. None of the existing tests call `t.Parallel()` here; keep it that way.
- **Race-mode timeouts**: spawning the Go test binary is slower than spawning a bash script (especially under `-race`). Parent-test contexts that fan out to multiple connecting subtests need a generous deadline (~30s for 3-6 subtests) so the budget isn't consumed by startup.

<!-- END MANUAL -->
