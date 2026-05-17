package claudecode

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/severity1/claude-agent-sdk-go/internal/cli"
	"github.com/severity1/claude-agent-sdk-go/internal/subprocess"
)

// ErrNoMoreMessages indicates the message iterator has no more messages.
var ErrNoMoreMessages = errors.New("no more messages")

// Query executes a one-shot query with automatic cleanup.
// The prompt is written to stdin as a user-message JSON line after the
// initialize handshake, matching the TypeScript SDK behavior.
func Query(ctx context.Context, prompt string, opts ...Option) (MessageIterator, error) {
	options := NewOptions(opts...)

	transport, err := createQueryTransport(options)
	if err != nil {
		return nil, fmt.Errorf("failed to create query transport: %w", err)
	}

	return queryWithTransportAndOptions(ctx, prompt, transport, options)
}

// QueryWithTransport executes a query with a custom transport.
// The transport parameter is required and must not be nil.
func QueryWithTransport(
	ctx context.Context,
	prompt string,
	transport Transport,
	opts ...Option,
) (MessageIterator, error) {
	if transport == nil {
		return nil, fmt.Errorf("transport is required")
	}

	options := NewOptions(opts...)
	return queryWithTransportAndOptions(ctx, prompt, transport, options)
}

// Internal helper functions
func queryWithTransportAndOptions(
	ctx context.Context,
	prompt string,
	transport Transport,
	options *Options,
) (MessageIterator, error) {
	if transport == nil {
		return nil, fmt.Errorf("transport is required")
	}

	// Create iterator that manages the transport lifecycle
	return &queryIterator{
		transport: transport,
		prompt:    prompt,
		ctx:       ctx,
		options:   options,
	}, nil
}

// queryIterator implements MessageIterator for simple queries
type queryIterator struct {
	transport          Transport
	prompt             string
	ctx                context.Context
	options            *Options
	started            bool
	msgChan            <-chan Message
	errChan            <-chan error
	mu                 sync.Mutex
	closed             bool
	closeOnce          sync.Once
	endInputAfterFirst bool
	endInputOnce       sync.Once
}

func (qi *queryIterator) Next(_ context.Context) (Message, error) {
	qi.mu.Lock()
	if qi.closed {
		qi.mu.Unlock()
		return nil, ErrNoMoreMessages
	}

	// Initialize on first call
	if !qi.started {
		if err := qi.start(); err != nil {
			qi.mu.Unlock()
			return nil, err
		}
		qi.started = true
	}
	qi.mu.Unlock()

	// Read from message channels
	select {
	case msg, ok := <-qi.msgChan:
		if !ok {
			qi.mu.Lock()
			qi.closed = true
			qi.mu.Unlock()
			return nil, ErrNoMoreMessages
		}
		qi.maybeEndInputAfterResult(msg)
		return msg, nil
	case err := <-qi.errChan:
		qi.mu.Lock()
		qi.closed = true
		qi.mu.Unlock()
		return nil, err
	case <-qi.ctx.Done():
		qi.mu.Lock()
		qi.closed = true
		qi.mu.Unlock()
		return nil, qi.ctx.Err()
	}
}

// maybeEndInputAfterResult closes the transport stdin write side after the
// first ResultMessage observed in bidirectional mode (hooks / CanUseTool /
// SDK MCP / file checkpointing configured). For non-bidirectional queries,
// stdin is already closed in start().
func (qi *queryIterator) maybeEndInputAfterResult(msg Message) {
	if !qi.endInputAfterFirst {
		return
	}
	if _, ok := msg.(*ResultMessage); !ok {
		return
	}
	qi.endInputOnce.Do(func() {
		if ender, ok := qi.transport.(interface {
			EndInput(context.Context) error
		}); ok {
			_ = ender.EndInput(qi.ctx)
		}
	})
}

func (qi *queryIterator) Close() error {
	var err error
	qi.closeOnce.Do(func() {
		qi.mu.Lock()
		qi.closed = true
		qi.mu.Unlock()
		if qi.transport != nil {
			err = qi.transport.Close()
		}
	})
	return err
}

func (qi *queryIterator) start() error {
	// Connect to transport. Initialize handshake (including the agents
	// field) runs inside Connect.
	if err := qi.transport.Connect(qi.ctx); err != nil {
		return fmt.Errorf("failed to connect transport: %w", err)
	}

	msgChan, errChan := qi.transport.ReceiveMessages(qi.ctx)
	qi.msgChan = msgChan
	qi.errChan = errChan

	// Write the prompt as a user-message JSON line on stdin, matching the
	// wire shape of Python's `{"type":"user","session_id":"",
	// "message":{"role":"user","content":...},"parent_tool_use_id":null}`.
	streamMsg := StreamMessage{
		Type: "user",
		Message: map[string]any{
			"role":    "user",
			"content": qi.prompt,
		},
		SessionID:       "",
		ParentToolUseID: nil,
	}

	if err := qi.transport.SendMessage(qi.ctx, streamMsg); err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	qi.endInputAfterFirst = needsBidirectionalStdin(qi.options)
	if !qi.endInputAfterFirst {
		qi.endInputOnce.Do(func() {
			if ender, ok := qi.transport.(interface {
				EndInput(context.Context) error
			}); ok {
				_ = ender.EndInput(qi.ctx)
			}
		})
	}

	return nil
}

// needsBidirectionalStdin reports whether the query needs the CLI to keep
// reading from stdin after the prompt is written. Hooks, permission
// callbacks, SDK-MCP servers, and file checkpointing all rely on the
// control protocol over stdin while the query runs, so stdin must stay
// open until the first ResultMessage is observed.
func needsBidirectionalStdin(options *Options) bool {
	if options == nil {
		return false
	}
	if options.Hooks != nil || options.CanUseTool != nil || options.EnableFileCheckpointing {
		return true
	}
	for _, config := range options.McpServers {
		if sdk, ok := config.(*McpSdkServerConfig); ok && sdk.Instance != nil {
			return true
		}
	}
	return false
}

// createQueryTransport creates a transport for one-shot queries.
//
// If the caller supplied a CLI path via WithCLIPath, that path is used directly
// and CLI auto-discovery is skipped. This matches the documented behaviour of
// the option (and the Python SDK's `cli_path` parameter) and lets callers
// bypass exec.LookPath when, for example, the npm shim on their platform
// mishandles command-line arguments.
func createQueryTransport(options *Options) (Transport, error) {
	cliPath, err := resolveCLIPath(options)
	if err != nil {
		return nil, err
	}
	return subprocess.New(cliPath, options, "sdk-go"), nil
}

// resolveCLIPath returns the CLI path the transport should invoke. When
// options.CLIPath is set and non-empty, it wins over auto-discovery — the
// caller has explicitly opted out of FindCLI's PATH/well-known-location
// search.
func resolveCLIPath(options *Options) (string, error) {
	if options != nil && options.CLIPath != nil && *options.CLIPath != "" {
		return *options.CLIPath, nil
	}
	return cli.FindCLI()
}
