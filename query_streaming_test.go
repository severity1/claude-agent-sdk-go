package claudecode

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

// TestQuerySendsPromptAsUserMessage pins the prompt-line wire shape: a
// user-message JSON object that always carries `session_id` and
// `parent_tool_use_id` keys, with role and content nested under `message`.
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

	// Wire-bytes assertion: the JSON line the CLI would receive must carry
	// both `session_id` (empty string) and `parent_tool_use_id` (null)
	// keys regardless of whether they were explicitly set.
	raw, err := json.Marshal(sent)
	if err != nil {
		t.Fatalf("json.Marshal(sent): %v", err)
	}
	var wire map[string]any
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatalf("json.Unmarshal: %v (raw=%s)", err, raw)
	}
	sessionVal, ok := wire["session_id"]
	if !ok {
		t.Errorf("wire JSON missing `session_id` key: %s", raw)
	} else if sessionVal != "" {
		t.Errorf("wire `session_id` = %v, want \"\"", sessionVal)
	}
	parentVal, ok := wire["parent_tool_use_id"]
	if !ok {
		t.Errorf("wire JSON missing `parent_tool_use_id` key: %s", raw)
	} else if parentVal != nil {
		t.Errorf("wire `parent_tool_use_id` = %v, want null", parentVal)
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
