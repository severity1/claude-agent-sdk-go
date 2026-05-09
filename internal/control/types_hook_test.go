package control

import (
	"context"
	"encoding/json"
	"testing"
)

// =============================================================================
// Hook Event Tests
// =============================================================================

func TestHookEventConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant HookEvent
		expected string
	}{
		{"pre_tool_use", HookEventPreToolUse, "PreToolUse"},
		{"post_tool_use", HookEventPostToolUse, "PostToolUse"},
		{"user_prompt_submit", HookEventUserPromptSubmit, "UserPromptSubmit"},
		{"stop", HookEventStop, "Stop"},
		{"subagent_stop", HookEventSubagentStop, "SubagentStop"},
		{"pre_compact", HookEventPreCompact, "PreCompact"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.constant) != tt.expected {
				t.Errorf("HookEvent constant %s = %q, want %q", tt.name, tt.constant, tt.expected)
			}
		})
	}
}

func TestHookEventCount(t *testing.T) {
	// Ensure all 10 hook events are defined per Python SDK parity (Phase 1).
	events := []HookEvent{
		HookEventPreToolUse,
		HookEventPostToolUse,
		HookEventPostToolUseFailure,
		HookEventUserPromptSubmit,
		HookEventStop,
		HookEventSubagentStop,
		HookEventPreCompact,
		HookEventNotification,
		HookEventSubagentStart,
		HookEventPermissionRequest,
	}

	if len(events) != 10 {
		t.Errorf("Expected 10 hook events for Python SDK parity, got %d", len(events))
	}
}

// =============================================================================
// Hook Input Type Tests
// =============================================================================

func TestBaseHookInputSerialization(t *testing.T) {
	input := BaseHookInput{
		SessionID:      "session-123",
		TranscriptPath: "/tmp/transcript.json",
		Cwd:            "/home/user/project",
		PermissionMode: "default",
	}

	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("Failed to marshal BaseHookInput: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal to map: %v", err)
	}

	// Verify JSON field names match Python SDK
	assertHookJSONField(t, result, "session_id", "session-123")
	assertHookJSONField(t, result, "transcript_path", "/tmp/transcript.json")
	assertHookJSONField(t, result, "cwd", "/home/user/project")
	assertHookJSONField(t, result, "permission_mode", "default")
}

func TestPreToolUseHookInputSerialization(t *testing.T) {
	input := PreToolUseHookInput{
		BaseHookInput: BaseHookInput{
			SessionID:      "session-123",
			TranscriptPath: "/tmp/transcript.json",
			Cwd:            "/home/user",
		},
		HookEventName: "PreToolUse",
		ToolName:      "Bash",
		ToolInput:     map[string]any{"command": "ls -la"},
	}

	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("Failed to marshal PreToolUseHookInput: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal to map: %v", err)
	}

	// Verify JSON field names match Python SDK
	assertHookJSONField(t, result, "hook_event_name", "PreToolUse")
	assertHookJSONField(t, result, "tool_name", "Bash")

	toolInput, ok := result["tool_input"].(map[string]any)
	if !ok {
		t.Fatal("tool_input should be a map")
	}
	if toolInput["command"] != "ls -la" {
		t.Errorf("tool_input.command = %v, want %q", toolInput["command"], "ls -la")
	}
}

func TestPostToolUseHookInputSerialization(t *testing.T) {
	input := PostToolUseHookInput{
		BaseHookInput: BaseHookInput{
			SessionID:      "session-123",
			TranscriptPath: "/tmp/transcript.json",
			Cwd:            "/home/user",
		},
		HookEventName: "PostToolUse",
		ToolName:      "Bash",
		ToolInput:     map[string]any{"command": "ls -la"},
		ToolResponse:  "file1.txt\nfile2.txt",
	}

	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("Failed to marshal PostToolUseHookInput: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal to map: %v", err)
	}

	// Verify JSON field names match Python SDK
	assertHookJSONField(t, result, "hook_event_name", "PostToolUse")
	assertHookJSONField(t, result, "tool_name", "Bash")
	assertHookJSONField(t, result, "tool_response", "file1.txt\nfile2.txt")
}

func TestUserPromptSubmitHookInputSerialization(t *testing.T) {
	input := UserPromptSubmitHookInput{
		BaseHookInput: BaseHookInput{
			SessionID:      "session-123",
			TranscriptPath: "/tmp/transcript.json",
			Cwd:            "/home/user",
		},
		HookEventName: "UserPromptSubmit",
		Prompt:        "Please help me fix this bug",
	}

	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("Failed to marshal UserPromptSubmitHookInput: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal to map: %v", err)
	}

	assertHookJSONField(t, result, "hook_event_name", "UserPromptSubmit")
	assertHookJSONField(t, result, "prompt", "Please help me fix this bug")
}

func TestStopHookInputSerialization(t *testing.T) {
	input := StopHookInput{
		BaseHookInput: BaseHookInput{
			SessionID:      "session-123",
			TranscriptPath: "/tmp/transcript.json",
			Cwd:            "/home/user",
		},
		HookEventName:  "Stop",
		StopHookActive: true,
	}

	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("Failed to marshal StopHookInput: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal to map: %v", err)
	}

	assertHookJSONField(t, result, "hook_event_name", "Stop")
	if result["stop_hook_active"] != true {
		t.Errorf("stop_hook_active = %v, want true", result["stop_hook_active"])
	}
}

func TestSubagentStopHookInputSerialization(t *testing.T) {
	input := SubagentStopHookInput{
		BaseHookInput: BaseHookInput{
			SessionID:      "session-123",
			TranscriptPath: "/tmp/transcript.json",
			Cwd:            "/home/user",
		},
		HookEventName:  "SubagentStop",
		StopHookActive: false,
	}

	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("Failed to marshal SubagentStopHookInput: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal to map: %v", err)
	}

	assertHookJSONField(t, result, "hook_event_name", "SubagentStop")
	if result["stop_hook_active"] != false {
		t.Errorf("stop_hook_active = %v, want false", result["stop_hook_active"])
	}
}

func TestPreCompactHookInputSerialization(t *testing.T) {
	customInstructions := "Be concise"
	input := PreCompactHookInput{
		BaseHookInput: BaseHookInput{
			SessionID:      "session-123",
			TranscriptPath: "/tmp/transcript.json",
			Cwd:            "/home/user",
		},
		HookEventName:      "PreCompact",
		Trigger:            "auto",
		CustomInstructions: &customInstructions,
	}

	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("Failed to marshal PreCompactHookInput: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal to map: %v", err)
	}

	assertHookJSONField(t, result, "hook_event_name", "PreCompact")
	assertHookJSONField(t, result, "trigger", "auto")
	assertHookJSONField(t, result, "custom_instructions", "Be concise")
}

func TestPreCompactHookInputSerializationNilCustomInstructions(t *testing.T) {
	input := PreCompactHookInput{
		BaseHookInput: BaseHookInput{
			SessionID:      "session-123",
			TranscriptPath: "/tmp/transcript.json",
			Cwd:            "/home/user",
		},
		HookEventName:      "PreCompact",
		Trigger:            "manual",
		CustomInstructions: nil,
	}

	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("Failed to marshal PreCompactHookInput: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal to map: %v", err)
	}

	// custom_instructions should be omitted when nil
	if _, exists := result["custom_instructions"]; exists {
		t.Error("custom_instructions should be omitted when nil")
	}
}

// =============================================================================
// Hook Output Type Tests
// =============================================================================

func TestHookJSONOutputSerialization(t *testing.T) {
	continueVal := true
	decision := "block" //nolint:goconst // test value - no benefit from constant
	systemMessage := "Tool blocked"
	reason := "Security policy"

	output := HookJSONOutput{
		Continue:      &continueVal,
		Decision:      &decision,
		SystemMessage: &systemMessage,
		Reason:        &reason,
	}

	data, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("Failed to marshal HookJSONOutput: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal to map: %v", err)
	}

	// Verify JSON field names - note: Go can use "continue" directly (not a keyword)
	if result["continue"] != true {
		t.Errorf("continue = %v, want true", result["continue"])
	}
	assertHookJSONField(t, result, "decision", "block")
	assertHookJSONField(t, result, "systemMessage", "Tool blocked")
	assertHookJSONField(t, result, "reason", "Security policy")
}

func TestHookJSONOutputOmitEmpty(t *testing.T) {
	output := HookJSONOutput{} // All fields nil

	data, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("Failed to marshal HookJSONOutput: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal to map: %v", err)
	}

	// All optional fields should be omitted
	unexpectedFields := []string{"continue", "suppressOutput", "stopReason", "decision", "systemMessage", "reason", "hookSpecificOutput"}
	for _, field := range unexpectedFields {
		if _, exists := result[field]; exists {
			t.Errorf("Field %q should be omitted when nil", field)
		}
	}
}

func TestAsyncHookJSONOutputSerialization(t *testing.T) {
	output := AsyncHookJSONOutput{
		Async:        true,
		AsyncTimeout: 5000, // 5 seconds in milliseconds
	}

	data, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("Failed to marshal AsyncHookJSONOutput: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal to map: %v", err)
	}

	// Verify JSON field names - note: Go can use "async" directly (not a keyword)
	if result["async"] != true {
		t.Errorf("async = %v, want true", result["async"])
	}
	// JSON numbers unmarshal as float64
	if result["asyncTimeout"] != float64(5000) {
		t.Errorf("asyncTimeout = %v, want 5000", result["asyncTimeout"])
	}
}

func TestPreToolUseHookSpecificOutputSerialization(t *testing.T) {
	decision := "allow"
	reason := "User approved"
	output := PreToolUseHookSpecificOutput{
		HookEventName:            "PreToolUse",
		PermissionDecision:       &decision,
		PermissionDecisionReason: &reason,
		UpdatedInput:             map[string]any{"command": "ls"},
	}

	data, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("Failed to marshal PreToolUseHookSpecificOutput: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal to map: %v", err)
	}

	assertHookJSONField(t, result, "hookEventName", "PreToolUse")
	assertHookJSONField(t, result, "permissionDecision", "allow")
	assertHookJSONField(t, result, "permissionDecisionReason", "User approved")

	updatedInput, ok := result["updatedInput"].(map[string]any)
	if !ok {
		t.Fatal("updatedInput should be a map")
	}
	if updatedInput["command"] != "ls" {
		t.Errorf("updatedInput.command = %v, want %q", updatedInput["command"], "ls")
	}
}

func TestPostToolUseHookSpecificOutputSerialization(t *testing.T) {
	context := "Tool executed with warnings"
	output := PostToolUseHookSpecificOutput{
		HookEventName:     "PostToolUse",
		AdditionalContext: &context,
	}

	data, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("Failed to marshal PostToolUseHookSpecificOutput: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal to map: %v", err)
	}

	assertHookJSONField(t, result, "hookEventName", "PostToolUse")
	assertHookJSONField(t, result, "additionalContext", "Tool executed with warnings")
}

func TestUserPromptSubmitHookSpecificOutputSerialization(t *testing.T) {
	context := "Additional instructions applied"
	output := UserPromptSubmitHookSpecificOutput{
		HookEventName:     "UserPromptSubmit",
		AdditionalContext: &context,
	}

	data, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("Failed to marshal UserPromptSubmitHookSpecificOutput: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal to map: %v", err)
	}

	assertHookJSONField(t, result, "hookEventName", "UserPromptSubmit")
	assertHookJSONField(t, result, "additionalContext", "Additional instructions applied")
}

// =============================================================================
// Hook Matcher Tests
// =============================================================================

func TestHookMatcherSerialization(t *testing.T) {
	timeout := 30.0
	matcher := HookMatcher{
		Matcher: "Bash|Write",
		Timeout: &timeout,
		// Hooks are not serialized (json:"-")
	}

	data, err := json.Marshal(matcher)
	if err != nil {
		t.Fatalf("Failed to marshal HookMatcher: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal to map: %v", err)
	}

	assertHookJSONField(t, result, "matcher", "Bash|Write")
	if result["timeout"] != float64(30.0) {
		t.Errorf("timeout = %v, want 30.0", result["timeout"])
	}

	// Hooks should not be serialized
	if _, exists := result["hooks"]; exists {
		t.Error("hooks should not be serialized")
	}
}

func TestHookMatcherConfigSerialization(t *testing.T) {
	timeout := 60.0
	config := HookMatcherConfig{
		Matcher:         "Read",
		HookCallbackIDs: []string{"hook_0", "hook_1"},
		Timeout:         &timeout,
	}

	data, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("Failed to marshal HookMatcherConfig: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal to map: %v", err)
	}

	assertHookJSONField(t, result, "matcher", "Read")

	callbackIDs, ok := result["hookCallbackIds"].([]any)
	if !ok {
		t.Fatal("hookCallbackIds should be an array")
	}
	if len(callbackIDs) != 2 {
		t.Errorf("hookCallbackIds length = %d, want 2", len(callbackIDs))
	}
	if callbackIDs[0] != "hook_0" || callbackIDs[1] != "hook_1" {
		t.Errorf("hookCallbackIds = %v, want [hook_0, hook_1]", callbackIDs)
	}
}

// =============================================================================
// HookContext Tests
// =============================================================================

func TestHookContextCreation(t *testing.T) {
	ctx := context.Background()
	hookCtx := HookContext{
		Signal: ctx,
	}

	if hookCtx.Signal != ctx {
		t.Error("HookContext.Signal should hold the provided context")
	}
}

// =============================================================================
// HookCallback Type Tests
// =============================================================================

func TestHookCallbackSignature(t *testing.T) {
	// Verify the callback signature matches expected pattern
	var callback HookCallback = func(
		_ context.Context,
		_ any,
		_ *string,
		_ HookContext,
	) (HookJSONOutput, error) {
		return HookJSONOutput{}, nil
	}

	// Just verify it compiles with correct signature
	ctx := context.Background()
	result, err := callback(ctx, nil, nil, HookContext{})
	if err != nil {
		t.Errorf("Callback returned unexpected error: %v", err)
	}
	if result.Continue != nil {
		t.Error("Empty HookJSONOutput should have nil Continue")
	}
}

// =============================================================================
// PostToolUseFailure Hook Tests (Python SDK PR #535)
// =============================================================================

func TestPostToolUseFailureHookInput_Parsing(t *testing.T) {
	agentID := "agent-123"
	agentType := "subagent"
	isInterrupt := true

	inputData := map[string]any{
		"session_id":      "session-456",
		"transcript_path": "/tmp/transcript.json",
		"cwd":             "/home/user",
		"hook_event_name": "PostToolUseFailure",
		"tool_name":       "Bash", //nolint:goconst // test value - no benefit from constant
		"tool_input":      map[string]any{"command": "rm -rf /"},
		"tool_use_id":     "tool-use-999",
		"error":           "permission denied",
		"is_interrupt":    isInterrupt,
		"agent_id":        agentID,
		"agent_type":      agentType,
	}

	var p Protocol
	result := p.parseHookInput(HookEventPostToolUseFailure, inputData)

	hookInput, ok := result.(*PostToolUseFailureHookInput)
	if !ok {
		t.Fatalf("parseHookInput returned %T, want *PostToolUseFailureHookInput", result)
		return
	}

	if hookInput.HookEventName != "PostToolUseFailure" {
		t.Errorf("HookEventName = %q, want %q", hookInput.HookEventName, "PostToolUseFailure")
	}
	if hookInput.ToolName != "Bash" { //nolint:goconst // test value - no benefit from constant
		t.Errorf("ToolName = %q, want %q", hookInput.ToolName, "Bash")
	}
	if hookInput.Error != "permission denied" {
		t.Errorf("Error = %q, want %q", hookInput.Error, "permission denied")
	}
	if hookInput.ToolUseID != "tool-use-999" {
		t.Errorf("ToolUseID = %q, want %q", hookInput.ToolUseID, "tool-use-999")
	}
	if hookInput.IsInterrupt == nil || !*hookInput.IsInterrupt {
		t.Errorf("IsInterrupt = %v, want true", hookInput.IsInterrupt)
	}
	if hookInput.AgentID == nil || *hookInput.AgentID != agentID {
		t.Errorf("AgentID = %v, want %q", hookInput.AgentID, agentID)
	}
	if hookInput.AgentType == nil || *hookInput.AgentType != agentType {
		t.Errorf("AgentType = %v, want %q", hookInput.AgentType, agentType)
	}
}

func TestPostToolUseFailureHookInput_Serialization(t *testing.T) {
	agentID := "agent-abc"
	isInterrupt := false

	input := PostToolUseFailureHookInput{
		BaseHookInput: BaseHookInput{
			SessionID:      "session-123",
			TranscriptPath: "/tmp/transcript.json",
			Cwd:            "/home/user",
		},
		HookEventName: "PostToolUseFailure",
		ToolName:      "Write",
		ToolInput:     map[string]any{"path": "/etc/passwd"},
		ToolUseID:     "tool-use-1",
		Error:         "access denied",
		IsInterrupt:   &isInterrupt,
		AgentID:       &agentID,
	}

	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("Failed to marshal PostToolUseFailureHookInput: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal to map: %v", err)
	}

	assertHookJSONField(t, result, "hook_event_name", "PostToolUseFailure")
	assertHookJSONField(t, result, "tool_name", "Write")
	assertHookJSONField(t, result, "tool_use_id", "tool-use-1")
	assertHookJSONField(t, result, "error", "access denied")
	assertHookJSONField(t, result, "agent_id", "agent-abc")

	if result["is_interrupt"] != false {
		t.Errorf("is_interrupt = %v, want false", result["is_interrupt"])
	}
}

func TestPostToolUseFailureHookSpecificOutput_Serialization(t *testing.T) {
	ctx := "Failure logged"
	output := PostToolUseFailureHookSpecificOutput{
		HookEventName:     "PostToolUseFailure",
		AdditionalContext: &ctx,
	}

	data, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("Failed to marshal PostToolUseFailureHookSpecificOutput: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal to map: %v", err)
	}

	assertHookJSONField(t, result, "hookEventName", "PostToolUseFailure")
	assertHookJSONField(t, result, "additionalContext", "Failure logged")
}

// =============================================================================
// New Hook Events Tests (Python SDK PR #545)
// =============================================================================

func TestNewHookEvents_Parsing(t *testing.T) {
	tests := []struct {
		name      string
		event     HookEvent
		inputData map[string]any
		check     func(t *testing.T, result any)
	}{
		{
			name:  "notification",
			event: HookEventNotification,
			inputData: map[string]any{
				"session_id":        "session-1",
				"transcript_path":   "/tmp/t.json",
				"cwd":               "/home/user",
				"hook_event_name":   "Notification",
				"message":           "Build completed",
				"title":             "CI",
				"notification_type": "info",
			},
			check: func(t *testing.T, result any) {
				t.Helper()
				hookInput, ok := result.(*NotificationHookInput)
				if !ok {
					t.Fatalf("parseHookInput returned %T, want *NotificationHookInput", result)
					return
				}
				if hookInput.HookEventName != "Notification" {
					t.Errorf("HookEventName = %q, want %q", hookInput.HookEventName, "Notification")
				}
				if hookInput.Message != "Build completed" {
					t.Errorf("Message = %q, want %q", hookInput.Message, "Build completed")
				}
				if hookInput.Title == nil || *hookInput.Title != "CI" {
					t.Errorf("Title = %v, want %q", hookInput.Title, "CI")
				}
				if hookInput.NotificationType != "info" {
					t.Errorf("NotificationType = %q, want %q", hookInput.NotificationType, "info")
				}
			},
		},
		{
			name:  "subagent_start",
			event: HookEventSubagentStart,
			inputData: map[string]any{
				"session_id":      "session-2",
				"transcript_path": "/tmp/t.json",
				"cwd":             "/home/user",
				"hook_event_name": "SubagentStart",
				"agent_id":        "agent-42",
				"agent_type":      "worker",
			},
			check: func(t *testing.T, result any) {
				t.Helper()
				hookInput, ok := result.(*SubagentStartHookInput)
				if !ok {
					t.Fatalf("parseHookInput returned %T, want *SubagentStartHookInput", result)
					return
				}
				if hookInput.HookEventName != "SubagentStart" {
					t.Errorf("HookEventName = %q, want %q", hookInput.HookEventName, "SubagentStart")
				}
				if hookInput.AgentID != "agent-42" {
					t.Errorf("AgentID = %q, want %q", hookInput.AgentID, "agent-42")
				}
				if hookInput.AgentType != "worker" {
					t.Errorf("AgentType = %q, want %q", hookInput.AgentType, "worker")
				}
			},
		},
		{
			name:  "permission_request",
			event: HookEventPermissionRequest,
			inputData: map[string]any{
				"session_id":             "session-3",
				"transcript_path":        "/tmp/t.json",
				"cwd":                    "/home/user",
				"hook_event_name":        "PermissionRequest",
				"tool_name":              "Bash",
				"tool_input":             map[string]any{"command": "sudo rm"},
				"permission_suggestions": []any{"allow", "deny"},
				"agent_id":               "agent-99",
				"agent_type":             "main",
			},
			check: func(t *testing.T, result any) {
				t.Helper()
				hookInput, ok := result.(*PermissionRequestHookInput)
				if !ok {
					t.Fatalf("parseHookInput returned %T, want *PermissionRequestHookInput", result)
					return
				}
				if hookInput.HookEventName != "PermissionRequest" {
					t.Errorf("HookEventName = %q, want %q", hookInput.HookEventName, "PermissionRequest")
				}
				if hookInput.ToolName != "Bash" {
					t.Errorf("ToolName = %q, want %q", hookInput.ToolName, "Bash")
				}
				if len(hookInput.PermissionSuggestions) != 2 {
					t.Errorf("PermissionSuggestions length = %d, want 2", len(hookInput.PermissionSuggestions))
				}
				if hookInput.AgentID == nil || *hookInput.AgentID != "agent-99" {
					t.Errorf("AgentID = %v, want %q", hookInput.AgentID, "agent-99")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var p Protocol
			result := p.parseHookInput(tt.event, tt.inputData)
			tt.check(t, result)
		})
	}
}

func TestNotificationHookSpecificOutput_Serialization(t *testing.T) {
	ctx := "Notification processed"
	output := NotificationHookSpecificOutput{
		HookEventName:     "Notification",
		AdditionalContext: &ctx,
	}

	data, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("Failed to marshal NotificationHookSpecificOutput: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal to map: %v", err)
	}

	assertHookJSONField(t, result, "hookEventName", "Notification")
	assertHookJSONField(t, result, "additionalContext", "Notification processed")
}

func TestSubagentStartHookSpecificOutput_Serialization(t *testing.T) {
	output := SubagentStartHookSpecificOutput{
		HookEventName: "SubagentStart",
	}

	data, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("Failed to marshal SubagentStartHookSpecificOutput: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal to map: %v", err)
	}

	assertHookJSONField(t, result, "hookEventName", "SubagentStart")

	if _, exists := result["additionalContext"]; exists {
		t.Error("additionalContext should be omitted when nil")
	}
}

func TestPermissionRequestHookSpecificOutput_Serialization(t *testing.T) {
	output := PermissionRequestHookSpecificOutput{
		HookEventName: "PermissionRequest",
		Decision: map[string]any{
			"behavior": "allow",
			"updatedInput": map[string]any{
				"command": "ls -la",
			},
		},
	}

	data, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("Failed to marshal PermissionRequestHookSpecificOutput: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal to map: %v", err)
	}

	assertHookJSONField(t, result, "hookEventName", "PermissionRequest")

	decision, ok := result["decision"].(map[string]any)
	if !ok {
		t.Fatal("expected decision field of type map[string]any")
		return
	}
	if decision["behavior"] != "allow" {
		t.Errorf("decision.behavior = %v, want allow", decision["behavior"])
	}
}

func TestPermissionRequestHookSpecificOutput_DecisionRequired(t *testing.T) {
	// Decision is required (no omitempty), so even with nil it must serialize.
	output := PermissionRequestHookSpecificOutput{
		HookEventName: "PermissionRequest",
	}

	data, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("Failed to marshal PermissionRequestHookSpecificOutput: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal to map: %v", err)
	}

	if _, exists := result["decision"]; !exists {
		t.Error("decision field must always be present (required by Python SDK)")
	}
}

func TestPreToolUseHookSpecificOutput_AdditionalContext(t *testing.T) {
	ctx := "extra context for Claude"
	output := PreToolUseHookSpecificOutput{
		HookEventName:     "PreToolUse",
		AdditionalContext: &ctx,
	}

	data, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	assertHookJSONField(t, result, "additionalContext", ctx)
}

func TestPostToolUseHookSpecificOutput_UpdatedMCPToolOutput(t *testing.T) {
	output := PostToolUseHookSpecificOutput{
		HookEventName: "PostToolUse",
		UpdatedMCPToolOutput: map[string]any{
			"replaced": "by hook",
		},
	}

	data, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	updated, ok := result["updatedMCPToolOutput"].(map[string]any)
	if !ok {
		t.Fatal("expected updatedMCPToolOutput as map")
		return
	}
	if updated["replaced"] != "by hook" {
		t.Errorf("updatedMCPToolOutput.replaced = %v, want 'by hook'", updated["replaced"])
	}
}

// TestPostToolUseHookSpecificOutput_UpdatedMCPToolOutputOmitted verifies the
// `updatedMCPToolOutput` field is absent from the wire when unset (nil),
// matching the `omitempty` contract documented on the struct field.
func TestPostToolUseHookSpecificOutput_UpdatedMCPToolOutputOmitted(t *testing.T) {
	ctxStr := "just context"
	output := PostToolUseHookSpecificOutput{
		HookEventName:     "PostToolUse",
		AdditionalContext: &ctxStr,
		// UpdatedMCPToolOutput left nil
	}

	data, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if _, exists := result["updatedMCPToolOutput"]; exists {
		t.Error("updatedMCPToolOutput should be omitted when nil")
	}
	if got, _ := result["additionalContext"].(string); got != ctxStr {
		t.Errorf("additionalContext = %q, want %q", got, ctxStr)
	}
}

// =============================================================================
// Existing Hooks - Agent Fields Tests (Python SDK PR #545)
// =============================================================================

func TestExistingHooks_AgentFields(t *testing.T) {
	t.Run("pre_tool_use_agent_fields", func(t *testing.T) {
		inputData := map[string]any{
			"session_id":      "session-1",
			"transcript_path": "/tmp/t.json",
			"cwd":             "/home/user",
			"hook_event_name": "PreToolUse",
			"tool_name":       "Read",
			"tool_input":      map[string]any{"path": "/etc/hosts"},
			"tool_use_id":     "tuid-1",
			"agent_id":        "agent-1",
			"agent_type":      "main",
		}

		var p Protocol
		result := p.parseHookInput(HookEventPreToolUse, inputData)

		hookInput, ok := result.(*PreToolUseHookInput)
		if !ok {
			t.Fatalf("parseHookInput returned %T, want *PreToolUseHookInput", result)
			return
		}

		if hookInput.ToolUseID != "tuid-1" {
			t.Errorf("ToolUseID = %q, want %q", hookInput.ToolUseID, "tuid-1")
		}
		if hookInput.AgentID == nil || *hookInput.AgentID != "agent-1" {
			t.Errorf("AgentID = %v, want %q", hookInput.AgentID, "agent-1")
		}
		if hookInput.AgentType == nil || *hookInput.AgentType != "main" {
			t.Errorf("AgentType = %v, want %q", hookInput.AgentType, "main")
		}
	})

	t.Run("post_tool_use_agent_fields", func(t *testing.T) {
		inputData := map[string]any{
			"session_id":      "session-1",
			"transcript_path": "/tmp/t.json",
			"cwd":             "/home/user",
			"hook_event_name": "PostToolUse",
			"tool_name":       "Write",
			"tool_input":      map[string]any{"path": "/tmp/out.txt"},
			"tool_response":   "ok",
			"tool_use_id":     "tuid-2",
			"agent_id":        "agent-2",
			"agent_type":      "subagent",
		}

		var p Protocol
		result := p.parseHookInput(HookEventPostToolUse, inputData)

		hookInput, ok := result.(*PostToolUseHookInput)
		if !ok {
			t.Fatalf("parseHookInput returned %T, want *PostToolUseHookInput", result)
			return
		}

		if hookInput.ToolUseID != "tuid-2" {
			t.Errorf("ToolUseID = %q, want %q", hookInput.ToolUseID, "tuid-2")
		}
		if hookInput.AgentID == nil || *hookInput.AgentID != "agent-2" {
			t.Errorf("AgentID = %v, want %q", hookInput.AgentID, "agent-2")
		}
		if hookInput.AgentType == nil || *hookInput.AgentType != "subagent" {
			t.Errorf("AgentType = %v, want %q", hookInput.AgentType, "subagent")
		}
	})

	t.Run("subagent_stop_agent_fields", func(t *testing.T) {
		inputData := map[string]any{
			"session_id":            "session-1",
			"transcript_path":       "/tmp/t.json",
			"cwd":                   "/home/user",
			"hook_event_name":       "SubagentStop",
			"stop_hook_active":      true,
			"agent_id":              "agent-3",
			"agent_transcript_path": "/tmp/agent-transcript.json",
			"agent_type":            "worker",
		}

		var p Protocol
		result := p.parseHookInput(HookEventSubagentStop, inputData)

		hookInput, ok := result.(*SubagentStopHookInput)
		if !ok {
			t.Fatalf("parseHookInput returned %T, want *SubagentStopHookInput", result)
			return
		}

		if hookInput.AgentID != "agent-3" {
			t.Errorf("AgentID = %q, want %q", hookInput.AgentID, "agent-3")
		}
		if hookInput.AgentTranscriptPath != "/tmp/agent-transcript.json" {
			t.Errorf("AgentTranscriptPath = %q, want %q", hookInput.AgentTranscriptPath, "/tmp/agent-transcript.json")
		}
		if hookInput.AgentType != "worker" {
			t.Errorf("AgentType = %q, want %q", hookInput.AgentType, "worker")
		}
	})
}

func TestNewHookEventConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant HookEvent
		expected string
	}{
		{"post_tool_use_failure", HookEventPostToolUseFailure, "PostToolUseFailure"},
		{"notification", HookEventNotification, "Notification"},
		{"subagent_start", HookEventSubagentStart, "SubagentStart"},
		{"permission_request", HookEventPermissionRequest, "PermissionRequest"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.constant) != tt.expected {
				t.Errorf("HookEvent constant %s = %q, want %q", tt.name, tt.constant, tt.expected)
			}
		})
	}
}

// =============================================================================
// Helper Functions
// =============================================================================

func assertHookJSONField(t *testing.T, result map[string]any, field string, expected string) {
	t.Helper()
	if result[field] != expected {
		t.Errorf("%s = %v, want %q", field, result[field], expected)
	}
}
