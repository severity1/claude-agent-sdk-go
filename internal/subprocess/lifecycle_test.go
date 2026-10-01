package subprocess

import (
	"context"
	"errors"
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
