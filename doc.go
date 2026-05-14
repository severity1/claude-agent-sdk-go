// Package claudecode provides the Claude Agent SDK for Go.
//
// This SDK enables programmatic interaction with Claude Code CLI through two main APIs:
// - Query() for one-shot requests with automatic cleanup
// - Client for bidirectional streaming conversations
//
// The SDK follows Go-native patterns with goroutines and channels instead of
// async/await, providing context-first design for cancellation and timeouts.
//
// # Basic Usage
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
//
// # ExtraArgs Helpers
//
// The SDK provides convenient helpers for adding arbitrary CLI flags via ExtraArgs:
//
//	// Boolean flags (no value)
//	opts := claudecode.NewOptions(
//		claudecode.WithExtraFlag("fork-session"),
//		claudecode.WithExtraFlag("verbose"),
//	)
//
//	// Flags with values
//	opts := claudecode.NewOptions(
//		claudecode.WithExtraArg("output-format", "json"),
//		claudecode.WithExtraArg("log-level", "debug"),
//	)
//
//	// Mixing both types
//	opts := claudecode.NewOptions(
//		claudecode.WithExtraFlag("verbose"),
//		claudecode.WithExtraArg("format", "xml"),
//	)
//
// These helpers replace manual map[string]*string creation, making the code cleaner
// and easier to use.
//
// The SDK provides 100% feature parity with the Python SDK while embracing
// Go idioms and patterns.
package claudecode

// Version represents the current version of the Claude Agent SDK for Go.
const Version = "0.1.0"
