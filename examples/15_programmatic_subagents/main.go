// Package main demonstrates Programmatic Subagents configuration.
//
// This example shows how to define specialized agents programmatically
// using the SDK. Programmatic subagents enable:
// - Custom agent definitions without external configuration files
// - Specialized agents with specific tools and prompts
// - Model selection per agent (Sonnet, Haiku, Opus, or Inherit)
// - Dynamic agent configuration at runtime
//
// Key components:
// - WithAgent: Add a single agent definition
// - WithAgents: Add multiple agent definitions at once
// - AgentDefinition: Struct containing agent configuration
// - AgentModel: Constants for model selection (AgentModelSonnet, etc.)
//
// Run: go run main.go
package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	claudecode "github.com/severity1/claude-agent-sdk-go"
)

func main() {
	fmt.Println("Claude Agent SDK - Programmatic Subagents Example")
	fmt.Println("==================================================")
	fmt.Println()

	// Example 1: Single Agent Definition
	fmt.Println("--- Example 1: Single Agent Definition ---")
	fmt.Println("Defining a code reviewer agent with specific tools and model...")
	runSingleAgentExample()

	// Example 2: Multiple Agents
	fmt.Println()
	fmt.Println("--- Example 2: Multiple Agents ---")
	fmt.Println("Defining multiple specialized agents for a development workflow...")
	runMultipleAgentsExample()

	// Example 3: Agent Model Options
	fmt.Println()
	fmt.Println("--- Example 3: Agent Model Options ---")
	fmt.Println("Demonstrating available agent model constants...")
	showAgentModelOptions()

	// Example 4: Large Agent Payload (initialize-streaming smoke test)
	fmt.Println()
	fmt.Println("--- Example 4: Large Agent Payload ---")
	fmt.Println("Registering a large agent payload that would previously hit ARG_MAX...")
	runLargePayloadAgentsExample()

	fmt.Println()
	fmt.Println("Programmatic subagents example completed!")
}

// runSingleAgentExample demonstrates adding a single agent with WithAgent
func runSingleAgentExample() {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Define a code reviewer agent
	codeReviewerAgent := claudecode.AgentDefinition{
		Description: "Reviews code for best practices, security issues, and style",
		Prompt:      "You are an expert code reviewer. Analyze code for bugs, security vulnerabilities, and adherence to best practices. Provide constructive feedback.",
		Tools:       []string{"Read", "Grep", "Glob"},
		Model:       claudecode.AgentModelSonnet,
	}

	fmt.Printf("Agent: code-reviewer\n")
	fmt.Printf("  Description: %s\n", codeReviewerAgent.Description)
	fmt.Printf("  Tools: %v\n", codeReviewerAgent.Tools)
	fmt.Printf("  Model: %s\n", codeReviewerAgent.Model)
	fmt.Println()

	// Use WithClient with the agent configuration
	err := claudecode.WithClient(ctx, func(client claudecode.Client) error {
		// Ask Claude to use the defined agent
		if err := client.Query(ctx, "Using the code-reviewer agent, briefly describe what a code review should check for. Keep your response under 50 words."); err != nil {
			return err
		}
		return streamResponse(ctx, client)
	},
		claudecode.WithAgent("code-reviewer", codeReviewerAgent),
		claudecode.WithMaxTurns(2),
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
		fmt.Printf("Error: %v\n", err)
	}
}

// runMultipleAgentsExample demonstrates adding multiple agents with WithAgents
func runMultipleAgentsExample() {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Define multiple specialized agents for a development workflow
	agents := map[string]claudecode.AgentDefinition{
		"test-writer": {
			Description: "Writes comprehensive unit tests for code",
			Prompt:      "You are a test engineer. Write thorough unit tests with edge cases and clear assertions.",
			Tools:       []string{"Read", "Write", "Bash"},
			Model:       claudecode.AgentModelHaiku, // Fast model for test generation
		},
		"documentation": {
			Description: "Creates and updates code documentation",
			Prompt:      "You are a technical writer. Create clear, concise documentation with examples.",
			Tools:       []string{"Read", "Write", "Edit"},
			Model:       claudecode.AgentModelSonnet,
		},
		"refactorer": {
			Description: "Refactors code for better maintainability",
			Prompt:      "You are a refactoring expert. Improve code structure while maintaining functionality.",
			Tools:       []string{"Read", "Write", "Edit", "Grep"},
			Model:       claudecode.AgentModelInherit, // Use parent model
		},
	}

	fmt.Printf("Defined %d agents:\n", len(agents))
	for name, agent := range agents {
		fmt.Printf("  - %s (%s): %s\n", name, agent.Model, agent.Description)
	}
	fmt.Println()

	// Use WithClient with multiple agents
	err := claudecode.WithClient(ctx, func(client claudecode.Client) error {
		// Ask Claude to list the available agents
		if err := client.Query(ctx, "List the specialized agents available to you and their purposes. Keep response under 75 words."); err != nil {
			return err
		}
		return streamResponse(ctx, client)
	},
		claudecode.WithAgents(agents),
		claudecode.WithMaxTurns(2),
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
		fmt.Printf("Error: %v\n", err)
	}
}

// runLargePayloadAgentsExample registers ~260 KB of agent definitions over the
// initialize control request. A payload this size would exceed ARG_MAX if
// passed via argv.
func runLargePayloadAgentsExample() {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	const (
		agentCount      = 20
		promptSizeBytes = 13 * 1024
		promptUnit      = "You are a specialist agent. Follow project conventions. "
	)

	repeatCount := promptSizeBytes / len(promptUnit)
	basePrompt := strings.Repeat(promptUnit, repeatCount)

	agents := make(map[string]claudecode.AgentDefinition, agentCount)
	for i := 0; i < agentCount; i++ {
		name := fmt.Sprintf("specialist-%02d", i)
		agents[name] = claudecode.AgentDefinition{
			Description: fmt.Sprintf("Specialist agent #%d for large-payload registration test.", i),
			Prompt:      basePrompt,
			Tools:       []string{"Read"},
			Model:       claudecode.AgentModelInherit,
		}
	}

	totalBytes := 0
	for _, agent := range agents {
		totalBytes += len(agent.Description) + len(agent.Prompt)
	}
	fmt.Printf("Registering %d agents, total payload ~%d KB.\n", len(agents), totalBytes/1024)
	fmt.Println("Note: this would previously fail with E2BIG via --agents; now streams via initialize.")

	err := claudecode.WithClient(ctx, func(_ claudecode.Client) error {
		fmt.Println("Connected; agents registered via the initialize control request (no ARG_MAX error).")
		return nil
	},
		claudecode.WithAgents(agents),
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
		fmt.Printf("Error: %v\n", err)
	}
}

// showAgentModelOptions displays all available agent model constants
func showAgentModelOptions() {
	fmt.Println("Available AgentModel constants:")
	fmt.Printf("  - AgentModelSonnet:  %q - Claude Sonnet (balanced)\n", claudecode.AgentModelSonnet)
	fmt.Printf("  - AgentModelHaiku:   %q - Claude Haiku (fast)\n", claudecode.AgentModelHaiku)
	fmt.Printf("  - AgentModelOpus:    %q - Claude Opus (powerful)\n", claudecode.AgentModelOpus)
	fmt.Printf("  - AgentModelInherit: %q - Inherit parent model\n", claudecode.AgentModelInherit)
}

// streamResponse reads and displays messages from the client
func streamResponse(ctx context.Context, client claudecode.Client) error {
	msgChan := client.ReceiveMessages(ctx)

	for {
		select {
		case message := <-msgChan:
			if message == nil {
				fmt.Println()
				return nil
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
