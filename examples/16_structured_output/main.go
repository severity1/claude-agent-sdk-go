// Package main demonstrates Structured Output with JSON Schema.
//
// This example shows how to constrain Claude's responses to match a
// specific JSON schema, enabling type-safe data extraction. Structured
// output enables:
// - Extracting structured data from natural language
// - Type-safe responses that match your Go types
// - Reliable parsing without string manipulation
// - Integration with strongly-typed applications
//
// Key components:
// - WithJSONSchema: Convenience function for JSON schema constraint
// - WithOutputFormat: Explicit output format control
// - OutputFormatJSONSchema: Creates OutputFormat from schema
// - ResultMessage.StructuredOutput: Access the parsed output
//
// Run: go run main.go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	claudecode "github.com/severity1/claude-agent-sdk-go"
)

func main() {
	fmt.Println("Claude Agent SDK - Structured Output Example")
	fmt.Println("=============================================")
	fmt.Println()

	// Example 1: Extract Tasks from Natural Language
	fmt.Println("--- Example 1: Task Extraction ---")
	fmt.Println("Extracting structured task data from natural language...")
	runTaskExtractionExample()

	// Example 2: Extract Contact Information
	fmt.Println()
	fmt.Println("--- Example 2: Contact Information Extraction ---")
	fmt.Println("Extracting structured contact data...")
	runContactExtractionExample()

	// Example 3: Using OutputFormatJSONSchema directly
	fmt.Println()
	fmt.Println("--- Example 3: Explicit OutputFormat ---")
	fmt.Println("Using WithOutputFormat for explicit control...")
	runExplicitOutputFormatExample()

	fmt.Println()
	fmt.Println("Structured output example completed!")
}

// TaskList represents a list of tasks extracted from text
type TaskList struct {
	Tasks    []string `json:"tasks"`
	Priority string   `json:"priority"`
}

// runTaskExtractionExample demonstrates extracting tasks from natural language
func runTaskExtractionExample() {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Define JSON schema for task extraction
	taskSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"tasks": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "List of tasks extracted from the text",
			},
			"priority": map[string]any{
				"type":        "string",
				"enum":        []string{"low", "medium", "high"},
				"description": "Overall priority level of the tasks",
			},
		},
		"required": []string{"tasks", "priority"},
	}

	inputText := "I need to fix the login bug urgently, then add unit tests, and finally update the documentation."
	fmt.Printf("Input: %q\n", inputText)
	fmt.Println()

	var structuredOutput any

	err := claudecode.WithClient(ctx, func(client claudecode.Client) error {
		prompt := fmt.Sprintf("Extract the tasks and determine priority from: %q", inputText)
		if err := client.Query(ctx, prompt); err != nil {
			return err
		}

		// Stream and capture structured output
		msgChan := client.ReceiveMessages(ctx)
		for {
			select {
			case message := <-msgChan:
				if message == nil {
					return nil
				}
				switch msg := message.(type) {
				case *claudecode.ResultMessage:
					if msg.IsError {
						if msg.Result != nil {
							return fmt.Errorf("error: %s", *msg.Result)
						}
						if len(msg.Errors) > 0 {
							return fmt.Errorf("error (%s): %s", msg.Subtype, strings.Join(msg.Errors, "; "))
						}
						return fmt.Errorf("error: unknown error (subtype=%s)", msg.Subtype)
					}
					// Capture the structured output
					structuredOutput = msg.StructuredOutput
					return nil
				}
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	},
		claudecode.WithJSONSchema(taskSchema),
		claudecode.WithMaxTurns(3),
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
		return
	}

	// Display the structured output
	if structuredOutput != nil {
		fmt.Println("Structured Output:")
		jsonBytes, _ := json.MarshalIndent(structuredOutput, "  ", "  ")
		fmt.Printf("  %s\n", string(jsonBytes))

		// Type assertion to access fields
		if output, ok := structuredOutput.(map[string]any); ok {
			if tasks, ok := output["tasks"].([]any); ok {
				fmt.Printf("\nExtracted %d tasks:\n", len(tasks))
				for i, task := range tasks {
					fmt.Printf("  %d. %v\n", i+1, task)
				}
			}
			if priority, ok := output["priority"].(string); ok {
				fmt.Printf("Priority: %s\n", priority)
			}
		}
	} else {
		fmt.Println("No structured output received")
	}
}

// Contact represents contact information
type Contact struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Phone string `json:"phone,omitempty"`
}

// runContactExtractionExample demonstrates extracting contact information
func runContactExtractionExample() {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Define JSON schema for contact extraction
	contactSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{
				"type":        "string",
				"description": "Full name of the person",
			},
			"email": map[string]any{
				"type":        "string",
				"description": "Email address",
			},
			"phone": map[string]any{
				"type":        "string",
				"description": "Phone number if available",
			},
		},
		"required": []string{"name", "email"},
	}

	inputText := "Please contact John Smith at john.smith@example.com or call 555-1234."
	fmt.Printf("Input: %q\n", inputText)
	fmt.Println()

	var structuredOutput any

	err := claudecode.WithClient(ctx, func(client claudecode.Client) error {
		prompt := fmt.Sprintf("Extract contact information from: %q", inputText)
		if err := client.Query(ctx, prompt); err != nil {
			return err
		}

		msgChan := client.ReceiveMessages(ctx)
		for {
			select {
			case message := <-msgChan:
				if message == nil {
					return nil
				}
				switch msg := message.(type) {
				case *claudecode.ResultMessage:
					if msg.IsError {
						if msg.Result != nil {
							return fmt.Errorf("error: %s", *msg.Result)
						}
						if len(msg.Errors) > 0 {
							return fmt.Errorf("error (%s): %s", msg.Subtype, strings.Join(msg.Errors, "; "))
						}
						return fmt.Errorf("error: unknown error (subtype=%s)", msg.Subtype)
					}
					structuredOutput = msg.StructuredOutput
					return nil
				}
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	},
		claudecode.WithJSONSchema(contactSchema),
		claudecode.WithMaxTurns(3),
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
		return
	}

	if structuredOutput != nil {
		fmt.Println("Extracted Contact:")
		jsonBytes, _ := json.MarshalIndent(structuredOutput, "  ", "  ")
		fmt.Printf("  %s\n", string(jsonBytes))
	} else {
		fmt.Println("No structured output received")
	}
}

// runExplicitOutputFormatExample demonstrates using WithOutputFormat directly
func runExplicitOutputFormatExample() {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Create schema for a simple response
	summarySchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"summary": map[string]any{
				"type":        "string",
				"description": "Brief summary",
			},
			"word_count": map[string]any{
				"type":        "integer",
				"description": "Approximate word count",
			},
		},
		"required": []string{"summary", "word_count"},
	}

	// Create OutputFormat explicitly
	outputFormat := claudecode.OutputFormatJSONSchema(summarySchema)

	fmt.Printf("OutputFormat Type: %s\n", outputFormat.Type)
	fmt.Println()

	var structuredOutput any

	err := claudecode.WithClient(ctx, func(client claudecode.Client) error {
		if err := client.Query(ctx, "Summarize: The quick brown fox jumps over the lazy dog. This is a classic pangram."); err != nil {
			return err
		}

		msgChan := client.ReceiveMessages(ctx)
		for {
			select {
			case message := <-msgChan:
				if message == nil {
					return nil
				}
				switch msg := message.(type) {
				case *claudecode.ResultMessage:
					if msg.IsError {
						if msg.Result != nil {
							return fmt.Errorf("error: %s", *msg.Result)
						}
						if len(msg.Errors) > 0 {
							return fmt.Errorf("error (%s): %s", msg.Subtype, strings.Join(msg.Errors, "; "))
						}
						return fmt.Errorf("error: unknown error (subtype=%s)", msg.Subtype)
					}
					structuredOutput = msg.StructuredOutput
					return nil
				}
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	},
		claudecode.WithOutputFormat(outputFormat),
		claudecode.WithMaxTurns(3),
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
		return
	}

	if structuredOutput != nil {
		fmt.Println("Structured Summary:")
		jsonBytes, _ := json.MarshalIndent(structuredOutput, "  ", "  ")
		fmt.Printf("  %s\n", string(jsonBytes))
	} else {
		fmt.Println("No structured output received")
	}
}
