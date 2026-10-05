package claudecode

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/severity1/claude-agent-sdk-go/internal/subprocess"
)

const (
	defaultSessionID = "default"
	windowsOS        = "windows"
)

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
	// GetMcpStatus returns the connection status of all configured MCP servers.
	// Only works in streaming mode (after Connect()).
	GetMcpStatus(ctx context.Context) (*McpStatusResponse, error)
	// StopTask stops a single running task, such as one subagent, by the
	// task ID from its TaskStartedMessage.
	// Only works in streaming mode (after Connect()).
	StopTask(ctx context.Context, taskID string) error
	// ReconnectMcpServer reconnects a disconnected or failed MCP server by
	// the name from its configuration.
	// Only works in streaming mode (after Connect()).
	ReconnectMcpServer(ctx context.Context, serverName string) error
	// ToggleMcpServer enables or disables an MCP server by the name from its
	// configuration. A disabled server shows status "disabled" in GetMcpStatus.
	// Only works in streaming mode (after Connect()).
	ToggleMcpServer(ctx context.Context, serverName string, enabled bool) error
	GetStreamIssues() []StreamIssue
	GetStreamStats() StreamStats
	GetServerInfo(ctx context.Context) (map[string]interface{}, error)
	// Done returns a channel that is closed when the connected CLI process
	// exits, on its own or through Disconnect. It does not wait for the
	// ReceiveMessages channel: messages the CLI wrote before it exited can
	// still be in flight, and a process the CLI started can keep that channel
	// open after the CLI is gone. Before Connect and after Disconnect the
	// channel is closed. With a custom Transport that does not report its
	// process, the channel closes on Disconnect.
	Done() <-chan struct{}
	// Err returns nil while the CLI process runs. Once Done is closed it
	// returns why: a *ProcessError for a non-zero exit (ExitCode -1 for a
	// signal), a *ConnectionError for a clean exit, or a "client not
	// connected" error before Connect and after Disconnect. Calls that write
	// to the CLI then fail with a *ConnectionError that wraps it.
	Err() error
}

// processWatcher is implemented by transports that report the exit of their
// CLI process. Done and Err follow Client.Done and Client.Err.
type processWatcher interface {
	Done() <-chan struct{}
	Err() error
}

// closedDone is the Done channel of a client with no connection.
var closedDone = func() chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}()

// ClientImpl implements the Client interface.
type ClientImpl struct {
	mu              sync.RWMutex
	transport       Transport
	customTransport Transport // For testing with WithTransport
	options         *Options
	connected       bool
	msgChan         <-chan Message
	stream          *streamReader // shared by every ReceiveResponse iterator
	streamErrChan   chan error    // writable; receives errors from QueryStream goroutine
	// disconnected closes on Disconnect. Done returns it when the transport
	// cannot report its process.
	disconnected chan struct{}
	// watch is guarded by watchMu, not mu, so Done and Err never wait for
	// Disconnect. It is nil while the client is not connected.
	watchMu sync.Mutex
	watch   *connectionWatch
}

// connectionWatch is what Done and Err read for one connection.
type connectionWatch struct {
	watcher      processWatcher // nil when the transport cannot report its process
	disconnected chan struct{}
}

// NewClient creates a new Client with the given options.
func NewClient(opts ...Option) Client {
	options := NewOptions(opts...)
	client := &ClientImpl{
		options: options,
	}
	return client
}

// NewClientWithTransport creates a new Client with a custom transport (for testing).
func NewClientWithTransport(transport Transport, opts ...Option) Client {
	options := NewOptions(opts...)
	return &ClientImpl{
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
//	            for _, block := range assistantMsg.Content {
//	                if text, ok := block.(*claudecode.TextBlock); ok {
//	                    fmt.Println("Claude:", text.Text)
//	                }
//	            }
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
		// This matches patterns in database/sql, os.File, and other stdlib packages
		if disconnectErr := client.Disconnect(); disconnectErr != nil {
			// Log cleanup errors but don't return them to preserve the original error
			// This follows the standard Go pattern for resource cleanup
			_ = disconnectErr // Explicitly acknowledge we're ignoring this error
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
		if disconnectErr := client.Disconnect(); disconnectErr != nil {
			// Log cleanup errors but don't return them to preserve the original error
			_ = disconnectErr // Explicitly acknowledge we're ignoring this error
		}
	}()

	return fn(client)
}

// permissionPromptToolStdio routes permission prompts over the control protocol.
const permissionPromptToolStdio = "stdio"

// prepareOptions applies defaults and validates options. Query and
// Client.Connect both call it, as Python's _configure_can_use_tool runs for
// query() and connect().
func prepareOptions(options *Options) error {
	if options == nil {
		return nil // Nil options are acceptable (use defaults)
	}

	// CanUseTool only fires when the CLI routes permission prompts over stdio.
	if options.CanUseTool != nil {
		toolName := options.PermissionPromptToolName
		if toolName != nil && *toolName != permissionPromptToolStdio {
			return fmt.Errorf("CanUseTool callback cannot be used with PermissionPromptToolName %q: use one or the other", *toolName)
		}
		stdio := permissionPromptToolStdio
		options.PermissionPromptToolName = &stdio
	}

	// Validate working directory
	if options.Cwd != nil {
		if _, err := os.Stat(*options.Cwd); os.IsNotExist(err) {
			return fmt.Errorf("working directory does not exist: %s", *options.Cwd)
		}
	}

	if err := validateWindowsArgs(runtime.GOOS, options); err != nil {
		return err
	}

	// Validate max turns
	if options.MaxTurns < 0 {
		return fmt.Errorf("max_turns must be non-negative, got: %d", options.MaxTurns)
	}

	// Validate permission mode
	if options.PermissionMode != nil {
		validModes := map[PermissionMode]bool{
			PermissionModeDefault:           true,
			PermissionModeAcceptEdits:       true,
			PermissionModePlan:              true,
			PermissionModeBypassPermissions: true,
		}
		if !validModes[*options.PermissionMode] {
			return fmt.Errorf("invalid permission mode: %s", string(*options.PermissionMode))
		}
	}

	return nil
}

// windowsCmdMetacharacters are the characters cmd.exe interprets (Python _CMD_EXE_METACHARACTERS).
const windowsCmdMetacharacters = "&|<>^%!\"\r\n"

// validateWindowsArgs checks each option that becomes a --flag=value argv token.
func validateWindowsArgs(goos string, options *Options) error {
	values := []struct {
		name  string
		value *string
	}{
		{"resume", options.Resume},
		{"resume_session_at", options.ResumeSessionAt},
		{"resume_drops_turn", options.ResumeDropsTurn},
	}
	for _, v := range values {
		if v.value == nil {
			continue
		}
		if err := validateWindowsArgValue(goos, v.name, *v.value); err != nil {
			return err
		}
	}
	return nil
}

// validateWindowsArgValue rejects cmd.exe metacharacters in an argv value on Windows.
// Defense in depth: values such as resume often come from external input (Python #1123).
func validateWindowsArgValue(goos, name, value string) error {
	if goos != windowsOS {
		return nil
	}
	var bad []string
	for _, c := range windowsCmdMetacharacters {
		if strings.ContainsRune(value, c) {
			bad = append(bad, strconv.QuoteRune(c))
		}
	}
	if len(bad) == 0 {
		return nil
	}
	return fmt.Errorf("%s value %q contains characters that are unsafe to pass on a Windows command line: %s",
		name, value, strings.Join(bad, " "))
}

// Connect establishes a connection to the Claude Code CLI. ctx bounds only
// the connection setup (like net.Dialer.DialContext): once Connect returns,
// the session lives until Disconnect, even after ctx is cancelled.
func (c *ClientImpl) Connect(ctx context.Context, _ ...StreamMessage) error {
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
	if err := prepareOptions(c.options); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	// Use custom transport if provided, otherwise create default
	if c.customTransport != nil {
		c.transport = c.customTransport
	} else {
		// Honor WithCLIPath when set, otherwise fall back to auto-discovery.
		cliPath, err := resolveCLIPath(c.options)
		if err != nil {
			return fmt.Errorf("claude CLI not found: %w", err)
		}

		c.transport = subprocess.New(cliPath, c.options, "sdk-go-client")
	}

	// Connect the transport
	if err := c.transport.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect transport: %w", err)
	}

	// Get message channels
	msgChan, errChan := c.transport.ReceiveMessages(ctx)
	c.msgChan = msgChan
	c.stream = newStreamReader(msgChan, errChan)
	c.streamErrChan = make(chan error, 1)
	c.disconnected = make(chan struct{})

	watch := &connectionWatch{disconnected: c.disconnected}
	if watcher, ok := c.transport.(processWatcher); ok {
		watch.watcher = watcher
	}
	c.watchMu.Lock()
	c.watch = watch
	c.watchMu.Unlock()

	c.connected = true
	return nil
}

// Disconnect closes the connection to the Claude Code CLI.
func (c *ClientImpl) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Clear the watch before Close: during Close the transport's own state
	// is torn down, so Err reports not-connected, never a transport error.
	watch := c.swapWatch(nil)
	if c.transport != nil && c.connected {
		if err := c.transport.Close(); err != nil {
			c.swapWatch(watch)
			return fmt.Errorf("failed to close transport: %w", err)
		}
	}
	if c.connected {
		close(c.disconnected)
	}
	c.connected = false
	c.transport = nil
	c.msgChan = nil
	c.stream = nil
	c.streamErrChan = nil
	return nil
}

// Query sends a simple text query using the default session.
// This is equivalent to QueryWithSession(ctx, prompt, "default").
//
// Example:
//
//	client.Query(ctx, "What is Go?")
func (c *ClientImpl) Query(ctx context.Context, prompt string) error {
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
func (c *ClientImpl) QueryWithSession(ctx context.Context, prompt string, sessionID string) error {
	// Use default session if empty session ID provided
	if sessionID == "" {
		sessionID = defaultSessionID
	}
	return c.queryWithSession(ctx, prompt, sessionID)
}

// queryWithSession is the internal implementation for sending queries with session management.
func (c *ClientImpl) queryWithSession(ctx context.Context, prompt string, sessionID string) error {
	// Check context before proceeding
	if ctx.Err() != nil {
		return ctx.Err()
	}

	transport, err := c.liveTransport()
	if err != nil {
		return err
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

// QueryStream sends a stream of messages. It returns an error when the client
// cannot write (see Err); a later send error is returned by ReceiveResponse.
func (c *ClientImpl) QueryStream(ctx context.Context, messages <-chan StreamMessage) error {
	c.mu.RLock()
	transport, err := c.liveTransportLocked()
	streamErrChan := c.streamErrChan
	c.mu.RUnlock()

	if err != nil {
		return err
	}

	// Send messages from channel in a goroutine
	go func() {
		for {
			select {
			case msg, ok := <-messages:
				if !ok {
					return // Channel closed
				}
				if err := transport.SendMessage(ctx, msg); err != nil {
					// ReceiveResponse returns this error; log only if it is dropped.
					select {
					case streamErrChan <- fmt.Errorf("stream send error: %w", err):
					default:
						fmt.Fprintf(os.Stderr, "claude-agent-sdk: QueryStream send error dropped: %v\n", err)
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
func (c *ClientImpl) ReceiveMessages(_ context.Context) <-chan Message {
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

// ReceiveResponse returns an iterator for the messages of the current turn.
// The iterator returns the ResultMessage, then ErrNoMoreMessages on the next call.
// Call it again after the next Query to read that turn.
func (c *ClientImpl) ReceiveResponse(_ context.Context) MessageIterator {
	// Check connection status with read lock
	c.mu.RLock()
	connected := c.connected
	msgChan := c.msgChan
	stream := c.stream
	streamErrChan := c.streamErrChan
	c.mu.RUnlock()

	if !connected || msgChan == nil {
		closed := make(chan Message)
		close(closed)
		return &clientIterator{stream: newStreamReader(closed, make(chan error))}
	}

	return &clientIterator{
		stream:        stream,
		streamErrChan: streamErrChan,
	}
}

// Interrupt asks the CLI to stop the current turn. It sends an interrupt
// control request; the CLI stays connected for the next query.
func (c *ClientImpl) Interrupt(ctx context.Context) error {
	// Check context before proceeding
	if ctx.Err() != nil {
		return ctx.Err()
	}

	transport, err := c.liveTransport()
	if err != nil {
		return err
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
func (c *ClientImpl) SetModel(ctx context.Context, model *string) error {
	// Check context before proceeding (Go idiom: fail fast)
	if ctx.Err() != nil {
		return ctx.Err()
	}

	transport, err := c.liveTransport()
	if err != nil {
		return err
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
func (c *ClientImpl) SetPermissionMode(ctx context.Context, mode PermissionMode) error {
	// Check context before proceeding (Go idiom: fail fast)
	if ctx.Err() != nil {
		return ctx.Err()
	}

	transport, err := c.liveTransport()
	if err != nil {
		return err
	}

	return transport.SetPermissionMode(ctx, mode)
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
func (c *ClientImpl) RewindFiles(ctx context.Context, messageUUID string) error {
	// Check context before proceeding (Go idiom: fail fast)
	if ctx.Err() != nil {
		return ctx.Err()
	}

	transport, err := c.liveTransport()
	if err != nil {
		return err
	}

	return transport.RewindFiles(ctx, messageUUID)
}

// GetMcpStatus returns the connection status of all configured MCP servers.
// Returns error if not connected or if the control request fails.
func (c *ClientImpl) GetMcpStatus(ctx context.Context) (*McpStatusResponse, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	transport, err := c.liveTransport()
	if err != nil {
		return nil, err
	}

	return transport.GetMcpStatus(ctx)
}

// Done returns a channel that is closed when the connected CLI process exits.
func (c *ClientImpl) Done() <-chan struct{} {
	watch := c.currentWatch()
	if watch == nil {
		return closedDone
	}
	if watch.watcher != nil {
		return watch.watcher.Done()
	}
	return watch.disconnected
}

// Err returns nil while the connected CLI process runs, and why it stopped
// once Done is closed.
func (c *ClientImpl) Err() error {
	watch := c.currentWatch()
	if watch == nil {
		return notConnectedError()
	}
	if watch.watcher != nil {
		return watch.watcher.Err()
	}
	return nil
}

func (c *ClientImpl) currentWatch() *connectionWatch {
	c.watchMu.Lock()
	defer c.watchMu.Unlock()
	return c.watch
}

// swapWatch sets the watch and returns the old one.
func (c *ClientImpl) swapWatch(watch *connectionWatch) *connectionWatch {
	c.watchMu.Lock()
	defer c.watchMu.Unlock()
	old := c.watch
	c.watch = watch
	return old
}

// liveTransport returns the transport of a connected client whose CLI
// process is still running.
func (c *ClientImpl) liveTransport() (Transport, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.liveTransportLocked()
}

// liveTransportLocked is liveTransport for callers that hold c.mu.
func (c *ClientImpl) liveTransportLocked() (Transport, error) {
	if !c.connected || c.transport == nil {
		return nil, notConnectedError()
	}
	if watcher, ok := c.transport.(processWatcher); ok {
		if err := watcher.Err(); err != nil {
			// Python raises CLIConnectionError from the exit error.
			return nil, NewConnectionError("cannot write to terminated CLI process", err)
		}
	}
	return c.transport, nil
}

// StopTask stops a single running task, such as one subagent, by the task ID
// from its TaskStartedMessage. The rest of the session keeps running.
// Returns error if not connected or if the control request fails.
//
// The CLI then reports the task's end as a TaskUpdatedMessage whose status is
// terminal (killed). A TaskNotificationMessage with status stopped may follow,
// but the CLI sometimes omits it, so clear the task on a terminal status from
// either message (see IsTerminalTaskStatus).
//
// Example:
//
//	if sys, ok := msg.(*claudecode.SystemMessage); ok {
//	    if started, ok := sys.AsTaskStarted(); ok {
//	        err := client.StopTask(ctx, started.TaskID)
//	    }
//	}
func (c *ClientImpl) StopTask(ctx context.Context, taskID string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	transport, err := c.liveTransport()
	if err != nil {
		return err
	}

	return transport.StopTask(ctx, taskID)
}

// ReconnectMcpServer reconnects a disconnected or failed MCP server.
// Returns error if not connected or if the CLI rejects the request, for
// example for an unknown server name.
func (c *ClientImpl) ReconnectMcpServer(ctx context.Context, serverName string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	transport, err := c.liveTransport()
	if err != nil {
		return err
	}

	return transport.ReconnectMcpServer(ctx, serverName)
}

// ToggleMcpServer enables or disables an MCP server.
// Returns error if not connected or if the CLI rejects the request, for
// example for an unknown server name.
func (c *ClientImpl) ToggleMcpServer(ctx context.Context, serverName string, enabled bool) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	transport, err := c.liveTransport()
	if err != nil {
		return err
	}

	return transport.ToggleMcpServer(ctx, serverName, enabled)
}

// clientIterator implements MessageIterator for client message reception
type clientIterator struct {
	stream        *streamReader
	streamErrChan <-chan error
	mu            sync.Mutex
	closed        bool
}

func (ci *clientIterator) Next(ctx context.Context) (Message, error) {
	ci.mu.Lock()
	if ci.closed {
		ci.mu.Unlock()
		return nil, ErrNoMoreMessages
	}
	ci.mu.Unlock()

	msg, err := ci.stream.next(ctx, ci.streamErrChan)
	if err != nil {
		ci.markClosed()
		return nil, err
	}
	// The turn ends at its ResultMessage (Python receive_response); the channel stays open for the next turn.
	if _, isResult := msg.(*ResultMessage); isResult {
		ci.markClosed()
	}
	return msg, nil
}

func (ci *clientIterator) markClosed() {
	ci.mu.Lock()
	ci.closed = true
	ci.mu.Unlock()
}

func (ci *clientIterator) Close() error {
	ci.mu.Lock()
	ci.closed = true
	ci.mu.Unlock()
	return nil
}

// GetStreamIssues returns validation issues found in the message stream.
// This can help diagnose problems like missing tool results or incomplete streams.
func (c *ClientImpl) GetStreamIssues() []StreamIssue {
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
func (c *ClientImpl) GetStreamStats() StreamStats {
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

// serverInfoSource is implemented by transports that keep the initialize
// response the CLI sent during Connect.
type serverInfoSource interface {
	InitializationResult() map[string]interface{}
}

// GetServerInfo returns the initialize response the CLI sent during Connect,
// as decoded JSON: its commands, output styles, models, account and other
// capabilities (Python: get_server_info). Each call returns a copy. It returns
// nil when the transport does not keep the response, and an error when the
// client is not connected.
//
// This method is thread-safe and can be called concurrently from multiple goroutines.
//
// Example:
//
//	info, err := client.GetServerInfo(ctx)
//	if err != nil {
//	    log.Printf("Client not connected: %v", err)
//	    return
//	}
//	if models, ok := info["models"].([]interface{}); ok {
//	    fmt.Printf("Models available: %d\n", len(models))
//	}
func (c *ClientImpl) GetServerInfo(_ context.Context) (map[string]interface{}, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.connected || c.transport == nil {
		return nil, notConnectedError()
	}

	if source, ok := c.transport.(serverInfoSource); ok {
		return source.InitializationResult(), nil
	}
	return nil, nil
}
