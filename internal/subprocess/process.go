package subprocess

import (
	"os"
	"strings"
	"syscall"
	"time"
)

// isProcessAlreadyFinishedError checks if an error indicates the process has already terminated.
// This follows the Python SDK pattern of suppressing "process not found" type errors.
func isProcessAlreadyFinishedError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "process already finished") ||
		strings.Contains(errStr, "process already released") ||
		strings.Contains(errStr, "no child processes") ||
		strings.Contains(errStr, "signal: killed") ||
		// Windows returns this when TerminateProcess targets an already-exited process.
		strings.Contains(errStr, "TerminateProcess: Access is denied")
}

// startProcessWaiter makes one goroutine the sole cmd.Wait caller. Go's
// exec.Cmd forbids a second Wait, so other code waits on processDone instead
// (the Go equivalent of Python's idempotent process.wait()).
func (t *Transport) startProcessWaiter() {
	done := make(chan struct{})
	cmd := t.cmd
	t.processDone = done
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
}

// terminateProcess implements the 5-second SIGTERM -> SIGKILL sequence.
// It never calls cmd.Wait; startProcessWaiter owns that call.
func (t *Transport) terminateProcess() error {
	if t.cmd == nil || t.cmd.Process == nil || t.processDone == nil {
		return nil
	}

	// Already exited and reaped. stdout EOF alone is not proof of exit.
	select {
	case <-t.processDone:
		return nil
	default:
	}

	if err := t.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		if !isProcessAlreadyFinishedError(err) {
			if killErr := t.cmd.Process.Kill(); killErr != nil && !isProcessAlreadyFinishedError(killErr) {
				return killErr
			}
		}
		<-t.processDone
		return nil
	}

	select {
	case <-t.processDone:
		return nil
	case <-time.After(terminationTimeoutSeconds * time.Second):
	case <-t.ctx.Done():
	}
	if killErr := t.cmd.Process.Kill(); killErr != nil && !isProcessAlreadyFinishedError(killErr) {
		return killErr
	}
	<-t.processDone
	return nil
}

// cleanup cleans up all resources
func (t *Transport) cleanup() {
	t.closeChildPipeEnds()

	if t.stdout != nil {
		_ = t.stdout.Close()
		t.stdout = nil
	}

	if t.stderrPipe != nil {
		_ = t.stderrPipe.Close()
		t.stderrPipe = nil
	}

	if t.stderr != nil {
		// Graceful cleanup matching Python SDK pattern
		// Python: except Exception: pass
		_ = t.stderr.Close()
		_ = os.Remove(t.stderr.Name()) // Ignore cleanup errors
		t.stderr = nil
	}

	if t.mcpConfigFile != nil {
		// Clean up temporary MCP config file
		_ = t.mcpConfigFile.Close()
		_ = os.Remove(t.mcpConfigFile.Name()) // Ignore cleanup errors
		t.mcpConfigFile = nil
	}

	// Reset state
	t.cmd = nil
	t.processDone = nil
}
