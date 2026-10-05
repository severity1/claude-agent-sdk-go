package shared

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// TestErrorTypes tests all error types using table-driven approach
func TestErrorTypes(t *testing.T) {
	tests := []struct {
		name         string
		createError  func() SDKError
		expectedType string
		validateFunc func(*testing.T, SDKError)
	}{
		{
			name: "connection_error",
			createError: func() SDKError {
				return NewConnectionError("connection failed", errors.New("network error"))
			},
			expectedType: "connection_error",
			validateFunc: validateConnectionError,
		},
		{
			name: "cli_not_found_error",
			createError: func() SDKError {
				return NewCLINotFoundError("/usr/bin/claude", "CLI not found")
			},
			expectedType: "cli_not_found_error",
			validateFunc: validateCLINotFoundError,
		},
		{
			name: "process_error",
			createError: func() SDKError {
				return NewProcessError("process failed", 1, "permission denied")
			},
			expectedType: "process_error",
			validateFunc: validateProcessError,
		},
		{
			name: "json_decode_error",
			createError: func() SDKError {
				return NewJSONDecodeError(`{"invalid": json}`, 15, errors.New("syntax error"))
			},
			expectedType: "json_decode_error",
			validateFunc: validateJSONDecodeError,
		},
		{
			name: "message_parse_error",
			createError: func() SDKError {
				return NewMessageParseError("invalid structure", map[string]any{"type": "unknown"})
			},
			expectedType: "message_parse_error",
			validateFunc: validateMessageParseError,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.createError()
			assertErrorType(t, err, test.expectedType)
			assertErrorMessage(t, err, "")
			test.validateFunc(t, err)
		})
	}
}

// TestErrorInterfaceCompliance tests SDKError interface compliance
func TestErrorInterfaceCompliance(t *testing.T) {
	// Create all error types for interface testing
	errorInstances := []SDKError{
		NewConnectionError("test", nil),
		NewCLINotFoundError("", "test"),
		NewProcessError("test", 1, "stderr"),
		NewJSONDecodeError("line", 0, nil),
		NewMessageParseError("test", nil),
	}

	for i, err := range errorInstances {
		// Must implement standard error interface
		if err.Error() == "" {
			t.Errorf("Error %d: Error() returned empty string", i)
		}

		// Must implement SDKError interface
		assertSDKErrorInterface(t, err)

		// Type method must return non-empty string
		if err.Type() == "" {
			t.Errorf("Error %d: Type() returned empty string", i)
		}
	}
}

// TestErrorWrappingBehavior tests error wrapping and unwrapping
func TestErrorWrappingBehavior(t *testing.T) {
	cause := errors.New("root cause")

	// Test ConnectionError wrapping
	connErr := NewConnectionError("connection failed", cause)
	assertErrorWrapping(t, connErr, cause)

	// Test JSONDecodeError wrapping
	jsonErr := NewJSONDecodeError("invalid json", 0, cause)
	assertErrorWrapping(t, jsonErr, cause)
}

// TestErrorFieldValidation tests error-specific field validation
func TestErrorFieldValidation(t *testing.T) {
	// Test CLINotFoundError path handling
	path := "/usr/bin/claude"
	message := "CLI not found"
	errorWithPath := NewCLINotFoundError(path, message)

	expectedMsg := fmt.Sprintf("%s: %s", message, path)
	if errorWithPath.Error() != expectedMsg {
		t.Errorf("Expected error message %q, got %q", expectedMsg, errorWithPath.Error())
	}
	if errorWithPath.Path != path {
		t.Errorf("Expected path %q, got %q", path, errorWithPath.Path)
	}

	// Test ProcessError fields
	exitCode := 1
	stderr := "permission denied"
	processErr := NewProcessError("process failed", exitCode, stderr)

	if processErr.ExitCode != exitCode {
		t.Errorf("Expected exit code %d, got %d", exitCode, processErr.ExitCode)
	}
	if processErr.Stderr != stderr {
		t.Errorf("Expected stderr %q, got %q", stderr, processErr.Stderr)
	}

	// Test JSONDecodeError line truncation
	longLine := strings.Repeat("x", 150)
	longErr := NewJSONDecodeError(longLine, 0, nil)
	if len(longErr.Error()) >= len(longLine) {
		t.Error("Expected long line to be truncated in error message")
	}
}

// TestBaseErrorDirect tests BaseError when used directly (not embedded)
func TestBaseErrorDirect(t *testing.T) {
	// Test basic BaseError functionality
	baseErr := &BaseError{message: "direct base error"}

	assertErrorType(t, baseErr, "base_error")
	assertErrorMessage(t, baseErr, "direct base error")
	assertSDKErrorInterface(t, baseErr)

	// Test with cause
	cause := errors.New("underlying cause")
	baseErrWithCause := &BaseError{message: "wrapper error", cause: cause}

	assertErrorWrapping(t, baseErrWithCause, cause)
	if !strings.Contains(baseErrWithCause.Error(), "wrapper error") {
		t.Error("Expected error message to contain wrapper message")
	}
	if !strings.Contains(baseErrWithCause.Error(), "underlying cause") {
		t.Error("Expected error message to contain cause message")
	}
}

// TestResultMessageMarshaling tests ResultMessage JSON marshaling
func TestResultMessageMarshaling(t *testing.T) {
	// Test basic ResultMessage marshaling
	result := &ResultMessage{
		Subtype:       "completion",
		DurationMs:    1000,
		DurationAPIMs: 800,
		IsError:       false,
		NumTurns:      1,
		SessionID:     "test-session",
		TotalCostUSD:  floatPtr(0.05),
	}

	data, err := result.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON failed: %v", err)
	}

	// Verify JSON contains expected fields
	jsonStr := string(data)
	expectedFields := []string{
		`"type":"result"`,
		`"subtype":"completion"`,
		`"duration_ms":1000`,
		`"session_id":"test-session"`,
	}

	for _, field := range expectedFields {
		if !strings.Contains(jsonStr, field) {
			t.Errorf("Expected JSON to contain %s, got: %s", field, jsonStr)
		}
	}

	// Test that JSON is valid by unmarshaling back
	var unmarshaled map[string]any
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Errorf("Generated JSON is not valid: %v", err)
	}

	// Verify type field is correctly set
	if typeField, ok := unmarshaled["type"]; !ok || typeField != "result" {
		t.Errorf("Expected type field to be 'result', got: %v", typeField)
	}

	if _, ok := unmarshaled["stop_reason"]; ok {
		t.Errorf("Expected stop_reason absent when nil, got: %s", jsonStr)
	}

	stopReason := "end_turn"
	result.StopReason = &stopReason
	data, err = result.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON failed: %v", err)
	}
	if !strings.Contains(string(data), `"stop_reason":"end_turn"`) {
		t.Errorf("Expected stop_reason in JSON, got: %s", data)
	}
}

// Helper functions

// assertErrorType verifies error has expected type
func assertErrorType(t *testing.T, err SDKError, expectedType string) {
	t.Helper()
	if err.Type() != expectedType {
		t.Errorf("Expected error type %q, got %q", expectedType, err.Type())
	}
}

// assertErrorMessage verifies error message is non-empty or contains substring
func assertErrorMessage(t *testing.T, err error, expectedSubstring string) {
	t.Helper()
	msg := err.Error()
	if msg == "" {
		t.Error("Expected non-empty error message")
		return
	}
	if expectedSubstring != "" && !strings.Contains(msg, expectedSubstring) {
		t.Errorf("Expected error message to contain %q, got %q", expectedSubstring, msg)
	}
}

// assertSDKErrorInterface verifies SDKError interface compliance
func assertSDKErrorInterface(t *testing.T, err SDKError) {
	t.Helper()
	sdkErr := err
	if sdkErr.Error() == "" {
		t.Error("Expected error message from SDKError interface")
	}
	if sdkErr.Type() == "" {
		t.Error("Expected error type from SDKError interface")
	}
}

// assertErrorWrapping verifies error wrapping behavior
func assertErrorWrapping(t *testing.T, wrapper, cause error) {
	t.Helper()
	if !errors.Is(wrapper, cause) {
		t.Error("Expected error to wrap cause with errors.Is() support")
	}
}

// Error-specific validation functions

// validateConnectionError validates ConnectionError specifics
func validateConnectionError(t *testing.T, err SDKError) {
	t.Helper()
	connErr, ok := err.(*ConnectionError)
	if !ok {
		t.Fatalf("Expected *ConnectionError, got %T", err)
	}
	if connErr.message == "" {
		t.Error("Expected non-empty message field")
	}
}

// validateCLINotFoundError validates CLINotFoundError specifics
func validateCLINotFoundError(t *testing.T, err SDKError) {
	t.Helper()
	cliErr, ok := err.(*CLINotFoundError)
	if !ok {
		t.Fatalf("Expected *CLINotFoundError, got %T", err)
	}
	if !strings.Contains(err.Error(), "CLI not found") {
		t.Error("Expected error message to contain 'CLI not found'")
	}
	if cliErr.Path != "/usr/bin/claude" {
		t.Errorf("Expected path '/usr/bin/claude', got %q", cliErr.Path)
	}
}

// validateProcessError validates ProcessError specifics
func validateProcessError(t *testing.T, err SDKError) {
	t.Helper()
	procErr, ok := err.(*ProcessError)
	if !ok {
		t.Fatalf("Expected *ProcessError, got %T", err)
	}
	if procErr.ExitCode != 1 {
		t.Errorf("Expected exit code 1, got %d", procErr.ExitCode)
	}
	if procErr.Stderr != "permission denied" {
		t.Errorf("Expected stderr 'permission denied', got %q", procErr.Stderr)
	}
	if !strings.Contains(err.Error(), "exit code: 1") {
		t.Error("Expected error message to contain exit code")
	}
}

// validateJSONDecodeError validates JSONDecodeError specifics
func validateJSONDecodeError(t *testing.T, err SDKError) {
	t.Helper()
	jsonErr, ok := err.(*JSONDecodeError)
	if !ok {
		t.Fatalf("Expected *JSONDecodeError, got %T", err)
	}
	if jsonErr.Line == "" {
		t.Error("Expected non-empty line field")
	}
	if jsonErr.Position != 15 {
		t.Errorf("Expected position 15, got %d", jsonErr.Position)
	}
	if !strings.Contains(err.Error(), "Failed to decode JSON") {
		t.Error("Expected error message to contain 'Failed to decode JSON'")
	}
}

// validateMessageParseError validates MessageParseError specifics
func validateMessageParseError(t *testing.T, err SDKError) {
	t.Helper()
	msgErr, ok := err.(*MessageParseError)
	if !ok {
		t.Fatalf("Expected *MessageParseError, got %T", err)
	}
	if msgErr.Data == nil {
		t.Error("Expected data to be preserved")
		return
	}
	dataMap, ok := msgErr.Data.(map[string]any)
	if !ok {
		t.Errorf("Expected data to be map[string]any, got %T", msgErr.Data)
		return
	}
	if dataMap["type"] != "unknown" {
		t.Errorf("Expected data type 'unknown', got %v", dataMap["type"])
	}
}

// floatPtr creates a float64 pointer for testing
func floatPtr(f float64) *float64 {
	return &f
}
