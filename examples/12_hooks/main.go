// Package main demonstrates the Hook System for lifecycle events.
//
// This example shows how to use hooks to intercept and respond to
// lifecycle events during Claude Code CLI execution. Hooks enable:
// - Logging and auditing of tool usage (PreToolUse, PostToolUse)
// - Blocking dangerous commands before execution
// - Adding context to tool responses
// - Monitoring session lifecycle events
//
// Hook events supported:
// - PreToolUse: Before a tool executes (can block or modify input)
// - PostToolUse: After a tool executes (can add context)
// - PostToolUseFailure: After a tool fails (can inject recovery context)
// - UserPromptSubmit: When user submits a prompt
// - Stop: When session is stopping
// - SubagentStop: When a subagent is stopping
// - PreCompact: Before context compaction
// - Notification: When the CLI emits a notification
// - SubagentStart: When a subagent starts
// - PermissionRequest: When a permission is requested
//
// NOTE: Hooks are invoked when the CLI sends hook callback requests
// to the SDK. The callbacks demonstrate the correct API usage pattern
// for handling these lifecycle events.
//
// Run: go run main.go
package main

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	claudecode "github.com/severity1/claude-agent-sdk-go"
)

// exampleDir returns the directory containing this source file.
func exampleDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Dir(file)
}

func main() {
	fmt.Println("Claude Agent SDK - Hook System Example")
	fmt.Println("======================================")
	fmt.Println()

	// Example 1: Basic tool logging with PreToolUse and PostToolUse hooks
	fmt.Println("--- Example 1: Tool Logging Hooks ---")
	fmt.Println("Hook: Log all tool usage before and after execution")
	fmt.Println()
	runToolLoggingExample()

	// Example 2: Blocking dangerous commands
	fmt.Println()
	fmt.Println("--- Example 2: Command Blocking Hook ---")
	fmt.Println("Hook: Block dangerous bash commands before execution")
	fmt.Println()
	runBlockingExample()

	// Example 3: Adding context to tool responses
	fmt.Println()
	fmt.Println("--- Example 3: Context Injection Hook ---")
	fmt.Println("Hook: Add timing information after tool execution")
	fmt.Println()
	runContextInjectionExample()

	// Example 4: Recovering from tool failures
	fmt.Println()
	fmt.Println("--- Example 4: Tool Failure Recovery Hook ---")
	fmt.Println("Hook: Inject recovery context when a Bash command fails")
	fmt.Println()
	runFailureRecoveryExample()

	// Example 5: Observing CLI notifications
	fmt.Println()
	fmt.Println("--- Example 5: Notification Hook ---")
	fmt.Println("Hook: Observe CLI-emitted notifications via the generic WithHook API")
	fmt.Println()
	runNotificationExample()

	fmt.Println()
	fmt.Println("Hook system examples completed!")
}

// runToolLoggingExample demonstrates logging tool usage with hooks
func runToolLoggingExample() {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Thread-safe log storage
	var toolLog []ToolLogEntry
	var logMu sync.Mutex

	// PreToolUse hook - log before execution
	preToolHook := claudecode.WithPreToolUseHook("", func(
		_ context.Context,
		input any,
		_ *string,
		_ claudecode.HookContext,
	) (claudecode.HookJSONOutput, error) {
		preInput, ok := input.(*claudecode.PreToolUseHookInput)
		if !ok {
			return claudecode.HookJSONOutput{}, nil
		}

		entry := ToolLogEntry{
			Timestamp: time.Now(),
			Tool:      preInput.ToolName,
			Phase:     "PRE",
		}

		logMu.Lock()
		toolLog = append(toolLog, entry)
		logMu.Unlock()

		fmt.Printf("  [PRE]  Tool: %-10s | Session: %s\n",
			preInput.ToolName, truncate(preInput.SessionID, 12))

		return claudecode.HookJSONOutput{}, nil
	})

	// PostToolUse hook - log after execution
	postToolHook := claudecode.WithPostToolUseHook("", func(
		_ context.Context,
		input any,
		_ *string,
		_ claudecode.HookContext,
	) (claudecode.HookJSONOutput, error) {
		postInput, ok := input.(*claudecode.PostToolUseHookInput)
		if !ok {
			return claudecode.HookJSONOutput{}, nil
		}

		entry := ToolLogEntry{
			Timestamp: time.Now(),
			Tool:      postInput.ToolName,
			Phase:     "POST",
		}

		logMu.Lock()
		toolLog = append(toolLog, entry)
		logMu.Unlock()

		fmt.Printf("  [POST] Tool: %-10s | Response type: %T\n",
			postInput.ToolName, postInput.ToolResponse)

		return claudecode.HookJSONOutput{}, nil
	})

	fmt.Println("Asking Claude to read a file...")

	err := claudecode.WithClient(ctx, func(client claudecode.Client) error {
		if err := client.Query(ctx, "Read the file demo/sample.txt and tell me what it contains in one sentence."); err != nil {
			return err
		}
		return streamResponse(ctx, client)
	}, preToolHook, postToolHook, claudecode.WithMaxTurns(3), claudecode.WithCwd(exampleDir()))

	if err != nil {
		if cliErr := claudecode.AsCLINotFoundError(err); cliErr != nil {
			fmt.Printf("Claude CLI not found: %v\n", cliErr)
			fmt.Println("Install with: npm install -g @anthropic-ai/claude-code")
			return
		}
		if connErr := claudecode.AsConnectionError(err); connErr != nil {
			fmt.Printf("Connection failed: %v\n", connErr)
			return
		}
		fmt.Printf("Error: %v\n", err)
	}

	// Print log summary
	fmt.Println("\n--- Tool Log Summary ---")
	logMu.Lock()
	for i, entry := range toolLog {
		fmt.Printf("  %d. [%s] %s at %s\n",
			i+1, entry.Phase, entry.Tool, entry.Timestamp.Format("15:04:05.000"))
	}
	fmt.Printf("Total hook invocations: %d\n", len(toolLog))
	logMu.Unlock()
}

// runBlockingExample demonstrates blocking dangerous commands with PreToolUse hooks
func runBlockingExample() {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Dangerous command patterns to block
	dangerousPatterns := []string{"rm -rf", "sudo", "chmod 777", "> /dev/"}

	// PreToolUse hook that blocks dangerous Bash commands
	blockingHook := claudecode.WithPreToolUseHook("Bash", func(
		_ context.Context,
		input any,
		_ *string,
		_ claudecode.HookContext,
	) (claudecode.HookJSONOutput, error) {
		preInput, ok := input.(*claudecode.PreToolUseHookInput)
		if !ok {
			return claudecode.HookJSONOutput{}, nil
		}

		// Extract command from tool input
		command, ok := preInput.ToolInput["command"].(string)
		if !ok {
			return claudecode.HookJSONOutput{}, nil
		}

		// Check for dangerous patterns
		for _, pattern := range dangerousPatterns {
			if strings.Contains(strings.ToLower(command), strings.ToLower(pattern)) {
				fmt.Printf("  [BLOCK] Dangerous command detected: %q\n", truncate(command, 50))

				// Block the command
				decision := "block"
				reason := fmt.Sprintf("Command blocked: contains dangerous pattern '%s'", pattern)
				return claudecode.HookJSONOutput{
					Decision: &decision,
					Reason:   &reason,
				}, nil
			}
		}

		fmt.Printf("  [ALLOW] Safe command: %q\n", truncate(command, 50))
		return claudecode.HookJSONOutput{}, nil
	})

	fmt.Println("Asking Claude to run some commands (dangerous ones will be blocked)...")

	err := claudecode.WithClient(ctx, func(client claudecode.Client) error {
		if err := client.Query(ctx, "Run 'ls -la demo/' to list files, then run 'echo hello' to test."); err != nil {
			return err
		}
		return streamResponse(ctx, client)
	}, blockingHook, claudecode.WithMaxTurns(5), claudecode.WithCwd(exampleDir()))

	if err != nil {
		if cliErr := claudecode.AsCLINotFoundError(err); cliErr != nil {
			fmt.Printf("Claude CLI not found: %v\n", cliErr)
			fmt.Println("Install with: npm install -g @anthropic-ai/claude-code")
			return
		}
		if connErr := claudecode.AsConnectionError(err); connErr != nil {
			fmt.Printf("Connection failed: %v\n", connErr)
			return
		}
		fmt.Printf("Error: %v\n", err)
	}
}

// runContextInjectionExample demonstrates adding context after tool execution
func runContextInjectionExample() {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Track tool execution timing
	toolStartTimes := make(map[string]time.Time)
	var timingMu sync.Mutex

	// PreToolUse hook - record start time
	preHook := claudecode.WithPreToolUseHook("", func(
		_ context.Context,
		_ any,
		toolUseID *string,
		_ claudecode.HookContext,
	) (claudecode.HookJSONOutput, error) {
		if toolUseID == nil {
			return claudecode.HookJSONOutput{}, nil
		}

		timingMu.Lock()
		toolStartTimes[*toolUseID] = time.Now()
		timingMu.Unlock()

		return claudecode.HookJSONOutput{}, nil
	})

	// PostToolUse hook - add timing context
	postHook := claudecode.WithPostToolUseHook("", func(
		_ context.Context,
		input any,
		toolUseID *string,
		_ claudecode.HookContext,
	) (claudecode.HookJSONOutput, error) {
		postInput, ok := input.(*claudecode.PostToolUseHookInput)
		if !ok || toolUseID == nil {
			return claudecode.HookJSONOutput{}, nil
		}

		timingMu.Lock()
		startTime, exists := toolStartTimes[*toolUseID]
		delete(toolStartTimes, *toolUseID)
		timingMu.Unlock()

		if !exists {
			return claudecode.HookJSONOutput{}, nil
		}

		duration := time.Since(startTime)
		context := fmt.Sprintf("Tool %s completed in %v", postInput.ToolName, duration)
		fmt.Printf("  [TIMING] %s\n", context)

		// Add timing context for Claude
		return claudecode.HookJSONOutput{
			HookSpecificOutput: claudecode.PostToolUseHookSpecificOutput{
				HookEventName:     "PostToolUse",
				AdditionalContext: &context,
			},
		}, nil
	})

	fmt.Println("Asking Claude to perform file operations (timing will be tracked)...")

	err := claudecode.WithClient(ctx, func(client claudecode.Client) error {
		if err := client.Query(ctx, "List the files in demo/ directory and read sample.txt if it exists."); err != nil {
			return err
		}
		return streamResponse(ctx, client)
	}, preHook, postHook, claudecode.WithMaxTurns(5), claudecode.WithCwd(exampleDir()))

	if err != nil {
		if cliErr := claudecode.AsCLINotFoundError(err); cliErr != nil {
			fmt.Printf("Claude CLI not found: %v\n", cliErr)
			fmt.Println("Install with: npm install -g @anthropic-ai/claude-code")
			return
		}
		if connErr := claudecode.AsConnectionError(err); connErr != nil {
			fmt.Printf("Connection failed: %v\n", connErr)
			return
		}
		fmt.Printf("Error: %v\n", err)
	}
}

// runFailureRecoveryExample demonstrates injecting recovery context after a tool failure.
//
// The PostToolUseFailure event fires when a tool invocation exits non-zero (Bash) or
// otherwise reports an error. Unlike PostToolUse, it gives the hook a chance to attach
// guidance Claude can use on its next turn - file path hints, retry advice, etc.
//
// User-initiated interrupts (IsInterrupt non-nil and dereferences to true) are
// treated specially: the hook stays silent rather than encouraging a retry, since
// Ctrl+C signals stop intent. A nil IsInterrupt means the CLI omitted the field
// (Python NotRequired[bool] semantics) and must not be treated as an interrupt.
func runFailureRecoveryExample() {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// PostToolUseFailure hook - inject recovery context when Bash fails.
	// Registered via generic WithHook because no convenience helper exists for this event.
	failureHook := claudecode.WithHook(
		claudecode.HookEventPostToolUseFailure,
		"Bash",
		recoveryCallback,
	)

	fmt.Println("Asking Claude to run a Bash command that will fail...")

	err := claudecode.WithClient(ctx, func(client claudecode.Client) error {
		if err := client.Query(ctx, "Run 'cat /nonexistent-file' and tell me what happens."); err != nil {
			return err
		}
		return streamResponse(ctx, client)
	}, failureHook, claudecode.WithMaxTurns(3), claudecode.WithCwd(exampleDir()))

	if err != nil {
		if cliErr := claudecode.AsCLINotFoundError(err); cliErr != nil {
			fmt.Printf("Claude CLI not found: %v\n", cliErr)
			fmt.Println("Install with: npm install -g @anthropic-ai/claude-code")
			return
		}
		if connErr := claudecode.AsConnectionError(err); connErr != nil {
			fmt.Printf("Connection failed: %v\n", connErr)
			return
		}
		fmt.Printf("Error: %v\n", err)
	}
}

// runNotificationExample demonstrates observing CLI-emitted notifications.
//
// The Notification event fires when the CLI surfaces a system-level notification
// (e.g., permission prompts, rate-limit warnings). Unlike Pre/PostToolUse, there
// is no user-controlled way to force-trigger one from a single query - the hook
// observes whatever the CLI decides to notify about during the session.
//
// Registered via generic WithHook because no convenience helper exists for this event.
func runNotificationExample() {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	notificationHook := claudecode.WithHook(
		claudecode.HookEventNotification,
		"", // matcher unused for non-tool events
		notificationCallback,
	)

	fmt.Println("Running a query; any CLI notifications will be observed by the hook...")

	err := claudecode.WithClient(ctx, func(client claudecode.Client) error {
		if err := client.Query(ctx, "Briefly summarize the current directory."); err != nil {
			return err
		}
		return streamResponse(ctx, client)
	}, notificationHook, claudecode.WithMaxTurns(2), claudecode.WithCwd(exampleDir()))

	if err != nil {
		if cliErr := claudecode.AsCLINotFoundError(err); cliErr != nil {
			fmt.Printf("Claude CLI not found: %v\n", cliErr)
			fmt.Println("Install with: npm install -g @anthropic-ai/claude-code")
			return
		}
		if connErr := claudecode.AsConnectionError(err); connErr != nil {
			fmt.Printf("Connection failed: %v\n", connErr)
			return
		}
		fmt.Printf("Error: %v\n", err)
	}
}

// notificationCallback is the Notification handler for runNotificationExample.
// Logs the notification fields with a nil-guard on the optional Title.
func notificationCallback(
	_ context.Context,
	input any,
	_ *string,
	_ claudecode.HookContext,
) (claudecode.HookJSONOutput, error) {
	notif, ok := input.(*claudecode.NotificationHookInput)
	if !ok {
		return claudecode.HookJSONOutput{}, nil
	}

	title := "(none)"
	if notif.Title != nil {
		title = *notif.Title
	}
	fmt.Printf("  [NOTIFY] type=%s title=%q message=%q\n",
		notif.NotificationType, title, truncate(notif.Message, 80))

	return claudecode.HookJSONOutput{}, nil
}

// recoveryCallback is the PostToolUseFailure handler for runFailureRecoveryExample.
// Skips context injection when IsInterrupt is true to respect user stop intent.
func recoveryCallback(
	_ context.Context,
	input any,
	_ *string,
	_ claudecode.HookContext,
) (claudecode.HookJSONOutput, error) {
	failInput, ok := input.(*claudecode.PostToolUseFailureHookInput)
	if !ok {
		return claudecode.HookJSONOutput{}, nil
	}

	if failInput.IsInterrupt != nil && *failInput.IsInterrupt {
		fmt.Printf("  [INTERRUPT] Tool %s cancelled by user; no recovery hint injected\n",
			failInput.ToolName)
		return claudecode.HookJSONOutput{}, nil
	}

	fmt.Printf("  [RECOVERY] Tool %s failed: %s\n",
		failInput.ToolName, truncate(failInput.Error, 80))

	return claudecode.HookJSONOutput{
		HookSpecificOutput: claudecode.PostToolUseFailureHookSpecificOutput{
			HookEventName:     "PostToolUseFailure",
			AdditionalContext: ptrTo("The previous command failed. Verify the file path exists before retrying, or try a different approach."),
		},
	}, nil
}

// ptrTo returns a pointer to the given value. Useful for *string fields like AdditionalContext.
func ptrTo[T any](v T) *T { return &v }

// ToolLogEntry represents a logged tool usage event
type ToolLogEntry struct {
	Timestamp time.Time
	Tool      string
	Phase     string // "PRE" or "POST"
}

// streamResponse reads and displays messages from the client
func streamResponse(ctx context.Context, client claudecode.Client) error {
	msgChan := client.ReceiveMessages(ctx)

	for {
		select {
		case message := <-msgChan:
			if message == nil {
				return nil
			}

			switch msg := message.(type) {
			case *claudecode.AssistantMessage:
				for _, block := range msg.Content {
					if textBlock, ok := block.(*claudecode.TextBlock); ok {
						text := textBlock.Text
						if len(text) > 150 {
							text = text[:150] + "..."
						}
						fmt.Printf("Response: %s\n", strings.ReplaceAll(text, "\n", " "))
					}
				}
			case *claudecode.ResultMessage:
				if msg.IsError {
					if msg.Result != nil {
						return fmt.Errorf("error: %s", *msg.Result)
					}
					return fmt.Errorf("error: unknown error")
				}
				return nil
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// truncate shortens a string to maxLen characters
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
