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
└── discovery_bench_test.go # Performance benchmarks
```

**Key Functions**:
- `FindCLI()`: Searches PATH and platform-specific locations for Claude CLI
- `BuildCommand()`: Constructs CLI arguments from Options
- `BuildCommandWithPrompt()`: Constructs CLI command for one-shot queries; prompt appended last after all flags so CLI parses flags (e.g. `--mcp-config`) correctly
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

<!-- END MANUAL -->
