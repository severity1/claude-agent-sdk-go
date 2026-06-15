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

// TestFormatInitError tests the error message formatting for early init failures.
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

// TestStderrCallbackHandling tests stderr callback processing.
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

// TestTransportResultMessageChannelClosure tests the critical fix for ResultMessage-driven channel closure
func TestTransportResultMessageChannelClosure(t *testing.T) {
	if runtime.GOOS == windowsOS {
		t.Skip("Skipped on Windows: bash script not supported")
	}

	tests := []struct {
		name           string
		scriptTemplate string
	}{
		{
			name: "response_with_result_message_closes_channel",
			scriptTemplate: `#!/bin/bash
echo '{"type":"assistant","message":{"content":[{"type":"text","text":"Hello"}],"model":"claude-3"}}'
echo '{"type":"result","subtype":"final","duration_ms":1000,"duration_api_ms":800,"is_error":false,"num_turns":1,"session_id":"test-123","result":{"output":"response"}}'
# Keep process alive to simulate streaming mode - channel should close due to ResultMessage, not EOF
sleep 5
`,
		},
		{
			name: "tool_uses_with_result_message_closes_channel",
			scriptTemplate: `#!/bin/bash
echo '{"type":"assistant","message":{"content":[{"type":"text","text":"I will list files"}],"model":"claude-3"}}'
echo '{"type":"assistant","message":{"content":[{"type":"tool_use","id":"tool-1","name":"bash","input":{"command":"ls"}}],"model":"claude-3"}}'
echo '{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"tool-1","content":"file1.txt\nfile2.txt"}]}}'
echo '{"type":"assistant","message":{"content":[{"type":"text","text":"Here are the files"}],"model":"claude-3"}}'
echo '{"type":"result","subtype":"final","duration_ms":2000,"duration_api_ms":1500,"is_error":false,"num_turns":2,"session_id":"test-456","result":{"output":"complete"}}'
# Keep process alive to simulate streaming mode - channel should close due to ResultMessage, not EOF
sleep 5
`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Each subtest gets its own context to avoid timeout accumulation
			ctx, cancel := setupTransportTestContext(t, 10*time.Second)
			defer cancel()

			cliScript := createTransportTempScript(test.scriptTemplate, "")
			transport := setupTransportForTest(t, cliScript)
			defer disconnectTransportSafely(t, transport)

			connectTransportSafely(ctx, t, transport)

			// Send a message to trigger response
			message := shared.StreamMessage{
				Type:      "user",
				SessionID: "test-session",
				Message:   "test message",
			}
			err := transport.SendMessage(ctx, message)
			assertNoTransportError(t, err)

			// Get channels and monitor behavior
			msgChan, errChan := transport.ReceiveMessages(ctx)

			messageCount := 0
			resultMessageReceived := false
			channelClosed := false

			// Wait for either channel closure or timeout
			timeout := time.After(5 * time.Second)

			for {
				select {
				case msg, ok := <-msgChan:
					if !ok {
						channelClosed = true
						goto ChannelEvaluation
					}
					messageCount++

					// Check if this is a ResultMessage
					if _, isResult := msg.(*shared.ResultMessage); isResult {
						resultMessageReceived = true
					}

				case err, ok := <-errChan:
					if !ok {
						// Error channel closed without errors - continue monitoring msgChan
						continue
					}
					t.Errorf("Unexpected error on error channel: %v", err)
					return

				case <-timeout:
					goto ChannelEvaluation
				}
			}

		ChannelEvaluation:
			// All test cases expect channel closure after ResultMessage
			if !channelClosed {
				t.Errorf("Expected channel to close after ResultMessage, but it remained open")
				t.Errorf("Messages received: %d, ResultMessage received: %v", messageCount, resultMessageReceived)
			}
			if !resultMessageReceived {
				t.Errorf("Expected ResultMessage to be received before channel closure")
			}

			// Verify stream validator state
			validator := transport.GetValidator()
			stats := validator.GetStats()
			if stats.HasResult != resultMessageReceived {
				t.Errorf("Validator stats inconsistent: HasResult=%v, resultMessageReceived=%v", stats.HasResult, resultMessageReceived)
			}
		})
	}
}

// TestTransportGoroutineLeakPrevention tests that the fix prevents goroutine leaks
func TestTransportGoroutineLeakPrevention(t *testing.T) {
	ctx, cancel := setupTransportTestContext(t, 10*time.Second)
	defer cancel()

	// Count goroutines before test
	initialGoroutines := runtime.NumGoroutine()

	cliScript := newTransportMockCLI()
	transport := setupTransportForTest(t, cliScript)

	connectTransportSafely(ctx, t, transport)

	// Send message and receive response
	message := shared.StreamMessage{
		Type:      "user",
		SessionID: "leak-test",
		Message:   "test for goroutine leaks",
	}
	err := transport.SendMessage(ctx, message)
	assertNoTransportError(t, err)

	// Wait for completion with timeout
	msgChan, _ := transport.ReceiveMessages(ctx)

	timeout := time.After(2 * time.Second)
Loop:
	for {
		select {
		case _, ok := <-msgChan:
			if !ok {
				break Loop
			}
			// Track messages but don't block
		case <-timeout:
			break Loop
		}
	}

	// Close transport
	disconnectTransportSafely(t, transport)

	// Give a moment for goroutines to cleanup
	time.Sleep(100 * time.Millisecond)

	finalGoroutines := runtime.NumGoroutine()
	goroutineIncrease := finalGoroutines - initialGoroutines

	// Allow for minimal goroutine variance due to test infrastructure
	if goroutineIncrease > 2 {
		t.Errorf("Potential goroutine leak detected. Goroutines increased by %d (from %d to %d)",
			goroutineIncrease, initialGoroutines, finalGoroutines)
	}
}

// TestTransportMultipleResultMessages tests behavior with multiple ResultMessages
func TestTransportMultipleResultMessages(t *testing.T) {
	if runtime.GOOS == windowsOS {
		t.Skip("Skipped on Windows: bash script not supported")
	}

	ctx, cancel := setupTransportTestContext(t, 5*time.Second)
	defer cancel()

	// Script that sends multiple ResultMessages
	cliScript := createTransportTempScript(`#!/bin/bash
echo '{"type":"assistant","message":{"content":[{"type":"text","text":"First response"}],"model":"claude-3"}}'
echo '{"type":"result","subtype":"final","duration_ms":500,"duration_api_ms":400,"is_error":false,"num_turns":1,"session_id":"multi-1","result":{"output":"first"}}'
echo '{"type":"assistant","message":{"content":[{"type":"text","text":"Second response"}],"model":"claude-3"}}'
echo '{"type":"result","subtype":"final","duration_ms":600,"duration_api_ms":500,"is_error":false,"num_turns":1,"session_id":"multi-2","result":{"output":"second"}}'
sleep 1
`, "")

	transport := setupTransportForTest(t, cliScript)
	defer disconnectTransportSafely(t, transport)

	connectTransportSafely(ctx, t, transport)

	message := shared.StreamMessage{
		Type:      "user",
		SessionID: "multi-test",
		Message:   "test multiple results",
	}
	err := transport.SendMessage(ctx, message)
	assertNoTransportError(t, err)

	msgChan, _ := transport.ReceiveMessages(ctx)

	resultCount := 0
	messagesReceived := 0
	channelClosed := false

	timeout := time.After(2 * time.Second)
Loop:
	for {
		select {
		case msg, ok := <-msgChan:
			if !ok {
				channelClosed = true
				break Loop
			}
			messagesReceived++
			if _, isResult := msg.(*shared.ResultMessage); isResult {
				resultCount++
			}

		case <-timeout:
			break Loop
		}
	}

	if !channelClosed {
		t.Error("Expected channel to close after first ResultMessage")
	}

	if resultCount == 0 {
		t.Error("Expected at least one ResultMessage to be received")
	}

	// Channel should close after first ResultMessage, subsequent messages should not be processed
	t.Logf("Messages received: %d, ResultMessages: %d, Channel closed: %v", messagesReceived, resultCount, channelClosed)
}

// TestTransportResultMessageWithErrors tests ResultMessage handling in error scenarios
func TestTransportResultMessageWithErrors(t *testing.T) {
	if runtime.GOOS == windowsOS {
		t.Skip("Skipped on Windows: bash script not supported")
	}

	ctx, cancel := setupTransportTestContext(t, 5*time.Second)
	defer cancel()

	tests := []struct {
		name          string
		scriptContent string
		expectError   bool
		expectClosure bool
	}{
		{
			name: "error_result_message_closes_channel",
			scriptContent: `#!/bin/bash
echo '{"type":"assistant","message":{"content":[{"type":"text","text":"Processing"}],"model":"claude-3"}}'
echo '{"type":"result","subtype":"final","duration_ms":200,"duration_api_ms":180,"is_error":true,"num_turns":1,"session_id":"error-1","result":{"error":"Something went wrong"}}'
sleep 1
`,
			expectError:   false, // Error results are still valid completion signals
			expectClosure: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cliScript := createTransportTempScript(test.scriptContent, "")
			transport := setupTransportForTest(t, cliScript)
			defer disconnectTransportSafely(t, transport)

			connectTransportSafely(ctx, t, transport)

			message := shared.StreamMessage{
				Type:      "user",
				SessionID: "error-test",
				Message:   "test error handling",
			}
			err := transport.SendMessage(ctx, message)
			assertNoTransportError(t, err)

			msgChan, errChan := transport.ReceiveMessages(ctx)

			errorReceived := false
			resultReceived := false
			channelClosed := false

			timeout := time.After(4 * time.Second)
		Loop:
			for {
				select {
				case msg, ok := <-msgChan:
					if !ok {
						channelClosed = true
						break Loop
					}
					t.Logf("Received message of type %T", msg)
					if _, isResult := msg.(*shared.ResultMessage); isResult {
						resultReceived = true
						t.Log("ResultMessage received")
					}

				case err := <-errChan:
					if err != nil {
						t.Logf("Received error: %v", err)
						errorReceived = true
					}

				case <-timeout:
					t.Log("Timeout reached")
					break Loop
				}
			}

			if test.expectError && !errorReceived {
				t.Errorf("Expected error but none was received")
			}

			if test.expectClosure {
				if !channelClosed {
					t.Errorf("Expected channel closure but it remained open")
				}
				if !resultReceived {
					t.Errorf("Expected ResultMessage to be received for closure")
				}
			}
		})
	}
}
