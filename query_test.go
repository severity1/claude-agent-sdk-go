package claudecode

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// contextKey is a custom type for context keys to avoid collisions
type contextKey string

const cancelKey contextKey = "cancel"

// TestQueryBasicExecution tests simple query functionality
// Python Reference: test_client.py::TestQueryFunction::test_query_single_prompt
func TestQueryBasicExecution(t *testing.T) {
	ctx, cancel := setupQueryTestContext(t, 10*time.Second)
	defer cancel()

	transport := newQueryMockTransport(WithQueryAssistantResponse("4"))

	iter, err := QueryWithTransport(ctx, "What is 2+2?", transport)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	defer func() { _ = iter.Close() }()

	messages := collectQueryMessages(ctx, t, iter)

	if len(messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(messages))
	}

	assistantMsg := assertQueryAssistantMessage(t, messages[0])
	assertQueryTextContent(t, assistantMsg, "4")
	assertQueryMessageModel(t, assistantMsg, "claude-opus-4-1-20250805")
}

// TestQueryWithOptions tests query configuration options
// Python Reference: test_client.py::TestQueryFunction::test_query_with_options
func TestQueryWithOptions(t *testing.T) {
	ctx, cancel := setupQueryTestContext(t, 10*time.Second)
	defer cancel()

	transport := newQueryMockTransport(WithQueryAssistantResponse("Hello!"))

	options := []Option{
		WithAllowedTools("Read", "Write"),
		WithSystemPrompt("You are helpful"),
		WithPermissionMode("acceptEdits"),
		WithMaxTurns(5),
	}

	iter, err := QueryWithTransport(ctx, "Hi", transport, options...)
	if err != nil {
		t.Fatalf("Query with options failed: %v", err)
	}
	defer func() { _ = iter.Close() }()

	messages := collectQueryMessages(ctx, t, iter)

	if len(messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(messages))
	}

	assistantMsg := assertQueryAssistantMessage(t, messages[0])
	assertQueryTextContent(t, assistantMsg, "Hello!")

	// Verify options were applied to transport (mock would track this)
	assertQueryTransportReceivedOptions(t, transport, true)
}

// TestQueryResponseProcessing tests message processing and content blocks
func TestQueryResponseProcessing(t *testing.T) {
	ctx, cancel := setupQueryTestContext(t, 10*time.Second)
	defer cancel()

	// Test complex assistant message with multiple content blocks
	transport := newQueryMockTransport(
		WithQueryMultipleMessages([]*AssistantMessage{
			{
				Content: []ContentBlock{
					&TextBlock{Text: "Assistant response"},
					&ThinkingBlock{Thinking: "Let me think...", Signature: "assistant"},
				},
				Model: "claude-opus-4-1-20250805",
			},
		}),
	)

	iter, err := QueryWithTransport(ctx, "Test query", transport)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	defer func() { _ = iter.Close() }()

	messages := collectQueryMessages(ctx, t, iter)

	if len(messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(messages))
	}

	assistantMsg := assertQueryAssistantMessage(t, messages[0])

	// Verify message has multiple content blocks
	if len(assistantMsg.Content) != 2 {
		t.Fatalf("Expected 2 content blocks, got %d", len(assistantMsg.Content))
	}

	// Verify first content block is TextBlock
	textBlock, ok := assistantMsg.Content[0].(*TextBlock)
	if !ok {
		t.Fatalf("Expected first content block to be TextBlock, got %T", assistantMsg.Content[0])
	}
	if textBlock.Text != "Assistant response" {
		t.Errorf("Expected text 'Assistant response', got '%s'", textBlock.Text)
	}

	// Verify second content block is ThinkingBlock
	thinkingBlock, ok := assistantMsg.Content[1].(*ThinkingBlock)
	if !ok {
		t.Fatalf("Expected second content block to be ThinkingBlock, got %T", assistantMsg.Content[1])
	}
	if thinkingBlock.Thinking != "Let me think..." {
		t.Errorf("Expected thinking 'Let me think...', got '%s'", thinkingBlock.Thinking)
	}
}

// TestQueryMultipleMessageTypes tests system and result message processing
func TestQueryMultipleMessageTypes(t *testing.T) {
	ctx, cancel := setupQueryTestContext(t, 10*time.Second)
	defer cancel()

	// Test transport that sends different message types
	transport := newQueryMockTransport(
		WithQueryAssistantResponse("Assistant response"),
		WithQuerySystemMessage("tool_use", map[string]any{"tool": "Read", "file": "test.txt"}),
		WithQueryResultMessage(false, 2500, 3),
	)

	iter, err := QueryWithTransport(ctx, "Test multiple message types", transport)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	defer func() { _ = iter.Close() }()

	var assistantMessages []*AssistantMessage
	var systemMessages []*SystemMessage
	var resultMessages []*ResultMessage

	messageCount := 0
	for {
		msg, err := iter.Next(ctx)
		if err != nil {
			if err == ErrNoMoreMessages {
				break
			}
			t.Fatalf("Iterator error: %v", err)
		}

		if msg != nil {
			messageCount++
			switch msg.Type() {
			case MessageTypeAssistant:
				if assistantMsg, ok := msg.(*AssistantMessage); ok {
					assistantMessages = append(assistantMessages, assistantMsg)
				}
			case MessageTypeSystem:
				if systemMsg, ok := msg.(*SystemMessage); ok {
					systemMessages = append(systemMessages, systemMsg)
				}
			case MessageTypeResult:
				if resultMsg, ok := msg.(*ResultMessage); ok {
					resultMessages = append(resultMessages, resultMsg)
				}
			default:
				t.Errorf("Unexpected message type: %s (actual type: %T)", msg.Type(), msg)
			}
		}
	}

	// Verify we got all expected message types
	if len(assistantMessages) != 1 {
		t.Errorf("Expected 1 assistant message, got %d", len(assistantMessages))
	}

	if len(systemMessages) != 1 {
		t.Errorf("Expected 1 system message, got %d", len(systemMessages))
	}

	if len(resultMessages) != 1 {
		t.Errorf("Expected 1 result message, got %d", len(resultMessages))
	}

	// Verify assistant message content (only if type assertion succeeded)
	if len(assistantMessages) > 0 && assistantMessages[0] != nil {
		assertQueryTextContent(t, assistantMessages[0], "Assistant response")
	}

	// Verify system message content (only if type assertion succeeded)
	if len(systemMessages) > 0 && systemMessages[0] != nil {
		if systemMessages[0].Subtype != "tool_use" {
			t.Errorf("Expected system message subtype 'tool_use', got '%s'", systemMessages[0].Subtype)
		}
		if tool, ok := systemMessages[0].Data["tool"].(string); !ok || tool != "Read" {
			t.Errorf("Expected system message data tool 'Read', got %v", systemMessages[0].Data["tool"])
		}
	}

	// Verify result message content (only if type assertion succeeded)
	if len(resultMessages) > 0 && resultMessages[0] != nil {
		if resultMessages[0].IsError != false {
			t.Errorf("Expected result message IsError=false, got %v", resultMessages[0].IsError)
		}
		if resultMessages[0].NumTurns != 3 {
			t.Errorf("Expected result message NumTurns=3, got %d", resultMessages[0].NumTurns)
		}
		if resultMessages[0].SessionID != "test-session" {
			t.Errorf("Expected result message SessionID='test-session', got '%s'", resultMessages[0].SessionID)
		}
	}
}

// TestQueryErrorHandling tests error scenarios with table-driven approach
func TestQueryErrorHandling(t *testing.T) {
	ctx, cancel := setupQueryTestContext(t, 10*time.Second)
	defer cancel()

	tests := []struct {
		name           string
		setupTransport func() *queryMockTransport
		operation      func(context.Context, *queryMockTransport) error
		wantErr        bool
		errorContains  string
	}{
		{
			name: "nil_transport",
			setupTransport: func() *queryMockTransport {
				return nil
			},
			operation: func(ctx context.Context, _ *queryMockTransport) error {
				_, err := QueryWithTransport(ctx, "test", nil)
				return err
			},
			wantErr:       true,
			errorContains: "transport is required",
		},
		{
			name: "connection_error",
			setupTransport: func() *queryMockTransport {
				return newQueryMockTransport(WithQueryConnectError(fmt.Errorf("connection failed")))
			},
			operation: func(ctx context.Context, transport *queryMockTransport) error {
				iter, err := QueryWithTransport(ctx, "test", transport)
				if err != nil {
					return err
				}
				defer func() { _ = iter.Close() }()
				_, err = iter.Next(ctx)
				return err
			},
			wantErr:       true,
			errorContains: "failed to connect transport",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transport := test.setupTransport()
			err := test.operation(ctx, transport)
			assertQueryError(t, err, test.wantErr, test.errorContains)
		})
	}
}

// TestQueryContextCancellation tests context cancellation scenarios
func TestQueryContextCancellation(t *testing.T) {
	tests := []struct {
		name           string
		setupContext   func() (context.Context, context.CancelFunc)
		setupTransport func() *queryMockTransport
		operation      func(context.Context, *queryMockTransport) error
		expectedError  error
	}{
		{
			name: "timeout_during_query",
			setupContext: func() (context.Context, context.CancelFunc) {
				return context.WithTimeout(context.Background(), 50*time.Millisecond)
			},
			setupTransport: func() *queryMockTransport {
				return newQueryMockTransport(WithQueryDelay(200 * time.Millisecond))
			},
			operation: func(ctx context.Context, transport *queryMockTransport) error {
				iter, err := QueryWithTransport(ctx, "slow query", transport)
				if err != nil {
					return err
				}
				defer func() { _ = iter.Close() }()
				_, err = iter.Next(ctx)
				return err
			},
			expectedError: context.DeadlineExceeded,
		},
		{
			name: "manual_cancellation",
			setupContext: func() (context.Context, context.CancelFunc) {
				return context.WithCancel(context.Background())
			},
			setupTransport: func() *queryMockTransport {
				return newQueryMockTransport(WithQueryDelay(500 * time.Millisecond))
			},
			operation: func(ctx context.Context, transport *queryMockTransport) error {
				// Cancel after 100ms
				go func() {
					time.Sleep(100 * time.Millisecond)
					if cancel, ok := ctx.Value(cancelKey).(context.CancelFunc); ok {
						cancel()
					}
				}()

				iter, err := QueryWithTransport(ctx, "slow query", transport)
				if err != nil {
					return err
				}
				defer func() { _ = iter.Close() }()
				_, err = iter.Next(ctx)
				return err
			},
			expectedError: context.Canceled,
		},
		{
			name: "immediate_cancellation",
			setupContext: func() (context.Context, context.CancelFunc) {
				// Create a context that is already canceled deterministically
				ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Hour))
				return ctx, cancel
			},
			setupTransport: func() *queryMockTransport {
				// Add a message to ensure we're testing context cancellation, not empty channel race
				return newQueryMockTransport(WithQueryAssistantResponse("test response"))
			},
			operation: func(ctx context.Context, transport *queryMockTransport) error {
				iter, err := QueryWithTransport(ctx, "test", transport)
				if err != nil {
					return err
				}
				defer func() { _ = iter.Close() }()
				_, err = iter.Next(ctx)
				return err
			},
			expectedError: context.DeadlineExceeded,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := test.setupContext()
			if test.name == "manual_cancellation" {
				ctx = context.WithValue(ctx, cancelKey, cancel)
			} else {
				defer cancel()
			}

			transport := test.setupTransport()
			err := test.operation(ctx, transport)

			if err == nil {
				t.Fatal("Expected context cancellation error")
			}

			if !isQueryContextError(err, test.expectedError) {
				t.Errorf("Expected %v (or wrapped), got %v", test.expectedError, err)
			}
		})
	}
}

// TestQueryPublicAPI tests the actual Query() function that users call
// This uses QueryWithTransport to test the Query logic without CLI dependency
func TestQueryPublicAPI(t *testing.T) {
	ctx, cancel := setupQueryTestContext(t, 10*time.Second)
	defer cancel()

	tests := []struct {
		name            string
		prompt          string
		options         []Option
		setupTransport  func() Transport
		expectError     bool
		errorContains   string
		validateResults func(t *testing.T, iter MessageIterator)
	}{
		{
			name:    "basic_query_success",
			prompt:  "What is 2+2?",
			options: []Option{},
			setupTransport: func() Transport {
				return newQueryMockTransport(WithQueryAssistantResponse("4"))
			},
			expectError: false,
			validateResults: func(t *testing.T, iter MessageIterator) {
				t.Helper()
				messages := collectQueryMessages(ctx, t, iter)
				if len(messages) != 1 {
					t.Fatalf("Expected 1 message, got %d", len(messages))
				}
				assistantMsg := assertQueryAssistantMessage(t, messages[0])
				assertQueryTextContent(t, assistantMsg, "4")
			},
		},
		{
			name:   "query_with_system_prompt",
			prompt: "Hello",
			options: []Option{
				WithSystemPrompt("You are helpful"),
				WithModel("claude-sonnet-3-5-20241022"),
			},
			setupTransport: func() Transport {
				return newQueryMockTransport(WithQueryAssistantResponse("Hi there!"))
			},
			expectError: false,
			validateResults: func(t *testing.T, iter MessageIterator) {
				t.Helper()
				messages := collectQueryMessages(ctx, t, iter)
				if len(messages) != 1 {
					t.Fatalf("Expected 1 message, got %d", len(messages))
				}
				assertQueryAssistantMessage(t, messages[0])
			},
		},
		{
			name:    "query_with_empty_prompt",
			prompt:  "",
			options: []Option{},
			setupTransport: func() Transport {
				return newQueryMockTransport(WithQueryAssistantResponse("Empty prompt handled"))
			},
			expectError: false,
			validateResults: func(t *testing.T, iter MessageIterator) {
				t.Helper()
				messages := collectQueryMessages(ctx, t, iter)
				if len(messages) != 1 {
					t.Fatalf("Expected 1 message, got %d", len(messages))
				}
				assertQueryAssistantMessage(t, messages[0])
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transport := test.setupTransport()

			// Test Query behavior using QueryWithTransport for testability
			// This exercises the Query API logic without CLI dependency
			iter, err := QueryWithTransport(ctx, test.prompt, transport, test.options...)

			if test.expectError {
				if err == nil {
					t.Fatal("Expected error, got none")
				}
				if test.errorContains != "" && !strings.Contains(err.Error(), test.errorContains) {
					t.Errorf("Expected error to contain %q, got: %v", test.errorContains, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			defer func() { _ = iter.Close() }()

			if test.validateResults != nil {
				test.validateResults(t, iter)
			}
		})
	}
}

// TestCreateQueryTransport tests the transport creation function
func TestCreateQueryTransport(t *testing.T) {
	tests := []struct {
		name        string
		prompt      string
		options     *Options
		setupMock   func(t *testing.T) (cleanup func())
		expectError bool
		errorMsg    string
	}{
		{
			name:        "cli_not_found_error",
			prompt:      "test prompt",
			options:     NewOptions(),
			setupMock:   setupIsolatedEnvironment, // Isolate PATH to ensure CLI is not found
			expectError: true,
			errorMsg:    "Claude Code requires Node.js", // Should get Node.js not found error
		},
		{
			name:   "cli_not_found_with_options",
			prompt: "test with options",
			options: NewOptions(
				WithSystemPrompt("Test system prompt"),
				WithModel("claude-sonnet-3-5-20241022"),
			),
			setupMock:   setupIsolatedEnvironment,
			expectError: true,
			errorMsg:    "Claude Code requires Node.js",
		},
		{
			name:        "empty_prompt_cli_not_found",
			prompt:      "",
			options:     NewOptions(),
			setupMock:   setupIsolatedEnvironment,
			expectError: true,
			errorMsg:    "Claude Code requires Node.js",
		},
		{
			name:        "nil_options_cli_not_found",
			prompt:      "test prompt",
			options:     nil,
			setupMock:   setupIsolatedEnvironment,
			expectError: true,
			errorMsg:    "Claude Code requires Node.js",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cleanup := test.setupMock(t)
			defer cleanup()

			// Call createQueryTransport directly - this will exercise the real function
			transport, err := createQueryTransport(test.prompt, test.options)

			if test.expectError {
				if err == nil {
					t.Fatal("Expected error, got none")
				}
				if !strings.Contains(err.Error(), test.errorMsg) {
					t.Errorf("Expected error to contain %q, got: %v", test.errorMsg, err)
				}
				// Transport should be nil on error
				if transport != nil {
					t.Error("Expected nil transport on error, got non-nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			// Verify transport is not nil on success
			if transport == nil {
				t.Fatal("Expected transport to be created, got nil")
			}

			// Clean up transport if created
			if transport != nil {
				_ = transport.Close()
			}
		})
	}
}

// TestQuery tests the public Query function behavior using QueryWithTransport for testability
func TestQuery(t *testing.T) {
	ctx, cancel := setupQueryTestContext(t, 10*time.Second)
	defer cancel()

	tests := []struct {
		name            string
		prompt          string
		options         []Option
		setupTransport  func() Transport
		expectError     bool
		errorContains   string
		validateResults func(t *testing.T, iter MessageIterator)
	}{
		{
			name:    "successful_query",
			prompt:  "What is 2+2?",
			options: []Option{WithModel("claude-sonnet-3-5-20241022")},
			setupTransport: func() Transport {
				return newQueryMockTransport(WithQueryAssistantResponse("4"))
			},
			expectError: false,
			validateResults: func(t *testing.T, iter MessageIterator) {
				t.Helper()
				messages := collectQueryMessages(ctx, t, iter)
				if len(messages) != 1 {
					t.Fatalf("Expected 1 message, got %d", len(messages))
				}
				assistantMsg := assertQueryAssistantMessage(t, messages[0])
				assertQueryTextContent(t, assistantMsg, "4")
			},
		},
		{
			name:    "query_with_system_prompt",
			prompt:  "Hello",
			options: []Option{WithSystemPrompt("You are a helpful assistant")},
			setupTransport: func() Transport {
				return newQueryMockTransport(WithQueryAssistantResponse("Hi there!"))
			},
			expectError: false,
			validateResults: func(t *testing.T, iter MessageIterator) {
				t.Helper()
				messages := collectQueryMessages(ctx, t, iter)
				if len(messages) != 1 {
					t.Fatalf("Expected 1 message, got %d", len(messages))
				}
				assertQueryAssistantMessage(t, messages[0])
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transport := test.setupTransport()

			// Test Query() behavior using QueryWithTransport for dependency injection
			iter, err := QueryWithTransport(ctx, test.prompt, transport, test.options...)

			if test.expectError {
				if err == nil {
					t.Fatalf("Expected error, got none")
				}
				if test.errorContains != "" && !strings.Contains(err.Error(), test.errorContains) {
					t.Errorf("Expected error to contain %q, got: %v", test.errorContains, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			defer func() { _ = iter.Close() }()

			if test.validateResults != nil {
				test.validateResults(t, iter)
			}
		})
	}
}

// TestQueryIteratorErrorPaths tests missing error paths in queryIterator.Next
// Targets 84.0% coverage in Next (query.go:81)
func TestQueryIteratorErrorPaths(t *testing.T) {
	ctx, cancel := setupQueryTestContext(t, 5*time.Second)
	defer cancel()

	tests := []struct {
		name           string
		setupTransport func() *queryMockTransport
		testFlow       func(*testing.T, context.Context, MessageIterator)
	}{
		{
			name: "iterator_start_error",
			setupTransport: func() *queryMockTransport {
				return newQueryMockTransport(WithQueryConnectError(fmt.Errorf("start failed")))
			},
			testFlow: func(t *testing.T, ctx context.Context, iter MessageIterator) {
				// First call to Next should trigger start() and fail
				msg, err := iter.Next(ctx)
				if err == nil {
					t.Error("Expected error on iterator start failure")
				}
				if msg != nil {
					t.Errorf("Expected nil message on error, got: %v", msg)
				}
				// Verify error message
				if !strings.Contains(err.Error(), "start failed") {
					t.Errorf("Expected start error, got: %v", err)
				}
			},
		},
		{
			name: "iterator_already_closed",
			setupTransport: func() *queryMockTransport {
				return newQueryMockTransport(WithQueryAssistantResponse("test"))
			},
			testFlow: func(t *testing.T, ctx context.Context, iter MessageIterator) {
				// Close iterator first
				err := iter.Close()
				if err != nil {
					t.Fatalf("Failed to close iterator: %v", err)
				}

				// Now try to call Next - should get ErrNoMoreMessages
				msg, err := iter.Next(ctx)
				if err != ErrNoMoreMessages {
					t.Errorf("Expected ErrNoMoreMessages, got: %v", err)
				}
				if msg != nil {
					t.Errorf("Expected nil message after close, got: %v", msg)
				}
			},
		},
		{
			name: "error_channel_receives_error",
			setupTransport: func() *queryMockTransport {
				// Create transport that will send error on error channel
				// Add a message to ensure we're testing error channel, not empty channel race
				transport := newQueryMockTransport(WithQueryAssistantResponse("test response"))
				transport.sendError = fmt.Errorf("transport error during operation")
				return transport
			},
			testFlow: func(t *testing.T, ctx context.Context, iter MessageIterator) {
				// This should trigger the start and then hit the error channel path
				msg, err := iter.Next(ctx)
				// Should get some kind of error (either from start or from error channel)
				if err == nil {
					t.Error("Expected error from error channel")
				}
				if msg != nil {
					t.Errorf("Expected nil message on error, got: %v", msg)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transport := test.setupTransport()
			iter, err := QueryWithTransport(ctx, "test query", transport)
			if err != nil {
				t.Fatalf("QueryWithTransport failed: %v", err)
			}
			defer func() { _ = iter.Close() }()

			test.testFlow(t, ctx, iter)
		})
	}
}

// TestQueryWithNilOptions tests Query function with nil options
// Targets coverage in internal option handling paths
func TestQueryWithNilOptions(t *testing.T) {
	ctx, cancel := setupQueryTestContext(t, 5*time.Second)
	defer cancel()

	transport := newQueryMockTransport(WithQueryAssistantResponse("response"))

	// Test QueryWithTransport with no options (nil options internally)
	iter, err := QueryWithTransport(ctx, "test prompt", transport)
	if err != nil {
		t.Fatalf("QueryWithTransport with no options failed: %v", err)
	}
	defer func() { _ = iter.Close() }()

	// Should be able to get response normally
	msg, err := iter.Next(ctx)
	if err != nil {
		t.Fatalf("Next() failed: %v", err)
	}
	if msg == nil {
		t.Fatal("Expected message, got nil")
	}

	// Verify message type
	assistantMsg, ok := msg.(*AssistantMessage)
	if !ok {
		t.Fatalf("Expected AssistantMessage, got %T", msg)
	}

	assertQueryTextContent(t, assistantMsg, "response")
}

// TestQueryIteratorContextCancellation tests context cancellation during iteration
func TestQueryIteratorContextCancellation(t *testing.T) {
	ctx, cancel := setupQueryTestContext(t, 5*time.Second)
	defer cancel()

	// Create transport with delay to allow cancellation
	transport := newQueryMockTransport(WithQueryDelay(100 * time.Millisecond))

	iter, err := QueryWithTransport(ctx, "test query", transport)
	if err != nil {
		t.Fatalf("QueryWithTransport failed: %v", err)
	}
	defer func() { _ = iter.Close() }()

	// Cancel context before calling Next
	cancel()

	// This should handle context cancellation
	msg, err := iter.Next(ctx)
	if err == nil {
		t.Error("Expected context cancellation error")
	}
	if msg != nil {
		t.Errorf("Expected nil message on context cancellation, got: %v", msg)
	}
}

// Mock Transport Implementation
type queryMockTransport struct {
	mu               sync.RWMutex
	connected        bool
	msgChan          chan Message
	errChan          chan error
	receivedMessages []StreamMessage
	responseMessages []Message
	systemMessages   []*SystemMessage
	resultMessages   []*ResultMessage
	connectError     error
	sendError        error
	delay            time.Duration
	optionsReceived  bool
}

func (q *queryMockTransport) Connect(ctx context.Context) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	// Check context cancellation first, like real transport would
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if q.connectError != nil {
		return q.connectError
	}

	q.connected = true
	q.msgChan = make(chan Message, 10)
	q.errChan = make(chan error, 10)

	go func() {
		defer close(q.msgChan)
		defer close(q.errChan)

		if q.delay > 0 {
			select {
			case <-time.After(q.delay):
			case <-ctx.Done():
				q.errChan <- ctx.Err()
				return
			}
		}

		// Send all configured messages
		q.mu.RLock()
		messages := make([]Message, len(q.responseMessages))
		copy(messages, q.responseMessages)
		q.mu.RUnlock()

		for _, msg := range messages {
			select {
			case q.msgChan <- msg:
			case <-ctx.Done():
				return
			}
		}

		// Keep channels open for a brief moment to allow iterator to consume messages
		// This prevents race condition where channels close before ReceiveMessages() is called
		select {
		case <-time.After(10 * time.Millisecond):
		case <-ctx.Done():
		}
	}()

	return nil
}

func (q *queryMockTransport) SendMessage(ctx context.Context, message StreamMessage) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	// Check context cancellation first, like real transport would
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if q.sendError != nil {
		return q.sendError
	}

	if !q.connected {
		return fmt.Errorf("not connected")
	}

	q.receivedMessages = append(q.receivedMessages, message)
	q.optionsReceived = true
	return nil
}

func (q *queryMockTransport) ReceiveMessages(_ context.Context) (<-chan Message, <-chan error) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.msgChan, q.errChan
}

func (q *queryMockTransport) Interrupt(_ context.Context) error {
	return nil
}

func (q *queryMockTransport) SetModel(_ context.Context, _ *string) error {
	return nil
}

func (q *queryMockTransport) SetPermissionMode(_ context.Context, _ PermissionMode) error {
	return nil
}

func (q *queryMockTransport) RewindFiles(_ context.Context, _ string) error {
	return nil
}

func (q *queryMockTransport) GetMcpStatus(_ context.Context) (*McpStatusResponse, error) {
	return &McpStatusResponse{}, nil
}

func (q *queryMockTransport) Close() error {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.connected = false
	return nil
}

func (q *queryMockTransport) GetValidator() *StreamValidator {
	return &StreamValidator{}
}

// Mock helper methods

func (q *queryMockTransport) hasReceivedOptions() bool {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.optionsReceived
}

// Mock Transport Options
type QueryMockOption func(*queryMockTransport)

func WithQueryAssistantResponse(text string) QueryMockOption {
	return func(q *queryMockTransport) {
		q.responseMessages = append(q.responseMessages, &AssistantMessage{
			Content: []ContentBlock{&TextBlock{Text: text}},
			Model:   "claude-opus-4-1-20250805",
		})
	}
}

func WithQueryMultipleMessages(messages []*AssistantMessage) QueryMockOption {
	return func(q *queryMockTransport) {
		for _, msg := range messages {
			q.responseMessages = append(q.responseMessages, msg)
		}
	}
}

func WithQuerySystemMessage(subtype string, data map[string]any) QueryMockOption {
	return func(q *queryMockTransport) {
		systemMsg := &SystemMessage{
			Subtype: subtype,
			Data:    data,
		}
		q.responseMessages = append(q.responseMessages, systemMsg)
	}
}

func WithQueryResultMessage(isError bool, durationMs, numTurns int) QueryMockOption {
	return func(q *queryMockTransport) {
		resultMsg := &ResultMessage{
			Subtype:       "success",
			DurationMs:    durationMs,
			DurationAPIMs: durationMs - 500,
			IsError:       isError,
			NumTurns:      numTurns,
			SessionID:     "test-session",
		}
		q.responseMessages = append(q.responseMessages, resultMsg)
	}
}

func WithQueryConnectError(err error) QueryMockOption {
	return func(q *queryMockTransport) {
		q.connectError = err
	}
}

func WithQuerySendError(err error) QueryMockOption {
	return func(q *queryMockTransport) {
		q.sendError = err
	}
}

func WithQueryDelay(delay time.Duration) QueryMockOption {
	return func(q *queryMockTransport) {
		q.delay = delay
	}
}

// Factory Functions
func newQueryMockTransport(options ...QueryMockOption) *queryMockTransport {
	transport := &queryMockTransport{
		responseMessages: make([]Message, 0),
		systemMessages:   make([]*SystemMessage, 0),
		resultMessages:   make([]*ResultMessage, 0),
		receivedMessages: make([]StreamMessage, 0),
	}
	for _, option := range options {
		option(transport)
	}
	return transport
}

// Helper Functions
func setupQueryTestContext(t *testing.T, timeout time.Duration) (context.Context, context.CancelFunc) {
	t.Helper()
	return context.WithTimeout(context.Background(), timeout)
}

func collectQueryMessages(ctx context.Context, t *testing.T, iter MessageIterator) []Message {
	t.Helper()
	var messages []Message
	for {
		msg, err := iter.Next(ctx)
		if err != nil {
			if err == ErrNoMoreMessages {
				break
			}
			t.Fatalf("Iterator error: %v", err)
		}
		if msg != nil {
			messages = append(messages, msg)
		}
	}
	return messages
}

func assertQueryAssistantMessage(t *testing.T, msg Message) *AssistantMessage {
	t.Helper()
	assistantMsg, ok := msg.(*AssistantMessage)
	if !ok {
		t.Fatalf("Expected AssistantMessage, got %T", msg)
	}
	return assistantMsg
}

func assertQueryTextContent(t *testing.T, msg *AssistantMessage, expectedText string) {
	t.Helper()
	if len(msg.Content) != 1 {
		t.Fatalf("Expected 1 content block, got %d", len(msg.Content))
	}

	textBlock, ok := msg.Content[0].(*TextBlock)
	if !ok {
		t.Fatalf("Expected TextBlock, got %T", msg.Content[0])
	}

	if textBlock.Text != expectedText {
		t.Errorf("Expected text '%s', got '%s'", expectedText, textBlock.Text)
	}
}

func assertQueryMessageModel(t *testing.T, msg *AssistantMessage, expectedModel string) {
	t.Helper()
	if msg.Model != expectedModel {
		t.Errorf("Expected model '%s', got '%s'", expectedModel, msg.Model)
	}
}

func assertQueryTransportReceivedOptions(t *testing.T, transport *queryMockTransport, expected bool) {
	t.Helper()
	actual := transport.hasReceivedOptions()
	if actual != expected {
		t.Errorf("Expected options received = %v, got %v", expected, actual)
	}
}

func assertQueryError(t *testing.T, err error, wantErr bool, msgContains string) {
	t.Helper()
	if (err != nil) != wantErr {
		t.Errorf("error = %v, wantErr %v", err, wantErr)
		return
	}
	if wantErr && msgContains != "" && !strings.Contains(err.Error(), msgContains) {
		t.Errorf("error = %v, expected message to contain %q", err, msgContains)
	}
}

func isQueryContextError(err, target error) bool {
	if err == target {
		return true
	}
	if err != nil && target != nil {
		return strings.Contains(err.Error(), target.Error())
	}
	return false
}

// setupIsolatedEnvironment creates an isolated environment for testing CLI discovery
func setupIsolatedEnvironment(t *testing.T) func() {
	t.Helper()
	tempHome := t.TempDir()
	originalHome := os.Getenv("HOME")
	originalPath := os.Getenv("PATH")

	if runtime.GOOS == "windows" {
		originalHome = os.Getenv("USERPROFILE")
		_ = os.Setenv("USERPROFILE", tempHome)
	} else {
		_ = os.Setenv("HOME", tempHome)
	}
	_ = os.Setenv("PATH", "/nonexistent/path")

	return func() {
		if runtime.GOOS == "windows" {
			_ = os.Setenv("USERPROFILE", originalHome)
		} else {
			_ = os.Setenv("HOME", originalHome)
		}
		_ = os.Setenv("PATH", originalPath)
	}
}

// TestQueryFunction tests the actual Query() function with CLI discovery
func TestQueryFunction(t *testing.T) {
	tests := []struct {
		name        string
		prompt      string
		options     []Option
		setupMock   func(t *testing.T) (cleanup func())
		expectError bool
		errorMsg    string
	}{
		{
			name:        "query_cli_not_found",
			prompt:      "What is 2+2?",
			options:     []Option{},
			setupMock:   setupIsolatedEnvironment,
			expectError: true,
			errorMsg:    "failed to create query transport",
		},
		{
			name:   "query_with_options_cli_not_found",
			prompt: "Hello world",
			options: []Option{
				WithSystemPrompt("You are helpful"),
				WithModel("claude-sonnet-3-5-20241022"),
			},
			setupMock:   setupIsolatedEnvironment,
			expectError: true,
			errorMsg:    "failed to create query transport",
		},
		{
			name:        "query_empty_prompt_cli_not_found",
			prompt:      "",
			options:     []Option{},
			setupMock:   setupIsolatedEnvironment,
			expectError: true,
			errorMsg:    "failed to create query transport",
		},
		{
			name:   "query_multiple_options_cli_not_found",
			prompt: "Complex query",
			options: []Option{
				WithSystemPrompt("Test system"),
				WithAllowedTools("Read", "Write"),
				WithMaxTurns(3),
				WithPermissionMode(PermissionModeAcceptEdits),
			},
			setupMock:   setupIsolatedEnvironment,
			expectError: true,
			errorMsg:    "failed to create query transport",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cleanup := test.setupMock(t)
			defer cleanup()

			ctx, cancel := setupQueryTestContext(t, 5*time.Second)
			defer cancel()

			// Call the actual Query() function - this exercises the real function
			iter, err := Query(ctx, test.prompt, test.options...)

			if test.expectError {
				if err == nil {
					t.Fatal("Expected error, got none")
				}
				if !strings.Contains(err.Error(), test.errorMsg) {
					t.Errorf("Expected error to contain %q, got: %v", test.errorMsg, err)
				}
				// Iterator should be nil on error
				if iter != nil {
					t.Error("Expected nil iterator on error, got non-nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			// Clean up iterator if created
			if iter != nil {
				_ = iter.Close()
			}
		})
	}
}
