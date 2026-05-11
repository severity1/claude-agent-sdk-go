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
		{"post_tool_use_failure", HookEventPostToolUseFailure, "PostToolUseFailure"},
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
	// Ensure we have exactly 7 hook events as per Python SDK parity
	// (PR #535 added PostToolUseFailure; events from PR #545 land in item #4).
	events := []HookEvent{
		HookEventPreToolUse,
		HookEventPostToolUse,
		HookEventPostToolUseFailure,
		HookEventUserPromptSubmit,
		HookEventStop,
		HookEventSubagentStop,
		HookEventPreCompact,
	}

	if len(events) != 7 {
		t.Errorf("Expected 7 hook events for Python SDK parity, got %d", len(events))
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

func TestPostToolUseFailureHookInputSerialization(t *testing.T) {
	isInterrupt := true
	input := PostToolUseFailureHookInput{
		BaseHookInput: BaseHookInput{
			SessionID:      "session-123",
			TranscriptPath: "/tmp/transcript.json",
			Cwd:            "/home/user",
		},
		HookEventName: "PostToolUseFailure",
		ToolName:      "Bash",
		ToolInput:     map[string]any{"command": "sleep 60"},
		ToolUseID:     "tool_use_abc",
		Error:         "exit status 1: command not found",
		IsInterrupt:   &isInterrupt,
	}

	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("Failed to marshal PostToolUseFailureHookInput: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal to map: %v", err)
	}

	// Verify JSON field names match Python SDK (PR #535)
	assertHookJSONField(t, result, "hook_event_name", "PostToolUseFailure")
	assertHookJSONField(t, result, "tool_name", "Bash")
	assertHookJSONField(t, result, "tool_use_id", "tool_use_abc")
	assertHookJSONField(t, result, "error", "exit status 1: command not found")

	toolInput, ok := result["tool_input"].(map[string]any)
	if !ok {
		t.Fatal("tool_input should be a map")
		return
	}
	if toolInput["command"] != "sleep 60" {
		t.Errorf("tool_input.command = %v, want %q", toolInput["command"], "sleep 60")
	}

	if result["is_interrupt"] != true {
		t.Errorf("is_interrupt = %v, want true", result["is_interrupt"])
	}
}

func TestPostToolUseFailureHookInputSerializationNilIsInterrupt(t *testing.T) {
	input := PostToolUseFailureHookInput{
		BaseHookInput: BaseHookInput{
			SessionID:      "session-123",
			TranscriptPath: "/tmp/transcript.json",
			Cwd:            "/home/user",
		},
		HookEventName: "PostToolUseFailure",
		ToolName:      "Bash",
		ToolInput:     map[string]any{"command": "ls"},
		ToolUseID:     "tool_use_abc",
		Error:         "boom",
		IsInterrupt:   nil,
	}

	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("Failed to marshal PostToolUseFailureHookInput: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal to map: %v", err)
	}

	// is_interrupt should be omitted when nil (Python's NotRequired[bool])
	if _, exists := result["is_interrupt"]; exists {
		t.Error("is_interrupt should be omitted when IsInterrupt is nil")
	}
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
	decision := testDecisionBlock
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

func TestPostToolUseFailureHookSpecificOutputSerialization(t *testing.T) {
	ctx := "Retry recommended due to transient error"
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
	assertHookJSONField(t, result, "additionalContext", "Retry recommended due to transient error")
}

func TestPostToolUseFailureHookSpecificOutputOmitEmpty(t *testing.T) {
	output := PostToolUseFailureHookSpecificOutput{
		HookEventName: "PostToolUseFailure",
		// AdditionalContext deliberately nil
	}

	data, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("Failed to marshal PostToolUseFailureHookSpecificOutput: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal to map: %v", err)
	}

	if _, exists := result["additionalContext"]; exists {
		t.Error("additionalContext should be omitted when nil")
	}
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
// Helper Functions
// =============================================================================

func assertHookJSONField(t *testing.T, result map[string]any, field string, expected string) {
	t.Helper()
	if result[field] != expected {
		t.Errorf("%s = %v, want %q", field, result[field], expected)
	}
}
