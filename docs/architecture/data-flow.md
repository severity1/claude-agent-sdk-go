# Data Flow

This document describes how data flows through the SDK for different API patterns.

## Query API Flow (One-Shot)

The Query API is designed for single-shot operations where you send a prompt and iterate through the response.

```
Application
    │
    ▼
Query(ctx, "prompt", opts...)
    │
    ├─1─► createQueryTransport()
    │         │
    │         └─► subprocess.NewWithPrompt(cliPath, options, prompt)
    │
    ├─2─► transport.Connect(ctx)
    │         │
    │         ├─► Spawn CLI subprocess with prompt as argument
    │         ├─► Create stdout/stderr pipes
    │         └─► Start handleStdout() goroutine
    │
    └─3─► Return MessageIterator
              │
              ▼
         iterator.Next(ctx)  [loop]
              │
              ├─► Read from message channel
              ├─► parser.ProcessLine(line)
              │       │
              │       └─► Type discrimination on "type" field
              │               ├─► "assistant" → AssistantMessage
              │               ├─► "user" → UserMessage
              │               ├─► "system" → SystemMessage
              │               └─► "result" → ResultMessage
              │
              └─► Return Message (or ErrNoMoreMessages when done)
              │
              ▼
         iterator.Close()
              │
              └─► transport.Close()
                      │
                      ├─► Send SIGTERM
                      ├─► Wait 5 seconds
                      ├─► Send SIGKILL (if still running)
                      └─► Clean up resources
```

### Sequence Diagram

```
┌─────┐          ┌─────────┐          ┌───────────┐          ┌─────────┐
│ App │          │ Query() │          │ Transport │          │   CLI   │
└──┬──┘          └────┬────┘          └─────┬─────┘          └────┬────┘
   │                  │                     │                     │
   │ Query(prompt)    │                     │                     │
   │─────────────────►│                     │                     │
   │                  │ Connect()           │                     │
   │                  │────────────────────►│                     │
   │                  │                     │ exec.Command()      │
   │                  │                     │────────────────────►│
   │                  │                     │                     │
   │ iterator         │                     │◄─ stdout stream ────│
   │◄─────────────────│                     │                     │
   │                  │                     │                     │
   │ Next()           │                     │                     │
   │─────────────────►│ Read from channel   │                     │
   │                  │────────────────────►│ JSON message        │
   │                  │                     │◄────────────────────│
   │ Message          │◄────────────────────│                     │
   │◄─────────────────│                     │                     │
   │                  │                     │                     │
   │ Close()          │                     │                     │
   │─────────────────►│ Close()             │                     │
   │                  │────────────────────►│ SIGTERM             │
   │                  │                     │────────────────────►│
   │                  │                     │ (wait 5s)           │
   │                  │                     │────────────────────►│
   │                  │                     │                     │
└──┴──┘          └────┴────┘          └─────┴─────┘          └────┴────┘
```

## Client API Flow (Streaming)

The Client API maintains a persistent connection for multi-turn conversations.

```
Application
    │
    ▼
NewClient(opts...)
    │
    └─► Create ClientImpl with options
              │
              ▼
         client.Connect(ctx)
              │
              ├─1─► cli.FindCLI() - Locate Claude CLI binary
              │
              ├─2─► Build command with options
              │
              ├─3─► Spawn subprocess (closeStdin=false for streaming)
              │         │
              │         ├─► Create stdin/stdout/stderr pipes
              │         └─► Start handleStdout() goroutine
              │
              ├─4─► Initialize control protocol (if hooks/permissions)
              │         │
              │         └─► Send Initialize request, await response
              │
              └─5─► Return nil (connected)
              │
              ▼
         client.Query(ctx, "prompt")
              │
              └─► transport.SendMessage(ctx, StreamMessage{
                      Type: "user",
                      Content: "prompt",
                  })
              │
              ▼
         client.ReceiveMessages(ctx) → <-chan Message
              │
              │  [goroutine: handleStdout()]
              │       │
              │       ├─► Read line from stdout
              │       ├─► parser.ProcessLine(line)
              │       ├─► Is control message?
              │       │       │
              │       │       YES─► control.Protocol.HandleIncomingMessageAsync()
              │       │       │         │
              │       │       │         ├─► control_response (inline)
              │       │       │         │       ├─► Match request ID
              │       │       │         │       └─► Route to pending channel
              │       │       │         │
              │       │       │         └─► control_request (own goroutine)
              │       │       │                 ├─► Run hook / permission / MCP handler
              │       │       │                 └─► Write control_response to stdin
              │       │       │
              │       │       NO──► Send to msgChan
              │       │
              │       └─► validator.TrackMessage()
              │
              └─► Application receives messages from channel
              │
              ▼
         client.Disconnect()
              │
              └─► transport.Close()
                      │
                      └─► Graceful shutdown sequence
```

Incoming control requests are dispatched asynchronously: `HandleIncomingMessageAsync()`
runs each `control_request` on its own goroutine so the reader never blocks on a user
callback. A slow hook, permission check, or SDK MCP tool handler therefore cannot stall
later messages on the same stream. Control responses and regular messages are still
routed inline, in read order.

### WithClient Pattern

Go-idiomatic resource management:

```
WithClient(ctx, fn, opts...)
    │
    ├─1─► NewClient(opts...)
    │
    ├─2─► client.Connect(ctx)
    │
    ├─3─► fn(client)  [execute user function]
    │         │
    │         └─► User code uses client
    │
    └─4─► defer client.Disconnect()
              │
              ├─► Guaranteed cleanup (even if fn panics)
              └─► Ignores disconnect errors
```

## Control Protocol Flow

Bidirectional communication for advanced features. Requests arriving from the CLI
(permission checks, hooks, SDK MCP calls) each run on their own goroutine, so several
can be in flight at once and the read loop keeps draining the stream while they work:

```
                        SDK                                   CLI
                         │                                     │
    ┌────────────────────┼─────────────────────────────────────┼──────────────────┐
    │                    │         Initialize Handshake         │                  │
    │                    │                                     │                  │
    │ Protocol.Start()   │                                     │                  │
    │        │           │ {"type":"control_request",          │                  │
    │        └──────────►│  "subtype":"initialize", ...}       │                  │
    │                    │────────────────────────────────────►│                  │
    │                    │                                     │                  │
    │                    │ {"type":"control_response",         │                  │
    │                    │  "subtype":"initialize", ...}       │                  │
    │                    │◄────────────────────────────────────│                  │
    │        ┌───────────│                                     │                  │
    │        ▼           │                                     │                  │
    │ Store capabilities │                                     │                  │
    └────────────────────┼─────────────────────────────────────┼──────────────────┘
                         │                                     │
    ┌────────────────────┼─────────────────────────────────────┼──────────────────┐
    │                    │         Permission Request           │                  │
    │                    │                                     │                  │
    │                    │ {"type":"control_request",          │                  │
    │                    │  "subtype":"can_use_tool",          │                  │
    │                    │  "tool":"Bash", ...}                │                  │
    │                    │◄────────────────────────────────────│                  │
    │        ┌───────────│                                     │                  │
    │        ▼           │                                     │                  │
    │ canUseToolCallback │                                     │                  │
    │        │           │                                     │                  │
    │        ▼           │                                     │                  │
    │ Return decision    │ {"type":"control_response",         │                  │
    │        └──────────►│  "result":"allow", ...}             │                  │
    │                    │────────────────────────────────────►│                  │
    └────────────────────┼─────────────────────────────────────┼──────────────────┘
                         │                                     │
    ┌────────────────────┼─────────────────────────────────────┼──────────────────┐
    │                    │         Hook Execution               │                  │
    │                    │                                     │                  │
    │                    │ {"type":"control_request",          │                  │
    │                    │  "subtype":"hook",                  │                  │
    │                    │  "event":"PreToolUse", ...}         │                  │
    │                    │◄────────────────────────────────────│                  │
    │        ┌───────────│                                     │                  │
    │        ▼           │                                     │                  │
    │ Match hook         │                                     │                  │
    │ Execute callback   │                                     │                  │
    │        │           │                                     │                  │
    │        ▼           │                                     │                  │
    │ Return result      │ {"type":"control_response",         │                  │
    │        └──────────►│  "result":{...}, ...}               │                  │
    │                    │────────────────────────────────────►│                  │
    └────────────────────┼─────────────────────────────────────┼──────────────────┘
                         │                                     │
    ┌────────────────────┼─────────────────────────────────────┼──────────────────┐
    │                    │         Control Request (SDK→CLI)    │                  │
    │                    │                                     │                  │
    │ SetModel("opus")   │                                     │                  │
    │        │           │ {"type":"control_request",          │                  │
    │        └──────────►│  "subtype":"set_model",             │                  │
    │                    │  "model":"opus", ...}               │                  │
    │                    │────────────────────────────────────►│                  │
    │                    │                                     │                  │
    │                    │ {"type":"control_response",         │                  │
    │                    │  "success":true, ...}               │                  │
    │                    │◄────────────────────────────────────│                  │
    │        ┌───────────│                                     │                  │
    │        ▼           │                                     │                  │
    │ Return success     │                                     │                  │
    └────────────────────┼─────────────────────────────────────┼──────────────────┘
```

## Message Parsing Flow

How JSON messages are parsed from CLI stdout:

```
stdout line: {"type":"assistant","content":[{"type":"text","text":"Hello"}]}
    │
    ▼
parser.ProcessLine(line)
    │
    ├─► Check buffer + line length < 1MB
    │
    ├─► Append to buffer
    │
    ├─► Attempt json.Unmarshal()
    │       │
    │       ├─► SUCCESS: Parse complete
    │       │       │
    │       │       └─► Clear buffer, continue
    │       │
    │       └─► INCOMPLETE: Wait for more data
    │               │
    │               └─► Return nil, continue accumulating
    │
    └─► Extract "type" field
            │
            ├─► "assistant" → parseAssistantMessage()
            │       │
            │       └─► Parse content blocks:
            │               ├─► "text" → TextBlock
            │               ├─► "thinking" → ThinkingBlock
            │               ├─► "tool_use" → ToolUseBlock
            │               └─► "tool_result" → ToolResultBlock
            │
            ├─► "user" → parseUserMessage()
            │
            ├─► "system" → parseSystemMessage()
            │
            ├─► "result" → parseResultMessage()
            │
            ├─► "control_response" → route to control.Protocol
            │
            └─► "stream_event" → parseStreamEvent()
```

## Error Flow

How errors propagate through the system:

```
Error Source                    Error Type                      Handling
───────────────────────────────────────────────────────────────────────────
CLI not found         →    CLINotFoundError           →    AsCLINotFoundError()
                                │
                                └─► Contains installation instructions

Connection failed     →    ConnectionError            →    AsConnectionError()
                                │
                                └─► Contains exit code, stderr

JSON parse failed     →    MessageParseError          →    AsMessageParseError()
                                │
                                └─► Contains raw JSON data

Context cancelled     →    context.Canceled           →    errors.Is(err, context.Canceled)

Timeout               →    context.DeadlineExceeded   →    errors.Is(err, context.DeadlineExceeded)
```
