package subprocess

import (
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/severity1/claude-agent-sdk-go/internal/shared"
)

const processExitArbitrationDelay = 25 * time.Millisecond

type processResult struct {
	err      error
	exitCode int
}

// startProcessWaiter establishes the sole cmd.Wait owner for this process.
func (t *Transport) startProcessWaiter() {
	done := make(chan struct{})
	cmd := t.cmd
	t.processMu.Lock()
	t.processDone = done
	t.processResult = processResult{}
	t.processMu.Unlock()

	go func() {
		err := cmd.Wait()
		exitCode := 0
		if cmd.ProcessState != nil {
			exitCode = cmd.ProcessState.ExitCode()
		}
		t.processMu.Lock()
		t.processResult = processResult{err: err, exitCode: exitCode}
		t.processMu.Unlock()
		close(done)
	}()
}

func (t *Transport) processExitBeforeControlResponseError() error {
	t.processMu.RLock()
	result := t.processResult
	t.processMu.RUnlock()
	if result.err == nil && result.exitCode == 0 {
		return fmt.Errorf("process exited before control response")
	}
	return shared.NewProcessError("process exited before control response", result.exitCode, "")
}

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
		strings.Contains(errStr, "signal: killed")
}

// terminateProcess implements the 5-second SIGTERM -> SIGKILL sequence. It
// never calls cmd.Wait; startProcessWaiter is the sole Wait owner.
func (t *Transport) terminateProcess() error {
	if t.cmd == nil || t.cmd.Process == nil || t.processDone == nil {
		return nil
	}

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
		return t.waitForProcessReap()
	}

	select {
	case <-t.processDone:
		return nil
	case <-time.After(terminationTimeoutSeconds * time.Second):
		if killErr := t.cmd.Process.Kill(); killErr != nil && !isProcessAlreadyFinishedError(killErr) {
			return killErr
		}
		return t.waitForProcessReap()
	}
}

func (t *Transport) waitForProcessReap() error {
	select {
	case <-t.processDone:
		return nil
	case <-time.After(terminationTimeoutSeconds * time.Second):
		return fmt.Errorf("timed out waiting for process reap after SIGKILL")
	}
}

// closeStartedProcess handles failed Connect paths, where connected was never
// published but the child still has to be terminated and reaped.
func (t *Transport) closeStartedProcess() error {
	if t.protocol != nil {
		_ = t.protocol.Close()
	}
	if t.protocolAdapter != nil {
		_ = t.protocolAdapter.Close()
	}
	if t.stdin != nil {
		_ = t.stdin.Close()
		t.stdin = nil
	}
	err := t.terminateProcess()
	if t.cancel != nil {
		t.cancel()
	}
	t.wg.Wait()
	t.cleanup()
	return err
}

// cleanup cleans up all resources. Started processes must be reaped before it
// is called; this method never owns process termination.
func (t *Transport) cleanup() {
	if t.cancel != nil {
		t.cancel()
	}
	if t.protocol != nil {
		_ = t.protocol.Close()
		t.protocol = nil
	}
	if t.protocolAdapter != nil {
		_ = t.protocolAdapter.Close()
		t.protocolAdapter = nil
	}
	if t.stdin != nil {
		_ = t.stdin.Close()
		t.stdin = nil
	}
	if t.stdout != nil {
		_ = t.stdout.Close()
		t.stdout = nil
	}
	if t.stderrPipe != nil {
		_ = t.stderrPipe.Close()
		t.stderrPipe = nil
	}
	if t.stderr != nil {
		_ = t.stderr.Close()
		_ = os.Remove(t.stderr.Name())
		t.stderr = nil
	}
	if t.mcpConfigFile != nil {
		_ = t.mcpConfigFile.Close()
		_ = os.Remove(t.mcpConfigFile.Name())
		t.mcpConfigFile = nil
	}

	t.cmd = nil
	t.ctx = nil
	t.cancel = nil
	t.connected = false
	t.initFailure = nil
	t.processMu.Lock()
	t.processDone = nil
	t.processResult = processResult{}
	t.processMu.Unlock()
}
