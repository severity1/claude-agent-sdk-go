package claudecode

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/severity1/claude-agent-sdk-go/internal/control"
	"github.com/severity1/claude-agent-sdk-go/internal/subprocess"
)

const (
	userMessageType = "user"
	testModelSonnet = "claude-sonnet-4-5"
)

// TestClientLifecycleManagement tests connection, resource cleanup, and transport integration
// Covers T133: Client Auto Connect Context Manager + resource management + transport integration
func TestClientLifecycleManagement(t *testing.T) {
	ctx, cancel := setupClientTestContext(t, 10*time.Second)
	defer cancel()

	subtests := []struct {
		name string
		test func(context.Context, *testing.T)
	}{
		{"basic_lifecycle", testBasicLifecycle},
		{"resource_cleanup_cycles", testResourceCleanupCycles},
		{"transport_integration", testTransportIntegration},
	}

	for _, subtest := range subtests {
		t.Run(subtest.name, func(t *testing.T) {
			subtest.test(ctx, t)
		})
	}
}

func testBasicLifecycle(ctx context.Context, t *testing.T) {
	t.Helper()
	transport := newClientMockTransport()

	// Test defer-based resource management.
	func() {
		client := setupClientForTest(t, transport)
		defer disconnectClientSafely(t, client)
		connectClientSafely(ctx, t, client)
		assertClientConnected(t, transport)
		err := client.Query(ctx, "test message")
		assertNoError(t, err)
	}() // Defer should trigger disconnect

	assertClientDisconnected(t, transport)

	// Test manual connection lifecycle
	client := setupClientForTest(t, transport)
	connectClientSafely(ctx, t, client)
	assertClientConnected(t, transport)
	disconnectClientSafely(t, client)
	assertClientDisconnected(t, transport)
}

func testResourceCleanupCycles(ctx context.Context, t *testing.T) {
	t.Helper()
	transport := newClientMockTransport()

	// Test resource cleanup with multiple connect/disconnect cycles
	for i := 0; i < 3; i++ {
		client := setupClientForTest(t, transport)
		connectClientSafely(ctx, t, client)
		assertClientConnected(t, transport)
		err := client.Query(ctx, fmt.Sprintf("test query %d", i))
		assertNoError(t, err)
		disconnectClientSafely(t, client)
		assertClientDisconnected(t, transport)
		transport.reset()
	}

	// Verify no resource leaks (basic check)
	if transport.getSentMessageCount() != 0 {
		t.Error("Expected transport to be reset after cleanup")
	}
}

func testTransportIntegration(ctx context.Context, t *testing.T) {
	t.Helper()
	transport := newClientMockTransport()
	client := setupClientForTest(t, transport)
	defer disconnectClientSafely(t, client)

	// Verify interface compliance
	var _ Transport = transport

	// Test transport operations through client
	err := client.Connect(ctx)
	assertNoError(t, err)
	if !transport.connected {
		t.Error("Expected transport to be connected via client")
	}

	// Test message sending
	err = client.Query(ctx, "test message")
	assertNoError(t, err)
	if transport.getSentMessageCount() != 1 {
		t.Errorf("Expected 1 message sent, got %d", transport.getSentMessageCount())
	}

	// Test disconnect
	err = client.Disconnect()
	assertNoError(t, err)
	if transport.connected {
		t.Error("Expected transport to be disconnected via client")
	}
}

// TestClientQueryExecution tests one-shot query functionality
func TestClientQueryExecution(t *testing.T) {
	ctx, cancel := setupClientTestContext(t, 10*time.Second)
	defer cancel()

	transport := newClientMockTransport()
	client := setupClientForTest(t, transport)
	defer disconnectClientSafely(t, client)

	connectClientSafely(ctx, t, client)

	// Execute query through connected client
	err := client.Query(ctx, "What is 2+2?")
	assertNoError(t, err)

	// Verify message was sent to transport
	assertClientMessageCount(t, transport, 1)

	// Verify message content
	sentMsg, ok := transport.getSentMessage(0)
	if !ok {
		t.Fatal("Failed to get sent message")
	}
	if sentMsg.Type != userMessageType {
		t.Errorf("Expected message type 'user', got '%s'", sentMsg.Type)
	}

	messageMap, ok := sentMsg.Message.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected map[string]interface{}, got %T", sentMsg.Message)
	}

	if role, ok := messageMap["role"]; !ok || role != userMessageType {
		t.Errorf("Expected message role 'user', got '%v'", role)
	}
	if content, ok := messageMap["content"]; !ok || content != "What is 2+2?" {
		t.Errorf("Expected content 'What is 2+2?', got '%v'", content)
	}
}

// TestClientStreamQuery tests streaming query with message handling
func TestClientStreamQuery(t *testing.T) {
	ctx, cancel := setupClientTestContext(t, 10*time.Second)
	defer cancel()

	transport := newClientMockTransport()
	client := setupClientForTest(t, transport)
	defer disconnectClientSafely(t, client)

	connectClientSafely(ctx, t, client)

	// Create message channel
	messages := make(chan StreamMessage, 3)
	messages <- StreamMessage{
		Type: "request",
		Message: &UserMessage{
			Content: "Hello",
		},
	}
	messages <- StreamMessage{
		Type: "request",
		Message: &UserMessage{
			Content: "How are you?",
		},
	}
	close(messages)

	// Execute stream query
	err := client.QueryStream(ctx, messages)
	assertNoError(t, err)

	// Wait a bit for async processing to complete
	time.Sleep(100 * time.Millisecond)

	// Verify messages were sent
	assertClientMessageCount(t, transport, 2)
}

// TestClientErrorHandling tests connection, send, and async error scenarios - streamlined
func TestClientErrorHandling(t *testing.T) {
	ctx, cancel := setupClientTestContext(t, 10*time.Second)
	defer cancel()

	errorTests := map[string]struct {
		errorType string
		operation string
		errorMsg  string
	}{
		"connection_error":           {"connect", "Connect", "connection failed"},
		"send_error":                 {"send", "Query", "send failed"},
		"successful_op":              {"", "Query", ""},
		"interrupt_not_connected":    {"", "Interrupt", "client not connected"},
		"query_stream_not_connected": {"", "QueryStream", "client not connected"},
	}

	for name, test := range errorTests {
		t.Run(name, func(t *testing.T) {
			var transport *clientMockTransport
			if test.errorType == "" {
				transport = newClientMockTransport()
			} else {
				transport = newMockTransportWithError(test.errorType, errors.New(test.errorMsg))
			}

			client := setupClientForTest(t, transport)
			defer disconnectClientSafely(t, client)

			var err error
			switch test.operation {
			case "Connect":
				err = client.Connect(ctx)
			case "Query":
				if test.errorType != "connect" {
					connectClientSafely(ctx, t, client)
				}
				err = client.Query(ctx, "test")
			case "Interrupt":
				// Don't connect for interrupt_not_connected test
				err = client.Interrupt(ctx)
			case "QueryStream":
				// Don't connect for query_stream_not_connected test
				messages := make(chan StreamMessage)
				close(messages)
				err = client.QueryStream(ctx, messages)
			}

			wantErr := test.errorMsg != ""
			assertClientError(t, err, wantErr, test.errorMsg)
		})
	}
}

// TestClientConcurrency tests basic thread safety validation
func TestClientConcurrency(t *testing.T) {
	ctx, cancel := setupClientTestContext(t, 30*time.Second)
	defer cancel()

	transport := newClientMockTransport()
	client := setupClientForTest(t, transport)
	defer disconnectClientSafely(t, client)

	connectClientSafely(ctx, t, client)

	// Run concurrent queries
	const numGoroutines = 10
	const queriesPerGoroutine = 5

	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines*queriesPerGoroutine)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < queriesPerGoroutine; j++ {
				err := client.Query(ctx, fmt.Sprintf("query %d-%d", id, j))
				if err != nil {
					errors <- err
				}
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
		t.Errorf("Concurrent query error: %v", err)
	}

	// Verify all messages were sent
	expectedMessages := numGoroutines * queriesPerGoroutine
	assertClientMessageCount(t, transport, expectedMessages)
}

// TestClientConfiguration tests options application and validation with proper behavior verification
func TestClientConfiguration(t *testing.T) {
	tests := []struct {
		name     string
		options  []Option
		validate func(*testing.T, Client, *clientMockTransport)
	}{
		{"default_configuration", []Option{}, verifyDefaultConfiguration},
		{"system_prompt_configuration", []Option{
			WithSystemPrompt("You are a test assistant. Always respond with 'TEST_RESPONSE'."),
		}, verifySystemPromptConfig},
		{"tools_configuration", []Option{
			WithAllowedTools("Read", "Write"),
			WithDisallowedTools("Bash", "WebSearch"),
		}, verifyToolsConfig},
		{"multiple_options_precedence", []Option{
			WithSystemPrompt("First prompt"),
			WithMaxThinkingTokens(5000),
			WithSystemPrompt("Second prompt"),
			WithMaxThinkingTokens(10000),
			WithAllowedTools("Read"),
			WithAllowedTools("Read", "Write"),
		}, verifyOptionsConfig},
		{"complex_configuration", []Option{
			WithSystemPrompt("Complex test system prompt"),
			WithAllowedTools("Read", "Write", "Edit"),
			WithDisallowedTools("Bash"),
			WithContinueConversation(true),
			WithMaxThinkingTokens(8000),
			WithPermissionMode(PermissionModeAcceptEdits),
		}, verifyComplexConfig},
		{"session_configuration", []Option{
			WithContinueConversation(true),
			WithResume("test-session-123"),
		}, verifySessionConfig},
		{"validation_error_negative_max_turns", []Option{
			WithMaxTurns(-1),
		}, verifyValidationError},
		{"validation_error_invalid_cwd", []Option{
			WithCwd("/nonexistent/test/directory"),
		}, verifyValidationError},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transport := newClientMockTransport()
			client := NewClientWithTransport(transport, test.options...)
			defer disconnectClientSafely(t, client)

			test.validate(t, client, transport)
		})
	}
}

// TestClientCanUseToolAutoConfiguresPermissionPromptToolName verifies that when
// CanUseTool callback is set but PermissionPromptToolName is not, validateOptions
// automatically configures PermissionPromptToolName to "stdio" for control protocol routing.
func TestClientCanUseToolAutoConfiguresPermissionPromptToolName(t *testing.T) {
	ctx, cancel := setupClientTestContext(t, 5*time.Second)
	defer cancel()

	// Create a simple permission callback
	callback := func(_ context.Context, _ string, _ map[string]any, _ ToolPermissionContext) (PermissionResult, error) {
		return NewPermissionResultAllow(), nil
	}

	// Create client with CanUseTool but without PermissionPromptToolName
	transport := newClientMockTransport()
	client := NewClientWithTransport(transport, WithCanUseTool(callback))
	defer disconnectClientSafely(t, client)

	// Connect triggers validateOptions which should auto-configure PermissionPromptToolName
	err := client.Connect(ctx)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	// Access internal options via type assertion
	impl, ok := client.(*ClientImpl)
	if !ok {
		t.Fatal("Expected client to be *ClientImpl")
	}

	// Verify PermissionPromptToolName was auto-configured to "stdio"
	if impl.options.PermissionPromptToolName == nil {
		t.Error("Expected PermissionPromptToolName to be auto-configured, got nil")
	} else if *impl.options.PermissionPromptToolName != "stdio" {
		t.Errorf("Expected PermissionPromptToolName = 'stdio', got %q", *impl.options.PermissionPromptToolName)
	}
}

// TestClientCanUseToolReconnectAndConflict verifies a second Connect still
// works after the first one set PermissionPromptToolName to "stdio", and a
// conflicting tool name fails fast (Python raises ValueError).
func TestClientCanUseToolReconnectAndConflict(t *testing.T) {
	ctx, cancel := setupClientTestContext(t, 5*time.Second)
	defer cancel()
	callback := func(_ context.Context, _ string, _ map[string]any, _ ToolPermissionContext) (PermissionResult, error) {
		return NewPermissionResultAllow(), nil
	}

	client := NewClientWithTransport(newClientMockTransport(), WithCanUseTool(callback))
	connectClientSafely(ctx, t, client)
	if err := client.Disconnect(); err != nil {
		t.Fatalf("Disconnect failed: %v", err)
	}
	connectClientSafely(ctx, t, client)
	disconnectClientSafely(t, client)

	conflict := NewClientWithTransport(newClientMockTransport(),
		WithCanUseTool(callback), WithPermissionPromptToolName("mcp__perm__ask"))
	err := conflict.Connect(ctx)
	if err == nil || !strings.Contains(err.Error(), "cannot be used with PermissionPromptToolName") {
		t.Fatalf("Connect() error = %v, want CanUseTool conflict error", err)
	}
}

// TestIteratorsTreatClosedErrChanAsNoError verifies a closed error channel
// does not make Next return (nil, nil) or skip buffered messages (Issue #144).
// The transport closes errChan before msgChan when the CLI exits.
func TestIteratorsTreatClosedErrChanAsNoError(t *testing.T) {
	newChannels := func() (chan Message, chan error) {
		msgChan := make(chan Message, 2)
		msgChan <- &AssistantMessage{Model: "claude-3"}
		msgChan <- &AssistantMessage{Model: "claude-3"}
		close(msgChan)
		errChan := make(chan error)
		close(errChan)
		return msgChan, errChan
	}
	drain := func(t *testing.T, iter MessageIterator) {
		t.Helper()
		ctx, cancel := setupClientTestContext(t, 5*time.Second)
		defer cancel()
		for i := 0; i < 2; i++ {
			msg, err := iter.Next(ctx)
			if err != nil || msg == nil {
				t.Fatalf("Next() #%d = (%v, %v), want a message", i+1, msg, err)
			}
		}
		if _, err := iter.Next(ctx); !errors.Is(err, ErrNoMoreMessages) {
			t.Fatalf("Next() after drain error = %v, want ErrNoMoreMessages", err)
		}
	}

	t.Run("client_iterator", func(t *testing.T) {
		msgChan, errChan := newChannels()
		streamErrChan := make(chan error)
		close(streamErrChan)
		drain(t, &clientIterator{stream: newStreamReader(msgChan, errChan), streamErrChan: streamErrChan})
	})
	t.Run("query_iterator", func(t *testing.T) {
		msgChan, errChan := newChannels()
		drain(t, &queryIterator{
			transport: newQueryMockTransport(),
			ctx:       context.Background(),
			started:   true,
			stream:    newStreamReader(msgChan, errChan),
		})
	})
}

// streamErrorIterators builds each iterator type over hand-fed transport channels.
func streamErrorIterators() map[string]func(msgChan <-chan Message, errChan <-chan error) MessageIterator {
	return map[string]func(<-chan Message, <-chan error) MessageIterator{
		"client_iterator": func(msgChan <-chan Message, errChan <-chan error) MessageIterator {
			return &clientIterator{stream: newStreamReader(msgChan, errChan)}
		},
		"query_iterator": func(msgChan <-chan Message, errChan <-chan error) MessageIterator {
			return &queryIterator{
				transport: newQueryMockTransport(),
				ctx:       context.Background(),
				started:   true,
				stream:    newStreamReader(msgChan, errChan),
			}
		},
	}
}

// streamErrorTrials repeats each scenario on fresh channels. With a message and
// an error both ready, select picks either at random, so the old code failed a
// single trial about half the time and fails 50 in a row with probability
// 1-2^-50.
const streamErrorTrials = 50

// TestIteratorsDeliverBufferedMessagesBeforeStreamError verifies Next hands out
// every buffered message before the transport's terminal error, and does not
// lose that error to a closed msgChan. The transport sends the error after
// the last message and closes both channels, so Python's ProcessError always
// follows the last message.
func TestIteratorsDeliverBufferedMessagesBeforeStreamError(t *testing.T) {
	exitErr := NewProcessError("Claude Code process exited unexpectedly", 1, "")

	tests := []struct {
		name        string
		buffered    int
		closeMsg    bool
		wantMessage int
	}{
		{"error_ready_while_messages_buffered", 3, true, 3},
		{"error_ready_with_msgchan_still_open", 3, false, 3},
		{"msgchan_closed_with_error_in_closed_errchan", 0, true, 0},
	}

	for name, newIterator := range streamErrorIterators() {
		for _, tt := range tests {
			t.Run(name+"/"+tt.name, func(t *testing.T) {
				ctx, cancel := setupClientTestContext(t, 10*time.Second)
				defer cancel()

				for trial := 0; trial < streamErrorTrials; trial++ {
					msgChan := make(chan Message, tt.buffered)
					for i := 0; i < tt.buffered; i++ {
						msgChan <- &AssistantMessage{Model: fmt.Sprintf("m%d", i)}
					}
					errChan := make(chan error, 1)
					errChan <- exitErr
					close(errChan)
					if tt.closeMsg {
						close(msgChan)
					}
					iter := newIterator(msgChan, errChan)

					for i := 0; i < tt.wantMessage; i++ {
						msg, err := iter.Next(ctx)
						if err != nil {
							t.Fatalf("trial %d: Next() #%d = %v, want message m%d before the error", trial, i+1, err, i)
						}
						if got := msg.(*AssistantMessage).Model; got != fmt.Sprintf("m%d", i) {
							t.Fatalf("trial %d: Next() #%d = %q, want m%d", trial, i+1, got, i)
						}
					}
					if _, err := iter.Next(ctx); err != exitErr {
						t.Fatalf("trial %d: Next() after %d messages error = %v, want the exit error", trial, tt.wantMessage, err)
					}
					if _, err := iter.Next(ctx); !errors.Is(err, ErrNoMoreMessages) {
						t.Fatalf("trial %d: Next() after the exit error = %v, want ErrNoMoreMessages", trial, err)
					}
				}
			})
		}
	}
}

// TestQueryIteratorDeliversResultBeforeExitError pins the symptom: a CLI that
// exits non-zero right after an error ResultMessage must still yield that
// ResultMessage to a consumer that was slow to read.
func TestQueryIteratorDeliversResultBeforeExitError(t *testing.T) {
	ctx, cancel := setupClientTestContext(t, 10*time.Second)
	defer cancel()
	exitErr := NewProcessError("Claude Code returned an error result: boom", 1, "")

	for trial := 0; trial < streamErrorTrials; trial++ {
		msgChan := make(chan Message, 10)
		for i := 0; i < 9; i++ {
			msgChan <- &AssistantMessage{Model: "claude-3"}
		}
		msgChan <- &ResultMessage{IsError: true, Subtype: "error_during_execution"}
		errChan := make(chan error, 1)
		errChan <- exitErr
		close(errChan)
		close(msgChan)
		iter := streamErrorIterators()["query_iterator"](msgChan, errChan)

		sawResult := false
		var end error
		for end == nil {
			msg, err := iter.Next(ctx)
			if _, ok := msg.(*ResultMessage); ok {
				sawResult = true
			}
			end = err
		}
		if !sawResult {
			t.Fatalf("trial %d: ResultMessage lost to %v", trial, end)
		}
		if end != exitErr {
			t.Fatalf("trial %d: ended with %v, want the exit error", trial, end)
		}
	}
}

// TestReceiveResponseExitErrorBelongsToNextCall verifies the exit error that
// follows a turn's ResultMessage is not consumed by that turn: ReceiveResponse
// ends at the ResultMessage (Python receive_response), and the error comes
// out of the next call.
func TestReceiveResponseExitErrorBelongsToNextCall(t *testing.T) {
	ctx, cancel := setupClientTestContext(t, 10*time.Second)
	defer cancel()
	exitErr := NewProcessError("Claude Code returned an error result: boom", 1, "")

	for trial := 0; trial < streamErrorTrials; trial++ {
		msgChan := make(chan Message, 3)
		msgChan <- &AssistantMessage{Model: "claude-3"}
		msgChan <- &AssistantMessage{Model: "claude-3"}
		msgChan <- &ResultMessage{IsError: true, Subtype: "error_during_execution"}
		close(msgChan)
		errChan := make(chan error, 1)
		errChan <- exitErr
		close(errChan)
		stream := newStreamReader(msgChan, errChan)

		turn := &clientIterator{stream: stream}
		for i := 0; i < 3; i++ {
			if _, err := turn.Next(ctx); err != nil {
				t.Fatalf("trial %d: turn Next() #%d = %v, want the turn's messages", trial, i+1, err)
			}
		}
		if _, err := turn.Next(ctx); !errors.Is(err, ErrNoMoreMessages) {
			t.Fatalf("trial %d: turn Next() after the ResultMessage = %v, want ErrNoMoreMessages", trial, err)
		}

		next := &clientIterator{stream: stream}
		if _, err := next.Next(ctx); err != exitErr {
			t.Fatalf("trial %d: next call = %v, want the exit error", trial, err)
		}
	}
}

// An iterator blocked in next while another sharing the reader sets the error
// aside must still return that error when msgChan closes under it.
func TestStreamReaderEndOfStreamReturnsPendingError(t *testing.T) {
	exitErr := NewProcessError("Claude Code process exited unexpectedly", 1, "")
	msgChan := make(chan Message)
	close(msgChan)
	errChan := make(chan error)
	close(errChan)
	stream := newStreamReader(msgChan, errChan)

	stream.setPending(exitErr)
	if err := stream.endOfStream(); err != exitErr {
		t.Fatalf("endOfStream() = %v, want the pending exit error", err)
	}
	if err := stream.endOfStream(); !errors.Is(err, ErrNoMoreMessages) {
		t.Fatalf("endOfStream() after the error was taken = %v, want ErrNoMoreMessages", err)
	}
}

// TestClientReceiveMessages tests message reception through client channels
// Covers T137: Client Message Reception
func TestClientReceiveMessages(t *testing.T) {
	ctx, cancel := setupClientTestContext(t, 10*time.Second)
	defer cancel()

	transport := newClientMockTransport()
	client := setupClientForTest(t, transport)
	defer disconnectClientSafely(t, client)

	connectClientSafely(ctx, t, client)

	// Get message channel from client
	msgChan := client.ReceiveMessages(ctx)
	if msgChan == nil {
		t.Fatal("Expected message channel, got nil")
	}

	// Create and inject a test message for this test
	testMessage := &AssistantMessage{
		Content: []ContentBlock{&TextBlock{Text: "Test response message"}},
		Model:   "claude-3-5-sonnet-20241022",
	}
	transport.injectTestMessage(testMessage)

	// Receive the message through client channel
	select {
	case msg := <-msgChan:
		if msg == nil {
			t.Error("Received nil message")
			return
		}

		assistantMsg, ok := msg.(*AssistantMessage)
		if !ok {
			t.Errorf("Expected AssistantMessage, got %T", msg)
			return
		}

		if len(assistantMsg.Content) != 1 {
			t.Errorf("Expected 1 content block, got %d", len(assistantMsg.Content))
			return
		}

		textBlock, ok := assistantMsg.Content[0].(*TextBlock)
		if !ok {
			t.Errorf("Expected TextBlock, got %T", assistantMsg.Content[0])
			return
		}

		if textBlock.Text != "Test response message" {
			t.Errorf("Expected 'Test response message', got '%s'", textBlock.Text)
		}

	case <-time.After(1 * time.Second):
		t.Error("Timeout waiting for message from client channel")
	}

	// Test ReceiveMessages when not connected - covers missing branch
	disconnectedClient := setupClientForTest(t, newClientMockTransport())
	defer disconnectClientSafely(t, disconnectedClient)
	// Note: Don't call connectClientSafely here

	disconnectedMsgChan := disconnectedClient.ReceiveMessages(ctx)
	select {
	case msg, ok := <-disconnectedMsgChan:
		if ok {
			t.Errorf("Expected closed channel from disconnected client, but received: %v", msg)
		}
		// Channel should be closed immediately
	case <-time.After(50 * time.Millisecond):
		t.Error("Expected immediate closed channel from disconnected client")
	}
}

// TestClientResponseIterator tests response iteration through MessageIterator
// Covers T138: Client Response Iterator
func TestClientResponseIterator(t *testing.T) {
	ctx, cancel := setupClientTestContext(t, 10*time.Second)
	defer cancel()

	transport := newClientMockTransport()
	client := setupClientForTest(t, transport)
	defer disconnectClientSafely(t, client)

	connectClientSafely(ctx, t, client)

	// Get response iterator from client
	iter := client.ReceiveResponse(ctx)
	if iter == nil {
		t.Fatal("Expected MessageIterator, got nil")
	}

	// Inject test messages for iterator testing
	transport.injectTestMessage(&AssistantMessage{
		Content: []ContentBlock{&TextBlock{Text: "First response"}},
		Model:   "claude-3-5-sonnet-20241022",
	})
	transport.injectTestMessage(&AssistantMessage{
		Content: []ContentBlock{&TextBlock{Text: "Second response"}},
		Model:   "claude-3-5-sonnet-20241022",
	})

	// Iterate through messages using iterator
	receivedCount := 0
	expectedTexts := []string{"First response", "Second response"}

	for i := 0; i < len(expectedTexts); i++ {
		msg, err := iter.Next(ctx)
		if err != nil {
			t.Fatalf("Iterator error: %v", err)
		}

		if msg == nil {
			t.Fatal("Expected message from iterator, got nil")
		}

		assistantMsg, ok := msg.(*AssistantMessage)
		if !ok {
			t.Errorf("Expected AssistantMessage, got %T", msg)
			continue
		}

		if len(assistantMsg.Content) != 1 {
			t.Errorf("Expected 1 content block, got %d", len(assistantMsg.Content))
			continue
		}

		textBlock, ok := assistantMsg.Content[0].(*TextBlock)
		if !ok {
			t.Errorf("Expected TextBlock, got %T", assistantMsg.Content[0])
			continue
		}

		if textBlock.Text != expectedTexts[i] {
			t.Errorf("Expected '%s', got '%s'", expectedTexts[i], textBlock.Text)
		}

		receivedCount++
	}

	if receivedCount != len(expectedTexts) {
		t.Errorf("Expected %d messages, received %d", len(expectedTexts), receivedCount)
	}
}

// TestClientReceiveResponseNotConnected tests that ReceiveResponse returns a usable
// (non-nil) iterator even when the client is not connected.
func TestClientReceiveResponseNotConnected(t *testing.T) {
	ctx, cancel := setupClientTestContext(t, 5*time.Second)
	defer cancel()

	transport := newClientMockTransport()
	client := setupClientForTest(t, transport)
	// Intentionally do NOT connect

	iter := client.ReceiveResponse(ctx)
	if iter == nil {
		t.Fatal("ReceiveResponse returned nil on disconnected client; expected a closed iterator")
	}

	msg, err := iter.Next(ctx)
	if err != ErrNoMoreMessages {
		t.Errorf("Expected ErrNoMoreMessages from closed iterator, got: %v", err)
	}
	if msg != nil {
		t.Errorf("Expected nil message from closed iterator, got: %v", msg)
	}
}

// TestClientInterrupt tests interrupt functionality during operations
// Covers T139: Client Interrupt Functionality
func TestClientInterrupt(t *testing.T) {
	ctx, cancel := setupClientTestContext(t, 10*time.Second)
	defer cancel()

	transport := newClientMockTransport()
	client := setupClientForTest(t, transport)
	defer disconnectClientSafely(t, client)

	connectClientSafely(ctx, t, client)

	// Test interrupt on connected client
	err := client.Interrupt(ctx)
	assertNoError(t, err)

	// Test interrupt propagation to transport
	if transport.interruptError != nil {
		t.Errorf("Transport interrupt should not have error by default, got: %v", transport.interruptError)
	}

	// Test interrupt with transport error
	transportWithError := newClientMockTransportWithOptions(WithClientInterruptError(fmt.Errorf("interrupt failed")))
	clientWithError := setupClientForTest(t, transportWithError)
	defer disconnectClientSafely(t, clientWithError)

	connectClientSafely(ctx, t, clientWithError)

	err = clientWithError.Interrupt(ctx)
	assertClientError(t, err, true, "interrupt failed")

	// Test interrupt during query operation
	longRunningTransport := newClientMockTransport()
	longRunningClient := setupClientForTest(t, longRunningTransport)
	defer disconnectClientSafely(t, longRunningClient)

	connectClientSafely(ctx, t, longRunningClient)

	// Use a channel to synchronize the goroutine
	done := make(chan error, 1)

	// Start a query operation
	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- fmt.Errorf("goroutine panicked: %v", r)
				return
			}
		}()
		time.Sleep(50 * time.Millisecond) // Let query start
		err := longRunningClient.Interrupt(ctx)
		done <- err
	}()

	// Execute query (interrupt should not prevent this from completing)
	err = longRunningClient.Query(ctx, "test query")
	assertNoError(t, err)

	// Wait for goroutine to complete before test ends
	select {
	case goroutineErr := <-done:
		if goroutineErr != nil {
			t.Errorf("Interrupt during operation failed: %v", goroutineErr)
		}
	case <-time.After(200 * time.Millisecond):
		t.Error("Timeout waiting for interrupt goroutine to complete")
	}

	// Verify query was sent despite interrupt
	assertClientMessageCount(t, longRunningTransport, 1)
}

// TestClientSessionID tests session ID handling in client operations
// Covers T140: Client Session Management
func TestClientSessionID(t *testing.T) {
	ctx, cancel := setupClientTestContext(t, 10*time.Second)
	defer cancel()

	transport := newClientMockTransport()
	client := setupClientForTest(t, transport)
	defer disconnectClientSafely(t, client)

	connectClientSafely(ctx, t, client)

	// Test query with default session ID
	err := client.Query(ctx, "test message")
	assertNoError(t, err)

	// Verify default session ID was used
	sentMsg, ok := transport.getSentMessage(0)
	if !ok {
		t.Fatal("Failed to get sent message")
	}
	if sentMsg.SessionID != defaultSessionID {
		t.Errorf("Expected default session ID 'default', got '%s'", sentMsg.SessionID)
	}

	// Test query with custom session ID
	err = client.QueryWithSession(ctx, "test message 2", "custom-session")
	assertNoError(t, err)

	// Verify custom session ID was used
	sentMsg, ok = transport.getSentMessage(1)
	if !ok {
		t.Fatal("Failed to get second sent message")
	}
	if sentMsg.SessionID != "custom-session" {
		t.Errorf("Expected custom session ID 'custom-session', got '%s'", sentMsg.SessionID)
	}

	// Test query with empty session ID (should use default)
	err = client.QueryWithSession(ctx, "test message 3", "")
	assertNoError(t, err)

	// Verify default session ID was used for empty string
	sentMsg, ok = transport.getSentMessage(2)
	if !ok {
		t.Fatal("Failed to get third sent message")
	}
	if sentMsg.SessionID != defaultSessionID {
		t.Errorf("Expected default session ID for empty string, got '%s'", sentMsg.SessionID)
	}

	// Verify total message count
	assertClientMessageCount(t, transport, 3)
}

// TestClientMultipleSessions tests concurrent operations with different session IDs
// Covers T151: Client Multiple Sessions + T156: State Consistency
func TestClientMultipleSessions(t *testing.T) {
	ctx, cancel := setupClientTestContext(t, 15*time.Second)
	defer cancel()

	transport := newClientMockTransport()
	client := setupClientForTest(t, transport)
	defer disconnectClientSafely(t, client)

	connectClientSafely(ctx, t, client)

	// Test concurrent operations with different session IDs
	const numSessions = 3
	const queriesPerSession = 2
	sessionIDs := []string{"session-1", "session-2", "session-3"}

	var wg sync.WaitGroup
	errors := make(chan error, numSessions*queriesPerSession)

	// Launch concurrent operations for different sessions
	for i, sessionID := range sessionIDs {
		wg.Add(1)
		go func(id int, sess string) {
			defer wg.Done()
			for j := 0; j < queriesPerSession; j++ {
				err := client.QueryWithSession(ctx, fmt.Sprintf("query %d-%d", id, j), sess)
				if err != nil {
					errors <- fmt.Errorf("session %s query %d failed: %w", sess, j, err)
				}
			}
		}(i, sessionID)
	}

	wg.Wait()
	close(errors)

	// Check for any errors
	for err := range errors {
		t.Errorf("Concurrent session operation error: %v", err)
	}

	// Verify all messages were sent
	expectedMessageCount := numSessions * queriesPerSession
	assertClientMessageCount(t, transport, expectedMessageCount)

	// Verify session IDs were properly propagated
	sessionCounts := make(map[string]int)
	for i := 0; i < expectedMessageCount; i++ {
		sentMsg, ok := transport.getSentMessage(i)
		if !ok {
			t.Errorf("Failed to get sent message %d", i)
			continue
		}
		sessionCounts[sentMsg.SessionID]++
	}

	// Verify each session received the correct number of messages
	for _, sessionID := range sessionIDs {
		if sessionCounts[sessionID] != queriesPerSession {
			t.Errorf("Session %s: expected %d messages, got %d",
				sessionID, queriesPerSession, sessionCounts[sessionID])
		}
	}

	// Test state consistency: client should remain connected throughout
	if !transport.connected {
		t.Error("Expected client to remain connected after concurrent session operations")
	}

	// Test session isolation: different sessions should not interfere
	err := client.QueryWithSession(ctx, "final test", "session-1")
	assertNoError(t, err)

	// Verify the final message used correct session ID
	finalMsg, ok := transport.getSentMessage(expectedMessageCount)
	if !ok {
		t.Fatal("Failed to get final sent message")
	}
	if finalMsg.SessionID != "session-1" {
		t.Errorf("Expected final message session ID 'session-1', got '%s'", finalMsg.SessionID)
	}
}

// TestClientReconnection tests reconnection after transport failures
// Covers T150: Client Reconnection + T155: Error Recovery
func TestClientReconnection(t *testing.T) {
	ctx, cancel := setupClientTestContext(t, 5*time.Second)
	defer cancel()

	transport := newClientMockTransport()
	client := setupClientForTest(t, transport)
	defer disconnectClientSafely(t, client)

	// Initial connection
	connectClientSafely(ctx, t, client)
	err := client.Query(ctx, "test before disconnect")
	assertNoError(t, err)

	// Simulate disconnect and reconnect
	disconnectClientSafely(t, client)
	assertClientDisconnected(t, transport)
	transport.reset()
	connectClientSafely(ctx, t, client)

	// Test recovery after reconnection
	err = client.Query(ctx, "test after reconnect")
	assertNoError(t, err)
	assertClientMessageCount(t, transport, 1)
}

// TestClientAsyncErrorHandling tests async transport error scenarios
// Covers T142: Client Error Propagation + T155: Client Error Recovery
func TestClientAsyncErrorHandling(t *testing.T) {
	ctx, cancel := setupClientTestContext(t, 5*time.Second)
	defer cancel()

	// Test async error propagation
	asyncErr := fmt.Errorf("async transport failure")
	transport := newClientMockTransportWithOptions(WithClientAsyncError(asyncErr))
	client := setupClientForTest(t, transport)
	defer disconnectClientSafely(t, client)

	connectClientSafely(ctx, t, client)

	// Get error channel from ReceiveMessages
	_, errChan := transport.ReceiveMessages(ctx)

	// Should receive async error
	select {
	case receivedErr := <-errChan:
		if receivedErr.Error() != asyncErr.Error() {
			t.Errorf("Expected async error %q, got %q", asyncErr.Error(), receivedErr.Error())
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Expected to receive async error from errChan")
	}

	// Client should still be functional after async error
	err := client.Query(ctx, "test query after async error")
	assertNoError(t, err)
	assertClientMessageCount(t, transport, 1)
}

// TestClientQueryStreamSendError tests that QueryStream propagates send errors
// to the ReceiveResponse iterator rather than silently dropping them (C3).
func TestClientQueryStreamSendError(t *testing.T) {
	sendErr := fmt.Errorf("send failed")
	transport := newClientMockTransportWithOptions(WithClientSendError(sendErr))
	client := setupClientForTest(t, transport)
	defer disconnectClientSafely(t, client)

	ctx, cancel := setupClientTestContext(t, 5*time.Second)
	defer cancel()

	connectClientSafely(ctx, t, client)

	iter := client.ReceiveResponse(ctx)
	if iter == nil {
		t.Fatal("Expected non-nil iterator")
	}

	messages := make(chan StreamMessage, 1)
	messages <- StreamMessage{
		Type:    "user",
		Message: &UserMessage{Content: "hello"},
	}

	if err := client.QueryStream(ctx, messages); err != nil {
		t.Fatalf("QueryStream returned unexpected synchronous error: %v", err)
	}

	// The send error must be propagated to the iterator, not silently dropped.
	shortCtx, shortCancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer shortCancel()

	_, iterErr := iter.Next(shortCtx)
	if iterErr == nil {
		t.Fatal("Expected error from iterator after send failure, got nil")
	}
	if !strings.Contains(iterErr.Error(), "send failed") {
		t.Errorf("Expected error containing 'send failed', got: %v", iterErr)
	}
}

// TestClientCallsFailAfterProcessExit verifies that once the CLI process has
// exited, Done is closed, Err reports the exit, and every call that writes to
// the CLI fails with a *ConnectionError wrapping that exit error instead of
// being accepted (Python: CLIConnectionError raised from the exit error).
func TestClientCallsFailAfterProcessExit(t *testing.T) {
	ctx, cancel := setupClientTestContext(t, 10*time.Second)
	defer cancel()

	calls := []struct {
		name string
		call func(Client) error
	}{
		{"Query", func(c Client) error { return c.Query(ctx, "hello") }},
		{"QueryWithSession", func(c Client) error { return c.QueryWithSession(ctx, "hello", "s1") }},
		{"QueryStream", func(c Client) error {
			messages := make(chan StreamMessage, 1)
			messages <- StreamMessage{Type: userMessageType, Message: &UserMessage{Content: "hello"}}
			close(messages)
			return c.QueryStream(ctx, messages)
		}},
		{"Interrupt", func(c Client) error { return c.Interrupt(ctx) }},
		{"SetModel", func(c Client) error { model := testModelSonnet; return c.SetModel(ctx, &model) }},
		{"SetPermissionMode", func(c Client) error { return c.SetPermissionMode(ctx, PermissionModeAcceptEdits) }},
		{"RewindFiles", func(c Client) error { return c.RewindFiles(ctx, "uuid-1") }},
		{"GetMcpStatus", func(c Client) error { _, err := c.GetMcpStatus(ctx); return err }},
		{"StopTask", func(c Client) error { return c.StopTask(ctx, "task-1") }},
	}

	for _, test := range calls {
		t.Run(test.name, func(t *testing.T) {
			transport := newProcessMockTransport()
			client := setupClientForTest(t, transport)
			defer disconnectClientSafely(t, client)
			connectClientSafely(ctx, t, client)

			done := client.Done()
			assertChannelOpen(t, done, "Done() while the CLI runs")
			if err := client.Err(); err != nil {
				t.Fatalf("Err() = %v while the CLI runs, want nil", err)
			}

			exitErr := NewProcessError("Claude Code process exited unexpectedly (signal: killed)", -1, "")
			transport.exit(exitErr)

			assertChannelClosed(t, done, "Done() after the CLI exited")
			if err := client.Err(); !errors.Is(err, exitErr) {
				t.Fatalf("Err() = %v, want the exit error", err)
			}

			err := test.call(client)
			if !errors.Is(err, exitErr) || !IsConnectionError(err) {
				t.Fatalf("%s on a dead client = %v, want a *ConnectionError wrapping the exit error", test.name, err)
			}
			assertClientMessageCount(t, transport.clientMockTransport, 0)
		})
	}
}

// TestClientDoneAndErrLifecycle verifies Done and Err before Connect, while
// connected, after Disconnect and after a reconnect. A transport that does not
// report its process gets a Done that closes on Disconnect.
func TestClientDoneAndErrLifecycle(t *testing.T) {
	ctx, cancel := setupClientTestContext(t, 10*time.Second)
	defer cancel()

	tests := []struct {
		name      string
		transport func() Transport
	}{
		{"process_transport", func() Transport { return newProcessMockTransport() }},
		{"custom_transport_fallback", func() Transport { return newClientMockTransport() }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := setupClientForTest(t, test.transport())

			assertChannelClosed(t, client.Done(), "Done() before Connect")
			assertClientError(t, client.Err(), true, "client not connected")

			connectClientSafely(ctx, t, client)
			done := client.Done()
			assertChannelOpen(t, done, "Done() while connected")
			assertNoError(t, client.Err())

			disconnectClientSafely(t, client)
			assertChannelClosed(t, done, "Done() of the connection after Disconnect")
			assertChannelClosed(t, client.Done(), "Done() after Disconnect")
			assertClientError(t, client.Err(), true, "client not connected")

			connectClientSafely(ctx, t, client)
			defer disconnectClientSafely(t, client)
			assertChannelOpen(t, client.Done(), "Done() after reconnecting")
			assertNoError(t, client.Err())
		})
	}
}

// TestClientDoneAndErrDoNotBlockDuringDisconnect verifies that Done and Err
// return at once while Disconnect waits for a slow transport Close.
func TestClientDoneAndErrDoNotBlockDuringDisconnect(t *testing.T) {
	ctx, cancel := setupClientTestContext(t, 10*time.Second)
	defer cancel()

	transport := &blockingCloseTransport{
		processMockTransport: newProcessMockTransport(),
		closing:              make(chan struct{}),
		release:              make(chan struct{}),
	}
	client := setupClientForTest(t, transport)
	connectClientSafely(ctx, t, client)

	disconnected := make(chan error, 1)
	go func() { disconnected <- client.Disconnect() }()
	<-transport.closing

	returned := make(chan struct{})
	go func() {
		_ = client.Done()
		_ = client.Err()
		close(returned)
	}()
	select {
	case <-returned:
	case <-time.After(2 * time.Second):
		t.Error("Done() or Err() blocked while Disconnect ran")
	}

	close(transport.release)
	if err := <-disconnected; err != nil {
		t.Fatalf("Disconnect() = %v", err)
	}
}

// TestClientErrDuringDisconnect verifies that Err, called while Disconnect
// waits for Close, never returns the transport's own not-connected error.
// The mock follows the subprocess transport: the process exits first, then
// cleanup clears the exit state, then Close returns.
func TestClientErrDuringDisconnect(t *testing.T) {
	ctx, cancel := setupClientTestContext(t, 10*time.Second)
	defer cancel()

	transport := &cleanupThenBlockTransport{
		processMockTransport: newProcessMockTransport(),
		cleaned:              make(chan struct{}),
		release:              make(chan struct{}),
	}
	client := setupClientForTest(t, transport)
	connectClientSafely(ctx, t, client)
	done := client.Done()

	disconnected := make(chan error, 1)
	go func() { disconnected <- client.Disconnect() }()
	<-transport.cleaned

	assertChannelClosed(t, done, "Done() of the connection while Disconnect runs")
	err := client.Err()
	if !errors.Is(err, ErrNotConnected) || !IsConnectionError(err) {
		t.Errorf("Err() during Disconnect = %v, want a *ConnectionError wrapping ErrNotConnected", err)
	}

	close(transport.release)
	if err := <-disconnected; err != nil {
		t.Fatalf("Disconnect() = %v", err)
	}
}

// TestClientFailedDisconnectKeepsWatch verifies that Done and Err still
// follow the connection when Close fails and the client stays connected.
func TestClientFailedDisconnectKeepsWatch(t *testing.T) {
	ctx, cancel := setupClientTestContext(t, 10*time.Second)
	defer cancel()

	transport := newProcessMockTransport()
	client := setupClientForTest(t, transport)
	connectClientSafely(ctx, t, client)

	transport.mu.Lock()
	transport.closeError = errors.New("close failed")
	transport.mu.Unlock()
	if err := client.Disconnect(); err == nil {
		t.Fatal("Disconnect() = nil, want the close error")
	}

	assertChannelOpen(t, client.Done(), "Done() after a failed Disconnect")
	if err := client.Err(); err != nil {
		t.Errorf("Err() after a failed Disconnect = %v, want nil", err)
	}

	transport.mu.Lock()
	transport.closeError = nil
	transport.mu.Unlock()
	disconnectClientSafely(t, client)
}

// TestClientNotConnectedErrors verifies that every call on a client that is
// not connected returns a *ConnectionError that wraps ErrNotConnected.
func TestClientNotConnectedErrors(t *testing.T) {
	ctx, cancel := setupClientTestContext(t, 10*time.Second)
	defer cancel()

	calls := []struct {
		name string
		call func(Client) error
	}{
		{"Query", func(c Client) error { return c.Query(ctx, "hello") }},
		{"QueryWithSession", func(c Client) error { return c.QueryWithSession(ctx, "hello", "s1") }},
		{"QueryStream", func(c Client) error { return c.QueryStream(ctx, make(chan StreamMessage)) }},
		{"Interrupt", func(c Client) error { return c.Interrupt(ctx) }},
		{"SetModel", func(c Client) error { model := testModelSonnet; return c.SetModel(ctx, &model) }},
		{"SetPermissionMode", func(c Client) error { return c.SetPermissionMode(ctx, PermissionModeAcceptEdits) }},
		{"RewindFiles", func(c Client) error { return c.RewindFiles(ctx, "uuid-1") }},
		{"GetMcpStatus", func(c Client) error { _, err := c.GetMcpStatus(ctx); return err }},
		{"StopTask", func(c Client) error { return c.StopTask(ctx, "task-1") }},
		{"GetServerInfo", func(c Client) error { _, err := c.GetServerInfo(ctx); return err }},
		{"Err", func(c Client) error { return c.Err() }},
	}

	states := []struct {
		name  string
		setup func(t *testing.T, c Client)
	}{
		{"before_connect", func(*testing.T, Client) {}},
		{"after_disconnect", func(t *testing.T, c Client) {
			connectClientSafely(ctx, t, c)
			disconnectClientSafely(t, c)
		}},
	}

	for _, state := range states {
		for _, test := range calls {
			t.Run(state.name+"/"+test.name, func(t *testing.T) {
				client := setupClientForTest(t, newClientMockTransport())
				state.setup(t, client)

				err := test.call(client)
				if !errors.Is(err, ErrNotConnected) || !IsConnectionError(err) {
					t.Fatalf("%s = %v, want a *ConnectionError wrapping ErrNotConnected", test.name, err)
				}
			})
		}
	}
}

// TestErrProtocolClosedIsExported verifies that callers can match the
// control protocol's closed error with errors.Is.
func TestErrProtocolClosedIsExported(t *testing.T) {
	err := fmt.Errorf("interrupt: %w", control.ErrProtocolClosed)
	if !errors.Is(err, ErrProtocolClosed) {
		t.Fatalf("errors.Is(%v, ErrProtocolClosed) = false", err)
	}
}

// TestStreamReaderSharedPendingError verifies that when two iterators share
// one pending error, one gets it and the other keeps reading: neither gets
// (nil, nil).
func TestStreamReaderSharedPendingError(t *testing.T) {
	for i := 0; i < 50; i++ {
		stream := newStreamReader(make(chan Message), make(chan error))
		exitErr := errors.New("exit")
		stream.setPending(exitErr)

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		results := make(chan error, 2)
		for g := 0; g < 2; g++ {
			go func() {
				msg, err := stream.next(ctx, nil)
				if msg == nil && err == nil {
					results <- errors.New("next returned (nil, nil)")
					return
				}
				results <- err
			}()
		}
		first, second := <-results, <-results
		cancel()

		gotExit := errors.Is(first, exitErr) != errors.Is(second, exitErr)
		gotCtx := errors.Is(first, context.DeadlineExceeded) != errors.Is(second, context.DeadlineExceeded)
		if !gotExit || !gotCtx {
			t.Fatalf("iteration %d: results %v and %v, want the exit error once and a deadline error once", i, first, second)
		}
	}
}

// TestSubprocessTransportReportsProcessExit guards the optional interface
// that Done and Err type-assert: if the subprocess transport stopped
// satisfying it, Done would silently close only on Disconnect.
func TestSubprocessTransportReportsProcessExit(t *testing.T) {
	var transport Transport = subprocess.New("claude", NewOptions(), "sdk-go-client")
	if _, ok := transport.(processWatcher); !ok {
		t.Fatal("subprocess.Transport does not implement processWatcher")
	}
}

// TestClientResponseSequencing tests pre-configured response sequences
// Covers T137: Client Message Reception + T138: Client Response Iterator + T147: Client Message Ordering
func TestClientResponseSequencing(t *testing.T) {
	ctx, cancel := setupClientTestContext(t, 5*time.Second)
	defer cancel()

	// Create pre-configured response sequence
	testMessages := []Message{
		&AssistantMessage{
			Content: []ContentBlock{&TextBlock{Text: "First response"}},
			Model:   "claude-3-5-sonnet-20241022",
		},
		&AssistantMessage{
			Content: []ContentBlock{&TextBlock{Text: "Second response"}},
			Model:   "claude-3-5-sonnet-20241022",
		},
		&AssistantMessage{
			Content: []ContentBlock{&TextBlock{Text: "Third response"}},
			Model:   "claude-3-5-sonnet-20241022",
		},
	}

	transport := newClientMockTransportWithOptions(WithClientResponseMessages(testMessages))
	client := setupClientForTest(t, transport)
	defer disconnectClientSafely(t, client)

	connectClientSafely(ctx, t, client)

	// Get message channel from ReceiveMessages
	msgChan, _ := transport.ReceiveMessages(ctx)

	// Should receive messages in correct order
	expectedTexts := []string{"First response", "Second response", "Third response"}
	for i, expectedText := range expectedTexts {
		select {
		case msg := <-msgChan:
			assistantMsg, ok := msg.(*AssistantMessage)
			if !ok {
				t.Fatalf("Expected AssistantMessage at index %d, got %T", i, msg)
			}

			if len(assistantMsg.Content) != 1 {
				t.Fatalf("Expected 1 content block at index %d, got %d", i, len(assistantMsg.Content))
			}

			textBlock, ok := assistantMsg.Content[0].(*TextBlock)
			if !ok {
				t.Fatalf("Expected TextBlock at index %d, got %T", i, assistantMsg.Content[0])
			}

			if textBlock.Text != expectedText {
				t.Errorf("Expected message %d to be %q, got %q", i, expectedText, textBlock.Text)
			}

		case <-time.After(100 * time.Millisecond):
			t.Fatalf("Timeout waiting for message %d", i)
		}
	}

	// Wait a bit longer for any potential extra messages, then verify no more
	extraMessageCount := 0
	timeout := time.After(50 * time.Millisecond)

	for {
		select {
		case msg := <-msgChan:
			extraMessageCount++
			t.Logf("Received unexpected extra message %d: %T", extraMessageCount, msg)
		case <-timeout:
			if extraMessageCount > 0 {
				t.Errorf("Expected exactly 3 messages, but received %d extra messages", extraMessageCount)
			}
			return // Exit the test - expected behavior
		}
	}
}

// TestClientGracefulShutdown tests proper shutdown and configuration
// Covers T154: Graceful Shutdown + T153: Memory Management + T160: Option Order + T163: Protocol Compliance
func TestClientGracefulShutdown(t *testing.T) {
	ctx, cancel := setupClientTestContext(t, 5*time.Second)
	defer cancel()

	// Test option precedence (T160)
	transport := newClientMockTransport()
	client := NewClientWithTransport(transport,
		WithSystemPrompt("first"),
		WithSystemPrompt("second"), // Should override first
		WithAllowedTools("Read"),
	)
	defer disconnectClientSafely(t, client)

	connectClientSafely(ctx, t, client)

	// Test protocol compliance (T163) - messages should be properly formatted
	err := client.Query(ctx, "test message")
	assertNoError(t, err)

	sentMsg, ok := transport.getSentMessage(0)
	if !ok {
		t.Fatal("Failed to get sent message")
	}
	if sentMsg.Type != userMessageType {
		t.Errorf("Expected message type 'user', got '%s'", sentMsg.Type)
	}

	// Test memory management (T153) - multiple operations should not leak
	for i := 0; i < 5; i++ {
		err := client.Query(ctx, fmt.Sprintf("memory test %d", i))
		assertNoError(t, err)
	}

	// Test graceful shutdown (T154) - disconnect should clean up resources
	disconnectClientSafely(t, client)
	assertClientDisconnected(t, transport)
}

// TestNewClient tests the NewClient constructor function
func TestNewClient(t *testing.T) {
	// Note: With direct transport creation, we test the constructor logic
	// without mocking the factory. Connect() will be tested separately with
	// proper transport mocking at the subprocess level.

	tests := []struct {
		name    string
		options []Option
		verify  func(t *testing.T, client Client)
	}{
		{
			name:    "default_client",
			options: nil,
			verify: func(t *testing.T, client Client) {
				t.Helper()
				if client == nil {
					t.Fatal("Expected client to be created")
				}
				// Test constructor creates client without errors
				// (Connection testing done separately with transport mocks)
			},
		},
		{
			name:    "client_with_system_prompt",
			options: []Option{WithSystemPrompt("Test system prompt")},
			verify: func(t *testing.T, client Client) {
				t.Helper()
				if client == nil {
					t.Fatal("Expected client to be created with system prompt")
				}
				// Test constructor accepts system prompt option
			},
		},
		{
			name: "client_with_multiple_options",
			options: []Option{
				WithSystemPrompt("Multi-option test"),
				WithAllowedTools("Read", "Write"),
				WithModel("claude-sonnet-3-5-20241022"),
			},
			verify: func(t *testing.T, client Client) {
				t.Helper()
				if client == nil {
					t.Fatal("Expected client to be created with multiple options")
				}
				// Test constructor accepts multiple options
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := NewClient(test.options...)
			defer disconnectClientSafely(t, client)

			test.verify(t, client)
		})
	}

	// Note: Error cases for Connect() are tested in TestClientErrorHandling
	// with proper transport mocking
}

// TestClientIteratorClose tests the clientIterator Close method - consolidated
func TestClientIteratorClose(t *testing.T) {
	iteratorTests := map[string]iteratorCloseTest{
		"close_unused":            {"unused", false, false, false},
		"close_with_messages":     {"with_messages", true, false, false},
		"multiple_close_calls":    {"multiple_close", false, false, true},
		"close_after_consumption": {"after_consumption", true, true, false},
	}

	for name, test := range iteratorTests {
		t.Run(name, func(t *testing.T) {
			var transport *clientMockTransport
			if test.needQuery {
				transport = newClientMockTransportWithOptions(WithClientResponseMessages([]Message{
					&AssistantMessage{Content: []ContentBlock{&TextBlock{Text: "response1"}}, Model: "claude-sonnet-3-5-20241022"},
					&AssistantMessage{Content: []ContentBlock{&TextBlock{Text: "response2"}}, Model: "claude-sonnet-3-5-20241022"},
				}))
			} else {
				transport = newClientMockTransport()
			}

			client := setupClientForTest(t, transport)
			defer disconnectClientSafely(t, client)

			verifyIteratorClose(t, client, transport, test)
		})
	}
}

type clientMockTransport struct {
	mu           sync.Mutex
	connected    bool
	closed       bool
	sentMessages []StreamMessage

	// Minimal message support for essential tests
	testMessages []Message
	msgChan      chan Message
	errChan      chan error

	// Error injection for testing
	connectError           error
	sendError              error
	interruptError         error
	closeError             error
	asyncError             error // For async error testing
	setModelError          error
	setPermissionModeError error
	rewindFilesError       error
	getMcpStatusError      error
	getMcpStatusResponse   *McpStatusResponse
	stopTaskError          error
	stoppedTaskIDs         []string
}

func (c *clientMockTransport) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check context cancellation first
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if c.connectError != nil {
		return c.connectError
	}

	// For testing flexibility, allow reconnection of closed transports
	if c.closed {
		c.closed = false
	}

	c.connected = true
	return nil
}

func (c *clientMockTransport) SendMessage(ctx context.Context, message StreamMessage) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check context cancellation first
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if c.sendError != nil {
		return c.sendError
	}
	if !c.connected {
		return fmt.Errorf("not connected")
	}
	c.sentMessages = append(c.sentMessages, message)
	return nil
}

func (c *clientMockTransport) ReceiveMessages(_ context.Context) (msgChan <-chan Message, errChan <-chan error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		closedMsgChan := make(chan Message)
		closedErrChan := make(chan error)
		close(closedMsgChan)
		close(closedErrChan)
		return closedMsgChan, closedErrChan
	}

	// Initialize channels if not already done
	if c.msgChan == nil {
		c.msgChan = make(chan Message, 10)
		c.errChan = make(chan error, 10)

		// Send any pre-configured messages immediately
		for _, msg := range c.testMessages {
			c.msgChan <- msg
		}

		// Send async error if configured
		if c.asyncError != nil {
			c.errChan <- c.asyncError
		}
	}

	return c.msgChan, c.errChan
}

func (c *clientMockTransport) Interrupt(_ context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.interruptError != nil {
		return c.interruptError
	}
	return nil
}

func (c *clientMockTransport) EndInput(_ context.Context) error {
	return nil
}

func (c *clientMockTransport) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closeError != nil {
		return c.closeError
	}

	if c.closed {
		return nil // Already closed
	}

	c.connected = false
	c.closed = true

	// Close channels if they exist
	if c.msgChan != nil {
		close(c.msgChan)
		c.msgChan = nil
	}
	if c.errChan != nil {
		close(c.errChan)
		c.errChan = nil
	}

	return nil
}

// Helper methods
func (c *clientMockTransport) getSentMessageCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.sentMessages)
}

func (c *clientMockTransport) getSentMessage(index int) (StreamMessage, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if index < 0 || index >= len(c.sentMessages) {
		return StreamMessage{}, false
	}
	return c.sentMessages[index], true
}

func (c *clientMockTransport) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sentMessages = nil
	c.connected = false
	c.closed = false
}

// Simplified message injection helper
func (c *clientMockTransport) injectTestMessage(msg Message) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.testMessages == nil {
		c.testMessages = []Message{}
	}
	c.testMessages = append(c.testMessages, msg)
	if c.msgChan != nil {
		select {
		case c.msgChan <- msg:
		default:
		}
	}
}

func (c *clientMockTransport) GetValidator() *StreamValidator {
	return &StreamValidator{}
}

func (c *clientMockTransport) SetModel(_ context.Context, _ *string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.setModelError != nil {
		return c.setModelError
	}
	return nil
}

func (c *clientMockTransport) SetPermissionMode(_ context.Context, _ PermissionMode) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.setPermissionModeError != nil {
		return c.setPermissionModeError
	}
	return nil
}

func (c *clientMockTransport) RewindFiles(_ context.Context, _ string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.rewindFilesError != nil {
		return c.rewindFilesError
	}
	return nil
}

func (c *clientMockTransport) GetMcpStatus(_ context.Context) (*McpStatusResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.getMcpStatusError != nil {
		return nil, c.getMcpStatusError
	}
	if c.getMcpStatusResponse != nil {
		return c.getMcpStatusResponse, nil
	}
	return &McpStatusResponse{McpServers: []McpServerStatus{}}, nil
}

// processMockTransport is a clientMockTransport that reports the exit of its
// CLI process through Done and Err, like the subprocess transport.
type processMockTransport struct {
	*clientMockTransport
	procMu  sync.Mutex
	done    chan struct{}
	exitErr error
}

func newProcessMockTransport() *processMockTransport {
	return &processMockTransport{clientMockTransport: newClientMockTransport()}
}

func (p *processMockTransport) Connect(ctx context.Context) error {
	if err := p.clientMockTransport.Connect(ctx); err != nil {
		return err
	}
	p.procMu.Lock()
	defer p.procMu.Unlock()
	p.done = make(chan struct{})
	p.exitErr = nil
	return nil
}

// exit simulates the CLI process exiting with err.
func (p *processMockTransport) exit(err error) {
	p.procMu.Lock()
	defer p.procMu.Unlock()
	if p.done == nil || p.exitErr != nil {
		return
	}
	p.exitErr = err
	close(p.done)
}

func (p *processMockTransport) Close() error {
	if err := p.clientMockTransport.Close(); err != nil {
		return err
	}
	p.exit(errors.New("transport closed"))
	return nil
}

func (p *processMockTransport) Done() <-chan struct{} {
	p.procMu.Lock()
	defer p.procMu.Unlock()
	return p.done
}

func (p *processMockTransport) Err() error {
	p.procMu.Lock()
	defer p.procMu.Unlock()
	return p.exitErr
}

// blockingCloseTransport is a processMockTransport whose Close signals
// closing and then waits for release.
type blockingCloseTransport struct {
	*processMockTransport
	closing chan struct{}
	release chan struct{}
}

func (b *blockingCloseTransport) Close() error {
	close(b.closing)
	<-b.release
	return b.processMockTransport.Close()
}

// cleanupThenBlockTransport is a processMockTransport whose Close ends the
// process, then reports a plain not-connected Err like a cleaned-up
// subprocess transport, then signals cleaned and waits for release.
type cleanupThenBlockTransport struct {
	*processMockTransport
	cleaned chan struct{}
	release chan struct{}

	stateMu sync.Mutex
	cleared bool
}

func (c *cleanupThenBlockTransport) Close() error {
	c.exit(NewProcessError("Claude Code process exited unexpectedly (signal: terminated)", -1, ""))
	c.stateMu.Lock()
	c.cleared = true
	c.stateMu.Unlock()
	close(c.cleaned)
	<-c.release
	return c.clientMockTransport.Close()
}

func (c *cleanupThenBlockTransport) Err() error {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	if c.cleared {
		return errors.New("transport not connected")
	}
	return c.processMockTransport.Err()
}

// serverInfoTransport is a clientMockTransport that keeps an initialize
// response, like the subprocess transport.
type serverInfoTransport struct {
	*clientMockTransport
}

func newServerInfoTransport() Transport {
	return &serverInfoTransport{clientMockTransport: newClientMockTransport()}
}

func (s *serverInfoTransport) InitializationResult() map[string]interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.connected {
		return nil
	}
	return testInitializeResponse()
}

// testInitializeResponse is an initialize response in the shape the CLI sends.
func testInitializeResponse() map[string]interface{} {
	return map[string]interface{}{
		"commands":                []interface{}{map[string]interface{}{"name": "compact", "description": "Compact the conversation"}},
		"output_style":            "default",
		"available_output_styles": []interface{}{"default", "Explanatory"},
		"models": []interface{}{
			map[string]interface{}{
				"value": "opus[1m]", "displayName": "Opus (1M context)", "description": "Most capable",
				"supportsEffort": true, "supportedEffortLevels": []interface{}{"low", "high"},
			},
		},
	}
}

func (c *clientMockTransport) StopTask(_ context.Context, taskID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stopTaskError != nil {
		return c.stopTaskError
	}
	c.stoppedTaskIDs = append(c.stoppedTaskIDs, taskID)
	return nil
}

func (c *clientMockTransport) getStoppedTaskIDs() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.stoppedTaskIDs...)
}

// Streamlined Mock Transport Options - reduced from 11 to 6 essential functions
type ClientMockTransportOption func(*clientMockTransport)

func WithClientConnectError(err error) ClientMockTransportOption {
	return func(t *clientMockTransport) { t.connectError = err }
}

func WithClientSendError(err error) ClientMockTransportOption {
	return func(t *clientMockTransport) { t.sendError = err }
}

func WithClientInterruptError(err error) ClientMockTransportOption {
	return func(t *clientMockTransport) { t.interruptError = err }
}

func WithClientAsyncError(err error) ClientMockTransportOption {
	return func(t *clientMockTransport) { t.asyncError = err }
}

func WithClientResponseMessages(messages []Message) ClientMockTransportOption {
	return func(t *clientMockTransport) { t.testMessages = messages }
}

func WithClientSetModelError(err error) ClientMockTransportOption {
	return func(t *clientMockTransport) { t.setModelError = err }
}

func WithClientSetPermissionModeError(err error) ClientMockTransportOption {
	return func(t *clientMockTransport) { t.setPermissionModeError = err }
}

func WithClientRewindFilesError(err error) ClientMockTransportOption {
	return func(t *clientMockTransport) { t.rewindFilesError = err }
}

func WithClientGetMcpStatusError(err error) ClientMockTransportOption {
	return func(t *clientMockTransport) { t.getMcpStatusError = err }
}

func WithClientGetMcpStatusResponse(resp *McpStatusResponse) ClientMockTransportOption {
	return func(t *clientMockTransport) { t.getMcpStatusResponse = resp }
}

func WithClientStopTaskError(err error) ClientMockTransportOption {
	return func(t *clientMockTransport) { t.stopTaskError = err }
}

// Factory Functions - streamlined creation methods
func newClientMockTransport() *clientMockTransport {
	return &clientMockTransport{}
}

func newClientMockTransportWithOptions(options ...ClientMockTransportOption) *clientMockTransport {
	transport := &clientMockTransport{}
	for _, option := range options {
		option(transport)
	}
	return transport
}

// Convenience factory methods for common error scenarios
func newMockTransportWithError(errorType string, err error) *clientMockTransport {
	transport := newClientMockTransport()
	switch errorType {
	case "connect":
		transport.connectError = err
	case "send":
		transport.sendError = err
	case "interrupt":
		transport.interruptError = err
	case "async":
		transport.asyncError = err
	}
	return transport
}

// Helper Functions
func setupClientTestContext(t *testing.T, timeout time.Duration) (context.Context, context.CancelFunc) {
	t.Helper()
	return context.WithTimeout(context.Background(), timeout)
}

func setupClientForTest(t *testing.T, transport Transport) Client {
	t.Helper()
	return NewClientWithTransport(transport)
}

func connectClientSafely(ctx context.Context, t *testing.T, client Client) {
	t.Helper()
	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Client connect failed: %v", err)
	}
}

func disconnectClientSafely(t *testing.T, client Client) {
	t.Helper()
	if err := client.Disconnect(); err != nil {
		t.Errorf("Client disconnect failed: %v", err)
	}
}

// Assertion helpers with t.Helper()
func assertClientConnected(t *testing.T, transport *clientMockTransport) {
	t.Helper()
	transport.mu.Lock()
	connected := transport.connected
	transport.mu.Unlock()
	if !connected {
		t.Error("Expected transport to be connected")
	}
}

func assertClientDisconnected(t *testing.T, transport *clientMockTransport) {
	t.Helper()
	transport.mu.Lock()
	connected := transport.connected
	closed := transport.closed
	transport.mu.Unlock()
	if connected {
		t.Errorf("Expected transport to be disconnected, but connected=%t, closed=%t", connected, closed)
	}
}

func assertClientError(t *testing.T, err error, wantErr bool, msgContains string) {
	t.Helper()
	if (err != nil) != wantErr {
		t.Errorf("error = %v, wantErr %v", err, wantErr)
		return
	}
	if wantErr && msgContains != "" && !strings.Contains(err.Error(), msgContains) {
		t.Errorf("error = %v, expected message to contain %q", err, msgContains)
	}
}

func assertClientMessageCount(t *testing.T, transport *clientMockTransport, expected int) {
	t.Helper()
	actual := transport.getSentMessageCount()
	if actual != expected {
		t.Errorf("Expected %d sent messages, got %d", expected, actual)
	}
}

func assertChannelOpen(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
		t.Fatalf("%s is closed, want open", what)
	default:
	}
}

func assertChannelClosed(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatalf("%s is open, want closed", what)
	}
}

// Helper for success-only assertions - replaces verbose assertNoError(t, err)
func assertNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
}

// Configuration verification helper - consolidated from 8 redundant functions
type clientConfigTest struct {
	name         string
	messageCount int
	sessionID    string
	queryText    string
	validateFn   func(*testing.T, *clientMockTransport)
}

func verifyClientConfiguration(t *testing.T, client Client, transport *clientMockTransport, config clientConfigTest) {
	t.Helper()
	ctx, cancel := setupClientTestContext(t, 5*time.Second)
	defer cancel()

	if client == nil {
		t.Fatalf("Expected client to be created for %s", config.name)
	}

	connectClientSafely(ctx, t, client)

	// Execute queries based on message count
	for i := 0; i < config.messageCount; i++ {
		queryText := config.queryText
		if config.messageCount > 1 {
			queryText = fmt.Sprintf("%s %d", config.queryText, i+1)
		}

		var err error
		if config.sessionID != "" {
			err = client.QueryWithSession(ctx, queryText, config.sessionID)
		} else {
			err = client.Query(ctx, queryText)
		}
		assertNoError(t, err)
	}

	assertClientMessageCount(t, transport, config.messageCount)

	// Apply custom validation if provided
	if config.validateFn != nil {
		config.validateFn(t, transport)
	}
}

// Specific validation functions for different config types
func verifyDefaultConfiguration(t *testing.T, client Client, transport *clientMockTransport) {
	t.Helper()
	verifyClientConfiguration(t, client, transport, clientConfigTest{
		name:         "default_configuration",
		messageCount: 1,
		queryText:    "default test",
		validateFn: func(t *testing.T, tr *clientMockTransport) {
			sentMsg, ok := tr.getSentMessage(0)
			if !ok {
				t.Fatal("Expected sent message")
			}
			if sentMsg.SessionID != defaultSessionID {
				t.Errorf("Expected default session ID 'default', got %q", sentMsg.SessionID)
			}
		},
	})
}

func verifySystemPromptConfig(t *testing.T, client Client, transport *clientMockTransport) {
	t.Helper()
	verifyClientConfiguration(t, client, transport, clientConfigTest{
		name:         "system_prompt_configuration",
		messageCount: 1,
		queryText:    "test with system prompt",
	})
}

func verifyToolsConfig(t *testing.T, client Client, transport *clientMockTransport) {
	t.Helper()
	verifyClientConfiguration(t, client, transport, clientConfigTest{
		name:         "tools_configuration",
		messageCount: 1,
		queryText:    "test with tools config",
	})
}

func verifyOptionsConfig(t *testing.T, client Client, transport *clientMockTransport) {
	t.Helper()
	verifyClientConfiguration(t, client, transport, clientConfigTest{
		name:         "multiple_options",
		messageCount: 1,
		queryText:    "test option precedence",
	})
}

func verifyComplexConfig(t *testing.T, client Client, transport *clientMockTransport) {
	t.Helper()
	verifyClientConfiguration(t, client, transport, clientConfigTest{
		name:         "complex_configuration",
		messageCount: 2,
		queryText:    "complex query",
		validateFn: func(t *testing.T, tr *clientMockTransport) {
			for i := 0; i < 2; i++ {
				sentMsg, ok := tr.getSentMessage(i)
				if !ok {
					t.Fatalf("Expected sent message %d", i)
				}
				if sentMsg.Type != userMessageType {
					t.Errorf("Expected message type 'user', got %q", sentMsg.Type)
				}
			}
		},
	})
}

func verifySessionConfig(t *testing.T, client Client, transport *clientMockTransport) {
	t.Helper()
	verifyClientConfiguration(t, client, transport, clientConfigTest{
		name:         "session_configuration",
		messageCount: 1,
		sessionID:    "custom-session-456",
		queryText:    "session test",
		validateFn: func(t *testing.T, tr *clientMockTransport) {
			sentMsg, ok := tr.getSentMessage(0)
			if !ok {
				t.Fatal("Expected sent message")
			}
			if sentMsg.SessionID != "custom-session-456" {
				t.Errorf("Expected session ID 'custom-session-456', got %q", sentMsg.SessionID)
			}
		},
	})
}

// verifyValidationError verifies that client creation fails due to validation errors
func verifyValidationError(t *testing.T, client Client, _ *clientMockTransport) {
	t.Helper()
	ctx, cancel := setupClientTestContext(t, 5*time.Second)
	defer cancel()

	// Connection should fail due to validation error
	err := client.Connect(ctx)
	if err == nil {
		t.Error("Expected validation error to prevent connection")
	} else {
		// Verify it's a validation error (contains expected validation messages)
		errStr := err.Error()
		if !strings.Contains(errStr, "max_turns must be non-negative") &&
			!strings.Contains(errStr, "working directory does not exist") {
			t.Errorf("Expected validation error, got: %v", err)
		}
	}
}

// Iterator verification helper - consolidated from 4 redundant functions
type iteratorCloseTest struct {
	name          string
	needQuery     bool
	consumeFirst  bool
	multipleCalls bool
}

func verifyIteratorClose(t *testing.T, client Client, _ *clientMockTransport, test iteratorCloseTest) {
	t.Helper()
	ctx, cancel := setupClientTestContext(t, 10*time.Second)
	defer cancel()

	connectClientSafely(ctx, t, client)

	// Send query if needed for the test scenario
	if test.needQuery {
		err := client.Query(ctx, fmt.Sprintf("%s query", test.name))
		assertNoError(t, err)
	}

	iter := client.ReceiveResponse(ctx)
	if iter == nil {
		t.Fatal("Expected non-nil iterator from ReceiveResponse")
	}

	// Consume first message if requested
	if test.consumeFirst {
		msg, err := iter.Next(ctx)
		if err != nil {
			t.Fatalf("Expected first message, got error: %v", err)
		}
		if msg == nil {
			t.Fatal("Expected first message, got nil")
		}
	}

	// Perform close operation(s)
	closeCount := 1
	if test.multipleCalls {
		closeCount = 3
	}

	for i := 1; i <= closeCount; i++ {
		err := iter.Close()
		if err != nil {
			t.Errorf("Expected Close() call %d to succeed, got: %v", i, err)
		}
	}

	// Verify Next() behavior after close
	nextCalls := 1
	if test.multipleCalls {
		nextCalls = 3
	}

	for i := 0; i < nextCalls; i++ {
		msg, err := iter.Next(ctx)
		if err != ErrNoMoreMessages {
			t.Errorf("Expected ErrNoMoreMessages on Next() call %d after close, got: %v", i+1, err)
		}
		if msg != nil {
			t.Errorf("Expected nil message on Next() call %d after close, got message", i+1)
		}
	}
}

// TestClientContextManager tests automatic resource lifecycle management
// via the Go-idiomatic context manager pattern.
func TestClientContextManager(t *testing.T) {
	tests := []struct {
		name           string
		setupTransport func() *clientMockTransport
		operation      func(Client) error
		wantErr        bool
		validate       func(*testing.T, *clientMockTransport)
	}{
		{
			name:           "automatic_resource_management",
			setupTransport: newClientMockTransport,
			operation: func(c Client) error {
				return c.Query(context.Background(), "test")
			},
			wantErr: false,
			validate: func(t *testing.T, tr *clientMockTransport) {
				assertClientDisconnected(t, tr)
			},
		},
		{
			name: "error_handling_with_cleanup",
			setupTransport: func() *clientMockTransport {
				return newClientMockTransportWithOptions(WithClientSendError(fmt.Errorf("send failed")))
			},
			operation: func(c Client) error {
				return c.Query(context.Background(), "test")
			},
			wantErr: true,
			validate: func(t *testing.T, tr *clientMockTransport) {
				assertClientDisconnected(t, tr)
			},
		},
		{
			name:           "context_cancellation_with_cleanup",
			setupTransport: newClientMockTransport,
			operation: func(c Client) error {
				ctx, cancel := context.WithCancel(context.Background())
				cancel() // Cancel immediately
				return c.Query(ctx, "test")
			},
			wantErr: true,
			validate: func(t *testing.T, tr *clientMockTransport) {
				assertClientDisconnected(t, tr)
			},
		},
		{
			name: "connection_error_no_cleanup_needed",
			setupTransport: func() *clientMockTransport {
				return newClientMockTransportWithOptions(WithClientConnectError(fmt.Errorf("connect failed")))
			},
			operation: func(c Client) error {
				return c.Query(context.Background(), "test")
			},
			wantErr: true,
			validate: func(t *testing.T, tr *clientMockTransport) {
				// Should not be connected if connect failed
				if tr.connected {
					t.Error("Expected transport to not be connected after connect failure")
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transport := test.setupTransport()

			err := WithClientTransport(context.Background(), transport, test.operation)

			assertClientError(t, err, test.wantErr, "")
			test.validate(t, transport)
		})
	}
}

// TestWithClientConcurrentUsage tests concurrent access patterns with context manager
func TestWithClientConcurrentUsage(t *testing.T) {
	ctx, cancel := setupClientTestContext(t, 15*time.Second)
	defer cancel()

	const numGoroutines = 5
	const operationsPerGoroutine = 3

	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines*operationsPerGoroutine)

	// Track all operations and their transports
	var allTransports []*clientMockTransport
	var transportsMu sync.Mutex

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < operationsPerGoroutine; j++ {
				// Create a new transport for each operation to avoid race conditions
				transport := newClientMockTransport()
				transportsMu.Lock()
				allTransports = append(allTransports, transport)
				transportsMu.Unlock()

				err := WithClientTransport(ctx, transport, func(client Client) error {
					return client.Query(ctx, fmt.Sprintf("concurrent query %d-%d", id, j))
				})
				if err != nil {
					errors <- fmt.Errorf("goroutine %d operation %d: %w", id, j, err)
				}
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Check for any errors
	for err := range errors {
		t.Errorf("Concurrent context manager operation error: %v", err)
	}

	// Verify all operations completed successfully
	expectedOperations := numGoroutines * operationsPerGoroutine
	if len(allTransports) != expectedOperations {
		t.Errorf("Expected %d transport instances, got %d", expectedOperations, len(allTransports))
	}

	// Verify each transport sent exactly one message and was properly cleaned up
	totalMessages := 0
	for i, transport := range allTransports {
		messageCount := transport.getSentMessageCount()
		if messageCount != 1 {
			t.Errorf("Transport %d: expected 1 message, got %d", i, messageCount)
		}
		totalMessages += messageCount

		// Verify cleanup occurred
		assertClientDisconnected(t, transport)
	}

	// Verify total message count
	if totalMessages != expectedOperations {
		t.Errorf("Expected %d total messages, got %d", expectedOperations, totalMessages)
	}
}

// TestWithClientContextCancellation tests context cancellation behavior
func TestWithClientContextCancellation(t *testing.T) {
	tests := []struct {
		name         string
		setupContext func() (context.Context, context.CancelFunc)
		wantErr      bool
		errorMsg     string
	}{
		{
			name: "already_canceled_context",
			setupContext: func() (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithCancel(context.Background())
				cancel() // Cancel immediately
				return ctx, cancel
			},
			wantErr:  true,
			errorMsg: "context canceled",
		},
		{
			name: "timeout_context",
			setupContext: func() (context.Context, context.CancelFunc) {
				// Create a context that has already timed out deterministically
				ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Hour))
				return ctx, cancel
			},
			wantErr:  true,
			errorMsg: "context deadline exceeded",
		},
		{
			name: "valid_context",
			setupContext: func() (context.Context, context.CancelFunc) {
				return context.WithTimeout(context.Background(), 5*time.Second)
			},
			wantErr: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := test.setupContext()
			defer cancel()

			transport := newClientMockTransport()

			err := WithClientTransport(ctx, transport, func(client Client) error {
				return client.Query(ctx, "context test")
			})

			assertClientError(t, err, test.wantErr, test.errorMsg)

			// Cleanup should always occur, even with context cancellation
			assertClientDisconnected(t, transport)
		})
	}
}

// TestWithClientOptionsPropagate tests that options are properly passed through
func TestWithClientOptionsPropagate(t *testing.T) {
	ctx, cancel := setupClientTestContext(t, 5*time.Second)
	defer cancel()

	transport := newClientMockTransport()

	// Test with various options
	err := WithClientTransport(ctx, transport, func(client Client) error {
		// Verify client was created and connected
		return client.QueryWithSession(ctx, "options test", "custom-session")
	},
		WithSystemPrompt("Test system prompt"),
		WithAllowedTools("Read", "Write"),
	)

	assertNoError(t, err)
	assertClientMessageCount(t, transport, 1)

	// Verify message was sent with correct session
	sentMsg, ok := transport.getSentMessage(0)
	if !ok {
		t.Fatal("Expected sent message")
	}
	if sentMsg.SessionID != "custom-session" {
		t.Errorf("Expected session ID 'custom-session', got %q", sentMsg.SessionID)
	}

	// Verify cleanup
	assertClientDisconnected(t, transport)
}

// TestClientPythonSDKCompatibility tests Client with Python SDK compatible message format and streaming
func TestClientPythonSDKCompatibility(t *testing.T) {
	ctx, cancel := setupClientTestContext(t, 10*time.Second)
	defer cancel()

	// Create mock messages similar to what Python SDK would receive
	costValue := 0.001234
	testMessages := []Message{
		&AssistantMessage{
			Content: []ContentBlock{
				&TextBlock{
					Text: "Hello! I understand you want to test the streaming functionality.",
				},
			},
		},
		&ResultMessage{
			TotalCostUSD: &costValue,
		},
	}

	transport := newClientMockTransportWithOptions(
		WithClientResponseMessages(testMessages),
	)
	client := setupClientForTest(t, transport)
	defer disconnectClientSafely(t, client)

	// Test complete workflow: Connect → Query → ReceiveMessages
	connectClientSafely(ctx, t, client)
	assertClientConnected(t, transport)

	// Send query using new Python SDK compatible format
	err := client.QueryWithSession(ctx, "Test streaming with Python SDK format", "test-session")
	assertNoError(t, err)

	// Verify message was sent in correct Python SDK format
	assertClientMessageCount(t, transport, 1)
	sentMsg, ok := transport.getSentMessage(0)
	if !ok {
		t.Fatal("Failed to get sent message")
	}

	// Verify Python SDK compatible message structure
	if sentMsg.Type != userMessageType {
		t.Errorf("Expected message type 'user', got '%s'", sentMsg.Type)
	}
	if sentMsg.SessionID != "test-session" {
		t.Errorf("Expected session ID 'test-session', got '%s'", sentMsg.SessionID)
	}
	if sentMsg.ParentToolUseID != nil {
		t.Errorf("Expected nil ParentToolUseID, got '%v'", sentMsg.ParentToolUseID)
	}

	// Verify nested message structure matches Python format
	messageMap, ok := sentMsg.Message.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected Message to be map[string]interface{}, got %T", sentMsg.Message)
	}
	if role, ok := messageMap["role"]; !ok || role != userMessageType {
		t.Errorf("Expected message role 'user', got '%v'", role)
	}
	if content, ok := messageMap["content"]; !ok || content != "Test streaming with Python SDK format" {
		t.Errorf("Expected content to match prompt, got '%v'", content)
	}

	// Test message receiving functionality
	msgChan := client.ReceiveMessages(ctx)
	if msgChan == nil {
		t.Fatal("ReceiveMessages returned nil channel")
	}

	// Receive first message (AssistantMessage)
	select {
	case msg := <-msgChan:
		if msg == nil {
			t.Fatal("Received nil message")
		}
		assistantMsg, ok := msg.(*AssistantMessage)
		if !ok {
			t.Fatalf("Expected AssistantMessage, got %T", msg)
		}
		if len(assistantMsg.Content) != 1 {
			t.Fatalf("Expected 1 content block, got %d", len(assistantMsg.Content))
		}
		textBlock, ok := assistantMsg.Content[0].(*TextBlock)
		if !ok {
			t.Fatalf("Expected TextBlock, got %T", assistantMsg.Content[0])
		}
		if !strings.Contains(textBlock.Text, "streaming functionality") {
			t.Errorf("Expected text to mention streaming functionality, got: %s", textBlock.Text)
		}
	case <-time.After(time.Second):
		t.Fatal("Timeout waiting for first message")
	}

	// Receive second message (ResultMessage)
	select {
	case msg := <-msgChan:
		if msg == nil {
			t.Fatal("Received nil message")
		}
		resultMsg, ok := msg.(*ResultMessage)
		if !ok {
			t.Fatalf("Expected ResultMessage, got %T", msg)
		}
		if resultMsg.TotalCostUSD == nil || *resultMsg.TotalCostUSD != 0.001234 {
			var cost float64
			if resultMsg.TotalCostUSD != nil {
				cost = *resultMsg.TotalCostUSD
			}
			t.Errorf("Expected cost 0.001234, got %f", cost)
		}
	case <-time.After(time.Second):
		t.Fatal("Timeout waiting for second message")
	}

	// Test iterator pattern with ReceiveResponse (basic functionality)
	iter := client.ReceiveResponse(ctx)
	if iter == nil {
		t.Fatal("ReceiveResponse returned nil iterator")
	}

	// Test that iterator can be closed immediately.
	err = iter.Close()
	assertNoError(t, err)
}

// TestWithClient tests the WithClient convenience function with automatic CLI discovery
// This tests the actual WithClient function (not WithClientTransport) which has 0% coverage
func TestWithClient(t *testing.T) {
	tests := []struct {
		name    string
		ctx     func(t *testing.T) (context.Context, context.CancelFunc)
		fn      func(Client) error
		opts    []Option
		wantErr bool
		errMsg  string
	}{
		{
			name: "canceled_context",
			ctx: func(t *testing.T) (context.Context, context.CancelFunc) {
				t.Helper()
				ctx, cancel := context.WithCancel(context.Background())
				cancel() // Cancel immediately
				return ctx, cancel
			},
			fn: func(_ Client) error {
				return nil // Should not be called
			},
			wantErr: true,
			errMsg:  "context canceled",
		},
		{
			name: "function_returns_error_on_successful_connection",
			ctx: func(t *testing.T) (context.Context, context.CancelFunc) {
				t.Helper()
				return setupClientTestContext(t, 5*time.Second)
			},
			fn: func(_ Client) error {
				// If we get here, connection succeeded
				return fmt.Errorf("test function error")
			},
			opts:    []Option{WithCLIPath("nonexistent")}, // Force failure
			wantErr: true,
			errMsg:  "", // Will either be connection error or function error
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := test.ctx(t)
			defer cancel()

			// This will attempt to auto-discover CLI, which will fail in test environment
			// but that's the expected behavior we want to test
			err := WithClient(ctx, test.fn, test.opts...)

			if test.wantErr {
				if err == nil {
					t.Errorf("Expected error, got nil")
				} else if test.errMsg != "" && !strings.Contains(err.Error(), test.errMsg) {
					t.Errorf("Expected error to contain %q, got %v", test.errMsg, err)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

// TestClientIteratorNextErrorPaths tests error scenarios in clientIterator.Next() method
// Targets the missing 45.5% coverage in Next function error paths
func TestClientIteratorNextErrorPaths(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(t *testing.T) (*clientIterator, context.Context, context.CancelFunc)
		validate func(t *testing.T, msg Message, err error)
	}{
		{
			name: "next_on_closed_iterator",
			setup: func(t *testing.T) (*clientIterator, context.Context, context.CancelFunc) {
				t.Helper()
				msgChan := make(chan Message)
				errChan := make(chan error)
				iter := &clientIterator{
					stream: newStreamReader(msgChan, errChan),
					closed: true, // Already closed
				}
				ctx, cancel := setupClientTestContext(t, 5*time.Second)
				return iter, ctx, cancel
			},
			validate: func(t *testing.T, msg Message, err error) {
				t.Helper()
				if err != ErrNoMoreMessages {
					t.Errorf("Expected ErrNoMoreMessages on closed iterator, got: %v", err)
				}
				if msg != nil {
					t.Errorf("Expected nil message on closed iterator, got: %v", msg)
				}
			},
		},
		{
			name: "context_canceled_while_waiting",
			setup: func(t *testing.T) (*clientIterator, context.Context, context.CancelFunc) {
				t.Helper()
				msgChan := make(chan Message)
				errChan := make(chan error)
				iter := &clientIterator{
					stream: newStreamReader(msgChan, errChan),
					closed: false,
				}
				ctx, cancel := setupClientTestContext(t, 50*time.Millisecond)
				return iter, ctx, cancel
			},
			validate: func(t *testing.T, msg Message, err error) {
				t.Helper()
				if err != context.DeadlineExceeded {
					t.Errorf("Expected context.DeadlineExceeded, got: %v", err)
				}
				if msg != nil {
					t.Errorf("Expected nil message on context cancellation, got: %v", msg)
				}
			},
		},
		{
			name: "error_received_on_error_channel",
			setup: func(t *testing.T) (*clientIterator, context.Context, context.CancelFunc) {
				t.Helper()
				msgChan := make(chan Message)
				errChan := make(chan error, 1)
				iter := &clientIterator{
					stream: newStreamReader(msgChan, errChan),
					closed: false,
				}

				// Send error to error channel
				expectedErr := fmt.Errorf("transport error")
				errChan <- expectedErr

				ctx, cancel := setupClientTestContext(t, 5*time.Second)
				return iter, ctx, cancel
			},
			validate: func(t *testing.T, msg Message, err error) {
				t.Helper()
				if err == nil {
					t.Fatal("Expected error from error channel, got nil")
				}
				if err.Error() != "transport error" {
					t.Errorf("Expected 'transport error', got: %v", err)
				}
				if msg != nil {
					t.Errorf("Expected nil message on error, got: %v", msg)
				}
			},
		},
		{
			name: "message_channel_closed",
			setup: func(t *testing.T) (*clientIterator, context.Context, context.CancelFunc) {
				t.Helper()
				msgChan := make(chan Message)
				errChan := make(chan error)
				iter := &clientIterator{
					stream: newStreamReader(msgChan, errChan),
					closed: false,
				}

				// Close the message channel
				close(msgChan)

				ctx, cancel := setupClientTestContext(t, 5*time.Second)
				return iter, ctx, cancel
			},
			validate: func(t *testing.T, msg Message, err error) {
				t.Helper()
				if err != ErrNoMoreMessages {
					t.Errorf("Expected ErrNoMoreMessages on closed channel, got: %v", err)
				}
				if msg != nil {
					t.Errorf("Expected nil message on closed channel, got: %v", msg)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			iter, ctx, cancel := test.setup(t)
			defer cancel()

			msg, err := iter.Next(ctx)
			test.validate(t, msg, err)

			// Verify iterator is closed after error conditions
			if test.name != "next_on_closed_iterator" && !iter.closed {
				t.Error("Expected iterator to be closed after error condition")
			}
		})
	}
}

// Tests for the Query API without variadic parameters.

func TestClientQueryDefaultSession(t *testing.T) {
	ctx, cancel := setupClientTestContext(t, 5*time.Second)
	defer cancel()

	transport := newClientMockTransport()
	client := setupClientForTest(t, transport)
	defer disconnectClientSafely(t, client)

	connectClientSafely(ctx, t, client)

	// Test that Query() with no session uses "default"
	err := client.Query(ctx, "test message")
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	// Verify the sent message used default session
	sentMsg, ok := transport.getSentMessage(0)
	if !ok {
		t.Fatal("Expected a message to be sent")
	}

	if sentMsg.SessionID != defaultSessionID {
		t.Errorf("Expected session ID 'default', got %q", sentMsg.SessionID)
	}

	if sentMsg.Type != userMessageType {
		t.Errorf("Expected message type 'user', got %q", sentMsg.Type)
	}

	message, ok := sentMsg.Message.(map[string]interface{})
	if !ok {
		t.Fatal("Expected message to be a map")
	}

	if content, ok := message["content"].(string); !ok || content != "test message" {
		t.Errorf("Expected message content 'test message', got %v", message["content"])
	}
}

func TestClientQueryWithCustomSession(t *testing.T) {
	ctx, cancel := setupClientTestContext(t, 5*time.Second)
	defer cancel()

	transport := newClientMockTransport()
	client := setupClientForTest(t, transport)
	defer disconnectClientSafely(t, client)

	connectClientSafely(ctx, t, client)

	customSession := "my-custom-session-123"

	// Test that QueryWithSession() uses the provided session
	err := client.QueryWithSession(ctx, "test message", customSession)
	if err != nil {
		t.Fatalf("QueryWithSession failed: %v", err)
	}

	// Verify the sent message used custom session
	sentMsg, ok := transport.getSentMessage(0)
	if !ok {
		t.Fatal("Expected a message to be sent")
	}

	if sentMsg.SessionID != customSession {
		t.Errorf("Expected session ID %q, got %q", customSession, sentMsg.SessionID)
	}

	if sentMsg.Type != userMessageType {
		t.Errorf("Expected message type 'user', got %q", sentMsg.Type)
	}

	message, ok := sentMsg.Message.(map[string]interface{})
	if !ok {
		t.Fatal("Expected message to be a map")
	}

	if content, ok := message["content"].(string); !ok || content != "test message" {
		t.Errorf("Expected message content 'test message', got %v", message["content"])
	}
}

func TestClientQueryWithSessionValidation(t *testing.T) {
	ctx, cancel := setupClientTestContext(t, 5*time.Second)
	defer cancel()

	transport := newClientMockTransport()
	client := setupClientForTest(t, transport)
	defer disconnectClientSafely(t, client)

	connectClientSafely(ctx, t, client)

	// Test with empty session ID - should use default
	err := client.QueryWithSession(ctx, "test message", "")
	if err != nil {
		t.Fatalf("QueryWithSession with empty session failed: %v", err)
	}

	// Verify the sent message used default session when empty provided
	sentMsg, ok := transport.getSentMessage(0)
	if !ok {
		t.Fatal("Expected a message to be sent")
	}

	if sentMsg.SessionID != defaultSessionID {
		t.Errorf("Expected session ID 'default' when empty provided, got %q", sentMsg.SessionID)
	}
}

func TestClientQuerySessionBehaviorParity(t *testing.T) {
	// This test ensures our Go implementation matches Python SDK behavior
	tests := []struct {
		name           string
		useDefault     bool
		sessionID      string
		expectedResult string
	}{
		{
			name:           "default_session_behavior",
			useDefault:     true,
			expectedResult: "default",
		},
		{
			name:           "custom_session_behavior",
			useDefault:     false,
			sessionID:      "python-parity-test",
			expectedResult: "python-parity-test",
		},
		{
			name:           "empty_session_falls_back_to_default",
			useDefault:     false,
			sessionID:      "",
			expectedResult: "default",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := setupClientTestContext(t, 5*time.Second)
			defer cancel()

			transport := newClientMockTransport()
			client := setupClientForTest(t, transport)
			defer disconnectClientSafely(t, client)

			connectClientSafely(ctx, t, client)

			var err error
			if test.useDefault {
				err = client.Query(ctx, "parity test")
			} else {
				err = client.QueryWithSession(ctx, "parity test", test.sessionID)
			}

			if err != nil {
				t.Fatalf("Query failed: %v", err)
			}

			// Verify session behavior matches expected
			sentMsg, ok := transport.getSentMessage(0)
			if !ok {
				t.Fatal("Expected a message to be sent")
			}

			if sentMsg.SessionID != test.expectedResult {
				t.Errorf("Expected session ID %q, got %q", test.expectedResult, sentMsg.SessionID)
			}
		})
	}
}

func TestClientQueryNotConnectedError(t *testing.T) {
	ctx, cancel := setupClientTestContext(t, 5*time.Second)
	defer cancel()

	transport := newClientMockTransport()
	client := setupClientForTest(t, transport)
	defer disconnectClientSafely(t, client)

	// Note: We don't call connectClientSafely() here - client should be disconnected

	// Test Query() when not connected
	err := client.Query(ctx, "test message")
	if err == nil {
		t.Fatal("Expected error when not connected")
	}
	if !strings.Contains(err.Error(), "not connected") {
		t.Errorf("Expected 'not connected' error, got: %v", err)
	}

	// Test QueryWithSession() when not connected
	err = client.QueryWithSession(ctx, "test message", "custom")
	if err == nil {
		t.Fatal("Expected error when not connected")
	}
	if !strings.Contains(err.Error(), "not connected") {
		t.Errorf("Expected 'not connected' error, got: %v", err)
	}
}

// TestGetServerInfo verifies that GetServerInfo returns the initialize
// response the CLI sent (Python get_server_info returns
// Query._initialization_result), and an error when not connected.
func TestGetServerInfo(t *testing.T) {
	tests := []struct {
		name       string
		transport  func() Transport
		connect    bool
		disconnect bool
		query      bool
		wantErr    bool
		want       map[string]interface{}
	}{
		{name: "not_connected", transport: newServerInfoTransport, wantErr: true},
		{name: "initialize_response", transport: newServerInfoTransport, connect: true, want: testInitializeResponse()},
		{name: "after_query", transport: newServerInfoTransport, connect: true, query: true, want: testInitializeResponse()},
		{name: "after_disconnect", transport: newServerInfoTransport, connect: true, disconnect: true, wantErr: true},
		// A transport that does not keep the response has no server info (Python: None).
		{name: "custom_transport", transport: func() Transport { return newClientMockTransport() }, connect: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := setupClientTestContext(t, 5*time.Second)
			defer cancel()

			client := setupClientForTest(t, test.transport())
			defer disconnectClientSafely(t, client)

			if test.connect {
				connectClientSafely(ctx, t, client)
			}
			if test.query {
				assertNoError(t, client.Query(ctx, "test message"))
			}
			if test.disconnect {
				disconnectClientSafely(t, client)
			}

			info, err := client.GetServerInfo(ctx)
			assertClientError(t, err, test.wantErr, "not connected")
			if !reflect.DeepEqual(info, test.want) {
				t.Errorf("GetServerInfo() = %v, want %v", info, test.want)
			}
		})
	}
}

// TestGetServerInfoConcurrent tests thread-safety of GetServerInfo
func TestGetServerInfoConcurrent(t *testing.T) {
	ctx, cancel := setupClientTestContext(t, 15*time.Second)
	defer cancel()

	client := setupClientForTest(t, newServerInfoTransport())
	defer disconnectClientSafely(t, client)

	connectClientSafely(ctx, t, client)

	const numGoroutines = 10
	errs := make(chan error, numGoroutines)
	results := make(chan map[string]interface{}, numGoroutines)

	var wg sync.WaitGroup
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			info, err := client.GetServerInfo(ctx)
			if err != nil {
				errs <- err
				return
			}
			results <- info
		}()
	}

	wg.Wait()
	close(errs)
	close(results)

	for err := range errs {
		t.Errorf("Concurrent GetServerInfo error: %v", err)
	}
	for info := range results {
		if !reflect.DeepEqual(info, testInitializeResponse()) {
			t.Errorf("GetServerInfo() = %v, want the initialize response", info)
		}
	}
}

// TestSubprocessTransportKeepsServerInfo guards the optional interface that
// GetServerInfo type-asserts: if the subprocess transport stopped satisfying
// it, GetServerInfo would silently return nil.
func TestSubprocessTransportKeepsServerInfo(t *testing.T) {
	var transport Transport = subprocess.New("claude", NewOptions(), "sdk-go-client")
	if _, ok := transport.(serverInfoSource); !ok {
		t.Fatal("subprocess.Transport does not implement serverInfoSource")
	}
}

func TestClientDynamicControl(t *testing.T) {
	t.Run("set_model", testClientSetModel)
	t.Run("set_permission_mode", testClientSetPermissionMode)
}

func testClientSetModel(t *testing.T) {
	t.Run("success", testClientSetModelSuccess)
	t.Run("not_connected", testClientSetModelNotConnected)
	t.Run("context_cancelled", testClientSetModelContextCancelled)
	t.Run("transport_error", testClientSetModelTransportError)
}

func testClientSetModelSuccess(t *testing.T) {
	t.Helper()

	ctx, cancel := setupClientTestContext(t, 5*time.Second)
	defer cancel()

	transport := newClientMockTransport()
	client := setupClientForTest(t, transport)
	defer disconnectClientSafely(t, client)

	connectClientSafely(ctx, t, client)

	model := testModelSonnet
	err := client.SetModel(ctx, &model)
	assertNoError(t, err)
}

func testClientSetModelNotConnected(t *testing.T) {
	t.Helper()

	ctx, cancel := setupClientTestContext(t, 5*time.Second)
	defer cancel()

	transport := newClientMockTransport()
	client := setupClientForTest(t, transport)
	// Note: NOT connecting the client

	model := testModelSonnet
	err := client.SetModel(ctx, &model)

	if err == nil {
		t.Fatal("expected error when not connected, got nil")
	}
	if !strings.Contains(err.Error(), "not connected") {
		t.Errorf("expected 'not connected' error, got: %v", err)
	}
}

func testClientSetModelContextCancelled(t *testing.T) {
	t.Helper()

	ctx, cancel := setupClientTestContext(t, 5*time.Second)
	transport := newClientMockTransport()
	client := setupClientForTest(t, transport)
	defer disconnectClientSafely(t, client)

	connectClientSafely(ctx, t, client)

	// Cancel context before calling SetModel
	cancel()

	model := testModelSonnet
	err := client.SetModel(ctx, &model)

	if err == nil {
		t.Fatal("expected error when context cancelled, got nil")
	}
}

func testClientSetModelTransportError(t *testing.T) {
	t.Helper()

	ctx, cancel := setupClientTestContext(t, 5*time.Second)
	defer cancel()

	expectedErr := errors.New("transport set model error")
	transport := newClientMockTransportWithOptions(
		WithClientSetModelError(expectedErr),
	)
	client := setupClientForTest(t, transport)
	defer disconnectClientSafely(t, client)

	connectClientSafely(ctx, t, client)

	model := testModelSonnet
	err := client.SetModel(ctx, &model)

	if err == nil {
		t.Fatal("expected error from transport, got nil")
	}
	if !strings.Contains(err.Error(), "transport set model error") {
		t.Errorf("expected transport error, got: %v", err)
	}
}

func testClientSetPermissionMode(t *testing.T) {
	t.Run("success", testClientSetPermissionModeSuccess)
	t.Run("not_connected", testClientSetPermissionModeNotConnected)
	t.Run("context_cancelled", testClientSetPermissionModeContextCancelled)
	t.Run("transport_error", testClientSetPermissionModeTransportError)
}

func testClientSetPermissionModeSuccess(t *testing.T) {
	t.Helper()

	ctx, cancel := setupClientTestContext(t, 5*time.Second)
	defer cancel()

	transport := newClientMockTransport()
	client := setupClientForTest(t, transport)
	defer disconnectClientSafely(t, client)

	connectClientSafely(ctx, t, client)

	err := client.SetPermissionMode(ctx, PermissionModeAcceptEdits)
	assertNoError(t, err)
}

func testClientSetPermissionModeNotConnected(t *testing.T) {
	t.Helper()

	ctx, cancel := setupClientTestContext(t, 5*time.Second)
	defer cancel()

	transport := newClientMockTransport()
	client := setupClientForTest(t, transport)
	// Note: NOT connecting the client

	err := client.SetPermissionMode(ctx, PermissionModeAcceptEdits)

	if err == nil {
		t.Fatal("expected error when not connected, got nil")
	}
	if !strings.Contains(err.Error(), "not connected") {
		t.Errorf("expected 'not connected' error, got: %v", err)
	}
}

func testClientSetPermissionModeContextCancelled(t *testing.T) {
	t.Helper()

	ctx, cancel := setupClientTestContext(t, 5*time.Second)
	transport := newClientMockTransport()
	client := setupClientForTest(t, transport)
	defer disconnectClientSafely(t, client)

	connectClientSafely(ctx, t, client)

	// Cancel context before calling SetPermissionMode
	cancel()

	err := client.SetPermissionMode(ctx, PermissionModeAcceptEdits)

	if err == nil {
		t.Fatal("expected error when context cancelled, got nil")
	}
}

func testClientSetPermissionModeTransportError(t *testing.T) {
	t.Helper()

	ctx, cancel := setupClientTestContext(t, 5*time.Second)
	defer cancel()

	expectedErr := errors.New("transport set permission mode error")
	transport := newClientMockTransportWithOptions(
		WithClientSetPermissionModeError(expectedErr),
	)
	client := setupClientForTest(t, transport)
	defer disconnectClientSafely(t, client)

	connectClientSafely(ctx, t, client)

	err := client.SetPermissionMode(ctx, PermissionModeAcceptEdits)

	if err == nil {
		t.Fatal("expected error from transport, got nil")
	}
	if !strings.Contains(err.Error(), "transport set permission mode error") {
		t.Errorf("expected transport error, got: %v", err)
	}
}

func TestClientRewindFiles(t *testing.T) {
	t.Run("success", testClientRewindFilesSuccess)
	t.Run("not_connected", testClientRewindFilesNotConnected)
	t.Run("context_cancelled", testClientRewindFilesContextCancelled)
	t.Run("transport_error", testClientRewindFilesTransportError)
}

func testClientRewindFilesSuccess(t *testing.T) {
	t.Helper()

	ctx, cancel := setupClientTestContext(t, 5*time.Second)
	defer cancel()

	transport := newClientMockTransport()
	client := setupClientForTest(t, transport)
	defer disconnectClientSafely(t, client)

	connectClientSafely(ctx, t, client)

	err := client.RewindFiles(ctx, "msg-uuid-12345")
	assertNoError(t, err)
}

func testClientRewindFilesNotConnected(t *testing.T) {
	t.Helper()

	ctx, cancel := setupClientTestContext(t, 5*time.Second)
	defer cancel()

	transport := newClientMockTransport()
	client := setupClientForTest(t, transport)
	// Note: NOT connecting the client

	err := client.RewindFiles(ctx, "msg-uuid-12345")

	if err == nil {
		t.Fatal("expected error when not connected, got nil")
	}
	if !strings.Contains(err.Error(), "not connected") {
		t.Errorf("expected 'not connected' error, got: %v", err)
	}
}

func testClientRewindFilesContextCancelled(t *testing.T) {
	t.Helper()

	ctx, cancel := setupClientTestContext(t, 5*time.Second)
	transport := newClientMockTransport()
	client := setupClientForTest(t, transport)
	defer disconnectClientSafely(t, client)

	connectClientSafely(ctx, t, client)

	// Cancel context before calling RewindFiles
	cancel()

	err := client.RewindFiles(ctx, "msg-uuid-12345")

	if err == nil {
		t.Fatal("expected error when context cancelled, got nil")
	}
}

func testClientRewindFilesTransportError(t *testing.T) {
	t.Helper()

	ctx, cancel := setupClientTestContext(t, 5*time.Second)
	defer cancel()

	expectedErr := errors.New("transport rewind files error")
	transport := newClientMockTransportWithOptions(
		WithClientRewindFilesError(expectedErr),
	)
	client := setupClientForTest(t, transport)
	defer disconnectClientSafely(t, client)

	connectClientSafely(ctx, t, client)

	err := client.RewindFiles(ctx, "msg-uuid-12345")

	if err == nil {
		t.Fatal("expected error from transport, got nil")
	}
	if !strings.Contains(err.Error(), "transport rewind files error") {
		t.Errorf("expected transport error, got: %v", err)
	}
}

// TestClientGetMcpStatus tests GetMcpStatus delegation through the client layer.
func TestClientGetMcpStatus(t *testing.T) {
	t.Run("success", testClientGetMcpStatusSuccess)
	t.Run("not_connected", testClientGetMcpStatusNotConnected)
	t.Run("context_cancelled", testClientGetMcpStatusContextCancelled)
	t.Run("transport_error", testClientGetMcpStatusTransportError)
}

func testClientGetMcpStatusSuccess(t *testing.T) {
	t.Helper()

	ctx, cancel := setupClientTestContext(t, 5*time.Second)
	defer cancel()

	serverName := "test-server"
	scope := "local"
	resp := &McpStatusResponse{
		McpServers: []McpServerStatus{
			{
				Name:   serverName,
				Status: McpServerConnectionStatusConnected,
				Scope:  &scope,
			},
		},
	}
	transport := newClientMockTransportWithOptions(
		WithClientGetMcpStatusResponse(resp),
	)
	client := setupClientForTest(t, transport)
	defer disconnectClientSafely(t, client)

	connectClientSafely(ctx, t, client)

	got, err := client.GetMcpStatus(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil response")
	}
	if len(got.McpServers) != 1 {
		t.Fatalf("expected 1 server, got %d", len(got.McpServers))
	}
	if got.McpServers[0].Name != serverName {
		t.Errorf("expected server name %q, got %q", serverName, got.McpServers[0].Name)
	}
	if got.McpServers[0].Status != McpServerConnectionStatusConnected {
		t.Errorf("expected status connected, got %q", got.McpServers[0].Status)
	}
}

func testClientGetMcpStatusNotConnected(t *testing.T) {
	t.Helper()

	ctx, cancel := setupClientTestContext(t, 5*time.Second)
	defer cancel()

	transport := newClientMockTransport()
	client := setupClientForTest(t, transport)
	defer disconnectClientSafely(t, client)

	_, err := client.GetMcpStatus(ctx)

	if err == nil {
		t.Fatal("expected error when not connected, got nil")
	}
	if !strings.Contains(err.Error(), "not connected") {
		t.Errorf("expected 'not connected' error, got: %v", err)
	}
}

func testClientGetMcpStatusContextCancelled(t *testing.T) {
	t.Helper()

	ctx, cancel := setupClientTestContext(t, 5*time.Second)
	transport := newClientMockTransport()
	client := setupClientForTest(t, transport)
	defer disconnectClientSafely(t, client)

	connectClientSafely(ctx, t, client)

	cancel()

	_, err := client.GetMcpStatus(ctx)

	if err == nil {
		t.Fatal("expected error when context cancelled, got nil")
	}
}

func testClientGetMcpStatusTransportError(t *testing.T) {
	t.Helper()

	ctx, cancel := setupClientTestContext(t, 5*time.Second)
	defer cancel()

	expectedErr := errors.New("transport mcp status error")
	transport := newClientMockTransportWithOptions(
		WithClientGetMcpStatusError(expectedErr),
	)
	client := setupClientForTest(t, transport)
	defer disconnectClientSafely(t, client)

	connectClientSafely(ctx, t, client)

	_, err := client.GetMcpStatus(ctx)

	if err == nil {
		t.Fatal("expected error from transport, got nil")
	}
	if !strings.Contains(err.Error(), "transport mcp status error") {
		t.Errorf("expected transport error, got: %v", err)
	}
}

// TestClientStopTask tests StopTask delegation through the client layer.
func TestClientStopTask(t *testing.T) {
	tests := []struct {
		name        string
		options     []ClientMockTransportOption
		connect     bool
		cancelFirst bool
		wantErr     string
		wantStopped []string
	}{
		{
			name:        "success",
			connect:     true,
			wantStopped: []string{"task-abc123"},
		},
		{
			name:    "not_connected",
			wantErr: "not connected",
		},
		{
			name:        "context_cancelled",
			connect:     true,
			cancelFirst: true,
			wantErr:     "context canceled",
		},
		{
			name:    "transport_error",
			options: []ClientMockTransportOption{WithClientStopTaskError(errors.New("transport stop task error"))},
			connect: true,
			wantErr: "transport stop task error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := setupClientTestContext(t, 5*time.Second)
			defer cancel()

			transport := newClientMockTransportWithOptions(tt.options...)
			client := setupClientForTest(t, transport)
			defer disconnectClientSafely(t, client)

			if tt.connect {
				connectClientSafely(ctx, t, client)
			}
			if tt.cancelFirst {
				cancel()
			}

			err := client.StopTask(ctx, "task-abc123")

			assertClientErrorContains(t, err, tt.wantErr)
			if got := transport.getStoppedTaskIDs(); fmt.Sprint(got) != fmt.Sprint(tt.wantStopped) {
				t.Errorf("stopped task IDs = %v, want %v", got, tt.wantStopped)
			}
		})
	}
}

func assertClientErrorContains(t *testing.T, err error, want string) {
	t.Helper()
	if want == "" {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return
	}
	if err == nil {
		t.Fatalf("expected error containing %q, got nil", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Errorf("expected error containing %q, got: %v", want, err)
	}
}

// TestClientIteratorStopsAfterResultMessage pins Python receive_response(): yield up to and including the ResultMessage.
func TestClientIteratorStopsAfterResultMessage(t *testing.T) {
	ctx, cancel := setupClientTestContext(t, 5*time.Second)
	defer cancel()

	msgChan := make(chan Message, 3)
	msgChan <- &AssistantMessage{Content: []ContentBlock{&TextBlock{Text: "first"}}}
	msgChan <- &ResultMessage{SessionID: "s1"}
	msgChan <- &AssistantMessage{Content: []ContentBlock{&TextBlock{Text: "next turn"}}}
	iter := &clientIterator{stream: newStreamReader(msgChan, make(chan error))}

	if msg, err := iter.Next(ctx); err != nil {
		t.Fatalf("first Next: %v", err)
	} else if _, ok := msg.(*AssistantMessage); !ok {
		t.Fatalf("first Next = %T, want *AssistantMessage", msg)
	}
	if msg, err := iter.Next(ctx); err != nil {
		t.Fatalf("second Next: %v", err)
	} else if _, ok := msg.(*ResultMessage); !ok {
		t.Fatalf("second Next = %T, want *ResultMessage", msg)
	}
	if msg, err := iter.Next(ctx); !errors.Is(err, ErrNoMoreMessages) {
		t.Fatalf("Next after ResultMessage = (%T, %v), want ErrNoMoreMessages", msg, err)
	}
	if got := len(msgChan); got != 1 {
		t.Errorf("iterator consumed the next turn: %d messages left in channel, want 1", got)
	}
}

// TestClientReceiveResponseMultiTurn verifies each ReceiveResponse ends at its own turn's ResultMessage.
func TestClientReceiveResponseMultiTurn(t *testing.T) {
	ctx, cancel := setupClientTestContext(t, 5*time.Second)
	defer cancel()

	transport := newClientMockTransport()
	client := setupClientForTest(t, transport)
	defer disconnectClientSafely(t, client)
	connectClientSafely(ctx, t, client)

	for turn := 1; turn <= 2; turn++ {
		text := fmt.Sprintf("answer %d", turn)
		assertNoError(t, client.Query(ctx, fmt.Sprintf("question %d", turn)))
		transport.injectTestMessage(&AssistantMessage{Content: []ContentBlock{&TextBlock{Text: text}}})
		transport.injectTestMessage(&ResultMessage{SessionID: "s1"})

		iter := client.ReceiveResponse(ctx)
		var got []Message
		for {
			msg, err := iter.Next(ctx)
			if errors.Is(err, ErrNoMoreMessages) {
				break
			}
			if err != nil {
				t.Fatalf("turn %d: Next: %v", turn, err)
			}
			got = append(got, msg)
		}
		if len(got) != 2 {
			t.Fatalf("turn %d: got %d messages, want 2", turn, len(got))
		}
		assistant, ok := got[0].(*AssistantMessage)
		if !ok {
			t.Fatalf("turn %d: first message = %T, want *AssistantMessage", turn, got[0])
		}
		if tb, ok := assistant.Content[0].(*TextBlock); !ok || tb.Text != text {
			t.Errorf("turn %d: text = %v, want %q", turn, assistant.Content[0], text)
		}
		if _, ok := got[1].(*ResultMessage); !ok {
			t.Errorf("turn %d: last message = %T, want *ResultMessage", turn, got[1])
		}
	}
}

// TestValidateWindowsArgValue pins Python _reject_windows_cmd_metacharacters: Windows only, cmd.exe metacharacters and CR/LF.
func TestValidateWindowsArgValue(t *testing.T) {
	tests := []struct {
		name    string
		goos    string
		value   string
		wantErr bool
	}{
		{"windows_uuid", "windows", "550e8400-e29b-41d4-a716-446655440000", false},
		{"windows_title_with_space", "windows", "my session title", false},
		{"windows_dash_value", "windows", "--version", false},
		{"windows_ampersand", "windows", "abc & calc.exe", true},
		{"windows_pipe", "windows", "a|b", true},
		{"windows_redirect_in", "windows", "a<b", true},
		{"windows_redirect_out", "windows", "a>b", true},
		{"windows_caret", "windows", "a^b", true},
		{"windows_percent", "windows", "%PATH%", true},
		{"windows_bang", "windows", "!x!", true},
		{"windows_quote", "windows", `a"b`, true},
		{"windows_cr", "windows", "a\rb", true},
		{"windows_lf", "windows", "a\nb", true},
		{"linux_metacharacters_allowed", "linux", `a&|<>^%!"b`, false},
		{"darwin_newline_allowed", "darwin", "a\nb", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateWindowsArgValue(tt.goos, "resume", tt.value)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateWindowsArgValue(%q, %q) error = %v, wantErr %v", tt.goos, tt.value, err, tt.wantErr)
			}
			if err != nil && !strings.Contains(err.Error(), "resume") {
				t.Errorf("error %q does not name the option", err)
			}
		})
	}
}

// TestValidateWindowsArgs pins Python _reject_windows_cmd_metacharacters for each argv value option.
func TestValidateWindowsArgs(t *testing.T) {
	bad := "x&calc"
	tests := []struct {
		name    string
		goos    string
		options *Options
		wantErr string
	}{
		{"resume_windows", windowsOS, &Options{Resume: &bad}, "resume"},
		{"resume_session_at_windows", windowsOS, &Options{ResumeSessionAt: &bad}, "resume_session_at"},
		{"resume_drops_turn_windows", windowsOS, &Options{ResumeDropsTurn: &bad}, "resume_drops_turn"},
		{"resume_session_at_linux", "linux", &Options{ResumeSessionAt: &bad}, ""},
		{"unset_windows", windowsOS, &Options{}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateWindowsArgs(tt.goos, tt.options)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validateWindowsArgs() error = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.HasPrefix(err.Error(), tt.wantErr+" value") {
				t.Fatalf("validateWindowsArgs() error = %v, want an error that names %s", err, tt.wantErr)
			}
		})
	}
}

// TestPrepareOptionsResumeMetacharacters covers the call site: only Windows rejects the value.
func TestPrepareOptionsResumeMetacharacters(t *testing.T) {
	resume := "abc & calc.exe"
	err := prepareOptions(&Options{Resume: &resume})
	if wantErr := runtime.GOOS == windowsOS; (err != nil) != wantErr {
		t.Fatalf("prepareOptions(Resume=%q) on %s: error = %v, wantErr %v", resume, runtime.GOOS, err, wantErr)
	}
}
