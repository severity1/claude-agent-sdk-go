package claudecode

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
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

	// Test defer-based resource management (Go equivalent of Python context manager)
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

	if role, ok := messageMap["role"]; !ok || role != "user" {
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

// TestQueryStream_SendErrorSurfacesViaReceive verifies that when the transport
// rejects a send inside QueryStream's goroutine, the error is observable to a
// caller blocking on ReceiveResponse / ReceiveMessages instead of being
// swallowed into the log and silently dropping downstream. Without this path,
// streaming writers that lose the pipe mid-conversation look identical to a
// healthy quiet stream.
func TestQueryStream_SendErrorSurfacesViaReceive(t *testing.T) {
	ctx, cancel := setupClientTestContext(t, 5*time.Second)
	defer cancel()

	sendErr := errors.New("simulated broken pipe")
	transport := newClientMockTransportWithOptions(WithClientSendError(sendErr))
	client := setupClientForTest(t, transport)
	defer disconnectClientSafely(t, client)
	connectClientSafely(ctx, t, client)

	messages := make(chan StreamMessage, 1)
	messages <- StreamMessage{Type: "request", Message: &UserMessage{Content: "hi"}}
	close(messages)

	if err := client.QueryStream(ctx, messages); err != nil {
		t.Fatalf("QueryStream should return nil on launch, got %v", err)
		return
	}

	iter := client.ReceiveResponse(ctx)
	if iter == nil {
		t.Fatal("ReceiveResponse returned nil iterator")
		return
	}
	defer func() { _ = iter.Close() }()

	iterCtx, iterCancel := context.WithTimeout(ctx, 2*time.Second)
	defer iterCancel()
	_, err := iter.Next(iterCtx)
	if err == nil {
		t.Fatal("expected Next to return send error, got nil")
		return
	}
	if !errors.Is(err, sendErr) && !strings.Contains(err.Error(), sendErr.Error()) {
		t.Errorf("expected error to carry %q, got %v", sendErr.Error(), err)
	}
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
	impl, ok := client.(*clientImpl)
	if !ok {
		t.Fatal("Expected client to be *clientImpl")
	}

	// Verify PermissionPromptToolName was auto-configured to "stdio"
	if impl.options.PermissionPromptToolName == nil {
		t.Error("Expected PermissionPromptToolName to be auto-configured, got nil")
	} else if *impl.options.PermissionPromptToolName != "stdio" {
		t.Errorf("Expected PermissionPromptToolName = 'stdio', got %q", *impl.options.PermissionPromptToolName)
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

// TestClientGetMcpStatus covers the public Client.GetMcpStatus() API end-to-end:
// delegation to transport when connected, error when disconnected, and
// propagation of the transport error.
func TestClientGetMcpStatus(t *testing.T) {
	ctx, cancel := setupClientTestContext(t, 5*time.Second)
	defer cancel()

	t.Run("returns_response_when_connected", func(t *testing.T) {
		expected := &McpStatusResponse{
			McpServers: []McpServerStatus{
				{Name: "svr1", Status: McpServerConnectionStatus("connected")},
			},
		}
		transport := newClientMockTransportWithOptions(WithClientMcpStatus(expected))
		client := setupClientForTest(t, transport)
		connectClientSafely(ctx, t, client)
		defer disconnectClientSafely(t, client)

		got, err := client.GetMcpStatus(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got == nil || len(got.McpServers) != 1 {
			t.Fatalf("expected 1 server, got %+v", got)
			return
		}
		if got.McpServers[0].Name != "svr1" {
			t.Errorf("expected svr1, got %s", got.McpServers[0].Name)
		}
	})

	t.Run("errors_when_not_connected", func(t *testing.T) {
		client := NewClient()
		_, err := client.GetMcpStatus(ctx)
		if err == nil {
			t.Fatal("expected error when not connected, got nil")
		}
		if !strings.Contains(err.Error(), "not connected") {
			t.Errorf("expected 'not connected' error, got: %v", err)
		}
	})

	t.Run("propagates_transport_error", func(t *testing.T) {
		transportErr := errors.New("cli busy")
		transport := newClientMockTransportWithOptions(WithClientMcpStatusError(transportErr))
		client := setupClientForTest(t, transport)
		connectClientSafely(ctx, t, client)
		defer disconnectClientSafely(t, client)

		_, err := client.GetMcpStatus(ctx)
		if err == nil {
			t.Fatal("expected transport error, got nil")
		}
		if !errors.Is(err, transportErr) && !strings.Contains(err.Error(), "cli busy") {
			t.Errorf("expected transport error to propagate, got: %v", err)
		}
	})
}

// Mock Transport Implementation - simplified following options_test.go patterns
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
	mcpStatusResponse      *McpStatusResponse
	mcpStatusError         error
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

func (c *clientMockTransport) SetPermissionMode(_ context.Context, _ string) error {
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
	if c.mcpStatusError != nil {
		return nil, c.mcpStatusError
	}
	if c.mcpStatusResponse != nil {
		return c.mcpStatusResponse, nil
	}
	return &McpStatusResponse{McpServers: []McpServerStatus{}}, nil
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

func WithClientMcpStatus(resp *McpStatusResponse) ClientMockTransportOption {
	return func(t *clientMockTransport) { t.mcpStatusResponse = resp }
}

func WithClientMcpStatusError(err error) ClientMockTransportOption {
	return func(t *clientMockTransport) { t.mcpStatusError = err }
}

func WithClientRewindFilesError(err error) ClientMockTransportOption {
	return func(t *clientMockTransport) { t.rewindFilesError = err }
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

// TestClientContextManager tests Go-idiomatic context manager pattern following Python SDK parity
// Covers the single critical improvement: automatic resource lifecycle management
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
	if role, ok := messageMap["role"]; !ok || role != "user" {
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

	// Test that iterator can be closed immediately (following existing test patterns)
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
					msgChan: msgChan,
					errChan: errChan,
					closed:  true, // Already closed
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
					msgChan: msgChan,
					errChan: errChan,
					closed:  false,
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
					msgChan: msgChan,
					errChan: errChan,
					closed:  false,
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
					msgChan: msgChan,
					errChan: errChan,
					closed:  false,
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

// ===== NEW CLEAN API TESTS =====
// These tests are for the new clean Query API without variadic parameters

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

// TestGetServerInfo tests the GetServerInfo method for diagnostic information retrieval
// Covers Issue #13: Add GetServerInfo Method for Diagnostics
func TestGetServerInfo(t *testing.T) {
	tests := []struct {
		name     string
		setup    func() (*clientMockTransport, Client)
		connect  bool
		wantErr  bool
		validate func(*testing.T, map[string]interface{}, error)
	}{
		{
			name: "returns_error_when_not_connected",
			setup: func() (*clientMockTransport, Client) {
				transport := newClientMockTransport()
				client := setupClientForTest(t, transport)
				return transport, client
			},
			connect: false,
			wantErr: true,
			validate: func(t *testing.T, info map[string]interface{}, err error) {
				t.Helper()
				if err == nil {
					t.Error("Expected error when not connected")
					return
				}
				if !strings.Contains(err.Error(), "not connected") {
					t.Errorf("Expected error to contain 'not connected', got: %v", err)
				}
				if info != nil {
					t.Errorf("Expected nil info when not connected, got: %v", info)
				}
			},
		},
		{
			name: "returns_info_when_connected",
			setup: func() (*clientMockTransport, Client) {
				transport := newClientMockTransport()
				client := setupClientForTest(t, transport)
				return transport, client
			},
			connect: true,
			wantErr: false,
			validate: func(t *testing.T, info map[string]interface{}, err error) {
				t.Helper()
				if err != nil {
					t.Errorf("Expected no error, got: %v", err)
					return
				}
				if info == nil {
					t.Error("Expected info map, got nil")
					return
				}
				// Verify expected fields
				if connected, ok := info["connected"].(bool); !ok || !connected {
					t.Errorf("Expected connected=true, got: %v", info["connected"])
				}
				if transportType, ok := info["transport_type"].(string); !ok || transportType != "subprocess" {
					t.Errorf("Expected transport_type='subprocess', got: %v", info["transport_type"])
				}
			},
		},
		{
			name: "returns_info_after_query",
			setup: func() (*clientMockTransport, Client) {
				transport := newClientMockTransport()
				client := setupClientForTest(t, transport)
				return transport, client
			},
			connect: true,
			wantErr: false,
			validate: func(t *testing.T, info map[string]interface{}, err error) {
				t.Helper()
				if err != nil {
					t.Errorf("Expected no error after query, got: %v", err)
					return
				}
				if info == nil {
					t.Error("Expected info map after query, got nil")
					return
				}
				// Should still show connected after operations
				if connected, ok := info["connected"].(bool); !ok || !connected {
					t.Errorf("Expected connected=true after query, got: %v", info["connected"])
				}
			},
		},
		{
			name: "returns_error_after_disconnect",
			setup: func() (*clientMockTransport, Client) {
				transport := newClientMockTransport()
				client := setupClientForTest(t, transport)
				return transport, client
			},
			connect: true, // Will connect then disconnect before calling GetServerInfo
			wantErr: true,
			validate: func(t *testing.T, info map[string]interface{}, err error) {
				t.Helper()
				if err == nil {
					t.Error("Expected error after disconnect")
					return
				}
				if !strings.Contains(err.Error(), "not connected") {
					t.Errorf("Expected error to contain 'not connected', got: %v", err)
				}
				if info != nil {
					t.Errorf("Expected nil info after disconnect, got: %v", info)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := setupClientTestContext(t, 5*time.Second)
			defer cancel()

			_, client := test.setup()
			defer disconnectClientSafely(t, client)

			if test.connect {
				connectClientSafely(ctx, t, client)
			}

			// For the "after query" test, send a query first
			if test.name == "returns_info_after_query" {
				err := client.Query(ctx, "test message")
				assertNoError(t, err)
			}

			// For the "after disconnect" test, disconnect before calling GetServerInfo
			if test.name == "returns_error_after_disconnect" {
				disconnectClientSafely(t, client)
			}

			info, err := client.GetServerInfo(ctx)
			test.validate(t, info, err)
		})
	}
}

// TestGetServerInfoConcurrent tests thread-safety of GetServerInfo
func TestGetServerInfoConcurrent(t *testing.T) {
	ctx, cancel := setupClientTestContext(t, 15*time.Second)
	defer cancel()

	transport := newClientMockTransport()
	client := setupClientForTest(t, transport)
	defer disconnectClientSafely(t, client)

	connectClientSafely(ctx, t, client)

	const numGoroutines = 10
	errors := make(chan error, numGoroutines)
	results := make(chan map[string]interface{}, numGoroutines)

	var wg sync.WaitGroup
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			info, err := client.GetServerInfo(ctx)
			if err != nil {
				errors <- err
				return
			}
			results <- info
		}()
	}

	wg.Wait()
	close(errors)
	close(results)

	// Check for errors
	for err := range errors {
		t.Errorf("Concurrent GetServerInfo error: %v", err)
	}

	// Verify all results are consistent
	var prevConnected bool
	first := true
	for info := range results {
		connected := info["connected"].(bool)
		if first {
			prevConnected = connected
			first = false
		} else if connected != prevConnected {
			t.Error("Inconsistent connected state across concurrent calls")
		}
	}
}

// =============================================================================
// Dynamic Control Methods Tests (SetModel, SetPermissionMode) - Issues #51, #52
// =============================================================================

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

// =============================================================================
// RewindFiles Tests (Issue #32)
// =============================================================================

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
