package claudecode

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// QueueManager manages message queues for multiple sessions.
// Messages stay in SDK-side queue until processed.
// Supports removal, reordering, and priority before sending.
type QueueManager struct {
	client Client
	queues map[string]*MessageQueue
	mu     sync.RWMutex
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// MessageQueue represents a queue for a specific session.
type MessageQueue struct {
	sessionID  string
	messages   []*QueuedMessage
	processing *QueuedMessage
	mu         sync.RWMutex
	paused     bool
	resumeChan chan struct{}
}

// QueuedMessage represents a message in the queue.
type QueuedMessage struct {
	ID        string
	Content   string
	Priority  int
	Timestamp time.Time
	Handle    QueryHandle
	Status    MessageStatus
}

// MessageStatus represents the state of a queued message.
type MessageStatus int

const (
	// MessageStatusQueued indicates the message is queued but not yet processing
	MessageStatusQueued MessageStatus = iota
	// MessageStatusProcessing indicates the message is actively being processed
	MessageStatusProcessing
	// MessageStatusCompleted indicates the message completed successfully
	MessageStatusCompleted
	// MessageStatusFailed indicates the message failed with an error
	MessageStatusFailed
	// MessageStatusCancelled indicates the message was cancelled
	MessageStatusCancelled
	// MessageStatusRemoved indicates the message was removed from queue
	MessageStatusRemoved
)

// String returns the string representation of the message status.
func (ms MessageStatus) String() string {
	switch ms {
	case MessageStatusQueued:
		return "queued"
	case MessageStatusProcessing:
		return "processing"
	case MessageStatusCompleted:
		return "completed"
	case MessageStatusFailed:
		return "failed"
	case MessageStatusCancelled:
		return "cancelled"
	case MessageStatusRemoved:
		return "removed"
	default:
		return statusUnknown
	}
}

// QueueStatus represents the current state of a queue.
type QueueStatus struct {
	Length          int
	Processing      *QueuedMessage
	PendingMessages []*QueuedMessage
}

// NewQueueManager creates a new queue manager for the specified client.
func NewQueueManager(client Client) *QueueManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &QueueManager{
		client: client,
		queues: make(map[string]*MessageQueue),
		ctx:    ctx,
		cancel: cancel,
	}
}

// Enqueue adds a message to the queue for the specified session.
// Messages stay in queue until processor picks them up (not sent immediately).
func (qm *QueueManager) Enqueue(_ context.Context, sessionID, message string) (*QueuedMessage, error) {
	// Check if QueueManager is closed
	select {
	case <-qm.ctx.Done():
		return nil, fmt.Errorf("queue manager is closed")
	default:
	}

	// Get or create queue for this session
	queue := qm.getOrCreateQueue(sessionID)

	// Create QueuedMessage
	queuedMsg := &QueuedMessage{
		ID:        generateMessageID(),
		Content:   message,
		Timestamp: time.Now(),
		Status:    MessageStatusQueued,
	}

	// Add to queue
	queue.mu.Lock()
	queue.messages = append(queue.messages, queuedMsg)
	queue.mu.Unlock()

	return queuedMsg, nil
}

// RemoveFromQueue removes a message from the queue before it's sent.
// Returns error if message is already processing or not found.
func (qm *QueueManager) RemoveFromQueue(sessionID, messageID string) error {
	// Get queue
	qm.mu.RLock()
	queue, exists := qm.queues[sessionID]
	qm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	queue.mu.Lock()
	defer queue.mu.Unlock()

	// Check if message is currently processing
	if queue.processing != nil && queue.processing.ID == messageID {
		return fmt.Errorf("cannot remove message that is already processing")
	}

	// Find and remove message from queue
	for i, msg := range queue.messages {
		if msg.ID == messageID {
			// Remove from slice
			queue.messages = append(queue.messages[:i], queue.messages[i+1:]...)
			// Update status
			msg.Status = MessageStatusRemoved
			return nil
		}
	}

	return fmt.Errorf("message not found: %s", messageID)
}

// ClearQueue removes all pending messages from the queue.
// Currently processing message is NOT cancelled.
func (qm *QueueManager) ClearQueue(sessionID string) error {
	// Get queue
	qm.mu.RLock()
	queue, exists := qm.queues[sessionID]
	qm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	queue.mu.Lock()
	defer queue.mu.Unlock()

	// Clear pending messages (keep processing message)
	queue.messages = make([]*QueuedMessage, 0)

	return nil
}

// GetQueueStatus returns the current status of the queue.
func (qm *QueueManager) GetQueueStatus(sessionID string) (*QueueStatus, error) {
	// Get queue
	qm.mu.RLock()
	queue, exists := qm.queues[sessionID]
	qm.mu.RUnlock()

	if !exists {
		// Return empty status for non-existent queue
		return &QueueStatus{
			Length:          0,
			Processing:      nil,
			PendingMessages: make([]*QueuedMessage, 0),
		}, nil
	}

	queue.mu.RLock()
	defer queue.mu.RUnlock()

	// Build status
	status := &QueueStatus{
		Length:          len(queue.messages),
		Processing:      queue.processing,
		PendingMessages: make([]*QueuedMessage, len(queue.messages)),
	}

	// Copy pending messages
	copy(status.PendingMessages, queue.messages)

	return status, nil
}

// GetQueueLength returns the number of pending messages in the queue.
// Processing message is NOT included in count.
func (qm *QueueManager) GetQueueLength(sessionID string) int {
	// Get queue
	qm.mu.RLock()
	queue, exists := qm.queues[sessionID]
	qm.mu.RUnlock()

	if !exists {
		return 0
	}

	queue.mu.RLock()
	defer queue.mu.RUnlock()

	return len(queue.messages)
}

// PauseQueue temporarily stops processing messages from the queue.
// Currently processing message will complete, but no new messages will be processed.
func (qm *QueueManager) PauseQueue(sessionID string) error {
	// Check if QueueManager is closed
	select {
	case <-qm.ctx.Done():
		return fmt.Errorf("queue manager is closed")
	default:
	}

	// Get or create queue (to allow pausing before any messages)
	queue := qm.getOrCreateQueue(sessionID)

	queue.mu.Lock()
	defer queue.mu.Unlock()

	queue.paused = true

	return nil
}

// ResumeQueue resumes processing messages from the queue.
func (qm *QueueManager) ResumeQueue(sessionID string) error {
	// Get queue
	qm.mu.RLock()
	queue, exists := qm.queues[sessionID]
	qm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	queue.mu.Lock()
	wasPaused := queue.paused
	queue.paused = false
	queue.mu.Unlock()

	// Signal resume if was paused
	if wasPaused {
		select {
		case queue.resumeChan <- struct{}{}:
		default:
			// Channel already has signal or processor not waiting
		}
	}

	return nil
}

// ReorderQueue changes the order of messages in the queue.
func (qm *QueueManager) ReorderQueue(sessionID string, messageIDs []string) error {
	// Get queue
	qm.mu.RLock()
	queue, exists := qm.queues[sessionID]
	qm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	queue.mu.Lock()
	defer queue.mu.Unlock()

	// Build map of existing messages
	msgMap := make(map[string]*QueuedMessage)
	for _, msg := range queue.messages {
		msgMap[msg.ID] = msg
	}

	// Rebuild queue in new order
	newQueue := make([]*QueuedMessage, 0, len(messageIDs))
	for _, id := range messageIDs {
		if msg, exists := msgMap[id]; exists {
			newQueue = append(newQueue, msg)
			delete(msgMap, id) // Remove from map to track remaining
		}
	}

	// Add any remaining messages not in reorder list
	for _, msg := range queue.messages {
		if _, exists := msgMap[msg.ID]; exists {
			newQueue = append(newQueue, msg)
		}
	}

	queue.messages = newQueue

	return nil
}

// SetPriority sets the priority of a message in the queue.
func (qm *QueueManager) SetPriority(sessionID, messageID string, priority int) error {
	// Get queue
	qm.mu.RLock()
	queue, exists := qm.queues[sessionID]
	qm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	queue.mu.Lock()
	defer queue.mu.Unlock()

	// Find message and set priority
	for _, msg := range queue.messages {
		if msg.ID == messageID {
			msg.Priority = priority
			return nil
		}
	}

	return fmt.Errorf("message not found: %s", messageID)
}

// Close shuts down the queue manager and stops all processing.
func (qm *QueueManager) Close() error {
	// Cancel context to signal all processors to stop
	qm.cancel()

	// Wait for all processors to finish (with timeout)
	done := make(chan struct{})
	go func() {
		qm.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// All processors stopped
	case <-time.After(5 * time.Second):
		// Timeout - processors still running
		// This is acceptable, they'll be garbage collected
	}

	return nil
}

// getOrCreateQueue retrieves or creates a queue for the specified session.
// This method is thread-safe.
func (qm *QueueManager) getOrCreateQueue(sessionID string) *MessageQueue {
	// Fast path: check with read lock
	qm.mu.RLock()
	queue, exists := qm.queues[sessionID]
	qm.mu.RUnlock()

	if exists {
		return queue
	}

	// Slow path: create with write lock
	qm.mu.Lock()
	defer qm.mu.Unlock()

	// Double-check after acquiring write lock
	queue, exists = qm.queues[sessionID]
	if exists {
		return queue
	}

	// Create new queue - start unpaused but with delay in processor
	queue = &MessageQueue{
		sessionID:  sessionID,
		messages:   make([]*QueuedMessage, 0),
		processing: nil,
		paused:     false,
		resumeChan: make(chan struct{}, 1), // Buffered to prevent blocking
	}

	qm.queues[sessionID] = queue

	// Start processor goroutine
	qm.wg.Add(1)
	go qm.processQueue(queue)

	return queue
}

// processQueue processes messages from the queue (internal goroutine).
// Runs continuously until QueueManager is closed.
func (qm *QueueManager) processQueue(queue *MessageQueue) {
	defer qm.wg.Done()

	// Initial delay to allow multiple messages to be enqueued before processing starts
	// This prevents immediate processing when queue is first created
	select {
	case <-qm.ctx.Done():
		return
	case <-time.After(100 * time.Millisecond):
		// Continue to processing
	}

	for {
		// Check if QueueManager is shutting down
		select {
		case <-qm.ctx.Done():
			return
		default:
		}

		// Check if paused
		queue.mu.RLock()
		paused := queue.paused
		queue.mu.RUnlock()

		if paused {
			// Wait for resume signal or shutdown
			select {
			case <-queue.resumeChan:
				// Resume processing
				continue
			case <-qm.ctx.Done():
				return
			case <-time.After(100 * time.Millisecond):
				// Periodic check for shutdown
				continue
			}
		}

		// Get next message from queue
		queue.mu.Lock()
		if len(queue.messages) == 0 {
			queue.mu.Unlock()
			// No messages, sleep and retry
			select {
			case <-qm.ctx.Done():
				return
			case <-time.After(100 * time.Millisecond):
				continue
			}
		}

		// Pop first message (FIFO)
		msg := queue.messages[0]
		queue.messages = queue.messages[1:]
		queue.processing = msg
		queue.mu.Unlock()

		// Update message status
		msg.Status = MessageStatusProcessing

		// Send message via client's async API
		handle, err := qm.client.QueryWithSessionAsync(qm.ctx, msg.Content, queue.sessionID)
		if err != nil {
			// Failed to send
			msg.Status = MessageStatusFailed
			queue.mu.Lock()
			queue.processing = nil
			queue.mu.Unlock()
			continue
		}

		// Store handle
		msg.Handle = handle

		// Wait for completion
		err = handle.Wait()

		// Update status based on result
		if err != nil {
			msg.Status = MessageStatusFailed
		} else {
			msg.Status = MessageStatusCompleted
		}

		// Clear processing message
		queue.mu.Lock()
		queue.processing = nil
		queue.mu.Unlock()
	}
}

// generateMessageID creates a unique message ID using cryptographically secure random bytes.
// Falls back to timestamp-based ID if crypto/rand fails.
func generateMessageID() string {
	// Use 16 bytes of random data for 32-character hex string
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to timestamp if crypto/rand fails
		return fmt.Sprintf("msg_%d", time.Now().UnixNano())
	}
	return "msg_" + hex.EncodeToString(bytes)
}
