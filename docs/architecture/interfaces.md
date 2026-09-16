# Key Interfaces

This document describes the core interfaces that define the SDK's abstractions.

## Transport Interface

The `Transport` interface is the primary abstraction for CLI communication. It allows the SDK to be tested with mock implementations.

```go
// Transport abstracts subprocess communication for testing and flexibility.
type Transport interface {
    // Connect establishes connection to Claude CLI subprocess.
    // Returns error if CLI not found or connection fails.
    Connect(ctx context.Context) error

    // SendMessage sends a StreamMessage to the CLI via stdin.
    // The message is serialized as JSON with a newline terminator.
    SendMessage(ctx context.Context, message StreamMessage) error

    // ReceiveMessages returns channels for messages and errors.
    // Messages are parsed from CLI stdout as JSON.
    // The message channel closes when the stream ends.
    ReceiveMessages(ctx context.Context) (<-chan Message, <-chan error)

    // Interrupt sends an interrupt signal to pause/stop the current operation.
    Interrupt(ctx context.Context) error

    // SetModel changes the AI model during a streaming session.
    // Pass nil to reset to default model.
    SetModel(ctx context.Context, model *string) error

    // SetPermissionMode changes permission handling during a session.
    // Valid modes: "default", "acceptEdits", "plan", "bypassPermissions".
    // Note: Transport uses string; Client uses typed PermissionMode and converts.
    SetPermissionMode(ctx context.Context, mode string) error

    // RewindFiles reverts tracked files to state at a specific message.
    // Requires file checkpointing to be enabled.
    RewindFiles(ctx context.Context, userMessageID string) error

    // Close terminates the connection and cleans up resources.
    // Implements graceful shutdown: SIGTERM -> 5s wait -> SIGKILL
    Close() error

    // GetValidator returns the stream validator for diagnostics.
    GetValidator() *StreamValidator
}
```

### Implementation

The concrete implementation is in `internal/subprocess/transport.go`. Key fields shown below (simplified - actual struct has 20+ fields):

```go
type Transport struct {
    cmd             *exec.Cmd
    stdin           io.WriteCloser
    stdout          io.ReadCloser
    stderr          *os.File          // Temporary file for stderr isolation
    stderrPipe      io.ReadCloser     // Pipe for callback-based stderr handling
    parser          *parser.Parser
    msgChan         chan Message
    errChan         chan error
    validator       *StreamValidator
    controlProtocol *control.Protocol
    // ... see transport.go for complete definition
}
```

### Testing with Mocks

For unit testing, use `NewClientWithTransport()`:

```go
type mockTransport struct {
    mu           sync.Mutex
    connected    bool
    sentMessages []StreamMessage
    messages     []Message
    // ... mock behavior
}

func TestClient(t *testing.T) {
    mock := newMockTransport()
    client := NewClientWithTransport(mock)
    // ... test client behavior
}
```

## Message Interface

The `Message` interface provides polymorphic message handling through type discrimination.

```go
// Message represents any message type in the Claude Code protocol.
// All message types implement this interface.
type Message interface {
    // Type returns the message type discriminator.
    // Values: "user", "assistant", "system", "result"
    Type() string
}
```

### Concrete Message Types

```go
// UserMessage represents input from the user.
type UserMessage struct {
    MessageType     string      `json:"type"`      // Always "user"
    Content         interface{} `json:"content"`   // string or []ContentBlock
    UUID            *string     `json:"uuid,omitempty"`
    ParentToolUseID *string     `json:"parent_tool_use_id,omitempty"`
}

func (m *UserMessage) Type() string { return "user" }

// AssistantMessage represents Claude's response.
type AssistantMessage struct {
    MessageType     string                 `json:"type"`    // Always "assistant"
    Content         []ContentBlock         `json:"content"` // Text, thinking, tool use blocks
    Model           string                 `json:"model"`   // Model used for response
    Error           *AssistantMessageError `json:"error,omitempty"`
    ParentToolUseID *string                `json:"parent_tool_use_id,omitempty"` // Set when produced inside a subagent
    Usage           *map[string]any        `json:"usage,omitempty"`              // Per-API-call token usage, not a turn total
}

func (m *AssistantMessage) Type() string { return "assistant" }

// SystemMessage represents system-level messages.
type SystemMessage struct {
    MessageType string         `json:"type"`    // Always "system"
    Subtype     string         `json:"subtype"` // "init", "result", etc.
    Data        map[string]any `json:"-"`       // Preserved original data (not serialized)
}

func (m *SystemMessage) Type() string { return "system" }

// ResultMessage represents the final result of an operation.
type ResultMessage struct {
    MessageType      string          `json:"type"`                       // Always "result"
    Subtype          string          `json:"subtype"`
    DurationMs       int             `json:"duration_ms"`                // Total duration in ms
    DurationAPIMs    int             `json:"duration_api_ms"`            // API call duration
    IsError          bool            `json:"is_error"`
    NumTurns         int             `json:"num_turns"`                  // Number of conversation turns
    SessionID        string          `json:"session_id"`
    TotalCostUSD     *float64        `json:"total_cost_usd,omitempty"`   // Total cost in USD
    Usage            *map[string]any `json:"usage,omitempty"`            // Token usage details
    Result           *string         `json:"result,omitempty"`
    StructuredOutput any             `json:"structured_output,omitempty"`
}

func (m *ResultMessage) Type() string { return "result" }
```

### Type Discrimination Pattern

```go
// Processing messages with type switch
for msg := range client.ReceiveMessages(ctx) {
    switch m := msg.(type) {
    case *AssistantMessage:
        for _, block := range m.Content {
            if text, ok := block.(*TextBlock); ok {
                fmt.Print(text.Text)
            }
        }
    case *ResultMessage:
        if m.IsError {
            log.Printf("Error: %s", *m.Result)
        }
    case *UserMessage:
        // Echo of user input
    case *SystemMessage:
        // System-level events
    }
}
```

## ContentBlock Interface

The `ContentBlock` interface provides polymorphic content handling within messages.

```go
// ContentBlock represents any content block within a message.
type ContentBlock interface {
    // BlockType returns the content block type discriminator.
    // Values: "text", "thinking", "tool_use", "tool_result"
    BlockType() string
}
```

### Concrete Content Block Types

```go
// TextBlock contains text content from Claude.
type TextBlock struct {
    MessageType string `json:"type"`  // Always "text"
    Text        string `json:"text"`
}

func (b *TextBlock) BlockType() string { return "text" }

// ThinkingBlock contains Claude's reasoning (extended thinking).
type ThinkingBlock struct {
    MessageType string `json:"type"`      // Always "thinking"
    Thinking    string `json:"thinking"`
    Signature   string `json:"signature"`
}

func (b *ThinkingBlock) BlockType() string { return "thinking" }

// ToolUseBlock represents Claude requesting to use a tool.
type ToolUseBlock struct {
    MessageType string         `json:"type"`        // Always "tool_use"
    ToolUseID   string         `json:"tool_use_id"`
    Name        string         `json:"name"`
    Input       map[string]any `json:"input"`
}

func (b *ToolUseBlock) BlockType() string { return "tool_use" }

// ToolResultBlock contains the result of a tool execution.
type ToolResultBlock struct {
    MessageType string      `json:"type"`       // Always "tool_result"
    ToolUseID   string      `json:"tool_use_id"`
    Content     interface{} `json:"content"`    // string or structured data
    IsError     *bool       `json:"is_error,omitempty"`
}

func (b *ToolResultBlock) BlockType() string { return "tool_result" }
```

## Client Interface

The `Client` interface defines the public API for streaming communication.

```go
// Client provides bidirectional streaming communication with Claude Code CLI.
type Client interface {
    // Connection lifecycle
    Connect(ctx context.Context, prompt ...StreamMessage) error
    Disconnect() error

    // Query methods
    Query(ctx context.Context, prompt string) error
    QueryWithSession(ctx context.Context, prompt string, sessionID string) error
    QueryStream(ctx context.Context, messages <-chan StreamMessage) error

    // Response handling
    ReceiveMessages(ctx context.Context) <-chan Message
    ReceiveResponse(ctx context.Context) MessageIterator

    // Control operations
    Interrupt(ctx context.Context) error
    SetModel(ctx context.Context, model *string) error
    SetPermissionMode(ctx context.Context, mode PermissionMode) error
    RewindFiles(ctx context.Context, messageUUID string) error

    // Diagnostics
    GetStreamIssues() []StreamIssue
    GetStreamStats() StreamStats
    GetServerInfo(ctx context.Context) (map[string]interface{}, error)
}
```

### Method Categories

**Connection Lifecycle**
- `Connect()` - Establish connection to CLI
- `Disconnect()` - Close connection and clean up

**Query Methods**
- `Query()` - Send prompt using default session
- `QueryWithSession()` - Send prompt with specific session ID
- `QueryStream()` - Stream messages from a channel

**Response Handling**
- `ReceiveMessages()` - Get channel for streaming messages
- `ReceiveResponse()` - Get iterator for message-by-message processing

**Control Operations**
- `Interrupt()` - Stop current operation
- `SetModel()` - Change AI model mid-session
- `SetPermissionMode()` - Change permission handling
- `RewindFiles()` - Revert files to checkpoint

**Diagnostics**
- `GetStreamIssues()` - Get list of stream problems
- `GetStreamStats()` - Get stream statistics
- `GetServerInfo()` - Get CLI server information

## Control Protocol Transport

Internal interface for control protocol communication:

```go
// control.Transport abstracts I/O for the control protocol.
type Transport interface {
    // Write sends data to CLI stdin.
    Write(ctx context.Context, data []byte) error

    // Read returns channel receiving data from CLI stdout.
    Read(ctx context.Context) <-chan []byte

    // Close closes the transport.
    Close() error
}
```

This is implemented by `subprocess.ProtocolAdapter` which bridges the subprocess stdin to the control protocol.

## MessageIterator Interface

Iterator for processing messages one at a time:

```go
// MessageIterator provides sequential access to messages.
type MessageIterator interface {
    // Next returns the next message or ErrNoMoreMessages when done.
    Next(ctx context.Context) (Message, error)

    // Close releases resources associated with the iterator.
    Close() error
}
```

### Usage Pattern

```go
iterator, err := claudecode.Query(ctx, "Hello")
if err != nil {
    return err
}
defer iterator.Close()

for {
    msg, err := iterator.Next(ctx)
    if errors.Is(err, claudecode.ErrNoMoreMessages) {
        break
    }
    if err != nil {
        return err
    }
    // Process msg
}
```
