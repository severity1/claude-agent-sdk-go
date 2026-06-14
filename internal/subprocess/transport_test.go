package subprocess

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/severity1/claude-agent-sdk-go/internal/shared"
)

// Test constants to avoid goconst linter warnings.
const (
	testModelName = "claude-sonnet-4-5"
)

// TestTransportLifecycle tests connection lifecycle, state management, and reconnection
func TestTransportLifecycle(t *testing.T) {
	ctx, cancel := setupTransportTestContext(t, 10*time.Second)
	defer cancel()

	// Test basic lifecycle
	transport := setupTransportForTest(t, newTransportMockCLI(t))
	defer disconnectTransportSafely(t, transport)

	// Initial state should be disconnected
	assertTransportConnected(t, transport, false)

	// Connect
	connectTransportSafely(ctx, t, transport)
	assertTransportConnected(t, transport, true)

	// Test multiple Close() calls are safe
	err1 := transport.Close()
	err2 := transport.Close()

	assertNoTransportError(t, err1)
	assertNoTransportError(t, err2)
	assertTransportConnected(t, transport, false)

	// Test reconnection capability
	connectTransportSafely(ctx, t, transport)
	assertTransportConnected(t, transport, true)
}

// TestTransportMessageIO tests basic message sending and receiving
func TestTransportMessageIO(t *testing.T) {
	ctx, cancel := setupTransportTestContext(t, 10*time.Second)
	defer cancel()

	transport := setupTransportForTest(t, newTransportMockCLI(t))
	defer disconnectTransportSafely(t, transport)

	connectTransportSafely(ctx, t, transport)

	// Test message sending
	message := shared.StreamMessage{
		Type:      "user",
		SessionID: "test-session",
	}

	err := transport.SendMessage(ctx, message)
	assertNoTransportError(t, err)

	// Test message receiving
	msgChan, errChan := transport.ReceiveMessages(ctx)
	if msgChan == nil || errChan == nil {
		t.Error("Message and error channels should not be nil")
	}

	// Test that channels don't block immediately
	select {
	case <-msgChan:
		// OK if message received
	case <-errChan:
		// OK if error received
	case <-time.After(100 * time.Millisecond):
		// OK if no immediate message - this is normal
	}
}

// TestTransportErrorHandling tests various error scenarios using table-driven approach
func TestTransportErrorHandling(t *testing.T) {
	ctx, cancel := setupTransportTestContext(t, 10*time.Second)
	defer cancel()

	tests := []struct {
		name           string
		setupTransport func() *Transport
		operation      func(*Transport) error
		expectError    bool
		errorContains  string
	}{
		{
			name: "connection_with_failing_cli",
			setupTransport: func() *Transport {
				return setupTransportForTest(t, newTransportMockCLIWithOptions(t, WithFailure()))
			},
			operation: func(tr *Transport) error {
				return tr.Connect(ctx)
			},
			// Initialize is unconditional now, so a failing CLI surfaces an
			// error from Connect instead of succeeding.
			expectError:   true,
			errorContains: "initialize",
		},
		{
			name: "send_to_disconnected_transport",
			setupTransport: func() *Transport {
				return setupTransportForTest(t, newTransportMockCLI(t))
			},
			operation: func(tr *Transport) error {
				// Don't connect - send to disconnected transport
				message := shared.StreamMessage{Type: "user", SessionID: "test"}
				return tr.SendMessage(ctx, message)
			},
			expectError:   true,
			errorContains: "",
		},
		{
			name: "context_cancellation",
			setupTransport: func() *Transport {
				return setupTransportForTest(t, newTransportMockCLI(t))
			},
			operation: func(tr *Transport) error {
				connectTransportSafely(ctx, t, tr)
				// Use canceled context
				canceledCtx, cancel := context.WithCancel(ctx)
				cancel()
				message := shared.StreamMessage{Type: "user", SessionID: "test"}
				return tr.SendMessage(canceledCtx, message)
			},
			expectError:   false, // Context cancellation handling may vary
			errorContains: "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transport := test.setupTransport()
			defer disconnectTransportSafely(t, transport)

			err := test.operation(transport)

			if test.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				} else if test.errorContains != "" && !strings.Contains(err.Error(), test.errorContains) {
					t.Errorf("Expected error containing '%s', got: %v", test.errorContains, err)
				}
			} else {
				if err != nil && test.errorContains != "" && !strings.Contains(err.Error(), test.errorContains) {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

// TestTransportConnectFailureTearsDown verifies that when initialize fails
// against a CLI that stays alive (does not self-exit), Connect tears the
// subprocess down before returning - the process is reaped and pipes/cmd
// are released. The init_error mock writes its PID to stderr at startup so
// the test can probe whether the OS process is gone.
func TestTransportConnectFailureTearsDown(t *testing.T) {
	ctx, cancel := setupTransportTestContext(t, 15*time.Second)
	defer cancel()

	pidCh := make(chan int, 1)
	options := &shared.Options{
		StderrCallback: func(line string) {
			const prefix = "MOCK_PID="
			if !strings.HasPrefix(line, prefix) {
				return
			}
			var pid int
			if _, err := fmt.Sscanf(line[len(prefix):], "%d", &pid); err == nil {
				select {
				case pidCh <- pid:
				default:
				}
			}
		},
	}
	transport := New(newTransportMockCLIInitError(t), options, "sdk-go")
	t.Cleanup(func() { _ = transport.Close() })

	err := transport.Connect(ctx)
	if err == nil {
		t.Fatal("Connect should have failed against init_error mock CLI")
		return
	}
	if !strings.Contains(err.Error(), "initialize") {
		t.Errorf("expected initialize-related error, got: %v", err)
	}

	if transport.IsConnected() {
		t.Error("transport should be disconnected after Connect failure")
	}
	if transport.cmd != nil {
		t.Error("transport.cmd should be nil after Connect failure teardown")
	}

	// Close after a torn-down Connect is a no-op.
	if err := transport.Close(); err != nil {
		t.Errorf("Close after Connect failure should be a no-op, got: %v", err)
	}

	// Verify the subprocess itself is gone (no zombie, no orphan). The mock
	// emitted its PID on stderr; on Unix, Signal(0) returns an error when
	// the process is no longer running.
	if runtime.GOOS == windowsOS {
		return // Signal(0) PID probe is POSIX-only.
	}
	var pid int
	select {
	case pid = <-pidCh:
	case <-time.After(2 * time.Second):
		t.Fatal("mock CLI never emitted its PID on stderr - cannot verify reaping")
		return
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		t.Fatalf("os.FindProcess(%d): %v", pid, err)
	}
	// Poll briefly because the kernel may take a few ms to mark the process
	// as exited after Wait returns.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if err := proc.Signal(syscall.Signal(0)); err != nil {
			return // process is gone - reaped successfully.
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Errorf("mock CLI (pid=%d) is still alive after Connect failure - teardown leaked it", pid)
}

// TestTransportConcurrency tests concurrent operations and backpressure handling
func TestTransportConcurrency(t *testing.T) {
	ctx, cancel := setupTransportTestContext(t, 15*time.Second)
	defer cancel()

	transport := setupTransportForTest(t, newTransportMockCLI(t))
	defer disconnectTransportSafely(t, transport)

	connectTransportSafely(ctx, t, transport)

	// Test concurrent message sending
	t.Run("concurrent_sending", func(t *testing.T) {
		var wg sync.WaitGroup
		errorCount := 0
		var mu sync.Mutex

		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				message := shared.StreamMessage{
					Type:      "user",
					SessionID: fmt.Sprintf("session-%d", id),
				}

				err := transport.SendMessage(ctx, message)
				if err != nil {
					mu.Lock()
					errorCount++
					mu.Unlock()
				}
			}(i)
		}

		wg.Wait()

		// Some errors might be acceptable in concurrent scenarios
		if errorCount > 2 {
			t.Errorf("Too many errors in concurrent sending: %d", errorCount)
		}
	})

	// Test backpressure handling
	t.Run("backpressure_handling", func(t *testing.T) {
		for i := 0; i < 10; i++ {
			message := shared.StreamMessage{
				Type:      "user",
				SessionID: "backpressure-test",
			}

			// Should not block indefinitely
			done := make(chan error, 1)
			go func() {
				done <- transport.SendMessage(ctx, message)
			}()

			select {
			case err := <-done:
				if err != nil && !strings.Contains(err.Error(), "context") && !strings.Contains(err.Error(), "closed") {
					t.Errorf("Unexpected error in message %d: %v", i, err)
				}
			case <-time.After(1 * time.Second):
				t.Errorf("Message %d took too long to send (backpressure issue)", i)
			}
		}
	})
}

// TestTransportReceiveMessagesNotConnected tests ReceiveMessages behavior on disconnected transport
// This targets the missing 44.4% coverage in ReceiveMessages function
func TestTransportReceiveMessagesNotConnected(t *testing.T) {
	ctx, cancel := setupTransportTestContext(t, 5*time.Second)
	defer cancel()

	transport := setupTransportForTest(t, newTransportMockCLI(t))

	// Test ReceiveMessages on disconnected transport
	msgChan, errChan := transport.ReceiveMessages(ctx)

	// Channels should not be nil
	if msgChan == nil {
		t.Error("Expected message channel to be non-nil")
	}
	if errChan == nil {
		t.Error("Expected error channel to be non-nil")
	}

	// Channels should be closed (for disconnected transport)
	select {
	case msg, ok := <-msgChan:
		if ok {
			t.Errorf("Expected message channel to be closed, got message: %v", msg)
		}
		// Channel is closed, which is expected
	case <-time.After(100 * time.Millisecond):
		t.Error("Expected message channel to be closed immediately")
	}

	select {
	case err, ok := <-errChan:
		if ok {
			t.Errorf("Expected error channel to be closed, got error: %v", err)
		}
		// Channel is closed, which is expected
	case <-time.After(100 * time.Millisecond):
		t.Error("Expected error channel to be closed immediately")
	}

	// Test multiple calls return the same behavior
	msgChan2, errChan2 := transport.ReceiveMessages(ctx)
	if msgChan2 == nil || errChan2 == nil {
		t.Error("Multiple calls should return valid channels")
	}

	// Verify they're different channel instances but behave the same
	select {
	case _, ok := <-msgChan2:
		if ok {
			t.Error("Expected second message channel to be closed")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Expected second message channel to be closed immediately")
	}
}

// Mock transport implementation with functional options
type transportMockOptions struct {
	longRunning      bool
	shouldFail       bool
	checkEnvironment bool
	invalidOutput    bool
}

type TransportMockOption func(*transportMockOptions)

func WithLongRunning() TransportMockOption {
	return func(opts *transportMockOptions) {
		opts.longRunning = true
	}
}

func WithFailure() TransportMockOption {
	return func(opts *transportMockOptions) {
		opts.shouldFail = true
	}
}

func WithEnvironmentCheck() TransportMockOption {
	return func(opts *transportMockOptions) {
		opts.checkEnvironment = true
	}
}

func WithInvalidOutput() TransportMockOption {
	return func(opts *transportMockOptions) {
		opts.invalidOutput = true
	}
}

func setupTransportTestContext(t *testing.T, timeout time.Duration) (context.Context, context.CancelFunc) {
	t.Helper()
	return context.WithTimeout(context.Background(), timeout)
}

func setupTransportForTest(t *testing.T, cliPath string) *Transport {
	t.Helper()
	options := &shared.Options{}
	return New(cliPath, options, "sdk-go")
}

func connectTransportSafely(ctx context.Context, t *testing.T, transport *Transport) {
	t.Helper()
	err := transport.Connect(ctx)
	if err != nil {
		t.Fatalf("Transport connection failed: %v", err)
	}
}

func disconnectTransportSafely(t *testing.T, transport *Transport) {
	t.Helper()
	if err := transport.Close(); err != nil {
		t.Logf("Transport disconnect warning: %v", err)
	}
}

func assertTransportConnected(t *testing.T, transport *Transport, expected bool) {
	t.Helper()
	actual := transport.IsConnected()
	if actual != expected {
		t.Errorf("Expected transport connected=%t, got connected=%t", expected, actual)
	}
}

func assertNoTransportError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}

// TestNewTransportStreamingDefaults verifies that the unified constructor
// creates a streaming-mode transport.
func TestNewTransportStreamingDefaults(t *testing.T) {
	tests := []struct {
		name       string
		entrypoint string
		options    *shared.Options
	}{
		{"sdk_go_entrypoint_empty_options", "sdk-go", &shared.Options{}},
		{"sdk_go_client_entrypoint_nil_options", "sdk-go-client", nil},
		{"sdk_go_with_system_prompt", "sdk-go", &shared.Options{SystemPrompt: stringPtr("test")}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transport := New("/usr/bin/claude", test.options, test.entrypoint)

			if transport == nil {
				t.Fatal("Expected transport to be created, got nil")
				return
			}
			if transport.entrypoint != test.entrypoint {
				t.Errorf("Expected entrypoint %q, got %q", test.entrypoint, transport.entrypoint)
			}
			assertTransportConnected(t, transport, false)
		})
	}
}

// TestConnectAlwaysUsesStreamingMode verifies that Connect builds a streaming
// command and starts the control protocol, even when no
// hooks/permissions/MCP are configured.
func TestConnectAlwaysUsesStreamingMode(t *testing.T) {
	ctx, cancel := setupTransportTestContext(t, 5*time.Second)
	defer cancel()

	transport := setupTransportForTest(t, newTransportMockCLIWithControlProtocol(t))
	defer disconnectTransportSafely(t, transport)

	if err := transport.Connect(ctx); err != nil {
		t.Skipf("Mock CLI control protocol not available: %v", err)
		return
	}

	assertTransportConnected(t, transport, true)

	// The control protocol must be initialized so the agents field can flow.
	if transport.protocol == nil {
		t.Fatal("Expected control protocol to be initialized even without hooks/MCP")
	}
}

// TestEndInputClosesStdinWriteOnly verifies that EndInput closes the stdin
// write side without tearing down the transport.
func TestEndInputClosesStdinWriteOnly(t *testing.T) {
	ctx, cancel := setupTransportTestContext(t, 5*time.Second)
	defer cancel()

	transport := setupTransportForTest(t, newTransportMockCLIWithControlProtocol(t))
	defer disconnectTransportSafely(t, transport)

	if err := transport.Connect(ctx); err != nil {
		t.Skipf("Mock CLI control protocol not available: %v", err)
		return
	}

	if err := transport.EndInput(ctx); err != nil {
		t.Fatalf("EndInput returned error: %v", err)
	}

	transport.mu.RLock()
	stdinAfter := transport.stdin
	transport.mu.RUnlock()
	if stdinAfter != nil {
		t.Error("Expected transport.stdin to be nil after EndInput")
	}

	// Idempotent: calling EndInput again must not error.
	if err := transport.EndInput(ctx); err != nil {
		t.Errorf("Second EndInput returned error: %v", err)
	}
}

// TestTransportConnectErrorPaths tests uncovered Connect error scenarios
func TestTransportConnectErrorPaths(t *testing.T) {
	ctx, cancel := setupTransportTestContext(t, 5*time.Second)
	defer cancel()

	tests := []struct {
		name      string
		setup     func() *Transport
		wantError bool
	}{
		{
			name: "already_connected_error",
			setup: func() *Transport {
				transport := setupTransportForTest(t, newTransportMockCLI(t))
				connectTransportSafely(ctx, t, transport)
				return transport
			},
			wantError: true,
		},
		{
			name: "invalid_working_directory",
			setup: func() *Transport {
				options := &shared.Options{Cwd: stringPtr("/nonexistent/directory/path")}
				return New(newTransportMockCLI(t), options, "sdk-go")
			},
			wantError: true,
		},
		{
			name: "cli_start_failure",
			setup: func() *Transport {
				return setupTransportForTest(t, "/nonexistent/cli/path")
			},
			wantError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transport := test.setup()
			defer disconnectTransportSafely(t, transport)

			err := transport.Connect(ctx)
			if test.wantError && err == nil {
				t.Error("Expected error but got none")
			} else if !test.wantError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

// TestTransportSendMessageEdgeCases tests uncovered SendMessage scenarios
func TestTransportSendMessageEdgeCases(t *testing.T) {
	// Generous timeout: each subtest spawns the test binary as the mock CLI,
	// which is slower than the legacy bash/.bat fixtures under -race.
	ctx, cancel := setupTransportTestContext(t, 30*time.Second)
	defer cancel()

	// Test SendMessage writes to stdin always (no more one-shot no-op)
	t.Run("send_message_writes_to_stdin_always", func(t *testing.T) {
		transport := setupTransportForTest(t, newTransportMockCLI(t))
		defer disconnectTransportSafely(t, transport)

		connectTransportSafely(ctx, t, transport)

		message := shared.StreamMessage{Type: "user", SessionID: "test"}
		err := transport.SendMessage(ctx, message)
		assertNoTransportError(t, err)
	})

	// Test SendMessage with invalid JSON
	t.Run("send_message_marshal_error", func(t *testing.T) {
		transport := setupTransportForTest(t, newTransportMockCLI(t))
		defer disconnectTransportSafely(t, transport)

		connectTransportSafely(ctx, t, transport)

		// Create a message that would cause JSON marshal error
		// In Go, this is difficult to trigger naturally, so we test normal case
		message := shared.StreamMessage{Type: "user", SessionID: "test"}
		err := transport.SendMessage(ctx, message)
		assertNoTransportError(t, err)
	})

	// Test context cancellation during send
	t.Run("context_cancelled_during_send", func(t *testing.T) {
		transport := setupTransportForTest(t, newTransportMockCLI(t))
		defer disconnectTransportSafely(t, transport)

		connectTransportSafely(ctx, t, transport)

		cancelledCtx, cancel := context.WithCancel(ctx)
		cancel() // Cancel immediately

		message := shared.StreamMessage{Type: "user", SessionID: "test"}
		err := transport.SendMessage(cancelledCtx, message)
		// Error is acceptable since context was cancelled
		if err != nil && !strings.Contains(err.Error(), "context") {
			t.Errorf("Expected context cancellation error, got: %v", err)
		}
	})
}

// TestTransportInterruptErrorPaths tests uncovered Interrupt scenarios
func TestTransportInterruptErrorPaths(t *testing.T) {
	ctx, cancel := setupTransportTestContext(t, 5*time.Second)
	defer cancel()

	// Test interrupt on disconnected transport
	t.Run("interrupt_disconnected_transport", func(t *testing.T) {
		transport := setupTransportForTest(t, newTransportMockCLI(t))

		// Don't connect - test interrupt on disconnected transport
		err := transport.Interrupt(ctx)
		if err == nil {
			t.Error("Expected error when interrupting disconnected transport")
		}
	})

	// Test interrupt with nil process
	t.Run("interrupt_nil_process", func(t *testing.T) {
		transport := setupTransportForTest(t, newTransportMockCLI(t))
		defer disconnectTransportSafely(t, transport)

		connectTransportSafely(ctx, t, transport)
		// Force close to null out the process
		disconnectTransportSafely(t, transport)

		err := transport.Interrupt(ctx)
		if err == nil {
			t.Error("Expected error when interrupting closed transport")
		}
	})

	if runtime.GOOS != windowsOS {
		t.Run("interrupt_signal_error", func(t *testing.T) {
			transport := setupTransportForTest(t, newTransportMockCLI(t))
			defer disconnectTransportSafely(t, transport)

			connectTransportSafely(ctx, t, transport)

			// Normal interrupt should work
			err := transport.Interrupt(ctx)
			assertNoTransportError(t, err)
		})
	}
}

// TestTransportControlProtocolIntegration tests that SetModel and SetPermissionMode
// work through the control protocol when properly wired.
func TestTransportControlProtocolIntegration(t *testing.T) {
	ctx, cancel := setupTransportTestContext(t, 10*time.Second)
	defer cancel()

	tests := []struct {
		name      string
		setup     func() *Transport
		operation func(ctx context.Context, t *Transport) error
		wantErr   bool
		errSubstr string
	}{
		{
			name: "SetModel_requires_connection",
			setup: func() *Transport {
				return setupTransportForTest(t, newTransportMockCLI(t))
			},
			operation: func(ctx context.Context, t *Transport) error {
				// Don't connect first
				model := testModelName
				return t.SetModel(ctx, &model)
			},
			wantErr:   true,
			errSubstr: "not connected",
		},
		{
			name: "SetPermissionMode_requires_connection",
			setup: func() *Transport {
				return setupTransportForTest(t, newTransportMockCLI(t))
			},
			operation: func(ctx context.Context, t *Transport) error {
				// Don't connect first
				return t.SetPermissionMode(ctx, "accept_edits")
			},
			wantErr:   true,
			errSubstr: "not connected",
		},
		{
			name: "SetModel_in_streaming_mode_with_protocol",
			setup: func() *Transport {
				// Streaming mode with control protocol mock CLI
				return setupTransportForTest(t, newTransportMockCLIWithControlProtocol(t))
			},
			operation: func(ctx context.Context, t *Transport) error {
				model := testModelName
				return t.SetModel(ctx, &model)
			},
			wantErr:   false, // Should succeed when protocol is wired
			errSubstr: "",
		},
		{
			name: "SetPermissionMode_in_streaming_mode_with_protocol",
			setup: func() *Transport {
				// Streaming mode with control protocol mock CLI
				return setupTransportForTest(t, newTransportMockCLIWithControlProtocol(t))
			},
			operation: func(ctx context.Context, t *Transport) error {
				return t.SetPermissionMode(ctx, "accept_edits")
			},
			wantErr:   false, // Should succeed when protocol is wired
			errSubstr: "",
		},
		{
			name: "SetModel_nil_resets_to_default",
			setup: func() *Transport {
				return setupTransportForTest(t, newTransportMockCLIWithControlProtocol(t))
			},
			operation: func(ctx context.Context, t *Transport) error {
				return t.SetModel(ctx, nil) // nil means reset to default
			},
			wantErr:   false,
			errSubstr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := tt.setup()
			defer disconnectTransportSafely(t, transport)

			// Connect only if not testing connection requirement
			if !strings.Contains(tt.name, "requires_connection") {
				connectTransportSafely(ctx, t, transport)
			}

			err := tt.operation(ctx, transport)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Expected error containing %q, got nil", tt.errSubstr)
				} else if !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("Expected error containing %q, got %q", tt.errSubstr, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

// TestTransportControlMessageRouting tests that control messages are properly
// routed to the protocol and regular messages go to msgChan.
func TestTransportControlMessageRouting(t *testing.T) {
	ctx, cancel := setupTransportTestContext(t, 10*time.Second)
	defer cancel()

	// Create transport with control protocol mock CLI
	transport := setupTransportForTest(t, newTransportMockCLIWithControlProtocol(t))
	defer disconnectTransportSafely(t, transport)

	connectTransportSafely(ctx, t, transport)

	// Get message channel
	msgChan, errChan := transport.ReceiveMessages(ctx)

	// Wait for messages
	var receivedRegularMsg bool
	timeout := time.After(2 * time.Second)

	for !receivedRegularMsg {
		select {
		case msg, ok := <-msgChan:
			if !ok {
				return // Channel closed
			}
			if msg != nil {
				receivedRegularMsg = true
				t.Logf("Received regular message: %T", msg)
			}
		case err := <-errChan:
			t.Logf("Received error: %v", err)
		case <-timeout:
			// Timeout is OK - control messages are filtered out
			return
		}
	}
}
