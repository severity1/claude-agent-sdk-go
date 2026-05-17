package claudecode

import (
	"context"
	"strings"
	"testing"
)

// TestResolveCLIPathHonorsWithCLIPath verifies that when the caller has
// supplied a CLI path via WithCLIPath, resolveCLIPath returns that path and
// does NOT invoke cli.FindCLI. This guards against the regression where
// Options.CLIPath was being set by WithCLIPath but never read anywhere in
// the transport-creation path, making the option silently dead.
func TestResolveCLIPathHonorsWithCLIPath(t *testing.T) {
	// Force CLI auto-discovery to fail by isolating PATH/HOME — if the option
	// is honored, this isolation should not matter.
	cleanup := setupIsolatedEnvironment(t)
	defer cleanup()

	const suppliedPath = "/totally/made/up/path/to/claude"
	options := NewOptions(WithCLIPath(suppliedPath))

	got, err := resolveCLIPath(options)
	if err != nil {
		t.Fatalf("resolveCLIPath returned error when WithCLIPath was set: %v", err)
	}
	if got != suppliedPath {
		t.Errorf("resolveCLIPath returned %q, want %q", got, suppliedPath)
	}
}

// TestResolveCLIPathFallsBackToFindCLI verifies that when no CLIPath was
// supplied (or it was supplied as empty), resolveCLIPath delegates to
// cli.FindCLI. Auto-discovery failure inside the isolated environment is
// the signal that FindCLI ran (and not the supplied-path early-return).
func TestResolveCLIPathFallsBackToFindCLI(t *testing.T) {
	cleanup := setupIsolatedEnvironment(t)
	defer cleanup()

	tests := []struct {
		name    string
		options *Options
	}{
		{name: "nil_options", options: nil},
		{name: "nil_cli_path", options: NewOptions()},
		{name: "empty_cli_path", options: NewOptions(WithCLIPath(""))},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := resolveCLIPath(tc.options)
			if err == nil {
				t.Fatal("expected resolveCLIPath to fail via FindCLI in isolated env, got nil")
			}
			// FindCLI returns one of two messages depending on node availability.
			if !strings.Contains(err.Error(), "Claude Code") {
				t.Errorf("expected FindCLI-shaped error, got: %v", err)
			}
		})
	}
}

// TestCreateQueryTransportHonorsWithCLIPath confirms the wiring is complete
// end-to-end: createQueryTransport must consult Options.CLIPath rather than
// always calling FindCLI. Before the fix, this test would fail because
// FindCLI ran unconditionally and produced "Claude Code not found" /
// "requires Node.js" in the isolated environment.
func TestCreateQueryTransportHonorsWithCLIPath(t *testing.T) {
	cleanup := setupIsolatedEnvironment(t)
	defer cleanup()

	options := NewOptions(WithCLIPath("/totally/made/up/path/to/claude"))

	transport, err := createQueryTransport(options)
	if err != nil {
		t.Fatalf("createQueryTransport returned error with WithCLIPath set: %v", err)
	}
	if transport == nil {
		t.Fatal("createQueryTransport returned nil transport with WithCLIPath set")
	}
}

// TestClientConnectHonorsWithCLIPath confirms the same wiring through the
// streaming-mode entrypoint. Client.Connect uses resolveCLIPath; before
// the fix it called cli.FindCLI directly and ignored WithCLIPath.
//
// We can't fully Connect to the fake path (it doesn't exist), but we can
// observe which path the SDK tried — the error message will contain the
// supplied path rather than the FindCLI "not found" boilerplate.
func TestClientConnectHonorsWithCLIPath(t *testing.T) {
	cleanup := setupIsolatedEnvironment(t)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const suppliedPath = "/totally/made/up/path/to/claude"
	client := NewClient(WithCLIPath(suppliedPath))

	err := client.Connect(ctx)
	if err == nil {
		// We don't actually expect Connect to succeed against a fake path,
		// but if some future change makes it work, just disconnect cleanly.
		_ = client.Disconnect()
		return
	}
	// The error should reference the supplied path (transport tried to exec
	// it and failed) rather than the FindCLI "Claude Code not found" message.
	if strings.Contains(err.Error(), "Claude Code not found") ||
		strings.Contains(err.Error(), "requires Node.js") {
		t.Errorf("Connect fell back to FindCLI despite WithCLIPath being set, error: %v", err)
	}
}
