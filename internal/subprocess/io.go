package subprocess

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/severity1/claude-agent-sdk-go/internal/control"
	"github.com/severity1/claude-agent-sdk-go/internal/parser"
	"github.com/severity1/claude-agent-sdk-go/internal/shared"
)

// handleStdout processes stdout in a separate goroutine. protocol is passed in
// because teardownLocked clears t.protocol while this goroutine may still run.
func (t *Transport) handleStdout(protocol *control.Protocol) {
	defer t.wg.Done()
	defer close(t.msgChan)
	defer close(t.errChan)
	defer t.validator.MarkStreamEnd() // Mark stream end for validation
	// Capture channel locally so reassignment on a subsequent Connect()
	// doesn't race with this defer.
	stdoutDone := t.stdoutDone
	defer func() {
		if stdoutDone != nil {
			close(stdoutDone)
		}
	}()

	scanner := bufio.NewScanner(t.stdout)

	// Scanner token size must match the parser's buffer limit so lines aren't
	// truncated before parsing. Default is 64KB; respect MaxBufferSize if set.
	scanTokenSize := parser.MaxBufferSize
	if t.options != nil && t.options.MaxBufferSize != nil {
		scanTokenSize = *t.options.MaxBufferSize
	}
	buf := make([]byte, scanTokenSize)
	scanner.Buffer(buf, scanTokenSize)

	for scanner.Scan() {
		select {
		case <-t.ctx.Done():
			return
		default:
		}

		line := scanner.Text()
		if line == "" {
			continue
		}

		// Parse line with the parser
		messages, err := t.parser.ProcessLine(line)
		if err != nil {
			select {
			case t.errChan <- err:
			case <-t.ctx.Done():
				return
			}
			continue
		}

		// Send parsed messages and track for validation
		for _, msg := range messages {
			if msg == nil {
				continue
			}

			// If this is an error ResultMessage before we're fully connected,
			// it means the CLI failed during init (e.g., invalid session ID).
			// Route the error to the control protocol to unblock Initialize().
			routeInitError(protocol, msg)

			// Check if this is a control message that should be routed to the protocol
			if rawCtrl, ok := msg.(*shared.RawControlMessage); ok {
				// Route control messages to the protocol for request/response correlation
				if protocol != nil {
					// HandleIncomingMessage routes control responses to pending requests
					// and forwards non-control messages to the protocol's message stream
					_ = protocol.HandleIncomingMessage(t.ctx, rawCtrl.Data)
				}
				// Don't send control messages to msgChan - they're internal to the protocol
				continue
			}

			// Track regular message for stream validation
			t.validator.TrackMessage(msg)

			select {
			case t.msgChan <- msg:
			case <-t.ctx.Done():
				return
			}
		}
	}

	if err := scanner.Err(); err != nil {
		select {
		case t.errChan <- fmt.Errorf("stdout scanner error: %w", err):
		case <-t.ctx.Done():
		}
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
	var err error
	t.stdin, err = t.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	t.stdout, err = t.newChildOutputPipe(&t.cmd.Stdout)
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	if err := t.setupStderr(); err != nil {
		return err
	}

	return nil
}
