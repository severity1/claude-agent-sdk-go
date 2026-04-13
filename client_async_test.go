package claudecode

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// ===== TEST FUNCTIONS (PRIMARY PURPOSE) =====

// TestClientQueryAsync tests basic async query functionality
// Verifies that QueryAsync returns immediately with a handle and processes query in background
func TestClientQueryAsync(t *testing.T) {
	ctx, cancel := setupClientAsyncTestContext(t, 10*time.Second)
	defer cancel()

	transport := newMockTransportForAsync()
	client := setupClientForTest(t, transport)
	defer disconnectClientSafely(t, client)

	connectClientSafely(ctx, t, client)

	// Execute async query - should return immediately
	startTime := time.Now()
	handle, err := client.QueryAsync(ctx, "What is 2+2?")
	elapsed := time.Since(startTime)

	assertNoError(t, err)
	if handle == nil {
		t.Fatal("Expected QueryHandle, got nil")
	}

	// Verify returned immediately (< 100ms)
	if elapsed > 100*time.Millisecond {
		t.Errorf("QueryAsync took too long: %v (expected < 100ms)", elapsed)
	}

	// Verify handle has correct session ID
	if handle.SessionID() != defaultSessionID {
		t.Errorf("Expected session ID 'default', got '%s'", handle.SessionID())
	}

	// Verify handle has non-empty ID
	if handle.ID() == "" {
		t.Error("Expected non-empty query ID")
	}

	// Verify initial status is queued or processing
	status := handle.Status()
	if status != QueryStatusQueued && status != QueryStatusProcessing {
		t.Errorf("Expected status queued or processing, got %s", status.String())
	}

	// Configure transport to complete successfully
	transport.setSimulateSuccess()

	// Wait for completion
	waitErr := handle.Wait()
	assertNoError(t, waitErr)

	// Verify final status is completed
	if handle.Status() != QueryStatusCompleted {
		t.Errorf("Expected status completed, got %s", handle.Status().String())
	}

	// Verify message was sent to transport
	assertClientMessageCount(t, transport.clientMockTransport, 1)

	sentMsg, ok := transport.getSentMessage(0)
	if !ok {
		t.Fatal("Failed to get sent message")
	}

	if sentMsg.Type != userMessageType {
		t.Errorf("Expected message type 'user', got '%s'", sentMsg.Type)
	}
}

// TestClientQueryWithSessionAsync tests session-specific async queries
// Verifies that QueryWithSessionAsync correctly propagates custom session IDs
func TestClientQueryWithSessionAsync(t *testing.T) {
	ctx, cancel := setupClientAsyncTestContext(t, 10*time.Second)
	defer cancel()

	transport := newMockTransportForAsync()
	client := setupClientForTest(t, transport)
	defer disconnectClientSafely(t, client)

	connectClientSafely(ctx, t, client)

	customSession := "custom-session-123"

	// Execute async query with custom session
	handle, err := client.QueryWithSessionAsync(ctx, "Test query", customSession)
	assertNoError(t, err)

	if handle == nil {
		t.Fatal("Expected QueryHandle, got nil")
	}

	// Verify handle has correct session ID
	if handle.SessionID() != customSession {
		t.Errorf("Expected session ID '%s', got '%s'", customSession, handle.SessionID())
	}

	// Configure transport for success
	transport.setSimulateSuccess()

	// Wait for completion
	waitErr := handle.Wait()
	assertNoError(t, waitErr)

	// Verify message was sent with correct session ID
	assertClientMessageCount(t, transport.clientMockTransport, 1)

	sentMsg, ok := transport.getSentMessage(0)
	if !ok {
		t.Fatal("Failed to get sent message")
	}

	if sentMsg.SessionID != customSession {
		t.Errorf("Expected session ID '%s', got '%s'", customSession, sentMsg.SessionID)
	}

	// Test with empty session ID - should use default
	handle2, err := client.QueryWithSessionAsync(ctx, "Test query 2", "")
	assertNoError(t, err)

	if handle2.SessionID() != defaultSessionID {
		t.Errorf("Expected default session ID when empty provided, got '%s'", handle2.SessionID())
	}

	transport.setSimulateSuccess()
	waitErr = handle2.Wait()
	assertNoError(t, waitErr)
}

// TestQueryAsyncNotConnected tests error when client is not connected
// Verifies that async queries fail immediately if client is not connected
func TestQueryAsyncNotConnected(t *testing.T) {
	ctx, cancel := setupClientAsyncTestContext(t, 5*time.Second)
	defer cancel()

	transport := newMockTransportForAsync()
	client := setupClientForTest(t, transport)
	defer disconnectClientSafely(t, client)

	// Don't connect client

	// Attempt async query without connection
	handle, err := client.QueryAsync(ctx, "test query")

	if err == nil {
		t.Fatal("Expected error when not connected, got nil")
	}

	if handle != nil {
		t.Errorf("Expected nil handle when not connected, got %v", handle)
	}

	// Same test for QueryWithSessionAsync
	handle2, err2 := client.QueryWithSessionAsync(ctx, "test query", "session")

	if err2 == nil {
		t.Fatal("Expected error when not connected, got nil")
	}

	if handle2 != nil {
		t.Errorf("Expected nil handle when not connected, got %v", handle2)
	}
}

// TestQueryHandleWait tests that Wait() blocks until completion
// Verifies blocking behavior and proper error handling
func TestQueryHandleWait(t *testing.T) {
	ctx, cancel := setupClientAsyncTestContext(t, 15*time.Second)
	defer cancel()

	tests := []struct {
		name          string
		simulateDelay time.Duration
		simulateError error
		wantErr       bool
	}{
		{
			name:          "successful_completion",
			simulateDelay: 100 * time.Millisecond,
			simulateError: nil,
			wantErr:       false,
		},
		{
			name:          "completion_with_error",
			simulateDelay: 50 * time.Millisecond,
			simulateError: fmt.Errorf("query execution failed"),
			wantErr:       true,
		},
		{
			name:          "immediate_completion",
			simulateDelay: 0,
			simulateError: nil,
			wantErr:       false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transport := newMockTransportForAsync()
			client := setupClientForTest(t, transport)
			defer disconnectClientSafely(t, client)

			connectClientSafely(ctx, t, client)

			handle, err := client.QueryAsync(ctx, "test query")
			assertNoError(t, err)

			// Configure transport behavior
			if test.simulateError != nil {
				transport.setSimulateError(test.simulateError)
			} else {
				transport.setSimulateSuccess()
			}

			if test.simulateDelay > 0 {
				transport.setSimulateDelay(test.simulateDelay)
			}

			// Wait should block for at least the simulated delay
			startTime := time.Now()
			waitErr := handle.Wait()
			elapsed := time.Since(startTime)

			if test.wantErr {
				if waitErr == nil {
					t.Error("Expected error from Wait(), got nil")
				}
			} else {
				assertNoError(t, waitErr)
			}

			// Verify blocking behavior
			if test.simulateDelay > 0 && elapsed < test.simulateDelay {
				t.Errorf("Wait() returned too quickly: %v (expected >= %v)", elapsed, test.simulateDelay)
			}

			// Multiple Wait() calls should return same error/result
			waitErr2 := handle.Wait()
			if test.wantErr {
				if waitErr2 == nil {
					t.Error("Expected error from second Wait(), got nil")
				}
			} else {
				assertNoError(t, waitErr2)
			}
		})
	}
}

// TestQueryHandleCancellation tests Cancel() stops query execution
// Verifies cancellation propagates correctly and status updates
func TestQueryHandleCancellation(t *testing.T) {
	ctx, cancel := setupClientAsyncTestContext(t, 10*time.Second)
	defer cancel()

	transport := newMockTransportForAsync()
	client := setupClientForTest(t, transport)
	defer disconnectClientSafely(t, client)

	connectClientSafely(ctx, t, client)

	// Start a long-running async query
	handle, err := client.QueryAsync(ctx, "long running query")
	assertNoError(t, err)

	// Configure transport with long delay
	transport.setSimulateDelay(5 * time.Second)
	transport.setSimulateSuccess()

	// Cancel the query after a short delay
	go func() {
		time.Sleep(100 * time.Millisecond)
		handle.Cancel()
	}()

	// Wait should return quickly due to cancellation
	startTime := time.Now()
	_ = handle.Wait()
	elapsed := time.Since(startTime)

	// Should complete faster than the 5 second delay
	if elapsed > 2*time.Second {
		t.Errorf("Cancel() did not stop query quickly enough: %v", elapsed)
	}

	// Wait should return error or nil depending on implementation
	// Status should be cancelled
	if handle.Status() != QueryStatusCancelled {
		t.Errorf("Expected status cancelled after Cancel(), got %s", handle.Status().String())
	}

	// Done channel should be closed
	select {
	case <-handle.Done():
		// Expected
	case <-time.After(100 * time.Millisecond):
		t.Error("Done channel not closed after cancellation")
	}

	// Subsequent Cancel() calls should be safe
	handle.Cancel()
	handle.Cancel()

	// Multiple Wait() calls after cancel should still work
	_ = handle.Wait()
}

// TestQueryHandleErrorPropagation tests errors flow through error channel
// Verifies error channel behavior and proper error handling
func TestQueryHandleErrorPropagation(t *testing.T) {
	ctx, cancel := setupClientAsyncTestContext(t, 10*time.Second)
	defer cancel()

	transport := newMockTransportForAsync()
	client := setupClientForTest(t, transport)
	defer disconnectClientSafely(t, client)

	connectClientSafely(ctx, t, client)

	handle, err := client.QueryAsync(ctx, "test query")
	assertNoError(t, err)

	// Configure transport to simulate error
	expectedError := fmt.Errorf("transport error occurred")
	transport.setSimulateError(expectedError)

	// Error should be available on error channel
	errChan := handle.Errors()

	select {
	case receivedErr := <-errChan:
		if receivedErr == nil {
			t.Error("Expected error on error channel, got nil")
		} else if receivedErr.Error() != expectedError.Error() {
			t.Errorf("Expected error '%v', got '%v'", expectedError, receivedErr)
		}
	case <-time.After(2 * time.Second):
		t.Error("Timeout waiting for error on error channel")
	}

	// Wait should also return the error
	waitErr := handle.Wait()
	if waitErr == nil {
		t.Error("Expected error from Wait(), got nil")
	}

	// Status should be failed
	if handle.Status() != QueryStatusFailed {
		t.Errorf("Expected status failed after error, got %s", handle.Status().String())
	}
}

// TestConcurrentAsyncQueries tests multiple simultaneous queries
// Verifies thread safety and proper isolation between queries
func TestConcurrentAsyncQueries(t *testing.T) {
	ctx, cancel := setupClientAsyncTestContext(t, 30*time.Second)
	defer cancel()

	transport := newMockTransportForAsync()
	client := setupClientForTest(t, transport)
	defer disconnectClientSafely(t, client)

	connectClientSafely(ctx, t, client)

	// Configure transport for success
	transport.setSimulateSuccess()

	const numQueries = 10

	var wg sync.WaitGroup
	handles := make([]QueryHandle, numQueries)
	errors := make(chan error, numQueries)

	// Launch concurrent async queries
	for i := 0; i < numQueries; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			handle, err := client.QueryAsync(ctx, fmt.Sprintf("concurrent query %d", index))
			if err != nil {
				errors <- fmt.Errorf("query %d failed to start: %w", index, err)
				return
			}

			handles[index] = handle

			// Wait for completion
			if err := handle.Wait(); err != nil {
				errors <- fmt.Errorf("query %d failed to complete: %w", index, err)
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Check for any errors
	for err := range errors {
		t.Errorf("Concurrent query error: %v", err)
	}

	// Verify all queries completed successfully
	for i, handle := range handles {
		if handle == nil {
			t.Errorf("Handle %d is nil", i)
			continue
		}

		if handle.Status() != QueryStatusCompleted {
			t.Errorf("Handle %d status: expected completed, got %s", i, handle.Status().String())
		}

		// Verify each handle has unique ID
		for j := i + 1; j < numQueries; j++ {
			if handles[j] != nil && handle.ID() == handles[j].ID() {
				t.Errorf("Handles %d and %d have duplicate IDs: %s", i, j, handle.ID())
			}
		}
	}

	// Verify all messages were sent
	expectedMessages := numQueries
	assertClientMessageCount(t, transport.clientMockTransport, expectedMessages)
}

// TestQueryHandleMessageStreaming tests messages arrive in order
// Verifies message channel behavior and ordering
func TestQueryHandleMessageStreaming(t *testing.T) {
	ctx, cancel := setupClientAsyncTestContext(t, 10*time.Second)
	defer cancel()

	transport := newMockTransportForAsync()
	client := setupClientForTest(t, transport)
	defer disconnectClientSafely(t, client)

	connectClientSafely(ctx, t, client)

	handle, err := client.QueryAsync(ctx, "test query")
	assertNoError(t, err)

	// Configure transport to send multiple messages
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

	transport.setSimulateMessages(testMessages)
	transport.setSimulateSuccess()

	// Receive messages in order
	msgChan := handle.Messages()
	expectedTexts := []string{"First response", "Second response", "Third response"}
	receivedCount := 0

	for i, expectedText := range expectedTexts {
		select {
		case msg := <-msgChan:
			if msg == nil {
				t.Fatalf("Received nil message at index %d", i)
			}

			assistantMsg, ok := msg.(*AssistantMessage)
			if !ok {
				t.Errorf("Expected AssistantMessage at index %d, got %T", i, msg)
				continue
			}

			if len(assistantMsg.Content) != 1 {
				t.Errorf("Expected 1 content block at index %d, got %d", i, len(assistantMsg.Content))
				continue
			}

			textBlock, ok := assistantMsg.Content[0].(*TextBlock)
			if !ok {
				t.Errorf("Expected TextBlock at index %d, got %T", i, assistantMsg.Content[0])
				continue
			}

			if textBlock.Text != expectedText {
				t.Errorf("Expected message %d text '%s', got '%s'", i, expectedText, textBlock.Text)
			}

			receivedCount++

		case <-time.After(2 * time.Second):
			t.Fatalf("Timeout waiting for message %d", i)
		}
	}

	if receivedCount != len(expectedTexts) {
		t.Errorf("Expected %d messages, received %d", len(expectedTexts), receivedCount)
	}

	// Wait for completion
	waitErr := handle.Wait()
	assertNoError(t, waitErr)

	// Verify status
	if handle.Status() != QueryStatusCompleted {
		t.Errorf("Expected status completed, got %s", handle.Status().String())
	}
}

// TestQueryHandleStatusTransitions tests status changes correctly
// Verifies state machine transitions from queued -> processing -> completed/failed/cancelled
func TestQueryHandleStatusTransitions(t *testing.T) {
	ctx, cancel := setupClientAsyncTestContext(t, 10*time.Second)
	defer cancel()

	tests := []struct {
		name                string
		simulateError       error
		shouldCancel        bool
		expectedFinalStatus QueryStatus
	}{
		{
			name:                "success_transition",
			simulateError:       nil,
			shouldCancel:        false,
			expectedFinalStatus: QueryStatusCompleted,
		},
		{
			name:                "error_transition",
			simulateError:       fmt.Errorf("execution error"),
			shouldCancel:        false,
			expectedFinalStatus: QueryStatusFailed,
		},
		{
			name:                "cancel_transition",
			simulateError:       nil,
			shouldCancel:        true,
			expectedFinalStatus: QueryStatusCancelled,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transport := newMockTransportForAsync()
			client := setupClientForTest(t, transport)
			defer disconnectClientSafely(t, client)

			connectClientSafely(ctx, t, client)

			handle, err := client.QueryAsync(ctx, "test query")
			assertNoError(t, err)

			// Verify initial status
			initialStatus := handle.Status()
			if initialStatus != QueryStatusQueued && initialStatus != QueryStatusProcessing {
				t.Errorf("Expected initial status queued or processing, got %s", initialStatus.String())
			}

			// Configure transport behavior
			if test.simulateError != nil {
				transport.setSimulateError(test.simulateError)
			} else {
				transport.setSimulateSuccess()
			}

			transport.setSimulateDelay(100 * time.Millisecond)

			// Cancel if needed
			if test.shouldCancel {
				go func() {
					time.Sleep(50 * time.Millisecond)
					handle.Cancel()
				}()
			}

			// Wait for completion
			_ = handle.Wait()

			// Verify final status
			finalStatus := handle.Status()
			if finalStatus != test.expectedFinalStatus {
				t.Errorf("Expected final status %s, got %s",
					test.expectedFinalStatus.String(), finalStatus.String())
			}

			// Verify Done channel is closed
			select {
			case <-handle.Done():
				// Expected
			case <-time.After(100 * time.Millisecond):
				t.Error("Done channel not closed after completion")
			}
		})
	}
}

// TestQueryTrackingCleanup tests active queries map cleaned up properly
// Verifies that completed queries are removed from tracking
func TestQueryTrackingCleanup(t *testing.T) {
	ctx, cancel := setupClientAsyncTestContext(t, 15*time.Second)
	defer cancel()

	transport := newMockTransportForAsync()
	client := setupClientForTest(t, transport)
	defer disconnectClientSafely(t, client)

	connectClientSafely(ctx, t, client)

	// Configure transport for success
	transport.setSimulateSuccess()

	// Start multiple async queries
	const numQueries = 5
	handles := make([]QueryHandle, numQueries)

	for i := 0; i < numQueries; i++ {
		handle, err := client.QueryAsync(ctx, fmt.Sprintf("query %d", i))
		assertNoError(t, err)
		handles[i] = handle
	}

	// Verify queries are tracked (internal implementation detail, tested via behavior)
	// Wait for all queries to complete
	for i, handle := range handles {
		if err := handle.Wait(); err != nil {
			t.Errorf("Query %d failed: %v", i, err)
		}
	}

	// All queries should be completed
	for i, handle := range handles {
		if handle.Status() != QueryStatusCompleted {
			t.Errorf("Query %d: expected status completed, got %s", i, handle.Status().String())
		}
	}

	// Start new queries - if cleanup works, these should succeed without issues
	for i := 0; i < 3; i++ {
		handle, err := client.QueryAsync(ctx, fmt.Sprintf("new query %d", i))
		assertNoError(t, err)

		if err := handle.Wait(); err != nil {
			t.Errorf("New query %d failed: %v", i, err)
		}
	}

	// Total messages sent should be numQueries + 3
	expectedTotal := numQueries + 3
	assertClientMessageCount(t, transport.clientMockTransport, expectedTotal)
}

// TestQueryHandleInterfaceCompliance tests QueryHandle interface is properly implemented
// Verifies all interface methods are available and work correctly
func TestQueryHandleInterfaceCompliance(t *testing.T) {
	ctx, cancel := setupClientAsyncTestContext(t, 10*time.Second)
	defer cancel()

	transport := newMockTransportForAsync()
	client := setupClientForTest(t, transport)
	defer disconnectClientSafely(t, client)

	connectClientSafely(ctx, t, client)

	handle, err := client.QueryAsync(ctx, "test query")
	assertNoError(t, err)

	// Test all interface methods

	// ID() returns non-empty string
	id := handle.ID()
	if id == "" {
		t.Error("ID() returned empty string")
	}

	// SessionID() returns correct session
	sessionID := handle.SessionID()
	if sessionID != defaultSessionID {
		t.Errorf("SessionID() returned '%s', expected '%s'", sessionID, defaultSessionID)
	}

	// Status() returns valid status
	status := handle.Status()
	validStatuses := map[QueryStatus]bool{
		QueryStatusQueued:     true,
		QueryStatusProcessing: true,
		QueryStatusCompleted:  true,
		QueryStatusFailed:     true,
		QueryStatusCancelled:  true,
	}
	if !validStatuses[status] {
		t.Errorf("Status() returned invalid status: %s", status.String())
	}

	// Messages() returns non-nil channel
	msgChan := handle.Messages()
	if msgChan == nil {
		t.Error("Messages() returned nil channel")
	}

	// Errors() returns non-nil channel
	errChan := handle.Errors()
	if errChan == nil {
		t.Error("Errors() returned nil channel")
	}

	// Done() returns non-nil channel
	doneChan := handle.Done()
	if doneChan == nil {
		t.Error("Done() returned nil channel")
	}

	// Configure transport for success
	transport.setSimulateSuccess()

	// Wait() blocks until completion
	waitErr := handle.Wait()
	assertNoError(t, waitErr)

	// Cancel() is callable (even after completion)
	handle.Cancel()

	// Verify QueryStatus constants are defined
	_ = QueryStatusQueued
	_ = QueryStatusProcessing
	_ = QueryStatusCompleted
	_ = QueryStatusFailed
	_ = QueryStatusCancelled

	// Verify String() method works for all statuses
	statuses := []QueryStatus{
		QueryStatusQueued,
		QueryStatusProcessing,
		QueryStatusCompleted,
		QueryStatusFailed,
		QueryStatusCancelled,
	}

	for _, qs := range statuses {
		str := qs.String()
		if str == "" || str == statusUnknown {
			t.Errorf("Status %d String() returned invalid string: '%s'", qs, str)
		}
	}
}

// ===== MOCK IMPLEMENTATIONS (SUPPORTING TYPES) =====

// mockTransportForAsync is a thread-safe mock transport for async testing
type mockTransportForAsync struct {
	*clientMockTransport // Embed for basic transport functionality

	mu sync.Mutex

	// Simulation controls
	simulateDelay    time.Duration
	simulateError    error
	simulateMessages []Message
	simulateSuccess  bool
}

// newMockTransportForAsync creates a new mock transport for async testing
func newMockTransportForAsync() *mockTransportForAsync {
	return &mockTransportForAsync{
		clientMockTransport: newClientMockTransport(),
		simulateSuccess:     false,
	}
}

func (m *mockTransportForAsync) setSimulateDelay(delay time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.simulateDelay = delay
}

func (m *mockTransportForAsync) setSimulateError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.simulateError = err
	m.simulateSuccess = false
}

func (m *mockTransportForAsync) setSimulateMessages(messages []Message) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.simulateMessages = messages
}

func (m *mockTransportForAsync) setSimulateSuccess() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.simulateSuccess = true
	m.simulateError = nil
}

// SendMessage simulates async behavior for testing
func (m *mockTransportForAsync) SendMessage(ctx context.Context, message StreamMessage) error {
	// First call base implementation to track message
	if err := m.clientMockTransport.SendMessage(ctx, message); err != nil {
		return err
	}

	m.mu.Lock()
	delay := m.simulateDelay
	shouldError := m.simulateError
	messages := m.simulateMessages
	success := m.simulateSuccess
	m.mu.Unlock()

	// Simulate async processing in goroutine
	go func() {
		// Simulate delay if configured
		if delay > 0 {
			time.Sleep(delay)
		}

		// Check if we should simulate error
		if shouldError != nil {
			m.clientMockTransport.mu.Lock()
			if m.errChan != nil {
				select {
				case m.errChan <- shouldError:
				default:
				}
			}
			m.clientMockTransport.mu.Unlock()
			return
		}

		// Send simulated messages if configured
		if len(messages) > 0 {
			m.clientMockTransport.mu.Lock()
			if m.msgChan != nil {
				for _, msg := range messages {
					select {
					case m.msgChan <- msg:
					default:
					}
				}
			}
			m.clientMockTransport.mu.Unlock()
		}

		// If success flag set, send a ResultMessage to signal completion
		if success {
			m.clientMockTransport.mu.Lock()
			if m.msgChan != nil {
				// Send a ResultMessage to signal query completion
				resultMsg := &ResultMessage{
					MessageType:   MessageTypeResult,
					Subtype:       "result",
					DurationMs:    100,
					DurationAPIMs: 50,
					IsError:       false,
					NumTurns:      1,
					SessionID:     "default",
				}
				select {
				case m.msgChan <- resultMsg:
				default:
				}
			}
			m.clientMockTransport.mu.Unlock()
		}
	}()

	return nil
}

// ===== HELPER FUNCTIONS (UTILITIES) =====

// setupClientAsyncTestContext creates a context with timeout for async tests
func setupClientAsyncTestContext(t *testing.T, timeout time.Duration) (context.Context, context.CancelFunc) {
	t.Helper()
	return context.WithTimeout(context.Background(), timeout)
}
