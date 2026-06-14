package shared

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
)

// TestStreamMessageJSON tests StreamMessage JSON marshaling and field behavior
func TestStreamMessageJSON(t *testing.T) {
	tests := []struct {
		name     string
		msg      *StreamMessage
		expected map[string]any
	}{
		{
			name: "complete_message",
			msg: &StreamMessage{
				Type:            "request",
				Message:         "test message",
				ParentToolUseID: stringPtr("tool-123"),
				SessionID:       "session-456",
				RequestID:       "req-789",
				Request:         map[string]any{"key": "value"},
				Response:        map[string]any{"status": "ok"},
			},
			expected: map[string]any{
				"type":               "request",
				"message":            "test message",
				"parent_tool_use_id": "tool-123",
				"session_id":         "session-456",
				"request_id":         "req-789",
				"request":            map[string]any{"key": "value"},
				"response":           map[string]any{"status": "ok"},
			},
		},
		{
			name: "minimal_message_always_emits_session_and_parent",
			msg: &StreamMessage{
				Type: "user",
				// session_id and parent_tool_use_id are emitted even when
				// empty/nil so the wire shape matches the Python/TS SDKs.
			},
			expected: map[string]any{
				"type":               "user",
				"session_id":         "",
				"parent_tool_use_id": nil,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertStreamMessageJSON(t, test.msg, test.expected)
		})
	}
}

// TestMessageIteratorInterface tests MessageIterator interface compliance
func TestMessageIteratorInterface(t *testing.T) {
	// Create a simple mock implementation to verify interface compliance
	iter := &mockIterator{}
	assertMessageIteratorInterface(t, iter)
}

// Simple mock implementation for interface compliance testing
type mockIterator struct{}

func (m *mockIterator) Next(_ context.Context) (Message, error) {
	return &UserMessage{Content: "test"}, nil
}

func (m *mockIterator) Close() error {
	return nil
}

// assertStreamMessageJSON verifies JSON marshaling behavior
func assertStreamMessageJSON(t *testing.T, msg *StreamMessage, expected map[string]any) {
	t.Helper()

	// Test marshaling
	jsonData, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Failed to marshal StreamMessage: %v", err)
	}

	// Parse and verify structure
	var parsed map[string]any
	if err := json.Unmarshal(jsonData, &parsed); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	// Verify expected fields are present and correct
	for key, expectedValue := range expected {
		actualValue, exists := parsed[key]
		if !exists {
			t.Errorf("Expected JSON key %q not found", key)
			continue
		}
		if !deepEqual(actualValue, expectedValue) {
			t.Errorf("JSON key %q: expected %v, got %v", key, expectedValue, actualValue)
		}
	}

	// Verify omitempty behavior - no unexpected fields for minimal message
	if len(expected) == 1 && expected["type"] != nil {
		if len(parsed) != 1 {
			t.Errorf("Expected only 'type' field for minimal message, got %d fields: %v", len(parsed), parsed)
		}
	}
}

// assertMessageIteratorInterface verifies interface compliance
func assertMessageIteratorInterface(t *testing.T, iter MessageIterator) {
	t.Helper()

	// Verify Next method works
	ctx := context.Background()
	msg, err := iter.Next(ctx)
	if err != nil {
		t.Errorf("Next() failed: %v", err)
	}
	if msg == nil {
		t.Error("Next() returned nil message")
	}

	// Verify Close method works
	if err := iter.Close(); err != nil {
		t.Errorf("Close() failed: %v", err)
	}
}

// Helper functions

func stringPtr(s string) *string {
	return &s
}

// deepEqual performs simple deep comparison for test values
func deepEqual(a, b any) bool {
	aJSON, aErr := json.Marshal(a)
	bJSON, bErr := json.Marshal(b)
	if aErr != nil || bErr != nil {
		return false
	}
	return bytes.Equal(aJSON, bJSON)
}
