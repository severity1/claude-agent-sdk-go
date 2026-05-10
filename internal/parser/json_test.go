package parser

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/severity1/claude-agent-sdk-go/internal/shared"
)

// Test constants to avoid goconst warnings
const testResultAnswer42 = "The answer is 42"
const validSystemStatusJSON = `{"type": "system", "subtype": "status"}`

// TestParseValidMessages tests parsing of valid message types
func TestParseValidMessages(t *testing.T) { //nolint:gocyclo
	tests := []struct {
		name         string
		data         map[string]any
		expectedType string
		validate     func(*testing.T, shared.Message)
	}{
		{
			name: "user_message_string_content",
			data: map[string]any{
				"type":    "user",
				"message": map[string]any{"content": "Hello world"},
			},
			expectedType: shared.MessageTypeUser,
		},
		{
			name: "user_message_block_content",
			data: map[string]any{
				"type": "user",
				"message": map[string]any{
					"content": []any{
						map[string]any{"type": "text", "text": "Hello"},
						map[string]any{"type": "tool_use", "id": "t1", "name": "Read"},
					},
				},
			},
			expectedType: shared.MessageTypeUser,
		},
		// Issue #24: UUID and ParentToolUseID field tests
		{
			name: "user_message_with_uuid",
			data: map[string]any{
				"type":    "user",
				"uuid":    "msg-123-abc",
				"message": map[string]any{"content": "Hello"},
			},
			expectedType: shared.MessageTypeUser,
			validate: func(t *testing.T, msg shared.Message) {
				t.Helper()
				um := msg.(*shared.UserMessage)
				if um.UUID == nil || *um.UUID != "msg-123-abc" {
					t.Errorf("expected UUID 'msg-123-abc', got %v", um.UUID)
				}
			},
		},
		{
			name: "user_message_with_parent_tool_use_id",
			data: map[string]any{
				"type":               "user",
				"parent_tool_use_id": "tool-456",
				"message":            map[string]any{"content": "Tool response"},
			},
			expectedType: shared.MessageTypeUser,
			validate: func(t *testing.T, msg shared.Message) {
				t.Helper()
				um := msg.(*shared.UserMessage)
				if um.ParentToolUseID == nil || *um.ParentToolUseID != "tool-456" {
					t.Errorf("expected ParentToolUseID 'tool-456', got %v", um.ParentToolUseID)
				}
			},
		},
		{
			name: "user_message_with_uuid_and_parent_tool_use_id",
			data: map[string]any{
				"type":               "user",
				"uuid":               "msg-789",
				"parent_tool_use_id": "tool-012",
				"message":            map[string]any{"content": "Both fields"},
			},
			expectedType: shared.MessageTypeUser,
			validate: func(t *testing.T, msg shared.Message) {
				t.Helper()
				um := msg.(*shared.UserMessage)
				if um.UUID == nil || *um.UUID != "msg-789" {
					t.Errorf("expected UUID 'msg-789', got %v", um.UUID)
				}
				if um.ParentToolUseID == nil || *um.ParentToolUseID != "tool-012" {
					t.Errorf("expected ParentToolUseID 'tool-012', got %v", um.ParentToolUseID)
				}
			},
		},
		{
			name: "user_message_without_optional_fields",
			data: map[string]any{
				"type":    "user",
				"message": map[string]any{"content": "No optional fields"},
			},
			expectedType: shared.MessageTypeUser,
			validate: func(t *testing.T, msg shared.Message) {
				t.Helper()
				um := msg.(*shared.UserMessage)
				if um.UUID != nil {
					t.Errorf("expected UUID nil, got %v", um.UUID)
				}
				if um.ParentToolUseID != nil {
					t.Errorf("expected ParentToolUseID nil, got %v", um.ParentToolUseID)
				}
			},
		},
		{
			name: "assistant_message",
			data: map[string]any{
				"type": "assistant",
				"message": map[string]any{
					"content": []any{map[string]any{"type": "text", "text": "Hi"}},
					"model":   "claude-3-sonnet",
				},
			},
			expectedType: shared.MessageTypeAssistant,
			validate: func(t *testing.T, msg shared.Message) {
				t.Helper()
				am := msg.(*shared.AssistantMessage)
				if am.ParentToolUseID != nil {
					t.Errorf("expected ParentToolUseID nil, got %v", am.ParentToolUseID)
				}
			},
		},
		{
			name: "assistant_message_with_parent_tool_use_id",
			data: map[string]any{
				"type":               "assistant",
				"parent_tool_use_id": "tool-abc",
				"message": map[string]any{
					"content": []any{map[string]any{"type": "text", "text": "Subagent reply"}},
					"model":   "claude-3-sonnet",
				},
			},
			expectedType: shared.MessageTypeAssistant,
			validate: func(t *testing.T, msg shared.Message) {
				t.Helper()
				am := msg.(*shared.AssistantMessage)
				if am.ParentToolUseID == nil || *am.ParentToolUseID != "tool-abc" {
					t.Errorf("expected ParentToolUseID 'tool-abc', got %v", am.ParentToolUseID)
				}
				if am.GetParentToolUseID() != "tool-abc" {
					t.Errorf("expected GetParentToolUseID() 'tool-abc', got %q", am.GetParentToolUseID())
				}
			},
		},
		// Python PR #506: error field is at the top level of the event, not nested
		// inside data["message"]. CLI wire format: {"type":"assistant","error":"rate_limit","message":{...}}
		{
			name: "assistant_message_with_rate_limit_error",
			data: map[string]any{
				"type":  "assistant",
				"error": "rate_limit",
				"message": map[string]any{
					"content": []any{map[string]any{"type": "text", "text": "Rate limited"}},
					"model":   "claude-3-sonnet",
				},
			},
			expectedType: shared.MessageTypeAssistant,
			validate: func(t *testing.T, msg shared.Message) {
				t.Helper()
				am := msg.(*shared.AssistantMessage)
				if am.Error == nil {
					t.Fatal("expected Error to be set, got nil")
				}
				if *am.Error != shared.AssistantMessageErrorRateLimit {
					t.Errorf("expected Error 'rate_limit', got %v", *am.Error)
				}
				if !am.IsRateLimited() {
					t.Error("expected IsRateLimited() to return true")
				}
			},
		},
		{
			name: "assistant_message_with_auth_error",
			data: map[string]any{
				"type":  "assistant",
				"error": "authentication_failed",
				"message": map[string]any{
					"content": []any{map[string]any{"type": "text", "text": "Auth failed"}},
					"model":   "claude-3-sonnet",
				},
			},
			expectedType: shared.MessageTypeAssistant,
			validate: func(t *testing.T, msg shared.Message) {
				t.Helper()
				am := msg.(*shared.AssistantMessage)
				if am.Error == nil {
					t.Fatal("expected Error to be set, got nil")
				}
				if *am.Error != shared.AssistantMessageErrorAuthFailed {
					t.Errorf("expected Error 'authentication_failed', got %v", *am.Error)
				}
				if !am.HasError() {
					t.Error("expected HasError() to return true")
				}
				if am.IsRateLimited() {
					t.Error("expected IsRateLimited() to return false for auth error")
				}
			},
		},
		{
			name: "assistant_message_with_billing_error",
			data: map[string]any{
				"type":  "assistant",
				"error": "billing_error",
				"message": map[string]any{
					"content": []any{map[string]any{"type": "text", "text": "Billing error"}},
					"model":   "claude-3-sonnet",
				},
			},
			expectedType: shared.MessageTypeAssistant,
			validate: func(t *testing.T, msg shared.Message) {
				t.Helper()
				am := msg.(*shared.AssistantMessage)
				if am.Error == nil {
					t.Fatal("expected Error to be set, got nil")
				}
				if *am.Error != shared.AssistantMessageErrorBilling {
					t.Errorf("expected Error 'billing_error', got %v", *am.Error)
				}
				if !am.HasError() {
					t.Error("expected HasError() to return true")
				}
				if am.IsRateLimited() {
					t.Error("expected IsRateLimited() to return false for billing error")
				}
			},
		},
		{
			name: "assistant_message_with_server_error",
			data: map[string]any{
				"type":  "assistant",
				"error": "server_error",
				"message": map[string]any{
					"content": []any{map[string]any{"type": "text", "text": "Server error"}},
					"model":   "claude-3-sonnet",
				},
			},
			expectedType: shared.MessageTypeAssistant,
			validate: func(t *testing.T, msg shared.Message) {
				t.Helper()
				am := msg.(*shared.AssistantMessage)
				if am.Error == nil {
					t.Fatal("expected Error to be set, got nil")
				}
				if *am.Error != shared.AssistantMessageErrorServer {
					t.Errorf("expected Error 'server_error', got %v", *am.Error)
				}
				if !am.HasError() {
					t.Error("expected HasError() to return true")
				}
			},
		},
		{
			name: "assistant_message_with_invalid_request_error",
			data: map[string]any{
				"type":  "assistant",
				"error": "invalid_request",
				"message": map[string]any{
					"content": []any{map[string]any{"type": "text", "text": "Invalid request"}},
					"model":   "claude-3-sonnet",
				},
			},
			expectedType: shared.MessageTypeAssistant,
			validate: func(t *testing.T, msg shared.Message) {
				t.Helper()
				am := msg.(*shared.AssistantMessage)
				if am.Error == nil {
					t.Fatal("expected Error to be set, got nil")
				}
				if *am.Error != shared.AssistantMessageErrorInvalidRequest {
					t.Errorf("expected Error 'invalid_request', got %v", *am.Error)
				}
				if !am.HasError() {
					t.Error("expected HasError() to return true")
				}
			},
		},
		{
			name: "assistant_message_with_unknown_error",
			data: map[string]any{
				"type":  "assistant",
				"error": "unknown",
				"message": map[string]any{
					"content": []any{map[string]any{"type": "text", "text": "Unknown error"}},
					"model":   "claude-3-sonnet",
				},
			},
			expectedType: shared.MessageTypeAssistant,
			validate: func(t *testing.T, msg shared.Message) {
				t.Helper()
				am := msg.(*shared.AssistantMessage)
				if am.Error == nil {
					t.Fatal("expected Error to be set, got nil")
				}
				if *am.Error != shared.AssistantMessageErrorUnknown {
					t.Errorf("expected Error 'unknown', got %v", *am.Error)
				}
				if !am.HasError() {
					t.Error("expected HasError() to return true")
				}
			},
		},
		{
			name: "assistant_message_error_in_nested_message_ignored",
			// Regression: error inside data["message"] must NOT be picked up.
			// Only top-level data["error"] is the wire format.
			data: map[string]any{
				"type": "assistant",
				"message": map[string]any{
					"content": []any{map[string]any{"type": "text", "text": "ok"}},
					"model":   "claude-3-sonnet",
					"error":   "rate_limit",
				},
			},
			expectedType: shared.MessageTypeAssistant,
			validate: func(t *testing.T, msg shared.Message) {
				t.Helper()
				am := msg.(*shared.AssistantMessage)
				if am.Error != nil {
					t.Errorf("expected Error nil when error is nested in message object, got %v", *am.Error)
				}
			},
		},
		{
			name: "assistant_message_without_error",
			data: map[string]any{
				"type": "assistant",
				"message": map[string]any{
					"content": []any{map[string]any{"type": "text", "text": "Success"}},
					"model":   "claude-3-sonnet",
				},
			},
			expectedType: shared.MessageTypeAssistant,
			validate: func(t *testing.T, msg shared.Message) {
				t.Helper()
				am := msg.(*shared.AssistantMessage)
				if am.Error != nil {
					t.Errorf("expected Error to be nil, got %v", am.Error)
				}
				if am.HasError() {
					t.Error("expected HasError() to return false")
				}
			},
		},
		{
			name:         "system_message",
			data:         map[string]any{"type": "system", "subtype": "status"},
			expectedType: shared.MessageTypeSystem,
		},
		{
			name: "result_message",
			data: map[string]any{
				"type":            "result",
				"subtype":         "completed",
				"duration_ms":     1500.0,
				"duration_api_ms": 800.0,
				"is_error":        false,
				"num_turns":       2.0,
				"session_id":      "s123",
			},
			expectedType: shared.MessageTypeResult,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			parser := setupParserTest(t)
			message, err := parser.ParseMessage(test.data)
			assertParseSuccess(t, err, message)
			assertMessageType(t, message, test.expectedType)
			if test.validate != nil {
				test.validate(t, message)
			}
		})
	}
}

// Issue #98: TestParseUserMessageToolUseResult tests tool_use_result field parsing
// Python SDK v0.1.22 parity (PR #495)
func TestParseUserMessageToolUseResult(t *testing.T) {
	tests := []struct {
		name     string
		data     map[string]any
		validate func(*testing.T, *shared.UserMessage)
	}{
		{
			name: "array_content_with_full_tool_use_result",
			data: map[string]any{
				"type": "user",
				"message": map[string]any{
					"role": "user",
					"content": []any{
						map[string]any{
							"tool_use_id": "toolu_vrtx_01KXWexk3NJdwkjWzPMGQ2F1",
							"type":        "tool_result",
							"content":     "The file has been updated.",
						},
					},
				},
				"session_id": "84afb479-17ae-49af-8f2b-666ac2530c3a",
				"uuid":       "2ace3375-1879-48a0-a421-6bce25a9295a",
				"tool_use_result": map[string]any{
					"filePath":     "/path/to/file.py",
					"oldString":    "old code",
					"newString":    "new code",
					"originalFile": "full file contents",
					"structuredPatch": []any{
						map[string]any{
							"oldStart": float64(33),
							"oldLines": float64(7),
							"newStart": float64(33),
							"newLines": float64(7),
							"lines":    []any{"   # comment", "-      old line", "+      new line"},
						},
					},
					"userModified": false,
					"replaceAll":   false,
				},
			},
			validate: func(t *testing.T, um *shared.UserMessage) {
				t.Helper()
				if um.ToolUseResult == nil {
					t.Fatal("expected ToolUseResult to be set")
				}
				if um.ToolUseResult["filePath"] != "/path/to/file.py" {
					t.Errorf("expected filePath '/path/to/file.py', got %v", um.ToolUseResult["filePath"])
				}
				if um.ToolUseResult["oldString"] != "old code" {
					t.Errorf("expected oldString 'old code', got %v", um.ToolUseResult["oldString"])
				}
				if um.ToolUseResult["newString"] != "new code" {
					t.Errorf("expected newString 'new code', got %v", um.ToolUseResult["newString"])
				}
				// Verify nested structuredPatch preserved
				patch, ok := um.ToolUseResult["structuredPatch"].([]any)
				if !ok || len(patch) == 0 {
					t.Fatal("expected structuredPatch array")
				}
				patchItem, ok := patch[0].(map[string]any)
				if !ok {
					t.Fatal("expected structuredPatch[0] to be map")
				}
				if patchItem["oldStart"] != float64(33) {
					t.Errorf("expected oldStart 33, got %v", patchItem["oldStart"])
				}
				if um.UUID == nil || *um.UUID != "2ace3375-1879-48a0-a421-6bce25a9295a" {
					t.Errorf("expected UUID '2ace3375-1879-48a0-a421-6bce25a9295a'")
				}
			},
		},
		{
			name: "string_content_with_tool_use_result",
			data: map[string]any{
				"type":    "user",
				"message": map[string]any{"content": "Simple string content"},
				"tool_use_result": map[string]any{
					"filePath":     "/path/to/file.py",
					"userModified": true,
				},
			},
			validate: func(t *testing.T, um *shared.UserMessage) {
				t.Helper()
				if um.Content != "Simple string content" {
					t.Errorf("expected string content 'Simple string content', got %v", um.Content)
				}
				if um.ToolUseResult == nil {
					t.Fatal("expected ToolUseResult to be set")
				}
				if um.ToolUseResult["filePath"] != "/path/to/file.py" {
					t.Errorf("expected filePath '/path/to/file.py'")
				}
				if um.ToolUseResult["userModified"] != true {
					t.Errorf("expected userModified true")
				}
			},
		},
		{
			name: "without_tool_use_result_backward_compat",
			data: map[string]any{
				"type":    "user",
				"message": map[string]any{"content": "No tool use result"},
			},
			validate: func(t *testing.T, um *shared.UserMessage) {
				t.Helper()
				if um.ToolUseResult != nil {
					t.Errorf("expected ToolUseResult nil, got %v", um.ToolUseResult)
				}
				if um.HasToolUseResult() {
					t.Error("expected HasToolUseResult() to return false")
				}
			},
		},
	}

	parser := New()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			msg, err := parser.ParseMessage(tc.data)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			um, ok := msg.(*shared.UserMessage)
			if !ok {
				t.Fatalf("expected *UserMessage, got %T", msg)
			}
			tc.validate(t, um)
		})
	}
}

// TestParseErrors tests various error conditions
func TestParseErrors(t *testing.T) {
	tests := []struct {
		name        string
		data        map[string]any
		expectError string
	}{
		{
			name:        "missing_type_field",
			data:        map[string]any{"message": map[string]any{"content": "test"}},
			expectError: "missing or invalid type field",
		},
		{
			name:        "unknown_message_type",
			data:        map[string]any{"type": "unknown_type", "content": "test"},
			expectError: "unknown message type: unknown_type",
		},
		{
			name:        "user_message_missing_message_field",
			data:        map[string]any{"type": "user"},
			expectError: "user message missing message field",
		},
		{
			name: "user_message_missing_content_field",
			data: map[string]any{
				"type":    "user",
				"message": map[string]any{},
			},
			expectError: "user message missing content field",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			parser := setupParserTest(t)

			_, err := parser.ParseMessage(test.data)
			assertParseError(t, err, test.expectError)
		})
	}
}

// TestSpeculativeJSONParsing tests incomplete JSON handling
func TestSpeculativeJSONParsing(t *testing.T) {
	parser := setupParserTest(t)

	// Send incomplete JSON
	msg1, err1 := parser.processJSONLine(`{"type": "user", "message":`)
	assertNoParseError(t, err1)
	assertNoMessage(t, msg1)
	assertBufferNotEmpty(t, parser)

	// Complete the JSON
	msg2, err2 := parser.processJSONLine(` {"content": [{"type": "text", "text": "Hello"}]}}`)
	assertNoParseError(t, err2)
	assertMessageExists(t, msg2)
	assertBufferEmpty(t, parser)

	// Verify the parsed message
	userMsg, ok := msg2.(*shared.UserMessage)
	if !ok {
		t.Fatalf("Expected UserMessage, got %T", msg2)
	}

	blocks, ok := userMsg.Content.([]shared.ContentBlock)
	if !ok {
		t.Fatalf("Expected Content to be []ContentBlock, got %T", userMsg.Content)
	}
	assertContentBlockCount(t, blocks, 1)
	assertTextBlockContent(t, blocks[0], "Hello")
}

// TestBufferManagement tests buffer overflow protection and management
func TestBufferManagement(t *testing.T) {
	t.Run("buffer_overflow_protection", func(t *testing.T) {
		parser := setupParserTest(t)

		// Create a string larger than MaxBufferSize (1MB)
		largeString := strings.Repeat("x", MaxBufferSize+1000)

		_, err := parser.processJSONLine(largeString)
		assertBufferOverflowError(t, err)
		assertBufferEmpty(t, parser)
	})

	t.Run("buffer_reset_on_success", func(t *testing.T) {
		parser := setupParserTest(t)

		validJSON := validSystemStatusJSON
		msg, err := parser.processJSONLine(validJSON)

		assertNoParseError(t, err)
		assertMessageExists(t, msg)
		assertBufferEmpty(t, parser)
	})

	t.Run("partial_message_accumulation", func(t *testing.T) {
		parser := setupParserTest(t)

		parts := []string{
			`{"type": "user",`,
			` "message": {"content":`,
			` [{"type": "text",`,
			` "text": "Complete"}]}}`,
		}

		var finalMessage shared.Message
		for i, part := range parts {
			msg, err := parser.processJSONLine(part)
			assertNoParseError(t, err)

			if i < len(parts)-1 {
				assertNoMessage(t, msg)
				assertBufferNotEmpty(t, parser)
			} else {
				assertMessageExists(t, msg)
				assertBufferEmpty(t, parser)
				finalMessage = msg
			}
		}

		// Verify final message
		userMsg, ok := finalMessage.(*shared.UserMessage)
		if !ok {
			t.Fatalf("Expected UserMessage, got %T", finalMessage)
		}
		blocks, ok := userMsg.Content.([]shared.ContentBlock)
		if !ok {
			t.Fatalf("Expected Content to be []ContentBlock, got %T", userMsg.Content)
		}
		assertTextBlockContent(t, blocks[0], "Complete")
	})

	t.Run("explicit_buffer_reset", func(t *testing.T) {
		parser := setupParserTest(t)

		// Add content to buffer via partial JSON
		msg1, err1 := parser.processJSONLine(`{"type": "user", "message":`)
		assertNoParseError(t, err1)
		assertNoMessage(t, msg1)
		assertBufferNotEmpty(t, parser)

		// Explicit reset should clear buffer
		parser.Reset()
		assertBufferEmpty(t, parser)

		// Parser should work normally after reset
		validJSON := validSystemStatusJSON
		msg2, err2 := parser.processJSONLine(validJSON)
		assertNoParseError(t, err2)
		assertMessageExists(t, msg2)
		assertBufferEmpty(t, parser)
	})
}

// TestMultipleJSONObjects tests handling of multiple JSON objects
func TestMultipleJSONObjects(t *testing.T) {
	parser := setupParserTest(t)

	obj1 := `{"type": "user", "message": {"content": [{"type": "text", "text": "First"}]}}`
	obj2 := `{"type": "system", "subtype": "status", "message": "ok"}`
	line := obj1 + "\n" + obj2

	messages, err := parser.ProcessLine(line)
	assertNoParseError(t, err)
	assertMessageCount(t, messages, 2)

	// Verify first message
	userMsg, ok := messages[0].(*shared.UserMessage)
	if !ok {
		t.Fatalf("Expected UserMessage, got %T", messages[0])
	}
	blocks, ok := userMsg.Content.([]shared.ContentBlock)
	if !ok {
		t.Fatalf("Expected Content to be []ContentBlock, got %T", userMsg.Content)
	}
	assertTextBlockContent(t, blocks[0], "First")

	// Verify second message
	systemMsg, ok := messages[1].(*shared.SystemMessage)
	if !ok {
		t.Fatalf("Expected SystemMessage, got %T", messages[1])
	}
	if systemMsg.Subtype != "status" {
		t.Errorf("Expected subtype 'status', got %q", systemMsg.Subtype)
	}
}

// TestUnicodeAndEscapeHandling tests Unicode and JSON escape sequences
func TestUnicodeAndEscapeHandling(t *testing.T) {
	parser := setupParserTest(t)

	jsonString := `{"type": "user", "message": {"content": [{"type": "text", "text": "Hello 🌍\nEscaped\"Quote"}]}}`
	messages, err := parser.ProcessLine(jsonString)
	assertNoParseError(t, err)
	assertMessageCount(t, messages, 1)

	userMsg := messages[0].(*shared.UserMessage)
	blocks := userMsg.Content.([]shared.ContentBlock)
	assertTextBlockContent(t, blocks[0], "Hello 🌍\nEscaped\"Quote")
}

// TestConcurrentAccess tests thread safety
func TestConcurrentAccess(t *testing.T) {
	parser := setupParserTest(t)
	const numGoroutines = 5
	const messagesPerGoroutine = 10

	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines*messagesPerGoroutine)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()

			for j := 0; j < messagesPerGoroutine; j++ {
				testJSON := fmt.Sprintf(`{"type": "system", "subtype": "goroutine_%d_msg_%d"}`, goroutineID, j)

				msg, err := parser.processJSONLine(testJSON)
				if err != nil {
					errors <- fmt.Errorf("goroutine %d, message %d: %v", goroutineID, j, err)
					return
				}
				if msg == nil {
					errors <- fmt.Errorf("goroutine %d, message %d: expected message", goroutineID, j)
					return
				}

				systemMsg, ok := msg.(*shared.SystemMessage)
				if !ok {
					errors <- fmt.Errorf("goroutine %d, message %d: wrong type %T", goroutineID, j, msg)
					return
				}

				expectedSubtype := fmt.Sprintf("goroutine_%d_msg_%d", goroutineID, j)
				if systemMsg.Subtype != expectedSubtype {
					errors <- fmt.Errorf("goroutine %d, message %d: expected %s, got %s",
						goroutineID, j, expectedSubtype, systemMsg.Subtype)
					return
				}
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Error(err)
	}
}

// TestLargeMessageHandling tests handling of large messages
func TestLargeMessageHandling(t *testing.T) {
	parser := setupParserTest(t)

	// Test large message under limit (950KB)
	largeContent := strings.Repeat("X", 950*1024)
	largeJSON := fmt.Sprintf(`{"type": "user", "message": {"content": [{"type": "text", "text": %q}]}}`, largeContent)

	if len(largeJSON) >= MaxBufferSize {
		t.Fatalf("Test setup error: large JSON exceeds MaxBufferSize")
	}

	msg, err := parser.processJSONLine(largeJSON)
	assertNoParseError(t, err)
	assertMessageExists(t, msg)

	userMsg, ok := msg.(*shared.UserMessage)
	if !ok {
		t.Fatalf("Expected UserMessage, got %T", msg)
	}
	blocks, ok := userMsg.Content.([]shared.ContentBlock)
	if !ok {
		t.Fatalf("Expected Content to be []ContentBlock, got %T", userMsg.Content)
	}
	textBlock, ok := blocks[0].(*shared.TextBlock)
	if !ok {
		t.Fatalf("Expected TextBlock, got %T", blocks[0])
	}

	if len(textBlock.Text) != len(largeContent) {
		t.Errorf("Expected text length %d, got %d", len(largeContent), len(textBlock.Text))
	}

	assertBufferEmpty(t, parser)
}

// TestEmptyAndWhitespaceHandling tests handling of empty lines
func TestEmptyAndWhitespaceHandling(t *testing.T) {
	parser := setupParserTest(t)

	emptyInputs := []string{"", "   ", "\t\n"}
	for _, input := range emptyInputs {
		messages, err := parser.ProcessLine(input)
		assertNoParseError(t, err)
		assertMessageCount(t, messages, 0)
	}
}

// TestParseMessages tests the convenience function
func TestParseMessages(t *testing.T) {
	// Test successful parsing
	lines := []string{
		`{"type": "user", "message": {"content": "Hello"}}`,
		validSystemStatusJSON,
	}

	messages, err := ParseMessages(lines)
	assertNoParseError(t, err)
	assertMessageCount(t, messages, 2)

	// Test error handling
	errorLines := []string{
		`{"type": "user", "message": {"content": "Valid"}}`,
		`{"type": "invalid"}`, // This should cause an error
	}

	_, err = ParseMessages(errorLines)
	if err == nil {
		t.Error("Expected error for invalid message type")
	}
	if !strings.Contains(err.Error(), "error parsing line 1") {
		t.Errorf("Expected line number in error, got: %v", err)
	}
}

// TestParseErrorConditions tests comprehensive error scenarios
func TestParseErrorConditions(t *testing.T) {
	parser := setupParserTest(t)

	tests := []struct {
		name        string
		data        map[string]any
		expectError string
	}{
		{
			name: "user_message_invalid_content_type",
			data: map[string]any{
				"type":    "user",
				"message": map[string]any{"content": 123}, // Invalid type
			},
			expectError: "invalid user message content type",
		},
		{
			name: "user_message_content_block_parse_error",
			data: map[string]any{
				"type": "user",
				"message": map[string]any{
					"content": []any{
						map[string]any{"type": "text"}, // Missing text field
					},
				},
			},
			expectError: "failed to parse content block 0",
		},
		{
			name:        "assistant_message_missing_message",
			data:        map[string]any{"type": "assistant"},
			expectError: "assistant message missing message field",
		},
		{
			name: "assistant_message_content_not_array",
			data: map[string]any{
				"type": "assistant",
				"message": map[string]any{
					"content": "not an array",
					"model":   "claude-3",
				},
			},
			expectError: "assistant message content must be array",
		},
		{
			name: "assistant_message_missing_model",
			data: map[string]any{
				"type": "assistant",
				"message": map[string]any{
					"content": []any{},
				},
			},
			expectError: "assistant message missing model field",
		},
		{
			name: "assistant_message_content_block_error",
			data: map[string]any{
				"type": "assistant",
				"message": map[string]any{
					"content": []any{
						map[string]any{"type": "unknown_block"},
					},
					"model": "claude-3",
				},
			},
			expectError: "failed to parse content block 0",
		},
		{
			name:        "system_message_missing_subtype",
			data:        map[string]any{"type": "system"},
			expectError: "system message missing subtype field",
		},
		{
			name:        "system_message_invalid_subtype",
			data:        map[string]any{"type": "system", "subtype": 123},
			expectError: "system message missing subtype field",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parser.ParseMessage(test.data)
			assertParseError(t, err, test.expectError)
		})
	}
}

// TestResultMessageErrorConditions tests uncovered result message parsing paths
func TestResultMessageErrorConditions(t *testing.T) {
	parser := setupParserTest(t)

	tests := []struct {
		name        string
		data        map[string]any
		expectError string
	}{
		{
			name:        "missing_subtype",
			data:        map[string]any{"type": "result"},
			expectError: "result message missing subtype field",
		},
		{
			name:        "invalid_subtype_type",
			data:        map[string]any{"type": "result", "subtype": 123},
			expectError: "result message missing subtype field",
		},
		{
			name: "missing_duration_ms",
			data: map[string]any{
				"type":    "result",
				"subtype": "test",
			},
			expectError: "result message missing or invalid duration_ms field",
		},
		{
			name: "invalid_duration_ms_type",
			data: map[string]any{
				"type":        "result",
				"subtype":     "test",
				"duration_ms": "not a number",
			},
			expectError: "result message missing or invalid duration_ms field",
		},
		{
			name: "missing_duration_api_ms",
			data: map[string]any{
				"type":        "result",
				"subtype":     "test",
				"duration_ms": 100.0,
			},
			expectError: "result message missing or invalid duration_api_ms field",
		},
		{
			name: "invalid_is_error_type",
			data: map[string]any{
				"type":            "result",
				"subtype":         "test",
				"duration_ms":     100.0,
				"duration_api_ms": 50.0,
				"is_error":        "not a boolean",
			},
			expectError: "result message missing or invalid is_error field",
		},
		{
			name: "invalid_num_turns_type",
			data: map[string]any{
				"type":            "result",
				"subtype":         "test",
				"duration_ms":     100.0,
				"duration_api_ms": 50.0,
				"is_error":        false,
				"num_turns":       "not a number",
			},
			expectError: "result message missing or invalid num_turns field",
		},
		{
			name: "missing_session_id",
			data: map[string]any{
				"type":            "result",
				"subtype":         "test",
				"duration_ms":     100.0,
				"duration_api_ms": 50.0,
				"is_error":        false,
				"num_turns":       1.0,
			},
			expectError: "result message missing session_id field",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parser.ParseMessage(test.data)
			assertParseError(t, err, test.expectError)
		})
	}
}

// TestResultMessageOptionalFields tests optional field handling
func TestResultMessageOptionalFields(t *testing.T) {
	parser := setupParserTest(t)

	baseData := map[string]any{
		"type":            "result",
		"subtype":         "test",
		"duration_ms":     100.0,
		"duration_api_ms": 50.0,
		"is_error":        false,
		"num_turns":       1.0,
		"session_id":      "s123",
	}

	// Test with all optional fields
	dataWithOptionals := make(map[string]any)
	for k, v := range baseData {
		dataWithOptionals[k] = v
	}
	dataWithOptionals["total_cost_usd"] = 0.05
	dataWithOptionals["usage"] = map[string]any{"input_tokens": 100}
	dataWithOptionals["result"] = testResultAnswer42

	msg, err := parser.ParseMessage(dataWithOptionals)
	assertNoParseError(t, err)

	resultMsg := msg.(*shared.ResultMessage)
	if resultMsg.TotalCostUSD == nil || *resultMsg.TotalCostUSD != 0.05 {
		t.Errorf("Expected total_cost_usd = 0.05, got %v", resultMsg.TotalCostUSD)
	}
	if resultMsg.Usage == nil {
		t.Error("Expected usage field to be set")
	}
	if resultMsg.Result == nil {
		t.Error("Expected result field to be set")
	}
	if *resultMsg.Result != testResultAnswer42 {
		t.Errorf("Expected result = %q, got %v", testResultAnswer42, *resultMsg.Result)
	}

	// Test with invalid result type (not string)
	dataWithInvalidResult := make(map[string]any)
	for k, v := range baseData {
		dataWithInvalidResult[k] = v
	}
	dataWithInvalidResult["result"] = map[string]any{"not": "a string"}

	msg2, err2 := parser.ParseMessage(dataWithInvalidResult)
	assertNoParseError(t, err2)
	resultMsg2 := msg2.(*shared.ResultMessage)
	if resultMsg2.Result != nil {
		t.Error("Expected result field to be nil for invalid type")
	}
}

// TestContentBlockErrorConditions tests uncovered content block parsing paths
func TestContentBlockErrorConditions(t *testing.T) {
	parser := setupParserTest(t)

	tests := []struct {
		name        string
		blockData   any
		expectError string
	}{
		{
			name:        "non_object_block",
			blockData:   "not an object",
			expectError: "content block must be an object",
		},
		{
			name:        "missing_type_field",
			blockData:   map[string]any{"text": "hello"},
			expectError: "content block missing type field",
		},
		{
			name:        "invalid_type_field",
			blockData:   map[string]any{"type": 123},
			expectError: "content block missing type field",
		},
		{
			name:        "unknown_block_type",
			blockData:   map[string]any{"type": "unknown_type"},
			expectError: "unknown content block type: unknown_type",
		},
		{
			name:        "text_block_missing_text",
			blockData:   map[string]any{"type": "text"},
			expectError: "text block missing text field",
		},
		{
			name:        "text_block_invalid_text_type",
			blockData:   map[string]any{"type": "text", "text": 123},
			expectError: "text block missing text field",
		},
		{
			name:        "thinking_block_missing_thinking",
			blockData:   map[string]any{"type": "thinking"},
			expectError: "thinking block missing thinking field",
		},
		{
			name:        "thinking_block_invalid_thinking_type",
			blockData:   map[string]any{"type": "thinking", "thinking": 123},
			expectError: "thinking block missing thinking field",
		},
		{
			name:        "tool_use_block_missing_id",
			blockData:   map[string]any{"type": "tool_use", "name": "Read"},
			expectError: "tool_use block missing id field",
		},
		{
			name:        "tool_use_block_invalid_id_type",
			blockData:   map[string]any{"type": "tool_use", "id": 123, "name": "Read"},
			expectError: "tool_use block missing id field",
		},
		{
			name:        "tool_use_block_missing_name",
			blockData:   map[string]any{"type": "tool_use", "id": "t1"},
			expectError: "tool_use block missing name field",
		},
		{
			name:        "tool_use_block_invalid_name_type",
			blockData:   map[string]any{"type": "tool_use", "id": "t1", "name": 123},
			expectError: "tool_use block missing name field",
		},
		{
			name:        "tool_result_block_missing_tool_use_id",
			blockData:   map[string]any{"type": "tool_result", "content": "result"},
			expectError: "tool_result block missing tool_use_id field",
		},
		{
			name:        "tool_result_block_invalid_tool_use_id_type",
			blockData:   map[string]any{"type": "tool_result", "tool_use_id": 123, "content": "result"},
			expectError: "tool_result block missing tool_use_id field",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parser.parseContentBlock(test.blockData)
			assertParseError(t, err, test.expectError)
		})
	}
}

// TestContentBlockOptionalFields tests optional field handling
func TestContentBlockOptionalFields(t *testing.T) {
	parser := setupParserTest(t)

	// Test thinking block without signature
	thinkingBlock, err := parser.parseContentBlock(map[string]any{
		"type":     "thinking",
		"thinking": "I need to think...",
	})
	assertNoParseError(t, err)
	thinking := thinkingBlock.(*shared.ThinkingBlock)
	if thinking.Signature != "" {
		t.Errorf("Expected empty signature, got %q", thinking.Signature)
	}

	// Test tool use block without input
	toolUseBlock, err := parser.parseContentBlock(map[string]any{
		"type": "tool_use",
		"id":   "t1",
		"name": "Read",
	})
	assertNoParseError(t, err)
	toolUse := toolUseBlock.(*shared.ToolUseBlock)
	if toolUse.Input == nil {
		t.Error("Expected empty input map, got nil")
	}
	if len(toolUse.Input) != 0 {
		t.Errorf("Expected empty input map, got %v", toolUse.Input)
	}

	// Test tool result block with invalid is_error type
	toolResultBlock, err := parser.parseContentBlock(map[string]any{
		"type":        "tool_result",
		"tool_use_id": "t1",
		"content":     "result",
		"is_error":    "not a boolean",
	})
	assertNoParseError(t, err)
	toolResult := toolResultBlock.(*shared.ToolResultBlock)
	if toolResult.IsError != nil {
		t.Errorf("Expected nil IsError for invalid type, got %v", toolResult.IsError)
	}
}

// TestProcessLineEdgeCases tests uncovered ProcessLine scenarios
func TestProcessLineEdgeCases(t *testing.T) {
	parser := setupParserTest(t)

	// Test line with content block parse error
	invalidBlockLine := `{"type": "user", "message": {"content": [{"type": "unknown_block"}]}}`
	messages, err := parser.ProcessLine(invalidBlockLine)
	if err == nil {
		t.Error("Expected error for invalid content block")
	}
	if len(messages) != 0 {
		t.Errorf("Expected no messages on error, got %d", len(messages))
	}

	// Test multiple lines with one having an error
	mixedLine := `{"type": "system", "subtype": "ok"}` + "\n" + `{"type": "invalid"}`
	messages2, err2 := parser.ProcessLine(mixedLine)
	if err2 == nil {
		t.Error("Expected error for second invalid message")
	}
	// Should return the first valid message before error
	if len(messages2) != 1 {
		t.Errorf("Expected 1 message before error, got %d", len(messages2))
	}
}

// Mock and Helper Functions

// setupParserTest creates a new parser for testing
func setupParserTest(t *testing.T) *Parser {
	t.Helper()
	return New()
}

// Assertion helpers

func assertParseSuccess(t *testing.T, err error, result any) {
	t.Helper()
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if result == nil {
		t.Fatal("Expected parse result, got nil")
	}
}

func assertParseError(t *testing.T, err error, expectedMsg string) {
	t.Helper()
	if err == nil {
		t.Fatal("Expected parse error, got nil")
	}
	if !strings.Contains(err.Error(), expectedMsg) {
		t.Errorf("Expected error containing %q, got %q", expectedMsg, err.Error())
	}
}

func assertNoParseError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("Unexpected parse error: %v", err)
	}
}

func assertMessageType(t *testing.T, msg shared.Message, expectedType string) {
	t.Helper()
	if msg.Type() != expectedType {
		t.Errorf("Expected message type %s, got %s", expectedType, msg.Type())
	}
}

func assertMessageExists(t *testing.T, msg shared.Message) {
	t.Helper()
	if msg == nil {
		t.Fatal("Expected message, got nil")
	}
}

func assertNoMessage(t *testing.T, msg shared.Message) {
	t.Helper()
	if msg != nil {
		t.Fatalf("Expected no message, got %T", msg)
	}
}

func assertMessageCount(t *testing.T, messages []shared.Message, expected int) {
	t.Helper()
	if len(messages) != expected {
		t.Errorf("Expected %d messages, got %d", expected, len(messages))
	}
}

func assertContentBlockCount(t *testing.T, blocks []shared.ContentBlock, expected int) {
	t.Helper()
	if len(blocks) != expected {
		t.Errorf("Expected %d content blocks, got %d", expected, len(blocks))
	}
}

func assertTextBlockContent(t *testing.T, block shared.ContentBlock, expectedText string) {
	t.Helper()
	textBlock, ok := block.(*shared.TextBlock)
	if !ok {
		t.Fatalf("Expected TextBlock, got %T", block)
	}
	if textBlock.Text != expectedText {
		t.Errorf("Expected text %q, got %q", expectedText, textBlock.Text)
	}
}

func assertBufferEmpty(t *testing.T, parser *Parser) {
	t.Helper()
	if parser.BufferSize() != 0 {
		t.Errorf("Expected empty buffer, got size %d", parser.BufferSize())
	}
}

func assertBufferNotEmpty(t *testing.T, parser *Parser) {
	t.Helper()
	if parser.BufferSize() == 0 {
		t.Error("Expected non-empty buffer, got empty")
	}
}

func assertBufferOverflowError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("Expected buffer overflow error, got nil")
	}
	jsonDecodeErr := shared.AsJSONDecodeError(err)
	if jsonDecodeErr == nil {
		t.Fatalf("Expected JSONDecodeError, got %T", err)
		return
	}
	if !strings.Contains(jsonDecodeErr.Error(), "buffer overflow") {
		t.Errorf("Expected buffer overflow error, got %q", jsonDecodeErr.Error())
	}
}

// TestResultMessageStructuredOutput tests structured_output field parsing in ResultMessage
func TestResultMessageStructuredOutput(t *testing.T) {
	parser := setupParserTest(t)

	baseData := func() map[string]any {
		return map[string]any{
			"type":            "result",
			"subtype":         "completed",
			"duration_ms":     1000.0,
			"duration_api_ms": 500.0,
			"is_error":        false,
			"num_turns":       1.0,
			"session_id":      "s123",
		}
	}

	tests := []struct {
		name     string
		getData  func() map[string]any
		validate func(*testing.T, *shared.ResultMessage)
	}{
		{
			name: "structured_output_object",
			getData: func() map[string]any {
				data := baseData()
				data["structured_output"] = map[string]any{
					"name":  "test",
					"count": 42.0,
				}
				return data
			},
			validate: func(t *testing.T, msg *shared.ResultMessage) {
				t.Helper()
				if msg.StructuredOutput == nil {
					t.Fatal("Expected StructuredOutput to be set")
				}
				output, ok := msg.StructuredOutput.(map[string]any)
				if !ok {
					t.Fatalf("Expected map[string]any, got %T", msg.StructuredOutput)
				}
				if output["name"] != "test" {
					t.Errorf("Expected name = 'test', got %v", output["name"])
				}
				if output["count"] != 42.0 {
					t.Errorf("Expected count = 42.0, got %v", output["count"])
				}
			},
		},
		{
			name: "structured_output_array",
			getData: func() map[string]any {
				data := baseData()
				data["structured_output"] = []any{"item1", "item2", "item3"}
				return data
			},
			validate: func(t *testing.T, msg *shared.ResultMessage) {
				t.Helper()
				if msg.StructuredOutput == nil {
					t.Fatal("Expected StructuredOutput to be set")
				}
				output, ok := msg.StructuredOutput.([]any)
				if !ok {
					t.Fatalf("Expected []any, got %T", msg.StructuredOutput)
				}
				if len(output) != 3 {
					t.Errorf("Expected 3 items, got %d", len(output))
				}
				if output[0] != "item1" {
					t.Errorf("Expected first item = 'item1', got %v", output[0])
				}
			},
		},
		{
			name: "structured_output_string",
			getData: func() map[string]any {
				data := baseData()
				data["structured_output"] = "simple string output"
				return data
			},
			validate: func(t *testing.T, msg *shared.ResultMessage) {
				t.Helper()
				if msg.StructuredOutput == nil {
					t.Fatal("Expected StructuredOutput to be set")
				}
				output, ok := msg.StructuredOutput.(string)
				if !ok {
					t.Fatalf("Expected string, got %T", msg.StructuredOutput)
				}
				if output != "simple string output" {
					t.Errorf("Expected 'simple string output', got %q", output)
				}
			},
		},
		{
			name: "structured_output_number",
			getData: func() map[string]any {
				data := baseData()
				data["structured_output"] = 42.5
				return data
			},
			validate: func(t *testing.T, msg *shared.ResultMessage) {
				t.Helper()
				if msg.StructuredOutput == nil {
					t.Fatal("Expected StructuredOutput to be set")
				}
				output, ok := msg.StructuredOutput.(float64)
				if !ok {
					t.Fatalf("Expected float64, got %T", msg.StructuredOutput)
				}
				if output != 42.5 {
					t.Errorf("Expected 42.5, got %v", output)
				}
			},
		},
		{
			name: "structured_output_boolean",
			getData: func() map[string]any {
				data := baseData()
				data["structured_output"] = true
				return data
			},
			validate: func(t *testing.T, msg *shared.ResultMessage) {
				t.Helper()
				if msg.StructuredOutput == nil {
					t.Fatal("Expected StructuredOutput to be set")
				}
				output, ok := msg.StructuredOutput.(bool)
				if !ok {
					t.Fatalf("Expected bool, got %T", msg.StructuredOutput)
				}
				if !output {
					t.Error("Expected true, got false")
				}
			},
		},
		{
			name: "structured_output_nil",
			getData: func() map[string]any {
				return baseData() // No structured_output field
			},
			validate: func(t *testing.T, msg *shared.ResultMessage) {
				t.Helper()
				if msg.StructuredOutput != nil {
					t.Errorf("Expected StructuredOutput = nil, got %v", msg.StructuredOutput)
				}
			},
		},
		{
			name: "structured_output_nested_object",
			getData: func() map[string]any {
				data := baseData()
				data["structured_output"] = map[string]any{
					"results": []any{
						map[string]any{"id": 1.0, "name": "first"},
						map[string]any{"id": 2.0, "name": "second"},
					},
					"metadata": map[string]any{
						"total": 2.0,
						"page":  1.0,
					},
				}
				return data
			},
			validate: func(t *testing.T, msg *shared.ResultMessage) {
				t.Helper()
				if msg.StructuredOutput == nil {
					t.Fatal("Expected StructuredOutput to be set")
				}
				output, ok := msg.StructuredOutput.(map[string]any)
				if !ok {
					t.Fatalf("Expected map[string]any, got %T", msg.StructuredOutput)
				}
				results, ok := output["results"].([]any)
				if !ok || len(results) != 2 {
					t.Errorf("Expected results array with 2 items, got %v", output["results"])
				}
				metadata, ok := output["metadata"].(map[string]any)
				if !ok {
					t.Errorf("Expected metadata map, got %v", output["metadata"])
				}
				if metadata["total"] != 2.0 {
					t.Errorf("Expected total = 2, got %v", metadata["total"])
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := parser.ParseMessage(tt.getData())
			assertNoParseError(t, err)

			resultMsg, ok := msg.(*shared.ResultMessage)
			if !ok {
				t.Fatalf("Expected ResultMessage, got %T", msg)
			}
			tt.validate(t, resultMsg)
		})
	}
}

// TestControlMessageParsing tests parsing of control protocol messages
func TestControlMessageParsing(t *testing.T) {
	parser := setupParserTest(t)

	tests := []struct {
		name         string
		data         map[string]any
		expectedType string
		validate     func(*testing.T, shared.Message)
	}{
		{
			name: "control_request_message",
			data: map[string]any{
				"type":       "control_request",
				"request_id": "req_1_abc123",
				"request": map[string]any{
					"subtype": "interrupt",
				},
			},
			expectedType: shared.MessageTypeControlRequest,
			validate: func(t *testing.T, msg shared.Message) {
				t.Helper()
				rawMsg, ok := msg.(*shared.RawControlMessage)
				if !ok {
					t.Fatalf("Expected RawControlMessage, got %T", msg)
				}
				if rawMsg.Data["request_id"] != "req_1_abc123" {
					t.Errorf("Expected request_id 'req_1_abc123', got %v", rawMsg.Data["request_id"])
				}
				request, ok := rawMsg.Data["request"].(map[string]any)
				if !ok {
					t.Fatal("Expected request to be a map")
				}
				if request["subtype"] != "interrupt" {
					t.Errorf("Expected subtype 'interrupt', got %v", request["subtype"])
				}
			},
		},
		{
			name: "control_response_success",
			data: map[string]any{
				"type": "control_response",
				"response": map[string]any{
					"subtype":    "success",
					"request_id": "req_2_def456",
					"response":   map[string]any{"status": "ok"},
				},
			},
			expectedType: shared.MessageTypeControlResponse,
			validate: func(t *testing.T, msg shared.Message) {
				t.Helper()
				rawMsg, ok := msg.(*shared.RawControlMessage)
				if !ok {
					t.Fatalf("Expected RawControlMessage, got %T", msg)
				}
				response, ok := rawMsg.Data["response"].(map[string]any)
				if !ok {
					t.Fatal("Expected response to be a map")
				}
				if response["subtype"] != "success" {
					t.Errorf("Expected subtype 'success', got %v", response["subtype"])
				}
				if response["request_id"] != "req_2_def456" {
					t.Errorf("Expected request_id 'req_2_def456', got %v", response["request_id"])
				}
			},
		},
		{
			name: "control_response_error",
			data: map[string]any{
				"type": "control_response",
				"response": map[string]any{
					"subtype":    "error",
					"request_id": "req_3_ghi789",
					"error":      "initialization failed",
				},
			},
			expectedType: shared.MessageTypeControlResponse,
			validate: func(t *testing.T, msg shared.Message) {
				t.Helper()
				rawMsg, ok := msg.(*shared.RawControlMessage)
				if !ok {
					t.Fatalf("Expected RawControlMessage, got %T", msg)
				}
				response, ok := rawMsg.Data["response"].(map[string]any)
				if !ok {
					t.Fatal("Expected response to be a map")
				}
				if response["error"] != "initialization failed" {
					t.Errorf("Expected error 'initialization failed', got %v", response["error"])
				}
			},
		},
		{
			name: "control_request_initialize",
			data: map[string]any{
				"type":       "control_request",
				"request_id": "req_4_jkl012",
				"request": map[string]any{
					"subtype": "initialize",
				},
			},
			expectedType: shared.MessageTypeControlRequest,
			validate: func(t *testing.T, msg shared.Message) {
				t.Helper()
				rawMsg, ok := msg.(*shared.RawControlMessage)
				if !ok {
					t.Fatalf("Expected RawControlMessage, got %T", msg)
				}
				request, ok := rawMsg.Data["request"].(map[string]any)
				if !ok {
					t.Fatal("Expected request to be a map")
				}
				if request["subtype"] != "initialize" {
					t.Errorf("Expected subtype 'initialize', got %v", request["subtype"])
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			message, err := parser.ParseMessage(test.data)
			assertParseSuccess(t, err, message)
			assertMessageType(t, message, test.expectedType)
			if test.validate != nil {
				test.validate(t, message)
			}
		})
	}
}

// TestControlMessageMixedWithRegular tests parsing control messages alongside regular messages
func TestControlMessageMixedWithRegular(t *testing.T) {
	parser := setupParserTest(t)

	// Test a line with a regular message followed by a control message
	obj1 := `{"type": "user", "message": {"content": "Hello"}}`
	obj2 := `{"type": "control_response", "response": {"subtype": "success", "request_id": "req_1", "response": {}}}`
	line := obj1 + "\n" + obj2

	messages, err := parser.ProcessLine(line)
	assertNoParseError(t, err)
	assertMessageCount(t, messages, 2)

	// First should be user message
	_, ok := messages[0].(*shared.UserMessage)
	if !ok {
		t.Fatalf("Expected UserMessage, got %T", messages[0])
	}

	// Second should be control message
	rawMsg, ok := messages[1].(*shared.RawControlMessage)
	if !ok {
		t.Fatalf("Expected RawControlMessage, got %T", messages[1])
	}
	if rawMsg.Type() != shared.MessageTypeControlResponse {
		t.Errorf("Expected control_response type, got %s", rawMsg.Type())
	}
}

// TestResultMessageStructuredOutputWithOtherFields tests structured_output works with other optional fields
func TestResultMessageStructuredOutputWithOtherFields(t *testing.T) {
	parser := setupParserTest(t)

	data := map[string]any{
		"type":            "result",
		"subtype":         "completed",
		"duration_ms":     1000.0,
		"duration_api_ms": 500.0,
		"is_error":        false,
		"num_turns":       1.0,
		"session_id":      "s123",
		"total_cost_usd":  0.05,
		"result":          testResultAnswer42,
		"usage":           map[string]any{"input_tokens": 100.0, "output_tokens": 50.0},
		"structured_output": map[string]any{
			"answer":     "42",
			"confidence": 0.95,
		},
	}

	msg, err := parser.ParseMessage(data)
	assertNoParseError(t, err)

	resultMsg := msg.(*shared.ResultMessage)

	// Verify all fields are set correctly
	if resultMsg.TotalCostUSD == nil || *resultMsg.TotalCostUSD != 0.05 {
		t.Errorf("Expected total_cost_usd = 0.05, got %v", resultMsg.TotalCostUSD)
	}
	if resultMsg.Result == nil || *resultMsg.Result != testResultAnswer42 {
		t.Errorf("Expected result = %q, got %v", testResultAnswer42, resultMsg.Result)
	}
	if resultMsg.Usage == nil {
		t.Error("Expected usage to be set")
	}
	if resultMsg.StructuredOutput == nil {
		t.Error("Expected structured_output to be set")
	}

	output := resultMsg.StructuredOutput.(map[string]any)
	if output["answer"] != "42" {
		t.Errorf("Expected answer = '42', got %v", output["answer"])
	}
	if output["confidence"] != 0.95 {
		t.Errorf("Expected confidence = 0.95, got %v", output["confidence"])
	}
}

// TestStreamEventMessageParsing tests parsing of stream_event messages
// Following established pattern from TestParseValidMessages
func TestStreamEventMessageParsing(t *testing.T) {
	tests := []struct {
		name         string
		data         map[string]any
		expectedType string
		validate     func(*testing.T, shared.Message)
	}{
		{
			name: "stream_event_all_fields",
			data: map[string]any{
				"type":               "stream_event",
				"uuid":               "evt-123",
				"session_id":         "sess-456",
				"event":              map[string]any{"type": "content_block_delta"},
				"parent_tool_use_id": "tool-789",
			},
			expectedType: shared.MessageTypeStreamEvent,
			validate: func(t *testing.T, msg shared.Message) {
				t.Helper()
				se := msg.(*shared.StreamEvent)
				if se.UUID != "evt-123" {
					t.Errorf("expected UUID 'evt-123', got %q", se.UUID)
				}
				if se.SessionID != "sess-456" {
					t.Errorf("expected SessionID 'sess-456', got %q", se.SessionID)
				}
				if se.ParentToolUseID == nil || *se.ParentToolUseID != "tool-789" {
					t.Errorf("expected ParentToolUseID 'tool-789', got %v", se.ParentToolUseID)
				}
				if se.Event == nil {
					t.Error("expected Event to be set")
				}
				if se.Event["type"] != "content_block_delta" {
					t.Errorf("expected Event type 'content_block_delta', got %v", se.Event["type"])
				}
			},
		},
		{
			name: "stream_event_without_parent_tool_use_id",
			data: map[string]any{
				"type":       "stream_event",
				"uuid":       "evt-abc",
				"session_id": "sess-def",
				"event":      map[string]any{"type": "message_start"},
			},
			expectedType: shared.MessageTypeStreamEvent,
			validate: func(t *testing.T, msg shared.Message) {
				t.Helper()
				se := msg.(*shared.StreamEvent)
				if se.ParentToolUseID != nil {
					t.Errorf("expected ParentToolUseID nil, got %v", se.ParentToolUseID)
				}
			},
		},
		{
			name: "stream_event_message_stop",
			data: map[string]any{
				"type":       "stream_event",
				"uuid":       "evt-stop",
				"session_id": "sess-ghi",
				"event":      map[string]any{"type": "message_stop"},
			},
			expectedType: shared.MessageTypeStreamEvent,
			validate: func(t *testing.T, msg shared.Message) {
				t.Helper()
				se := msg.(*shared.StreamEvent)
				if se.Event["type"] != "message_stop" {
					t.Errorf("expected Event type 'message_stop', got %v", se.Event["type"])
				}
			},
		},
		{
			name: "stream_event_content_block_start",
			data: map[string]any{
				"type":       "stream_event",
				"uuid":       "evt-cbs",
				"session_id": "sess-jkl",
				"event": map[string]any{
					"type":  "content_block_start",
					"index": 0.0,
				},
			},
			expectedType: shared.MessageTypeStreamEvent,
			validate: func(t *testing.T, msg shared.Message) {
				t.Helper()
				se := msg.(*shared.StreamEvent)
				if se.Event["type"] != "content_block_start" {
					t.Errorf("expected Event type 'content_block_start', got %v", se.Event["type"])
				}
				if se.Event["index"] != 0.0 {
					t.Errorf("expected Event index 0, got %v", se.Event["index"])
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			parser := setupParserTest(t)
			message, err := parser.ParseMessage(test.data)
			assertParseSuccess(t, err, message)
			assertMessageType(t, message, test.expectedType)
			if test.validate != nil {
				test.validate(t, message)
			}
		})
	}
}

// TestStreamEventErrorConditions tests error conditions for stream_event parsing
// Following established pattern from TestParseErrorConditions
func TestStreamEventErrorConditions(t *testing.T) {
	tests := []struct {
		name        string
		data        map[string]any
		expectError string
	}{
		{
			name:        "stream_event_missing_uuid",
			data:        map[string]any{"type": "stream_event", "session_id": "s1", "event": map[string]any{}},
			expectError: "stream_event missing uuid field",
		},
		{
			name:        "stream_event_missing_session_id",
			data:        map[string]any{"type": "stream_event", "uuid": "u1", "event": map[string]any{}},
			expectError: "stream_event missing session_id field",
		},
		{
			name:        "stream_event_missing_event",
			data:        map[string]any{"type": "stream_event", "uuid": "u1", "session_id": "s1"},
			expectError: "stream_event missing event field",
		},
		{
			name:        "stream_event_invalid_uuid_type",
			data:        map[string]any{"type": "stream_event", "uuid": 123, "session_id": "s1", "event": map[string]any{}},
			expectError: "stream_event missing uuid field",
		},
		{
			name:        "stream_event_invalid_session_id_type",
			data:        map[string]any{"type": "stream_event", "uuid": "u1", "session_id": 123, "event": map[string]any{}},
			expectError: "stream_event missing session_id field",
		},
		{
			name:        "stream_event_invalid_event_type",
			data:        map[string]any{"type": "stream_event", "uuid": "u1", "session_id": "s1", "event": "not a map"},
			expectError: "stream_event missing event field",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			parser := setupParserTest(t)
			_, err := parser.ParseMessage(test.data)
			assertParseError(t, err, test.expectError)
		})
	}
}

// TestResultMessageErrorsField tests the errors array field on ResultMessage (Issue #110)
func TestResultMessageErrorsField(t *testing.T) {
	parser := setupParserTest(t)

	baseData := map[string]any{
		"type":            "result",
		"subtype":         "error",
		"duration_ms":     0.0,
		"duration_api_ms": 0.0,
		"is_error":        true,
		"num_turns":       0.0,
		"session_id":      "s123",
	}

	t.Run("with_errors_array", func(t *testing.T) {
		data := make(map[string]any)
		for k, v := range baseData {
			data[k] = v
		}
		data["errors"] = []any{
			"No conversation found with session ID: abc-123",
			"second error",
		}

		msg, err := parser.ParseMessage(data)
		assertNoParseError(t, err)

		resultMsg := msg.(*shared.ResultMessage)
		if len(resultMsg.Errors) != 2 {
			t.Fatalf("expected 2 errors, got %d", len(resultMsg.Errors))
		}
		if resultMsg.Errors[0] != "No conversation found with session ID: abc-123" {
			t.Errorf("unexpected first error: %s", resultMsg.Errors[0])
		}
		if resultMsg.Errors[1] != "second error" {
			t.Errorf("unexpected second error: %s", resultMsg.Errors[1])
		}
	})

	t.Run("without_errors_array", func(t *testing.T) {
		msg, err := parser.ParseMessage(baseData)
		assertNoParseError(t, err)

		resultMsg := msg.(*shared.ResultMessage)
		if len(resultMsg.Errors) != 0 {
			t.Errorf("expected empty errors, got %v", resultMsg.Errors)
		}
	})

	t.Run("with_empty_errors_array", func(t *testing.T) {
		data := make(map[string]any)
		for k, v := range baseData {
			data[k] = v
		}
		data["errors"] = []any{}

		msg, err := parser.ParseMessage(data)
		assertNoParseError(t, err)

		resultMsg := msg.(*shared.ResultMessage)
		if len(resultMsg.Errors) != 0 {
			t.Errorf("expected empty errors, got %v", resultMsg.Errors)
		}
	})

	t.Run("with_non_string_errors_ignored", func(t *testing.T) {
		data := make(map[string]any)
		for k, v := range baseData {
			data[k] = v
		}
		data["errors"] = []any{"valid error", 123, true}

		msg, err := parser.ParseMessage(data)
		assertNoParseError(t, err)

		resultMsg := msg.(*shared.ResultMessage)
		if len(resultMsg.Errors) != 1 {
			t.Fatalf("expected 1 error (non-strings skipped), got %d", len(resultMsg.Errors))
		}
		if resultMsg.Errors[0] != "valid error" {
			t.Errorf("unexpected error: %s", resultMsg.Errors[0])
		}
	})
}
