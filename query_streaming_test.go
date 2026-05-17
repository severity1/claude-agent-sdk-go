package claudecode

import (
	"context"
	"testing"
	"time"
)

// TestQuerySendsPromptAsUserMessage verifies Python SDK PR #468 wire shape:
// the prompt is written as a user-message JSON line on stdin after init,
// with role/content nested under "message" and ParentToolUseID nil.
func TestQuerySendsPromptAsUserMessage(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	transport := newQueryMockTransport(WithQueryAssistantResponse("ok"))
	iter, err := QueryWithTransport(ctx, "What is 2+2?", transport)
	if err != nil {
		t.Fatalf("QueryWithTransport: %v", err)
	}
	defer func() { _ = iter.Close() }()

	// Drain at least one message to ensure SendMessage ran.
	if _, err := iter.Next(ctx); err != nil {
		t.Fatalf("iter.Next: %v", err)
	}

	transport.mu.RLock()
	defer transport.mu.RUnlock()
	if len(transport.receivedMessages) != 1 {
		t.Fatalf("expected exactly 1 stream message, got %d", len(transport.receivedMessages))
	}
	sent := transport.receivedMessages[0]

	if sent.Type != "user" {
		t.Errorf("StreamMessage.Type = %q, want \"user\"", sent.Type)
	}
	if sent.SessionID != "" {
		t.Errorf("StreamMessage.SessionID = %q, want \"\"", sent.SessionID)
	}
	if sent.ParentToolUseID != nil {
		t.Errorf("StreamMessage.ParentToolUseID = %v, want nil", sent.ParentToolUseID)
	}
	msgMap, ok := sent.Message.(map[string]any)
	if !ok {
		t.Fatalf("StreamMessage.Message type %T, want map[string]any", sent.Message)
	}
	if msgMap["role"] != "user" {
		t.Errorf("message.role = %v, want \"user\"", msgMap["role"])
	}
	if msgMap["content"] != "What is 2+2?" {
		t.Errorf("message.content = %v, want \"What is 2+2?\"", msgMap["content"])
	}
}

// TestNeedsBidirectionalStdin pins the decision matrix that drives EndInput
// timing in Query(): hooks / CanUseTool / file checkpointing / SDK MCP keep
// stdin open; bare queries close stdin immediately.
func TestNeedsBidirectionalStdin(t *testing.T) {
	tests := []struct {
		name    string
		options *Options
		want    bool
	}{
		{"nil_options", nil, false},
		{"empty_options", &Options{}, false},
		{"with_hooks", &Options{Hooks: map[HookEvent][]HookMatcher{HookEventPreToolUse: nil}}, true},
		{
			"with_can_use_tool",
			&Options{CanUseTool: func(_ context.Context, _ string, _ map[string]any, _ any) (any, error) {
				return NewPermissionResultAllow(), nil
			}},
			true,
		},
		{"with_file_checkpointing", &Options{EnableFileCheckpointing: true}, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := needsBidirectionalStdin(tc.options)
			if got != tc.want {
				t.Errorf("needsBidirectionalStdin = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestQueryWithAgentsRoutesThroughInitialize verifies that Options.Agents
// flows to the transport without reaching CLI args. Because we use a mock
// transport here, we can only check the Options are preserved end-to-end
// for the transport to consume - the wire-level test for the initialize
// request itself lives in internal/control/initialize_agents_test.go.
func TestQueryWithAgentsRoutesThroughInitialize(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	transport := newQueryMockTransport(WithQueryAssistantResponse("ok"))
	agents := map[string]AgentDefinition{
		"reviewer": {
			Description: "Reviews code",
			Prompt:      "You are a reviewer.",
		},
	}

	iter, err := QueryWithTransport(ctx, "review", transport, WithAgents(agents))
	if err != nil {
		t.Fatalf("QueryWithTransport: %v", err)
	}
	defer func() { _ = iter.Close() }()

	if _, err := iter.Next(ctx); err != nil {
		t.Fatalf("iter.Next: %v", err)
	}
	// At minimum, the query path completed with WithAgents set. The wire-level
	// guarantee is enforced by TestBuildCommandNeverEmitsAgentsFlag and
	// TestInitializeRequestIncludesAgents.
}
