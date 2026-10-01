# Architecture

This document provides a high-level overview of the Claude Agent SDK for Go architecture.

## Quick Links

- [Component Structure](docs/architecture/components.md) - Package organization and responsibilities
- [Data Flow](docs/architecture/data-flow.md) - How data moves through the system
- [Interfaces](docs/architecture/interfaces.md) - Core interface definitions
- [Design Patterns](docs/architecture/patterns.md) - Patterns used throughout the SDK
- [Advanced Features](docs/architecture/advanced.md) - Hooks, permissions, MCP, and more

## Overview

The Claude Agent SDK for Go provides programmatic access to Claude Code CLI through two APIs:

- **Query API**: One-shot operations for automation and scripting
- **Client API**: Streaming connections for interactive, multi-turn conversations

The SDK achieves 100% feature parity with the Python SDK while following idiomatic Go patterns.

## System Design

```
┌─────────────────────────────────────────────────────────────┐
│                      Your Application                        │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                    Public API (claudecode)                   │
│                                                             │
│   Query(ctx, prompt)              WithClient(ctx, fn)       │
│   NewClient(opts...)              WithSystemPrompt(...)     │
│                                   WithAllowedTools(...)     │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                    Transport Interface                       │
│                                                             │
│   Connect()  SendMessage()  ReceiveMessages()  Close()      │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                  internal/subprocess                         │
│                                                             │
│   Process Management    I/O Streams    Message Routing      │
└─────────────────────────────────────────────────────────────┘
           │                    │                    │
           ▼                    ▼                    ▼
┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐
│  internal/cli   │  │ internal/parser │  │internal/control │
│                 │  │                 │  │                 │
│  CLI Discovery  │  │  JSON Parsing   │  │ Control Protocol│
│  Command Build  │  │  Type Discrim.  │  │ Hooks/Perms/MCP │
└─────────────────┘  └─────────────────┘  └─────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                     Claude Code CLI                          │
│                   (Subprocess: stdin/stdout)                 │
└─────────────────────────────────────────────────────────────┘
```

## Key Concepts

### Interface-Driven Design

All major components are defined through interfaces, enabling testing with mocks and future flexibility. The `Transport` interface is the primary abstraction for CLI communication.

### Message Type System

Messages use interface-based polymorphism with type discrimination:

```go
switch msg := message.(type) {
case *AssistantMessage:
    // Handle Claude's response
case *ResultMessage:
    // Handle completion
}
```

### Functional Options

Configuration uses the functional options pattern for clean, extensible APIs:

```go
claudecode.Query(ctx, "prompt",
    claudecode.WithSystemPrompt("..."),
    claudecode.WithAllowedTools("Read", "Write"),
)
```

### Context-First

All blocking operations accept `context.Context` as the first parameter for cancellation and timeout support.

### Graceful Shutdown

`Close()` follows the Python sequence: close stdin -> wait up to 5 seconds for a clean exit -> SIGTERM -> wait up to 5 seconds -> SIGKILL. A CLI that exits on stdin EOF (the normal case) ends without a signal. The `Connect` context bounds only the connect step; the CLI runs until `Close()`.

## Package Structure

```
github.com/severity1/claude-agent-sdk-go
├── (root)              # Public API
├── internal/
│   ├── cli/            # CLI discovery and command building
│   ├── control/        # Control protocol (hooks, permissions)
│   ├── parser/         # JSON message parsing
│   ├── shared/         # Shared types and interfaces
│   └── subprocess/     # Process management
└── examples/           # Usage examples (01-20)
```

## Further Reading

For detailed documentation on each aspect of the architecture:

1. Start with [Components](docs/architecture/components.md) to understand the package layout
2. Review [Data Flow](docs/architecture/data-flow.md) to see how requests are processed
3. Study [Interfaces](docs/architecture/interfaces.md) for the core abstractions
4. Learn the [Design Patterns](docs/architecture/patterns.md) used throughout
5. Explore [Advanced Features](docs/architecture/advanced.md) for production capabilities
