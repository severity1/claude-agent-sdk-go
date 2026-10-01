package subprocess

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/severity1/claude-agent-sdk-go/internal/shared"
)

// minGrace is the lower bound a test accepts for a 5 second shutdown wait.
const minGrace = 4500 * time.Millisecond

// TestCloseWaitsForExitAfterEOF verifies that Close lets a CLI that still
// writes output after stdin EOF exit on its own, without a signal (Python #625).
func TestCloseWaitsForExitAfterEOF(t *testing.T) {
	ctx, cancel := setupTransportTestContext(t, 30*time.Second)
	defer cancel()

	eventLog := setMockEventLog(t)
	transport := setupTransportForTest(t, newTransportMockCLIMode(t, mockModeSlowExitAfterEOF))
	connectTransportSafely(ctx, t, transport)
	cmd, processDone := transport.cmd, transport.processDone

	start := time.Now()
	assertNoTransportError(t, transport.Close())
	duration := time.Since(start)

	<-processDone
	events := readMockEvents(t, eventLog)
	if _, ok := events[mockEventSIGTERM]; ok {
		t.Errorf("CLI received SIGTERM during a clean shutdown, events: %v", events)
	}
	if _, ok := events[mockEventExit]; !ok {
		t.Errorf("CLI did not reach its own exit, events: %v", events)
	}
	if code := cmd.ProcessState.ExitCode(); code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if duration < slowExitDelay || duration > 4*time.Second {
		t.Errorf("Close took %v, want between %v and 4s", duration, slowExitDelay)
	}
}

// TestCloseSIGTERMGraceBeforeKill verifies the shutdown order for a CLI that
// ignores stdin EOF and SIGTERM: 5s grace, SIGTERM, 5s grace, SIGKILL.
func TestCloseSIGTERMGraceBeforeKill(t *testing.T) {
	ctx, cancel := setupTransportTestContext(t, 30*time.Second)
	defer cancel()

	eventLog := setMockEventLog(t)
	transport := setupTransportForTest(t, newTransportMockCLIMode(t, mockModeIgnoreSIGTERM))
	connectTransportSafely(ctx, t, transport)
	exitedAt := watchProcessExit(transport.processDone)

	start := time.Now()
	assertNoTransportError(t, transport.Close())
	if duration := time.Since(start); duration > 13*time.Second {
		t.Errorf("Close took %v, want under 13s", duration)
	}

	exitMs := (<-exitedAt).UnixNano() / int64(time.Millisecond)
	events := readMockEvents(t, eventLog)
	eofMs, ok := events[mockEventEOF]
	if !ok {
		t.Fatalf("CLI did not see stdin EOF, events: %v", events)
		return
	}
	if runtime.GOOS == windowsOS {
		// Windows has no SIGTERM delivery: the kill follows the EOF grace.
		assertGap(t, "EOF to kill", eofMs, exitMs, minGrace)
		return
	}
	termMs, ok := events[mockEventSIGTERM]
	if !ok {
		t.Fatalf("CLI did not receive SIGTERM, events: %v", events)
		return
	}
	assertGap(t, "EOF to SIGTERM", eofMs, termMs, minGrace)
	assertGap(t, "SIGTERM to kill", termMs, exitMs, minGrace)
}

// TestConnectContextCancelKeepsCLIAlive verifies that the Connect context
// bounds only connection setup, like net.Dialer.DialContext.
func TestConnectContextCancelKeepsCLIAlive(t *testing.T) {
	ctx, cancel := setupTransportTestContext(t, 30*time.Second)
	defer cancel()

	transport := setupTransportForTest(t, newTransportMockCLIWithControlProtocol(t))
	defer disconnectTransportSafely(t, transport)

	connectCtx, connectCancel := context.WithCancel(ctx)
	connectTransportSafely(connectCtx, t, transport)
	processDone := transport.processDone
	connectCancel()
	time.Sleep(300 * time.Millisecond)

	select {
	case <-processDone:
		t.Fatalf("CLI exited after the Connect context was cancelled: %v", transport.cmd.ProcessState)
		return
	default:
	}
	assertTransportConnected(t, transport, true)
	assertNoTransportError(t, transport.SendMessage(ctx, shared.StreamMessage{Type: "user"}))
}

// TestCloseUnblocksPendingWrite verifies that Close does not wait for a stdin
// write that blocks because the CLI stopped reading.
func TestCloseUnblocksPendingWrite(t *testing.T) {
	ctx, cancel := setupTransportTestContext(t, 30*time.Second)
	defer cancel()

	transport := setupTransportForTest(t, newTransportMockCLIMode(t, mockModeStopReading))
	connectTransportSafely(ctx, t, transport)
	cmd := transport.cmd

	writerDone := make(chan error, 1)
	go func() {
		big := shared.StreamMessage{Type: "user", Message: strings.Repeat("x", 64*1024)}
		for {
			if err := transport.SendMessage(ctx, big); err != nil {
				writerDone <- err
				return
			}
		}
	}()
	// The pipe buffer fills at once; give the writer time to block.
	time.Sleep(time.Second)

	closeDone := make(chan error, 1)
	start := time.Now()
	go func() { closeDone <- transport.Close() }()
	select {
	case err := <-closeDone:
		assertNoTransportError(t, err)
		if duration := time.Since(start); duration > 13*time.Second {
			t.Errorf("Close took %v, want under 13s", duration)
		}
	case <-time.After(20 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("Close did not return while a stdin write was blocked")
		return
	}

	select {
	case err := <-writerDone:
		if err == nil {
			t.Error("blocked writer returned no error after Close")
		}
	case <-time.After(5 * time.Second):
		t.Error("blocked writer did not return after Close")
	}
}

// setMockEventLog points the mock CLI event log at a file in a temp dir.
func setMockEventLog(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "events.log")
	t.Setenv(envMockEventLog, path)
	return path
}

// readMockEvents returns the first unix-ms timestamp of each logged event.
func readMockEvents(t *testing.T, path string) map[string]int64 {
	t.Helper()
	data, err := os.ReadFile(path) //nolint:gosec // path is constructed from test temp dir
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("read event log: %v", err)
	}
	events := make(map[string]int64)
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		ms, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			continue
		}
		if _, seen := events[fields[0]]; !seen {
			events[fields[0]] = ms
		}
	}
	return events
}

// watchProcessExit records when processDone closes.
func watchProcessExit(processDone <-chan struct{}) <-chan time.Time {
	exitedAt := make(chan time.Time, 1)
	go func() {
		<-processDone
		exitedAt <- time.Now()
	}()
	return exitedAt
}

// assertGap fails when the time from fromMs to toMs is shorter than minGap.
func assertGap(t *testing.T, name string, fromMs, toMs int64, minGap time.Duration) {
	t.Helper()
	if gap := time.Duration(toMs-fromMs) * time.Millisecond; gap < minGap {
		t.Errorf("%s gap = %v, want at least %v", name, gap, minGap)
	}
}
