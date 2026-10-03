package subprocess

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/severity1/claude-agent-sdk-go/internal/control"
	"github.com/severity1/claude-agent-sdk-go/internal/parser"
	"github.com/severity1/claude-agent-sdk-go/internal/shared"
)

// childProcess is the CLI process that handleStdout watches. Captured at
// Connect so cleanup clearing t.cmd cannot race with the reader.
type childProcess struct {
	cmd  *exec.Cmd
	done <-chan struct{} // closed after the sole cmd.Wait returns
}

// exitError waits for the CLI to exit after stdout EOF. It returns a
// ProcessError for a non-zero exit, and nil for a clean exit or when the SDK
// itself shut the CLI down (Python: ProcessError after read_messages ends).
// The caller drops the error when the SDK is closing the CLI.
func (c childProcess) exitError(ctx context.Context, lastErrorResult string) error {
	if c.cmd == nil || c.done == nil {
		return nil
	}
	select {
	case <-c.done:
	case <-ctx.Done():
		return nil
	}
	if ctx.Err() != nil || c.cmd.ProcessState == nil || c.cmd.ProcessState.Success() {
		return nil
	}
	state := c.cmd.ProcessState
	message := fmt.Sprintf("Claude Code process exited unexpectedly (%s)", state)
	if lastErrorResult != "" {
		// The CLI exits non-zero on purpose after an error result; that result
		// is the actionable error (Python #918).
		message = "Claude Code returned an error result: " + lastErrorResult
	}
	return shared.NewProcessError(message, state.ExitCode(), "Check stderr output for details")
}

// handleStdout processes stdout in a separate goroutine. protocol and child
// are passed in because teardownLocked clears t.protocol and t.cmd while this
// goroutine may still run.
func (t *Transport) handleStdout(protocol *control.Protocol, child childProcess) {
	defer t.wg.Done()
	defer close(t.msgChan)
	defer close(t.errChan)
	defer t.validator.MarkStreamEnd() // Mark stream end for validation
	// Capture channel locally so reassignment on a subsequent Connect()
	// doesn't race with this goroutine. It closes at stdout EOF, before the
	// wait for the process exit, so the init watcher sees EOF at once.
	stdoutDone := t.stdoutDone
	signalStdoutDone := func() {
		if stdoutDone != nil {
			close(stdoutDone)
			stdoutDone = nil
		}
	}
	defer signalStdoutDone()

	scanner := bufio.NewScanner(t.stdout)

	maxLineSize := parser.MaxBufferSize
	if t.options != nil && t.options.MaxBufferSize != nil {
		maxLineSize = *t.options.MaxBufferSize
	}
	// The scanner buffer also holds the newline, so +1 lets a line of exactly
	// the limit through (Python rejects only a longer line).
	scanner.Buffer(make([]byte, maxLineSize+1), maxLineSize+1)

	var lastErrorResult string
	for scanner.Scan() {
		if t.ctx.Err() != nil {
			return
		}
		if !t.processStdoutLine(protocol, scanner.Text(), &lastErrorResult) {
			return
		}
	}

	signalStdoutDone()
	if err := scanner.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			t.sendStreamError(parser.NewBufferOverflowError(maxLineSize, err))
			return
		}
		t.sendStreamError(fmt.Errorf("stdout scanner error: %w", err))
		return
	}
	// A signal from our own teardown is not a CLI failure.
	if err := child.exitError(t.ctx, lastErrorResult); err != nil && !t.isClosing() {
		t.sendStreamError(err)
	}
}

// processStdoutLine parses one stdout line and routes its messages. It
// returns false when the transport context is done.
func (t *Transport) processStdoutLine(protocol *control.Protocol, line string, lastErrorResult *string) bool {
	if line == "" {
		return true
	}

	messages, err := t.parser.ProcessLine(line)
	if err != nil {
		return t.sendStreamError(err)
	}

	for _, msg := range messages {
		if msg == nil {
			continue
		}

		// An error ResultMessage before initialize completes means the CLI
		// failed during init (e.g. invalid session ID); unblock Initialize().
		routeInitError(protocol, msg)

		if rawCtrl, ok := msg.(*shared.RawControlMessage); ok {
			if protocol != nil && !t.isClosing() {
				// Control requests run on their own goroutine so a slow callback
				// cannot stall this reader; responses route inline.
				_ = protocol.HandleIncomingMessageAsync(t.ctx, rawCtrl.Data)
			}
			continue
		}

		*lastErrorResult = trackErrorResult(*lastErrorResult, msg)
		t.validator.TrackMessage(msg)

		select {
		case t.msgChan <- msg:
		case <-t.closing:
			// Close is in progress and nobody reads; keep draining stdout.
		case <-t.ctx.Done():
			return false
		}
	}
	return true
}

// trackErrorResult returns the error text of the latest error result, or ""
// once the conversation moves on (Python #918 reset rule).
func trackErrorResult(current string, msg shared.Message) string {
	switch m := msg.(type) {
	case *shared.ResultMessage:
		if !m.IsError {
			return ""
		}
		if len(m.Errors) > 0 {
			return strings.Join(m.Errors, "; ")
		}
		return m.Subtype
	case *shared.SystemMessage:
		if m.Subtype == "session_state_changed" {
			return current
		}
	}
	return ""
}

// sendStreamError delivers err to errChan, or drops it while Close runs. It
// returns false when the transport context is done.
func (t *Transport) sendStreamError(err error) bool {
	select {
	case t.errChan <- err:
		return true
	case <-t.closing:
		return true
	case <-t.ctx.Done():
		return false
	}
}

// isClosing reports whether teardown started.
func (t *Transport) isClosing() bool {
	select {
	case <-t.closing:
		return true
	default:
		return false
	}
}

// handleStderrCallback processes stderr in a separate goroutine.
// Reads line-by-line, strips trailing whitespace, skips empty lines, and
// silently ignores scanner errors.
func (t *Transport) handleStderrCallback() {
	defer t.wg.Done()

	scanner := bufio.NewScanner(t.stderrPipe)

	// Deliver every line already written, even after cancellation: Python's
	// stderr reader never drops a read line. EOF or cleanup closing the pipe
	// ends the loop.
	for scanner.Scan() {
		// Strip trailing whitespace (matches Python's rstrip())
		line := strings.TrimRight(scanner.Text(), " \t\r\n")

		// Skip empty lines (matches Python SDK behavior)
		if line == "" {
			continue
		}

		// Call the callback synchronously (matches Python SDK)
		// Recover from panics to prevent crashing the SDK
		func() {
			defer func() {
				_ = recover() // Silently ignore callback panics (matches Python's pass)
			}()
			t.options.StderrCallback(line)
		}()
	}
	// Silently ignore scanner errors (matches Python SDK's except Exception: pass)
}

// routeInitError checks if a message is an error ResultMessage arriving before
// the initialize handshake completes, and routes it to the control protocol to
// unblock Initialize().
func routeInitError(protocol *control.Protocol, msg shared.Message) {
	resultMsg, ok := msg.(*shared.ResultMessage)
	if !ok || !resultMsg.IsError || protocol == nil || protocol.IsInitialized() {
		return
	}
	protocol.HandleControlInitErr(errors.New(formatInitError(resultMsg)))
}

// formatInitError builds a meaningful error string from a ResultMessage that
// arrived during initialization. Prefers Errors, falls back to Result, then Subtype.
func formatInitError(msg *shared.ResultMessage) string {
	if len(msg.Errors) > 0 {
		return strings.Join(msg.Errors, "; ")
	}
	if msg.Result != nil && *msg.Result != "" {
		return *msg.Result
	}
	return fmt.Sprintf("initialization failed with subtype: %s", msg.Subtype)
}

// setupStderr configures stderr handling based on options.
// Precedence: StderrCallback > DebugWriter > temp file (default).
func (t *Transport) setupStderr() error {
	switch {
	case t.options != nil && t.options.StderrCallback != nil:
		// Create pipe for callback-based stderr handling
		stderrPipe, err := t.newChildOutputPipe(&t.cmd.Stderr)
		if err != nil {
			return fmt.Errorf("failed to create stderr pipe: %w", err)
		}
		t.stderrPipe = stderrPipe
	case t.options != nil && t.options.DebugWriter != nil:
		// Use custom debug writer provided by user
		t.cmd.Stderr = t.options.DebugWriter
	default:
		// Isolate stderr using temporary file to prevent deadlocks
		// This matches Python SDK pattern to avoid subprocess pipe deadlocks
		stderrFile, err := os.CreateTemp("", "claude_stderr_*.log")
		if err != nil {
			return fmt.Errorf("failed to create stderr file: %w", err)
		}
		t.stderr = stderrFile
		t.cmd.Stderr = t.stderr
	}
	return nil
}

// newChildOutputPipe connects a child output stream to an os.Pipe and returns
// the read end. Unlike cmd.StdoutPipe, cmd.Wait never closes this reader, so
// output still buffered in the pipe when the process exits is not lost.
func (t *Transport) newChildOutputPipe(childStream *io.Writer) (*os.File, error) {
	reader, writer, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	*childStream = writer
	t.childPipeEnds = append(t.childPipeEnds, writer)
	return reader, nil
}

// closeChildPipeEnds closes the parent's copies of the child-side pipe ends,
// so the readers see EOF once the child exits. Safe to call more than once.
func (t *Transport) closeChildPipeEnds() {
	for _, f := range t.childPipeEnds {
		_ = f.Close()
	}
	t.childPipeEnds = nil
}

// setupIoPipes configures stdin, stdout, and stderr pipes for the subprocess.
// Stdin is always opened so the SDK can write the initialize handshake and
// subsequent user messages. Stderr is configured via setupStderr.
func (t *Transport) setupIoPipes() error {
	stdinPipe, err := t.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}
	t.stdin = newStdinWriter(stdinPipe)

	t.stdout, err = t.newChildOutputPipe(&t.cmd.Stdout)
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	if err := t.setupStderr(); err != nil {
		return err
	}

	return nil
}
