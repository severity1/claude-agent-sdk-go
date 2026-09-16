package shared

import (
	"encoding/json"
	"testing"
)

// TestMessageTypes tests all message types using table-driven approach
func TestMessageTypes(t *testing.T) {
	tests := []struct {
		name         string
		createMsg    func() Message
		expectedType string
		validateFunc func(*testing.T, Message)
	}{
		{
			name: "user_message",
			createMsg: func() Message {
				return &UserMessage{Content: "Hello, Claude!"}
			},
			expectedType: MessageTypeUser,
			validateFunc: validateUserMessage,
		},
		{
			name: "assistant_message",
			createMsg: func() Message {
				return &AssistantMessage{
					Content: []ContentBlock{&TextBlock{Text: "Hello! How can I help?"}},
					Model:   "claude-3-sonnet",
				}
			},
			expectedType: MessageTypeAssistant,
			validateFunc: validateAssistantMessage,
		},
		{
			name: "system_message",
			createMsg: func() Message {
				return &SystemMessage{
					Subtype: "user_confirmation",
					Data: map[string]any{
						"type":     MessageTypeSystem,
						"subtype":  "user_confirmation",
						"question": "Do you want to proceed?",
						"options":  []string{"yes", "no"},
					},
				}
			},
			expectedType: MessageTypeSystem,
			validateFunc: validateSystemMessage,
		},
		{
			name: "result_message",
			createMsg: func() Message {
				return &ResultMessage{
					Subtype:   "completion",
					SessionID: "test-session",
				}
			},
			expectedType: MessageTypeResult,
			validateFunc: validateResultMessage,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			msg := test.createMsg()
			assertMessageType(t, msg, test.expectedType)
			test.validateFunc(t, msg)
		})
	}
}

// TestContentBlockTypes tests all content block types using table-driven approach
func TestContentBlockTypes(t *testing.T) {
	tests := []struct {
		name         string
		createBlock  func() ContentBlock
		expectedType string
		validateFunc func(*testing.T, ContentBlock)
	}{
		{
			name: "text_block",
			createBlock: func() ContentBlock {
				return &TextBlock{Text: "This is a text block"}
			},
			expectedType: ContentBlockTypeText,
			validateFunc: validateTextBlock,
		},
		{
			name: "thinking_block",
			createBlock: func() ContentBlock {
				return &ThinkingBlock{
					Thinking:  "Let me think about this...",
					Signature: "claude-3-sonnet-20240229",
				}
			},
			expectedType: ContentBlockTypeThinking,
			validateFunc: validateThinkingBlock,
		},
		{
			name: "tool_use_block",
			createBlock: func() ContentBlock {
				return &ToolUseBlock{
					ToolUseID: "tool_456",
					Name:      "Read",
					Input: map[string]any{
						"file_path": "/home/user/document.txt",
						"limit":     100,
					},
				}
			},
			expectedType: ContentBlockTypeToolUse,
			validateFunc: validateToolUseBlock,
		},
		{
			name: "tool_result_block",
			createBlock: func() ContentBlock {
				isError := false
				return &ToolResultBlock{
					ToolUseID: "tool_456",
					Content:   "File content here...",
					IsError:   &isError,
				}
			},
			expectedType: ContentBlockTypeToolResult,
			validateFunc: validateToolResultBlock,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			block := test.createBlock()
			assertContentBlockType(t, block, test.expectedType)
			test.validateFunc(t, block)
		})
	}
}

// TestJSONMarshaling tests JSON marshaling for complex message types
func TestJSONMarshaling(t *testing.T) {
	// Test SystemMessage preserves all data fields
	systemMsg := &SystemMessage{
		Subtype: "user_confirmation",
		Data: map[string]any{
			"type":     MessageTypeSystem,
			"subtype":  "user_confirmation",
			"question": "Do you want to proceed?",
			"options":  []string{"yes", "no"},
		},
	}

	jsonData, err := json.Marshal(systemMsg)
	if err != nil {
		t.Fatalf("Failed to marshal SystemMessage: %v", err)
	}

	assertJSONField(t, jsonData, "type", MessageTypeSystem)
	assertJSONField(t, jsonData, "subtype", "user_confirmation")
	assertJSONField(t, jsonData, "question", "Do you want to proceed?")

	// Test AssistantMessage with model field
	assistantMsg := &AssistantMessage{
		Content: []ContentBlock{&TextBlock{Text: "Hello!"}},
		Model:   "claude-3-sonnet",
	}

	jsonData, err = json.Marshal(assistantMsg)
	if err != nil {
		t.Fatalf("Failed to marshal AssistantMessage: %v", err)
	}

	assertJSONField(t, jsonData, "type", MessageTypeAssistant)
	assertJSONField(t, jsonData, "model", "claude-3-sonnet")

	// Test UserMessage with string content
	userMsg := &UserMessage{Content: "Hello, Claude!"}

	jsonData, err = json.Marshal(userMsg)
	if err != nil {
		t.Fatalf("Failed to marshal UserMessage: %v", err)
	}

	assertJSONField(t, jsonData, "type", MessageTypeUser)
	assertJSONField(t, jsonData, "content", "Hello, Claude!")
}

// TestInterfaceCompliance tests interface implementation for all types
func TestInterfaceCompliance(t *testing.T) {
	// Test Message interface compliance
	messages := []Message{
		&UserMessage{Content: "test"},
		&AssistantMessage{Content: []ContentBlock{}, Model: "test"},
		&SystemMessage{Subtype: "test", Data: map[string]any{}},
		&ResultMessage{Subtype: "completion", SessionID: "test"},
	}

	expectedTypes := []string{
		MessageTypeUser,
		MessageTypeAssistant,
		MessageTypeSystem,
		MessageTypeResult,
	}

	for i, msg := range messages {
		assertMessageType(t, msg, expectedTypes[i])
	}

	// Test ContentBlock interface compliance
	blocks := []ContentBlock{
		&TextBlock{Text: "test"},
		&ThinkingBlock{Thinking: "test", Signature: "test"},
		&ToolUseBlock{ToolUseID: "test", Name: "test", Input: map[string]any{}},
		&ToolResultBlock{ToolUseID: "test", Content: "test"},
	}

	expectedBlockTypes := []string{
		ContentBlockTypeText,
		ContentBlockTypeThinking,
		ContentBlockTypeToolUse,
		ContentBlockTypeToolResult,
	}

	for i, block := range blocks {
		assertContentBlockType(t, block, expectedBlockTypes[i])
	}
}

// Helper functions

// assertMessageType verifies message has expected type
func assertMessageType(t *testing.T, msg Message, expectedType string) {
	t.Helper()
	if msg.Type() != expectedType {
		t.Errorf("Expected message type %q, got %q", expectedType, msg.Type())
	}
}

// assertContentBlockType verifies content block has expected type
func assertContentBlockType(t *testing.T, block ContentBlock, expectedType string) {
	t.Helper()
	if block.BlockType() != expectedType {
		t.Errorf("Expected block type %q, got %q", expectedType, block.BlockType())
	}
}

// assertJSONField verifies JSON contains expected field with value
func assertJSONField(t *testing.T, jsonData []byte, field string, expected any) {
	t.Helper()
	var result map[string]any
	if err := json.Unmarshal(jsonData, &result); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	if result[field] != expected {
		t.Errorf("Expected JSON field %q = %v, got %v", field, expected, result[field])
	}
}

// Message-specific validation functions

// validateUserMessage validates UserMessage specifics
func validateUserMessage(t *testing.T, msg Message) {
	t.Helper()
	userMsg, ok := msg.(*UserMessage)
	if !ok {
		t.Fatalf("Expected *UserMessage, got %T", msg)
	}
	if userMsg.Content == nil {
		t.Error("Expected non-nil Content field")
	}
}

// validateAssistantMessage validates AssistantMessage specifics
func validateAssistantMessage(t *testing.T, msg Message) {
	t.Helper()
	assistantMsg, ok := msg.(*AssistantMessage)
	if !ok {
		t.Fatalf("Expected *AssistantMessage, got %T", msg)
	}
	if assistantMsg.Content == nil {
		t.Error("Expected non-nil Content field")
	}
	if assistantMsg.Model == "" {
		t.Error("Expected non-empty Model field")
	}
}

// validateSystemMessage validates SystemMessage specifics
func validateSystemMessage(t *testing.T, msg Message) {
	t.Helper()
	systemMsg, ok := msg.(*SystemMessage)
	if !ok {
		t.Fatalf("Expected *SystemMessage, got %T", msg)
	}
	if systemMsg.Subtype == "" {
		t.Error("Expected non-empty Subtype field")
	}
	if systemMsg.Data == nil {
		t.Error("Expected non-nil Data field")
	}
}

// validateResultMessage validates ResultMessage specifics
func validateResultMessage(t *testing.T, msg Message) {
	t.Helper()
	resultMsg, ok := msg.(*ResultMessage)
	if !ok {
		t.Fatalf("Expected *ResultMessage, got %T", msg)
	}
	if resultMsg.Subtype == "" {
		t.Error("Expected non-empty Subtype field")
	}
	if resultMsg.SessionID == "" {
		t.Error("Expected non-empty SessionID field")
	}
}

// Content block validation functions

// validateTextBlock validates TextBlock specifics
func validateTextBlock(t *testing.T, block ContentBlock) {
	t.Helper()
	textBlock, ok := block.(*TextBlock)
	if !ok {
		t.Fatalf("Expected *TextBlock, got %T", block)
	}
	if textBlock.Text == "" {
		t.Error("Expected non-empty Text field")
	}
}

// validateThinkingBlock validates ThinkingBlock specifics
func validateThinkingBlock(t *testing.T, block ContentBlock) {
	t.Helper()
	thinkingBlock, ok := block.(*ThinkingBlock)
	if !ok {
		t.Fatalf("Expected *ThinkingBlock, got %T", block)
	}
	if thinkingBlock.Thinking == "" {
		t.Error("Expected non-empty Thinking field")
	}
	if thinkingBlock.Signature == "" {
		t.Error("Expected non-empty Signature field")
	}
}

// validateToolUseBlock validates ToolUseBlock specifics
func validateToolUseBlock(t *testing.T, block ContentBlock) {
	t.Helper()
	toolBlock, ok := block.(*ToolUseBlock)
	if !ok {
		t.Fatalf("Expected *ToolUseBlock, got %T", block)
	}
	if toolBlock.ToolUseID == "" {
		t.Error("Expected non-empty ToolUseID field")
	}
	if toolBlock.Name == "" {
		t.Error("Expected non-empty Name field")
	}
	if toolBlock.Input == nil {
		t.Error("Expected non-nil Input field")
	}
}

// validateToolResultBlock validates ToolResultBlock specifics
func validateToolResultBlock(t *testing.T, block ContentBlock) {
	t.Helper()
	resultBlock, ok := block.(*ToolResultBlock)
	if !ok {
		t.Fatalf("Expected *ToolResultBlock, got %T", block)
	}
	if resultBlock.ToolUseID == "" {
		t.Error("Expected non-empty ToolUseID field")
	}
	if resultBlock.Content == nil {
		t.Error("Expected non-nil Content field")
	}
}

// TestUserMessageGetUUID tests the GetUUID helper method
func TestUserMessageGetUUID(t *testing.T) {
	tests := []struct {
		name     string
		uuid     *string
		expected string
	}{
		{"nil uuid returns empty", nil, ""},
		{"non-nil uuid returns value", strPtr("msg-123"), "msg-123"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := &UserMessage{UUID: tt.uuid}
			if got := msg.GetUUID(); got != tt.expected {
				t.Errorf("GetUUID() = %q, want %q", got, tt.expected)
			}
		})
	}
}

// TestUserMessageGetParentToolUseID tests the GetParentToolUseID helper method
func TestUserMessageGetParentToolUseID(t *testing.T) {
	tests := []struct {
		name     string
		id       *string
		expected string
	}{
		{"nil returns empty", nil, ""},
		{"non-nil returns value", strPtr("tool-456"), "tool-456"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := &UserMessage{ParentToolUseID: tt.id}
			if got := msg.GetParentToolUseID(); got != tt.expected {
				t.Errorf("GetParentToolUseID() = %q, want %q", got, tt.expected)
			}
		})
	}
}

// TestUserMessageJSONMarshalingWithOptionalFields tests JSON marshaling with UUID and ParentToolUseID
func TestUserMessageJSONMarshalingWithOptionalFields(t *testing.T) {
	// Test with both fields set
	uuid := "msg-123"
	parentToolUseID := "tool-456"
	userMsg := &UserMessage{
		Content:         "Hello",
		UUID:            &uuid,
		ParentToolUseID: &parentToolUseID,
	}

	jsonData, err := json.Marshal(userMsg)
	if err != nil {
		t.Fatalf("Failed to marshal UserMessage: %v", err)
	}

	assertJSONField(t, jsonData, "type", MessageTypeUser)
	assertJSONField(t, jsonData, "uuid", "msg-123")
	assertJSONField(t, jsonData, "parent_tool_use_id", "tool-456")

	// Test without optional fields (should not include them in JSON)
	userMsgNoOptional := &UserMessage{Content: "Hello"}
	jsonDataNoOptional, err := json.Marshal(userMsgNoOptional)
	if err != nil {
		t.Fatalf("Failed to marshal UserMessage: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(jsonDataNoOptional, &result); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	if _, exists := result["uuid"]; exists {
		t.Error("Expected 'uuid' field to be omitted when nil")
	}
	if _, exists := result["parent_tool_use_id"]; exists {
		t.Error("Expected 'parent_tool_use_id' field to be omitted when nil")
	}
}

// TestAssistantMessageGetParentToolUseID tests the GetParentToolUseID helper method
func TestAssistantMessageGetParentToolUseID(t *testing.T) {
	tests := []struct {
		name     string
		id       *string
		expected string
	}{
		{"nil returns empty", nil, ""},
		{"non-nil returns value", strPtr("tool-abc"), "tool-abc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := &AssistantMessage{ParentToolUseID: tt.id}
			if got := msg.GetParentToolUseID(); got != tt.expected {
				t.Errorf("GetParentToolUseID() = %q, want %q", got, tt.expected)
			}
		})
	}
}

// TestAssistantMessageJSONMarshalingWithParentToolUseID tests JSON marshaling of ParentToolUseID
func TestAssistantMessageJSONMarshalingWithParentToolUseID(t *testing.T) {
	parentToolUseID := "tool-abc"
	asstMsg := &AssistantMessage{
		Content:         []ContentBlock{},
		Model:           "claude-3-sonnet",
		ParentToolUseID: &parentToolUseID,
	}

	jsonData, err := json.Marshal(asstMsg)
	if err != nil {
		t.Fatalf("Failed to marshal AssistantMessage: %v", err)
	}
	assertJSONField(t, jsonData, "type", MessageTypeAssistant)
	assertJSONField(t, jsonData, "parent_tool_use_id", "tool-abc")

	asstMsgNoOptional := &AssistantMessage{Content: []ContentBlock{}, Model: "claude-3-sonnet"}
	jsonDataNoOptional, err := json.Marshal(asstMsgNoOptional)
	if err != nil {
		t.Fatalf("Failed to marshal AssistantMessage: %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal(jsonDataNoOptional, &result); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}
	if _, exists := result["parent_tool_use_id"]; exists {
		t.Error("Expected 'parent_tool_use_id' field to be omitted when nil")
	}
}

// strPtr is a helper to create a pointer to a string
func strPtr(s string) *string {
	return &s
}

// TestAssistantMessageJSONMarshalingWithUsage tests that Usage round-trips
// through MarshalJSON (which uses the type-alias embedding trick) and is
// omitted when nil.
func TestAssistantMessageJSONMarshalingWithUsage(t *testing.T) {
	usage := map[string]any{
		"input_tokens":            float64(2),
		"output_tokens":           float64(4),
		"cache_read_input_tokens": float64(9693),
	}
	asstMsg := &AssistantMessage{
		Content: []ContentBlock{},
		Model:   "claude-3-sonnet",
		Usage:   &usage,
	}

	jsonData, err := json.Marshal(asstMsg)
	if err != nil {
		t.Fatalf("Failed to marshal AssistantMessage: %v", err)
	}
	assertJSONField(t, jsonData, "type", MessageTypeAssistant)

	var result map[string]any
	if err := json.Unmarshal(jsonData, &result); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}
	roundTripped, ok := result["usage"].(map[string]any)
	if !ok {
		t.Fatalf("expected usage field to be an object, got %v", result["usage"])
	}
	if roundTripped["input_tokens"] != float64(2) {
		t.Errorf("expected input_tokens 2, got %v", roundTripped["input_tokens"])
	}

	asstMsgNoOptional := &AssistantMessage{Content: []ContentBlock{}, Model: "claude-3-sonnet"}
	jsonDataNoOptional, err := json.Marshal(asstMsgNoOptional)
	if err != nil {
		t.Fatalf("Failed to marshal AssistantMessage: %v", err)
	}
	var resultNoOptional map[string]any
	if err := json.Unmarshal(jsonDataNoOptional, &resultNoOptional); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}
	if _, exists := resultNoOptional["usage"]; exists {
		t.Error("Expected 'usage' field to be omitted when nil")
	}
}

// TestAssistantMessageWithError tests AssistantMessage with the Error field set
func TestAssistantMessageWithError(t *testing.T) {
	tests := []struct {
		name      string
		errType   AssistantMessageError
		wantError bool
		wantRate  bool
	}{
		{"rate_limit", AssistantMessageErrorRateLimit, true, true},
		{"auth_failed", AssistantMessageErrorAuthFailed, true, false},
		{"billing", AssistantMessageErrorBilling, true, false},
		{"invalid_request", AssistantMessageErrorInvalidRequest, true, false},
		{"server_error", AssistantMessageErrorServer, true, false},
		{"unknown", AssistantMessageErrorUnknown, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errType := tt.errType // Avoid memory aliasing in for loop
			msg := &AssistantMessage{
				Content: []ContentBlock{&TextBlock{Text: "Error response"}},
				Model:   "claude-3-sonnet",
				Error:   &errType,
			}

			if got := msg.HasError(); got != tt.wantError {
				t.Errorf("HasError() = %v, want %v", got, tt.wantError)
			}
			if got := msg.IsRateLimited(); got != tt.wantRate {
				t.Errorf("IsRateLimited() = %v, want %v", got, tt.wantRate)
			}
		})
	}
}

// TestAssistantMessageNoError tests AssistantMessage without Error field
func TestAssistantMessageNoError(t *testing.T) {
	msg := &AssistantMessage{
		Content: []ContentBlock{&TextBlock{Text: "Hello!"}},
		Model:   "claude-3-sonnet",
		Error:   nil,
	}

	if msg.HasError() {
		t.Error("HasError() = true, want false for nil error")
	}
	if msg.GetError() != "" {
		t.Errorf("GetError() = %q, want empty string for nil error", msg.GetError())
	}
	if msg.IsRateLimited() {
		t.Error("IsRateLimited() = true, want false for nil error")
	}
}

// TestAssistantMessageGetError tests the GetError helper method
func TestAssistantMessageGetError(t *testing.T) {
	tests := []struct {
		name     string
		errType  *AssistantMessageError
		expected AssistantMessageError
	}{
		{"nil error returns empty", nil, ""},
		{"rate_limit returns value", func() *AssistantMessageError { e := AssistantMessageErrorRateLimit; return &e }(), AssistantMessageErrorRateLimit},
		{"auth_failed returns value", func() *AssistantMessageError { e := AssistantMessageErrorAuthFailed; return &e }(), AssistantMessageErrorAuthFailed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := &AssistantMessage{Error: tt.errType}
			if got := msg.GetError(); got != tt.expected {
				t.Errorf("GetError() = %q, want %q", got, tt.expected)
			}
		})
	}
}

// TestAssistantMessageErrorJSONMarshaling tests JSON marshaling with error field
func TestAssistantMessageErrorJSONMarshaling(t *testing.T) {
	// Test with error field set
	errType := AssistantMessageErrorRateLimit
	msgWithError := &AssistantMessage{
		Content: []ContentBlock{&TextBlock{Text: "Rate limited"}},
		Model:   "claude-3-sonnet",
		Error:   &errType,
	}

	jsonData, err := json.Marshal(msgWithError)
	if err != nil {
		t.Fatalf("Failed to marshal AssistantMessage with error: %v", err)
	}

	assertJSONField(t, jsonData, "type", MessageTypeAssistant)
	assertJSONField(t, jsonData, "model", "claude-3-sonnet")
	assertJSONField(t, jsonData, "error", "rate_limit")

	// Test without error field (should be omitted)
	msgNoError := &AssistantMessage{
		Content: []ContentBlock{&TextBlock{Text: "Hello!"}},
		Model:   "claude-3-sonnet",
		Error:   nil,
	}

	jsonDataNoError, err := json.Marshal(msgNoError)
	if err != nil {
		t.Fatalf("Failed to marshal AssistantMessage: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(jsonDataNoError, &result); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	if _, exists := result["error"]; exists {
		t.Error("Expected 'error' field to be omitted when nil")
	}
}

// TestUserMessage_ToolUseResult tests ToolUseResult accessor methods
func TestUserMessage_ToolUseResult(t *testing.T) {
	tests := []struct {
		name          string
		toolUseResult map[string]any
		wantHas       bool
		wantNil       bool
	}{
		{
			name:          "nil_returns_false_and_nil",
			toolUseResult: nil,
			wantHas:       false,
			wantNil:       true,
		},
		{
			name:          "empty_map_returns_false_and_empty",
			toolUseResult: map[string]any{},
			wantHas:       false,
			wantNil:       false,
		},
		{
			name: "populated_returns_true_and_data",
			toolUseResult: map[string]any{
				"filePath":  "/path/to/file.py",
				"oldString": "old",
				"newString": "new",
			},
			wantHas: true,
			wantNil: false,
		},
		{
			name: "nested_structure_preserved",
			toolUseResult: map[string]any{
				"filePath": "/test.go",
				"structuredPatch": []any{
					map[string]any{"oldStart": float64(1), "lines": []any{"-old", "+new"}},
				},
			},
			wantHas: true,
			wantNil: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			um := &UserMessage{
				Content:       "test",
				ToolUseResult: tc.toolUseResult,
			}

			gotHas := um.HasToolUseResult()
			if gotHas != tc.wantHas {
				t.Errorf("HasToolUseResult() = %v, want %v", gotHas, tc.wantHas)
			}

			gotResult := um.GetToolUseResult()
			if (gotResult == nil) != tc.wantNil {
				t.Errorf("GetToolUseResult() nil = %v, want nil = %v", gotResult == nil, tc.wantNil)
			}

			// Verify data integrity for non-nil cases (skip nested structures)
			if !tc.wantNil && tc.toolUseResult != nil {
				if filePath, ok := tc.toolUseResult["filePath"].(string); ok {
					if gotResult["filePath"] != filePath {
						t.Errorf("GetToolUseResult()[\"filePath\"] = %v, want %v", gotResult["filePath"], filePath)
					}
				}
			}
		})
	}
}
