# Claude Agent SDK for Go

<div align="center">
  <img src="gopher.png" alt="Go Gopher" width="200"/>
</div>

<div align="center">

[![CI](https://github.com/severity1/claude-agent-sdk-go/actions/workflows/ci.yml/badge.svg)](https://github.com/severity1/claude-agent-sdk-go/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/severity1/claude-agent-sdk-go.svg)](https://pkg.go.dev/github.com/severity1/claude-agent-sdk-go)
[![Go Report Card](https://goreportcard.com/badge/github.com/severity1/claude-agent-sdk-go)](https://goreportcard.com/report/github.com/severity1/claude-agent-sdk-go)
[![codecov](https://codecov.io/gh/severity1/claude-agent-sdk-go/branch/main/graph/badge.svg)](https://codecov.io/gh/severity1/claude-agent-sdk-go)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

</div>

Unofficial Go SDK for Claude Code CLI integration. Build production-ready applications that leverage Claude's advanced code understanding, secure file operations, and external tool integrations through a clean, idiomatic Go API with comprehensive error handling and automatic resource management.

**Two powerful APIs for different use cases:**
- **Query API**: One-shot operations, automation, CI/CD integration  
- **Client API**: Interactive conversations, multi-turn workflows, streaming responses
- **WithClient**: Go-idiomatic context manager for automatic resource management

![Claude Agent SDK in Action](cc-sdk-go-in-action-v2.gif)

## Installation

```bash
go get github.com/severity1/claude-agent-sdk-go
```

**Prerequisites:** Go 1.18+, Node.js, Claude Code (`npm install -g @anthropic-ai/claude-code`)

## Key Features

**Two APIs for different needs** - Query for automation, Client for interaction
**100% Python SDK compatibility** - Same functionality, Go-native design
**Automatic resource management** - WithClient provides Go-idiomatic context manager pattern
**Session management** - Isolated conversation contexts with `Query()` and `QueryWithSession()`
**Built-in tool integration** - File operations, AWS, GitHub, databases, and more
**Production ready** - Comprehensive error handling, timeouts, resource cleanup
**Security focused** - Granular tool permissions and access controls
**Context-aware** - Maintain conversation state across multiple interactions
**Advanced capabilities** - Permission callbacks, lifecycle hooks, file checkpointing

## Usage

### Query API - One-Shot Operations
Best for automation, scripting, and tasks with clear completion criteria:

```go
package main

import (
    "context"
    "errors"
    "fmt"
    "log"

    "github.com/severity1/claude-agent-sdk-go"
)

func main() {
    fmt.Println("Claude Agent SDK - Query API Example")
    fmt.Println("Asking: What is 2+2?")

    ctx := context.Background()

    // Create and execute query
    iterator, err := claudecode.Query(ctx, "What is 2+2?")
    if err != nil {
        // Use error type helpers for specific error handling
        if cliErr := claudecode.AsCLINotFoundError(err); cliErr != nil {
            fmt.Printf("Claude CLI not found: %v\n", cliErr)
            fmt.Println("Install with: npm install -g @anthropic-ai/claude-code")
            return
        }
        if connErr := claudecode.AsConnectionError(err); connErr != nil {
            fmt.Printf("Connection failed: %v\n", connErr)
            return
        }
        log.Fatalf("Query failed: %v", err)
    }
    defer iterator.Close()

    fmt.Println("\nResponse:")

    // Iterate through messages
    for {
        message, err := iterator.Next(ctx)
        if err != nil {
            if errors.Is(err, claudecode.ErrNoMoreMessages) {
                break
            }
            log.Fatalf("Failed to get message: %v", err)
        }

        if message == nil {
            break
        }

        // Handle different message types
        switch msg := message.(type) {
        case *claudecode.AssistantMessage:
            for _, block := range msg.Content {
                if textBlock, ok := block.(*claudecode.TextBlock); ok {
                    fmt.Print(textBlock.Text)
                }
            }
        case *claudecode.ResultMessage:
            if msg.IsError {
                if msg.Result != nil {
                    log.Printf("Error: %s", *msg.Result)
                } else {
                    log.Printf("Error: unknown error")
                }
            }
        }
    }

    fmt.Println("\nQuery completed!")
}
```

### Client API - Interactive & Multi-Turn
**WithClient provides automatic resource management (equivalent to Python's `async with`):**

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/severity1/claude-agent-sdk-go"
)

func main() {
    fmt.Println("Claude Agent SDK - Client Streaming Example")
    fmt.Println("Asking: Explain Go goroutines with a simple example")

    ctx := context.Background()
    question := "Explain what Go goroutines are and show a simple example"

    // WithClient handles connection lifecycle automatically
    err := claudecode.WithClient(ctx, func(client claudecode.Client) error {
        fmt.Println("\nConnected! Streaming response:")

        // Simple query uses default session
        if err := client.Query(ctx, question); err != nil {
            return fmt.Errorf("query failed: %w", err)
        }

        // Stream messages in real-time
        msgChan := client.ReceiveMessages(ctx)
        for {
            select {
            case message := <-msgChan:
                if message == nil {
                    // The stream ended. Err says why, or is nil when the
                    // process exit is not known yet.
                    if err := client.Err(); err != nil {
                        return fmt.Errorf("stream ended: %w", err)
                    }
                    return nil
                }

                switch msg := message.(type) {
                case *claudecode.AssistantMessage:
                    // Print streaming text as it arrives
                    for _, block := range msg.Content {
                        if textBlock, ok := block.(*claudecode.TextBlock); ok {
                            fmt.Print(textBlock.Text)
                        }
                    }
                case *claudecode.ResultMessage:
                    if msg.IsError {
                        if msg.Result != nil {
                            return fmt.Errorf("error: %s", *msg.Result)
                        }
                        return fmt.Errorf("error: unknown error")
                    }
                    return nil // Success, stream complete
                }
            case <-ctx.Done():
                return ctx.Err()
            }
        }
    })

    if err != nil {
        log.Fatalf("Streaming failed: %v", err)
    }

    fmt.Println("\n\nStreaming completed!")
}
```

### Session Management

**Maintain conversation context across multiple queries with session management:**

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/severity1/claude-agent-sdk-go"
)

func main() {
    fmt.Println("Claude Agent SDK - Session Management Example")

    ctx := context.Background()

    err := claudecode.WithClient(ctx, func(client claudecode.Client) error {
        fmt.Println("\nDemonstrating isolated sessions:")

        // Session A: Math conversation
        sessionA := "math-session"
        if err := client.QueryWithSession(ctx, "Remember this: x = 5", sessionA); err != nil {
            return err
        }

        // Session B: Programming conversation
        sessionB := "programming-session"
        if err := client.QueryWithSession(ctx, "Remember this: language = Go", sessionB); err != nil {
            return err
        }

        // Query each session - they maintain separate contexts
        fmt.Println("\nQuerying math session:")
        if err := client.QueryWithSession(ctx, "What is x * 2?", sessionA); err != nil {
            return err
        }

        fmt.Println("\nQuerying programming session:")
        if err := client.QueryWithSession(ctx, "What language did I mention?", sessionB); err != nil {
            return err
        }

        // Default session query (separate from above)
        fmt.Println("\nDefault session (no context from above):")
        return client.Query(ctx, "What did I just ask about?") // Won't know about x or Go
    })

    if err != nil {
        log.Fatalf("Session demo failed: %v", err)
    }

    fmt.Println("Session management demo completed!")
}
```

**Traditional Client API (still supported):**

<details>
<summary>Click to see manual resource management approach</summary>

```go
func traditionalClientExample() {
    ctx := context.Background()
    
    client := claudecode.NewClient()
    if err := client.Connect(ctx); err != nil {
        log.Fatalf("Failed to connect: %v", err)
    }
    defer client.Disconnect() // Manual cleanup required
    
    // Use client...
}
```
</details>

## Tool Integration & External Services

Integrate with file systems, cloud services, databases, and development tools:

**Core Tools** (built-in file operations):
```go
// File analysis and documentation generation
claudecode.Query(ctx, "Read all Go files and create API documentation",
    claudecode.WithAllowedTools("Read", "Write"))
```

**MCP Tools** (external service integrations):
```go
// AWS infrastructure automation
claudecode.Query(ctx, "List my S3 buckets and analyze their security settings",
    claudecode.WithAllowedTools("mcp__aws-api-mcp__call_aws", "mcp__aws-api-mcp__suggest_aws_commands", "Write"))
```

## Configuration Options

Customize Claude's behavior with functional options:

**Tool & Permission Control:**
```go
claudecode.Query(ctx, prompt,
    claudecode.WithAllowedTools("Read", "Write"),
    claudecode.WithPermissionMode(claudecode.PermissionModeAcceptEdits))
```

**System Behavior:**
```go
claudecode.Query(ctx, prompt,
    claudecode.WithSystemPrompt("You are a senior Go developer"),
    claudecode.WithModel("claude-sonnet-4-5"),
    claudecode.WithMaxTurns(10))
```

**Environment Variables** (new in v0.2.5):
```go
// Proxy configuration
claudecode.NewClient(
    claudecode.WithEnv(map[string]string{
        "HTTP_PROXY":  "http://proxy.example.com:8080",
        "HTTPS_PROXY": "http://proxy.example.com:8080",
    }))

// Individual variables
claudecode.NewClient(
    claudecode.WithEnvVar("DEBUG", "1"),
    claudecode.WithEnvVar("CUSTOM_PATH", "/usr/local/bin"))
```

**Context & Working Directory:**
```go
claudecode.Query(ctx, prompt,
    claudecode.WithCwd("/path/to/project"),
    claudecode.WithAddDirs("src", "docs"))
```

**Settings Sources and CLAUDE.md** (changed in v0.8.0):

By default the CLI loads its settings sources (user, project and local), which include `CLAUDE.md`. This matches the Python SDK. Before v0.8.0, the Go SDK loaded no filesystem settings by default.
```go
// Load only the project settings and CLAUDE.md
claudecode.Query(ctx, prompt,
    claudecode.WithSettingSources(claudecode.SettingSourceProject))

// Isolation: load no filesystem settings and no CLAUDE.md
claudecode.Query(ctx, prompt,
    claudecode.WithSettingSources())
```

**Session Management** (Client API):
```go
// WithClient provides isolated session contexts
err := claudecode.WithClient(ctx, func(client claudecode.Client) error {
    // Default session
    client.Query(ctx, "Remember: x = 5")

    // Named session (isolated context)
    return client.QueryWithSession(ctx, "What is x?", "math-session")
})
```

**Programmatic Agents:**
```go
// Define custom agents for specialized tasks
claudecode.Query(ctx, "Review this codebase for security issues",
    claudecode.WithAgent("security-reviewer", claudecode.AgentDefinition{
        Description: "Reviews code for security vulnerabilities",
        Prompt:      "You are a security expert focused on OWASP top 10...",
        Tools:       []string{"Read", "Grep", "Glob"},
        Model:       claudecode.AgentModelSonnet,
    }))

// Multiple agents for complex workflows
claudecode.Query(ctx, "Analyze and improve this code",
    claudecode.WithAgents(map[string]claudecode.AgentDefinition{
        "code-reviewer": {
            Description: "Reviews code quality and best practices",
            Prompt:      "You are a senior engineer focused on code quality...",
            Tools:       []string{"Read", "Grep"},
            Model:       claudecode.AgentModelSonnet,
        },
        "test-writer": {
            Description: "Writes comprehensive unit tests",
            Prompt:      "You are a testing expert...",
            Tools:       []string{"Read", "Write", "Bash"},
            Model:       claudecode.AgentModelHaiku,
        },
    }))
```

Available agent models: `AgentModelSonnet`, `AgentModelOpus`, `AgentModelHaiku`, `AgentModelInherit`

## Documentation

- [Architecture](ARCHITECTURE.md) - System design and component overview
- [Contributing](CONTRIBUTING.md) - Development setup and guidelines
- [API Reference](docs/reference.md) - Complete SDK reference with all types, functions, and examples
- [Python SDK Parity](docs/parity.md) - Feature comparison with the Python SDK
- [pkg.go.dev](https://pkg.go.dev/github.com/severity1/claude-agent-sdk-go) - GoDoc reference

## Advanced Features

The SDK includes advanced capabilities for production use:

- **Permission Callbacks** - Programmatic tool access control ([Example 11](examples/11_permission_callback/))
- **Lifecycle Hooks** - Intercept tool execution events ([Example 12](examples/12_hooks/))
- **File Checkpointing** - Track and rewind file changes ([Example 13](examples/13_file_checkpointing/))
- **SDK MCP Servers** - Create in-process custom tools ([Example 14](examples/14_sdk_mcp_server/))
- **Stream Diagnostics** - Monitor stream health with `GetStreamIssues()` and `GetStreamStats()`
- **Process Lifecycle** - `Done()` closes when the CLI process exits and `Err()` says why
- **Task Lifecycle** - Follow subagents and background tasks with `SystemMessage.AsTaskStarted()`, `AsTaskProgress()`, `AsTaskNotification()` and `AsTaskUpdated()`, and stop a single task with `StopTask()`

See the [examples directory](examples/README.md) for complete documentation.

## When to Use Which API

**Use Query API when you:**
- Need one-shot automation or scripting
- Have clear task completion criteria  
- Want automatic resource cleanup
- Are building CI/CD integrations
- Prefer simple, stateless operations

**Use Client API (WithClient) when you:**  
- Need interactive conversations
- Want to build context across multiple requests
- Are creating complex, multi-step workflows
- Need real-time streaming responses
- Want to iterate and refine based on previous results
- **Need automatic resource management (recommended)**

## Examples

See [`examples/README.md`](examples/README.md) for detailed documentation.

### Getting Started
| Example | Description |
|---------|-------------|
| [`01_quickstart`](examples/01_quickstart/) | Query API fundamentals |
| [`02_client_streaming`](examples/02_client_streaming/) | WithClient streaming basics |
| [`03_client_multi_turn`](examples/03_client_multi_turn/) | Multi-turn conversations |

### Tool Integration
| Example | Description |
|---------|-------------|
| [`04_query_with_tools`](examples/04_query_with_tools/) | File operations with Query API |
| [`05_client_with_tools`](examples/05_client_with_tools/) | Interactive file workflows |
| [`06_query_with_mcp`](examples/06_query_with_mcp/) | External MCP server integration |
| [`07_client_with_mcp`](examples/07_client_with_mcp/) | Multi-turn MCP workflows |

### Production Patterns
| Example | Description |
|---------|-------------|
| [`08_client_advanced`](examples/08_client_advanced/) | Error handling, model switching |
| [`09_context_manager`](examples/09_context_manager/) | WithClient vs manual patterns |
| [`10_session_management`](examples/10_session_management/) | Session isolation |

### Security & Lifecycle
| Example | Description |
|---------|-------------|
| [`11_permission_callback`](examples/11_permission_callback/) | Permission callbacks |
| [`12_hooks`](examples/12_hooks/) | Lifecycle hooks |
| [`13_file_checkpointing`](examples/13_file_checkpointing/) | File rewind capabilities |
| [`14_sdk_mcp_server`](examples/14_sdk_mcp_server/) | In-process custom tools |

## License

MIT
