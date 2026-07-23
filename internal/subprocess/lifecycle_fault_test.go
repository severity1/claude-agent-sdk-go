//go:build !windows

package subprocess

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/severity1/claude-agent-sdk-go/internal/control"
	"github.com/severity1/claude-agent-sdk-go/internal/shared"
)

const lifecycleFailureDeadline = 500 * time.Millisecond

func TestConnectFailureClassifiesAndReapsProcess(t *testing.T) {
	tests := []struct {
		name          string
		body          string
		errorContains string
	}{
		{
			name:          "process exits before initialize response",
			body:          "exit 17",
			errorContains: "process exited before control response",
		},
		{
			name:          "stdout closes before initialize response",
			body:          "exec 1>&-\nwhile :; do :; done",
			errorContains: "stdout EOF before control response",
		},
		{
			name:          "invalid JSONL fails immediately",
			body:          "printf '%s\\n' 'not-json'\nwhile :; do :; done",
			errorContains: "invalid JSONL control record",
		},
		{
			name: "mismatched request id fails immediately",
			body: "IFS= read -r _\n" +
				"printf '%s\\n' '{\"type\":\"control_response\",\"response\":{\"subtype\":\"success\",\"request_id\":\"req_wrong\",\"response\":{}}}'\n" +
				"while :; do :; done",
			errorContains: "control response request ID mismatch",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cliPath, pidFile := writeLifecycleCLI(t, tc.body)
			transport := newLifecycleTransport(cliPath, pidFile)
			ctx, cancel := context.WithTimeout(context.Background(), lifecycleFailureDeadline)
			defer cancel()

			started := time.Now()
			err := transport.Connect(ctx)
			duration := time.Since(started)
			pid := readLifecyclePID(t, pidFile)
			defer forceReapLifecycleProcess(pid)

			if err == nil {
				t.Fatal("Connect() error = nil, want classified initialization failure")
			}
			if !strings.Contains(err.Error(), tc.errorContains) {
				t.Fatalf("Connect() error = %q, want substring %q", err, tc.errorContains)
			}
			if duration >= lifecycleFailureDeadline {
				t.Fatalf("Connect() returned after %s; want failure before context deadline %s", duration, lifecycleFailureDeadline)
			}

			assertLifecycleProcessReaped(t, pid)
		})
	}
}

func TestConnectCancellationReapsHungInitialize(t *testing.T) {
	cliPath, pidFile := writeLifecycleCLI(t, "IFS= read -r _\nwhile :; do :; done")
	transport := newLifecycleTransport(cliPath, pidFile)
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	err := transport.Connect(ctx)
	if err == nil {
		t.Fatal("Connect() error = nil, want context cancellation")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Connect() error = %v, want context deadline exceeded in chain", err)
	}

	pid := readLifecyclePID(t, pidFile)
	defer forceReapLifecycleProcess(pid)
	assertLifecycleProcessReaped(t, pid)
}

func TestConcurrentCancelAndCloseReapsProcess(t *testing.T) {
	cliPath, pidFile := writeLifecycleCLI(t, `
IFS= read -r request
request_id=$(printf '%s' "$request" | sed -n 's/.*"request_id":"\([^"]*\)".*/\1/p')
printf '{"type":"control_response","response":{"subtype":"success","request_id":"%s","response":{"supported_commands":[]}}}\n' "$request_id"
while IFS= read -r _; do :; done
`)
	transport := newLifecycleTransport(cliPath, pidFile)
	ctx, cancel := context.WithCancel(context.Background())
	if err := transport.Connect(ctx); err != nil {
		cancel()
		t.Fatalf("Connect() error = %v, want success", err)
	}
	pid := readLifecyclePID(t, pidFile)
	defer forceReapLifecycleProcess(pid)

	cancel()
	closeDone := make(chan error, 1)
	go func() { closeDone <- transport.Close() }()
	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Close() did not finish after context cancellation")
	}

	assertLifecycleProcessReaped(t, pid)
}

func newLifecycleTransport(cliPath, pidFile string) *Transport {
	return New(cliPath, &shared.Options{
		ExtraEnv: map[string]string{"SDK_TEST_PID_FILE": pidFile},
		Hooks: map[control.HookEvent][]control.HookMatcher{
			control.HookEventPreToolUse: nil,
		},
	}, false, "sdk-go-client")
}

func writeLifecycleCLI(t *testing.T, body string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "pid")
	path := filepath.Join(dir, "claude")
	script := fmt.Sprintf(`#!/bin/sh
if [ "${1:-}" = "-v" ]; then
  printf '3.0.0\n'
  exit 0
fi
printf '%%s\n' "$$" > "$SDK_TEST_PID_FILE"
%s
`, body)
	if err := os.WriteFile(path, []byte(script), 0o600); err != nil {
		t.Fatalf("write fake CLI: %v", err)
	}
	// #nosec G302 -- executable test-only fake CLI created under t.TempDir.
	if err := os.Chmod(path, 0o700); err != nil {
		t.Fatalf("chmod fake CLI: %v", err)
	}
	return path, pidFile
}

func readLifecyclePID(t *testing.T, pidFile string) int {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		// #nosec G304 -- pidFile is created under t.TempDir by writeLifecycleCLI.
		data, err := os.ReadFile(filepath.Clean(pidFile))
		if err == nil {
			pid, parseErr := strconv.Atoi(strings.TrimSpace(string(data)))
			if parseErr != nil {
				t.Fatalf("parse fake CLI PID: %v", parseErr)
			}
			return pid
		}
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("read fake CLI PID: %v", err)
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("fake CLI did not write PID file %s", pidFile)
	return 0
}

func assertLifecycleProcessReaped(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(300 * time.Millisecond)
	procPath := filepath.Join("/proc", strconv.Itoa(pid))
	for time.Now().Before(deadline) {
		_, err := os.Stat(procPath)
		if errors.Is(err, os.ErrNotExist) {
			return
		}
		if err != nil {
			t.Fatalf("stat %s: %v", procPath, err)
		}
		time.Sleep(5 * time.Millisecond)
	}
	// #nosec G304 -- procPath is built from a PID emitted by the test child.
	state, _ := os.ReadFile(filepath.Clean(filepath.Join(procPath, "status")))
	t.Fatalf("fake CLI pid %d was not reaped; status:\n%s", pid, boundedLifecycleStatus(state))
}

func boundedLifecycleStatus(status []byte) string {
	var kept []string
	for _, line := range strings.Split(string(status), "\n") {
		if strings.HasPrefix(line, "Name:") || strings.HasPrefix(line, "State:") || strings.HasPrefix(line, "PPid:") {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n")
}

func forceReapLifecycleProcess(pid int) {
	_ = syscall.Kill(pid, syscall.SIGKILL)
	var status syscall.WaitStatus
	for {
		_, err := syscall.Wait4(pid, &status, 0, nil)
		if err == nil || errors.Is(err, syscall.ECHILD) {
			return
		}
		if !errors.Is(err, syscall.EINTR) {
			return
		}
	}
}
