package subprocess

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/severity1/claude-agent-sdk-go/internal/control"
	"github.com/severity1/claude-agent-sdk-go/internal/shared"
)

// TestTransportReceivesAllOutputAfterFastExit guards against reaping the CLI
// before stdout is drained: exec.Cmd.Wait closes a StdoutPipe reader as soon
// as the process exits, which drops output still buffered in the pipe.
func TestTransportReceivesAllOutputAfterFastExit(t *testing.T) {
	ctx, cancel := setupTransportTestContext(t, 30*time.Second)
	defer cancel()

	transport := New(newTransportMockCLIBurstExit(t), &shared.Options{}, "sdk-go")
	t.Cleanup(func() { _ = transport.Close() })
	connectTransportSafely(ctx, t, transport)

	msgChan, _ := transport.ReceiveMessages(ctx)
	received := 0
	for {
		select {
		case msg, ok := <-msgChan:
			if !ok {
				if received != burstExitMessageCount {
					t.Fatalf("received %d messages, want %d", received, burstExitMessageCount)
				}
				return
			}
			if _, isAssistant := msg.(*shared.AssistantMessage); isAssistant {
				received++
			}
		case <-ctx.Done():
			t.Fatalf("timed out after %d of %d messages", received, burstExitMessageCount)
			return
		}
	}
}

// TestTransportSendsExitErrorAfterLastMessage verifies the exit error reaches
// errChan only after every message, including the error ResultMessage, is
// queued in msgChan, so a consumer that drains msgChan after seeing the error
// loses nothing. Python raises ProcessError after read_messages ends.
func TestTransportSendsExitErrorAfterLastMessage(t *testing.T) {
	ctx, cancel := setupTransportTestContext(t, 30*time.Second)
	defer cancel()

	transport := New(newTransportMockCLIMode(t, mockModeBurstErrorResult), &shared.Options{}, "sdk-go")
	t.Cleanup(func() { _ = transport.Close() })
	connectTransportSafely(ctx, t, transport)

	msgChan, errChan := transport.ReceiveMessages(ctx)
	total := burstErrorResultAssistants + 1
	consumed := total - cap(msgChan)
	for i := 0; i < consumed; i++ {
		select {
		case <-msgChan:
		case <-ctx.Done():
			t.Fatalf("timed out reading message %d of %d", i+1, consumed)
		}
	}

	select {
	case err := <-errChan:
		var processErr *shared.ProcessError
		if !errors.As(err, &processErr) {
			t.Fatalf("errChan error = %v (%T), want *ProcessError", err, err)
		}
	case <-ctx.Done():
		t.Fatal("no exit error reported")
	}
	if got := len(msgChan); got != cap(msgChan) {
		t.Fatalf("%d messages queued when the exit error arrived, want all %d", got, cap(msgChan))
	}

	var last shared.Message
	for msg := range msgChan {
		last = msg
	}
	if result, ok := last.(*shared.ResultMessage); !ok || !result.IsError {
		t.Errorf("last message = %T, want the error *ResultMessage", last)
	}
}

// TestCloseFailsPendingInterrupt verifies an Interrupt in flight when the
// transport closes returns control.ErrProtocolClosed at once, instead of
// waiting out its 5 second timeout for a CLI that never answers it.
func TestCloseFailsPendingInterrupt(t *testing.T) {
	ctx, cancel := setupTransportTestContext(t, 30*time.Second)
	defer cancel()

	eventLog := setMockEventLog(t)
	transport := setupTransportForTest(t, newTransportMockCLIMode(t, mockModeIgnoreInterrupt))
	t.Cleanup(func() { _ = transport.Close() })
	connectTransportSafely(ctx, t, transport)

	result := make(chan error, 1)
	go func() { result <- transport.Interrupt(ctx) }()

	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, ok := readMockEvents(t, eventLog)[mockEventInterrupt]; ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the CLI never received the interrupt request")
		}
		time.Sleep(10 * time.Millisecond)
	}

	assertNoTransportError(t, transport.Close())

	select {
	case err := <-result:
		if !errors.Is(err, control.ErrProtocolClosed) {
			t.Errorf("Interrupt error = %v, want control.ErrProtocolClosed", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Interrupt still pending 2s after Close")
	}
}

// TestStreamEndFailsPendingInterrupt verifies an Interrupt still waiting for
// its response when the CLI's stream ends with an error returns that error at
// once, instead of waiting out its 5 second timeout (Python: the reader's
// error is set on every pending control request).
func TestStreamEndFailsPendingInterrupt(t *testing.T) {
	smallBuffer := 1024
	tests := []struct {
		name    string
		mode    string
		options *shared.Options
		check   func(t *testing.T, err error)
	}{
		{
			name:    "cli_exits_non_zero",
			mode:    mockModeExitOnInterrupt,
			options: &shared.Options{},
			check: func(t *testing.T, err error) {
				t.Helper()
				var processErr *shared.ProcessError
				if !errors.As(err, &processErr) {
					t.Fatalf("Interrupt error = %v (%T), want *ProcessError", err, err)
				}
				if processErr.ExitCode != 1 {
					t.Errorf("ExitCode = %d, want 1", processErr.ExitCode)
				}
			},
		},
		{
			// An over-limit line ends the stream with the buffer-size error (Python #190).
			name:    "stdout_line_over_limit",
			mode:    mockModeOverflowOnInterrupt,
			options: &shared.Options{MaxBufferSize: &smallBuffer},
			check: func(t *testing.T, err error) {
				t.Helper()
				var decodeErr *shared.JSONDecodeError
				if !errors.As(err, &decodeErr) || !strings.Contains(err.Error(), "maximum buffer size") {
					t.Fatalf("Interrupt error = %v, want the buffer-size *JSONDecodeError", err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := setupTransportTestContext(t, 30*time.Second)
			defer cancel()

			transport := New(newTransportMockCLIMode(t, tt.mode), tt.options, "sdk-go")
			t.Cleanup(func() { _ = transport.Close() })
			connectTransportSafely(ctx, t, transport)

			start := time.Now()
			result := make(chan error, 1)
			go func() { result <- transport.Interrupt(ctx) }()

			select {
			case err := <-result:
				tt.check(t, err)
			case <-time.After(2 * time.Second):
				t.Fatalf("Interrupt still pending %v after the stream ended with an error", time.Since(start))
			}
		})
	}
}

// TestTransportConnectFailureReapsProcess verifies that a failed Connect
// returns and reaps the CLI even with no context deadline. Connect only
// returns after teardown observed the sole cmd.Wait completing.
func TestTransportConnectFailureReapsProcess(t *testing.T) {
	tests := []struct {
		name          string
		mode          string
		errorContains string
	}{
		{"process_exits_before_initialize", mockModeExitBeforeInit, "initialize"},
		// stdout EOF while the process is alive must not block on cmd.Wait.
		{"stdout_closes_process_stays_alive", mockModeStdoutClosedAlive, "CLI process exited before initialize handshake completed"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transport := New(newTransportMockCLIMode(t, test.mode), &shared.Options{}, "sdk-go")
			err := connectWithWatchdog(context.Background(), t, transport, 20*time.Second)
			if err == nil {
				t.Fatal("Connect() error = nil, want initialization failure")
				return
			}
			if !strings.Contains(err.Error(), test.errorContains) {
				t.Fatalf("Connect() error = %q, want substring %q", err, test.errorContains)
			}
			if transport.cmd != nil {
				t.Error("transport.cmd should be nil after Connect failure teardown")
			}
		})
	}
}

// TestTransportConnectCancellationReapsHungInitialize verifies that a context
// deadline during initialize ends Connect and reaps the CLI.
func TestTransportConnectCancellationReapsHungInitialize(t *testing.T) {
	transport := New(newTransportMockCLIMode(t, mockModeHangInit), &shared.Options{}, "sdk-go")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := connectWithWatchdog(ctx, t, transport, 20*time.Second)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Connect() error = %v, want context.DeadlineExceeded in chain", err)
	}
	if transport.cmd != nil {
		t.Error("transport.cmd should be nil after Connect failure teardown")
	}
}

// TestTransportConcurrentCancelAndClose verifies Close finishes promptly after
// the Connect context is cancelled.
func TestTransportConcurrentCancelAndClose(t *testing.T) {
	transport := New(newTransportMockCLI(t), &shared.Options{}, "sdk-go")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := connectWithWatchdog(ctx, t, transport, 30*time.Second); err != nil {
		t.Fatalf("Connect() error = %v", err)
		return
	}

	cancel()
	closeDone := make(chan error, 1)
	go func() { closeDone <- transport.Close() }()
	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Close() did not finish after context cancellation")
	}
}

// TestTransportEarlyErrorResultNoRace runs init error routing while Connect
// still assigns state. Meaningful under -race.
func TestTransportEarlyErrorResultNoRace(t *testing.T) {
	cliPath := newTransportMockCLIMode(t, mockModeEarlyErrorResult)
	for i := 0; i < 10; i++ {
		transport := New(cliPath, &shared.Options{}, "sdk-go")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		_ = transport.Connect(ctx)
		_ = transport.Close()
		cancel()
	}
}

// connectWithWatchdog fails the test if Connect does not return in time.
func connectWithWatchdog(ctx context.Context, t *testing.T, transport *Transport, limit time.Duration) error {
	t.Helper()
	t.Cleanup(func() { _ = transport.Close() })
	done := make(chan error, 1)
	go func() { done <- transport.Connect(ctx) }()
	select {
	case err := <-done:
		return err
	case <-time.After(limit):
		t.Fatalf("Connect() did not return within %s", limit)
		return nil
	}
}

// TestTransportSlowPermissionCallbackDoesNotBlockReader verifies on the real
// handleStdout path that a blocked CanUseTool callback does not stop the next
// control request from being handled.
func TestTransportSlowPermissionCallbackDoesNotBlockReader(t *testing.T) {
	ctx, cancel := setupTransportTestContext(t, 30*time.Second)
	defer cancel()

	fastCalled := make(chan struct{})
	slowSawFast := make(chan bool, 1)
	options := &shared.Options{
		CanUseTool: func(_ context.Context, toolName string, _ map[string]any, _ any) (any, error) {
			switch toolName {
			case mockFastToolName:
				close(fastCalled)
			case mockSlowToolName:
				select {
				case <-fastCalled:
					slowSawFast <- true
				case <-time.After(10 * time.Second):
					slowSawFast <- false
				}
			}
			return control.NewPermissionResultAllow(), nil
		},
	}
	transport := New(newTransportMockCLIMode(t, mockModeTwoPermissionReqs), options, "sdk-go")
	t.Cleanup(func() { _ = transport.Close() })
	connectTransportSafely(ctx, t, transport)

	select {
	case ok := <-slowSawFast:
		if !ok {
			t.Fatal("second can_use_tool request was not handled while the first callback blocked")
		}
	case <-ctx.Done():
		t.Fatal("slow callback never ran")
	}
}

// TestTransportReportsCLIExit verifies a CLI that exits non-zero after
// connecting is visible to the caller (Issue #144, Python ProcessError):
// errChan yields a *ProcessError, IsConnected turns false, and SendMessage
// returns a *ConnectionError.
func TestTransportReportsCLIExit(t *testing.T) {
	tests := []struct {
		name         string
		mode         string
		wantExitCode int
		wantMessage  string
	}{
		{"crash_after_output", mockModeExitNonZero, mockCrashExitCode, "exited unexpectedly"},
		// The CLI exits 1 on purpose after an error result (Python #918).
		{"exit_after_error_result", mockModeErrorResultExit, 1, "Claude Code returned an error result: max turns reached"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := setupTransportTestContext(t, 30*time.Second)
			defer cancel()

			transport := New(newTransportMockCLIMode(t, test.mode), &shared.Options{}, "sdk-go")
			t.Cleanup(func() { _ = transport.Close() })
			connectTransportSafely(ctx, t, transport)

			msgChan, errChan := transport.ReceiveMessages(ctx)
			var processErr *shared.ProcessError
			for msgChan != nil || errChan != nil {
				select {
				case _, ok := <-msgChan:
					if !ok {
						msgChan = nil
					}
				case err, ok := <-errChan:
					if !ok {
						errChan = nil
						continue
					}
					if !errors.As(err, &processErr) {
						t.Fatalf("errChan error = %v (%T), want *ProcessError", err, err)
					}
				case <-ctx.Done():
					t.Fatal("stream did not end after the CLI exited")
					return
				}
			}

			if processErr == nil {
				t.Fatal("no *ProcessError reported for a non-zero CLI exit")
				return
			}
			if processErr.ExitCode != test.wantExitCode {
				t.Errorf("ExitCode = %d, want %d", processErr.ExitCode, test.wantExitCode)
			}
			if !strings.Contains(processErr.Error(), test.wantMessage) {
				t.Errorf("error = %q, want substring %q", processErr.Error(), test.wantMessage)
			}
			if transport.IsConnected() {
				t.Error("IsConnected() = true after the CLI exited")
			}
			var connErr *shared.ConnectionError
			if err := transport.SendMessage(ctx, shared.StreamMessage{Type: "user"}); !errors.As(err, &connErr) {
				t.Errorf("SendMessage after exit = %v, want *ConnectionError", err)
			}
		})
	}
}

// TestTransportCloseReportsNoProcessError verifies the SDK's own shutdown is
// not reported as a CLI crash.
func TestTransportCloseReportsNoProcessError(t *testing.T) {
	ctx, cancel := setupTransportTestContext(t, 30*time.Second)
	defer cancel()

	transport := New(newTransportMockCLI(t), &shared.Options{}, "sdk-go")
	connectTransportSafely(ctx, t, transport)
	_, errChan := transport.ReceiveMessages(ctx)
	if err := transport.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	for err := range errChan {
		var processErr *shared.ProcessError
		if errors.As(err, &processErr) {
			t.Fatalf("Close reported a ProcessError: %v", err)
		}
	}
}

// TestTransportDoneReportsCLIExit verifies that Done closes when the CLI
// exits on its own and that Err then says why.
// TestTransportDoneAndErrDoNotTakeTransportLock verifies that Done and Err
// return while another goroutine holds t.mu, as Close does during teardown.
func TestTransportDoneAndErrDoNotTakeTransportLock(t *testing.T) {
	ctx, cancel := setupTransportTestContext(t, 30*time.Second)
	defer cancel()

	transport := New(newTransportMockCLIMode(t, mockModeDefault), &shared.Options{}, "sdk-go")
	t.Cleanup(func() { _ = transport.Close() })
	connectTransportSafely(ctx, t, transport)

	transport.mu.Lock()
	defer transport.mu.Unlock()

	returned := make(chan error, 1)
	go func() {
		_ = transport.Done()
		returned <- transport.Err()
	}()
	select {
	case err := <-returned:
		if err != nil {
			t.Fatalf("Err() = %v while the CLI runs, want nil", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Done() or Err() blocked on the transport lock")
	}
}

func TestTransportDoneReportsCLIExit(t *testing.T) {
	tests := []struct {
		name string
		mode string
		kill bool
		// wantExitCode is checked only when wantProcessError is set.
		wantExitCode     int
		wantProcessError bool
	}{
		{name: "non_zero_exit", mode: mockModeExitNonZero, wantExitCode: mockCrashExitCode, wantProcessError: true},
		{name: "killed", mode: mockModeDefault, kill: true, wantExitCode: killedExitCode(), wantProcessError: true},
		{name: "clean_exit", mode: mockModeExitClean},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := setupTransportTestContext(t, 30*time.Second)
			defer cancel()

			transport := New(newTransportMockCLIMode(t, test.mode), &shared.Options{}, "sdk-go")
			t.Cleanup(func() { _ = transport.Close() })
			connectTransportSafely(ctx, t, transport)

			if test.kill {
				if err := transport.Err(); err != nil {
					t.Fatalf("Err() = %v while the CLI runs, want nil", err)
				}
				if err := transport.cmd.Process.Kill(); err != nil {
					t.Fatalf("kill CLI: %v", err)
				}
			}

			select {
			case <-transport.Done():
			case <-ctx.Done():
				t.Fatal("Done() did not close after the CLI exited")
				return
			}

			assertExitReason(t, transport.Err(), test.wantProcessError, test.wantExitCode)
		})
	}
}

// TestTransportDoneClosesBeforeStdoutEOF verifies that Done reports the CLI
// exit while a descendant still holds stdout open.
func TestTransportDoneClosesBeforeStdoutEOF(t *testing.T) {
	ctx, cancel := setupTransportTestContext(t, 30*time.Second)
	defer cancel()

	transport := New(newTransportMockCLIMode(t, mockModeOrphanStdout), &shared.Options{}, "sdk-go")
	t.Cleanup(func() { _ = transport.Close() })
	connectTransportSafely(ctx, t, transport)
	msgChan, _ := transport.ReceiveMessages(ctx)

	for done := false; !done; {
		select {
		case <-transport.Done():
			done = true
		case _, ok := <-msgChan:
			if !ok {
				t.Fatal("message channel closed before Done(), want Done() first")
				return
			}
		case <-ctx.Done():
			t.Fatal("Done() did not close after the CLI exited")
			return
		}
	}
	assertExitReason(t, transport.Err(), true, mockCrashExitCode)

	// The stream ends once the descendant exits and closes stdout.
	for msgChan != nil {
		select {
		case _, ok := <-msgChan:
			if !ok {
				msgChan = nil
			}
		case <-ctx.Done():
			t.Fatal("message channel did not close after the descendant exited")
			return
		}
	}
}

// TestTransportDoneBeforeConnectAndAfterClose verifies that a transport with
// no running CLI reports a closed Done and a non-nil Err.
func TestTransportDoneBeforeConnectAndAfterClose(t *testing.T) {
	ctx, cancel := setupTransportTestContext(t, 30*time.Second)
	defer cancel()

	transport := New(newTransportMockCLI(t), &shared.Options{}, "sdk-go")
	assertTransportStopped(t, transport, "before Connect")

	connectTransportSafely(ctx, t, transport)
	done := transport.Done()
	select {
	case <-done:
		t.Fatal("Done() closed while the CLI runs")
	default:
	}
	if err := transport.Err(); err != nil {
		t.Fatalf("Err() = %v while the CLI runs, want nil", err)
	}

	if err := transport.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	select {
	case <-done:
	default:
		t.Fatal("Done() from the connection did not close after Close()")
	}
	assertTransportStopped(t, transport, "after Close")
}

// assertExitReason checks Err after the CLI exited on its own.
func assertExitReason(t *testing.T, err error, wantProcessError bool, wantExitCode int) {
	t.Helper()
	var processErr *shared.ProcessError
	isProcessErr := errors.As(err, &processErr)
	if !wantProcessError {
		var connErr *shared.ConnectionError
		if !errors.As(err, &connErr) || isProcessErr {
			t.Fatalf("Err() = %v (%T), want *ConnectionError for a clean exit", err, err)
		}
		return
	}
	if !isProcessErr {
		t.Fatalf("Err() = %v (%T), want *ProcessError", err, err)
		return
	}
	if processErr.ExitCode != wantExitCode {
		t.Errorf("ExitCode = %d, want %d", processErr.ExitCode, wantExitCode)
	}
}

// assertTransportStopped checks Done and Err when no CLI process is running.
func assertTransportStopped(t *testing.T, transport *Transport, when string) {
	t.Helper()
	select {
	case <-transport.Done():
	default:
		t.Fatalf("Done() is open %s, want closed", when)
	}
	if err := transport.Err(); err == nil {
		t.Fatalf("Err() = nil %s, want an error", when)
	}
}

// killedExitCode is the exit code os/exec reports for a killed process: -1
// for a signal, and 1 on Windows, where Kill is TerminateProcess(1).
func killedExitCode() int {
	if runtime.GOOS == windowsOS {
		return 1
	}
	return -1
}

// TestTransportInitializationResult verifies that the initialize response the
// CLI sent during Connect is kept whole, and dropped by Close.
func TestTransportInitializationResult(t *testing.T) {
	ctx, cancel := setupTransportTestContext(t, 30*time.Second)
	defer cancel()

	transport := New(newTransportMockCLIMode(t, mockModeServerInfo), &shared.Options{}, "sdk-go")
	if got := transport.InitializationResult(); got != nil {
		t.Fatalf("InitializationResult() before Connect = %v, want nil", got)
	}
	t.Cleanup(func() { _ = transport.Close() })
	connectTransportSafely(ctx, t, transport)

	var want map[string]any
	if err := json.Unmarshal([]byte(mockInitializeResponse), &want); err != nil {
		t.Fatalf("decode mockInitializeResponse: %v", err)
	}
	if got := transport.InitializationResult(); !reflect.DeepEqual(got, want) {
		t.Fatalf("InitializationResult() = %v, want %v", got, want)
	}

	if err := transport.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if got := transport.InitializationResult(); got != nil {
		t.Fatalf("InitializationResult() after Close = %v, want nil", got)
	}
}
