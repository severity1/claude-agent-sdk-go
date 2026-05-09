package claudecode

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"

	"github.com/severity1/claude-agent-sdk-go/internal/cli"
	"github.com/severity1/claude-agent-sdk-go/internal/subprocess"
)

const defaultSessionID = "default"

// Client provides bidirectional streaming communication with Claude Code CLI.
type Client interface {
	Connect(ctx context.Context, prompt ...StreamMessage) error
	Disconnect() error
	Query(ctx context.Context, prompt string) error
	QueryWithSession(ctx context.Context, prompt string, sessionID string) error
	QueryStream(ctx context.Context, messages <-chan StreamMessage) error
	ReceiveMessages(ctx context.Context) <-chan Message
	ReceiveResponse(ctx context.Context) MessageIterator
	Interrupt(ctx context.Context) error
	// SetModel changes the AI model during a streaming session.
	// Pass nil to reset to the default model.
	// Only works in streaming mode (after Connect()).
	SetModel(ctx context.Context, model *string) error
	// SetPermissionMode changes the permission mode during a streaming session.
	// Valid modes: PermissionModeDefault, PermissionModeAcceptEdits,
	// PermissionModePlan, PermissionModeBypassPermissions.
	// Only works in streaming mode (after Connect()).
	SetPermissionMode(ctx context.Context, mode PermissionMode) error
	// RewindFiles reverts tracked files to their state at a specific user message.
	// The messageUUID should be the UUID from a UserMessage received during the session.
	// Requires WithFileCheckpointing() or WithEnableFileCheckpointing(true) option.
	// Only works in streaming mode (after Connect()).
	RewindFiles(ctx context.Context, messageUUID string) error
	// GetMcpStatus returns the current status of all connected MCP servers.
	// Only works in streaming mode (after Connect()).
	GetMcpStatus(ctx context.Context) (*McpStatusResponse, error)
	GetStreamIssues() []StreamIssue
	GetStreamStats() StreamStats
	GetServerInfo(ctx context.Context) (map[string]interface{}, error)
}

// clientImpl implements the Client interface.
type clientImpl struct {
	mu              sync.RWMutex
	transport       Transport
	customTransport Transport // For testing with WithTransport
	options         *Options
	connected       bool
	msgChan         <-chan Message
	errChan         <-chan error
	// streamErrChan forwards non-transport-origin errors (e.g. QueryStream
	// send failures from the background goroutine) into the receive path so
	// callers observing Receive* can act on them instead of silently losing
	// messages when the outbound pipe breaks.
	streamErrChan chan error
}

// NewClient creates a new Client with the given options.
func NewClient(opts ...Option) Client {
	options := NewOptions(opts...)
	client := &clientImpl{
		options: options,
	}
	return client
}

// NewClientWithTransport creates a new Client that uses the supplied Transport
// instead of spawning the Claude CLI subprocess. Intended for tests and
// integration scaffolding where the CLI must be mocked or replaced; production
// callers should use [NewClient] or [WithClient].
func NewClientWithTransport(transport Transport, opts ...Option) Client {
	options := NewOptions(opts...)
	return &clientImpl{
		customTransport: transport,
		options:         options,
	}
}

// WithClient provides Go-idiomatic resource management equivalent to Python SDK's async context manager.
// It automatically connects to Claude Code CLI, executes the provided function, and ensures proper cleanup.
// This eliminates the need for manual Connect/Disconnect calls and prevents resource leaks.
//
// The function follows Go's established resource management patterns using defer for guaranteed cleanup,
// similar to how database connections, files, and other resources are typically managed in Go.
//
// Example - Basic usage:
//
//	err := claudecode.WithClient(ctx, func(client claudecode.Client) error {
//	    return client.Query(ctx, "What is 2+2?")
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//
// Example - With configuration options:
//
//	err := claudecode.WithClient(ctx, func(client claudecode.Client) error {
//	    if err := client.Query(ctx, "Calculate the area of a circle with radius 5"); err != nil {
//	        return err
//	    }
//
//	    // Process responses
//	    for msg := range client.ReceiveMessages(ctx) {
//	        if assistantMsg, ok := msg.(*claudecode.AssistantMessage); ok {
//	            fmt.Println("Claude:", assistantMsg.Content[0].(*claudecode.TextBlock).Text)
//	        }
//	    }
//	    return nil
//	}, claudecode.WithSystemPrompt("You are a helpful math tutor"),
//	   claudecode.WithAllowedTools("Read", "Write"))
//
// The client will be automatically connected before fn is called and disconnected after fn returns,
// even if fn returns an error or panics. This provides 100% functional parity with Python SDK's
// 'async with ClaudeSDKClient()' pattern while using idiomatic Go resource management.
//
// Parameters:
//   - ctx: Context for connection management and cancellation
//   - fn: Function to execute with the connected client
//   - opts: Optional client configuration options
//
// Returns an error if connection fails or if fn returns an error.
// Disconnect errors are handled gracefully without overriding the original error from fn.
func WithClient(ctx context.Context, fn func(Client) error, opts ...Option) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	client := NewClient(opts...)

	if err := client.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect client: %w", err)
	}

	defer func() {
		// Following Go idiom: cleanup errors don't override the original error
		// from fn, but log them so crash diagnostics surface in the caller's
		// logs instead of vanishing.
		if disconnectErr := client.Disconnect(); disconnectErr != nil {
			log.Printf("claude-sdk: WithClient disconnect error: %v", disconnectErr)
		}
	}()

	return fn(client)
}

// WithClientTransport provides Go-idiomatic resource management with a custom transport for testing.
// This is the testing-friendly version of WithClient that accepts an explicit transport parameter.
//
// Usage in tests:
//
//	transport := newClientMockTransport()
//	err := WithClientTransport(ctx, transport, func(client claudecode.Client) error {
//	    return client.Query(ctx, "What is 2+2?")
//	}, opts...)
//
// Parameters:
//   - ctx: Context for connection management and cancellation
//   - transport: Custom transport to use (typically a mock for testing)
//   - fn: Function to execute with the connected client
//   - opts: Optional client configuration options
//
// Returns an error if connection fails or if fn returns an error.
// Disconnect errors are handled gracefully without overriding the original error from fn.
func WithClientTransport(ctx context.Context, transport Transport, fn func(Client) error, opts ...Option) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	client := NewClientWithTransport(transport, opts...)

	if err := client.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect client: %w", err)
	}

	defer func() {
		// Following Go idiom: cleanup errors don't override the original error
		// from fn, but log them so broken-transport diagnostics aren't lost.
		if disconnectErr := client.Disconnect(); disconnectErr != nil {
			log.Printf("claude-sdk: WithClientTransport disconnect error: %v", disconnectErr)
		}
	}()

	return fn(client)
}

// validateOptions validates the client configuration options
func (c *clientImpl) validateOptions() error {
	if c.options == nil {
		return nil // Nil options are acceptable (use defaults)
	}

	// Auto-configure PermissionPromptToolName when CanUseTool callback is set
	// This tells CLI to route permission prompts through stdio (control protocol)
	// Matches Python SDK behavior: permission_prompt_tool_name="stdio"
	if c.options.CanUseTool != nil && c.options.PermissionPromptToolName == nil {
		stdio := "stdio"
		c.options.PermissionPromptToolName = &stdio
	}

	// Validate working directory
	if c.options.Cwd != nil {
		if _, err := os.Stat(*c.options.Cwd); os.IsNotExist(err) {
			return fmt.Errorf("working directory does not exist: %s", *c.options.Cwd)
		}
	}

	// Validate max turns
	if c.options.MaxTurns < 0 {
		return fmt.Errorf("max_turns must be non-negative, got: %d", c.options.MaxTurns)
	}

	// Validate permission mode
	if c.options.PermissionMode != nil {
		validModes := map[PermissionMode]bool{
			PermissionModeDefault:           true,
			PermissionModeAcceptEdits:       true,
			PermissionModePlan:              true,
			PermissionModeBypassPermissions: true,
		}
		if !validModes[*c.options.PermissionMode] {
			return fmt.Errorf("invalid permission mode: %s", string(*c.options.PermissionMode))
		}
	}

	return nil
}

// Connect establishes a connection to the Claude Code CLI.
func (c *clientImpl) Connect(ctx context.Context, _ ...StreamMessage) error {
	// Check context before acquiring lock
	if ctx.Err() != nil {
		return ctx.Err()
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Check context again after acquiring lock
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Validate configuration before connecting
	if err := c.validateOptions(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	// Use custom transport if provided, otherwise create default
	if c.customTransport != nil {
		c.transport = c.customTransport
	} else {
		// Create default subprocess transport directly (like Python SDK)
		cliPath, err := cli.FindCLI()
		if err != nil {
			return fmt.Errorf("claude CLI not found: %w", err)
		}

		// Create subprocess transport for streaming mode (closeStdin=false)
		c.transport = subprocess.New(cliPath, c.options, false, "sdk-go-client")
	}

	// Connect the transport
	if err := c.transport.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect transport: %w", err)
	}

	// Get message channels
	c.msgChan, c.errChan = c.transport.ReceiveMessages(ctx)

	// Buffered so QueryStream's goroutine can surface a send error without
	// blocking even when nothing is currently reading from the iterator.
	c.streamErrChan = make(chan error, 4)

	c.connected = true
	return nil
}

// Disconnect closes the connection to the Claude Code CLI.
func (c *clientImpl) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.transport != nil && c.connected {
		if err := c.transport.Close(); err != nil {
			return fmt.Errorf("failed to close transport: %w", err)
		}
	}
	c.connected = false
	c.transport = nil
	c.msgChan = nil
	c.errChan = nil
	if c.streamErrChan != nil {
		close(c.streamErrChan)
		c.streamErrChan = nil
	}
	return nil
}

// Query sends a simple text query using the default session.
// This is equivalent to QueryWithSession(ctx, prompt, "default").
//
// Example:
//
//	client.Query(ctx, "What is Go?")
func (c *clientImpl) Query(ctx context.Context, prompt string) error {
	return c.queryWithSession(ctx, prompt, defaultSessionID)
}

// QueryWithSession sends a simple text query using the specified session ID.
// Each session maintains its own conversation context, allowing for isolated
// conversations within the same client connection.
//
// If sessionID is empty, it defaults to "default".
//
// Example:
//
//	client.QueryWithSession(ctx, "Remember this", "my-session")
//	client.QueryWithSession(ctx, "What did I just say?", "my-session") // Remembers context
//	client.Query(ctx, "What did I just say?")                          // Won't remember, different session
func (c *clientImpl) QueryWithSession(ctx context.Context, prompt string, sessionID string) error {
	// Use default session if empty session ID provided
	if sessionID == "" {
		sessionID = defaultSessionID
	}
	return c.queryWithSession(ctx, prompt, sessionID)
}

// queryWithSession is the internal implementation for sending queries with session management.
func (c *clientImpl) queryWithSession(ctx context.Context, prompt string, sessionID string) error {
	// Check context before proceeding
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Check connection status with read lock
	c.mu.RLock()
	connected := c.connected
	transport := c.transport
	c.mu.RUnlock()

	if !connected || transport == nil {
		return fmt.Errorf("client not connected")
	}

	// Check context again after acquiring connection info
	if ctx.Err() != nil {
		return ctx.Err()
	}

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

	// Send message via transport (without holding mutex to avoid blocking other operations)
	return transport.SendMessage(ctx, streamMsg)
}

// QueryStream sends a stream of messages.
func (c *clientImpl) QueryStream(ctx context.Context, messages <-chan StreamMessage) error {
	// Check connection status with read lock
	c.mu.RLock()
	connected := c.connected
	transport := c.transport
	c.mu.RUnlock()

	if !connected || transport == nil {
		return fmt.Errorf("client not connected")
	}

	// Capture the error channel so the goroutine can forward send failures
	// without re-acquiring the lock after the caller may have Disconnected.
	c.mu.RLock()
	streamErrChan := c.streamErrChan
	c.mu.RUnlock()

	// Send messages from channel in a goroutine
	go func() {
		for {
			select {
			case msg, ok := <-messages:
				if !ok {
					return // Channel closed
				}
				if err := transport.SendMessage(ctx, msg); err != nil {
					// Streaming sends have no caller-visible return path on
					// QueryStream itself, so forward the error into the
					// receive-side channel so a caller blocked on Receive*
					// sees the broken pipe. Log as well for cases where no
					// receiver is active. Non-blocking send prevents hanging
					// the goroutine if the buffer is already full.
					log.Printf("claude-sdk: QueryStream SendMessage failed: %v", err)
					if streamErrChan != nil {
						select {
						case streamErrChan <- err:
						default:
						}
					}
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	return nil
}

// ReceiveMessages returns a channel of incoming messages.
func (c *clientImpl) ReceiveMessages(_ context.Context) <-chan Message {
	// Check connection status with read lock
	c.mu.RLock()
	connected := c.connected
	msgChan := c.msgChan
	c.mu.RUnlock()

	if !connected || msgChan == nil {
		// Return closed channel if not connected
		closedChan := make(chan Message)
		close(closedChan)
		return closedChan
	}

	// Return the transport's message channel directly
	return msgChan
}

// ReceiveResponse returns an iterator for the response messages.
func (c *clientImpl) ReceiveResponse(_ context.Context) MessageIterator {
	// Check connection status with read lock
	c.mu.RLock()
	connected := c.connected
	msgChan := c.msgChan
	errChan := c.errChan
	streamErrChan := c.streamErrChan
	c.mu.RUnlock()

	if !connected || msgChan == nil {
		return nil
	}

	// Create a simple iterator over the message channel
	return &clientIterator{
		msgChan:       msgChan,
		errChan:       errChan,
		streamErrChan: streamErrChan,
	}
}

// Interrupt sends an interrupt signal to stop the current operation.
func (c *clientImpl) Interrupt(ctx context.Context) error {
	// Check context before proceeding
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Check connection status with read lock
	c.mu.RLock()
	connected := c.connected
	transport := c.transport
	c.mu.RUnlock()

	if !connected || transport == nil {
		return fmt.Errorf("client not connected")
	}

	return transport.Interrupt(ctx)
}

// SetModel changes the AI model during a streaming session.
// Pass nil to reset to the default model.
// Returns error if not connected or if the control request fails.
//
// Example - Change to a specific model:
//
//	model := "claude-sonnet-4-5"
//	err := client.SetModel(ctx, &model)
//
// Example - Reset to default model:
//
//	err := client.SetModel(ctx, nil)
func (c *clientImpl) SetModel(ctx context.Context, model *string) error {
	// Check context before proceeding (Go idiom: fail fast)
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Check connection status with read lock (minimize lock duration)
	c.mu.RLock()
	connected := c.connected
	transport := c.transport
	c.mu.RUnlock()

	if !connected || transport == nil {
		return fmt.Errorf("client not connected")
	}

	return transport.SetModel(ctx, model)
}

// SetPermissionMode changes the permission mode during a streaming session.
// Valid modes: PermissionModeDefault, PermissionModeAcceptEdits,
// PermissionModePlan, PermissionModeBypassPermissions.
// Returns error if not connected or if the control request fails.
//
// Example - Enable auto-accept for edits:
//
//	err := client.SetPermissionMode(ctx, claudecode.PermissionModeAcceptEdits)
//
// Example - Switch to plan mode:
//
//	err := client.SetPermissionMode(ctx, claudecode.PermissionModePlan)
func (c *clientImpl) SetPermissionMode(ctx context.Context, mode PermissionMode) error {
	// Check context before proceeding (Go idiom: fail fast)
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Check connection status with read lock (minimize lock duration)
	c.mu.RLock()
	connected := c.connected
	transport := c.transport
	c.mu.RUnlock()

	if !connected || transport == nil {
		return fmt.Errorf("client not connected")
	}

	return transport.SetPermissionMode(ctx, string(mode))
}

// RewindFiles reverts tracked files to their state at a specific user message.
// The messageUUID should be the UUID from a UserMessage received during the session.
// Requires file checkpointing to be enabled via WithFileCheckpointing() option.
// Returns error if not connected or the request fails.
//
// Example:
//
//	client := claudecode.NewClient(claudecode.WithFileCheckpointing())
//	// ... connect and receive messages, capture UUID from UserMessage
//	if msg, ok := receivedMsg.(*claudecode.UserMessage); ok && msg.UUID != nil {
//	    err := client.RewindFiles(ctx, *msg.UUID)
//	}
func (c *clientImpl) RewindFiles(ctx context.Context, messageUUID string) error {
	// Check context before proceeding (Go idiom: fail fast)
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Check connection status with read lock (minimize lock duration)
	c.mu.RLock()
	connected := c.connected
	transport := c.transport
	c.mu.RUnlock()

	if !connected || transport == nil {
		return fmt.Errorf("client not connected")
	}

	return transport.RewindFiles(ctx, messageUUID)
}

// GetMcpStatus returns the current status of all connected MCP servers.
// Returns error if not connected or if the control request fails.
func (c *clientImpl) GetMcpStatus(ctx context.Context) (*McpStatusResponse, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	c.mu.RLock()
	connected := c.connected
	transport := c.transport
	c.mu.RUnlock()

	if !connected || transport == nil {
		return nil, fmt.Errorf("client not connected")
	}

	return transport.GetMcpStatus(ctx)
}

// clientIterator implements MessageIterator for client message reception.
// closed is guarded by mu since Next/Close may be called from concurrent goroutines.
type clientIterator struct {
	msgChan       <-chan Message
	errChan       <-chan error
	streamErrChan <-chan error
	mu            sync.Mutex
	closed        bool
}

func (ci *clientIterator) markClosed() {
	ci.mu.Lock()
	ci.closed = true
	ci.mu.Unlock()
}

func (ci *clientIterator) isClosed() bool {
	ci.mu.Lock()
	defer ci.mu.Unlock()
	return ci.closed
}

func (ci *clientIterator) Next(ctx context.Context) (Message, error) {
	if ci.isClosed() {
		return nil, ErrNoMoreMessages
	}

	select {
	case msg, ok := <-ci.msgChan:
		if !ok {
			ci.markClosed()
			return nil, ErrNoMoreMessages
		}
		return msg, nil
	case err := <-ci.errChan:
		ci.markClosed()
		return nil, err
	case err := <-ci.streamErrChan:
		// Send-side failure from QueryStream goroutine. Surface the same
		// way as transport errChan so callers see the broken pipe.
		ci.markClosed()
		return nil, err
	case <-ctx.Done():
		ci.markClosed()
		return nil, ctx.Err()
	}
}

func (ci *clientIterator) Close() error {
	ci.markClosed()
	return nil
}

// GetStreamIssues returns validation issues found in the message stream.
// This can help diagnose problems like missing tool results or incomplete streams.
func (c *clientImpl) GetStreamIssues() []StreamIssue {
	c.mu.RLock()
	transport := c.transport
	c.mu.RUnlock()

	if transport == nil {
		return nil
	}

	validator := transport.GetValidator()
	if validator == nil {
		return nil
	}

	return validator.GetIssues()
}

// GetStreamStats returns statistics about the message stream.
// This includes counts of tools requested/received and pending tools.
func (c *clientImpl) GetStreamStats() StreamStats {
	c.mu.RLock()
	transport := c.transport
	c.mu.RUnlock()

	if transport == nil {
		return StreamStats{}
	}

	validator := transport.GetValidator()
	if validator == nil {
		return StreamStats{}
	}

	return validator.GetStats()
}

// GetServerInfo returns diagnostic information about the client and its connection.
// This provides useful information for debugging, health checks, and support scenarios.
//
// This method is thread-safe and can be called concurrently from multiple goroutines.
//
// Returns a map containing:
//   - "connected": bool - Whether the client is currently connected
//   - "transport_type": string - The type of transport being used (e.g., "subprocess")
//
// Returns an error if the client is not connected.
//
// Example:
//
//	info, err := client.GetServerInfo(ctx)
//	if err != nil {
//	    log.Printf("Client not connected: %v", err)
//	    return
//	}
//	fmt.Printf("Connected: %v, Transport: %s\n",
//	    info["connected"], info["transport_type"])
func (c *clientImpl) GetServerInfo(_ context.Context) (map[string]interface{}, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.connected || c.transport == nil {
		return nil, fmt.Errorf("client not connected")
	}

	info := map[string]interface{}{
		"connected":      true,
		"transport_type": "subprocess",
	}

	return info, nil
}
