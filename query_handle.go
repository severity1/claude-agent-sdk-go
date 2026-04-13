package claudecode

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// QueryHandle represents a non-blocking query execution.
// It provides channels for receiving messages and errors, as well as methods
// for monitoring status and controlling execution (cancellation).
type QueryHandle interface {
	// ID returns the unique identifier for this query
	ID() string

	// SessionID returns the session ID for this query
	SessionID() string

	// Status returns the current status of the query
	Status() QueryStatus

	// Messages returns a channel for receiving response messages
	Messages() <-chan Message

	// Errors returns a channel for receiving errors
	Errors() <-chan error

	// Wait blocks until the query completes or fails
	Wait() error

	// Cancel cancels the query execution
	Cancel()

	// Done returns a channel that closes when the query completes
	Done() <-chan struct{}
}

// QueryStatus represents the state of an async query.
type QueryStatus int

const (
	// QueryStatusQueued indicates the query is queued but not yet processing
	QueryStatusQueued QueryStatus = iota
	// QueryStatusProcessing indicates the query is actively being processed
	QueryStatusProcessing
	// QueryStatusCompleted indicates the query completed successfully
	QueryStatusCompleted
	// QueryStatusFailed indicates the query failed with an error
	QueryStatusFailed
	// QueryStatusCancelled indicates the query was cancelled
	QueryStatusCancelled
)

// statusUnknown is the string representation for unknown status values.
const statusUnknown = "unknown"

// String returns the string representation of the query status.
func (qs QueryStatus) String() string {
	switch qs {
	case QueryStatusQueued:
		return "queued"
	case QueryStatusProcessing:
		return "processing"
	case QueryStatusCompleted:
		return "completed"
	case QueryStatusFailed:
		return "failed"
	case QueryStatusCancelled:
		return "cancelled"
	default:
		return statusUnknown
	}
}

// queryHandle implements QueryHandle interface with thread-safe operations.
type queryHandle struct {
	id        string
	sessionID string

	// Status tracking
	statusMu sync.RWMutex
	status   QueryStatus

	// Communication channels (buffered for non-blocking sends)
	messages chan Message
	errors   chan error
	done     chan struct{}

	// Cancellation support
	ctx       context.Context
	cancel    context.CancelFunc
	cancelMu  sync.Mutex
	cancelled bool

	// Completion tracking (ensures Wait() is idempotent)
	waitOnce sync.Once
	waitErr  error
	waitDone chan struct{}
}

// newQueryHandle creates a new query handle with the specified parent context and session ID.
// The handle is initialized with buffered channels and a cancelable context.
func newQueryHandle(parentCtx context.Context, sessionID string) *queryHandle {
	ctx, cancel := context.WithCancel(parentCtx)

	// Use default session if empty
	if sessionID == "" {
		sessionID = defaultSessionID
	}

	return &queryHandle{
		id:        generateQueryID(),
		sessionID: sessionID,
		status:    QueryStatusQueued,
		messages:  make(chan Message, 100), // Buffer size per spec
		errors:    make(chan error, 1),     // Only first error matters
		done:      make(chan struct{}),     // Unbuffered signaling channel
		ctx:       ctx,
		cancel:    cancel,
		waitDone:  make(chan struct{}),
	}
}

// ID returns the unique identifier for this query.
func (qh *queryHandle) ID() string {
	return qh.id
}

// SessionID returns the session ID for this query.
func (qh *queryHandle) SessionID() string {
	return qh.sessionID
}

// Status returns the current status of the query.
// This method is thread-safe.
func (qh *queryHandle) Status() QueryStatus {
	qh.statusMu.RLock()
	defer qh.statusMu.RUnlock()
	return qh.status
}

// Messages returns a read-only channel for receiving response messages.
func (qh *queryHandle) Messages() <-chan Message {
	return qh.messages
}

// Errors returns a read-only channel for receiving errors.
func (qh *queryHandle) Errors() <-chan error {
	return qh.errors
}

// Done returns a channel that closes when the query completes.
func (qh *queryHandle) Done() <-chan struct{} {
	return qh.done
}

// Wait blocks until the query completes or fails.
// Multiple calls to Wait return the same error.
// This method is idempotent and thread-safe.
func (qh *queryHandle) Wait() error {
	qh.waitOnce.Do(func() {
		// Wait for completion
		<-qh.waitDone
	})
	return qh.waitErr
}

// Cancel cancels the query execution.
// Multiple calls to Cancel are safe and idempotent.
func (qh *queryHandle) Cancel() {
	qh.cancelMu.Lock()
	defer qh.cancelMu.Unlock()

	if !qh.cancelled {
		qh.cancelled = true
		qh.cancel()
	}
}

// setStatus updates the query status in a thread-safe manner.
func (qh *queryHandle) setStatus(status QueryStatus) {
	qh.statusMu.Lock()
	defer qh.statusMu.Unlock()
	qh.status = status
}

// complete marks the query as finished and performs cleanup.
// This should only be called once by the query execution goroutine.
func (qh *queryHandle) complete(err error) {
	// Determine final status based on error type
	if err != nil {
		if qh.ctx.Err() == context.Canceled {
			qh.setStatus(QueryStatusCancelled)
		} else {
			qh.setStatus(QueryStatusFailed)
		}
	} else {
		qh.setStatus(QueryStatusCompleted)
	}

	// Store error for Wait()
	qh.waitErr = err

	// Close all channels
	close(qh.messages)
	close(qh.errors)
	close(qh.done)
	close(qh.waitDone)
}

// generateQueryID creates a unique query ID using cryptographically secure random bytes.
// Falls back to timestamp-based ID if crypto/rand fails.
func generateQueryID() string {
	// Use 16 bytes of random data for 32-character hex string
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to timestamp if crypto/rand fails
		return fmt.Sprintf("query_%d", time.Now().UnixNano())
	}
	return "query_" + hex.EncodeToString(bytes)
}

// isDoneMessage checks if a message indicates query completion.
// Returns true for ResultMessage which signals the end of a query.
func isDoneMessage(msg Message) bool {
	_, ok := msg.(*ResultMessage)
	return ok
}
