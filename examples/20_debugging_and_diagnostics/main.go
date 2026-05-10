// Package main demonstrates Debugging and Diagnostics features.
//
// This example shows how to configure debugging output, environment
// variables, and diagnostics for development and production monitoring.
// Debugging and diagnostics enable:
// - Capturing CLI debug output for troubleshooting
// - Setting environment variables for the subprocess
// - Monitoring stderr output in real-time
// - Checking connection health and statistics
//
// Key components:
// - WithDebugWriter: Redirect debug output to an io.Writer
// - WithDebugStderr: Convenience to send debug to os.Stderr
// - WithDebugDisabled: Disable all debug output
// - WithStderrCallback: Line-by-line stderr monitoring
// - WithEnv: Set multiple environment variables
// - WithEnvVar: Set a single environment variable
// - GetServerInfo: Get connection status information
// - GetStreamStats: Get streaming statistics
// - GetStreamIssues: Get validation issues from stream
//
// Run: go run main.go
package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	claudecode "github.com/severity1/claude-agent-sdk-go"
)

func main() {
	fmt.Println("Claude Agent SDK - Debugging and Diagnostics Example")
	fmt.Println("=====================================================")
	fmt.Println()

	// Example 1: Environment Variables
	fmt.Println("--- Example 1: Environment Variables ---")
	fmt.Println("Setting custom environment variables for subprocess...")
	demonstrateEnvironmentVariables()

	// Example 2: Debug Output Configuration
	fmt.Println()
	fmt.Println("--- Example 2: Debug Output Configuration ---")
	fmt.Println("Configuring debug output destinations...")
	demonstrateDebugOutput()

	// Example 3: Stderr Callback
	fmt.Println()
	fmt.Println("--- Example 3: Stderr Callback ---")
	fmt.Println("Setting up stderr monitoring callback...")
	demonstrateStderrCallback()

	// Example 4: Server Diagnostics
	fmt.Println()
	fmt.Println("--- Example 4: Server Diagnostics ---")
	fmt.Println("Using diagnostics methods for health monitoring...")
	demonstrateServerDiagnostics()

	fmt.Println()
	fmt.Println("Debugging and diagnostics example completed!")
}

// demonstrateEnvironmentVariables shows WithEnv and WithEnvVar
func demonstrateEnvironmentVariables() {
	// Set multiple environment variables at once
	envMap := map[string]string{
		"MY_API_KEY":     "secret-key-12345",
		"DEBUG":          "true",
		"LOG_LEVEL":      "debug",
		"CUSTOM_SETTING": "enabled",
	}

	fmt.Println("Using WithEnv for multiple variables:")
	for k, v := range envMap {
		// Mask sensitive values in output
		displayValue := v
		if k == "MY_API_KEY" {
			displayValue = "***masked***"
		}
		fmt.Printf("  %s=%s\n", k, displayValue)
	}

	client := claudecode.NewClient(
		claudecode.WithEnv(envMap),
	)
	fmt.Println("Client created with environment variables")
	fmt.Println()

	// Set individual environment variable
	fmt.Println("Using WithEnvVar for single variable:")
	fmt.Println("  SINGLE_VAR=single_value")

	client2 := claudecode.NewClient(
		claudecode.WithEnvVar("SINGLE_VAR", "single_value"),
	)
	fmt.Println("Client created with single environment variable")

	_ = client
	_ = client2
}

// demonstrateDebugOutput shows debug output configuration options
func demonstrateDebugOutput() {
	// Option 1: Debug to custom writer (buffer for capture)
	fmt.Println("Option 1: WithDebugWriter - capture to buffer")
	var debugBuffer bytes.Buffer
	client1 := claudecode.NewClient(
		claudecode.WithDebugWriter(&debugBuffer),
	)
	fmt.Printf("  Debug output will be written to buffer\n")
	fmt.Printf("  Buffer type: %T\n", &debugBuffer)
	_ = client1
	fmt.Println()

	// Option 2: Debug to stderr
	fmt.Println("Option 2: WithDebugStderr - output to stderr")
	client2 := claudecode.NewClient(
		claudecode.WithDebugStderr(),
	)
	fmt.Println("  Debug output will appear on stderr")
	_ = client2
	fmt.Println()

	// Option 3: Debug disabled
	fmt.Println("Option 3: WithDebugDisabled - no debug output")
	client3 := claudecode.NewClient(
		claudecode.WithDebugDisabled(),
	)
	fmt.Println("  Debug output is completely suppressed")
	_ = client3
	fmt.Println()

	// Option 4: Debug to file
	fmt.Println("Option 4: WithDebugWriter - output to file")
	fmt.Println("  Example: claudecode.WithDebugWriter(logFile)")
	fmt.Println("  Useful for production logging and post-mortem analysis")
}

// demonstrateStderrCallback shows real-time stderr monitoring
func demonstrateStderrCallback() {
	// Thread-safe stderr log
	var stderrLines []string
	var mu sync.Mutex

	// Create callback that captures stderr lines
	stderrCallback := func(line string) {
		mu.Lock()
		stderrLines = append(stderrLines, line)
		mu.Unlock()
		fmt.Printf("  [STDERR] %s\n", truncate(line, 60))
	}

	client := claudecode.NewClient(
		claudecode.WithStderrCallback(stderrCallback),
	)

	fmt.Println("Stderr callback configured")
	fmt.Println("Every line from CLI stderr will invoke the callback")
	fmt.Println()
	fmt.Println("Use cases:")
	fmt.Println("  - Real-time monitoring of CLI messages")
	fmt.Println("  - Capturing warnings and errors")
	fmt.Println("  - Building diagnostic dashboards")
	fmt.Println("  - Forwarding to logging services")

	_ = client
}

// demonstrateServerDiagnostics shows GetServerInfo and GetStreamStats
func demonstrateServerDiagnostics() {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	fmt.Println("Connecting to demonstrate diagnostics methods...")

	err := claudecode.WithClient(ctx, func(client claudecode.Client) error {
		// GetServerInfo - connection status
		fmt.Println()
		fmt.Println("GetServerInfo() - Connection status:")
		info, err := client.GetServerInfo(ctx)
		if err != nil {
			fmt.Printf("  Error: %v\n", err)
		} else {
			for k, v := range info {
				fmt.Printf("  %s: %v\n", k, v)
			}
		}

		// Send a query to generate some stats
		if err := client.Query(ctx, "What is 2+2? Answer in one word."); err != nil {
			return err
		}

		// Drain the response
		msgChan := client.ReceiveMessages(ctx)
		for {
			select {
			case message := <-msgChan:
				if message == nil {
					goto statsSection
				}
				switch msg := message.(type) {
				case *claudecode.AssistantMessage:
					// HasError checks the top-level error field (PR #506 fix).
					// This is distinct from ResultMessage.IsError - it signals
					// a rate limit, billing issue, or server error on the message itself.
					if msg.HasError() {
						errType := msg.GetError()
						if msg.IsRateLimited() {
							fmt.Printf("\n[rate limited] %s\n", errType)
						} else {
							fmt.Printf("\n[assistant error] %s\n", errType)
						}
					}
					for _, block := range msg.Content {
						if textBlock, ok := block.(*claudecode.TextBlock); ok {
							fmt.Printf("\nClaude: %s\n", textBlock.Text)
						}
					}
				case *claudecode.ResultMessage:
					if msg.IsError {
						if msg.Result != nil {
							return fmt.Errorf("error: %s", *msg.Result)
						}
						return fmt.Errorf("error: unknown error")
					}
					goto statsSection
				}
			case <-ctx.Done():
				return ctx.Err()
			}
		}

	statsSection:
		// GetStreamStats - streaming statistics
		fmt.Println()
		fmt.Println("GetStreamStats() - Streaming statistics:")
		stats := client.GetStreamStats()
		fmt.Printf("  Stats: %+v\n", stats)

		// GetStreamIssues - validation issues
		fmt.Println()
		fmt.Println("GetStreamIssues() - Validation issues:")
		issues := client.GetStreamIssues()
		if len(issues) == 0 {
			fmt.Println("  No issues detected (healthy stream)")
		} else {
			for i, issue := range issues {
				fmt.Printf("  %d. %+v\n", i+1, issue)
			}
		}

		return nil
	},
		claudecode.WithMaxTurns(1),
	)

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
		// Don't fail the example if CLI is not available
		fmt.Printf("Note: %v\n", err)
		fmt.Println("(Diagnostics methods require an active connection)")
	}
}

// truncate shortens a string to maxLen characters
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// Demonstrate debug to file pattern (not executed, just shown)
func init() {
	// This pattern is useful for production environments
	_ = func() {
		logFile, err := os.OpenFile("claude-debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return
		}
		defer logFile.Close()

		_ = claudecode.NewClient(
			claudecode.WithDebugWriter(logFile),
			claudecode.WithStderrCallback(func(line string) {
				// Also write stderr to the log file
				fmt.Fprintln(logFile, "[STDERR]", line)
			}),
		)
	}
}
