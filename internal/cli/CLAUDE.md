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
- Version validation: Use semantic versioning comparison; `MinimumCLIVersion = "2.0.76"` is the enforced minimum
- Error handling: Return `CLINotFoundError` with installation instructions
- Agents not in CLI flags: `addOptionsToCommand()` does NOT add `--agents`; agents are sent via the Initialize control protocol request instead
- SettingSources nil-guard: `--setting-sources` flag is emitted ONLY when `options.SettingSources != nil` (matches Python PR #778 "if effective_setting_sources is not None" guard); nil = use CLI default (no flag emitted); empty slice = emit `--setting-sources=` to force CLI to load nothing

<!-- END AUTO-MANAGED -->

<!-- AUTO-MANAGED: dependencies -->
## Key Dependencies

- `internal/shared`: Error types (`CLINotFoundError`)
- Standard library: `os/exec`, `path/filepath`, `runtime`

<!-- END AUTO-MANAGED -->

<!-- MANUAL -->
## Notes

<!-- END MANUAL -->
