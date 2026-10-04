# Agent SDK Reference - Go

Complete API reference for the Go Agent SDK, including all functions, types, and interfaces.

---

## Installation

```bash
go get github.com/severity1/claude-agent-sdk-go
```

**Prerequisites:**
- Go 1.18+
- Node.js (for Claude Code CLI)
- Claude Code CLI: `npm install -g @anthropic-ai/claude-code`

## Choosing Between `Query()` and `Client`

The Go SDK provides two ways to interact with Claude Code:

### Quick Comparison

| Feature             | `Query()`                     | `Client`                           |
|:--------------------|:------------------------------|:-----------------------------------|
| **Session**         | Creates new session each time | Reuses same session                |
| **Conversation**    | Single exchange               | Multiple exchanges in same context |
| **Connection**      | Managed automatically         | Manual or WithClient helper        |
| **Streaming**       | Via MessageIterator           | Via channels or iterator           |
| **Interrupts**      | Not supported                 | Supported                          |
| **Hooks**           | Supported                     | Supported                          |
| **Custom Tools**    | Supported (SDK MCP servers)   | Supported                          |
| **Continue Chat**   | New session each time         | Maintains conversation             |
| **Use Case**        | One-off tasks                 | Continuous conversations           |

### When to Use `Query()` (New Session Each Time)

**Best for:**
- One-off questions where you don't need conversation history
- Independent tasks that don't require context from previous exchanges
- Simple automation scripts
- CI/CD integrations
- When you want a fresh start each time

### When to Use `Client` (Continuous Conversation)

**Best for:**
- **Continuing conversations** - When you need Claude to remember context
- **Follow-up questions** - Building on previous responses
- **Interactive applications** - Chat interfaces, REPLs
- **Response-driven logic** - When next action depends on Claude's response
- **Session control** - Managing conversation lifecycle explicitly
- **Custom tools and hooks** - Advanced integrations

---

## Functions

### `Query()`

Creates a new session for each interaction with Claude Code. Returns a MessageIterator that yields messages as they arrive. Each call to `Query()` starts fresh with no memory of previous interactions.

```go
func Query(ctx context.Context, prompt string, opts ...Option) (MessageIterator, error)
```

#### Parameters

| Parameter | Type                | Description                                      |
|:----------|:--------------------|:-------------------------------------------------|
| `ctx`     | `context.Context`   | Context for cancellation and timeouts            |
| `prompt`  | `string`            | The input prompt                                 |
| `opts`    | `...Option`         | Optional configuration (functional options)      |

#### Returns

Returns a `MessageIterator` that yields messages from the conversation, and an error if the query fails to start.

#### Example - Basic Query

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
    ctx := context.Background()

    iterator, err := claudecode.Query(ctx, "What is 2+2?")
    if err != nil {
        log.Fatalf("Query failed: %v", err)
    }
    defer iterator.Close()

    for {
        message, err := iterator.Next(ctx)
        if err != nil {
            if errors.Is(err, claudecode.ErrNoMoreMessages) {
                break
            }
            log.Fatalf("Failed to get message: %v", err)
        }

        switch msg := message.(type) {
        case *claudecode.AssistantMessage:
            for _, block := range msg.Content {
                if textBlock, ok := block.(*claudecode.TextBlock); ok {
                    fmt.Print(textBlock.Text)
                }
            }
        case *claudecode.ResultMessage:
            if msg.IsError {
                log.Printf("Error: %v", msg.Result)
            }
        }
    }
}
```

#### Example - With Options

```go
iterator, err := claudecode.Query(ctx, "Create a Python web server",
    claudecode.WithSystemPrompt("You are an expert Python developer"),
    claudecode.WithPermissionMode(claudecode.PermissionModeAcceptEdits),
    claudecode.WithCwd("/home/user/project"),
    claudecode.WithAllowedTools("Read", "Write", "Bash"),
)
```

### `QueryWithTransport()`

Query with a custom transport implementation. Primarily used for testing.

```go
func QueryWithTransport(ctx context.Context, prompt string, transport Transport, opts ...Option) (MessageIterator, error)
```

#### Parameters

| Parameter   | Type                | Description                           |
|:------------|:--------------------|:--------------------------------------|
| `ctx`       | `context.Context`   | Context for cancellation and timeouts |
| `prompt`    | `string`            | The input prompt                      |
| `transport` | `Transport`         | Custom transport implementation       |
| `opts`      | `...Option`         | Optional configuration                |

### `NewClient()`

Creates a new Client for interactive conversations.

```go
func NewClient(opts ...Option) Client
```

#### Parameters

| Parameter | Type        | Description                        |
|:----------|:------------|:-----------------------------------|
| `opts`    | `...Option` | Optional configuration             |

#### Returns

Returns a `Client` interface for managing conversations.

### `NewClientWithTransport()`

Creates a new Client with a custom transport. Primarily used for testing.

```go
func NewClientWithTransport(transport Transport, opts ...Option) Client
```

### `WithClient()`

Resource management helper that automatically handles connection lifecycle. This is the Go-idiomatic equivalent of Python's `async with` context manager.

```go
func WithClient(ctx context.Context, fn func(Client) error, opts ...Option) error
```

#### Parameters

| Parameter | Type                    | Description                              |
|:----------|:------------------------|:-----------------------------------------|
| `ctx`     | `context.Context`       | Context for cancellation and timeouts    |
| `fn`      | `func(Client) error`    | Function to execute with the client      |
| `opts`    | `...Option`             | Optional configuration                   |

#### Example

```go
err := claudecode.WithClient(ctx, func(client claudecode.Client) error {
    if err := client.Query(ctx, "Hello Claude"); err != nil {
        return err
    }

    for msg := range client.ReceiveMessages(ctx) {
        // Process messages
    }
    return nil
})
```

### `WithClientTransport()`

Resource management helper with custom transport. Primarily used for testing.

```go
func WithClientTransport(ctx context.Context, transport Transport, fn func(Client) error, opts ...Option) error
```

### `CreateSDKMcpServer()`

Create an in-process MCP server that runs within your Go application.

```go
func CreateSDKMcpServer(name, version string, tools ...*McpTool) *McpSdkServerConfig
```

#### Parameters

| Parameter | Type          | Description                              |
|:----------|:--------------|:-----------------------------------------|
| `name`    | `string`      | Unique identifier for the server         |
| `version` | `string`      | Server version string                    |
| `tools`   | `...*McpTool` | Tool functions created with `NewTool()`  |

#### Returns

Returns an `*McpSdkServerConfig` that can be passed to `WithSdkMcpServer()`.

#### Example

```go
// Define tools
addTool := claudecode.NewTool(
    "add",
    "Add two numbers",
    map[string]any{"a": "number", "b": "number"},
    func(ctx context.Context, args map[string]any) (*claudecode.McpToolResult, error) {
        a := args["a"].(float64)
        b := args["b"].(float64)
        return &claudecode.McpToolResult{
            Content: []claudecode.McpContent{{
                Type: "text",
                Text: fmt.Sprintf("Result: %v", a+b),
            }},
        }, nil
    },
)

// Create server
calculator := claudecode.CreateSDKMcpServer("calculator", "1.0.0", addTool)

// Use with Query
iterator, err := claudecode.Query(ctx, "What is 5 + 3?",
    claudecode.WithSdkMcpServer("calc", calculator),
    claudecode.WithAllowedTools("mcp__calc__add"),
)
```

### `NewTool()`

Create a new MCP tool definition.

```go
func NewTool(name, description string, inputSchema map[string]any, handler McpToolHandler, opts ...ToolOption) *McpTool
```

#### Parameters

| Parameter     | Type               | Description                              |
|:--------------|:-------------------|:-----------------------------------------|
| `name`        | `string`           | Unique identifier for the tool           |
| `description` | `string`           | Human-readable description               |
| `inputSchema` | `map[string]any`   | JSON Schema for input validation         |
| `handler`     | `McpToolHandler`   | Function that handles tool execution     |
| `opts`        | `...ToolOption`    | Optional tool settings, such as `WithToolAnnotations()` |

#### Returns

Returns an `*McpTool` that can be passed to `CreateSDKMcpServer()`.

### `WithToolAnnotations()`

Attach MCP tool annotations (behavior hints) to a tool. The annotations are sent in the `tools/list` response. A nil value omits the `annotations` key.

```go
func WithToolAnnotations(ann *ToolAnnotations) ToolOption
```

```go
readOnly := true
searchTool := claudecode.NewTool(
    "search",
    "Search the index",
    map[string]any{"query": "string"},
    searchHandler,
    claudecode.WithToolAnnotations(&claudecode.ToolAnnotations{
        ReadOnlyHint: &readOnly,
    }),
)
```

### `ListSessions()`

List metadata for past sessions, sorted by `LastModified` (newest first). This reads session transcripts on disk and does not start the CLI.

```go
func ListSessions(opts ...SessionOption) ([]SDKSessionInfo, error)
```

```go
sessions, err := claudecode.ListSessions(
    claudecode.WithSessionDirectory("/path/to/project"),
    claudecode.WithSessionLimit(10),
)
```

### `GetSessionMessages()`

Read the user and assistant messages from a session transcript.

```go
func GetSessionMessages(sessionID string, opts ...SessionOption) ([]SessionMessage, error)
```

### `GetSessionInfo()`

Get metadata for one session. Returns `nil` (not an error) when the session is not found.

```go
func GetSessionInfo(sessionID string, opts ...SessionOption) (*SDKSessionInfo, error)
```

#### Session Options

| Option | Description |
|:-------|:------------|
| `WithSessionDirectory(dir string)` | Scope the search to one project directory. When omitted, all projects are searched |
| `WithSessionLimit(n int)` | Maximum number of results |
| `WithSessionOffset(n int)` | Skip the first n messages (`GetSessionMessages` only) |
| `WithIncludeWorktrees(include bool)` | Include git worktree directories (default true; only with `WithSessionDirectory`) |

---

## Client Interface

The `Client` interface provides methods for managing interactive conversations.

```go
type Client interface {
    Connect(ctx context.Context, prompt ...StreamMessage) error
    Disconnect() error
    Query(ctx context.Context, prompt string) error
    QueryWithSession(ctx context.Context, prompt string, sessionID string) error
    QueryStream(ctx context.Context, messages <-chan StreamMessage) error
    ReceiveMessages(ctx context.Context) <-chan Message
    ReceiveResponse(ctx context.Context) MessageIterator
    Interrupt(ctx context.Context) error
    SetModel(ctx context.Context, model *string) error
    SetPermissionMode(ctx context.Context, mode PermissionMode) error
    RewindFiles(ctx context.Context, messageUUID string) error
    GetMcpStatus(ctx context.Context) (*McpStatusResponse, error)
    StopTask(ctx context.Context, taskID string) error
    GetStreamIssues() []StreamIssue
    GetStreamStats() StreamStats
    GetServerInfo(ctx context.Context) (map[string]interface{}, error)
    Done() <-chan struct{}
    Err() error
}
```

### Methods

#### `Connect()`

Establish connection to Claude Code CLI.

```go
func (c *ClientImpl) Connect(ctx context.Context, prompt ...StreamMessage) error
```

#### `Disconnect()`

Close the connection and cleanup resources.

```go
func (c *ClientImpl) Disconnect() error
```

#### `Query()`

Send a query using the default session.

```go
func (c *ClientImpl) Query(ctx context.Context, prompt string) error
```

#### `QueryWithSession()`

Send a query with a specific session ID for isolated conversations.

```go
func (c *ClientImpl) QueryWithSession(ctx context.Context, prompt string, sessionID string) error
```

#### `QueryStream()`

Stream messages from a channel to Claude.

```go
func (c *ClientImpl) QueryStream(ctx context.Context, messages <-chan StreamMessage) error
```

#### `ReceiveMessages()`

Receive all incoming messages as a channel.

```go
func (c *ClientImpl) ReceiveMessages(ctx context.Context) <-chan Message
```

#### `ReceiveResponse()`

Get an iterator for response messages until ResultMessage.

```go
func (c *ClientImpl) ReceiveResponse(ctx context.Context) MessageIterator
```

#### `Interrupt()`

Ask the CLI to stop the current turn. Sends an `interrupt` control request (Python `interrupt()`); the CLI stays connected for the next query, and it works on every OS.

```go
func (c *ClientImpl) Interrupt(ctx context.Context) error
```

#### `SetModel()`

Change the model at runtime.

```go
func (c *ClientImpl) SetModel(ctx context.Context, model *string) error
```

#### `SetPermissionMode()`

Change the permission mode at runtime.

```go
func (c *ClientImpl) SetPermissionMode(ctx context.Context, mode PermissionMode) error
```

#### `RewindFiles()`

Restore files to their state at a specific user message. Requires `WithFileCheckpointing()`.

```go
func (c *ClientImpl) RewindFiles(ctx context.Context, messageUUID string) error
```

#### `GetMcpStatus()`

Get the connection status of all configured MCP servers. Only works after `Connect()`. See [MCP Status Types](#mcp-status-types).

```go
func (c *ClientImpl) GetMcpStatus(ctx context.Context) (*McpStatusResponse, error)
```

#### `StopTask()`

Stop one running task, such as a single subagent, by the `TaskID` of its [`TaskStartedMessage`](#task-messages). The rest of the session keeps running. Sends the `stop_task` control request. Only works after `Connect()`.

```go
func (c *ClientImpl) StopTask(ctx context.Context, taskID string) error
```

The CLI then reports the task's end as a `TaskUpdatedMessage` whose status is `killed`. A `TaskNotificationMessage` with status `stopped` may follow, but the CLI sometimes omits it, so clear the task on a terminal status from either message (`IsTerminalTaskStatus`).

#### `GetStreamIssues()`

Get validation issues from the stream.

```go
func (c *ClientImpl) GetStreamIssues() []StreamIssue
```

#### `GetStreamStats()`

Get stream statistics.

```go
func (c *ClientImpl) GetStreamStats() StreamStats
```

#### `GetServerInfo()`

Get the initialize response the CLI sent during `Connect()`, as decoded JSON (Python `get_server_info()`): its slash `commands`, `output_style` and `available_output_styles`, the `models` it offers (each with `value`, `displayName`, `description` and, when supported, `supportsEffort` and `supportedEffortLevels`), `account` and other capabilities. The keys are the CLI's own and vary by CLI version. Each call returns a copy. Returns nil with a custom `Transport` that does not implement `InitializationResult()`, and an error when not connected.

```go
func (c *ClientImpl) GetServerInfo(ctx context.Context) (map[string]interface{}, error)
```

#### `Done()`

Get a channel that closes when the connected CLI process exits, on its own or through `Disconnect()`. It follows `context.Context`: once it is closed, `Err()` says why. It does not wait for the `ReceiveMessages()` channel, which can still hold messages, or stay open while a process the CLI started holds its stdout. Before `Connect()` and after `Disconnect()` the channel is closed. With a custom `Transport` that does not implement `Done()`/`Err()`, it closes on `Disconnect()`.

```go
func (c *ClientImpl) Done() <-chan struct{}
```

#### `Err()`

Get why the CLI process stopped. Returns nil while it runs, a `*ProcessError` for a non-zero exit (`ExitCode` is -1 for a signal), a `*ConnectionError` for a clean exit, and a "client not connected" error before `Connect()` and after `Disconnect()`. Once the process is gone, `Query`, `QueryWithSession`, `QueryStream`, `Interrupt`, `SetModel`, `SetPermissionMode`, `RewindFiles` and `GetMcpStatus` return a `*ConnectionError` that wraps it (Python raises `CLIConnectionError` from the exit error).

```go
func (c *ClientImpl) Err() error

select {
case <-client.Done():
    if procErr := claudecode.AsProcessError(client.Err()); procErr != nil {
        log.Printf("CLI exited with code %d", procErr.ExitCode)
    }
case <-ctx.Done():
}
```

### Client Examples

#### Continuing a Conversation

```go
err := claudecode.WithClient(ctx, func(client claudecode.Client) error {
    // First question
    if err := client.Query(ctx, "What's the capital of France?"); err != nil {
        return err
    }

    for msg := range client.ReceiveMessages(ctx) {
        if assistant, ok := msg.(*claudecode.AssistantMessage); ok {
            for _, block := range assistant.Content {
                if text, ok := block.(*claudecode.TextBlock); ok {
                    fmt.Println(text.Text)
                }
            }
        }
        if _, ok := msg.(*claudecode.ResultMessage); ok {
            break
        }
    }

    // Follow-up - Claude remembers context
    if err := client.Query(ctx, "What's the population of that city?"); err != nil {
        return err
    }

    // Process follow-up response...
    return nil
})
```

#### Using Interrupts

```go
err := claudecode.WithClient(ctx, func(client claudecode.Client) error {
    // Start a long-running task
    if err := client.Query(ctx, "Count from 1 to 100 slowly"); err != nil {
        return err
    }

    // Let it run briefly
    time.Sleep(2 * time.Second)

    // Interrupt
    if err := client.Interrupt(ctx); err != nil {
        return err
    }

    // Send new command
    return client.Query(ctx, "Just say hello instead")
}, claudecode.WithAllowedTools("Bash"), claudecode.WithPermissionMode(claudecode.PermissionModeAcceptEdits))
```

#### Session Management

```go
err := claudecode.WithClient(ctx, func(client claudecode.Client) error {
    // Session A: Math conversation
    if err := client.QueryWithSession(ctx, "Remember: x = 5", "math-session"); err != nil {
        return err
    }

    // Session B: Programming conversation (isolated)
    if err := client.QueryWithSession(ctx, "Remember: lang = Go", "prog-session"); err != nil {
        return err
    }

    // Query math session - knows about x
    if err := client.QueryWithSession(ctx, "What is x * 2?", "math-session"); err != nil {
        return err
    }

    // Query prog session - knows about lang
    return client.QueryWithSession(ctx, "What language did I mention?", "prog-session")
})
```

---

## Configuration Options

All options use the functional options pattern. Pass them to `Query()`, `NewClient()`, or `WithClient()`.

### Tool & Permission Options

#### `WithAllowedTools()`

Specify which tools Claude can use.

```go
func WithAllowedTools(tools ...string) Option
```

```go
claudecode.Query(ctx, prompt, claudecode.WithAllowedTools("Read", "Write", "Bash"))
```

#### `WithDisallowedTools()`

Specify tools Claude cannot use.

```go
func WithDisallowedTools(tools ...string) Option
```

#### `WithTools()`

Set tools as a list.

```go
func WithTools(tools ...string) Option
```

#### `WithToolsPreset()`

Use a preset tool configuration.

```go
func WithToolsPreset(preset string) Option
```

#### `WithClaudeCodeTools()`

Use Claude Code's default tool set.

```go
func WithClaudeCodeTools() Option
```

#### `WithPermissionMode()`

Set the permission mode for tool usage.

```go
func WithPermissionMode(mode PermissionMode) Option
```

Available modes:
- `PermissionModeDefault` - Standard permission behavior
- `PermissionModeAcceptEdits` - Auto-accept file edits
- `PermissionModePlan` - Planning mode, no execution
- `PermissionModeBypassPermissions` - Bypass all checks (use with caution)

```go
claudecode.Query(ctx, prompt, claudecode.WithPermissionMode(claudecode.PermissionModeAcceptEdits))
```

#### `WithPermissionPromptToolName()`

Set MCP tool name for permission prompts.

```go
func WithPermissionPromptToolName(toolName string) Option
```

### System & Model Options

#### `WithSystemPrompt()`

Set a custom system prompt.

```go
func WithSystemPrompt(prompt string) Option
```

```go
claudecode.Query(ctx, prompt, claudecode.WithSystemPrompt("You are a senior Go developer"))
```

#### `WithAppendSystemPrompt()`

Append to the default system prompt.

```go
func WithAppendSystemPrompt(prompt string) Option
```

#### `WithModel()`

Specify the Claude model to use.

```go
func WithModel(model string) Option
```

```go
claudecode.Query(ctx, prompt, claudecode.WithModel("claude-sonnet-4-5"))
```

#### `WithFallbackModel()`

Set a fallback model if primary is unavailable.

```go
func WithFallbackModel(model string) Option
```

#### `WithEffort()`

Set the effort level (`--effort`). It controls how many tokens Claude spends per response. Values other than the constants are passed through to the CLI.

```go
func WithEffort(effort EffortLevel) Option
```

```go
type EffortLevel string

const (
    EffortLow    EffortLevel = "low"
    EffortMedium EffortLevel = "medium"
    EffortHigh   EffortLevel = "high"
    EffortXHigh  EffortLevel = "xhigh"
    EffortMax    EffortLevel = "max"
)
```

```go
claudecode.Query(ctx, prompt, claudecode.WithEffort(claudecode.EffortHigh))
```

#### `WithMaxTurns()`

Limit the number of conversation turns.

```go
func WithMaxTurns(turns int) Option
```

#### `WithMaxBudgetUSD()`

Set a maximum cost budget.

```go
func WithMaxBudgetUSD(budget float64) Option
```

#### `WithThinking()`

Set the extended thinking mode. It takes precedence over `WithMaxThinkingTokens()`.

```go
func WithThinking(config ThinkingConfig) Option
```

```go
type ThinkingConfigAdaptive struct { Display ThinkingDisplay }                  // --thinking adaptive
type ThinkingConfigEnabled struct { BudgetTokens int; Display ThinkingDisplay } // --max-thinking-tokens N
type ThinkingConfigDisabled struct{}                                            // --thinking disabled

const (
    ThinkingDisplaySummarized ThinkingDisplay = "summarized" // --thinking-display summarized
    ThinkingDisplayOmitted    ThinkingDisplay = "omitted"    // --thinking-display omitted
)
```

An empty `Display` sends no `--thinking-display` flag. `ThinkingConfigDisabled` never sends it.

```go
claudecode.WithThinking(claudecode.ThinkingConfigAdaptive{Display: claudecode.ThinkingDisplaySummarized})
```

#### `WithMaxThinkingTokens()`

Deprecated: use `WithThinking(ThinkingConfigEnabled{BudgetTokens: n})`. Set the maximum tokens for thinking (`--max-thinking-tokens`). The value 0 sends no flag. The SDK sets no default, so the CLI chooses.

```go
func WithMaxThinkingTokens(tokens int) Option
```

#### `WithUser()`

Set a user identifier.

```go
func WithUser(user string) Option
```

### Session & Conversation Options

#### `WithContinueConversation()`

Continue the most recent conversation.

```go
func WithContinueConversation(continueConversation bool) Option
```

#### `WithResume()`

Resume a specific session by ID.

```go
func WithResume(sessionID string) Option
```

#### `WithResumeSessionAt()`

When resuming, load the conversation only up to and including the message with this UUID. Use it with `WithResume()` (and usually `WithForkSession(true)`) to branch from an earlier point.

```go
func WithResumeSessionAt(messageUUID string) Option
```

#### `WithResumeDropsTurn()`

With `WithResumeSessionAt()`: the UUID of the user prompt whose turn the resume discards. The CLI refuses the resume when an entry after the cut point is not part of that turn. The error message contains `Resume rejected by --resume-drops-turn:`. Do not retry the same request.

```go
func WithResumeDropsTurn(messageUUID string) Option
```

```go
claudecode.Query(ctx, prompt,
    claudecode.WithResume(sessionID),
    claudecode.WithForkSession(true),
    claudecode.WithResumeSessionAt(lastKeptUUID),
    claudecode.WithResumeDropsTurn(nextPromptUUID))
```

#### `WithForkSession()`

Fork to a new session when resuming instead of continuing.

```go
func WithForkSession(fork bool) Option
```

### Directory & Environment Options

#### `WithCwd()`

Set the current working directory.

```go
func WithCwd(cwd string) Option
```

```go
claudecode.Query(ctx, prompt, claudecode.WithCwd("/path/to/project"))
```

#### `WithAddDirs()`

Add directories Claude can access.

```go
func WithAddDirs(dirs ...string) Option
```

#### `WithEnv()`

Set environment variables.

```go
func WithEnv(env map[string]string) Option
```

```go
claudecode.Query(ctx, prompt, claudecode.WithEnv(map[string]string{
    "HTTP_PROXY": "http://proxy.example.com:8080",
}))
```

#### `WithEnvVar()`

Set a single environment variable.

```go
func WithEnvVar(key, value string) Option
```

### MCP Server Options

#### `WithMcpServers()`

Configure MCP servers.

```go
func WithMcpServers(servers map[string]McpServerConfig) Option
```

#### `WithSdkMcpServer()`

Add an in-process SDK MCP server.

```go
func WithSdkMcpServer(name string, server *McpSdkServerConfig) Option
```

```go
calculator := claudecode.CreateSDKMcpServer("calculator", "1.0.0", addTool, multiplyTool)
claudecode.Query(ctx, prompt,
    claudecode.WithSdkMcpServer("calc", calculator),
    claudecode.WithAllowedTools("mcp__calc__add", "mcp__calc__multiply"),
)
```

### Settings Options

#### `WithSettings()`

Path to a settings file.

```go
func WithSettings(settings string) Option
```

#### `WithSettingSources()`

Control which filesystem settings to load. Without this option the CLI loads its default sources (user, project and local), which include `CLAUDE.md`. Call it with no arguments to load no filesystem settings. Before v0.8.0 the default was no settings.

```go
func WithSettingSources(sources ...SettingSource) Option
```

Available sources:
- `SettingSourceUser` - `~/.claude/settings.json`
- `SettingSourceProject` - `.claude/settings.json`
- `SettingSourceLocal` - `.claude/settings.local.json`

```go
claudecode.Query(ctx, prompt, claudecode.WithSettingSources(claudecode.SettingSourceProject))

// Isolation: no filesystem settings, no CLAUDE.md
claudecode.Query(ctx, prompt, claudecode.WithSettingSources())
```

### Skills Options

When Skills are set and `WithSettingSources()` is not used, the SDK sets the setting sources to `user` and `project` so the CLI can find installed Skills.

#### `WithSkillsAll()`

Enable every discovered Skill. Adds `Skill` to the allowed tools.

```go
func WithSkillsAll() Option
```

#### `WithSkillsList()`

Enable only the named Skills. Adds `Skill(name)` to the allowed tools for each name. Use `"plugin:skill"` for plugin Skills.

```go
func WithSkillsList(names ...string) Option
```

```go
claudecode.Query(ctx, prompt, claudecode.WithSkillsList("pdf", "docx"))
```

#### `WithSkillsDisabled()`

Disable all Skills (an empty list).

```go
func WithSkillsDisabled() Option
```

#### `WithSkills()`

Set the Skills value directly: `SkillsAll` (`"all"`), a `[]string` of names, or `[]string{}`. Prefer the typed helpers above.

```go
func WithSkills(skills any) Option
```

### Advanced Options

#### `WithExtraArgs()`

Pass additional CLI arguments.

```go
func WithExtraArgs(args map[string]*string) Option
```

#### `WithCLIPath()`

Specify a custom CLI path.

```go
func WithCLIPath(path string) Option
```

#### `WithMaxBufferSize()`

Set the maximum size in bytes of one CLI output message (default 1MB). A line of exactly this size is accepted. A longer line ends the stream with a `*JSONDecodeError` that contains `JSON message exceeded maximum buffer size of N bytes`.

```go
func WithMaxBufferSize(size int) Option
```

#### `WithBetas()`

Enable beta features.

```go
func WithBetas(betas ...SdkBeta) Option
```

```go
claudecode.Query(ctx, prompt, claudecode.WithBetas(claudecode.SdkBetaContext1M))
```

### Streaming Options

#### `WithIncludePartialMessages()`

Include partial message streaming events.

```go
func WithIncludePartialMessages(include bool) Option
```

#### `WithPartialStreaming()`

Enable partial message streaming (convenience wrapper).

```go
func WithPartialStreaming() Option
```

### File Checkpointing Options

#### `WithEnableFileCheckpointing()`

Enable file change tracking for rewinding.

```go
func WithEnableFileCheckpointing(enable bool) Option
```

#### `WithFileCheckpointing()`

Enable file checkpointing (convenience wrapper).

```go
func WithFileCheckpointing() Option
```

### Agent Options

#### `WithAgents()`

Define multiple custom agents. Both `Query()` and `Client` send the agent definitions on the `initialize` control request over stdin, not as a CLI flag, so large definitions are not limited by the OS argument size.

```go
func WithAgents(agents map[string]AgentDefinition) Option
```

```go
claudecode.Query(ctx, prompt, claudecode.WithAgents(map[string]claudecode.AgentDefinition{
    "security-reviewer": {
        Description: "Reviews code for security vulnerabilities",
        Prompt:      "You are a security expert...",
        Tools:       []string{"Read", "Grep", "Glob"},
        Model:       claudecode.AgentModelSonnet,
    },
}))
```

#### `WithAgent()`

Define a single custom agent.

```go
func WithAgent(name string, agent AgentDefinition) Option
```

### Plugin Options

#### `WithPlugins()`

Configure multiple plugins.

```go
func WithPlugins(plugins []SdkPluginConfig) Option
```

#### `WithPlugin()`

Add a single plugin.

```go
func WithPlugin(plugin SdkPluginConfig) Option
```

#### `WithLocalPlugin()`

Add a local plugin by path.

```go
func WithLocalPlugin(path string) Option
```

### Sandbox Options

#### `WithSandbox()`

Configure sandbox settings.

```go
func WithSandbox(sandbox *SandboxSettings) Option
```

#### `WithSandboxEnabled()`

Enable or disable sandbox mode.

```go
func WithSandboxEnabled(enabled bool) Option
```

#### `WithAutoAllowBashIfSandboxed()`

Auto-approve bash commands when sandboxed.

```go
func WithAutoAllowBashIfSandboxed(autoAllow bool) Option
```

#### `WithSandboxExcludedCommands()`

Commands that bypass sandbox restrictions.

```go
func WithSandboxExcludedCommands(commands ...string) Option
```

#### `WithSandboxNetwork()`

Configure sandbox network settings.

```go
func WithSandboxNetwork(network *SandboxNetworkConfig) Option
```

### Output Format Options

#### `WithOutputFormat()`

Set structured output format.

```go
func WithOutputFormat(format *OutputFormat) Option
```

#### `WithJSONSchema()`

Set JSON schema for structured output.

```go
func WithJSONSchema(schema map[string]any) Option
```

```go
claudecode.Query(ctx, prompt, claudecode.WithJSONSchema(map[string]any{
    "type": "object",
    "properties": map[string]any{
        "name":  map[string]any{"type": "string"},
        "score": map[string]any{"type": "number"},
    },
    "required": []string{"name", "score"},
}))
```

### Debug Options

#### `WithDebugWriter()`

Set a writer for debug output.

```go
func WithDebugWriter(w io.Writer) Option
```

#### `WithDebugStderr()`

Write debug output to stderr.

```go
func WithDebugStderr() Option
```

#### `WithDebugDisabled()`

Disable debug output.

```go
func WithDebugDisabled() Option
```

#### `WithStderrCallback()`

Set a callback for stderr output.

```go
func WithStderrCallback(callback func(string)) Option
```

### Permission Callback Options

#### `WithCanUseTool()`

Set a callback for programmatic tool permission control.

```go
func WithCanUseTool(callback CanUseToolCallback) Option
```

When a callback is set, both `Query()` and `Client.Connect()` set `PermissionPromptToolName` to `"stdio"` so the CLI sends permission requests to the callback. If you also set `WithPermissionPromptToolName()` to a value other than `"stdio"`, `Query()` and `Connect()` return an error.

```go
claudecode.Query(ctx, prompt, claudecode.WithCanUseTool(
    func(ctx context.Context, toolName string, input map[string]any, permCtx claudecode.ToolPermissionContext) (claudecode.PermissionResult, error) {
        // Block writes to system directories
        if toolName == "Write" {
            if path, ok := input["file_path"].(string); ok && strings.HasPrefix(path, "/system/") {
                return claudecode.NewPermissionResultDeny("System writes not allowed"), nil
            }
        }
        return claudecode.NewPermissionResultAllow(), nil
    },
))
```

### Hook Options

#### `WithHooks()`

Configure multiple hooks.

```go
func WithHooks(hooks map[HookEvent][]HookMatcher) Option
```

#### `WithHook()`

Add a single hook.

```go
func WithHook(event HookEvent, matcher string, callback HookCallback) Option
```

#### `WithPreToolUseHook()`

Add a pre-tool-use hook.

```go
func WithPreToolUseHook(matcher string, callback HookCallback) Option
```

#### `WithPostToolUseHook()`

Add a post-tool-use hook.

```go
func WithPostToolUseHook(matcher string, callback HookCallback) Option
```

---

## Message Types

### `Message`

Interface implemented by all message types.

```go
type Message interface {
    Type() string
}
```

### `UserMessage`

User input message.

```go
type UserMessage struct {
    MessageType     string
    Content         interface{} // string or []ContentBlock
    UUID            *string
    ParentToolUseID *string
    ToolUseResult   map[string]any
}
```

Methods:
- `GetUUID() string` - UUID or empty string
- `GetParentToolUseID() string` - Parent tool use ID or empty string
- `HasToolUseResult() bool` - Check if tool use result metadata is present
- `GetToolUseResult() map[string]any` - Tool use result metadata (for example `filePath`, `structuredPatch`)

### `AssistantMessage`

Assistant response message with content blocks.

```go
type AssistantMessage struct {
    MessageType     string
    Content         []ContentBlock
    Model           string
    Error           *AssistantMessageError
    ParentToolUseID *string
    Usage           *map[string]any // token usage for this one API round-trip
}
```

`Usage` is the usage for a single API round-trip, not a running total. One API response can arrive as several `AssistantMessage` events with the same `Usage`, so do not sum it across messages.

Methods:
- `HasError() bool` - Check if message contains an error
- `GetError() AssistantMessageError` - Get the error type
- `IsRateLimited() bool` - Check if rate limited

### `SystemMessage`

System message with metadata.

```go
type SystemMessage struct {
    MessageType string
    Subtype     string
    Data        map[string]any
}
```

Methods (see [Task Messages](#task-messages)):
- `AsTaskStarted() (*TaskStartedMessage, bool)`
- `AsTaskProgress() (*TaskProgressMessage, bool)`
- `AsTaskNotification() (*TaskNotificationMessage, bool)`
- `AsTaskUpdated() (*TaskUpdatedMessage, bool)`

### Task Messages

The CLI reports tasks (subagents started by the Agent/Task tool, background Bash commands) as `system` messages with the subtypes `task_started`, `task_progress`, `task_notification` and `task_updated`. They arrive on the stream as `*SystemMessage`, like every other system message. The `AsTask*` methods return the typed form; each typed message embeds the `SystemMessage`, so `Subtype` and the raw `Data` (including fields the typed form does not model) stay available.

```go
switch msg := message.(type) {
case *claudecode.SystemMessage:
    if started, ok := msg.AsTaskStarted(); ok {
        fmt.Println("task started:", started.TaskID, started.Description)
    }
    if updated, ok := msg.AsTaskUpdated(); ok && updated.Status != nil &&
        claudecode.IsTerminalTaskStatus(string(*updated.Status)) {
        fmt.Println("task ended:", updated.TaskID)
    }
}
```

```go
type TaskStartedMessage struct {
    SystemMessage
    TaskID      string
    Description string
    UUID        string
    SessionID   string
    ToolUseID   *string
    TaskType    *string
}

type TaskProgressMessage struct {
    SystemMessage
    TaskID       string
    Description  string
    Usage        TaskUsage
    UUID         string
    SessionID    string
    ToolUseID    *string
    LastToolName *string
}

type TaskNotificationMessage struct {
    SystemMessage
    TaskID     string
    Status     TaskNotificationStatus // completed, failed or stopped
    OutputFile string
    Summary    string
    UUID       string
    SessionID  string
    ToolUseID  *string
    Usage      *TaskUsage
}

type TaskUpdatedMessage struct {
    SystemMessage
    TaskID    string
    Patch     map[string]any     // the task fields that changed
    Status    *TaskUpdatedStatus // Patch["status"], when it is a string
    SessionID *string
    UUID      *string
}

type TaskUsage struct {
    TotalTokens int
    ToolUses    int
    DurationMs  int
}
```

A `task_started`, `task_progress` or `task_notification` message that lacks a required field (the non-pointer fields above) is a `MessageParseError`. A `task_updated` message never fails to parse: a missing `task_id` is `""` and a `patch` that is not an object is an empty map.

```go
const (
    SystemSubtypeTaskStarted      = "task_started"
    SystemSubtypeTaskProgress     = "task_progress"
    SystemSubtypeTaskNotification = "task_notification"
    SystemSubtypeTaskUpdated      = "task_updated"

    TaskNotificationStatusCompleted TaskNotificationStatus = "completed"
    TaskNotificationStatusFailed    TaskNotificationStatus = "failed"
    TaskNotificationStatusStopped   TaskNotificationStatus = "stopped"

    TaskUpdatedStatusPending   TaskUpdatedStatus = "pending"
    TaskUpdatedStatusRunning   TaskUpdatedStatus = "running"
    TaskUpdatedStatusPaused    TaskUpdatedStatus = "paused"
    TaskUpdatedStatusCompleted TaskUpdatedStatus = "completed"
    TaskUpdatedStatusFailed    TaskUpdatedStatus = "failed"
    TaskUpdatedStatusKilled    TaskUpdatedStatus = "killed"
)

// IsTerminalTaskStatus reports whether status is completed, failed, stopped or killed.
func IsTerminalTaskStatus(status string) bool
```

### `ResultMessage`

Final result message with cost and usage information.

```go
type ResultMessage struct {
    MessageType      string
    Subtype          string
    DurationMs       int
    DurationAPIMs    int
    IsError          bool
    Errors           []string
    NumTurns         int
    SessionID        string
    TotalCostUSD     *float64
    Usage            *map[string]any
    Result           *string
    StructuredOutput any
}
```

### `StreamEvent`

Stream event for partial message updates during streaming.

```go
type StreamEvent struct {
    UUID            string
    SessionID       string
    Event           map[string]any
    ParentToolUseID *string
}
```

### `RateLimitEventMessage`

Rate limit status update from the CLI.

```go
type RateLimitEventMessage struct {
    MessageType   string
    RateLimitInfo RateLimitInfo
    UUID          string
    SessionID     string
}

type RateLimitInfo struct {
    Status          string // for example RateLimitStatusAllowed ("allowed")
    ResetsAt        int64
    RateLimitType   string
    OverageStatus   string
    OverageResetsAt int64
    IsUsingOverage  bool
}
```

Methods:
- `IsAllowed() bool` - Check if `Status` is `RateLimitStatusAllowed`

### `RawControlMessage`

Raw control protocol message.

```go
type RawControlMessage struct {
    MessageType string
    Data        map[string]any
}
```

### Message Type Constants

```go
const (
    MessageTypeUser            = "user"
    MessageTypeAssistant       = "assistant"
    MessageTypeSystem          = "system"
    MessageTypeResult          = "result"
    MessageTypeControlRequest  = "control_request"
    MessageTypeControlResponse = "control_response"
    MessageTypeStreamEvent     = "stream_event"
    MessageTypeRateLimitEvent  = "rate_limit_event"
)
```

### AssistantMessageError Types

```go
const (
    AssistantMessageErrorAuthFailed     = "authentication_failed"
    AssistantMessageErrorBilling        = "billing_error"
    AssistantMessageErrorRateLimit      = "rate_limit"
    AssistantMessageErrorInvalidRequest = "invalid_request"
    AssistantMessageErrorServer         = "server_error"
    AssistantMessageErrorUnknown        = "unknown"
)
```

---

## Content Block Types

### `ContentBlock`

Interface implemented by all content block types.

```go
type ContentBlock interface {
    BlockType() string
}
```

### `TextBlock`

Text content block.

```go
type TextBlock struct {
    MessageType string
    Text        string
}
```

### `ThinkingBlock`

Thinking content block (for models with thinking capability).

```go
type ThinkingBlock struct {
    MessageType string
    Thinking    string
    Signature   string
}
```

### `ToolUseBlock`

Tool use request block.

```go
type ToolUseBlock struct {
    MessageType string
    ToolUseID   string
    Name        string
    Input       map[string]any
}
```

### `ToolResultBlock`

Tool execution result block.

```go
type ToolResultBlock struct {
    MessageType string
    ToolUseID   string
    Content     interface{} // string or structured data
    IsError     *bool
}
```

### Content Block Type Constants

```go
const (
    ContentBlockTypeText       = "text"
    ContentBlockTypeThinking   = "thinking"
    ContentBlockTypeToolUse    = "tool_use"
    ContentBlockTypeToolResult = "tool_result"
)
```

---

## Error Types

### `SDKError`

Interface for all SDK errors.

```go
type SDKError interface {
    error
    Type() string
}
```

### `BaseError`

Base error type.

```go
type BaseError struct {
    message string
    cause   error
}
```

### `ConnectionError`

Raised when connection to Claude Code fails.

```go
type ConnectionError struct {
    BaseError
}

func NewConnectionError(message string, cause error) *ConnectionError
```

### `CLINotFoundError`

Raised when Claude Code CLI is not installed or not found.

```go
type CLINotFoundError struct {
    BaseError
    Path string
}

func NewCLINotFoundError(path, message string) *CLINotFoundError
```

### `ProcessError`

Raised when the CLI process fails.

```go
type ProcessError struct {
    BaseError
    ExitCode int
    Stderr   string
}

func NewProcessError(message string, exitCode int, stderr string) *ProcessError
```

### `JSONDecodeError`

Raised when JSON parsing fails.

```go
type JSONDecodeError struct {
    BaseError
    Line          string
    Position      int
    OriginalError error
}

func NewJSONDecodeError(line string, position int, cause error) *JSONDecodeError
```

### `MessageParseError`

Raised when message parsing fails.

```go
type MessageParseError struct {
    BaseError
    Data any
}

func NewMessageParseError(message string, data any) *MessageParseError
```

### Error Type Helper Functions

Go-native helper functions following the `os.IsNotExist` pattern from the standard library. These helpers work with wrapped errors (using `errors.As` internally).

#### Is* Functions (Boolean Checks)

```go
func IsConnectionError(err error) bool
func IsCLINotFoundError(err error) bool
func IsProcessError(err error) bool
func IsJSONDecodeError(err error) bool
func IsMessageParseError(err error) bool
```

#### As* Functions (Type Extraction)

```go
func AsConnectionError(err error) *ConnectionError
func AsCLINotFoundError(err error) *CLINotFoundError
func AsProcessError(err error) *ProcessError
func AsJSONDecodeError(err error) *JSONDecodeError
func AsMessageParseError(err error) *MessageParseError
```

### Error Handling Example

```go
iterator, err := claudecode.Query(ctx, "Hello")
if err != nil {
    // Check for specific error types using As* helpers
    if cliErr := claudecode.AsCLINotFoundError(err); cliErr != nil {
        fmt.Printf("Claude CLI not found: %v\n", cliErr)
        fmt.Println("Install with: npm install -g @anthropic-ai/claude-code")
        return
    }
    if connErr := claudecode.AsConnectionError(err); connErr != nil {
        fmt.Printf("Connection failed: %v\n", connErr)
        return
    }
    if procErr := claudecode.AsProcessError(err); procErr != nil {
        log.Fatalf("Process failed with exit code: %d\n%s", procErr.ExitCode, procErr.Stderr)
    }
    if jsonErr := claudecode.AsJSONDecodeError(err); jsonErr != nil {
        log.Fatalf("Failed to parse response: %s", jsonErr.Line)
    }
    log.Fatalf("Unexpected error: %v", err)
}
defer iterator.Close()

// Alternative: Boolean checks for simple conditionals
if claudecode.IsCLINotFoundError(err) {
    // Handle CLI not found
}
```

---

## Hook Types

### `HookEvent`

Supported hook event types.

```go
type HookEvent string

const (
    HookEventPreToolUse         HookEvent = "PreToolUse"
    HookEventPostToolUse        HookEvent = "PostToolUse"
    HookEventPostToolUseFailure HookEvent = "PostToolUseFailure"
    HookEventUserPromptSubmit   HookEvent = "UserPromptSubmit"
    HookEventStop               HookEvent = "Stop"
    HookEventSubagentStop       HookEvent = "SubagentStop"
    HookEventPreCompact         HookEvent = "PreCompact"
    HookEventNotification       HookEvent = "Notification"
    HookEventSubagentStart      HookEvent = "SubagentStart"
    HookEventPermissionRequest  HookEvent = "PermissionRequest"
)
```

Only `PreToolUse` and `PostToolUse` have convenience helpers (`WithPreToolUseHook`, `WithPostToolUseHook`). Use `WithHook(event, matcher, callback)` for the other events.

### `HookCallback`

Function signature for hook callbacks.

```go
type HookCallback func(ctx context.Context, input any, toolUseID *string, hookCtx HookContext) (HookJSONOutput, error)
```

### `HookContext`

Context information for hook callbacks.

```go
type HookContext struct {
    Signal context.Context
}
```

### `HookMatcher`

Hook matcher configuration.

```go
type HookMatcher struct {
    Matcher string
    Hooks   []HookCallback
    Timeout *float64
}
```

### `HookJSONOutput`

Output from a hook callback.

```go
type HookJSONOutput struct {
    Continue           *bool
    SuppressOutput     *bool
    StopReason         *string
    Decision           *string
    SystemMessage      *string
    Reason             *string
    HookSpecificOutput any
}
```

### Hook Input Types

The `input` argument of a `HookCallback` is a pointer to one of these types (for example `*PreToolUseHookInput`). Use a type assertion to read it.

#### `BaseHookInput`

Common fields for all hook inputs.

```go
type BaseHookInput struct {
    SessionID      string
    TranscriptPath string
    Cwd            string
    PermissionMode string
}
```

#### `PreToolUseHookInput`

Input for PreToolUse hooks.

```go
type PreToolUseHookInput struct {
    BaseHookInput
    HookEventName string
    ToolName      string
    ToolInput     map[string]any
    ToolUseID     string
}
```

#### `PostToolUseHookInput`

Input for PostToolUse hooks.

```go
type PostToolUseHookInput struct {
    BaseHookInput
    HookEventName string
    ToolName      string
    ToolInput     map[string]any
    ToolResponse  any
    ToolUseID     string
}
```

#### `PostToolUseFailureHookInput`

Input for PostToolUseFailure hooks.

```go
type PostToolUseFailureHookInput struct {
    BaseHookInput
    HookEventName string
    ToolName      string
    ToolInput     map[string]any
    ToolUseID     string
    Error         string
    IsInterrupt   *bool // nil when the CLI omits the field
}
```

#### `UserPromptSubmitHookInput`

Input for UserPromptSubmit hooks.

```go
type UserPromptSubmitHookInput struct {
    BaseHookInput
    HookEventName string
    Prompt        string
}
```

#### `StopHookInput`

Input for Stop hooks.

```go
type StopHookInput struct {
    BaseHookInput
    HookEventName  string
    StopHookActive bool
}
```

#### `SubagentStopHookInput`

Input for SubagentStop hooks.

```go
type SubagentStopHookInput struct {
    BaseHookInput
    HookEventName       string
    StopHookActive      bool
    AgentID             string
    AgentTranscriptPath string
    AgentType           string
}
```

#### `PreCompactHookInput`

Input for PreCompact hooks.

```go
type PreCompactHookInput struct {
    BaseHookInput
    HookEventName      string
    Trigger            string
    CustomInstructions *string
}
```

#### `NotificationHookInput`

Input for Notification hooks.

```go
type NotificationHookInput struct {
    BaseHookInput
    HookEventName    string
    Message          string
    Title            *string // nil when the CLI omits the field
    NotificationType string
}
```

#### `SubagentStartHookInput`

Input for SubagentStart hooks.

```go
type SubagentStartHookInput struct {
    BaseHookInput
    HookEventName string
    AgentID       string
    AgentType     string
}
```

#### `PermissionRequestHookInput`

Input for PermissionRequest hooks.

```go
type PermissionRequestHookInput struct {
    BaseHookInput
    HookEventName         string
    ToolName              string
    ToolInput             map[string]any
    PermissionSuggestions []any // nil when the CLI omits the field
}
```

### Hook-Specific Output Types

Set `HookEventName` to the event name (for example `"PreToolUse"`). Optional fields are pointers so that an unset field is left out of the JSON.

#### `PreToolUseHookSpecificOutput`

```go
type PreToolUseHookSpecificOutput struct {
    HookEventName            string
    PermissionDecision       *string // "allow", "deny", or "ask"
    PermissionDecisionReason *string
    UpdatedInput             map[string]any
    AdditionalContext        *string
}
```

#### `PostToolUseHookSpecificOutput`

```go
type PostToolUseHookSpecificOutput struct {
    HookEventName        string
    AdditionalContext    *string
    UpdatedMCPToolOutput any // wire key: updatedMCPToolOutput
}
```

#### `PostToolUseFailureHookSpecificOutput`

```go
type PostToolUseFailureHookSpecificOutput struct {
    HookEventName     string
    AdditionalContext *string
}
```

#### `UserPromptSubmitHookSpecificOutput`

```go
type UserPromptSubmitHookSpecificOutput struct {
    HookEventName     string
    AdditionalContext *string
}
```

#### `NotificationHookSpecificOutput`

```go
type NotificationHookSpecificOutput struct {
    HookEventName     string
    AdditionalContext *string
}
```

#### `SubagentStartHookSpecificOutput`

```go
type SubagentStartHookSpecificOutput struct {
    HookEventName     string
    AdditionalContext *string
}
```

#### `PermissionRequestHookSpecificOutput`

```go
type PermissionRequestHookSpecificOutput struct {
    HookEventName string
    Decision      map[string]any // required
}
```

### Hook Example

```go
err := claudecode.WithClient(ctx, func(client claudecode.Client) error {
    return client.Query(ctx, "Run ls command")
},
    claudecode.WithAllowedTools("Bash"),
    claudecode.WithPreToolUseHook("Bash", func(ctx context.Context, input any, toolUseID *string, hookCtx claudecode.HookContext) (claudecode.HookJSONOutput, error) {
        hookInput, ok := input.(*claudecode.PreToolUseHookInput)
        if !ok {
            return claudecode.HookJSONOutput{}, nil
        }
        command, _ := hookInput.ToolInput["command"].(string)

        // Block dangerous commands
        if strings.Contains(command, "rm -rf") {
            decision := "deny"
            reason := "Dangerous command blocked"
            return claudecode.HookJSONOutput{
                HookSpecificOutput: claudecode.PreToolUseHookSpecificOutput{
                    HookEventName:            "PreToolUse",
                    PermissionDecision:       &decision,
                    PermissionDecisionReason: &reason,
                },
            }, nil
        }

        return claudecode.HookJSONOutput{}, nil
    }),
)
```

---

## MCP Types

### `McpTool`

Definition for an MCP tool.

```go
type McpTool struct {
    name        string
    description string
    inputSchema map[string]any
    handler     McpToolHandler
    annotations *ToolAnnotations
}
```

Methods:
- `Name() string`
- `Description() string`
- `InputSchema() map[string]any`
- `Annotations() *ToolAnnotations`
- `Call(ctx context.Context, args map[string]any) (*McpToolResult, error)`

### `ToolOption`

Functional option for `NewTool()`.

```go
type ToolOption func(*McpTool)
```

### `ToolAnnotations`

MCP tool behavior hints, set with `WithToolAnnotations()`. All fields are optional. Do not confuse this type with `McpToolAnnotations`, which is part of the MCP status response.

```go
type ToolAnnotations struct {
    Title           *string // wire key: title
    ReadOnlyHint    *bool   // wire key: readOnlyHint
    DestructiveHint *bool   // wire key: destructiveHint
    IdempotentHint  *bool   // wire key: idempotentHint
    OpenWorldHint   *bool   // wire key: openWorldHint
}
```

### `McpToolHandler`

Function signature for tool handlers.

```go
type McpToolHandler func(ctx context.Context, args map[string]any) (*McpToolResult, error)
```

### `McpToolResult`

Result from a tool execution.

```go
type McpToolResult struct {
    Content []McpContent
    IsError bool
}
```

### `McpContent`

Content in a tool result.

```go
type McpContent struct {
    Type     string // "text" or "image"
    Text     string
    Data     string // base64 for images
    MimeType string
}
```

### `McpToolDefinition`

Tool definition for listing.

```go
type McpToolDefinition struct {
    Name        string
    Description string
    InputSchema map[string]any
    Annotations *ToolAnnotations
}
```

### `McpServer`

Interface for MCP servers. It is defined in an internal package and is not exported from `claudecode`. `SdkMcpServer` implements it.

```go
type McpServer interface {
    Name() string
    Version() string
    ListTools(ctx context.Context) ([]McpToolDefinition, error)
    CallTool(ctx context.Context, name string, args map[string]any) (*McpToolResult, error)
}
```

### `SdkMcpServer`

In-process MCP server implementation.

```go
type SdkMcpServer struct {
    name    string
    version string
    tools   map[string]*McpTool
}
```

### MCP Server Config Types

#### `McpServerType`

```go
type McpServerType string

const (
    McpServerTypeStdio McpServerType = "stdio"
    McpServerTypeSSE   McpServerType = "sse"
    McpServerTypeHTTP  McpServerType = "http"
)
```

`McpSdkServerConfig` uses the type value `"sdk"`. `CreateSDKMcpServer()` sets it for you.

#### `McpStdioServerConfig`

```go
type McpStdioServerConfig struct {
    Type    McpServerType
    Command string
    Args    []string
    Env     map[string]string
}
```

#### `McpSSEServerConfig`

```go
type McpSSEServerConfig struct {
    Type    McpServerType
    URL     string
    Headers map[string]string
}
```

#### `McpHTTPServerConfig`

```go
type McpHTTPServerConfig struct {
    Type    McpServerType
    URL     string
    Headers map[string]string
}
```

#### `McpSdkServerConfig`

```go
type McpSdkServerConfig struct {
    Type       McpServerType
    Name       string
    Instance   McpServer // not sent to the CLI
    AlwaysLoad bool
}
```

### MCP Status Types

Returned by `Client.GetMcpStatus()`.

```go
type McpStatusResponse struct {
    McpServers []McpServerStatus
}

type McpServerStatus struct {
    Name       string
    Status     McpServerConnectionStatus
    ServerInfo *McpServerInfo // set only when Status is connected
    Error      *string        // set only when Status is failed
    Config     *McpServerStatusConfig
    Scope      *string
    Tools      []McpToolInfo  // set only when Status is connected
}

type McpServerInfo struct {
    Name    string
    Version string
}

type McpToolInfo struct {
    Name        string
    Description *string
    Annotations *McpToolAnnotations
}

type McpToolAnnotations struct {
    ReadOnly    *bool
    Destructive *bool
    OpenWorld   *bool
}

type McpServerStatusConfig struct {
    Type    string // see McpServerConfigType* constants
    Command *string
    Args    []string
    URL     *string
    Headers map[string]string
    Env     map[string]string
    Name    *string
    ID      *string
}
```

```go
type McpServerConnectionStatus string

const (
    McpServerConnectionStatusConnected McpServerConnectionStatus = "connected"
    McpServerConnectionStatusFailed    McpServerConnectionStatus = "failed"
    McpServerConnectionStatusNeedsAuth McpServerConnectionStatus = "needs-auth"
    McpServerConnectionStatusPending   McpServerConnectionStatus = "pending"
    McpServerConnectionStatusDisabled  McpServerConnectionStatus = "disabled"
)

const (
    McpServerConfigTypeStdio    = "stdio"
    McpServerConfigTypeSSE      = "sse"
    McpServerConfigTypeHTTP     = "http"
    McpServerConfigTypeSDK      = "sdk"
    McpServerConfigTypeClaudeAI = "claudeai-proxy"
)
```

---

## Permission Types

### `PermissionMode`

```go
type PermissionMode string

const (
    PermissionModeDefault           PermissionMode = "default"
    PermissionModeAcceptEdits       PermissionMode = "acceptEdits"
    PermissionModePlan              PermissionMode = "plan"
    PermissionModeBypassPermissions PermissionMode = "bypassPermissions"
)
```

### `CanUseToolCallback`

```go
type CanUseToolCallback func(ctx context.Context, toolName string, input map[string]any, permCtx ToolPermissionContext) (PermissionResult, error)
```

### `ToolPermissionContext`

```go
type ToolPermissionContext struct {
    Signal      any
    Suggestions []PermissionUpdate
}
```

### `PermissionResult`

Interface for permission results (sealed).

### `PermissionResultAllow`

```go
type PermissionResultAllow struct {
    Behavior           string // always "allow"
    UpdatedInput       map[string]any
    UpdatedPermissions []PermissionUpdate
}

func NewPermissionResultAllow() PermissionResultAllow
```

### `PermissionResultDeny`

```go
type PermissionResultDeny struct {
    Behavior  string // always "deny"
    Message   string
    Interrupt bool
}

func NewPermissionResultDeny(message string) PermissionResultDeny
```

### `PermissionUpdate`

```go
type PermissionUpdate struct {
    Type        PermissionUpdateType
    Rules       []PermissionRuleValue
    Behavior    *string
    Mode        *string
    Directories []string
    Destination *string
}
```

### `PermissionUpdateType`

```go
const (
    PermissionUpdateTypeAddRules         = "addRules"
    PermissionUpdateTypeReplaceRules     = "replaceRules"
    PermissionUpdateTypeRemoveRules      = "removeRules"
    PermissionUpdateTypeSetMode          = "setMode"
    PermissionUpdateTypeAddDirectories   = "addDirectories"
    PermissionUpdateTypeRemoveDirectories = "removeDirectories"
)
```

### `PermissionRuleValue`

```go
type PermissionRuleValue struct {
    ToolName    string
    RuleContent *string
}
```

---

## Sandbox Configuration

### `SandboxSettings`

```go
type SandboxSettings struct {
    Enabled                   bool
    AutoAllowBashIfSandboxed  bool
    ExcludedCommands          []string
    AllowUnsandboxedCommands  bool
    Network                   *SandboxNetworkConfig
    IgnoreViolations          *SandboxIgnoreViolations
    EnableWeakerNestedSandbox bool
}
```

| Field | Type | Default | Description |
|:------|:-----|:--------|:------------|
| `Enabled` | `bool` | `false` | Enable sandbox mode |
| `AutoAllowBashIfSandboxed` | `bool` | `false` | Auto-approve bash when sandboxed |
| `ExcludedCommands` | `[]string` | `[]` | Commands that bypass sandbox |
| `AllowUnsandboxedCommands` | `bool` | `false` | Allow model to request unsandboxed execution |
| `Network` | `*SandboxNetworkConfig` | `nil` | Network configuration |
| `IgnoreViolations` | `*SandboxIgnoreViolations` | `nil` | Violations to ignore |
| `EnableWeakerNestedSandbox` | `bool` | `false` | Enable weaker sandbox for Docker |

### `SandboxNetworkConfig`

```go
type SandboxNetworkConfig struct {
    AllowUnixSockets    []string
    AllowAllUnixSockets bool
    AllowLocalBinding   bool
    HTTPProxyPort       *int
    SOCKSProxyPort      *int
}
```

### `SandboxIgnoreViolations`

```go
type SandboxIgnoreViolations struct {
    File    []string
    Network []string
}
```

### Sandbox Example

```go
claudecode.Query(ctx, "Build and test my project",
    claudecode.WithSandbox(&claudecode.SandboxSettings{
        Enabled:                  true,
        AutoAllowBashIfSandboxed: true,
        ExcludedCommands:         []string{"docker"},
        Network: &claudecode.SandboxNetworkConfig{
            AllowLocalBinding: true,
            AllowUnixSockets:  []string{"/var/run/docker.sock"},
        },
    }),
)
```

---

## Agent Types

### `AgentDefinition`

```go
type AgentDefinition struct {
    Description string
    Prompt      string
    Tools       []string
    Model       AgentModel
}
```

### `AgentModel`

```go
type AgentModel string

const (
    AgentModelSonnet  AgentModel = "sonnet"
    AgentModelOpus    AgentModel = "opus"
    AgentModelHaiku   AgentModel = "haiku"
    AgentModelInherit AgentModel = "inherit"
)
```

---

## Plugin Types

### `SdkPluginConfig`

```go
type SdkPluginConfig struct {
    Type SdkPluginType
    Path string
}
```

### `SdkPluginType`

```go
type SdkPluginType string

const (
    SdkPluginTypeLocal SdkPluginType = "local"
)
```

---

## Structured Output

### `OutputFormat`

```go
type OutputFormat struct {
    Type   string         // always "json_schema"
    Schema map[string]any
}
```

### Helper Function

```go
func OutputFormatJSONSchema(schema map[string]any) *OutputFormat
```

---

## Beta Features

### `SdkBeta`

```go
type SdkBeta string

const (
    SdkBetaContext1M SdkBeta = "context-1m-2025-08-07"
)
```

---

## Setting Sources

### `SettingSource`

```go
type SettingSource string

const (
    SettingSourceUser    SettingSource = "user"
    SettingSourceProject SettingSource = "project"
    SettingSourceLocal   SettingSource = "local"
)
```

---

## Stream Validation

### `StreamValidator`

Validates stream integrity and tracks tool use/result pairs.

```go
type StreamValidator struct {
    // internal fields
}

func NewStreamValidator() *StreamValidator
```

Methods:
- `TrackMessage(msg Message)` - Track a message
- `MarkStreamEnd()` - Mark stream as ended
- `GetIssues() []StreamIssue` - Get validation issues
- `GetStats() StreamStats` - Get stream statistics
- `HasIssues() bool` - Check if there are issues

### `StreamIssue`

```go
type StreamIssue struct {
    Type        string
    Description string
    ToolUseID   string
}
```

### `StreamStats`

```go
type StreamStats struct {
    ToolsRequested int
    ToolsReceived  int
    PendingTools   []string
    HasResult      bool
    StreamEnded    bool
}
```

---

## Transport Interface

### `Transport`

Interface for CLI communication (primarily for testing).

```go
type Transport interface {
    Connect(ctx context.Context) error
    SendMessage(ctx context.Context, message StreamMessage) error
    EndInput(ctx context.Context) error
    ReceiveMessages(ctx context.Context) (<-chan Message, <-chan error)
    Interrupt(ctx context.Context) error
    SetModel(ctx context.Context, model *string) error
    SetPermissionMode(ctx context.Context, mode PermissionMode) error
    RewindFiles(ctx context.Context, userMessageID string) error
    GetMcpStatus(ctx context.Context) (*McpStatusResponse, error)
    StopTask(ctx context.Context, taskID string) error
    Close() error
    GetValidator() *StreamValidator
}
```

`EndInput()` closes the write side of the transport (stdin for the subprocess transport) and leaves the receive side open. It is idempotent. `Query()` calls it after it writes the prompt, or after the first `ResultMessage` when hooks, `CanUseTool`, SDK MCP servers, or file checkpointing need the control protocol. Custom transports must implement it; a no-op `return nil` is fine for tests.

---

## MessageIterator

### `MessageIterator`

Interface for iterating over messages.

```go
type MessageIterator interface {
    Next(ctx context.Context) (Message, error)
    Close() error
}
```

### `ErrNoMoreMessages`

Sentinel error indicating no more messages.

```go
var ErrNoMoreMessages = errors.New("no more messages")
```

---

## See Also

- [Feature Parity with Python SDK](parity.md)
- [Examples Directory](../examples/README.md)
- [pkg.go.dev Reference](https://pkg.go.dev/github.com/severity1/claude-agent-sdk-go)
