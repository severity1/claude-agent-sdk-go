package control

import (
	"context"
	"encoding/json"
	"testing"
)

const (
	testAgentID   = "agent-42"
	testAgentType = "researcher"
)

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

func TestHookEventCount(t *testing.T) {
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
		t.Errorf("Expected 10 hook events, got %d", len(events))
	}
}

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
		ToolUseID:     "tool_use_abc",
	}

	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("Failed to marshal PreToolUseHookInput: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal to map: %v", err)
	}

	assertHookJSONField(t, result, "hook_event_name", "PreToolUse")
	assertHookJSONField(t, result, "tool_name", "Bash")
	assertHookJSONField(t, result, "tool_use_id", "tool_use_abc")

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
		ToolUseID:     "tool_use_abc",
	}

	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("Failed to marshal PostToolUseHookInput: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal to map: %v", err)
	}

	assertHookJSONField(t, result, "hook_event_name", "PostToolUse")
	assertHookJSONField(t, result, "tool_name", "Bash")
	assertHookJSONField(t, result, "tool_response", "file1.txt\nfile2.txt")
	assertHookJSONField(t, result, "tool_use_id", "tool_use_abc")
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
		HookEventName:       "SubagentStop",
		StopHookActive:      false,
		AgentID:             "agent_xyz",
		AgentTranscriptPath: "/tmp/agent_transcript.json",
		AgentType:           testAgentType,
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
	assertHookJSONField(t, result, "agent_id", "agent_xyz")
	assertHookJSONField(t, result, "agent_transcript_path", "/tmp/agent_transcript.json")
	assertHookJSONField(t, result, "agent_type", testAgentType)
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
	addCtx := "Be careful when running this"
	output := PreToolUseHookSpecificOutput{
		HookEventName:            "PreToolUse",
		PermissionDecision:       &decision,
		PermissionDecisionReason: &reason,
		UpdatedInput:             map[string]any{"command": "ls"},
		AdditionalContext:        &addCtx,
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
	assertHookJSONField(t, result, "additionalContext", "Be careful when running this")

	updatedInput, ok := result["updatedInput"].(map[string]any)
	if !ok {
		t.Fatal("updatedInput should be a map")
	}
	if updatedInput["command"] != "ls" {
		t.Errorf("updatedInput.command = %v, want %q", updatedInput["command"], "ls")
	}
}

func TestPreToolUseHookSpecificOutputOmitEmptyAdditionalContext(t *testing.T) {
	output := PreToolUseHookSpecificOutput{
		HookEventName: "PreToolUse",
		// AdditionalContext deliberately nil
	}

	data, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("Failed to marshal PreToolUseHookSpecificOutput: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal to map: %v", err)
	}

	if _, exists := result["additionalContext"]; exists {
		t.Error("additionalContext should be omitted when nil")
	}
}

func TestPostToolUseHookSpecificOutputSerialization(t *testing.T) {
	context := "Tool executed with warnings"
	output := PostToolUseHookSpecificOutput{
		HookEventName:        "PostToolUse",
		AdditionalContext:    &context,
		UpdatedMCPToolOutput: map[string]any{"summary": "ok"},
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
	updated, ok := result["updatedMCPToolOutput"].(map[string]any)
	if !ok {
		t.Fatal("updatedMCPToolOutput should be a map")
		return
	}
	if updated["summary"] != "ok" {
		t.Errorf("updatedMCPToolOutput.summary = %v, want %q", updated["summary"], "ok")
	}
}

func TestPostToolUseHookSpecificOutputOmitEmptyUpdatedMCPToolOutput(t *testing.T) {
	output := PostToolUseHookSpecificOutput{
		HookEventName: "PostToolUse",
		// UpdatedMCPToolOutput deliberately nil
	}

	data, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("Failed to marshal PostToolUseHookSpecificOutput: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal to map: %v", err)
	}

	if _, exists := result["updatedMCPToolOutput"]; exists {
		t.Error("updatedMCPToolOutput should be omitted when nil")
	}
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

func TestNotificationHookInputSerialization(t *testing.T) {
	title := "Tool Approval Required"
	input := NotificationHookInput{
		BaseHookInput: BaseHookInput{
			SessionID:      "session-123",
			TranscriptPath: "/tmp/transcript.json",
			Cwd:            "/home/user",
		},
		HookEventName:    "Notification",
		Message:          "Bash requested permission to run a command",
		Title:            &title,
		NotificationType: "permission_request",
	}

	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("Failed to marshal NotificationHookInput: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal to map: %v", err)
	}

	assertHookJSONField(t, result, "hook_event_name", "Notification")
	assertHookJSONField(t, result, "message", "Bash requested permission to run a command")
	assertHookJSONField(t, result, "title", "Tool Approval Required")
	assertHookJSONField(t, result, "notification_type", "permission_request")
}

func TestNotificationHookInputSerializationNilTitle(t *testing.T) {
	input := NotificationHookInput{
		BaseHookInput: BaseHookInput{
			SessionID:      "session-123",
			TranscriptPath: "/tmp/transcript.json",
			Cwd:            "/home/user",
		},
		HookEventName:    "Notification",
		Message:          "Something happened",
		Title:            nil,
		NotificationType: "info",
	}

	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("Failed to marshal NotificationHookInput: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal to map: %v", err)
	}

	// title should be omitted when nil (Python's NotRequired[str])
	if _, exists := result["title"]; exists {
		t.Error("title should be omitted when Title is nil")
	}
}

func TestSubagentStartHookInputSerialization(t *testing.T) {
	input := SubagentStartHookInput{
		BaseHookInput: BaseHookInput{
			SessionID:      "session-123",
			TranscriptPath: "/tmp/transcript.json",
			Cwd:            "/home/user",
		},
		HookEventName: "SubagentStart",
		AgentID:       "agent_xyz",
		AgentType:     testAgentType,
	}

	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("Failed to marshal SubagentStartHookInput: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal to map: %v", err)
	}

	assertHookJSONField(t, result, "hook_event_name", "SubagentStart")
	assertHookJSONField(t, result, "agent_id", "agent_xyz")
	assertHookJSONField(t, result, "agent_type", testAgentType)
}

func TestPermissionRequestHookInputSerialization(t *testing.T) {
	input := PermissionRequestHookInput{
		BaseHookInput: BaseHookInput{
			SessionID:      "session-123",
			TranscriptPath: "/tmp/transcript.json",
			Cwd:            "/home/user",
		},
		HookEventName: "PermissionRequest",
		ToolName:      "Bash",
		ToolInput:     map[string]any{"command": "rm -rf foo"},
		PermissionSuggestions: []any{
			map[string]any{"type": "addRules", "rules": []any{"Bash(ls:*)"}},
		},
	}

	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("Failed to marshal PermissionRequestHookInput: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal to map: %v", err)
	}

	assertHookJSONField(t, result, "hook_event_name", "PermissionRequest")
	assertHookJSONField(t, result, "tool_name", "Bash")

	toolInput, ok := result["tool_input"].(map[string]any)
	if !ok {
		t.Fatal("tool_input should be a map")
		return
	}
	if toolInput["command"] != "rm -rf foo" {
		t.Errorf("tool_input.command = %v, want %q", toolInput["command"], "rm -rf foo")
	}

	suggestions, ok := result["permission_suggestions"].([]any)
	if !ok {
		t.Fatal("permission_suggestions should be a slice")
		return
	}
	if len(suggestions) != 1 {
		t.Errorf("permission_suggestions length = %d, want 1", len(suggestions))
	}
}

func TestPermissionRequestHookInputSerializationNilPermissionSuggestions(t *testing.T) {
	input := PermissionRequestHookInput{
		BaseHookInput: BaseHookInput{
			SessionID:      "session-123",
			TranscriptPath: "/tmp/transcript.json",
			Cwd:            "/home/user",
		},
		HookEventName:         "PermissionRequest",
		ToolName:              "Bash",
		ToolInput:             map[string]any{"command": "ls"},
		PermissionSuggestions: nil,
	}

	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("Failed to marshal PermissionRequestHookInput: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal to map: %v", err)
	}

	if _, exists := result["permission_suggestions"]; exists {
		t.Error("permission_suggestions should be omitted when nil")
	}
}

func TestNotificationHookSpecificOutputSerialization(t *testing.T) {
	addCtx := "User dismissed the notification"
	output := NotificationHookSpecificOutput{
		HookEventName:     "Notification",
		AdditionalContext: &addCtx,
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
	assertHookJSONField(t, result, "additionalContext", "User dismissed the notification")
}

func TestNotificationHookSpecificOutputOmitEmpty(t *testing.T) {
	output := NotificationHookSpecificOutput{
		HookEventName: "Notification",
	}

	data, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("Failed to marshal NotificationHookSpecificOutput: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal to map: %v", err)
	}

	if _, exists := result["additionalContext"]; exists {
		t.Error("additionalContext should be omitted when nil")
	}
}

func TestSubagentStartHookSpecificOutputSerialization(t *testing.T) {
	addCtx := "Spawning researcher subagent"
	output := SubagentStartHookSpecificOutput{
		HookEventName:     "SubagentStart",
		AdditionalContext: &addCtx,
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
	assertHookJSONField(t, result, "additionalContext", "Spawning researcher subagent")
}

func TestSubagentStartHookSpecificOutputOmitEmpty(t *testing.T) {
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

	if _, exists := result["additionalContext"]; exists {
		t.Error("additionalContext should be omitted when nil")
	}
}

func TestPermissionRequestHookSpecificOutputSerialization(t *testing.T) {
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
		t.Fatal("decision should be a map")
		return
	}
	if decision["behavior"] != "allow" {
		t.Errorf("decision.behavior = %v, want %q", decision["behavior"], "allow")
	}
}

func TestPermissionRequestHookSpecificOutputEmptyDecision(t *testing.T) {
	// Python's decision field is required (not NotRequired); ensure it always
	// serializes even when empty, including its key in JSON output.
	output := PermissionRequestHookSpecificOutput{
		HookEventName: "PermissionRequest",
		Decision:      map[string]any{},
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
		t.Error("decision key must be present even when map is empty (not omitempty)")
	}
}

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

func TestHookContextCreation(t *testing.T) {
	ctx := context.Background()
	hookCtx := HookContext{
		Signal: ctx,
	}

	if hookCtx.Signal != ctx {
		t.Error("HookContext.Signal should hold the provided context")
	}
}

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

func assertHookJSONField(t *testing.T, result map[string]any, field string, expected string) {
	t.Helper()
	if result[field] != expected {
		t.Errorf("%s = %v, want %q", field, result[field], expected)
	}
}

func TestToolHookInputsSubagentContextSerialization(t *testing.T) {
	tests := []struct {
		name       string
		subagent   any
		mainThread any
	}{
		{
			name:       "PreToolUse",
			subagent:   PreToolUseHookInput{HookEventName: "PreToolUse", AgentID: strPtrForTest(testAgentID), AgentType: strPtrForTest(testAgentType)},
			mainThread: PreToolUseHookInput{HookEventName: "PreToolUse"},
		},
		{
			name:       "PostToolUse",
			subagent:   PostToolUseHookInput{HookEventName: "PostToolUse", AgentID: strPtrForTest(testAgentID), AgentType: strPtrForTest(testAgentType)},
			mainThread: PostToolUseHookInput{HookEventName: "PostToolUse"},
		},
		{
			name:       "PostToolUseFailure",
			subagent:   PostToolUseFailureHookInput{HookEventName: "PostToolUseFailure", AgentID: strPtrForTest(testAgentID), AgentType: strPtrForTest(testAgentType)},
			mainThread: PostToolUseFailureHookInput{HookEventName: "PostToolUseFailure"},
		},
		{
			name:       "PermissionRequest",
			subagent:   PermissionRequestHookInput{HookEventName: "PermissionRequest", AgentID: strPtrForTest(testAgentID), AgentType: strPtrForTest(testAgentType)},
			mainThread: PermissionRequestHookInput{HookEventName: "PermissionRequest"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name+"/subagent", func(t *testing.T) {
			result := marshalHookInputToMap(t, tt.subagent)
			assertHookJSONField(t, result, "agent_id", testAgentID)
			assertHookJSONField(t, result, "agent_type", testAgentType)
		})
		t.Run(tt.name+"/main_thread", func(t *testing.T) {
			result := marshalHookInputToMap(t, tt.mainThread)
			for _, key := range []string{"agent_id", "agent_type"} {
				if _, ok := result[key]; ok {
					t.Errorf("%s should be absent on the main thread, got %v", key, result[key])
				}
			}
		})
	}
}

func TestParseHookInputSubagentContext(t *testing.T) {
	events := []HookEvent{
		HookEventPreToolUse,
		HookEventPostToolUse,
		HookEventPostToolUseFailure,
		HookEventPermissionRequest,
	}
	p := &Protocol{}

	for _, event := range events {
		t.Run(string(event)+"/subagent", func(t *testing.T) {
			parsed := p.parseHookInput(event, map[string]any{
				"hook_event_name": string(event),
				"tool_name":       "Bash",
				"agent_id":        testAgentID,
				"agent_type":      testAgentType,
			})
			agentID, agentType := subagentContextOf(t, parsed)
			if agentID == nil || *agentID != testAgentID {
				t.Errorf("AgentID = %v, want agent-42", agentID)
			}
			if agentType == nil || *agentType != testAgentType {
				t.Errorf("AgentType = %v, want researcher", agentType)
			}
		})
		t.Run(string(event)+"/main_thread", func(t *testing.T) {
			parsed := p.parseHookInput(event, map[string]any{
				"hook_event_name": string(event),
				"tool_name":       "Bash",
			})
			agentID, agentType := subagentContextOf(t, parsed)
			if agentID != nil || agentType != nil {
				t.Errorf("AgentID = %v, AgentType = %v, want both nil", agentID, agentType)
			}
		})
	}
}

func marshalHookInputToMap(t *testing.T, input any) map[string]any {
	t.Helper()
	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("Failed to marshal %T: %v", input, err)
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal to map: %v", err)
	}
	return result
}

func strPtrForTest(s string) *string { return &s }

func subagentContextOf(t *testing.T, parsed any) (agentID, agentType *string) {
	t.Helper()
	switch in := parsed.(type) {
	case *PreToolUseHookInput:
		return in.AgentID, in.AgentType
	case *PostToolUseHookInput:
		return in.AgentID, in.AgentType
	case *PostToolUseFailureHookInput:
		return in.AgentID, in.AgentType
	case *PermissionRequestHookInput:
		return in.AgentID, in.AgentType
	default:
		t.Fatalf("unexpected hook input type %T", parsed)
		return nil, nil
	}
}
