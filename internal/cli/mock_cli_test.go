package cli

import (
	"fmt"
	"os"
	"testing"
)

// TestMain dispatches based on a CLAUDE_SDK_TEST_CLI_MOCK_MODE env var so the
// compiled test binary can act as a cross-platform mock CLI when re-spawned.
//
// This mirrors the os/exec stdlib TestHelperProcess idiom and replaces the
// per-platform .bat / shell-script fixtures the cli tests previously created.
//
// When the env var is set the binary runs the mock handler and exits via
// os.Exit before m.Run() is reached. When unset, normal tests run.
func TestMain(m *testing.M) {
	if mode := os.Getenv(envCLIMockMode); mode != "" {
		runCLIMockCLI(mode)
		return
	}
	os.Exit(m.Run())
}

// Env vars used by the in-process cli-package mock CLI. Kept distinct from the
// subprocess-package mock var so the two test binaries don't accidentally
// trigger each other when shared modes are exported.
const (
	envCLIMockMode    = "CLAUDE_SDK_TEST_CLI_MOCK_MODE"
	envCLIMockVersion = "CLAUDE_SDK_TEST_CLI_MOCK_VERSION"
)

const (
	cliMockModeVersion = "version"
	cliMockModeFind    = "find"
)

// runCLIMockCLI dispatches to per-mode handlers based on the
// CLAUDE_SDK_TEST_CLI_MOCK_MODE env var.
func runCLIMockCLI(mode string) {
	switch mode {
	case cliMockModeVersion:
		runCLIMockVersion()
	case cliMockModeFind:
		runCLIMockFind()
	default:
		fmt.Fprintf(os.Stderr, "unknown cli mock mode: %s\n", mode)
		os.Exit(2)
	}
	os.Exit(0)
}

// runCLIMockVersion prints the value of CLAUDE_SDK_TEST_CLI_MOCK_VERSION so
// CheckCLIVersion's regex extracts a predictable version.
func runCLIMockVersion() {
	fmt.Println(os.Getenv(envCLIMockVersion))
}

// runCLIMockFind prints a fixed token so PATH-based discovery confirms the
// binary is executable. The token itself is irrelevant; only the exit status
// and that the file exists matter for FindCLI.
func runCLIMockFind() {
	fmt.Println("test")
}

// createVersionMockCLI returns os.Args[0] (the test binary) configured to act
// as a mock CLI that prints `version` when invoked. The version is propagated
// to the spawned child via t.Setenv (auto-cleaned at test end).
//
// Parallel-subtest constraint: t.Setenv is per-test and cannot be combined
// with t.Parallel(); none of the cli tests using this helper are parallel.
func createVersionMockCLI(t *testing.T, version string) string {
	t.Helper()
	t.Setenv(envCLIMockMode, cliMockModeVersion)
	t.Setenv(envCLIMockVersion, version)
	return os.Args[0]
}
