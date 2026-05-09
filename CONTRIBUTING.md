# Contributing

Thank you for your interest in contributing to the Claude Agent SDK for Go!

## Prerequisites

- **Go 1.18+** - [Download Go](https://go.dev/dl/)
- **Node.js** - [Download Node.js](https://nodejs.org/)
- **Claude Code CLI** - `npm install -g @anthropic-ai/claude-code`

## Development Setup

```bash
# Clone the repository
git clone https://github.com/severity1/claude-agent-sdk-go.git
cd claude-agent-sdk-go

# Verify installation
go build ./...
go test ./...
```

## Code Style

### Formatting and Linting

Run these before every commit:

```bash
go fmt ./...                  # Format code
go vet ./...                  # Static analysis
golangci-lint run             # Comprehensive linting
gocyclo -over 15 .            # Cyclomatic complexity check
deadcode -test=true \         # Find unreachable internal functions
  -filter='github.com/severity1/claude-agent-sdk-go/internal/...' \
  ./examples/... ./internal/...

# Or use Makefile (recommended)
make check             # Run all checks (fmt, vet, lint, cyclo, deadcode, fuzz-test)
```

### Cyclomatic Complexity

We use `gocyclo` to track function complexity. The threshold is **15** - functions above this should be refactored.

```bash
# Install gocyclo
go install github.com/fzipp/gocyclo/cmd/gocyclo@latest

# Check complexity
make cyclo             # Show functions over threshold
gocyclo -over 15 .     # Direct usage
```

**Guidelines:**
- Keep functions under complexity 15
- Higher complexity is acceptable for: table-driven tests, examples, orchestration code
- When complexity grows, extract helper methods

### Dead Code Detection

We use `golang.org/x/tools/cmd/deadcode` to find unreachable functions in `internal/*`. CI fails if any are detected.

```bash
# Install deadcode
go install golang.org/x/tools/cmd/deadcode@latest

# Show unreachable internal functions
make deadcode

# Or invoke directly:
deadcode -test=true \
  -filter='github.com/severity1/claude-agent-sdk-go/internal/...' \
  ./examples/... ./internal/...

# Fail on any internal dead code (CI mode)
make deadcode-check
```

**Why this scope:**
- The SDK is a library, so its only `main` packages are under `examples/`. Deadcode roots reachability from `main` packages.
- `-test=true` adds internal test packages as additional roots so test-only helpers (e.g. `Parser.processJSONLine`, mock factories) count as reachable instead of being flagged as dead.
- The public API (root `claudecode` package) is reachable by external consumers, not by us — running deadcode without a filter would flag every `WithXxx()` option as unused. The `-filter` restricts findings to `internal/*` where unreachability is real waste.

**Guidelines:**
- Remove unreachable internal helpers instead of suppressing them
- If a public API symbol drifts (e.g., an option exposed but never wired through `internal/*`), an example or unit test should exercise it so the wiring is reachable
- For a genuinely reserved internal symbol, document the rationale and add a test that calls it

### Go Conventions

- **Interface-driven design**: Define behavior through interfaces
- **Context-first**: All blocking functions accept `context.Context` as first parameter
- **Error wrapping**: Use `fmt.Errorf("context: %w", err)` for error chains
- **Functional options**: Use `WithXxx()` pattern for configuration
- **No unnecessary exports**: Keep identifiers unexported unless needed

### Naming

- Interfaces: Describe behavior (e.g., `Transport`, `Message`)
- Implementations: Concrete names (e.g., `ClientImpl`, `mockTransport`)
- Options: `WithXxx()` pattern (e.g., `WithSystemPrompt()`)
- Errors: `XxxError` suffix (e.g., `CLINotFoundError`)

## Testing

### Running Tests

```bash
go test ./...              # Run all tests
go test -v ./...           # Verbose output
go test -race ./...        # Race condition detection
go test -cover ./...       # Coverage report
```

### Test Patterns

Reference `client_test.go` as the gold standard for testing patterns.

**Test File Organization:**
```go
// 1. Test functions (primary purpose)
func TestFeature(t *testing.T) {...}

// 2. Mock implementations
type mockTransport struct {...}

// 3. Helper functions
func setupTest(t *testing.T) {...}
```

**Table-Driven Tests:**
```go
tests := []struct {
    name    string
    input   string
    want    string
    wantErr bool
}{
    {"valid input", "hello", "HELLO", false},
    {"empty input", "", "", true},
}

for _, tt := range tests {
    t.Run(tt.name, func(t *testing.T) {
        got, err := Process(tt.input)
        if (err != nil) != tt.wantErr {
            t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
        }
        if got != tt.want {
            t.Errorf("got %v, want %v", got, tt.want)
        }
    })
}
```

**Helper Functions:**
```go
func setupTestContext(t *testing.T, timeout time.Duration) (context.Context, context.CancelFunc) {
    t.Helper()  // Critical for correct line reporting
    return context.WithTimeout(context.Background(), timeout)
}
```

**Thread-Safe Mocks:**
```go
type mockTransport struct {
    mu        sync.Mutex
    connected bool
    messages  []Message
}

func (m *mockTransport) SendMessage(msg Message) {
    m.mu.Lock()
    defer m.mu.Unlock()
    m.messages = append(m.messages, msg)
}
```

### Benchmarks

Run benchmarks to measure performance:

```bash
make bench                           # Run all benchmarks
go test -bench=. -benchmem ./...     # Direct usage
go test -bench=. -benchmem ./internal/parser/  # Specific package
```

**Benchmark Best Practices:**

```go
// Sink prevents dead code elimination by the compiler
var sink any

func BenchmarkFeature(b *testing.B) {
    // Setup outside timed section
    fixture := setupFixture()

    b.ReportAllocs()  // Always track allocations
    b.ResetTimer()    // Exclude setup time

    for i := 0; i < b.N; i++ {
        result := functionUnderTest(fixture)
        sink = result  // Prevent optimization
    }
}
```

**Table-Driven Benchmarks:**

```go
func BenchmarkProcessLine(b *testing.B) {
    tests := []struct {
        name string
        input string
    }{
        {"simple", `{"type":"user","content":"hello"}`},
        {"complex", `{"type":"assistant","content":[...]}`},
    }

    for _, tc := range tests {
        b.Run(tc.name, func(b *testing.B) {
            b.ReportAllocs()
            b.ResetTimer()
            for i := 0; i < b.N; i++ {
                result, _ := Process(tc.input)
                sink = result
            }
        })
    }
}
```

**Key Points:**
- Use `var sink any` at package level to prevent dead code elimination
- Always call `b.ReportAllocs()` to track memory allocations
- Call `b.ResetTimer()` after setup code
- Reset mutable state between iterations (e.g., `parser.Reset()`)
- Use `b.Run()` for table-driven sub-benchmarks

## Commit Conventions

Use conventional commit messages:

```
<type>: <description>

[optional body]

[optional footer]
```

### Types

| Type | Description |
|------|-------------|
| `feat` | New feature |
| `fix` | Bug fix |
| `docs` | Documentation only |
| `test` | Adding or updating tests |
| `refactor` | Code change that neither fixes a bug nor adds a feature |
| `chore` | Maintenance tasks |

### Examples

```
feat: Add permission callback support (Issue #8)

Implement WithCanUseTool option for programmatic tool access control.

- Add CanUseToolCallback type
- Add PermissionResult types (Allow, Deny, AskUser)
- Update control protocol to handle permission requests
```

```
fix: Handle connection timeout correctly

The transport was not respecting context cancellation during
the initial handshake, causing hangs on slow connections.
```

### Issue References

Reference issues in commits:
- `(Issue #N)` - Related to issue
- `Closes #N` - Closes issue when merged (in PR body)

## Pull Request Process

### 1. Create a Feature Branch

```bash
git checkout main
git pull origin main
git checkout -b feature/issue-N-short-description
```

### 2. Make Changes

- Write tests first (TDD approach)
- Implement the feature
- Ensure all tests pass
- Run linting

### 3. Commit Changes

```bash
git add .
git commit -m "feat: Description (Issue #N)"
```

### 4. Push and Create PR

```bash
git push -u origin feature/issue-N-short-description
```

Then create a PR on GitHub with:
- Clear title matching commit convention
- Description of changes
- Link to related issue
- Test plan

### 5. PR Requirements

- [ ] All tests pass
- [ ] No linting errors
- [ ] Code follows style guidelines
- [ ] Documentation updated (if applicable)
- [ ] Commit messages follow conventions

## Issue Guidelines

### Before Creating an Issue

1. Search existing issues to avoid duplicates
2. Check if it's already fixed in `main`

### Creating an Issue

Include:
- **Clear title**: Descriptive summary
- **Description**: What you expected vs. what happened
- **Reproduction steps**: Minimal example to reproduce
- **Environment**: Go version, OS, CLI version
- **Proposed solution**: (Optional) How you think it should be fixed

### Issue Labels

| Label | Description |
|-------|-------------|
| `bug` | Something isn't working |
| `enhancement` | New feature request |
| `docs` | Documentation improvement |
| `good first issue` | Good for newcomers |

## Architecture

See [ARCHITECTURE.md](ARCHITECTURE.md) for system design documentation.

## Questions?

- Open a [GitHub Issue](https://github.com/severity1/claude-agent-sdk-go/issues)
- Check existing [documentation](docs/)
