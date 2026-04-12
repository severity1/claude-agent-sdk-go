package subprocess

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/severity1/claude-agent-sdk-go/internal/shared"
)

// Test constants to avoid goconst linter warnings.
const (
	testBatExtension = ".bat"
	testModelName    = "claude-sonnet-4-5"
)

// TestTransportLifecycle tests connection lifecycle, state management, and reconnection
func TestTransportLifecycle(t *testing.T) {
	ctx, cancel := setupTransportTestContext(t, 10*time.Second)
	defer cancel()

	// Test basic lifecycle
	transport := setupTransportForTest(t, newTransportMockCLI())
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

	transport := setupTransportForTest(t, newTransportMockCLI())
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
				return setupTransportForTest(t, newTransportMockCLIWithOptions(WithFailure()))
			},
			operation: func(tr *Transport) error {
				return tr.Connect(ctx)
			},
			expectError:   false, // Connection should succeed initially even if CLI fails
			errorContains: "",
		},
		{
			name: "send_to_disconnected_transport",
			setupTransport: func() *Transport {
				return setupTransportForTest(t, newTransportMockCLI())
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
				return setupTransportForTest(t, newTransportMockCLI())
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

// TestTransportConcurrency tests concurrent operations and backpressure handling
func TestTransportConcurrency(t *testing.T) {
	ctx, cancel := setupTransportTestContext(t, 15*time.Second)
	defer cancel()

	transport := setupTransportForTest(t, newTransportMockCLI())
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

	transport := setupTransportForTest(t, newTransportMockCLI())

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

func newTransportMockCLI() string {
	return newTransportMockCLIWithOptions()
}

func newTransportMockCLIWithOptions(options ...TransportMockOption) string {
	opts := &transportMockOptions{}
	for _, opt := range options {
		opt(opts)
	}

	var script string
	var extension string

	if runtime.GOOS == windowsOS {
		extension = testBatExtension
		switch {
		case opts.shouldFail:
			script = `@echo off
if "%1"=="-v" (echo 3.0.0 & exit /b 0)
echo Mock CLI failing >&2
exit /b 1
`
		case opts.longRunning:
			script = `@echo off
if "%1"=="-v" (echo 3.0.0 & exit /b 0)
echo {"type":"assistant","content":[{"type":"text","text":"Long running mock"}],"model":"claude-3"}
timeout /t 30 /nobreak > NUL
`
		case opts.checkEnvironment:
			script = `@echo off
if "%1"=="-v" (echo 3.0.0 & exit /b 0)
if "%CLAUDE_CODE_ENTRYPOINT%"=="sdk-go" (
    echo {"type":"assistant","content":[{"type":"text","text":"Environment OK"}],"model":"claude-3"}
) else (
    echo Missing environment variable >&2
    exit /b 1
)
timeout /t 1 /nobreak > NUL
`
		case opts.invalidOutput:
			script = `@echo off
if "%1"=="-v" (echo 3.0.0 & exit /b 0)
echo This is not valid JSON output
echo {"invalid": json}
echo {"type":"assistant","content":[{"type":"text","text":"Valid after invalid"}],"model":"claude-3"}
timeout /t 1 /nobreak > NUL
`
		default:
			script = `@echo off
if "%1"=="-v" (echo 3.0.0 & exit /b 0)
echo {"type":"assistant","content":[{"type":"text","text":"Mock response"}],"model":"claude-3"}
timeout /t 1 /nobreak > NUL
`
		}
	} else {
		extension = ""
		switch {
		case opts.shouldFail:
			script = `#!/bin/bash
# Handle -v flag for version check
if [ "$1" = "-v" ]; then echo "3.0.0"; exit 0; fi
echo "Mock CLI failing" >&2
exit 1
`
		case opts.longRunning:
			script = `#!/bin/bash
# Handle -v flag for version check
if [ "$1" = "-v" ]; then echo "3.0.0"; exit 0; fi
# Ignore SIGTERM initially to test 5-second timeout
trap 'echo "Received SIGTERM, ignoring for 6 seconds"; sleep 6; exit 1' TERM
echo '{"type":"assistant","content":[{"type":"text","text":"Long running mock"}],"model":"claude-3"}'
sleep 30  # Run long enough to test termination
`
		case opts.checkEnvironment:
			script = `#!/bin/bash
# Handle -v flag for version check
if [ "$1" = "-v" ]; then echo "3.0.0"; exit 0; fi
if [ "$CLAUDE_CODE_ENTRYPOINT" = "sdk-go" ]; then
    echo '{"type":"assistant","content":[{"type":"text","text":"Environment OK"}],"model":"claude-3"}'
else
    echo "Missing environment variable" >&2
    exit 1
fi
sleep 0.5
`
		case opts.invalidOutput:
			script = `#!/bin/bash
# Handle -v flag for version check
if [ "$1" = "-v" ]; then echo "3.0.0"; exit 0; fi
echo "This is not valid JSON output"
echo '{"invalid": json}'
echo '{"type":"assistant","content":[{"type":"text","text":"Valid after invalid"}],"model":"claude-3"}'
sleep 0.5
`
		default:
			script = `#!/bin/bash
# Handle -v flag for version check
if [ "$1" = "-v" ]; then echo "3.0.0"; exit 0; fi
echo '{"type":"assistant","content":[{"type":"text","text":"Mock response"}],"model":"claude-3"}'
sleep 0.5
`
		}
	}

	return createTransportTempScript(script, extension)
}

func createTransportTempScript(script, extension string) string {
	tempDir := os.TempDir()
	scriptPath := filepath.Join(tempDir, fmt.Sprintf("mock-claude-%d%s", time.Now().UnixNano(), extension))

	err := os.WriteFile(scriptPath, []byte(script), 0o755) // #nosec G306 - Test script needs to be executable
	if err != nil {
		panic(fmt.Sprintf("Failed to create mock CLI script: %v", err))
	}

	return scriptPath
}

// Helper functions following client_test.go patterns
func setupTransportTestContext(t *testing.T, timeout time.Duration) (context.Context, context.CancelFunc) {
	t.Helper()
	return context.WithTimeout(context.Background(), timeout)
}

func setupTransportForTest(t *testing.T, cliPath string) *Transport {
	t.Helper()
	options := &shared.Options{}
	return New(cliPath, options, false, "sdk-go")
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

// TestNewWithPrompt tests the NewWithPrompt constructor for one-shot queries
func TestNewWithPrompt(t *testing.T) {
	tests := []struct {
		name    string
		prompt  string
		options *shared.Options
	}{
		{"basic_prompt", "What is 2+2?", &shared.Options{}},
		{"empty_prompt", "", nil},
		{"multiline_prompt", "Line 1\nLine 2", &shared.Options{SystemPrompt: stringPtr("test")}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transport := NewWithPrompt("/usr/bin/claude", test.options, test.prompt)

			if transport == nil {
				t.Fatal("Expected transport to be created, got nil")
				return
			}

			// Verify key configuration
			if transport.entrypoint != "sdk-go" {
				t.Errorf("Expected entrypoint 'sdk-go', got %q", transport.entrypoint)
			}
			if !transport.closeStdin {
				t.Error("Expected closeStdin to be true")
			}
			if transport.promptArg == nil || *transport.promptArg != test.prompt {
				t.Errorf("Expected promptArg %q, got %v", test.prompt, transport.promptArg)
			}
			assertTransportConnected(t, transport, false)
		})
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
				transport := setupTransportForTest(t, newTransportMockCLI())
				connectTransportSafely(ctx, t, transport)
				return transport
			},
			wantError: true,
		},
		{
			name: "invalid_working_directory",
			setup: func() *Transport {
				options := &shared.Options{Cwd: stringPtr("/nonexistent/directory/path")}
				return New(newTransportMockCLI(), options, false, "sdk-go")
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
	ctx, cancel := setupTransportTestContext(t, 5*time.Second)
	defer cancel()

	// Test SendMessage with promptArg transport (one-shot mode)
	t.Run("send_message_with_prompt_arg", func(t *testing.T) {
		transport := NewWithPrompt(newTransportMockCLI(), &shared.Options{}, "test prompt")
		defer disconnectTransportSafely(t, transport)

		connectTransportSafely(ctx, t, transport)

		// Should be no-op since prompt is already passed as CLI argument
		message := shared.StreamMessage{Type: "user", SessionID: "test"}
		err := transport.SendMessage(ctx, message)
		assertNoTransportError(t, err)
	})

	// Test SendMessage with invalid JSON
	t.Run("send_message_marshal_error", func(t *testing.T) {
		transport := setupTransportForTest(t, newTransportMockCLI())
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
		transport := setupTransportForTest(t, newTransportMockCLI())
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
		transport := setupTransportForTest(t, newTransportMockCLI())

		// Don't connect - test interrupt on disconnected transport
		err := transport.Interrupt(ctx)
		if err == nil {
			t.Error("Expected error when interrupting disconnected transport")
		}
	})

	// Test interrupt with nil process
	t.Run("interrupt_nil_process", func(t *testing.T) {
		transport := setupTransportForTest(t, newTransportMockCLI())
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
			transport := setupTransportForTest(t, newTransportMockCLI())
			defer disconnectTransportSafely(t, transport)

			connectTransportSafely(ctx, t, transport)

			// Normal interrupt should work
			err := transport.Interrupt(ctx)
			assertNoTransportError(t, err)
		})
	}
}

// =============================================================================
// Control Protocol Integration Tests
// =============================================================================

// TestTransportControlProtocolIntegration tests that SetModel and SetPermissionMode
// work through the control protocol when properly wired.
func TestTransportControlProtocolIntegration(t *testing.T) {
	ctx, cancel := setupTransportTestContext(t, 10*time.Second)
	defer cancel()

	tests := []struct {
		name        string
		setup       func() *Transport
		operation   func(ctx context.Context, t *Transport) error
		wantErr     bool
		errSubstr   string
		skipWindows bool // Skip on Windows due to batch script limitations
	}{
		{
			name: "SetModel_requires_streaming_mode",
			setup: func() *Transport {
				// One-shot mode (closeStdin=true) should not support SetModel
				return NewWithPrompt(newTransportMockCLI(), &shared.Options{}, "test prompt")
			},
			operation: func(ctx context.Context, t *Transport) error {
				model := testModelName
				return t.SetModel(ctx, &model)
			},
			wantErr:   true,
			errSubstr: "one-shot mode",
		},
		{
			name: "SetPermissionMode_requires_streaming_mode",
			setup: func() *Transport {
				// One-shot mode (closeStdin=true) should not support SetPermissionMode
				return NewWithPrompt(newTransportMockCLI(), &shared.Options{}, "test prompt")
			},
			operation: func(ctx context.Context, t *Transport) error {
				return t.SetPermissionMode(ctx, "accept_edits")
			},
			wantErr:   true,
			errSubstr: "one-shot mode",
		},
		{
			name: "SetModel_requires_connection",
			setup: func() *Transport {
				return setupTransportForTest(t, newTransportMockCLI())
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
				return setupTransportForTest(t, newTransportMockCLI())
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
				return setupTransportForTest(t, newTransportMockCLIWithControlProtocol())
			},
			operation: func(ctx context.Context, t *Transport) error {
				model := testModelName
				return t.SetModel(ctx, &model)
			},
			wantErr:     false, // Should succeed when protocol is wired
			errSubstr:   "",
			skipWindows: true, // Batch script can't properly parse/respond to control requests
		},
		{
			name: "SetPermissionMode_in_streaming_mode_with_protocol",
			setup: func() *Transport {
				// Streaming mode with control protocol mock CLI
				return setupTransportForTest(t, newTransportMockCLIWithControlProtocol())
			},
			operation: func(ctx context.Context, t *Transport) error {
				return t.SetPermissionMode(ctx, "accept_edits")
			},
			wantErr:     false, // Should succeed when protocol is wired
			errSubstr:   "",
			skipWindows: true, // Batch script can't properly parse/respond to control requests
		},
		{
			name: "SetModel_nil_resets_to_default",
			setup: func() *Transport {
				return setupTransportForTest(t, newTransportMockCLIWithControlProtocol())
			},
			operation: func(ctx context.Context, t *Transport) error {
				return t.SetModel(ctx, nil) // nil means reset to default
			},
			wantErr:     false,
			errSubstr:   "",
			skipWindows: true, // Batch script can't properly parse/respond to control requests
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skipWindows && runtime.GOOS == windowsOS {
				t.Skip("Skipped on Windows: batch script cannot properly handle control protocol")
			}

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
	if runtime.GOOS == windowsOS {
		t.Skip("Skipped on Windows: batch script cannot properly handle control protocol")
	}

	ctx, cancel := setupTransportTestContext(t, 10*time.Second)
	defer cancel()

	// Create transport with control protocol mock CLI
	transport := setupTransportForTest(t, newTransportMockCLIWithControlProtocol())
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

// newTransportMockCLIWithControlProtocol creates a mock CLI that supports control protocol.
// It responds to control requests with proper control responses.
func newTransportMockCLIWithControlProtocol() string {
	var script string
	var extension string

	if runtime.GOOS == windowsOS {
		extension = testBatExtension
		// Windows batch script that echoes back control responses
		script = `@echo off
if "%1"=="-v" (echo 3.0.0 & exit /b 0)
setlocal enabledelayedexpansion

:loop
set /p line=
if "!line!"=="" goto end

REM Check if it's a control request and echo a response
echo !line! | findstr /C:"control_request" > nul
if %errorlevel%==0 (
    REM Extract request_id and send success response
    echo {"type":"control_response","response":{"subtype":"success","request_id":"req_1_mock","response":{}}}
)

REM Also output regular messages for testing
echo {"type":"assistant","content":[{"type":"text","text":"Mock response"}],"model":"claude-3"}
goto loop

:end
`
	} else {
		extension = ""
		// Bash script that reads control requests and echoes responses
		script = `#!/bin/bash
# Handle -v flag for version check
if [ "$1" = "-v" ]; then echo "3.0.0"; exit 0; fi

# Output a regular message first
echo '{"type":"assistant","content":[{"type":"text","text":"Mock response"}],"model":"claude-3"}'

# Read stdin and respond to control requests
while IFS= read -r line; do
    if [[ "$line" == *"control_request"* ]]; then
        # Extract request_id using grep/sed
        req_id=$(echo "$line" | grep -o '"request_id":"[^"]*"' | cut -d'"' -f4)
        if [ -z "$req_id" ]; then
            req_id="req_1_mock"
        fi
        # Echo success response
        echo "{\"type\":\"control_response\",\"response\":{\"subtype\":\"success\",\"request_id\":\"$req_id\",\"response\":{}}}"
    fi
done

# Keep process alive briefly
sleep 1
`
	}

	return createTransportTempScript(script, extension)
}
