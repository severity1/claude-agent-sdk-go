// Package main demonstrates the fork session functionality for branching conversations.
package main

import (
	"context"
	"fmt"
	"log"

	claudecode "github.com/severity1/claude-agent-sdk-go"
)

func main() {
	fmt.Println("=== Fork Session Example ===")

	// Example 1: Basic conversation branching
	fmt.Println("1. Basic Conversation Branching:")
	fmt.Println("   Demonstrates how to fork from an existing conversation to explore alternatives")
	fmt.Println()

	// Simulate an original session ID (in real usage, this would come from a previous conversation)
	originalSessionID := "uuid-original-session-abc123"

	// Fork from the original session to explore an alternative approach
	forkOptions := claudecode.NewOptions(
		claudecode.WithResume(originalSessionID),
		claudecode.WithForkSession(true), // Creates new session with old context
		claudecode.WithSystemPrompt("Let's explore an alternative approach"),
	)

	fmt.Printf("   Original Session: %s\n", originalSessionID)
	fmt.Printf("   Fork Options configured:\n")
	fmt.Printf("     - Resume from: %s\n", *forkOptions.Resume)
	fmt.Printf("     - Fork session enabled: %v\n", forkOptions.ExtraArgs["fork-session"] == nil)
	fmt.Println()

	// Example 2: Parallel experimentation
	fmt.Println("2. Parallel Experimentation:")
	fmt.Println("   Create multiple forks from the same base to try different approaches")
	fmt.Println()

	baseSessionID := "uuid-base-session-xyz789"

	// Fork 1: Optimization-focused approach
	fork1 := claudecode.NewOptions(
		claudecode.WithResume(baseSessionID),
		claudecode.WithForkSession(true),
		claudecode.WithExtraArg("experiment-type", "optimization"),
		claudecode.WithSystemPrompt("Focus on performance optimization"),
	)

	// Fork 2: Readability-focused approach
	fork2 := claudecode.NewOptions(
		claudecode.WithResume(baseSessionID),
		claudecode.WithForkSession(true),
		claudecode.WithExtraArg("experiment-type", "readability"),
		claudecode.WithSystemPrompt("Focus on code readability"),
	)

	fmt.Printf("   Base Session: %s\n", baseSessionID)
	fmt.Printf("   Fork 1 (Optimization):\n")
	fmt.Printf("     - Experiment type: %s\n", *fork1.ExtraArgs["experiment-type"])
	fmt.Printf("   Fork 2 (Readability):\n")
	fmt.Printf("     - Experiment type: %s\n", *fork2.ExtraArgs["experiment-type"])
	fmt.Println()

	// Example 3: Recovery scenario
	fmt.Println("3. Recovery from Known Good State:")
	fmt.Println("   Fork from a previous successful state to retry after an error")
	fmt.Println()

	knownGoodSessionID := "uuid-known-good-state"
	recoveryOptions := claudecode.NewOptions(
		claudecode.WithResume(knownGoodSessionID),
		claudecode.WithForkSession(true),
		claudecode.WithSystemPrompt("Retry with corrected input"),
		claudecode.WithExtraArg("retry-attempt", "2"),
	)

	fmt.Printf("   Known Good Session: %s\n", knownGoodSessionID)
	fmt.Printf("   Recovery attempt: %s\n", *recoveryOptions.ExtraArgs["retry-attempt"])
	fmt.Println()

	// Example 4: Real-world usage pattern with WithClient
	fmt.Println("4. Real-World Usage Pattern:")
	fmt.Println("   Demonstrates practical fork-session usage with the Client API")
	fmt.Println()

	// In a real scenario, you would get this from a database or previous conversation
	existingSessionID := "previous-conversation-uuid"

	ctx := context.Background()

	// Note: This is a demonstration of the API pattern.
	// In actual use, you would have a real session to fork from.
	fmt.Printf("   Pattern for forking a conversation:\n")
	fmt.Printf("   err := claudecode.WithClient(ctx, func(client claudecode.Client) error {\n")
	fmt.Printf("       return client.Query(ctx, \"Let's try a different approach\")\n")
	fmt.Printf("   }, claudecode.WithResume(%q),\n", existingSessionID)
	fmt.Printf("      claudecode.WithForkSession(true))\n")
	fmt.Println()

	// Example 5: Complete workflow comparison
	fmt.Println("5. Without Fork vs With Fork:")
	fmt.Println()

	fmt.Println("   WITHOUT fork-session (modifies original):")
	fmt.Println("   - Resume appends to existing conversation")
	fmt.Println("   - Changes affect the original session")
	fmt.Println("   - No way to preserve original path")
	fmt.Println()

	fmt.Println("   WITH fork-session (creates branch):")
	fmt.Println("   - Creates new session ID with old context")
	fmt.Println("   - Original conversation remains unchanged")
	fmt.Println("   - Can explore multiple paths from same point")
	fmt.Println()

	// Example 6: Demonstrating the options builder pattern
	fmt.Println("6. Builder Pattern Flexibility:")
	fmt.Println("   Combine WithForkSession with other options seamlessly")
	fmt.Println()

	complexOptions := claudecode.NewOptions(
		claudecode.WithResume("session-to-fork"),
		claudecode.WithForkSession(true),
		claudecode.WithModel("claude-3-5-sonnet-20241022"),
		claudecode.WithMaxTurns(10),
		claudecode.WithAllowedTools("Read", "Write", "Edit"),
		claudecode.WithExtraFlag("verbose"),
		claudecode.WithExtraArg("custom-setting", "value"),
	)

	fmt.Printf("   Complex configuration created:\n")
	fmt.Printf("     - Fork from: %s\n", *complexOptions.Resume)
	fmt.Printf("     - Model: %s\n", *complexOptions.Model)
	fmt.Printf("     - Max turns: %d\n", complexOptions.MaxTurns)
	fmt.Printf("     - Allowed tools: %v\n", complexOptions.AllowedTools)
	fmt.Printf("     - Fork session: enabled\n")
	fmt.Printf("     - Verbose: enabled\n")
	fmt.Printf("     - Custom setting: %s\n", *complexOptions.ExtraArgs["custom-setting"])
	fmt.Println()

	// Example 7: Error handling pattern
	fmt.Println("7. Error Handling Pattern:")
	fmt.Println("   How to handle fork-session in production code")
	fmt.Println()

	// Demonstrate the pattern (not executing actual API call)
	demonstrateErrorHandling(ctx, "prod-session-uuid")
}

// demonstrateErrorHandling shows the recommended pattern for using fork-session in production
func demonstrateErrorHandling(_ context.Context, _ string) {
	fmt.Println("   func processWithFork(ctx context.Context, sessionID string) error {")
	fmt.Println("       // Create fork options")
	fmt.Println("       opts := claudecode.NewOptions(")
	fmt.Println("           claudecode.WithResume(sessionID),")
	fmt.Println("           claudecode.WithForkSession(true),")
	fmt.Println("       )")
	fmt.Println()
	fmt.Println("       // Use with error handling")
	fmt.Println("       err := claudecode.WithClient(ctx, func(client claudecode.Client) error {")
	fmt.Println("           // Your conversation logic here")
	fmt.Println("           return client.Query(ctx, \"Your prompt\")")
	fmt.Println("       }, opts)")
	fmt.Println()
	fmt.Println("       if err != nil {")
	fmt.Println("           // Handle error - original session is unchanged")
	fmt.Println("           return fmt.Errorf(\"fork failed: %w\", err)")
	fmt.Println("       }")
	fmt.Println("       return nil")
	fmt.Println("   }")
	fmt.Println()

	// Note: We don't execute the actual call in this example since we don't have
	// a real Claude Code installation configured. This is for demonstration only.
	log.Println("   Note: This example demonstrates the API without executing actual calls")
}
