package subprocess

import (
	"bufio"
	"fmt"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestMain dispatches based on the CLAUDE_SDK_TEST_MOCK_MODE env var so the
// compiled test binary can act as a cross-platform mock CLI when re-spawned.
//
// This is the Go analog of Python's sys.executable -c "..." pattern and matches
// the os/exec stdlib idiom (TestHelperProcess). It replaces .bat/.sh fixtures
// so the same Go code handles control-protocol participation on every OS.
//
// When CLAUDE_SDK_TEST_MOCK_MODE is set, the binary runs the mock handler and
// exits via os.Exit before m.Run() is reached. The set of supported modes
// matches the TransportMockOption constructors and the per-test helpers below.
//
// Parallel-subtest constraint: mode selection uses t.Setenv, which is per-test
// and is incompatible with sibling subtests running in parallel under different
// modes. None of the affected tests call t.Parallel() at these sites; keep it
// that way when adding new helpers.
func TestMain(m *testing.M) {
	if mode := os.Getenv(envMockMode); mode != "" {
		runMockCLI(mode)
		// runMockCLI calls os.Exit. Defensive return for safety.
		return
	}
	os.Exit(m.Run())
}

// Env vars used to drive the in-process mock CLI. Keep these tightly scoped to
// the subprocess package so unrelated tests can't accidentally trigger them.
const (
	envMockMode = "CLAUDE_SDK_TEST_MOCK_MODE"
)

// Mock modes. Order matches the TransportMockOption constructors above so the
// mapping stays auditable.
const (
	mockModeDefault             = "default"
	mockModeLongRunning         = "long_running"
	mockModeShouldFail          = "should_fail"
	mockModeCheckEnvironment    = "check_environment"
	mockModeInvalidOutput       = "invalid_output"
	mockModeWithControlProtocol = "with_control_protocol"
	mockModeWithStderr          = "with_stderr"
	mockModeInitError           = "init_error"
)

// runMockCLI dispatches to per-mode handlers. Kept thin so gocyclo stays low.
//
// Every mode except shouldFail honors `-v` first to satisfy
// cli.CheckCLIVersion before the test exercises the streaming flow.
func runMockCLI(mode string) {
	if mode != mockModeShouldFail && handleVersionFlag() {
		os.Exit(0)
	}

	switch mode {
	case mockModeDefault:
		runMockDefault()
	case mockModeLongRunning:
		runMockLongRunning()
	case mockModeShouldFail:
		runMockShouldFail()
	case mockModeCheckEnvironment:
		runMockCheckEnvironment()
	case mockModeInvalidOutput:
		runMockInvalidOutput()
	case mockModeWithControlProtocol:
		runMockWithControlProtocol()
	case mockModeWithStderr:
		runMockWithStderr()
	case mockModeInitError:
		runMockInitError()
	default:
		fmt.Fprintf(os.Stderr, "unknown mock CLI mode: %s\n", mode)
		os.Exit(2)
	}
	os.Exit(0)
}

// handleVersionFlag prints "3.0.0" and returns true when invoked as `cli -v`,
// matching cli.CheckCLIVersion's expectations. Returns false otherwise so the
// caller can continue into stream-json mode.
func handleVersionFlag() bool {
	if len(os.Args) > 1 && os.Args[1] == "-v" {
		fmt.Println("3.0.0")
		return true
	}
	return false
}

const assistantMsg = `{"type":"assistant","content":[{"type":"text","text":"Mock response"}],"model":"claude-3"}`

// runMockDefault emits one assistant message, then loops on stdin echoing a
// control_response for every control_request so the unconditional initialize
// handshake completes.
func runMockDefault() {
	fmt.Println(assistantMsg)
	controlEchoLoop(os.Stdin, os.Stdout)
}

// runMockLongRunning blocks termination for 6s after SIGTERM to exercise the
// SIGTERM -> SIGKILL escalation. On Windows SIGTERM is delivered by os/exec
// only via TerminateProcess; signal.Notify wires SIGTERM where the runtime
// supports it (Unix), and the read-loop is what keeps the process alive long
// enough on Windows for Close() to escalate to Kill().
func runMockLongRunning() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM)

	go func() {
		<-sigChan
		// Ignore SIGTERM for 6 seconds to force the 5-second timeout path.
		time.Sleep(6 * time.Second)
		os.Exit(1)
	}()

	fmt.Println(`{"type":"assistant","content":[{"type":"text","text":"Long running mock"}],"model":"claude-3"}`)
	controlEchoLoop(os.Stdin, os.Stdout)
}

// runMockShouldFail writes an error line and exits non-zero so Connect can
// surface the initialize failure path.
func runMockShouldFail() {
	fmt.Fprintln(os.Stderr, "Mock CLI failing")
	os.Exit(1)
}

// runMockCheckEnvironment validates that the SDK forwarded CLAUDE_CODE_ENTRYPOINT
// before responding. Mirrors the original .sh check.
func runMockCheckEnvironment() {
	ep := os.Getenv("CLAUDE_CODE_ENTRYPOINT")
	if ep != "sdk-go" && ep != "sdk-go-client" {
		fmt.Fprintln(os.Stderr, "Missing environment variable")
		os.Exit(1)
	}
	fmt.Println(`{"type":"assistant","content":[{"type":"text","text":"Environment OK"}],"model":"claude-3"}`)
	controlEchoLoop(os.Stdin, os.Stdout)
}

// runMockInvalidOutput responds to the initialize control request first so the
// handshake completes, then emits garbage + invalid JSON + a valid message to
// exercise the parser's resilience.
func runMockInvalidOutput() {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	if scanner.Scan() {
		line := scanner.Text()
		if isControlRequest(line) {
			fmt.Println(buildControlResponse(extractRequestID(line)))
		}
	}
	fmt.Println("This is not valid JSON output")
	fmt.Println(`{"invalid": json}`)
	fmt.Println(`{"type":"assistant","content":[{"type":"text","text":"Valid after invalid"}],"model":"claude-3"}`)
	// Keep draining stdin and responding to any further control requests.
	for scanner.Scan() {
		line := scanner.Text()
		if isControlRequest(line) {
			fmt.Println(buildControlResponse(extractRequestID(line)))
		}
	}
}

// runMockWithControlProtocol drives the control-message routing test: emit a
// regular assistant message, then echo back a control_response for each
// control_request seen on stdin.
func runMockWithControlProtocol() {
	fmt.Println(assistantMsg)
	controlEchoLoop(os.Stdin, os.Stdout)
}

// runMockWithStderr emits stderr lines, then proceeds like the default mode.
func runMockWithStderr() {
	fmt.Fprintln(os.Stderr, "Stderr line 1")
	fmt.Fprintln(os.Stderr, "Stderr line 2")
	fmt.Println(assistantMsg)
	controlEchoLoop(os.Stdin, os.Stdout)
}

// runMockInitError emits its PID on stderr so the test can verify reaping,
// then responds to the initialize control request with an error subtype and
// keeps reading stdin so the SDK must explicitly terminate the process.
// Used to verify that a failed Connect tears the subprocess down.
func runMockInitError() {
	fmt.Fprintf(os.Stderr, "MOCK_PID=%d\n", os.Getpid())
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if isControlRequest(line) {
			fmt.Printf(
				`{"type":"control_response","response":{"subtype":"error","request_id":%q,"error":"mock init failure"}}`+"\n",
				extractRequestID(line),
			)
		}
	}
}

// controlEchoLoop reads JSON-line stdin and replies to every control_request
// with a success control_response carrying the matching request_id. Returns
// when stdin closes.
func controlEchoLoop(in *os.File, out *os.File) {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if isControlRequest(line) {
			_, _ = fmt.Fprintln(out, buildControlResponse(extractRequestID(line)))
		}
	}
}

// requestIDRegex matches `"request_id":"<value>"` in a JSON line. Compiled once
// at package init.
var requestIDRegex = regexp.MustCompile(`"request_id":"([^"]+)"`)

func isControlRequest(line string) bool {
	return strings.Contains(line, "control_request")
}

func extractRequestID(line string) string {
	m := requestIDRegex.FindStringSubmatch(line)
	if len(m) < 2 {
		return "req_1_mock"
	}
	return m[1]
}

func buildControlResponse(requestID string) string {
	return fmt.Sprintf(
		`{"type":"control_response","response":{"subtype":"success","request_id":"%s","response":{}}}`,
		requestID,
	)
}

// --- Helper API for tests ------------------------------------------------

// newTransportMockCLI returns os.Args[0] (the test binary) configured to act
// as a default-mode mock CLI for the duration of the test. Mode is propagated
// via t.Setenv (auto-cleaned at test end).
func newTransportMockCLI(t *testing.T) string {
	t.Helper()
	return newTransportMockCLIWithOptions(t)
}

// newTransportMockCLIWithOptions returns the test binary path configured for a
// specific mock mode. The mode is derived from the first matching option,
// keeping parity with the legacy script-based factory.
func newTransportMockCLIWithOptions(t *testing.T, options ...TransportMockOption) string {
	t.Helper()
	opts := &transportMockOptions{}
	for _, opt := range options {
		opt(opts)
	}

	mode := mockModeDefault
	switch {
	case opts.shouldFail:
		mode = mockModeShouldFail
	case opts.longRunning:
		mode = mockModeLongRunning
	case opts.checkEnvironment:
		mode = mockModeCheckEnvironment
	case opts.invalidOutput:
		mode = mockModeInvalidOutput
	}

	t.Setenv(envMockMode, mode)
	return os.Args[0]
}

// newTransportMockCLIWithControlProtocol returns the test binary configured to
// participate in the control protocol without emitting an initial assistant
// message before stdin is drained.
func newTransportMockCLIWithControlProtocol(t *testing.T) string {
	t.Helper()
	t.Setenv(envMockMode, mockModeWithControlProtocol)
	return os.Args[0]
}

// newTransportMockCLIWithStderr returns the test binary configured to emit
// stderr lines plus a default-mode assistant message and control-loop.
func newTransportMockCLIWithStderr(t *testing.T) string {
	t.Helper()
	t.Setenv(envMockMode, mockModeWithStderr)
	return os.Args[0]
}

// newTransportMockCLIInitError returns the test binary configured to reply to
// initialize with an error subtype while keeping the process alive on stdin.
// Exercises the Connect-fails cleanup path.
func newTransportMockCLIInitError(t *testing.T) string {
	t.Helper()
	t.Setenv(envMockMode, mockModeInitError)
	return os.Args[0]
}
