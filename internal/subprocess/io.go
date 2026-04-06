package subprocess

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/severity1/claude-agent-sdk-go/internal/shared"
)

// handleStdout processes stdout in a separate goroutine
func (t *Transport) handleStdout() {
	defer t.wg.Done()
	defer close(t.msgChan)
	defer close(t.errChan)
	defer t.validator.MarkStreamEnd() // Mark stream end for validation

	scanner := bufio.NewScanner(t.stdout)

	// Increase scanner buffer to handle large tool results (files, etc.)
	// Default bufio.Scanner has MaxScanTokenSize of 64KB which is insufficient
	// for tool results containing large files. We use 1MB to match parser's
	// MaxBufferSize and handle files up to ~900KB after JSON encoding overhead.
	const maxScanTokenSize = 1024 * 1024 // 1MB
	buf := make([]byte, maxScanTokenSize)
	scanner.Buffer(buf, maxScanTokenSize)

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

			// if this is a ResultMessage and we're not connected yet, it may be an initial connection message that we should ignore for stream validation
			if resultMsg, ok := msg.(*shared.ResultMessage); ok && !t.connected && resultMsg.IsError {
				// Route control messages to the controlProtocol for request/response correlation
				if t.protocol != nil {
					// HandleIncomingMessage routes control responses to pending requests
					// and forwards non-control messages to the controlProtocol's message stream
					t.protocol.HandleControlInitErr(fmt.Errorf("control protocol not yet initialized, %v", resultMsg.Errors))
				}
			}

			// Check if this is a control message that should be routed to the protocol
			if rawCtrl, ok := msg.(*shared.RawControlMessage); ok {
				// Route control messages to the protocol for request/response correlation
				if t.protocol != nil {
					// HandleIncomingMessage routes control responses to pending requests
					// and forwards non-control messages to the protocol's message stream
					_ = t.protocol.HandleIncomingMessage(t.ctx, rawCtrl.Data)
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
// Matches Python SDK behavior: line-by-line, strips trailing whitespace,
// skips empty lines, silently ignores all errors.
func (t *Transport) handleStderrCallback() {
	defer t.wg.Done()

	scanner := bufio.NewScanner(t.stderrPipe)

	for scanner.Scan() {
		select {
		case <-t.ctx.Done():
			return
		default:
		}

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

// setupStderr configures stderr handling based on options.
// Precedence: StderrCallback > DebugWriter > temp file (default).
// This extracts stderr setup logic from Connect to reduce cyclomatic complexity.
func (t *Transport) setupStderr() error {
	switch {
	case t.options != nil && t.options.StderrCallback != nil:
		// Create pipe for callback-based stderr handling
		stderrPipe, err := t.cmd.StderrPipe()
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

// setupIoPipes configures stdin, stdout, and stderr pipes for the subprocess.
// For streaming mode, creates a stdin pipe for sending messages. Always creates
// stdout pipe for receiving responses. Stderr is configured via setupStderr.
func (t *Transport) setupIoPipes() error {
	var err error
	if t.promptArg == nil {
		// Only create stdin pipe if we need to send messages via stdin
		t.stdin, err = t.cmd.StdinPipe()
		if err != nil {
			return fmt.Errorf("failed to create stdin pipe: %w", err)
		}
	}

	t.stdout, err = t.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	// Handle stderr configuration
	if err := t.setupStderr(); err != nil {
		return err
	}

	return nil
}
