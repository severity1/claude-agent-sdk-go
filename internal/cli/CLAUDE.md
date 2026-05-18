# Module: cli

<!-- AUTO-MANAGED: module-description -->
## Purpose

CLI discovery and command building functionality. Locates the Claude CLI binary, validates version compatibility, and constructs command-line arguments for subprocess execution.

<!-- END AUTO-MANAGED -->

<!-- AUTO-MANAGED: architecture -->
## Module Architecture

```
cli/
├── discovery.go            # FindCLI(), version checking, path resolution
├── discovery_test.go       # Discovery tests
├── discovery_bench_test.go # Performance benchmarks
└── mock_cli_test.go        # TestMain + os.Args[0] mock CLI (cross-platform, CLAUDE_SDK_TEST_CLI_MOCK_MODE)
```

**Key Functions**:
- `FindCLI()`: Searches PATH and platform-specific locations for Claude CLI
- `BuildCommand()`: Constructs CLI arguments from Options. Always emits `--input-format stream-json` (Python SDK PR #468 parity); never `--print` or `--agents` - prompts ride on stdin after the initialize handshake, and agents ride on the initialize control request
- `GetCLIVersion()`: Extracts and validates CLI version

<!-- END AUTO-MANAGED -->

<!-- AUTO-MANAGED: conventions -->
## Module-Specific Conventions

- Cross-platform support: Handle Windows vs Unix path differences
- Version validation: Use semantic versioning comparison
- Error handling: Return `CLINotFoundError` with installation instructions

<!-- END AUTO-MANAGED -->

<!-- AUTO-MANAGED: dependencies -->
## Key Dependencies

- `internal/shared`: Error types (`CLINotFoundError`)
- Standard library: `os/exec`, `path/filepath`, `runtime`

<!-- END AUTO-MANAGED -->

<!-- MANUAL -->
## Notes

### Cross-platform mock CLI (TestMain + os.Args[0] re-entrancy)

`TestCheckCLIVersion` and `TestCheckCLIVersionSkipEnvVar` need a mock `claude -v` binary that prints a specific version on every OS. `mock_cli_test.go` defines a `TestMain` that dispatches on `CLAUDE_SDK_TEST_CLI_MOCK_MODE` so the compiled test binary itself serves as the mock CLI.

- **Mode protocol**: `CLAUDE_SDK_TEST_CLI_MOCK_MODE=version` prints the value of `CLAUDE_SDK_TEST_CLI_MOCK_VERSION` and exits. Both env vars are set via `t.Setenv` inside `createVersionMockCLI(t, version)` and auto-cleaned at test end.
- **Why not run the subprocess-package modes here**: the cli package's tests don't exercise the streaming/control flow — they only need a binary whose stdout is a version string. Keeping the env-var namespace distinct (`*_CLI_MOCK_*` vs `*_MOCK_*`) prevents accidental cross-package dispatch if tests are ever run against the same binary.
- **Parallel-subtest constraint**: same as the subprocess package — `t.Setenv` is incompatible with `t.Parallel()` at the same scope.
- **TestFindCLISuccess**: never executes the CLI; only checks file existence + executability. A single-byte placeholder file with the exec bit set is sufficient — no TestMain dispatch needed.

<!-- END MANUAL -->
