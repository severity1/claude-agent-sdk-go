package claudecode

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// Test constants
const (
	testSessionID = "test-session-1"
	testMessage1  = "Message 1"
)

// ===== TEST FUNCTIONS (PRIMARY PURPOSE) =====

// TestQueueManagerEnqueue tests adding messages to queue without immediate sending
// Verifies that messages are queued (not sent immediately) and QueuedMessage returned with correct fields
func TestQueueManagerEnqueue(t *testing.T) {
	ctx, cancel := setupQueueTestContext(t, 10*time.Second)
	defer cancel()

	qm, mockClient := setupQueueTestManager(t)
	defer closeQueueSafely(t, qm)

	sessionID := testSessionID

	// Enqueue a message
	queuedMsg, err := qm.Enqueue(ctx, sessionID, "First message")
	assertNoError(t, err)

	// Verify QueuedMessage fields are correct
	if queuedMsg == nil {
		t.Fatal("Expected QueuedMessage, got nil")
	}

	if queuedMsg.ID == "" {
		t.Error("Expected non-empty message ID")
	}

	if queuedMsg.Content != "First message" {
		t.Errorf("Expected content 'First message', got '%s'", queuedMsg.Content)
	}

	if queuedMsg.Status != MessageStatusQueued {
		t.Errorf("Expected status queued, got %s", queuedMsg.Status)
	}

	if queuedMsg.Timestamp.IsZero() {
		t.Error("Expected non-zero timestamp")
	}

	// Verify message was NOT sent immediately to client
	// Give some time for any async processing
	time.Sleep(50 * time.Millisecond)

	if mockClient.getSentQueryCount() != 0 {
		t.Errorf("Expected 0 queries sent immediately, got %d", mockClient.getSentQueryCount())
	}

	// Verify queue length increased
	queueLen := qm.GetQueueLength(sessionID)
	if queueLen != 1 {
		t.Errorf("Expected queue length 1, got %d", queueLen)
	}

	// Enqueue multiple messages
	queuedMsg2, err := qm.Enqueue(ctx, sessionID, "Second message")
	assertNoError(t, err)

	queuedMsg3, err := qm.Enqueue(ctx, sessionID, "Third message")
	assertNoError(t, err)

	// Verify queue length updated correctly
	queueLen = qm.GetQueueLength(sessionID)
	if queueLen != 3 {
		t.Errorf("Expected queue length 3, got %d", queueLen)
	}

	// Verify each message has unique ID
	if queuedMsg.ID == queuedMsg2.ID || queuedMsg.ID == queuedMsg3.ID || queuedMsg2.ID == queuedMsg3.ID {
		t.Error("Expected unique message IDs for each queued message")
	}
}

// TestQueueManagerRemoveFromQueue tests removing messages before they are sent
// Verifies message removal works and errors when removing already-processing or non-existent messages
func TestQueueManagerRemoveFromQueue(t *testing.T) {
	ctx, cancel := setupQueueTestContext(t, 10*time.Second)
	defer cancel()

	qm, mockClient := setupQueueTestManager(t)
	defer closeQueueSafely(t, qm)

	sessionID := testSessionID

	// Pause queue to prevent automatic processing
	err := qm.PauseQueue(sessionID)
	assertNoError(t, err)

	// Enqueue multiple messages
	_, err = qm.Enqueue(ctx, sessionID, testMessage1)
	assertNoError(t, err)

	msg2, err := qm.Enqueue(ctx, sessionID, "Message 2")
	assertNoError(t, err)

	_, err = qm.Enqueue(ctx, sessionID, "Message 3")
	assertNoError(t, err)

	// Verify queue has 3 messages
	if qm.GetQueueLength(sessionID) != 3 {
		t.Fatalf("Expected queue length 3, got %d", qm.GetQueueLength(sessionID))
	}

	// Remove message 2 from queue
	err = qm.RemoveFromQueue(sessionID, msg2.ID)
	assertNoError(t, err)

	// Verify queue length decreased
	if qm.GetQueueLength(sessionID) != 2 {
		t.Errorf("Expected queue length 2 after removal, got %d", qm.GetQueueLength(sessionID))
	}

	// Resume queue and wait for processing
	err = qm.ResumeQueue(sessionID)
	assertNoError(t, err)

	// Wait for messages to be sent
	time.Sleep(200 * time.Millisecond)

	// Verify only 2 messages were sent (msg1 and msg3, not msg2)
	sentCount := mockClient.getSentQueryCount()
	if sentCount != 2 {
		t.Errorf("Expected 2 queries sent, got %d", sentCount)
	}

	// Verify the correct messages were sent (msg2 should be missing)
	sentQueries := mockClient.getSentQueries()
	if len(sentQueries) != 2 {
		t.Fatalf("Expected 2 sent queries, got %d", len(sentQueries))
	}

	// Should have msg1 and msg3, not msg2
	hasMsg1 := false
	hasMsg2 := false
	hasMsg3 := false

	for _, query := range sentQueries {
		switch query.prompt {
		case testMessage1:
			hasMsg1 = true
		case "Message 2":
			hasMsg2 = true
		case "Message 3":
			hasMsg3 = true
		}
	}

	if !hasMsg1 {
		t.Error("Expected testMessage1 to be sent")
	}
	if hasMsg2 {
		t.Error("Expected Message 2 to NOT be sent (was removed)")
	}
	if !hasMsg3 {
		t.Error("Expected Message 3 to be sent")
	}

	// Test removing non-existent message
	err = qm.RemoveFromQueue(sessionID, "non-existent-id")
	if err == nil {
		t.Error("Expected error when removing non-existent message")
	}
}

// TestQueueManagerRemoveDuringProcessing tests removing a message that is already being processed
// Verifies error when trying to remove a message that's already being sent
func TestQueueManagerRemoveDuringProcessing(t *testing.T) {
	ctx, cancel := setupQueueTestContext(t, 15*time.Second)
	defer cancel()

	qm, mockClient := setupQueueTestManager(t)
	defer closeQueueSafely(t, qm)

	sessionID := testSessionID

	// Configure mock client to simulate slow processing
	mockClient.setQueryDelay(500 * time.Millisecond)

	// Enqueue a message (will start processing immediately)
	msg, err := qm.Enqueue(ctx, sessionID, "Processing message")
	assertNoError(t, err)

	// Wait for processing to start (initial delay 100ms + processing loop)
	time.Sleep(250 * time.Millisecond)

	// Try to remove the message while it's processing
	err = qm.RemoveFromQueue(sessionID, msg.ID)
	if err == nil {
		t.Error("Expected error when removing message that is already processing")
	}

	// Verify the message completes normally despite removal attempt
	time.Sleep(600 * time.Millisecond)

	sentCount := mockClient.getSentQueryCount()
	if sentCount != 1 {
		t.Errorf("Expected 1 query sent (message completed normally), got %d", sentCount)
	}
}

// TestQueueManagerClearQueue tests clearing entire queue
// Verifies all pending messages are removed but currently processing message is NOT cancelled
func TestQueueManagerClearQueue(t *testing.T) {
	ctx, cancel := setupQueueTestContext(t, 15*time.Second)
	defer cancel()

	qm, mockClient := setupQueueTestManager(t)
	defer closeQueueSafely(t, qm)

	sessionID := testSessionID

	// Configure slow processing to allow queue buildup
	mockClient.setQueryDelay(500 * time.Millisecond)

	// Enqueue multiple messages
	_, err := qm.Enqueue(ctx, sessionID, testMessage1)
	assertNoError(t, err)

	// Wait for processing to start (initial delay 100ms + processing loop start)
	time.Sleep(250 * time.Millisecond)

	// Enqueue more messages (these should be pending)
	_, err = qm.Enqueue(ctx, sessionID, "Message 2")
	assertNoError(t, err)

	_, err = qm.Enqueue(ctx, sessionID, "Message 3")
	assertNoError(t, err)

	_, err = qm.Enqueue(ctx, sessionID, "Message 4")
	assertNoError(t, err)

	// Verify queue has pending messages
	if qm.GetQueueLength(sessionID) < 2 {
		t.Fatalf("Expected at least 2 pending messages, got %d", qm.GetQueueLength(sessionID))
	}

	// Clear queue
	err = qm.ClearQueue(sessionID)
	assertNoError(t, err)

	// Verify pending messages removed
	if qm.GetQueueLength(sessionID) != 0 {
		t.Errorf("Expected 0 pending messages after clear, got %d", qm.GetQueueLength(sessionID))
	}

	// Wait for msg1 (processing message) to complete
	time.Sleep(600 * time.Millisecond)

	// Verify only msg1 was sent (processing message NOT cancelled)
	sentCount := mockClient.getSentQueryCount()
	if sentCount != 1 {
		t.Errorf("Expected 1 query sent (processing message), got %d", sentCount)
	}

	sentQueries := mockClient.getSentQueries()
	if len(sentQueries) != 1 {
		t.Fatalf("Expected 1 sent query, got %d", len(sentQueries))
	}

	if sentQueries[0].prompt != testMessage1 {
		t.Errorf("Expected 'testMessage1' to be sent, got '%s'", sentQueries[0].prompt)
	}

	// Verify currently processing message completed normally (msg1)
	if sentQueries[0].prompt != testMessage1 {
		t.Errorf("Expected processing message 'testMessage1', got '%s'", sentQueries[0].prompt)
	}
}

// TestQueueManagerGetQueueStatus tests querying queue state
// Verifies status includes queue length, processing message, and pending messages
func TestQueueManagerGetQueueStatus(t *testing.T) {
	ctx, cancel := setupQueueTestContext(t, 10*time.Second)
	defer cancel()

	qm, mockClient := setupQueueTestManager(t)
	defer closeQueueSafely(t, qm)

	sessionID := testSessionID

	// Test status with empty queue
	status, err := qm.GetQueueStatus(sessionID)
	assertNoError(t, err)

	if status == nil {
		t.Fatal("Expected QueueStatus, got nil")
	}

	if status.Length != 0 {
		t.Errorf("Expected empty queue length 0, got %d", status.Length)
	}

	if status.Processing != nil {
		t.Errorf("Expected no processing message, got %v", status.Processing)
	}

	if len(status.PendingMessages) != 0 {
		t.Errorf("Expected 0 pending messages, got %d", len(status.PendingMessages))
	}

	// Configure slow processing
	mockClient.setQueryDelay(500 * time.Millisecond)

	// Enqueue messages
	msg1, err := qm.Enqueue(ctx, sessionID, "Processing message")
	assertNoError(t, err)

	// Wait for processing to start (initial delay 100ms + processing loop)
	time.Sleep(250 * time.Millisecond)

	// Enqueue more pending messages
	msg2, err := qm.Enqueue(ctx, sessionID, "Pending message 1")
	assertNoError(t, err)

	msg3, err := qm.Enqueue(ctx, sessionID, "Pending message 2")
	assertNoError(t, err)

	// Get status with processing and pending messages
	status, err = qm.GetQueueStatus(sessionID)
	assertNoError(t, err)

	// Verify queue length (pending only)
	if status.Length < 2 {
		t.Errorf("Expected at least 2 pending messages, got %d", status.Length)
	}

	// Verify processing message is set
	if status.Processing == nil {
		t.Error("Expected processing message to be set")
	} else if status.Processing.ID != msg1.ID {
		t.Errorf("Expected processing message ID %s, got %s", msg1.ID, status.Processing.ID)
	}

	// Verify pending messages
	if len(status.PendingMessages) < 2 {
		t.Errorf("Expected at least 2 pending messages in status, got %d", len(status.PendingMessages))
	}

	// Verify pending messages contain msg2 and msg3
	foundMsg2 := false
	foundMsg3 := false
	for _, pendingMsg := range status.PendingMessages {
		if pendingMsg.ID == msg2.ID {
			foundMsg2 = true
		}
		if pendingMsg.ID == msg3.ID {
			foundMsg3 = true
		}
	}

	if !foundMsg2 {
		t.Error("Expected msg2 in pending messages")
	}
	if !foundMsg3 {
		t.Error("Expected msg3 in pending messages")
	}

	// Test status for non-existent session
	status, err = qm.GetQueueStatus("non-existent-session")
	if err != nil {
		// Either error or empty status is acceptable
		if status != nil && status.Length != 0 {
			t.Error("Expected empty status for non-existent session")
		}
	}
}

// TestQueueManagerGetQueueLength tests getting queue size
// Verifies length with various queue states (0, 1, 10) and that processing message not included
func TestQueueManagerGetQueueLength(t *testing.T) {
	ctx, cancel := setupQueueTestContext(t, 10*time.Second)
	defer cancel()

	qm, mockClient := setupQueueTestManager(t)
	defer closeQueueSafely(t, qm)

	sessionID := testSessionID

	// Test empty queue
	length := qm.GetQueueLength(sessionID)
	if length != 0 {
		t.Errorf("Expected empty queue length 0, got %d", length)
	}

	// Pause queue to prevent processing
	err := qm.PauseQueue(sessionID)
	assertNoError(t, err)

	// Test queue with 1 message
	_, err = qm.Enqueue(ctx, sessionID, testMessage1)
	assertNoError(t, err)

	length = qm.GetQueueLength(sessionID)
	if length != 1 {
		t.Errorf("Expected queue length 1, got %d", length)
	}

	// Test queue with 10 messages
	for i := 2; i <= 10; i++ {
		_, err = qm.Enqueue(ctx, sessionID, fmt.Sprintf("Message %d", i))
		assertNoError(t, err)
	}

	length = qm.GetQueueLength(sessionID)
	if length != 10 {
		t.Errorf("Expected queue length 10, got %d", length)
	}

	// Set query delay BEFORE resuming to ensure processing takes time
	mockClient.setQueryDelay(500 * time.Millisecond)

	// Resume queue and let one message start processing
	err = qm.ResumeQueue(sessionID)
	assertNoError(t, err)

	// Wait for processing to start
	time.Sleep(200 * time.Millisecond)

	// Verify processing message NOT included in pending count
	length = qm.GetQueueLength(sessionID)
	if length != 9 {
		t.Errorf("Expected queue length 9 (processing message not counted), got %d", length)
	}

	// Test length for non-existent session
	length = qm.GetQueueLength("non-existent-session")
	if length != 0 {
		t.Errorf("Expected 0 for non-existent session, got %d", length)
	}
}

// TestQueueManagerProcessingOrder tests messages sent in FIFO order
// Verifies that messages are sent to client in the order they were enqueued: 1, 2, 3
func TestQueueManagerProcessingOrder(t *testing.T) {
	ctx, cancel := setupQueueTestContext(t, 15*time.Second)
	defer cancel()

	qm, mockClient := setupQueueTestManager(t)
	defer closeQueueSafely(t, qm)

	sessionID := testSessionID

	// Pause queue to enqueue all messages first
	err := qm.PauseQueue(sessionID)
	assertNoError(t, err)

	// Enqueue 5 messages
	messages := []string{"First", "Second", "Third", "Fourth", "Fifth"}
	for _, msg := range messages {
		_, err := qm.Enqueue(ctx, sessionID, msg)
		assertNoError(t, err)
	}

	// Resume queue to start processing
	err = qm.ResumeQueue(sessionID)
	assertNoError(t, err)

	// Wait for all messages to be sent
	time.Sleep(1 * time.Second)

	// Verify all 5 messages were sent
	sentCount := mockClient.getSentQueryCount()
	if sentCount != 5 {
		t.Errorf("Expected 5 queries sent, got %d", sentCount)
	}

	// Verify messages were sent in FIFO order
	sentQueries := mockClient.getSentQueries()
	if len(sentQueries) != 5 {
		t.Fatalf("Expected 5 sent queries, got %d", len(sentQueries))
	}

	for i, expectedMsg := range messages {
		if sentQueries[i].prompt != expectedMsg {
			t.Errorf("Expected message %d to be '%s', got '%s'", i, expectedMsg, sentQueries[i].prompt)
		}
	}

	// Verify session IDs match
	for i, query := range sentQueries {
		if query.sessionID != sessionID {
			t.Errorf("Expected query %d session ID '%s', got '%s'", i, sessionID, query.sessionID)
		}
	}
}

// TestQueueManagerConcurrentEnqueue tests thread safety of concurrent enqueuing
// Verifies 10 goroutines each enqueueing 5 messages with no race conditions
func TestQueueManagerConcurrentEnqueue(t *testing.T) {
	ctx, cancel := setupQueueTestContext(t, 30*time.Second)
	defer cancel()

	qm, mockClient := setupQueueTestManager(t)
	defer closeQueueSafely(t, qm)

	sessionID := testSessionID

	// Pause queue to prevent processing during enqueuing
	err := qm.PauseQueue(sessionID)
	assertNoError(t, err)

	const numGoroutines = 10
	const messagesPerGoroutine = 5
	const totalMessages = numGoroutines * messagesPerGoroutine

	var wg sync.WaitGroup
	errors := make(chan error, totalMessages)

	// Launch concurrent enqueuers
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()

			for j := 0; j < messagesPerGoroutine; j++ {
				msg := fmt.Sprintf("Goroutine-%d-Message-%d", goroutineID, j)
				_, err := qm.Enqueue(ctx, sessionID, msg)
				if err != nil {
					errors <- fmt.Errorf("goroutine %d message %d: %w", goroutineID, j, err)
				}
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
		t.Errorf("Concurrent enqueue error: %v", err)
	}

	// Verify all 50 messages were enqueued
	queueLen := qm.GetQueueLength(sessionID)
	if queueLen != totalMessages {
		t.Errorf("Expected queue length %d, got %d", totalMessages, queueLen)
	}

	// Resume queue and verify all messages are sent
	err = qm.ResumeQueue(sessionID)
	assertNoError(t, err)

	// Wait for all messages to be processed
	time.Sleep(2 * time.Second)

	// Verify all 50 messages were sent
	sentCount := mockClient.getSentQueryCount()
	if sentCount != totalMessages {
		t.Errorf("Expected %d queries sent, got %d", totalMessages, sentCount)
	}

	// Verify no duplicate message IDs (implies race conditions)
	status, err := qm.GetQueueStatus(sessionID)
	assertNoError(t, err)

	// All messages should be processed by now
	if status.Length != 0 {
		t.Errorf("Expected queue to be empty after processing, got %d pending", status.Length)
	}

	// Verify all sent queries have unique combinations
	sentQueries := mockClient.getSentQueries()
	if len(sentQueries) != totalMessages {
		t.Fatalf("Expected %d sent queries, got %d", totalMessages, len(sentQueries))
	}

	// Check for duplicates
	seenMessages := make(map[string]bool)
	for _, query := range sentQueries {
		if seenMessages[query.prompt] {
			t.Errorf("Duplicate message sent: %s", query.prompt)
		}
		seenMessages[query.prompt] = true
	}
}

// TestQueueManagerPauseResume tests pausing and resuming queue processing
// Verifies processing stops when paused and continues when resumed
func TestQueueManagerPauseResume(t *testing.T) {
	ctx, cancel := setupQueueTestContext(t, 15*time.Second)
	defer cancel()

	qm, mockClient := setupQueueTestManager(t)
	defer closeQueueSafely(t, qm)

	sessionID := testSessionID

	// Start with paused queue
	err := qm.PauseQueue(sessionID)
	assertNoError(t, err)

	// Enqueue messages while paused
	_, err = qm.Enqueue(ctx, sessionID, testMessage1)
	assertNoError(t, err)

	_, err = qm.Enqueue(ctx, sessionID, "Message 2")
	assertNoError(t, err)

	// Wait a bit - no messages should be sent
	time.Sleep(200 * time.Millisecond)

	sentCount := mockClient.getSentQueryCount()
	if sentCount != 0 {
		t.Errorf("Expected 0 queries sent while paused, got %d", sentCount)
	}

	// Resume queue
	err = qm.ResumeQueue(sessionID)
	assertNoError(t, err)

	// Wait for messages to be processed
	time.Sleep(500 * time.Millisecond)

	// Verify messages were sent after resume
	sentCount = mockClient.getSentQueryCount()
	if sentCount != 2 {
		t.Errorf("Expected 2 queries sent after resume, got %d", sentCount)
	}

	// Enqueue more messages
	_, err = qm.Enqueue(ctx, sessionID, "Message 3")
	assertNoError(t, err)

	// Pause again while message 3 is processing
	mockClient.setQueryDelay(500 * time.Millisecond)

	_, err = qm.Enqueue(ctx, sessionID, "Message 4")
	assertNoError(t, err)

	// Wait for msg3 to start processing
	time.Sleep(100 * time.Millisecond)

	err = qm.PauseQueue(sessionID)
	assertNoError(t, err)

	// Wait to ensure msg4 is not sent
	time.Sleep(300 * time.Millisecond)

	sentCount = mockClient.getSentQueryCount()
	if sentCount != 3 {
		t.Errorf("Expected 3 queries sent (msg3 processing, msg4 paused), got %d", sentCount)
	}

	// Resume and verify msg4 is sent
	err = qm.ResumeQueue(sessionID)
	assertNoError(t, err)

	time.Sleep(500 * time.Millisecond)

	sentCount = mockClient.getSentQueryCount()
	if sentCount != 4 {
		t.Errorf("Expected 4 queries sent after final resume, got %d", sentCount)
	}

	// Test multiple pause calls (should be idempotent)
	err = qm.PauseQueue(sessionID)
	assertNoError(t, err)

	err = qm.PauseQueue(sessionID)
	assertNoError(t, err)

	// Test multiple resume calls (should be idempotent)
	err = qm.ResumeQueue(sessionID)
	assertNoError(t, err)

	err = qm.ResumeQueue(sessionID)
	assertNoError(t, err)
}

// TestQueueManagerMultipleSessions tests isolated session queues
// Verifies that different sessions process independently and removal from one doesn't affect others
func TestQueueManagerMultipleSessions(t *testing.T) {
	ctx, cancel := setupQueueTestContext(t, 20*time.Second)
	defer cancel()

	qm, mockClient := setupQueueTestManager(t)
	defer closeQueueSafely(t, qm)

	session1 := "session-1"
	session2 := "session-2"

	// Pause both sessions
	err := qm.PauseQueue(session1)
	assertNoError(t, err)

	err = qm.PauseQueue(session2)
	assertNoError(t, err)

	// Enqueue messages to session-1
	msg1s1, err := qm.Enqueue(ctx, session1, "S1-Message-1")
	assertNoError(t, err)

	_, err = qm.Enqueue(ctx, session1, "S1-Message-2")
	assertNoError(t, err)

	_, err = qm.Enqueue(ctx, session1, "S1-Message-3")
	assertNoError(t, err)

	// Enqueue messages to session-2
	_, err = qm.Enqueue(ctx, session2, "S2-Message-1")
	assertNoError(t, err)

	_, err = qm.Enqueue(ctx, session2, "S2-Message-2")
	assertNoError(t, err)

	// Verify queue lengths are independent
	if qm.GetQueueLength(session1) != 3 {
		t.Errorf("Expected session-1 length 3, got %d", qm.GetQueueLength(session1))
	}

	if qm.GetQueueLength(session2) != 2 {
		t.Errorf("Expected session-2 length 2, got %d", qm.GetQueueLength(session2))
	}

	// Remove a message from session-1
	err = qm.RemoveFromQueue(session1, msg1s1.ID)
	assertNoError(t, err)

	// Verify only session-1 affected
	if qm.GetQueueLength(session1) != 2 {
		t.Errorf("Expected session-1 length 2 after removal, got %d", qm.GetQueueLength(session1))
	}

	if qm.GetQueueLength(session2) != 2 {
		t.Errorf("Expected session-2 length still 2, got %d", qm.GetQueueLength(session2))
	}

	// Resume only session-1
	err = qm.ResumeQueue(session1)
	assertNoError(t, err)

	// Wait for session-1 to process
	time.Sleep(500 * time.Millisecond)

	// Verify session-1 messages sent, session-2 not sent
	sentQueries := mockClient.getSentQueries()

	session1Count := 0
	session2Count := 0

	for _, query := range sentQueries {
		if query.sessionID == session1 {
			session1Count++
		}
		if query.sessionID == session2 {
			session2Count++
		}
	}

	if session1Count != 2 {
		t.Errorf("Expected 2 messages from session-1, got %d", session1Count)
	}

	if session2Count != 0 {
		t.Errorf("Expected 0 messages from session-2 (still paused), got %d", session2Count)
	}

	// Resume session-2
	err = qm.ResumeQueue(session2)
	assertNoError(t, err)

	// Wait for session-2 to process
	time.Sleep(500 * time.Millisecond)

	// Verify session-2 messages now sent
	sentQueries = mockClient.getSentQueries()

	session2Count = 0
	for _, query := range sentQueries {
		if query.sessionID == session2 {
			session2Count++
		}
	}

	if session2Count != 2 {
		t.Errorf("Expected 2 messages from session-2, got %d", session2Count)
	}

	// Verify total messages sent
	if mockClient.getSentQueryCount() != 4 {
		t.Errorf("Expected 4 total queries sent, got %d", mockClient.getSentQueryCount())
	}

	// Test clearing one session doesn't affect other
	_, err = qm.Enqueue(ctx, session1, "S1-Message-4")
	assertNoError(t, err)

	_, err = qm.Enqueue(ctx, session2, "S2-Message-3")
	assertNoError(t, err)

	err = qm.PauseQueue(session1)
	assertNoError(t, err)

	err = qm.PauseQueue(session2)
	assertNoError(t, err)

	err = qm.ClearQueue(session1)
	assertNoError(t, err)

	// Verify session-1 cleared, session-2 unchanged
	if qm.GetQueueLength(session1) != 0 {
		t.Errorf("Expected session-1 cleared, got length %d", qm.GetQueueLength(session1))
	}

	if qm.GetQueueLength(session2) != 1 {
		t.Errorf("Expected session-2 length still 1, got %d", qm.GetQueueLength(session2))
	}
}

// TestQueueManagerClose tests cleanup and shutdown
// Verifies processing goroutines stop and pending messages are handled correctly
func TestQueueManagerClose(t *testing.T) {
	ctx, cancel := setupQueueTestContext(t, 10*time.Second)
	defer cancel()

	qm, mockClient := setupQueueTestManager(t)

	sessionID := testSessionID

	// Enqueue messages
	_, err := qm.Enqueue(ctx, sessionID, testMessage1)
	assertNoError(t, err)

	_, err = qm.Enqueue(ctx, sessionID, "Message 2")
	assertNoError(t, err)

	_, err = qm.Enqueue(ctx, sessionID, "Message 3")
	assertNoError(t, err)

	// Close queue manager
	err = qm.Close()
	assertNoError(t, err)

	// Wait a bit for cleanup
	time.Sleep(200 * time.Millisecond)

	// Verify enqueuing after close returns error
	_, err = qm.Enqueue(ctx, sessionID, "Message 4")
	if err == nil {
		t.Error("Expected error when enqueueing after close")
	}

	// Multiple close calls should be safe
	err = qm.Close()
	if err != nil {
		t.Errorf("Expected second Close() to be safe, got error: %v", err)
	}

	// Verify goroutines stopped (can't test directly, but operations should fail)
	err = qm.PauseQueue(sessionID)
	if err == nil {
		t.Error("Expected error when pausing after close")
	}

	// Document behavior: pending messages may be lost or processed
	// This is implementation-specific and should be documented
	sentCount := mockClient.getSentQueryCount()
	t.Logf("Messages sent before/during close: %d (implementation-specific)", sentCount)

	// The test passes regardless of whether messages were sent
	// This documents that behavior is not strictly defined
}

// ===== MOCK IMPLEMENTATIONS (SUPPORTING TYPES) =====

// mockClientForQueue is a thread-safe mock Client implementation for queue testing
// It tracks QueryWithSessionAsync calls including order, sessionID, and prompt
type mockClientForQueue struct {
	mu sync.Mutex

	// Query tracking
	sentQueries []sentQuery
	queryDelay  time.Duration

	// Mock QueryHandle to return
	mockHandles []*mockQueryHandleForQueue

	// Interface implementation stubs
	connected bool
}

// sentQuery tracks a sent query with all relevant details
type sentQuery struct {
	sessionID string
	prompt    string
	timestamp time.Time
}

// mockQueryHandleForQueue is a simple mock QueryHandle for queue testing
type mockQueryHandleForQueue struct {
	id        string
	sessionID string
	status    QueryStatus
	messages  chan Message
	errors    chan error
	done      chan struct{}
	ctx       context.Context
	cancel    context.CancelFunc
}

// newMockClientForQueue creates a new mock client for queue testing
func newMockClientForQueue() *mockClientForQueue {
	return &mockClientForQueue{
		sentQueries: make([]sentQuery, 0),
		mockHandles: make([]*mockQueryHandleForQueue, 0),
	}
}

// QueryWithSessionAsync implements the Client interface for queue testing
// Records the call and returns a mock QueryHandle
func (m *mockClientForQueue) QueryWithSessionAsync(ctx context.Context, prompt string, sessionID string) (QueryHandle, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Record the query
	m.sentQueries = append(m.sentQueries, sentQuery{
		sessionID: sessionID,
		prompt:    prompt,
		timestamp: time.Now(),
	})

	// Simulate delay if configured
	if m.queryDelay > 0 {
		time.Sleep(m.queryDelay)
	}

	// Create and return a mock handle
	ctx, cancel := context.WithCancel(ctx)
	handle := &mockQueryHandleForQueue{
		id:        fmt.Sprintf("mock-query-%d", len(m.sentQueries)),
		sessionID: sessionID,
		status:    QueryStatusProcessing,
		messages:  make(chan Message, 10),
		errors:    make(chan error, 1),
		done:      make(chan struct{}),
		ctx:       ctx,
		cancel:    cancel,
	}

	m.mockHandles = append(m.mockHandles, handle)

	// Simulate successful completion
	go func() {
		time.Sleep(10 * time.Millisecond)
		handle.status = QueryStatusCompleted
		close(handle.messages)
		close(handle.errors)
		close(handle.done)
	}()

	return handle, nil
}

// getSentQueryCount returns the number of queries sent (thread-safe)
func (m *mockClientForQueue) getSentQueryCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sentQueries)
}

// getSentQueries returns a copy of sent queries (thread-safe)
func (m *mockClientForQueue) getSentQueries() []sentQuery {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Return a copy to avoid race conditions
	queriesCopy := make([]sentQuery, len(m.sentQueries))
	copy(queriesCopy, m.sentQueries)
	return queriesCopy
}

// setQueryDelay configures a delay to simulate slow processing (thread-safe)
func (m *mockClientForQueue) setQueryDelay(delay time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.queryDelay = delay
}

// Mock QueryHandle interface implementations

func (m *mockQueryHandleForQueue) ID() string {
	return m.id
}

func (m *mockQueryHandleForQueue) SessionID() string {
	return m.sessionID
}

func (m *mockQueryHandleForQueue) Status() QueryStatus {
	return m.status
}

func (m *mockQueryHandleForQueue) Messages() <-chan Message {
	return m.messages
}

func (m *mockQueryHandleForQueue) Errors() <-chan error {
	return m.errors
}

func (m *mockQueryHandleForQueue) Wait() error {
	<-m.done
	return nil
}

func (m *mockQueryHandleForQueue) Cancel() {
	m.cancel()
	m.status = QueryStatusCancelled
}

func (m *mockQueryHandleForQueue) Done() <-chan struct{} {
	return m.done
}

// Stub implementations of Client interface methods (not used in queue testing)

func (m *mockClientForQueue) Connect(_ context.Context, _ ...StreamMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connected = true
	return nil
}

func (m *mockClientForQueue) Disconnect() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connected = false
	return nil
}

func (m *mockClientForQueue) Query(_ context.Context, _ string) error {
	return nil
}

func (m *mockClientForQueue) QueryWithSession(_ context.Context, _ string, _ string) error {
	return nil
}

func (m *mockClientForQueue) QueryAsync(ctx context.Context, prompt string) (QueryHandle, error) {
	return m.QueryWithSessionAsync(ctx, prompt, defaultSessionID)
}

func (m *mockClientForQueue) QueryStream(_ context.Context, _ <-chan StreamMessage) error {
	return nil
}

func (m *mockClientForQueue) ReceiveMessages(_ context.Context) <-chan Message {
	return nil
}

func (m *mockClientForQueue) ReceiveResponse(_ context.Context) MessageIterator {
	return nil
}

func (m *mockClientForQueue) Interrupt(_ context.Context) error {
	return nil
}

func (m *mockClientForQueue) GetStreamIssues() []StreamIssue {
	return nil
}

func (m *mockClientForQueue) GetStreamStats() StreamStats {
	return StreamStats{}
}

func (m *mockClientForQueue) SetModel(_ context.Context, _ *string) error {
	return nil
}

func (m *mockClientForQueue) SetPermissionMode(_ context.Context, _ PermissionMode) error {
	return nil
}

func (m *mockClientForQueue) RewindFiles(_ context.Context, _ string) error {
	return nil
}

func (m *mockClientForQueue) GetServerInfo(_ context.Context) (map[string]interface{}, error) {
	return nil, nil
}

// ===== HELPER FUNCTIONS (UTILITIES) =====

// setupQueueTestContext creates a context with timeout for queue tests
func setupQueueTestContext(t *testing.T, timeout time.Duration) (context.Context, context.CancelFunc) {
	t.Helper()
	return context.WithTimeout(context.Background(), timeout)
}

// setupQueueTestManager creates a QueueManager with mock client for testing
func setupQueueTestManager(t *testing.T) (*QueueManager, *mockClientForQueue) {
	t.Helper()
	mockClient := newMockClientForQueue()

	// Note: This will fail until QueueManager is implemented
	// This is expected in TDD RED phase
	qm := NewQueueManager(mockClient)

	return qm, mockClient
}

// closeQueueSafely closes the queue manager and reports any errors
func closeQueueSafely(t *testing.T, qm *QueueManager) {
	t.Helper()
	if qm != nil {
		if err := qm.Close(); err != nil {
			t.Errorf("Failed to close queue manager: %v", err)
		}
	}
}
