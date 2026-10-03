# Design Patterns

This document describes the design patterns used throughout the SDK.

## Interface-Driven Design

All major components are defined through interfaces, enabling:
- **Testability**: Mock implementations for unit testing
- **Flexibility**: Alternative implementations possible
- **Decoupling**: Components depend on abstractions, not concretions

### Example: Transport Interface

```go
// Production code uses subprocess.Transport
transport := subprocess.New(cliPath, options)

// Tests use mock transport
type mockTransport struct {
    connected bool
    messages  []Message
}

client := NewClientWithTransport(mockTransport)
```

### Example: Message Interface

```go
// All message types implement Message interface
type Message interface {
    Type() string
}

// Enables polymorphic handling
func processMessage(msg Message) {
    switch m := msg.(type) {
    case *AssistantMessage:
        // Handle assistant response
    case *ResultMessage:
        // Handle result
    }
}
```

## Functional Options Pattern

Configuration uses the functional options pattern for:
- **Readable API**: Self-documenting option names
- **Extensibility**: New options without breaking changes
- **Defaults**: Sensible defaults, override only what's needed

### Implementation

```go
// Option type
type Option func(*Options)

// Option constructors
func WithSystemPrompt(prompt string) Option {
    return func(o *Options) {
        o.SystemPrompt = prompt
    }
}

func WithMaxTurns(turns int) Option {
    return func(o *Options) {
        o.MaxTurns = turns
    }
}

func WithAllowedTools(tools ...string) Option {
    return func(o *Options) {
        o.AllowedTools = tools
    }
}
```

### Usage

```go
// Clean, readable configuration
iterator, err := claudecode.Query(ctx, "Analyze this code",
    claudecode.WithSystemPrompt("You are a code reviewer"),
    claudecode.WithAllowedTools("Read", "Grep"),
    claudecode.WithMaxTurns(5),
)

// Or with client
client := claudecode.NewClient(
    claudecode.WithModel("claude-sonnet-4-5"),
    claudecode.WithPermissionMode(claudecode.PermissionModeAcceptEdits),
)
```

## Context-First Pattern

All blocking operations accept `context.Context` as the first parameter:
- **Cancellation**: Operations can be cancelled
- **Timeouts**: Deadline support built-in
- **Values**: Request-scoped data propagation

### Example

```go
// With timeout
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

iterator, err := claudecode.Query(ctx, "Hello")
if err != nil {
    if errors.Is(err, context.DeadlineExceeded) {
        // Handle timeout
    }
    return err
}

// With cancellation
ctx, cancel := context.WithCancel(context.Background())

go func() {
    // Cancel after user input
    <-userCancelChan
    cancel()
}()

err := client.Query(ctx, "Long running task")
if errors.Is(err, context.Canceled) {
    // Handle cancellation
}
```

## Error Wrapping Pattern

Errors are wrapped with context using `fmt.Errorf` and `%w`:
- **Chain preservation**: Original error accessible via `errors.Unwrap`
- **Context addition**: Each layer adds relevant information
- **Type checking**: `errors.Is` and `errors.As` work through chain

### Implementation

```go
// Wrapping errors with context
func (t *Transport) Connect(ctx context.Context) error {
    if err := t.cmd.Start(); err != nil {
        return fmt.Errorf("failed to start CLI process: %w", err)
    }
    return nil
}

// Structured error types
type CLINotFoundError struct {
    Path    string
    Message string
    cause   error
}

func (e *CLINotFoundError) Error() string {
    return e.Message
}

func (e *CLINotFoundError) Unwrap() error {
    return e.cause
}
```

### Usage

```go
err := client.Connect(ctx)
if err != nil {
    // Check for specific error type
    if cliErr := claudecode.AsCLINotFoundError(err); cliErr != nil {
        fmt.Printf("CLI not found at %s\n", cliErr.Path)
        fmt.Println("Install with: npm install -g @anthropic-ai/claude-code")
        return
    }

    // Check for wrapped error
    if errors.Is(err, os.ErrNotExist) {
        // Handle file not found
    }

    return fmt.Errorf("connection failed: %w", err)
}
```

## Resource Management Pattern

Go-idiomatic resource management using `WithClient`:
- **Automatic cleanup**: Resources released even on panic
- **Error preservation**: Original error not masked by cleanup errors
- **Familiar pattern**: Similar to database/sql and http patterns

### Implementation

```go
func WithClient(ctx context.Context, fn func(Client) error, opts ...Option) error {
    client := NewClient(opts...)

    if err := client.Connect(ctx); err != nil {
        return err
    }

    // Defer cleanup - ignores disconnect errors to preserve original
    defer func() {
        _ = client.Disconnect()
    }()

    return fn(client)
}
```

### Usage

```go
// Automatic resource management
err := claudecode.WithClient(ctx, func(client claudecode.Client) error {
    if err := client.Query(ctx, "First question"); err != nil {
        return err
    }

    // Process responses...

    return client.Query(ctx, "Follow-up question")
}, claudecode.WithSystemPrompt("..."))

if err != nil {
    // Handle error - client already disconnected
}
```

### Comparison with Manual Pattern

```go
// Manual pattern (error-prone)
client := claudecode.NewClient()
if err := client.Connect(ctx); err != nil {
    return err
}
defer client.Disconnect()  // Easy to forget

// WithClient pattern (recommended)
err := claudecode.WithClient(ctx, func(client claudecode.Client) error {
    // Client auto-connected, guaranteed cleanup
    return client.Query(ctx, "Hello")
})
```

## Graceful Shutdown Pattern

Process termination follows a graceful shutdown sequence:
- **SIGTERM first**: Allow clean exit
- **Grace period**: Wait for voluntary termination
- **SIGKILL fallback**: Force termination if needed

### Implementation

```go
func (t *Transport) Close() error {
    // 1. Close stdin to signal EOF
    if t.stdin != nil {
        t.stdin.Close()
    }

    // 2. Send SIGTERM
    if t.cmd.Process != nil {
        t.cmd.Process.Signal(syscall.SIGTERM)
    }

    // 3. Wait with timeout
    done := make(chan error, 1)
    go func() {
        done <- t.cmd.Wait()
    }()

    select {
    case <-done:
        // Process exited gracefully
    case <-time.After(5 * time.Second):
        // 4. Force kill if still running
        t.cmd.Process.Signal(syscall.SIGKILL)
        <-done
    }

    // 5. Clean up resources
    t.cleanup()
    return nil
}
```

## Type Discrimination Pattern

JSON union types are handled using type discrimination on the `"type"` field:
- **Delayed parsing**: Use `json.RawMessage` to defer parsing
- **Switch on type**: Determine concrete type from discriminator
- **Safe casting**: Return typed value after parsing

### Implementation

```go
func parseMessage(data []byte) (Message, error) {
    // 1. Extract discriminator
    var envelope struct {
        Type string `json:"type"`
    }
    if err := json.Unmarshal(data, &envelope); err != nil {
        return nil, err
    }

    // 2. Parse based on type
    switch envelope.Type {
    case "assistant":
        var msg AssistantMessage
        if err := json.Unmarshal(data, &msg); err != nil {
            return nil, err
        }
        return &msg, nil

    case "user":
        var msg UserMessage
        if err := json.Unmarshal(data, &msg); err != nil {
            return nil, err
        }
        return &msg, nil

    case "result":
        var msg ResultMessage
        if err := json.Unmarshal(data, &msg); err != nil {
            return nil, err
        }
        return &msg, nil

    default:
        return nil, fmt.Errorf("unknown message type: %s", envelope.Type)
    }
}
```

## Request/Response Correlation Pattern

Control protocol uses unique IDs to correlate requests with responses:
- **Unique IDs**: Each request gets a unique identifier
- **Pending map**: Track outstanding requests
- **Channel per request**: Response delivered via channel

### Implementation

```go
type Protocol struct {
    mu              sync.Mutex
    pendingRequests map[string]chan *Response
    requestCounter  int64
}

func (p *Protocol) SendRequest(ctx context.Context, req Request) (*Response, error) {
    // 1. Generate unique ID
    p.mu.Lock()
    p.requestCounter++
    reqID := fmt.Sprintf("req-%d-%d", time.Now().UnixNano(), p.requestCounter)
    req.ID = reqID

    // 2. Create response channel
    respChan := make(chan *Response, 1)
    p.pendingRequests[reqID] = respChan
    p.mu.Unlock()

    // 3. Send request
    if err := p.transport.Write(ctx, req.Marshal()); err != nil {
        p.mu.Lock()
        delete(p.pendingRequests, reqID)
        p.mu.Unlock()
        return nil, err
    }

    // 4. Wait for response
    select {
    case resp := <-respChan:
        return resp, nil
    case <-ctx.Done():
        p.mu.Lock()
        delete(p.pendingRequests, reqID)
        p.mu.Unlock()
        return nil, ctx.Err()
    }
}

func (p *Protocol) HandleResponse(resp *Response) {
    p.mu.Lock()
    if ch, ok := p.pendingRequests[resp.ID]; ok {
        delete(p.pendingRequests, resp.ID)
        p.mu.Unlock()
        ch <- resp
        return
    }
    p.mu.Unlock()
}
```

## Buffer Protection Pattern

Protect against memory exhaustion from malicious or malformed input:
- **Size limits**: Maximum buffer size enforced
- **Early rejection**: Fail fast when limit exceeded
- **Clear errors**: Informative error messages

### Implementation

```go
const MaxBufferSize = 1024 * 1024 // 1MB

func (p *Parser) ProcessLine(line string) ([]Message, error) {
    p.mu.Lock()
    defer p.mu.Unlock()

    // Check buffer size before adding
    if p.buffer.Len()+len(line) > p.maxBufferSize {
        return nil, NewBufferOverflowError(p.maxBufferSize, nil) // "JSON message exceeded maximum buffer size of N bytes"
    }

    p.buffer.WriteString(line)
    // ... continue processing
}
```
