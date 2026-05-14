// Package main demonstrates the ExtraArgs helper functions for configuring Claude Code queries.
package main

import (
	"context"
	"fmt"
	"log"

	claudecode "github.com/severity1/claude-agent-sdk-go"
)

func main() {
	fmt.Println("=== ExtraArgs Helper Functions Example ===")

	// Example 1: Using WithExtraFlag for boolean flags
	fmt.Println("1. Boolean flag example (fork-session):")
	optionsWithFlag := claudecode.NewOptions(
		claudecode.WithSystemPrompt("You are a helpful assistant"),
		claudecode.WithExtraFlag("fork-session"), // Easy boolean flag
	)
	fmt.Printf("   ExtraArgs: %v\n", formatExtraArgs(optionsWithFlag.ExtraArgs))
	fmt.Println()

	// Example 2: Using WithExtraArg for valued flags
	fmt.Println("2. Valued flag example (custom setting):")
	optionsWithArg := claudecode.NewOptions(
		claudecode.WithExtraArg("output-format", "json"),
		claudecode.WithExtraArg("log-level", "debug"),
	)
	fmt.Printf("   ExtraArgs: %v\n", formatExtraArgs(optionsWithArg.ExtraArgs))
	fmt.Println()

	// Example 3: Mixing boolean flags and valued flags
	fmt.Println("3. Mixed boolean and valued flags:")
	optionsMixed := claudecode.NewOptions(
		claudecode.WithExtraFlag("verbose"),         // Boolean flag
		claudecode.WithExtraArg("format", "xml"),    // Valued flag
		claudecode.WithExtraFlag("debug"),           // Another boolean flag
		claudecode.WithExtraArg("timeout", "30000"), // Another valued flag
	)
	fmt.Printf("   ExtraArgs: %v\n", formatExtraArgs(optionsMixed.ExtraArgs))
	fmt.Println()

	// Example 4: Combining with other options
	fmt.Println("4. Complete configuration example:")
	completeOptions := claudecode.NewOptions(
		claudecode.WithSystemPrompt("You are a code review assistant"),
		claudecode.WithModel("claude-sonnet-3-5-20241022"),
		claudecode.WithMaxThinkingTokens(10000),
		claudecode.WithExtraFlag("fork-session"), // Helper for boolean flag
		claudecode.WithExtraArg("review-depth", "thorough"),
		claudecode.WithAllowedTools("Read", "Write", "Edit"),
	)
	fmt.Printf("   System Prompt: %v\n", *completeOptions.SystemPrompt)
	fmt.Printf("   Model: %v\n", *completeOptions.Model)
	fmt.Printf("   Max Thinking Tokens: %d\n", completeOptions.MaxThinkingTokens)
	fmt.Printf("   ExtraArgs: %v\n", formatExtraArgs(completeOptions.ExtraArgs))
	fmt.Printf("   Allowed Tools: %v\n", completeOptions.AllowedTools)
	fmt.Println()

	// Example 5: Old way vs. new way comparison
	fmt.Println("5. Comparison of old vs. new syntax:")
	fmt.Println("   OLD (manual map creation):")
	forkSession := "fork-session"
	oldWay := claudecode.NewOptions(
		claudecode.WithExtraArgs(map[string]*string{
			forkSession: nil, // Had to create variable and use pointer
		}),
	)
	fmt.Printf("   %v\n", formatExtraArgs(oldWay.ExtraArgs))

	fmt.Println("   NEW (helper function):")
	newWay := claudecode.NewOptions(
		claudecode.WithExtraFlag("fork-session"), // Clean and simple!
	)
	fmt.Printf("   %v\n", formatExtraArgs(newWay.ExtraArgs))
	fmt.Println()

	// Example 6: Real-world usage with Query
	fmt.Println("6. Real-world usage with Query:")
	ctx := context.Background()
	iterator, err := claudecode.Query(
		ctx,
		"What is the capital of France?",
		claudecode.WithSystemPrompt("You are a geography expert"),
		claudecode.WithExtraFlag("verbose"), // Enable verbose output
		claudecode.WithExtraArg("response-format", "concise"),
	)

	if err != nil {
		log.Printf("   Error: %v\n", err)
	} else {
		defer func() { _ = iterator.Close() }()
		// Iterate through messages and find the final assistant message
		for {
			msg, err := iterator.Next(ctx)
			if err != nil {
				break
			}
			if assistantMsg, ok := msg.(*claudecode.AssistantMessage); ok {
				if len(assistantMsg.Content) > 0 {
					if textBlock, ok := assistantMsg.Content[0].(*claudecode.TextBlock); ok {
						fmt.Printf("   Response: %s\n", textBlock.Text)
						break
					}
				}
			}
		}
	}
}

// formatExtraArgs formats the ExtraArgs map for display
func formatExtraArgs(args map[string]*string) string {
	if len(args) == 0 {
		return "{}"
	}

	result := "{"
	first := true
	for key, val := range args {
		if !first {
			result += ", "
		}
		first = false

		if val == nil {
			result += fmt.Sprintf("%q: nil", key)
		} else {
			result += fmt.Sprintf("%q: %q", key, *val)
		}
	}
	result += "}"
	return result
}
