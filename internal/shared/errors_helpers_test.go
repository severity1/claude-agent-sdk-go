package shared

import (
	"errors"
	"fmt"
	"testing"
)

// Test constants to avoid magic strings.
const (
	testCLIPath      = "/usr/bin/claude"
	testStderrOutput = "permission denied"
)

// =============================================================================
// Test Functions (primary purpose - FIRST per project conventions)
// =============================================================================

// TestIsErrorHelpers tests all Is* helper functions with table-driven tests.
func TestIsErrorHelpers(t *testing.T) {
	// Create test errors
	connErr := NewConnectionError("connection failed", nil)
	cliErr := NewCLINotFoundError(testCLIPath, "CLI not found")
	procErr := NewProcessError("process failed", 1, "stderr output")
	jsonErr := NewJSONDecodeError("invalid json", 0, errors.New("syntax error"))
	msgErr := NewMessageParseError("parse failed", map[string]any{"type": "unknown"})

	tests := []struct {
		name    string
		err     error
		checker func(error) bool
		want    bool
	}{
		// Direct error checks - should return true
		{"connection_error_direct", connErr, IsConnectionError, true},
		{"cli_not_found_error_direct", cliErr, IsCLINotFoundError, true},
		{"process_error_direct", procErr, IsProcessError, true},
		{"json_decode_error_direct", jsonErr, IsJSONDecodeError, true},
		{"message_parse_error_direct", msgErr, IsMessageParseError, true},

		// Wrapped error checks - should return true (errors.As handles unwrapping)
		{"connection_error_wrapped", fmt.Errorf("wrapped: %w", connErr), IsConnectionError, true},
		{"cli_not_found_error_wrapped", fmt.Errorf("wrapped: %w", cliErr), IsCLINotFoundError, true},
		{"process_error_wrapped", fmt.Errorf("wrapped: %w", procErr), IsProcessError, true},
		{"json_decode_error_wrapped", fmt.Errorf("wrapped: %w", jsonErr), IsJSONDecodeError, true},
		{"message_parse_error_wrapped", fmt.Errorf("wrapped: %w", msgErr), IsMessageParseError, true},

		// Nil error checks - should return false
		{"nil_connection_error", nil, IsConnectionError, false},
		{"nil_cli_not_found_error", nil, IsCLINotFoundError, false},
		{"nil_process_error", nil, IsProcessError, false},
		{"nil_json_decode_error", nil, IsJSONDecodeError, false},
		{"nil_message_parse_error", nil, IsMessageParseError, false},

		// Generic error checks - should return false
		{"generic_not_connection", errors.New("generic"), IsConnectionError, false},
		{"generic_not_cli_not_found", errors.New("generic"), IsCLINotFoundError, false},
		{"generic_not_process", errors.New("generic"), IsProcessError, false},
		{"generic_not_json_decode", errors.New("generic"), IsJSONDecodeError, false},
		{"generic_not_message_parse", errors.New("generic"), IsMessageParseError, false},

		// Wrong type checks - should return false
		{"connection_not_cli", connErr, IsCLINotFoundError, false},
		{"cli_not_connection", cliErr, IsConnectionError, false},
		{"process_not_json", procErr, IsJSONDecodeError, false},
		{"json_not_message", jsonErr, IsMessageParseError, false},
		{"message_not_process", msgErr, IsProcessError, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.checker(tt.err)
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

// TestAsErrorHelpers tests all As* helper functions with table-driven tests.
func TestAsErrorHelpers(t *testing.T) {
	// Create test errors
	connErr := NewConnectionError("connection failed", nil)
	cliErr := NewCLINotFoundError(testCLIPath, "CLI not found")
	procErr := NewProcessError("process failed", 1, testStderrOutput)
	jsonErr := NewJSONDecodeError("invalid json", 0, errors.New("syntax error"))
	msgErr := NewMessageParseError("parse failed", map[string]any{"type": "unknown"})

	t.Run("direct_errors_return_typed_pointer", func(t *testing.T) {
		// Direct errors should return typed pointers
		if result := AsConnectionError(connErr); result == nil {
			t.Error("AsConnectionError returned nil for ConnectionError")
		}
		if result := AsCLINotFoundError(cliErr); result == nil {
			t.Error("AsCLINotFoundError returned nil for CLINotFoundError")
		}
		if result := AsProcessError(procErr); result == nil {
			t.Error("AsProcessError returned nil for ProcessError")
		}
		if result := AsJSONDecodeError(jsonErr); result == nil {
			t.Error("AsJSONDecodeError returned nil for JSONDecodeError")
		}
		if result := AsMessageParseError(msgErr); result == nil {
			t.Error("AsMessageParseError returned nil for MessageParseError")
		}
	})

	t.Run("wrapped_errors_return_typed_pointer", func(t *testing.T) {
		// Wrapped errors should still return typed pointers
		if result := AsConnectionError(fmt.Errorf("wrapped: %w", connErr)); result == nil {
			t.Error("AsConnectionError returned nil for wrapped ConnectionError")
		}
		if result := AsCLINotFoundError(fmt.Errorf("wrapped: %w", cliErr)); result == nil {
			t.Error("AsCLINotFoundError returned nil for wrapped CLINotFoundError")
		}
		if result := AsProcessError(fmt.Errorf("wrapped: %w", procErr)); result == nil {
			t.Error("AsProcessError returned nil for wrapped ProcessError")
		}
		if result := AsJSONDecodeError(fmt.Errorf("wrapped: %w", jsonErr)); result == nil {
			t.Error("AsJSONDecodeError returned nil for wrapped JSONDecodeError")
		}
		if result := AsMessageParseError(fmt.Errorf("wrapped: %w", msgErr)); result == nil {
			t.Error("AsMessageParseError returned nil for wrapped MessageParseError")
		}
	})

	t.Run("nil_errors_return_nil", func(t *testing.T) {
		if result := AsConnectionError(nil); result != nil {
			t.Error("AsConnectionError should return nil for nil error")
		}
		if result := AsCLINotFoundError(nil); result != nil {
			t.Error("AsCLINotFoundError should return nil for nil error")
		}
		if result := AsProcessError(nil); result != nil {
			t.Error("AsProcessError should return nil for nil error")
		}
		if result := AsJSONDecodeError(nil); result != nil {
			t.Error("AsJSONDecodeError should return nil for nil error")
		}
		if result := AsMessageParseError(nil); result != nil {
			t.Error("AsMessageParseError should return nil for nil error")
		}
	})

	t.Run("generic_errors_return_nil", func(t *testing.T) {
		genericErr := errors.New("generic error")
		if result := AsConnectionError(genericErr); result != nil {
			t.Error("AsConnectionError should return nil for generic error")
		}
		if result := AsCLINotFoundError(genericErr); result != nil {
			t.Error("AsCLINotFoundError should return nil for generic error")
		}
		if result := AsProcessError(genericErr); result != nil {
			t.Error("AsProcessError should return nil for generic error")
		}
		if result := AsJSONDecodeError(genericErr); result != nil {
			t.Error("AsJSONDecodeError should return nil for generic error")
		}
		if result := AsMessageParseError(genericErr); result != nil {
			t.Error("AsMessageParseError should return nil for generic error")
		}
	})

	t.Run("wrong_type_returns_nil", func(t *testing.T) {
		// Each error type checked against wrong helper should return nil
		if result := AsCLINotFoundError(connErr); result != nil {
			t.Error("AsCLINotFoundError should return nil for ConnectionError")
		}
		if result := AsConnectionError(cliErr); result != nil {
			t.Error("AsConnectionError should return nil for CLINotFoundError")
		}
		if result := AsJSONDecodeError(procErr); result != nil {
			t.Error("AsJSONDecodeError should return nil for ProcessError")
		}
		if result := AsMessageParseError(jsonErr); result != nil {
			t.Error("AsMessageParseError should return nil for JSONDecodeError")
		}
		if result := AsProcessError(msgErr); result != nil {
			t.Error("AsProcessError should return nil for MessageParseError")
		}
	})
}

// TestAsErrorHelpersFieldAccess tests that As* helpers return errors with accessible fields.
func TestAsErrorHelpersFieldAccess(t *testing.T) {
	t.Run("cli_not_found_error_path_field", func(t *testing.T) {
		err := NewCLINotFoundError(testCLIPath, "CLI not found")
		result := AsCLINotFoundError(err)
		if result == nil {
			t.Fatal("AsCLINotFoundError returned nil")
			return
		}
		if result.Path != testCLIPath {
			t.Errorf("Path field: got %q, want %q", result.Path, testCLIPath)
		}
	})

	t.Run("process_error_fields", func(t *testing.T) {
		exitCode := 42
		err := NewProcessError("process failed", exitCode, testStderrOutput)
		result := AsProcessError(err)
		if result == nil {
			t.Fatal("AsProcessError returned nil")
			return
		}
		if result.ExitCode != exitCode {
			t.Errorf("ExitCode field: got %d, want %d", result.ExitCode, exitCode)
		}
		if result.Stderr != testStderrOutput {
			t.Errorf("Stderr field: got %q, want %q", result.Stderr, testStderrOutput)
		}
	})

	t.Run("json_decode_error_fields", func(t *testing.T) {
		line := `{"invalid": json}`
		position := 15
		originalErr := errors.New("syntax error")
		err := NewJSONDecodeError(line, position, originalErr)
		result := AsJSONDecodeError(err)
		if result == nil {
			t.Fatal("AsJSONDecodeError returned nil")
			return
		}
		if result.Line != line {
			t.Errorf("Line field: got %q, want %q", result.Line, line)
		}
		if result.Position != position {
			t.Errorf("Position field: got %d, want %d", result.Position, position)
		}
		if result.OriginalError != originalErr {
			t.Errorf("OriginalError field: got %v, want %v", result.OriginalError, originalErr)
		}
	})

	t.Run("message_parse_error_data_field", func(t *testing.T) {
		data := map[string]any{"type": "unknown", "content": "test"}
		err := NewMessageParseError("parse failed", data)
		result := AsMessageParseError(err)
		if result == nil {
			t.Fatal("AsMessageParseError returned nil")
			return
		}
		dataMap, ok := result.Data.(map[string]any)
		if !ok {
			t.Fatalf("Data field type: got %T, want map[string]any", result.Data)
		}
		if dataMap["type"] != "unknown" {
			t.Errorf("Data[type]: got %v, want %q", dataMap["type"], "unknown")
		}
	})

	t.Run("wrapped_error_field_access", func(t *testing.T) {
		// Fields should be accessible even through wrapped errors
		path := "/custom/path"
		err := NewCLINotFoundError(path, "not found")
		wrapped := fmt.Errorf("level1: %w", err)
		doubleWrapped := fmt.Errorf("level2: %w", wrapped)

		result := AsCLINotFoundError(doubleWrapped)
		if result == nil {
			t.Fatal("AsCLINotFoundError returned nil for double-wrapped error")
			return
		}
		if result.Path != path {
			t.Errorf("Path field through double wrapping: got %q, want %q", result.Path, path)
		}
	})
}

// TestErrorHelpersWithWrappedChain tests multi-level error wrapping.
func TestErrorHelpersWithWrappedChain(t *testing.T) {
	original := NewConnectionError("original error", nil)
	wrapped := fmt.Errorf("level1: %w", original)
	doubleWrapped := fmt.Errorf("level2: %w", wrapped)
	tripleWrapped := fmt.Errorf("level3: %w", doubleWrapped)

	t.Run("is_helper_through_chain", func(t *testing.T) {
		if !IsConnectionError(wrapped) {
			t.Error("IsConnectionError should return true for single-wrapped error")
		}
		if !IsConnectionError(doubleWrapped) {
			t.Error("IsConnectionError should return true for double-wrapped error")
		}
		if !IsConnectionError(tripleWrapped) {
			t.Error("IsConnectionError should return true for triple-wrapped error")
		}
	})

	t.Run("as_helper_through_chain", func(t *testing.T) {
		if result := AsConnectionError(wrapped); result == nil {
			t.Error("AsConnectionError should return non-nil for single-wrapped error")
		}
		if result := AsConnectionError(doubleWrapped); result == nil {
			t.Error("AsConnectionError should return non-nil for double-wrapped error")
		}
		if result := AsConnectionError(tripleWrapped); result == nil {
			t.Error("AsConnectionError should return non-nil for triple-wrapped error")
		}
	})
}
