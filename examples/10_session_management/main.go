// Package main demonstrates session management with the Claude Agent SDK for Go.
// This example shows session creation, isolation, and resumption using the clean
// Query API with explicit methods for better clarity and Go idiom compliance.
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/severity1/claude-agent-sdk-go"
)

func main() {
	fmt.Println("Claude Agent SDK - Session Management Example")
	fmt.Println("==============================================")

	ctx := context.Background()

	if err := runExample(ctx); err != nil {
		log.Fatalf("Example failed: %v", err)
	}
}

func runExample(ctx context.Context) error {
	// Part 1: Default and Custom Sessions within a single client connection
	var capturedSessionID string

	err := claudecode.WithClient(ctx, func(client claudecode.Client) error {
		// 1. Default session (recommended for most use cases)
		fmt.Println("\n1. Default session with Query()")
		fmt.Println("   Asking: What's 2+2?")
		if err := client.Query(ctx, "Hello! What's 2+2? Reply briefly."); err != nil {
			return fmt.Errorf("default session query: %w", err)
		}
		result, err := streamResponse(ctx, client)
		if err != nil {
			return err
		}
		if result != nil {
			fmt.Printf("   [Session ID: %s]\n", result.SessionID)
		}

		// 2. Custom session (for isolated conversations)
		fmt.Println("\n2. Custom session with QueryWithSession()")
		fmt.Println("   Asking: What's 3+3?")
		if err := client.QueryWithSession(ctx, "Hello! What's 3+3? Reply briefly.", "math-session"); err != nil {
			return fmt.Errorf("custom session query: %w", err)
		}
		mathResult, err := streamResponse(ctx, client)
		if err != nil {
			return err
		}
		if mathResult != nil {
			capturedSessionID = mathResult.SessionID
			fmt.Printf("   [Session ID: %s]\n", capturedSessionID)
		}

		// 3. Session isolation demonstration
		fmt.Println("\n3. Session isolation demonstration")

		fmt.Println("   Default session asking about previous question:")
		if err := client.Query(ctx, "What was my previous math question? Reply briefly."); err != nil {
			return fmt.Errorf("isolation test default: %w", err)
		}
		if _, err := streamResponse(ctx, client); err != nil {
			return err
		}

		fmt.Println("\n   Math session remembers its own context:")
		if err := client.QueryWithSession(ctx, "What was my previous math question? Reply briefly.", "math-session"); err != nil {
			return fmt.Errorf("isolation test math: %w", err)
		}
		if _, err := streamResponse(ctx, client); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return err
	}

	// Part 2: Session Resumption with WithResume()
	// This demonstrates resuming a session from a previous client connection
	if capturedSessionID != "" {
		fmt.Println("\n4. Session Resumption with WithResume()")
		fmt.Printf("   Resuming session: %s\n", capturedSessionID)

		err = claudecode.WithClient(ctx, func(client claudecode.Client) error {
			fmt.Println("   Asking resumed session about previous context:")
			if err := client.Query(ctx, "What math problem did we discuss earlier? Reply briefly."); err != nil {
				return fmt.Errorf("resumed session query: %w", err)
			}
			if _, err := streamResponse(ctx, client); err != nil {
				return err
			}
			return nil
		}, claudecode.WithResume(capturedSessionID))
		if err != nil {
			return err
		}
	}

	fmt.Println("\nSession management demonstration completed!")
	return nil
}

// streamResponse streams a complete response from the client and returns the ResultMessage.
// This follows established SDK patterns for proper streaming output without messy duplicates.
func streamResponse(ctx context.Context, client claudecode.Client) (*claudecode.ResultMessage, error) {
	msgChan := client.ReceiveMessages(ctx)
	for {
		select {
		case message := <-msgChan:
			if message == nil {
				return nil, nil
			}

			switch msg := message.(type) {
			case *claudecode.AssistantMessage:
				for _, block := range msg.Content {
					if textBlock, ok := block.(*claudecode.TextBlock); ok {
						fmt.Print(textBlock.Text)
					}
				}
			case *claudecode.ResultMessage:
				fmt.Println() // Add newline after complete response
				if msg.IsError {
					if msg.Result != nil {
						return nil, fmt.Errorf("error: %s", *msg.Result)
					}
					return nil, fmt.Errorf("error: unknown error")
				}
				return msg, nil
			}
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}
