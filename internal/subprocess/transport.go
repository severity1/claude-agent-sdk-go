// Package subprocess provides the subprocess transport implementation for Claude Code CLI.
package subprocess

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"time"

	"github.com/severity1/claude-agent-sdk-go/internal/cli"
	"github.com/severity1/claude-agent-sdk-go/internal/control"
	"github.com/severity1/claude-agent-sdk-go/internal/parser"
	"github.com/severity1/claude-agent-sdk-go/internal/shared"
)

const (
	// channelBufferSize is the buffer size for message and error channels.
	channelBufferSize = 10
	// terminationTimeoutSeconds is the timeout for graceful process termination.
	terminationTimeoutSeconds = 5
	// windowsOS is the GOOS value for Windows platform.
	windowsOS = "windows"
)

// Transport implements the Transport interface using subprocess communication.
type Transport struct {
	// Process management
	cmd        *exec.Cmd
	cliPath    string
	options    *shared.Options
	closeStdin bool
	promptArg  *string // For one-shot queries, prompt passed as CLI argument
	entrypoint string  // CLAUDE_CODE_ENTRYPOINT value (sdk-go or sdk-go-client)

	// Connection state
	connected bool
	mu        sync.RWMutex

	// I/O streams
	stdin      io.WriteCloser
	stdout     io.ReadCloser
	stderr     *os.File      // Temporary file for stderr isolation
	stderrPipe io.ReadCloser // Pipe for callback-based stderr handling

	// Temporary files (cleaned up on Close)
	mcpConfigFile *os.File // Temporary MCP config file

	// Message parsing
	parser *parser.Parser

	// Stream validation
	validator *shared.StreamValidator

	// Channels for communication
	msgChan chan shared.Message
	errChan chan error

	// Control protocol (for streaming mode only)
	protocol        *control.Protocol
	protocolAdapter *ProtocolAdapter

	// Control and cleanup
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// New creates a new subprocess transport.
func New(cliPath string, options *shared.Options, closeStdin bool, entrypoint string) *Transport {
	return &Transport{
		cliPath:    cliPath,
		options:    options,
		closeStdin: closeStdin,
		entrypoint: entrypoint,
		parser:     newParser(options),
		validator:  shared.NewStreamValidator(),
	}
}

// NewWithPrompt creates a new subprocess transport for one-shot queries with prompt as CLI argument.
func NewWithPrompt(cliPath string, options *shared.Options, prompt string) *Transport {
	return &Transport{
		cliPath:    cliPath,
		options:    options,
		closeStdin: true,
		entrypoint: "sdk-go", // Query mode uses sdk-go
		parser:     newParser(options),
		validator:  shared.NewStreamValidator(),
		promptArg:  &prompt,
	}
}

// newParser creates a parser using the buffer size from options, or the default.
func newParser(options *shared.Options) *parser.Parser {
	if options != nil && options.MaxBufferSize != nil {
		return parser.NewWithSize(*options.MaxBufferSize)
	}
	return parser.New()
}

// IsConnected returns whether the transport is currently connected.
func (t *Transport) IsConnected() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.connected && t.cmd != nil && t.cmd.Process != nil
}

// Connect starts the Claude CLI subprocess.
func (t *Transport) Connect(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.connected {
		return fmt.Errorf("transport already connected")
	}

	// Generate MCP config file if McpServers are specified
	opts, err := t.prepareMcpConfig()
	if err != nil {
		return err
	}

	// Build command with all options
	var args []string
	if t.promptArg != nil {
		// One-shot query with prompt as CLI argument
		args = cli.BuildCommandWithPrompt(t.cliPath, opts, *t.promptArg)
	} else {
		// Streaming mode or regular one-shot
		args = cli.BuildCommand(t.cliPath, opts, t.closeStdin)
	}
	//nolint:gosec // G204: This is the core CLI SDK functionality - subprocess execution is required
	t.cmd = exec.CommandContext(ctx, args[0], args[1:]...)

	// Set up environment and apply to command
	t.cmd.Env = t.buildEnvironment()

	// Set working directory if specified
	if t.options != nil && t.options.Cwd != nil {
		if err := cli.ValidateWorkingDirectory(*t.options.Cwd); err != nil {
			return err
		}
		t.cmd.Dir = *t.options.Cwd
	}

	// Check CLI version and warn if outdated (non-blocking)
	t.emitCLIVersionWarning(ctx)

	// Set up I/O pipes
	if err := t.setupIoPipes(); err != nil {
		return err
	}

	// Start the process
	if err := t.cmd.Start(); err != nil {
		t.cleanup()
		return shared.NewConnectionError(
			fmt.Sprintf("failed to start Claude CLI: %v", err),
			err,
		)
	}

	// Set up context for goroutine management
	t.ctx, t.cancel = context.WithCancel(ctx)

	// Initialize channels
	t.msgChan = make(chan shared.Message, channelBufferSize)
	t.errChan = make(chan error, channelBufferSize)

	// Start I/O handling goroutines
	t.wg.Add(1)
	go t.handleStdout()

	// Start stderr callback goroutine if callback is configured
	if t.stderrPipe != nil && t.options != nil && t.options.StderrCallback != nil {
		t.wg.Add(1)
		go t.handleStderrCallback()
	}

	// Note: Do NOT close stdin here for one-shot mode
	// The CLI still needs stdin to receive the message, even with --print flag
	// stdin will be closed after sending the message in SendMessage()

	// Set up control protocol for streaming mode only
	if err := t.setupControlProtocol(t.ctx); err != nil {
		return err
	}

	t.connected = true
	return nil
}

// setupControlProtocol initializes control protocol for streaming mode.
// Returns nil immediately for one-shot mode (closeStdin == true).
func (t *Transport) setupControlProtocol(ctx context.Context) error {
	if t.closeStdin {
		return nil // One-shot mode doesn't need control protocol
	}

	t.protocolAdapter = NewProtocolAdapter(t.stdin)
	t.protocol = control.NewProtocol(t.protocolAdapter, t.buildProtocolOptions()...)

	if err := t.protocol.Start(ctx); err != nil {
		t.cleanup()
		return fmt.Errorf("failed to start control protocol: %w", err)
	}

	// Perform handshake when hooks, permissions, checkpointing, or SDK MCP servers configured
	if t.needsProtocolHandshake() {
		if _, err := t.protocol.Initialize(ctx); err != nil {
			t.cleanup()
			return fmt.Errorf("failed to initialize control protocol: %w", err)
		}
	}

	return nil
}

// needsProtocolHandshake returns true if control protocol handshake is required.
// Handshake is needed whenever a feature relies on the control channel
// (hooks, permission callbacks, checkpointing, SDK MCP servers, agents).
// Methods like GetMcpStatus lazy-init on demand so they work even when no
// control-channel feature is configured up front.
func (t *Transport) needsProtocolHandshake() bool {
	if t.options == nil {
		return false
	}
	return t.options.Hooks != nil ||
		t.options.CanUseTool != nil ||
		t.options.EnableFileCheckpointing ||
		t.hasSdkMcpServers() ||
		len(t.options.Agents) > 0
}

// ensureProtocolInitialized performs the control protocol handshake lazily
// when called, so callers that need the control channel on demand
// (e.g. GetMcpStatus) work even when no up-front feature requested init.
// Only valid in streaming mode; one-shot transports never initialize.
func (t *Transport) ensureProtocolInitialized(ctx context.Context) error {
	if t.closeStdin {
		return fmt.Errorf("control protocol not available in one-shot mode")
	}
	if t.protocol == nil {
		return fmt.Errorf("control protocol not started")
	}
	_, err := t.protocol.Initialize(ctx)
	return err
}

// SendMessage sends a message to the CLI subprocess.
func (t *Transport) SendMessage(ctx context.Context, message shared.StreamMessage) error {
	t.mu.RLock()
	defer t.mu.RUnlock()

	// For one-shot queries with promptArg, the prompt is already passed as CLI argument
	// so we don't need to send any messages via stdin
	if t.promptArg != nil {
		return nil // No-op for one-shot queries
	}

	if !t.connected || t.stdin == nil {
		return fmt.Errorf("transport not connected or stdin closed")
	}

	// Check context cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Serialize message to JSON
	data, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// Send with newline
	_, err = t.stdin.Write(append(data, '\n'))
	if err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}

	// For one-shot mode, close stdin after sending the message
	if t.closeStdin {
		_ = t.stdin.Close()
		t.stdin = nil
	}

	return nil
}

// ReceiveMessages returns channels for receiving messages and errors.
func (t *Transport) ReceiveMessages(_ context.Context) (<-chan shared.Message, <-chan error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if !t.connected {
		// Return closed channels if not connected
		msgChan := make(chan shared.Message)
		errChan := make(chan error)
		close(msgChan)
		close(errChan)
		return msgChan, errChan
	}

	return t.msgChan, t.errChan
}

// Interrupt sends an interrupt signal to the subprocess.
func (t *Transport) Interrupt(_ context.Context) error {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if !t.connected || t.cmd == nil || t.cmd.Process == nil {
		return fmt.Errorf("process not running")
	}

	// Windows doesn't support os.Interrupt signal
	if runtime.GOOS == windowsOS {
		return fmt.Errorf("interrupt not supported by windows")
	}

	// Send interrupt signal (Unix/Linux/macOS)
	return t.cmd.Process.Signal(os.Interrupt)
}

// Close terminates the subprocess connection.
func (t *Transport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.connected {
		return nil // Already closed
	}

	t.connected = false

	// Close control protocol first (before cancelling context)
	if t.protocol != nil {
		_ = t.protocol.Close()
		t.protocol = nil
	}
	if t.protocolAdapter != nil {
		_ = t.protocolAdapter.Close()
		t.protocolAdapter = nil
	}

	// Cancel context to stop goroutines
	if t.cancel != nil {
		t.cancel()
	}

	// Close stdin if open
	if t.stdin != nil {
		_ = t.stdin.Close()
		t.stdin = nil
	}

	// Wait for goroutines to finish with timeout
	done := make(chan struct{})
	go func() {
		t.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Goroutines finished gracefully
	case <-time.After(terminationTimeoutSeconds * time.Second):
		// Timeout: proceed with cleanup anyway
		// Goroutines should terminate when process is killed
	}

	// Terminate process with 5-second timeout
	var err error
	if t.cmd != nil && t.cmd.Process != nil {
		err = t.terminateProcess()
	}

	// Cleanup resources
	t.cleanup()

	return err
}
