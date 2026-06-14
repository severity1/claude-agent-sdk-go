package subprocess

import (
	"context"
	"runtime"
	"testing"
	"time"
)

// TestTransportProcessManagement tests process control and termination
func TestTransportProcessManagement(t *testing.T) {
	ctx, cancel := setupTransportTestContext(t, 15*time.Second)
	defer cancel()

	// Test 5-second termination sequence
	t.Run("five_second_termination", func(t *testing.T) {
		transport := setupTransportForTest(t, newTransportMockCLIWithOptions(t, WithLongRunning()))
		defer disconnectTransportSafely(t, transport)

		connectTransportSafely(ctx, t, transport)

		// Start timing the termination
		start := time.Now()
		err := transport.Close()
		duration := time.Since(start)

		assertNoTransportError(t, err)

		// Should complete in reasonable time (allowing buffer for 5-second sequence)
		if duration > 6*time.Second {
			t.Errorf("Termination took too long: %v", duration)
		}

		assertTransportConnected(t, transport, false)
	})

	// Test interrupt handling
	t.Run("interrupt_handling", func(t *testing.T) {
		if runtime.GOOS == windowsOS {
			t.Skip("Interrupt not supported on Windows")
		}

		transport := setupTransportForTest(t, newTransportMockCLI(t))
		defer disconnectTransportSafely(t, transport)

		connectTransportSafely(ctx, t, transport)

		err := transport.Interrupt(ctx)
		assertNoTransportError(t, err)

		// Process should still be manageable after interrupt
		assertTransportConnected(t, transport, true)
	})
}

// TestTransportTerminateProcessPaths tests uncovered terminateProcess scenarios
func TestTransportTerminateProcessPaths(t *testing.T) {
	if runtime.GOOS == windowsOS {
		t.Skip("Process termination testing requires Unix signals")
	}

	ctx, cancel := setupTransportTestContext(t, 15*time.Second)
	defer cancel()

	// Test normal termination
	t.Run("normal_termination", func(t *testing.T) {
		transport := setupTransportForTest(t, newTransportMockCLI(t))
		connectTransportSafely(ctx, t, transport)

		// Close should trigger terminateProcess
		err := transport.Close()
		assertNoTransportError(t, err)
	})

	// Test SIGTERM timeout (force SIGKILL)
	t.Run("sigterm_timeout_force_kill", func(t *testing.T) {
		transport := setupTransportForTest(t, newTransportMockCLIWithOptions(t, WithLongRunning()))
		connectTransportSafely(ctx, t, transport)

		// This transport ignores SIGTERM for 6 seconds, forcing SIGKILL
		start := time.Now()
		err := transport.Close()
		duration := time.Since(start)

		// Should complete within reasonable time after 5-second timeout
		if duration > 8*time.Second {
			t.Errorf("Termination took too long: %v", duration)
		}
		assertNoTransportError(t, err)
	})

	// Test context cancellation during termination
	t.Run("context_cancelled_during_termination", func(t *testing.T) {
		// Create a context that we can cancel
		shortCtx, shortCancel := context.WithCancel(ctx)

		transport := setupTransportForTest(t, newTransportMockCLI(t))

		// Connect with the cancellable context
		connectTransportSafely(shortCtx, t, transport)

		// Cancel the context to simulate cancellation during termination
		shortCancel()

		err := transport.Close()
		// Should not error even with cancelled context
		assertNoTransportError(t, err)
	})
}
