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

// terminateProcess runs after stdin is closed (Python close()): wait 5s for
// a clean exit, SIGTERM, wait 5s, SIGKILL, wait 5s. It uses fixed timers and
// no ctx, so a cancelled caller still gets every step. It never calls
// cmd.Wait; startProcessWaiter owns that call. On Windows SIGTERM fails and
// the sequence goes to Kill (Python: terminate() is TerminateProcess there).
func (t *Transport) terminateProcess() error {
	if t.cmd == nil || t.cmd.Process == nil || t.processDone == nil {
		return nil
	}

	// stdout EOF alone is not proof of exit; only processDone is.
	if t.waitProcessDone(terminationTimeoutSeconds * time.Second) {
		return nil
	}

	err := t.cmd.Process.Signal(syscall.SIGTERM)
	if err == nil || isProcessAlreadyFinishedError(err) {
		if t.waitProcessDone(terminationTimeoutSeconds * time.Second) {
			return nil
		}
	}

	if killErr := t.cmd.Process.Kill(); killErr != nil && !isProcessAlreadyFinishedError(killErr) {
		return killErr
	}
	// Bounded: startProcessWaiter still reaps the process if this times out.
	t.waitProcessDone(terminationTimeoutSeconds * time.Second)
	return nil
}

// waitProcessDone waits up to d for the process to exit and reports whether it did.
func (t *Transport) waitProcessDone(d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-t.processDone:
		return true
	case <-timer.C:
		return false
	}
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
