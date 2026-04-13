package claudecode

import (
	"context"
	"fmt"
)

// QueryAsync submits a query without blocking, using default session.
// Returns a handle immediately. Message is sent to CLI subprocess right away.
//
// Example:
//
//	handle, err := client.QueryAsync(ctx, "What is 2+2?")
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	for msg := range handle.Messages() {
//	    fmt.Println("Response:", msg)
//	}
func (c *ClientImpl) QueryAsync(ctx context.Context, prompt string) (QueryHandle, error) {
	return c.QueryWithSessionAsync(ctx, prompt, defaultSessionID)
}

// QueryWithSessionAsync submits a query without blocking to a specific session.
// Each session maintains its own conversation context.
// Returns a handle immediately. Message is sent to CLI subprocess right away.
//
// Example:
//
//	handle, err := client.QueryWithSessionAsync(ctx, "Remember this", "my-session")
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Cancel if needed
//	go func() {
//	    time.Sleep(5 * time.Second)
//	    handle.Cancel()
//	}()
func (c *ClientImpl) QueryWithSessionAsync(ctx context.Context, prompt string, sessionID string) (QueryHandle, error) {
	// Check context before proceeding
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Use default session if empty
	if sessionID == "" {
		sessionID = defaultSessionID
	}

	// Check connection state with read lock
	c.mu.RLock()
	connected := c.connected
	transport := c.transport
	c.mu.RUnlock()

	if !connected || transport == nil {
		return nil, fmt.Errorf("client not connected")
	}

	// Create query handle
	handle := newQueryHandle(ctx, sessionID)

	// Track query before starting execution
	c.trackQuery(handle)

	// Start goroutine for async execution - return immediately
	go c.executeAsyncQuery(handle, prompt, sessionID, transport)

	return handle, nil
}

// executeAsyncQuery runs in a goroutine to execute the query asynchronously.
// This follows the same pattern as the synchronous queryWithSession but streams
// results to the handle's channels instead of blocking the caller.
func (c *ClientImpl) executeAsyncQuery(handle *queryHandle, prompt string, sessionID string, transport Transport) {
	// Always untrack query when goroutine exits
	defer c.untrackQuery(handle.ID())

	// Update status to processing
	handle.setStatus(QueryStatusProcessing)

	// Create user message in Python SDK compatible format
	streamMsg := StreamMessage{
		Type: "user",
		Message: map[string]interface{}{
			"role":    "user",
			"content": prompt,
		},
		ParentToolUseID: nil,
		SessionID:       sessionID,
	}

	// Send message via transport
	if err := transport.SendMessage(handle.ctx, streamMsg); err != nil {
		handle.complete(err)
		return
	}

	// Get message and error channels with read lock
	c.mu.RLock()
	msgChan := c.msgChan
	errChan := c.errChan
	c.mu.RUnlock()

	// Stream responses to handle channels
	for {
		select {
		case msg, ok := <-msgChan:
			if !ok {
				// Channel closed - complete successfully
				handle.complete(nil)
				return
			}

			// Forward message to handle (non-blocking send)
			select {
			case handle.messages <- msg:
				// Message sent successfully
			case <-handle.ctx.Done():
				// Context cancelled while trying to send
				handle.complete(handle.ctx.Err())
				return
			}

			// Check if this is a done message
			if isDoneMessage(msg) {
				handle.complete(nil)
				return
			}

		case err, ok := <-errChan:
			if !ok {
				// Error channel closed - complete successfully
				handle.complete(nil)
				return
			}

			// Send error to handle's error channel (non-blocking)
			select {
			case handle.errors <- err:
				// Error sent successfully
			case <-handle.ctx.Done():
				// Context cancelled while trying to send error
			}

			// Complete with error
			handle.complete(err)
			return

		case <-handle.ctx.Done():
			// Query cancelled
			handle.complete(handle.ctx.Err())
			return
		}
	}
}

// trackQuery adds a query to the active queries map.
// This enables query cleanup on disconnect and debugging.
func (c *ClientImpl) trackQuery(handle *queryHandle) {
	c.queriesMu.Lock()
	defer c.queriesMu.Unlock()

	if c.activeQueries == nil {
		c.activeQueries = make(map[string]*queryHandle)
	}

	c.activeQueries[handle.ID()] = handle
}

// untrackQuery removes a query from the active queries map.
// Called when a query completes, fails, or is cancelled.
func (c *ClientImpl) untrackQuery(queryID string) {
	c.queriesMu.Lock()
	defer c.queriesMu.Unlock()
	delete(c.activeQueries, queryID)
}
