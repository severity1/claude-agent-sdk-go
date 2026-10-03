// Package claudecode provides the Claude Agent SDK for Go.
//
// This SDK enables programmatic interaction with Claude Code CLI through two main APIs:
// - Query() for one-shot requests with automatic cleanup
// - Client for bidirectional streaming conversations
//
// The SDK follows Go-native patterns with goroutines and channels, providing
// context-first design for cancellation and timeouts.
//
// Example usage:
//
//	import "github.com/severity1/claude-agent-sdk-go"
//
//	// One-shot query
//	messages, err := claudecode.Query(ctx, "Hello, Claude!")
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	// Streaming client
//	client := claudecode.NewClient(
//		claudecode.WithSystemPrompt("You are a helpful assistant"),
//	)
//	defer client.Close()
package claudecode

import "github.com/severity1/claude-agent-sdk-go/internal/shared"

// Version represents the current version of the Claude Agent SDK for Go.
const Version = shared.SDKVersion
