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
	entrypoint string // CLAUDE_CODE_ENTRYPOINT value (sdk-go or sdk-go-client)
	// processDone closes after the sole cmd.Wait call returns.
	processDone chan struct{}

	// Connection state
	connected bool
	mu        sync.RWMutex

	// I/O streams
	stdin      *stdinWriter
	stdout     io.ReadCloser
	stderr     *os.File      // Temporary file for stderr isolation
	stderrPipe io.ReadCloser // Pipe for callback-based stderr handling
	// childPipeEnds are the write ends given to the child; closed after Start.
	childPipeEnds []*os.File

	// Temporary files (cleaned up on Close)
	mcpConfigFile *os.File // Temporary MCP config file

	// Message parsing
	parser *parser.Parser

	// Stream validation
	validator *shared.StreamValidator

	// Channels for communication
	msgChan chan shared.Message
	errChan chan error

	// stdoutDone is closed by handleStdout on exit so init-time watchers can
	// detect the CLI exiting before the initialize handshake completes.
	stdoutDone chan struct{}
	// closing is closed when teardown starts; stdout is still drained after it.
	closing chan struct{}

	// Control protocol (for streaming mode only)
	protocol        *control.Protocol
	protocolAdapter *ProtocolAdapter

	// Control and cleanup
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// New creates a new subprocess transport. The transport always uses
// streaming mode (--input-format stream-json); the prompt for one-shot
// queries is written to stdin after the initialize handshake.
func New(cliPath string, options *shared.Options, entrypoint string) *Transport {
	return &Transport{
		cliPath:    cliPath,
		options:    options,
		entrypoint: entrypoint,
		parser:     newParser(options),
		validator:  shared.NewStreamValidator(),
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
	return t.connected && t.cmd != nil && t.cmd.Process != nil && !t.processExitedLocked()
}

// processExitedLocked reports whether the CLI process has exited. Callers hold t.mu.
func (t *Transport) processExitedLocked() bool {
	if t.processDone == nil {
		return false
	}
	select {
	case <-t.processDone:
		return true
	default:
		return false
	}
}

// Connect starts the Claude CLI subprocess.
func (t *Transport) Connect(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.connected {
		return fmt.Errorf("transport already connected")
	}

	// Before anything spawns the CLI, including the version probe below.
	if err := cli.RejectWindowsBatchCLI(runtime.GOOS, t.cliPath); err != nil {
		return err
	}

	// Generate MCP config file if McpServers are specified
	opts, err := t.prepareMcpConfig()
	if err != nil {
		return err
	}

	// Build command with all options. Streaming mode is unconditional;
	// the prompt is written to stdin after initialize, not via --print.
	args := cli.BuildCommand(t.cliPath, opts)
	//nolint:gosec // G204: This is the core CLI SDK functionality - subprocess execution is required
	t.cmd = exec.Command(args[0], args[1:]...)

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

	if err := ctx.Err(); err != nil {
		t.cleanup()
		return err
	}

	// Set up I/O pipes
	if err := t.setupIoPipes(); err != nil {
		return err
	}

	// Start the process
	err = t.cmd.Start()
	t.closeChildPipeEnds()
	if err != nil {
		t.cleanup()
		return shared.NewConnectionError(
			fmt.Sprintf("failed to start Claude CLI: %v", err),
			err,
		)
	}
	t.startProcessWaiter()

	// The connect ctx bounds only the connect step (net.Dialer.DialContext semantics).
	t.ctx, t.cancel = context.WithCancel(context.Background())

	// Initialize channels
	t.msgChan = make(chan shared.Message, channelBufferSize)
	t.errChan = make(chan error, channelBufferSize)
	t.stdoutDone = make(chan struct{})
	t.closing = make(chan struct{})

	// Build the protocol before handleStdout starts so early CLI output never
	// races with the t.protocol assignment.
	t.protocolAdapter = NewProtocolAdapter(t.stdin)
	t.protocol = control.NewProtocol(t.protocolAdapter, t.buildProtocolOptions()...)

	// Start I/O handling goroutines
	t.wg.Add(1)
	go t.handleStdout(t.protocol, childProcess{cmd: t.cmd, done: t.processDone})

	// Start stderr callback goroutine if callback is configured
	if t.stderrPipe != nil && t.options != nil && t.options.StderrCallback != nil {
		t.wg.Add(1)
		go t.handleStderrCallback()
	}

	// Set up control protocol and run the initialize handshake. Initialize
	// is unconditional so the agents map can travel on the initialize
	// request rather than via argv. Failure here means the subprocess is
	// running but we cannot use it - tear down so the process is reaped
	// and goroutines exit.
	if err := t.setupControlProtocol(ctx); err != nil {
		_ = t.teardownLocked()
		return err
	}

	t.connected = true
	return nil
}

// setupControlProtocol starts the control protocol and runs the initialize
// handshake. The handshake is unconditional so the agents map always
// reaches the CLI on every connection. ctx bounds only the handshake.
func (t *Transport) setupControlProtocol(ctx context.Context) error {
	if err := t.protocol.Start(t.ctx); err != nil {
		return fmt.Errorf("failed to start control protocol: %w", err)
	}

	// Watch for stdout closure so a CLI that dies before responding to
	// initialize unblocks the handshake instead of waiting for timeout.
	// Capture channel/protocol locally so the goroutine doesn't race with a
	// subsequent Connect() reassigning t.stdoutDone or t.protocol.
	//
	// initDone closes AFTER Initialize returns (below), so the watcher
	// exits cleanly on the success path. Any late stdoutDone fires from
	// after that are absorbed by HandleControlInitErr's post-init guard.
	initDone := make(chan struct{})
	stdoutDone := t.stdoutDone
	protocol := t.protocol
	go func() {
		select {
		case <-initDone:
		case <-stdoutDone:
			protocol.HandleControlInitErr(fmt.Errorf("CLI process exited before initialize handshake completed"))
		}
	}()

	_, err := t.protocol.Initialize(ctx)
	close(initDone)
	if err != nil {
		return fmt.Errorf("failed to initialize control protocol: %w", err)
	}

	return nil
}

// SendMessage sends a message to the CLI subprocess via stdin.
func (t *Transport) SendMessage(ctx context.Context, message shared.StreamMessage) error {
	stdin, err := t.openStdin()
	if err != nil {
		return err
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

	// Write outside t.mu so Close is never blocked behind a full stdin pipe.
	if _, err := stdin.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}

	return nil
}

// openStdin returns the stdin writer of a connected, live transport.
func (t *Transport) openStdin() (*stdinWriter, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if !t.connected || t.stdin == nil || t.stdin.isClosed() {
		return nil, fmt.Errorf("transport not connected or stdin closed")
	}
	if t.processExitedLocked() {
		return nil, shared.NewConnectionError(
			fmt.Sprintf("cannot write to terminated CLI process (%s)", t.cmd.ProcessState), nil)
	}
	return t.stdin, nil
}

// EndInput closes the stdin write side, signaling end-of-input to the CLI
// while leaving stdout/stderr open so messages can still be received.
// Idempotent: subsequent calls return nil. The context is unused because
// os.File.Close is a fast non-cancellable syscall.
func (t *Transport) EndInput(_ context.Context) error {
	t.mu.RLock()
	stdin := t.stdin
	t.mu.RUnlock()

	if stdin == nil {
		return nil
	}
	if err := stdin.EndInput(); err != nil {
		return fmt.Errorf("failed to close stdin: %w", err)
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

// Interrupt asks the CLI to stop the current turn with an interrupt control
// request. The CLI stays alive for the next query (Python: Query.interrupt).
// A signal would kill the CLI, and Windows has no SIGINT.
func (t *Transport) Interrupt(ctx context.Context) error {
	protocol, err := t.connectedProtocol()
	if err != nil {
		return err
	}
	return protocol.Interrupt(ctx)
}

// connectedProtocol returns the control protocol of a connected transport.
// Callers use it outside t.mu, so a pending control request never blocks Close.
func (t *Transport) connectedProtocol() (*control.Protocol, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if !t.connected {
		return nil, fmt.Errorf("transport not connected")
	}
	if t.protocol == nil {
		return nil, fmt.Errorf("internal error: transport connected but control protocol is nil")
	}
	return t.protocol, nil
}

// Close ends the CLI the way Python close() does: close stdin, wait up to 5s
// for a clean exit, then SIGTERM, wait up to 5s, then SIGKILL. A CLI that
// exits on stdin EOF (the normal case) ends without a signal.
func (t *Transport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.connected {
		return nil // Already closed
	}

	t.connected = false
	return t.teardownLocked()
}

// teardownLocked closes the control protocol and stdin, terminates the
// subprocess, then cancels the context, waits for I/O goroutines, and clears
// resources. Caller must hold t.mu. Safe to call from any post-Start state -
// each field is nil-checked. Used by both Close and Connect's error path.
func (t *Transport) teardownLocked() error {
	// Close control protocol first: it cancels in-flight handlers.
	if t.protocol != nil {
		_ = t.protocol.Close()
		t.protocol = nil
	}
	if t.protocolAdapter != nil {
		_ = t.protocolAdapter.Close()
		t.protocolAdapter = nil
	}

	// Not cleared: handleStdout keeps reading it until it returns.
	if t.closing != nil {
		close(t.closing)
	}
	if t.stdin != nil {
		// Does not wait for a blocked write, so Close never hangs on a full pipe.
		t.stdin.Close()
		t.stdin = nil
	}

	var err error
	if t.cmd != nil && t.cmd.Process != nil {
		err = t.terminateProcess()
	}

	// Cancel after the process ends, so stdout is drained during the grace period.
	if t.cancel != nil {
		t.cancel()
	}

	// Wait for goroutines to finish with timeout.
	done := make(chan struct{})
	go func() {
		t.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(terminationTimeoutSeconds * time.Second):
		// cleanup closes the pipes below, which ends the readers.
	}

	t.cleanup()
	return err
}
