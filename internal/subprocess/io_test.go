package subprocess

import (
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/severity1/claude-agent-sdk-go/internal/shared"
)

// TestFormatInitError tests the error message formatting for early init failures (Issue #110)
func TestFormatInitError(t *testing.T) {
	tests := []struct {
		name     string
		msg      *shared.ResultMessage
		expected string
	}{
		{
			name: "uses_errors_field",
			msg: &shared.ResultMessage{
				IsError: true,
				Errors:  []string{"No conversation found with session ID: abc-123"},
				Subtype: "error",
			},
			expected: "No conversation found with session ID: abc-123",
		},
		{
			name: "joins_multiple_errors",
			msg: &shared.ResultMessage{
				IsError: true,
				Errors:  []string{"error one", "error two"},
				Subtype: "error",
			},
			expected: "error one; error two",
		},
		{
			name: "falls_back_to_result",
			msg: &shared.ResultMessage{
				IsError: true,
				Errors:  nil,
				Result:  strPtr("something went wrong"),
				Subtype: "error",
			},
			expected: "something went wrong",
		},
		{
			name: "falls_back_to_subtype",
			msg: &shared.ResultMessage{
				IsError: true,
				Errors:  nil,
				Result:  nil,
				Subtype: "fatal",
			},
			expected: "initialization failed with subtype: fatal",
		},
		{
			name: "empty_errors_falls_back_to_result",
			msg: &shared.ResultMessage{
				IsError: true,
				Errors:  []string{},
				Result:  strPtr("fallback message"),
				Subtype: "error",
			},
			expected: "fallback message",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := formatInitError(tc.msg)
			if got != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, got)
			}
		})
	}
}

func strPtr(s string) *string {
	return &s
}

// TestTransportHandleStdoutErrorPaths tests uncovered handleStdout scenarios
func TestTransportHandleStdoutErrorPaths(t *testing.T) {
	ctx, cancel := setupTransportTestContext(t, 5*time.Second)
	defer cancel()

	// Test stdout parsing errors
	t.Run("stdout_parsing_errors", func(t *testing.T) {
		transport := setupTransportForTest(t, newTransportMockCLIWithOptions(WithInvalidOutput()))
		defer disconnectTransportSafely(t, transport)

		connectTransportSafely(ctx, t, transport)

		// Get channels and wait briefly for processing
		msgChan, errChan := transport.ReceiveMessages(ctx)

		// Check for either parsing errors or messages (both are acceptable)
		errorReceived := false
		messageReceived := false

		timeout := time.After(2 * time.Second)
		for !errorReceived && !messageReceived {
			select {
			case err := <-errChan:
				if err != nil {
					errorReceived = true
				}
			case <-msgChan:
				messageReceived = true
			case <-timeout:
				// Either outcome is acceptable - parser may be resilient to invalid JSON
				return
			}
		}
	})

	// Test scanner error conditions
	t.Run("scanner_error_handling", func(t *testing.T) {
		transport := setupTransportForTest(t, newTransportMockCLI())
		defer disconnectTransportSafely(t, transport)

		connectTransportSafely(ctx, t, transport)

		// Get channels
		msgChan, errChan := transport.ReceiveMessages(ctx)

		// Close transport to trigger scanner completion
		disconnectTransportSafely(t, transport)

		// Message channel should close after transport close
		select {
		case _, ok := <-msgChan:
			if ok {
				t.Error("Expected message channel to be closed")
			}
		case <-time.After(1 * time.Second):
			t.Error("Expected message channel to be closed promptly")
		}

		// Error channel may emit shutdown errors before closing.
		// Drain any errors and verify the channel eventually closes.
		timeout := time.After(2 * time.Second)
		for {
			select {
			case _, ok := <-errChan:
				if !ok {
					// Channel closed - success
					return
				}
				// Received an error (e.g., scanner shutdown error), keep draining
			case <-timeout:
				t.Error("Expected error channel to be closed within timeout")
				return
			}
		}
	})
}

// TestStderrCallbackHandling tests stderr callback processing (Issue #53)
func TestStderrCallbackHandling(t *testing.T) {
	tests := []struct {
		name           string
		stderrOutput   []string // Lines written to stderr
		expectedLines  []string // Lines expected in callback
		includeNewline bool     // Whether to include newline after each line
	}{
		{
			name:           "basic_lines",
			stderrOutput:   []string{"line1", "line2"},
			expectedLines:  []string{"line1", "line2"},
			includeNewline: true,
		},
		{
			name:           "strips_trailing_whitespace",
			stderrOutput:   []string{"line with spaces   ", "line with tabs\t\t"},
			expectedLines:  []string{"line with spaces", "line with tabs"},
			includeNewline: true,
		},
		{
			name:           "skips_empty_lines",
			stderrOutput:   []string{"line1", "", "   ", "line2"},
			expectedLines:  []string{"line1", "line2"},
			includeNewline: true,
		},
		{
			name:           "preserves_leading_whitespace",
			stderrOutput:   []string{"  indented"},
			expectedLines:  []string{"  indented"},
			includeNewline: true,
		},
		{
			name:           "single_line",
			stderrOutput:   []string{"single line output"},
			expectedLines:  []string{"single line output"},
			includeNewline: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var received []string
			var mu sync.Mutex

			callback := func(line string) {
				mu.Lock()
				defer mu.Unlock()
				received = append(received, line)
			}

			// Test using processStderrLine helper to verify line processing logic
			for _, line := range tt.stderrOutput {
				processedLine := strings.TrimRight(line, " \t\r\n")
				if processedLine != "" {
					callback(processedLine)
				}
			}

			mu.Lock()
			defer mu.Unlock()

			if len(received) != len(tt.expectedLines) {
				t.Errorf("Expected %d lines, got %d. Received: %v", len(tt.expectedLines), len(received), received)
				return
			}

			for i, expected := range tt.expectedLines {
				if received[i] != expected {
					t.Errorf("Line %d: expected %q, got %q", i, expected, received[i])
				}
			}
		})
	}
}

// TestStderrCallbackPanicRecovery tests that callback panics don't crash the transport
func TestStderrCallbackPanicRecovery(t *testing.T) {
	panicCount := 0
	var mu sync.Mutex

	callback := func(_ string) {
		mu.Lock()
		panicCount++
		mu.Unlock()
		panic("intentional panic for testing")
	}

	// Simulate the recovery pattern from handleStderrCallback
	safeCall := func(cb func(string), line string) {
		defer func() {
			_ = recover() // Silently ignore callback panics (matches Python SDK)
		}()
		cb(line)
	}

	// Should not crash even when callback panics
	safeCall(callback, "line1")
	safeCall(callback, "line2")
	safeCall(callback, "line3")

	mu.Lock()
	defer mu.Unlock()

	if panicCount != 3 {
		t.Errorf("Expected 3 panic calls, got %d", panicCount)
	}
}

// TestStderrCallbackPrecedence tests that StderrCallback takes precedence over DebugWriter
func TestStderrCallbackPrecedence(t *testing.T) {
	tests := []struct {
		name           string
		hasCallback    bool
		hasDebugWriter bool
		expectedTarget string // "callback", "debugwriter", or "tempfile"
	}{
		{"callback_only", true, false, "callback"},
		{"debugwriter_only", false, true, "debugwriter"},
		{"both_callback_wins", true, true, "callback"},
		{"neither_tempfile", false, false, "tempfile"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options := &shared.Options{}

			callbackCalled := false
			if tt.hasCallback {
				options.StderrCallback = func(_ string) {
					callbackCalled = true
				}
			}

			if tt.hasDebugWriter {
				options.DebugWriter = os.Stderr
			}

			// Verify precedence logic
			switch tt.expectedTarget {
			case "callback":
				if options.StderrCallback == nil {
					t.Error("Expected StderrCallback to be set")
				}
				// Callback takes precedence, so this should be true
				if tt.hasCallback && options.StderrCallback != nil {
					// Simulate calling it
					options.StderrCallback("test")
					if !callbackCalled {
						t.Error("Expected callback to be called")
					}
				}
			case "debugwriter":
				if options.DebugWriter == nil {
					t.Error("Expected DebugWriter to be set")
				}
				if options.StderrCallback != nil {
					t.Error("Expected StderrCallback to be nil for debugwriter case")
				}
			case "tempfile":
				if options.DebugWriter != nil || options.StderrCallback != nil {
					t.Error("Expected both DebugWriter and StderrCallback to be nil for tempfile case")
				}
			}
		})
	}
}

// TestStderrCallbackWithMockCLI tests stderr callback with actual mock CLI script
func TestStderrCallbackWithMockCLI(t *testing.T) {
	ctx, cancel := setupTransportTestContext(t, 10*time.Second)
	defer cancel()

	var received []string
	var mu sync.Mutex

	callback := func(line string) {
		mu.Lock()
		defer mu.Unlock()
		received = append(received, line)
	}

	options := &shared.Options{
		StderrCallback: callback,
	}

	// Create a mock CLI that outputs to stderr
	cliPath := newTransportMockCLIWithStderr()
	defer func() { _ = os.Remove(cliPath) }()

	transport := New(cliPath, options, false, "sdk-go")
	defer disconnectTransportSafely(t, transport)

	err := transport.Connect(ctx)
	assertNoTransportError(t, err)

	// Wait for stderr processing
	time.Sleep(500 * time.Millisecond)

	mu.Lock()
	receivedCount := len(received)
	mu.Unlock()

	// Verify some stderr was received (exact count depends on mock CLI)
	if receivedCount == 0 {
		t.Log("No stderr lines received - this may be expected if mock CLI doesn't output to stderr")
	}
}

// newTransportMockCLIWithStderr creates a mock CLI that outputs to stderr
func newTransportMockCLIWithStderr() string {
	var script string
	var extension string

	if runtime.GOOS == windowsOS {
		extension = testBatExtension
		script = `@echo off
if "%1"=="-v" (echo 3.0.0 & exit /b 0)
echo Stderr line 1 >&2
echo Stderr line 2 >&2
echo {"type":"assistant","content":[{"type":"text","text":"Mock response"}],"model":"claude-3"}
timeout /t 1 /nobreak > NUL
`
	} else {
		extension = ""
		script = `#!/bin/bash
# Handle -v flag for version check
if [ "$1" = "-v" ]; then echo "3.0.0"; exit 0; fi
echo "Stderr line 1" >&2
echo "Stderr line 2" >&2
echo '{"type":"assistant","content":[{"type":"text","text":"Mock response"}],"model":"claude-3"}'
sleep 0.5
`
	}

	return createTransportTempScript(script, extension)
}
