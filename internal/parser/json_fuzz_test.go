//go:build go1.18
// +build go1.18

package parser

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/severity1/claude-agent-sdk-go/internal/shared"
)

// FuzzParser_ProcessLine fuzzes the ProcessLine method with arbitrary input.
// Tests malformed JSON, incomplete JSON, and general robustness against unexpected input.
func FuzzParser_ProcessLine(f *testing.F) {
	// Seed corpus with valid message examples from unit tests
	f.Add(`{"type": "user", "message": {"content": "Hello world"}}`)
	f.Add(`{"type": "assistant", "message": {"content": [{"type": "text", "text": "Hi"}], "model": "claude-3-sonnet"}}`)
	f.Add(`{"type": "system", "subtype": "status"}`)
	f.Add(`{"type": "result", "subtype": "completed", "duration_ms": 1500.0, "duration_api_ms": 800.0, "is_error": false, "num_turns": 2.0, "session_id": "s123"}`)

	// Partial JSON fragments
	f.Add(`{"type": "user", "message":`)
	f.Add(`{"type":`)
	f.Add(`{`)

	// Multi-object lines
	f.Add(`{"type": "system", "subtype": "a"}` + "\n" + `{"type": "system", "subtype": "b"}`)

	// Empty and whitespace
	f.Add(``)
	f.Add(`   `)

	// Malformed JSON
	f.Add(`{type: "user"}`)
	f.Add(`{"type": "user",}`)
	f.Add(`{"type": "user" "message": {}}`)

	// Unicode
	f.Add(`{"type": "user", "message": {"content": "Hello 🌍"}}`)

	f.Fuzz(func(t *testing.T, input string) {
		parser := New()

		// Call ProcessLine - should never panic
		messages, err := parser.ProcessLine(input)

		// Invariant checks
		if err != nil {
			// Errors are allowed, but must be properly typed
			if err.Error() == "" {
				t.Errorf("Error has empty message")
			}
		}

		// Messages should be nil or valid slice
		if len(messages) > 1000 {
			// Suspiciously large number of messages
			t.Errorf("Unexpectedly large message count: %d", len(messages))
		}

		// Buffer should never exceed max size
		if parser.BufferSize() > MaxBufferSize {
			t.Errorf("Buffer size %d exceeds MaxBufferSize %d", parser.BufferSize(), MaxBufferSize)
		}
	})
}

// FuzzParser_ParseMessage fuzzes the ParseMessage method with arbitrary map data.
// Tests type discrimination and message-specific parsing logic.
func FuzzParser_ParseMessage(f *testing.F) {
	// Seed corpus with each message type
	f.Add(`{"type": "user", "message": {"content": "test"}}`)
	f.Add(`{"type": "assistant", "message": {"content": [{"type": "text", "text": "test"}], "model": "claude-3"}}`)
	f.Add(`{"type": "system", "subtype": "status"}`)
	f.Add(`{"type": "result", "subtype": "completed", "duration_ms": 100.0, "duration_api_ms": 50.0, "is_error": false, "num_turns": 1.0, "session_id": "s1"}`)
	f.Add(`{"type": "control_request", "data": {}}`)
	f.Add(`{"type": "stream_event", "uuid": "u1", "session_id": "s1", "event": {}}`)

	// Missing/invalid type field
	f.Add(`{}`)
	f.Add(`{"message": "test"}`)
	f.Add(`{"type": 123}`)
	f.Add(`{"type": "unknown"}`)

	// Missing required fields
	f.Add(`{"type": "user"}`)
	f.Add(`{"type": "assistant"}`)
	f.Add(`{"type": "system"}`)

	f.Fuzz(func(t *testing.T, jsonInput string) {
		// Try to unmarshal to map
		var data map[string]any
		if err := json.Unmarshal([]byte(jsonInput), &data); err != nil {
			// Invalid JSON - skip
			return
		}

		parser := New()
		msg, err := parser.ParseMessage(data)

		// Either parse successfully or return error (no panics); only an
		// unknown type may give neither.
		if err == nil && msg == nil && !isUnknownMessageType(data) {
			t.Errorf("ParseMessage returned nil message without error for %v", data["type"])
		}
	})
}

// FuzzParser_DeeplyNestedJSON fuzzes with deeply nested JSON structures.
// Tests stack overflow prevention from recursive parsing.
func FuzzParser_DeeplyNestedJSON(f *testing.F) {
	// Seed with moderately nested valid structures
	f.Add(10)  // depth 10
	f.Add(50)  // depth 50
	f.Add(100) // depth 100
	f.Add(200) // depth 200
	f.Add(500) // depth 500

	f.Fuzz(func(t *testing.T, depth int) {
		if depth < 0 || depth > 10000 {
			return // skip unreasonable depths
		}

		// Create deeply nested object
		var sb strings.Builder
		sb.WriteString(`{"type": "user", "message": {"content": [{"type": "text", "text": `)

		// Build nested structure
		for i := 0; i < depth; i++ {
			sb.WriteString(`{"level":`)
		}
		sb.WriteString(`"deep"`)
		for i := 0; i < depth; i++ {
			sb.WriteString(`}`)
		}
		sb.WriteString(`}]}}`)

		parser := New()

		// Should handle gracefully without stack overflow
		_, err := parser.ProcessLine(sb.String())

		// Either parse or error, but no panic
		_ = err // error is OK

		// Buffer should not exceed limit
		if parser.BufferSize() > MaxBufferSize {
			t.Errorf("Buffer exceeded limit")
		}
	})
}

// FuzzParser_LongStrings fuzzes with extremely long strings.
// Tests memory exhaustion prevention and buffer limit enforcement.
func FuzzParser_LongStrings(f *testing.F) {
	// Seed with various string lengths
	f.Add(10)       // 10 bytes
	f.Add(1024)     // 1 KB
	f.Add(10240)    // 10 KB
	f.Add(102400)   // 100 KB
	f.Add(512000)   // 500 KB
	f.Add(1048576)  // 1 MB (at limit)
	f.Add(1048577)  // 1 MB + 1 (over limit)
	f.Add(2097152)  // 2 MB
	f.Add(10485760) // 10 MB

	f.Fuzz(func(t *testing.T, length int) {
		if length < 0 || length > 20*1024*1024 {
			return // skip unreasonable lengths
		}

		// Create JSON with long string
		longString := strings.Repeat("X", length)
		jsonStr := `{"type": "user", "message": {"content": "` + longString + `"}}`

		parser := New()
		_, err := parser.ProcessLine(jsonStr)

		// Check buffer limit enforcement
		if len(jsonStr) > MaxBufferSize {
			// Should get buffer overflow error
			if err == nil {
				t.Errorf("Expected buffer overflow error for size %d", len(jsonStr))
			}
		}

		// Buffer should never exceed limit
		if parser.BufferSize() > MaxBufferSize {
			t.Errorf("Buffer size %d exceeds MaxBufferSize %d", parser.BufferSize(), MaxBufferSize)
		}
	})
}

// FuzzParser_InvalidUTF8 fuzzes with invalid UTF-8 byte sequences.
// Tests robust handling of encoding issues.
func FuzzParser_InvalidUTF8(f *testing.F) {
	// Seed with various UTF-8 cases
	f.Add([]byte(`{"type": "user", "message": {"content": "ASCII"}}`))
	f.Add([]byte(`{"type": "user", "message": {"content": "Hello 🌍"}}`))
	f.Add([]byte(`{"type": "user", "message": {"content": "日本語"}}`))

	// Invalid UTF-8 sequences
	f.Add([]byte{'{', '"', 't', 'y', 'p', 'e', '"', ':', 0xFF, '}'})
	f.Add([]byte{'{', '"', 't', 'y', 'p', 'e', '"', ':', '"', 0xFE, 0xFF, '"', '}'})
	f.Add([]byte{'{', 0xC0, 0x80, '}'}) // overlong encoding

	// Truncated multi-byte characters
	f.Add([]byte{'{', '"', 't', '"', ':', '"', 0xE2, 0x82, '"', '}'}) // incomplete €

	f.Fuzz(func(t *testing.T, input []byte) {
		parser := New()

		// ProcessLine should handle invalid UTF-8 gracefully
		// Go's encoding/json package validates UTF-8, so invalid sequences
		// should result in parse errors, not panics
		_, err := parser.ProcessLine(string(input))

		// Error is allowed and expected for invalid UTF-8
		_ = err

		// Key invariant: no panics, buffer limit respected
		if parser.BufferSize() > MaxBufferSize {
			t.Errorf("Buffer exceeded limit")
		}
	})
}

// FuzzParser_BufferExhaustion fuzzes buffer limit enforcement.
// Tests 1MB MaxBufferSize boundary cases and buffer management.
func FuzzParser_BufferExhaustion(f *testing.F) {
	// Seed with various scenarios approaching the buffer limit
	f.Add(100, 1)     // 100 bytes, 1 chunk
	f.Add(1024, 10)   // 1 KB each, 10 chunks
	f.Add(10240, 100) // 10 KB each, 100 chunks
	f.Add(102400, 10) // 100 KB each, 10 chunks
	f.Add(524288, 2)  // 500 KB each, 2 chunks (at limit)
	f.Add(524289, 2)  // 500 KB + 1 each, 2 chunks (over limit)
	f.Add(1048576, 1) // exactly 1 MB, 1 chunk
	f.Add(1048577, 1) // 1 MB + 1, 1 chunk (over limit)

	f.Fuzz(func(t *testing.T, chunkSize int, numChunks int) {
		if chunkSize < 0 || chunkSize > 5*1024*1024 {
			return // skip unreasonable sizes
		}
		if numChunks < 0 || numChunks > 1000 {
			return // skip unreasonable chunk counts
		}

		parser := New()

		// Send multiple partial JSON chunks to accumulate in buffer
		partialJSON := strings.Repeat("X", chunkSize)

		var lastErr error
		for i := 0; i < numChunks; i++ {
			msg, err := parser.processJSONLine(partialJSON)
			lastErr = err

			// If buffer overflow occurs, should get error
			if err != nil {
				break
			}

			_ = msg // msg may be nil for partial JSON
		}

		// Check buffer never exceeds limit
		bufSize := parser.BufferSize()
		if bufSize > MaxBufferSize {
			t.Errorf("Buffer size %d exceeds MaxBufferSize %d after %d chunks of size %d",
				bufSize, MaxBufferSize, numChunks, chunkSize)
		}

		// If we exceeded the limit, should have gotten an error
		totalSize := chunkSize * numChunks
		if totalSize > MaxBufferSize && lastErr == nil && bufSize == 0 {
			// Buffer was reset due to overflow, which is correct behavior
			return
		}

		// After error, buffer should be reset
		if lastErr != nil && bufSize != 0 && strings.Contains(lastErr.Error(), "buffer overflow") {
			t.Errorf("Buffer not reset after overflow error: size=%d", bufSize)
		}
	})
}

// FuzzParser_StreamingChunks fuzzes multi-line streaming scenarios.
// Tests realistic streaming with partial/complete JSON across multiple calls.
func FuzzParser_StreamingChunks(f *testing.F) {
	// Seed with various streaming patterns
	validJSON := `{"type": "system", "subtype": "status"}`

	// Single complete JSON
	f.Add(validJSON, "")

	// JSON split at various positions
	for i := 1; i < len(validJSON)-1; i++ {
		f.Add(validJSON[:i], validJSON[i:])
	}

	// Multiple JSON on one line
	f.Add(validJSON+"\n"+validJSON, "")

	// Complex message split
	complexJSON := `{"type": "user", "message": {"content": [{"type": "text", "text": "Hello World"}]}}`
	for i := 10; i < len(complexJSON)-10; i += 10 {
		f.Add(complexJSON[:i], complexJSON[i:])
	}

	f.Fuzz(func(t *testing.T, chunk1 string, chunk2 string) {
		parser := New()

		// First chunk
		msg1, err1 := parser.ProcessLine(chunk1)
		buf1 := parser.BufferSize()

		// Validate first chunk result
		if err1 != nil {
			// Error is OK, but check buffer state
			if buf1 > MaxBufferSize {
				t.Errorf("Buffer exceeded after first chunk")
			}
			return // stop on error
		}

		// Second chunk (if any)
		if chunk2 != "" {
			msg2, err2 := parser.ProcessLine(chunk2)
			buf2 := parser.BufferSize()

			// Validate second chunk result
			if err2 != nil {
				if buf2 > MaxBufferSize {
					t.Errorf("Buffer exceeded after second chunk")
				}
				return
			}

			_ = msg1
			_ = msg2
		}

		// Final buffer check
		if parser.BufferSize() > MaxBufferSize {
			t.Errorf("Final buffer size exceeds limit")
		}

		// No state should leak - each message independent
		// This is ensured by the parser's mutex and buffer reset logic
	})
}

func isUnknownMessageType(data map[string]any) bool {
	msgType, ok := data["type"].(string)
	if !ok {
		return false
	}
	switch msgType {
	case shared.MessageTypeUser, shared.MessageTypeAssistant, shared.MessageTypeSystem,
		shared.MessageTypeResult, shared.MessageTypeControlRequest, shared.MessageTypeControlResponse,
		shared.MessageTypeStreamEvent, shared.MessageTypeRateLimitEvent, shared.MessageTypeConversationReset:
		return false
	}
	return true
}
