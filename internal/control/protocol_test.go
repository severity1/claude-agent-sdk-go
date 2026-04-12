// Package control provides the SDK control protocol for bidirectional communication with Claude CLI.
package control

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// Test constants for model names used across dynamic control tests.
const testModelSonnet = "claude-sonnet-4-5"

// =============================================================================
// Phase 1: Control Message Type Tests
// =============================================================================

func TestControlMessageTypes(t *testing.T) {
	t.Run("message_type_constants", testMessageTypeConstants)
	t.Run("subtype_constants", testSubtypeConstants)
	t.Run("response_subtype_constants", testResponseSubtypeConstants)
}

func testMessageTypeConstants(t *testing.T) {
	t.Helper()

	tests := []struct {
		name     string
		constant string
		expected string
	}{
		{"control_request", MessageTypeControlRequest, "control_request"},
		{"control_response", MessageTypeControlResponse, "control_response"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.constant != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, tc.constant)
			}
		})
	}
}

func testSubtypeConstants(t *testing.T) {
	t.Helper()

	// These constants must match the Python SDK exactly for parity
	tests := []struct {
		name     string
		constant string
		expected string
	}{
		{"interrupt", SubtypeInterrupt, "interrupt"},
		{"can_use_tool", SubtypeCanUseTool, "can_use_tool"},
		{"initialize", SubtypeInitialize, "initialize"},
		{"set_permission_mode", SubtypeSetPermissionMode, "set_permission_mode"},
		{"set_model", SubtypeSetModel, "set_model"},
		{"hook_callback", SubtypeHookCallback, "hook_callback"},
		{"mcp_message", SubtypeMcpMessage, "mcp_message"},
		{"rewind_files", SubtypeRewindFiles, "rewind_files"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.constant != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, tc.constant)
			}
		})
	}
}

func testResponseSubtypeConstants(t *testing.T) {
	t.Helper()

	tests := []struct {
		name     string
		constant string
		expected string
	}{
		{"success", ResponseSubtypeSuccess, "success"},
		{"error", ResponseSubtypeError, "error"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.constant != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, tc.constant)
			}
		})
	}
}

func TestSDKControlRequestSerialization(t *testing.T) {
	t.Run("marshal_interrupt_request", testMarshalInterruptRequest)
	t.Run("marshal_initialize_request", testMarshalInitializeRequest)
	t.Run("unmarshal_control_request", testUnmarshalControlRequest)
}

func testMarshalInterruptRequest(t *testing.T) {
	t.Helper()

	req := SDKControlRequest{
		Type:      MessageTypeControlRequest,
		RequestID: "req_1_abc123",
		Request: InterruptRequest{
			Subtype: SubtypeInterrupt,
		},
	}

	data, err := json.Marshal(req)
	assertControlNoError(t, err)

	var parsed map[string]any
	err = json.Unmarshal(data, &parsed)
	assertControlNoError(t, err)

	assertControlEqual(t, "control_request", parsed["type"])
	assertControlEqual(t, "req_1_abc123", parsed["request_id"])

	request, ok := parsed["request"].(map[string]any)
	if !ok {
		t.Fatal("request field should be an object")
	}
	assertControlEqual(t, "interrupt", request["subtype"])
}

func testMarshalInitializeRequest(t *testing.T) {
	t.Helper()

	req := SDKControlRequest{
		Type:      MessageTypeControlRequest,
		RequestID: "req_2_def456",
		Request: InitializeRequest{
			Subtype: SubtypeInitialize,
		},
	}

	data, err := json.Marshal(req)
	assertControlNoError(t, err)

	var parsed map[string]any
	err = json.Unmarshal(data, &parsed)
	assertControlNoError(t, err)

	assertControlEqual(t, "control_request", parsed["type"])
	assertControlEqual(t, "req_2_def456", parsed["request_id"])

	request, ok := parsed["request"].(map[string]any)
	if !ok {
		t.Fatal("request field should be an object")
	}
	assertControlEqual(t, "initialize", request["subtype"])
}

func testUnmarshalControlRequest(t *testing.T) {
	t.Helper()

	jsonData := `{
		"type": "control_request",
		"request_id": "req_3_ghi789",
		"request": {
			"subtype": "interrupt"
		}
	}`

	var req SDKControlRequest
	err := json.Unmarshal([]byte(jsonData), &req)
	assertControlNoError(t, err)

	assertControlEqual(t, MessageTypeControlRequest, req.Type)
	assertControlEqual(t, "req_3_ghi789", req.RequestID)

	// Request is unmarshaled as map[string]any due to interface{} type
	request, ok := req.Request.(map[string]any)
	if !ok {
		t.Fatal("request field should unmarshal as map[string]any")
	}
	assertControlEqual(t, "interrupt", request["subtype"])
}

func TestSDKControlResponseSerialization(t *testing.T) {
	t.Run("marshal_success_response", testMarshalSuccessResponse)
	t.Run("marshal_error_response", testMarshalErrorResponse)
	t.Run("unmarshal_success_response", testUnmarshalSuccessResponse)
	t.Run("unmarshal_error_response", testUnmarshalErrorResponse)
}

func testMarshalSuccessResponse(t *testing.T) {
	t.Helper()

	resp := SDKControlResponse{
		Type: MessageTypeControlResponse,
		Response: Response{
			Subtype:   ResponseSubtypeSuccess,
			RequestID: "req_1_abc123",
			Response: map[string]any{
				"supported_commands": []string{"interrupt", "initialize"},
			},
		},
	}

	data, err := json.Marshal(resp)
	assertControlNoError(t, err)

	var parsed map[string]any
	err = json.Unmarshal(data, &parsed)
	assertControlNoError(t, err)

	assertControlEqual(t, "control_response", parsed["type"])

	response, ok := parsed["response"].(map[string]any)
	if !ok {
		t.Fatal("response field should be an object")
	}
	assertControlEqual(t, "success", response["subtype"])
	assertControlEqual(t, "req_1_abc123", response["request_id"])
}

func testMarshalErrorResponse(t *testing.T) {
	t.Helper()

	resp := SDKControlResponse{
		Type: MessageTypeControlResponse,
		Response: Response{
			Subtype:   ResponseSubtypeError,
			RequestID: "req_2_def456",
			Error:     "initialization failed: timeout",
		},
	}

	data, err := json.Marshal(resp)
	assertControlNoError(t, err)

	var parsed map[string]any
	err = json.Unmarshal(data, &parsed)
	assertControlNoError(t, err)

	response, ok := parsed["response"].(map[string]any)
	if !ok {
		t.Fatal("response field should be an object")
	}
	assertControlEqual(t, "error", response["subtype"])
	assertControlEqual(t, "initialization failed: timeout", response["error"])
}

func testUnmarshalSuccessResponse(t *testing.T) {
	t.Helper()

	jsonData := `{
		"type": "control_response",
		"response": {
			"subtype": "success",
			"request_id": "req_1_abc123",
			"response": {"status": "ok"}
		}
	}`

	var resp SDKControlResponse
	err := json.Unmarshal([]byte(jsonData), &resp)
	assertControlNoError(t, err)

	assertControlEqual(t, MessageTypeControlResponse, resp.Type)
	assertControlEqual(t, ResponseSubtypeSuccess, resp.Response.Subtype)
	assertControlEqual(t, "req_1_abc123", resp.Response.RequestID)
}

func testUnmarshalErrorResponse(t *testing.T) {
	t.Helper()

	jsonData := `{
		"type": "control_response",
		"response": {
			"subtype": "error",
			"request_id": "req_2_def456",
			"error": "unknown command"
		}
	}`

	var resp SDKControlResponse
	err := json.Unmarshal([]byte(jsonData), &resp)
	assertControlNoError(t, err)

	assertControlEqual(t, ResponseSubtypeError, resp.Response.Subtype)
	assertControlEqual(t, "unknown command", resp.Response.Error)
}

func TestInitializeRequestResponse(t *testing.T) {
	t.Run("initialize_request_structure", testInitializeRequestStructure)
	t.Run("initialize_response_structure", testInitializeResponseStructure)
}

func testInitializeRequestStructure(t *testing.T) {
	t.Helper()

	req := InitializeRequest{
		Subtype: SubtypeInitialize,
	}

	data, err := json.Marshal(req)
	assertControlNoError(t, err)

	var parsed map[string]any
	err = json.Unmarshal(data, &parsed)
	assertControlNoError(t, err)

	assertControlEqual(t, "initialize", parsed["subtype"])
}

func testInitializeResponseStructure(t *testing.T) {
	t.Helper()

	jsonData := `{
		"supported_commands": ["interrupt", "initialize", "set_permission_mode"]
	}`

	var resp InitializeResponse
	err := json.Unmarshal([]byte(jsonData), &resp)
	assertControlNoError(t, err)

	if len(resp.SupportedCommands) != 3 {
		t.Errorf("expected 3 supported commands, got %d", len(resp.SupportedCommands))
	}
	assertControlEqual(t, "interrupt", resp.SupportedCommands[0])
}

// =============================================================================
// Phase 2: Request/Response Correlation Tests
// =============================================================================

func TestRequestIDGeneration(t *testing.T) {
	t.Run("format_matches_python_sdk", testRequestIDFormat)
	t.Run("unique_ids", testRequestIDUniqueness)
	t.Run("counter_increments", testRequestIDCounterIncrements)
}

func testRequestIDFormat(t *testing.T) {
	t.Helper()

	transport := newControlMockTransport()
	protocol := NewProtocol(transport)

	id := protocol.generateRequestID()

	// Format: req_{counter}_{random_hex}
	// Example: req_1_a1b2c3d4
	if len(id) < 10 {
		t.Errorf("request ID too short: %s", id)
	}

	// Should start with "req_"
	if id[:4] != "req_" {
		t.Errorf("request ID should start with 'req_', got: %s", id)
	}
}

func testRequestIDUniqueness(t *testing.T) {
	t.Helper()

	transport := newControlMockTransport()
	protocol := NewProtocol(transport)

	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := protocol.generateRequestID()
		if ids[id] {
			t.Errorf("duplicate request ID generated: %s", id)
		}
		ids[id] = true
	}
}

func testRequestIDCounterIncrements(t *testing.T) {
	t.Helper()

	transport := newControlMockTransport()
	protocol := NewProtocol(transport)

	id1 := protocol.generateRequestID()
	id2 := protocol.generateRequestID()
	id3 := protocol.generateRequestID()

	// IDs should be different
	if id1 == id2 || id2 == id3 || id1 == id3 {
		t.Errorf("request IDs should be unique: %s, %s, %s", id1, id2, id3)
	}
}

func TestRequestResponseCorrelation(t *testing.T) {
	t.Run("response_matched_to_request", testResponseMatchedToRequest)
	t.Run("unknown_request_id_ignored", testUnknownRequestIDIgnored)
}

func testResponseMatchedToRequest(t *testing.T) {
	t.Helper()

	ctx, cancel := setupControlTestContext(t, 5*time.Second)
	defer cancel()

	transport := newControlMockTransport()
	protocol := NewProtocol(transport)

	// Start protocol to process messages
	err := protocol.Start(ctx)
	assertControlNoError(t, err)
	defer func() { _ = protocol.Close() }()

	// Send a request and inject the response
	go func() {
		// Wait for the request to be sent
		time.Sleep(50 * time.Millisecond)

		// Get the request ID from the sent message
		transport.mu.Lock()
		if len(transport.writtenData) == 0 {
			transport.mu.Unlock()
			return
		}
		var req SDKControlRequest
		_ = json.Unmarshal(transport.writtenData[0], &req)
		transport.mu.Unlock()

		// Inject matching response
		transport.injectResponse(req.RequestID, map[string]any{"status": "ok"})
	}()

	// Send control request
	result, err := protocol.SendControlRequest(ctx, InterruptRequest{Subtype: SubtypeInterrupt}, 5*time.Second)
	assertControlNoError(t, err)

	// Verify response was received
	if result == nil {
		t.Fatal("expected response, got nil")
	}
}

func testUnknownRequestIDIgnored(t *testing.T) {
	t.Helper()

	ctx, cancel := setupControlTestContext(t, 2*time.Second)
	defer cancel()

	transport := newControlMockTransport()
	protocol := NewProtocol(transport)

	err := protocol.Start(ctx)
	assertControlNoError(t, err)
	defer func() { _ = protocol.Close() }()

	// Inject response with unknown request ID
	transport.injectResponse("req_unknown_12345678", map[string]any{"status": "ok"})

	// Give time for message to be processed
	time.Sleep(100 * time.Millisecond)

	// Protocol should still be running (no panic or error)
	if protocol.IsClosed() {
		t.Error("protocol should not be closed after unknown response")
	}
}

func TestRequestTimeout(t *testing.T) {
	t.Run("timeout_after_duration", testTimeoutAfterDuration)
}

func testTimeoutAfterDuration(t *testing.T) {
	t.Helper()

	ctx, cancel := setupControlTestContext(t, 5*time.Second)
	defer cancel()

	transport := newControlMockTransport()
	protocol := NewProtocol(transport)

	err := protocol.Start(ctx)
	assertControlNoError(t, err)
	defer func() { _ = protocol.Close() }()

	// Send request with short timeout (no response will be sent)
	start := time.Now()
	_, err = protocol.SendControlRequest(ctx, InterruptRequest{Subtype: SubtypeInterrupt}, 100*time.Millisecond)
	duration := time.Since(start)

	// Should get timeout error
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}

	// Duration should be approximately 100ms
	if duration < 90*time.Millisecond || duration > 200*time.Millisecond {
		t.Errorf("timeout duration should be ~100ms, got %v", duration)
	}
}

func TestConcurrentRequests(t *testing.T) {
	t.Run("thread_safe_concurrent_requests", testThreadSafeConcurrentRequests)
}

func testThreadSafeConcurrentRequests(t *testing.T) {
	t.Helper()

	ctx, cancel := setupControlTestContext(t, 10*time.Second)
	defer cancel()

	transport := newControlMockTransport()
	protocol := NewProtocol(transport)

	err := protocol.Start(ctx)
	assertControlNoError(t, err)
	defer func() { _ = protocol.Close() }()

	const numGoroutines = 10
	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines)

	// Auto-respond to all requests
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				transport.mu.Lock()
				for i, data := range transport.writtenData {
					if transport.responded[i] {
						continue
					}
					var req SDKControlRequest
					if err := json.Unmarshal(data, &req); err == nil {
						transport.responded[i] = true
						transport.mu.Unlock()
						transport.injectResponse(req.RequestID, map[string]any{"status": "ok"})
						transport.mu.Lock()
					}
				}
				transport.mu.Unlock()
				time.Sleep(10 * time.Millisecond)
			}
		}
	}()

	// Launch concurrent requests
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := protocol.SendControlRequest(ctx, InterruptRequest{Subtype: SubtypeInterrupt}, 5*time.Second)
			if err != nil {
				errors <- err
			}
		}()
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("concurrent request error: %v", err)
	}
}

// =============================================================================
// Phase 3: Initialize Handshake Tests
// =============================================================================

func TestInitializeHandshake(t *testing.T) {
	t.Run("success", testInitializeSuccess)
	t.Run("timeout", testInitializeTimeout)
	t.Run("error_response", testInitializeErrorResponse)
	t.Run("cached_result", testInitializeCachedResult)
}

func testInitializeSuccess(t *testing.T) {
	t.Helper()

	ctx, cancel := setupControlTestContext(t, 5*time.Second)
	defer cancel()

	transport := newControlMockTransport()
	protocol := NewProtocol(transport)

	err := protocol.Start(ctx)
	assertControlNoError(t, err)
	defer func() { _ = protocol.Close() }()

	// Auto-respond to initialize request
	go func() {
		time.Sleep(50 * time.Millisecond)
		transport.mu.Lock()
		if len(transport.writtenData) > 0 {
			var req SDKControlRequest
			if err := json.Unmarshal(transport.writtenData[0], &req); err == nil {
				transport.mu.Unlock()
				transport.injectResponse(req.RequestID, map[string]any{
					"supported_commands": []string{"interrupt", "initialize"},
				})
				return
			}
		}
		transport.mu.Unlock()
	}()

	resp, err := protocol.Initialize(ctx)
	assertControlNoError(t, err)

	if resp == nil {
		t.Fatal("expected initialize response, got nil")
	}
}

func testInitializeTimeout(t *testing.T) {
	t.Helper()

	ctx, cancel := setupControlTestContext(t, 5*time.Second)
	defer cancel()

	transport := newControlMockTransport()
	// Use short init timeout for test
	protocol := NewProtocol(transport, WithInitTimeout(100*time.Millisecond))

	err := protocol.Start(ctx)
	assertControlNoError(t, err)
	defer func() { _ = protocol.Close() }()

	// Don't send any response - should timeout
	_, err = protocol.Initialize(ctx)

	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

func testInitializeErrorResponse(t *testing.T) {
	t.Helper()

	ctx, cancel := setupControlTestContext(t, 5*time.Second)
	defer cancel()

	transport := newControlMockTransport()
	protocol := NewProtocol(transport)

	err := protocol.Start(ctx)
	assertControlNoError(t, err)
	defer func() { _ = protocol.Close() }()

	// Respond with error
	go func() {
		time.Sleep(50 * time.Millisecond)
		transport.mu.Lock()
		if len(transport.writtenData) > 0 {
			var req SDKControlRequest
			if err := json.Unmarshal(transport.writtenData[0], &req); err == nil {
				transport.mu.Unlock()
				transport.injectErrorResponse(req.RequestID, "initialization failed")
				return
			}
		}
		transport.mu.Unlock()
	}()

	_, err = protocol.Initialize(ctx)

	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func testInitializeCachedResult(t *testing.T) {
	t.Helper()

	ctx, cancel := setupControlTestContext(t, 5*time.Second)
	defer cancel()

	transport := newControlMockTransport()
	protocol := NewProtocol(transport)

	err := protocol.Start(ctx)
	assertControlNoError(t, err)
	defer func() { _ = protocol.Close() }()

	// Respond to first initialize
	go func() {
		time.Sleep(50 * time.Millisecond)
		transport.mu.Lock()
		if len(transport.writtenData) > 0 {
			var req SDKControlRequest
			if err := json.Unmarshal(transport.writtenData[0], &req); err == nil {
				transport.mu.Unlock()
				transport.injectResponse(req.RequestID, map[string]any{
					"supported_commands": []string{"interrupt"},
				})
				return
			}
		}
		transport.mu.Unlock()
	}()

	resp1, err := protocol.Initialize(ctx)
	assertControlNoError(t, err)

	// Second call should return cached result without sending request
	initialWriteCount := transport.getWriteCount()
	resp2, err := protocol.Initialize(ctx)
	assertControlNoError(t, err)

	// Should be same instance (cached)
	if resp1 != resp2 {
		t.Error("expected cached result, got different instance")
	}

	// Should not have sent another request
	if transport.getWriteCount() != initialWriteCount {
		t.Error("should not have sent another initialize request")
	}
}

// =============================================================================
// Phase 4: Message Routing Tests
// =============================================================================

func TestMessageRouting(t *testing.T) {
	t.Run("route_control_response", testRouteControlResponse)
	t.Run("route_regular_message", testRouteRegularMessage)
	t.Run("route_unknown_type", testRouteUnknownType)
}

func testRouteControlResponse(t *testing.T) {
	t.Helper()

	ctx, cancel := setupControlTestContext(t, 5*time.Second)
	defer cancel()

	transport := newControlMockTransport()
	protocol := NewProtocol(transport)

	err := protocol.Start(ctx)
	assertControlNoError(t, err)
	defer func() { _ = protocol.Close() }()

	// Set up pending request
	requestID := "req_test_12345678"
	responseChan := make(chan *Response, 1)
	protocol.setPendingRequest(requestID, responseChan)

	// Route a control response
	msg := map[string]any{
		"type": MessageTypeControlResponse,
		"response": map[string]any{
			"subtype":    ResponseSubtypeSuccess,
			"request_id": requestID,
			"response":   map[string]any{"status": "ok"},
		},
	}

	err = protocol.HandleIncomingMessage(ctx, msg)
	assertControlNoError(t, err)

	// Verify response was routed to channel
	select {
	case resp := <-responseChan:
		if resp == nil {
			t.Fatal("expected response, got nil")
			return
		}
		assertControlEqual(t, ResponseSubtypeSuccess, resp.Subtype)
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for response")
	}
}

func testRouteRegularMessage(t *testing.T) {
	t.Helper()

	ctx, cancel := setupControlTestContext(t, 5*time.Second)
	defer cancel()

	transport := newControlMockTransport()
	protocol := NewProtocol(transport)

	err := protocol.Start(ctx)
	assertControlNoError(t, err)
	defer func() { _ = protocol.Close() }()

	// Route a regular message (user message)
	msg := map[string]any{
		"type": "user",
		"message": map[string]any{
			"content": "hello",
		},
	}

	err = protocol.HandleIncomingMessage(ctx, msg)
	assertControlNoError(t, err)

	// Verify message was forwarded to message stream
	select {
	case received := <-protocol.ReceiveMessages():
		if received["type"] != "user" {
			t.Errorf("expected user message, got %v", received["type"])
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for message")
	}
}

func testRouteUnknownType(t *testing.T) {
	t.Helper()

	ctx, cancel := setupControlTestContext(t, 5*time.Second)
	defer cancel()

	transport := newControlMockTransport()
	protocol := NewProtocol(transport)

	err := protocol.Start(ctx)
	assertControlNoError(t, err)
	defer func() { _ = protocol.Close() }()

	// Route an unknown message type
	msg := map[string]any{
		"type": "unknown_future_type",
		"data": "some data",
	}

	err = protocol.HandleIncomingMessage(ctx, msg)
	assertControlNoError(t, err)

	// Unknown messages should be forwarded to stream (forward compatibility)
	select {
	case received := <-protocol.ReceiveMessages():
		if received["type"] != "unknown_future_type" {
			t.Errorf("expected unknown_future_type, got %v", received["type"])
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for message")
	}
}

// =============================================================================
// Phase 6: Interrupt via Protocol Tests
// =============================================================================

func TestInterruptViaProtocol(t *testing.T) {
	t.Run("sends_interrupt_request", testInterruptSendsRequest)
}

func testInterruptSendsRequest(t *testing.T) {
	t.Helper()

	ctx, cancel := setupControlTestContext(t, 5*time.Second)
	defer cancel()

	transport := newControlMockTransport()
	protocol := NewProtocol(transport)

	err := protocol.Start(ctx)
	assertControlNoError(t, err)
	defer func() { _ = protocol.Close() }()

	// Auto-respond to interrupt request
	go func() {
		time.Sleep(50 * time.Millisecond)
		transport.mu.Lock()
		if len(transport.writtenData) > 0 {
			var req SDKControlRequest
			if err := json.Unmarshal(transport.writtenData[0], &req); err == nil {
				transport.mu.Unlock()
				transport.injectResponse(req.RequestID, nil)
				return
			}
		}
		transport.mu.Unlock()
	}()

	err = protocol.Interrupt(ctx)
	assertControlNoError(t, err)

	// Verify interrupt request was sent
	transport.mu.Lock()
	defer transport.mu.Unlock()

	if len(transport.writtenData) == 0 {
		t.Fatal("expected interrupt request to be sent")
	}

	var req SDKControlRequest
	err = json.Unmarshal(transport.writtenData[0], &req)
	assertControlNoError(t, err)

	// Verify it's an interrupt request
	request, ok := req.Request.(map[string]any)
	if !ok {
		t.Fatal("request should be a map")
	}
	assertControlEqual(t, SubtypeInterrupt, request["subtype"])
}

// =============================================================================
// Mock Transport for Control Protocol Tests
// =============================================================================

type controlMockTransport struct {
	mu          sync.Mutex
	writtenData [][]byte
	responded   []bool
	readChan    chan []byte
	writeErr    error
	closed      bool
}

func newControlMockTransport() *controlMockTransport {
	return &controlMockTransport{
		readChan:  make(chan []byte, 100),
		responded: make([]bool, 0),
	}
}

func (m *controlMockTransport) Write(_ context.Context, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.writeErr != nil {
		return m.writeErr
	}

	// Make a copy of the data
	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)
	m.writtenData = append(m.writtenData, dataCopy)
	m.responded = append(m.responded, false)
	return nil
}

func (m *controlMockTransport) Read(_ context.Context) <-chan []byte {
	return m.readChan
}

func (m *controlMockTransport) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.closed {
		m.closed = true
		close(m.readChan)
	}
	return nil
}

func (m *controlMockTransport) injectResponse(requestID string, response any) {
	resp := SDKControlResponse{
		Type: MessageTypeControlResponse,
		Response: Response{
			Subtype:   ResponseSubtypeSuccess,
			RequestID: requestID,
			Response:  response,
		},
	}
	data, _ := json.Marshal(resp)
	m.readChan <- data
}

func (m *controlMockTransport) injectErrorResponse(requestID string, errorMsg string) {
	resp := SDKControlResponse{
		Type: MessageTypeControlResponse,
		Response: Response{
			Subtype:   ResponseSubtypeError,
			RequestID: requestID,
			Error:     errorMsg,
		},
	}
	data, _ := json.Marshal(resp)
	m.readChan <- data
}

func (m *controlMockTransport) getWriteCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.writtenData)
}

// =============================================================================
// Test Helpers
// =============================================================================

func setupControlTestContext(t *testing.T, timeout time.Duration) (context.Context, context.CancelFunc) {
	t.Helper()
	return context.WithTimeout(context.Background(), timeout)
}

func assertControlNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func assertControlEqual(t *testing.T, expected, actual any) {
	t.Helper()
	if expected != actual {
		t.Errorf("expected %v, got %v", expected, actual)
	}
}

// =============================================================================
// Phase 7: Dynamic Control Methods Tests (SetModel, SetPermissionMode)
// =============================================================================

func TestDynamicControlMethods(t *testing.T) {
	t.Run("set_model", testSetModel)
	t.Run("set_permission_mode", testSetPermissionMode)
}

func testSetModel(t *testing.T) {
	t.Run("success", testSetModelSuccess)
	t.Run("with_nil_resets_default", testSetModelWithNil)
	t.Run("error_response", testSetModelError)
	t.Run("timeout", testSetModelTimeout)
}

func testSetModelSuccess(t *testing.T) {
	t.Helper()

	ctx, cancel := setupControlTestContext(t, 5*time.Second)
	defer cancel()

	transport := newControlMockTransport()
	protocol := NewProtocol(transport)

	err := protocol.Start(ctx)
	assertControlNoError(t, err)
	defer func() { _ = protocol.Close() }()

	// Auto-respond to set_model request
	go func() {
		time.Sleep(50 * time.Millisecond)
		transport.mu.Lock()
		if len(transport.writtenData) > 0 {
			var req SDKControlRequest
			if err := json.Unmarshal(transport.writtenData[0], &req); err == nil {
				transport.mu.Unlock()
				transport.injectResponse(req.RequestID, nil)
				return
			}
		}
		transport.mu.Unlock()
	}()

	model := testModelSonnet
	err = protocol.SetModel(ctx, &model)
	assertControlNoError(t, err)

	// Verify set_model request was sent with correct structure
	transport.mu.Lock()
	defer transport.mu.Unlock()

	if len(transport.writtenData) == 0 {
		t.Fatal("expected set_model request to be sent")
	}

	var req SDKControlRequest
	err = json.Unmarshal(transport.writtenData[0], &req)
	assertControlNoError(t, err)

	request, ok := req.Request.(map[string]any)
	if !ok {
		t.Fatal("request should be a map")
	}
	assertControlEqual(t, SubtypeSetModel, request["subtype"])
	assertControlEqual(t, testModelSonnet, request["model"])
}

func testSetModelWithNil(t *testing.T) {
	t.Helper()

	ctx, cancel := setupControlTestContext(t, 5*time.Second)
	defer cancel()

	transport := newControlMockTransport()
	protocol := NewProtocol(transport)

	err := protocol.Start(ctx)
	assertControlNoError(t, err)
	defer func() { _ = protocol.Close() }()

	// Auto-respond to set_model request
	go func() {
		time.Sleep(50 * time.Millisecond)
		transport.mu.Lock()
		if len(transport.writtenData) > 0 {
			var req SDKControlRequest
			if err := json.Unmarshal(transport.writtenData[0], &req); err == nil {
				transport.mu.Unlock()
				transport.injectResponse(req.RequestID, nil)
				return
			}
		}
		transport.mu.Unlock()
	}()

	// Pass nil to reset to default model
	err = protocol.SetModel(ctx, nil)
	assertControlNoError(t, err)

	// Verify set_model request was sent with null model
	transport.mu.Lock()
	defer transport.mu.Unlock()

	if len(transport.writtenData) == 0 {
		t.Fatal("expected set_model request to be sent")
	}

	var req SDKControlRequest
	err = json.Unmarshal(transport.writtenData[0], &req)
	assertControlNoError(t, err)

	request, ok := req.Request.(map[string]any)
	if !ok {
		t.Fatal("request should be a map")
	}
	assertControlEqual(t, SubtypeSetModel, request["subtype"])
	// model should be nil/null
	if request["model"] != nil {
		t.Errorf("expected model to be nil, got %v", request["model"])
	}
}

func testSetModelError(t *testing.T) {
	t.Helper()

	ctx, cancel := setupControlTestContext(t, 5*time.Second)
	defer cancel()

	transport := newControlMockTransport()
	protocol := NewProtocol(transport)

	err := protocol.Start(ctx)
	assertControlNoError(t, err)
	defer func() { _ = protocol.Close() }()

	// Respond with error
	go func() {
		time.Sleep(50 * time.Millisecond)
		transport.mu.Lock()
		if len(transport.writtenData) > 0 {
			var req SDKControlRequest
			if err := json.Unmarshal(transport.writtenData[0], &req); err == nil {
				transport.mu.Unlock()
				transport.injectErrorResponse(req.RequestID, "invalid model")
				return
			}
		}
		transport.mu.Unlock()
	}()

	model := "invalid-model"
	err = protocol.SetModel(ctx, &model)

	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func testSetModelTimeout(t *testing.T) {
	t.Helper()

	ctx, cancel := setupControlTestContext(t, 5*time.Second)
	defer cancel()

	transport := newControlMockTransport()
	protocol := NewProtocol(transport)

	err := protocol.Start(ctx)
	assertControlNoError(t, err)
	defer func() { _ = protocol.Close() }()

	// Don't send any response - should timeout
	// Note: SetModel uses 5-second timeout internally, but we test with shorter context
	shortCtx, shortCancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer shortCancel()

	model := testModelSonnet
	err = protocol.SetModel(shortCtx, &model)

	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

func testSetPermissionMode(t *testing.T) {
	t.Run("success", testSetPermissionModeSuccess)
	t.Run("error_response", testSetPermissionModeError)
	t.Run("timeout", testSetPermissionModeTimeout)
}

func testSetPermissionModeSuccess(t *testing.T) {
	t.Helper()

	ctx, cancel := setupControlTestContext(t, 5*time.Second)
	defer cancel()

	transport := newControlMockTransport()
	protocol := NewProtocol(transport)

	err := protocol.Start(ctx)
	assertControlNoError(t, err)
	defer func() { _ = protocol.Close() }()

	// Auto-respond to set_permission_mode request
	go func() {
		time.Sleep(50 * time.Millisecond)
		transport.mu.Lock()
		if len(transport.writtenData) > 0 {
			var req SDKControlRequest
			if err := json.Unmarshal(transport.writtenData[0], &req); err == nil {
				transport.mu.Unlock()
				transport.injectResponse(req.RequestID, nil)
				return
			}
		}
		transport.mu.Unlock()
	}()

	err = protocol.SetPermissionMode(ctx, "accept_edits")
	assertControlNoError(t, err)

	// Verify set_permission_mode request was sent with correct structure
	transport.mu.Lock()
	defer transport.mu.Unlock()

	if len(transport.writtenData) == 0 {
		t.Fatal("expected set_permission_mode request to be sent")
	}

	var req SDKControlRequest
	err = json.Unmarshal(transport.writtenData[0], &req)
	assertControlNoError(t, err)

	request, ok := req.Request.(map[string]any)
	if !ok {
		t.Fatal("request should be a map")
	}
	assertControlEqual(t, SubtypeSetPermissionMode, request["subtype"])
	assertControlEqual(t, "accept_edits", request["mode"])
}

func testSetPermissionModeError(t *testing.T) {
	t.Helper()

	ctx, cancel := setupControlTestContext(t, 5*time.Second)
	defer cancel()

	transport := newControlMockTransport()
	protocol := NewProtocol(transport)

	err := protocol.Start(ctx)
	assertControlNoError(t, err)
	defer func() { _ = protocol.Close() }()

	// Respond with error
	go func() {
		time.Sleep(50 * time.Millisecond)
		transport.mu.Lock()
		if len(transport.writtenData) > 0 {
			var req SDKControlRequest
			if err := json.Unmarshal(transport.writtenData[0], &req); err == nil {
				transport.mu.Unlock()
				transport.injectErrorResponse(req.RequestID, "invalid permission mode")
				return
			}
		}
		transport.mu.Unlock()
	}()

	err = protocol.SetPermissionMode(ctx, "invalid_mode")

	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func testSetPermissionModeTimeout(t *testing.T) {
	t.Helper()

	ctx, cancel := setupControlTestContext(t, 5*time.Second)
	defer cancel()

	transport := newControlMockTransport()
	protocol := NewProtocol(transport)

	err := protocol.Start(ctx)
	assertControlNoError(t, err)
	defer func() { _ = protocol.Close() }()

	// Don't send any response - should timeout
	shortCtx, shortCancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer shortCancel()

	err = protocol.SetPermissionMode(shortCtx, "accept_edits")

	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

// TestSetModelRequestSerialization tests the JSON serialization of SetModelRequest
func TestSetModelRequestSerialization(t *testing.T) {
	t.Run("marshal_with_model", testMarshalSetModelWithModel)
	t.Run("marshal_with_nil", testMarshalSetModelWithNil)
}

func testMarshalSetModelWithModel(t *testing.T) {
	t.Helper()

	model := testModelSonnet
	req := SDKControlRequest{
		Type:      MessageTypeControlRequest,
		RequestID: "req_1_abc123",
		Request: SetModelRequest{
			Subtype: SubtypeSetModel,
			Model:   &model,
		},
	}

	data, err := json.Marshal(req)
	assertControlNoError(t, err)

	var parsed map[string]any
	err = json.Unmarshal(data, &parsed)
	assertControlNoError(t, err)

	assertControlEqual(t, "control_request", parsed["type"])
	assertControlEqual(t, "req_1_abc123", parsed["request_id"])

	request, ok := parsed["request"].(map[string]any)
	if !ok {
		t.Fatal("request field should be an object")
	}
	assertControlEqual(t, "set_model", request["subtype"])
	assertControlEqual(t, testModelSonnet, request["model"])
}

func testMarshalSetModelWithNil(t *testing.T) {
	t.Helper()

	req := SDKControlRequest{
		Type:      MessageTypeControlRequest,
		RequestID: "req_2_def456",
		Request: SetModelRequest{
			Subtype: SubtypeSetModel,
			Model:   nil,
		},
	}

	data, err := json.Marshal(req)
	assertControlNoError(t, err)

	var parsed map[string]any
	err = json.Unmarshal(data, &parsed)
	assertControlNoError(t, err)

	request, ok := parsed["request"].(map[string]any)
	if !ok {
		t.Fatal("request field should be an object")
	}
	assertControlEqual(t, "set_model", request["subtype"])
	// Model should be nil/null when not specified
	if request["model"] != nil {
		t.Errorf("expected model to be nil, got %v", request["model"])
	}
}

// =============================================================================
// Phase 8: Permission Callback Tests (Issue #8)
// =============================================================================

func TestPermissionCallback(t *testing.T) {
	t.Run("allow_callback", testPermissionAllowCallback)
	t.Run("deny_callback", testPermissionDenyCallback)
	t.Run("deny_with_interrupt", testPermissionDenyWithInterrupt)
	t.Run("allow_with_updated_input", testPermissionAllowWithUpdatedInput)
	t.Run("allow_with_updated_permissions", testPermissionAllowWithUpdatedPermissions)
	t.Run("callback_error", testPermissionCallbackError)
	t.Run("no_callback_registered", testPermissionNoCallbackRegistered)
	t.Run("callback_panic_recovery", testPermissionCallbackPanicRecovery)
}

func testPermissionAllowCallback(t *testing.T) {
	t.Helper()

	ctx, cancel := setupControlTestContext(t, 5*time.Second)
	defer cancel()

	transport := newControlMockTransport()

	// Create callback that allows all tools
	callback := func(_ context.Context, toolName string, _ map[string]any, _ ToolPermissionContext) (PermissionResult, error) {
		if toolName != "Read" {
			t.Errorf("expected tool name 'Read', got '%s'", toolName)
		}
		return NewPermissionResultAllow(), nil
	}

	protocol := NewProtocol(transport, WithCanUseToolCallback(callback))

	err := protocol.Start(ctx)
	assertControlNoError(t, err)
	defer func() { _ = protocol.Close() }()

	// Simulate incoming can_use_tool request from CLI
	request := map[string]any{
		"type":       MessageTypeControlRequest,
		"request_id": "req_perm_1",
		"request": map[string]any{
			"subtype":   SubtypeCanUseTool,
			"tool_name": "Read",
			"input":     map[string]any{"file_path": "/tmp/test.txt"},
		},
	}

	err = protocol.HandleIncomingMessage(ctx, request)
	assertControlNoError(t, err)

	// Verify response was sent
	transport.mu.Lock()
	defer transport.mu.Unlock()

	if len(transport.writtenData) == 0 {
		t.Fatal("expected permission response to be sent")
	}

	var resp SDKControlResponse
	err = json.Unmarshal(transport.writtenData[0], &resp)
	assertControlNoError(t, err)

	assertControlEqual(t, MessageTypeControlResponse, resp.Type)
	assertControlEqual(t, ResponseSubtypeSuccess, resp.Response.Subtype)
	assertControlEqual(t, "req_perm_1", resp.Response.RequestID)

	// Verify response content has behavior: allow
	respData, ok := resp.Response.Response.(map[string]any)
	if !ok {
		t.Fatal("response should be a map")
	}
	assertControlEqual(t, "allow", respData["behavior"])
}

func testPermissionDenyCallback(t *testing.T) {
	t.Helper()

	ctx, cancel := setupControlTestContext(t, 5*time.Second)
	defer cancel()

	transport := newControlMockTransport()

	// Create callback that denies with a message
	callback := func(_ context.Context, _ string, _ map[string]any, _ ToolPermissionContext) (PermissionResult, error) {
		return NewPermissionResultDeny("tool not allowed"), nil
	}

	protocol := NewProtocol(transport, WithCanUseToolCallback(callback))

	err := protocol.Start(ctx)
	assertControlNoError(t, err)
	defer func() { _ = protocol.Close() }()

	// Simulate incoming can_use_tool request
	request := map[string]any{
		"type":       MessageTypeControlRequest,
		"request_id": "req_perm_2",
		"request": map[string]any{
			"subtype":   SubtypeCanUseTool,
			"tool_name": "Write",
			"input":     map[string]any{},
		},
	}

	err = protocol.HandleIncomingMessage(ctx, request)
	assertControlNoError(t, err)

	// Verify response was sent with deny
	transport.mu.Lock()
	defer transport.mu.Unlock()

	if len(transport.writtenData) == 0 {
		t.Fatal("expected permission response to be sent")
	}

	var resp SDKControlResponse
	err = json.Unmarshal(transport.writtenData[0], &resp)
	assertControlNoError(t, err)

	respData, ok := resp.Response.Response.(map[string]any)
	if !ok {
		t.Fatal("response should be a map")
	}
	assertControlEqual(t, "deny", respData["behavior"])
	assertControlEqual(t, "tool not allowed", respData["message"])
}

func testPermissionDenyWithInterrupt(t *testing.T) {
	t.Helper()

	ctx, cancel := setupControlTestContext(t, 5*time.Second)
	defer cancel()

	transport := newControlMockTransport()

	// Create callback that denies with interrupt flag
	callback := func(_ context.Context, _ string, _ map[string]any, _ ToolPermissionContext) (PermissionResult, error) {
		return PermissionResultDeny{
			Behavior:  "deny",
			Message:   "dangerous operation blocked",
			Interrupt: true,
		}, nil
	}

	protocol := NewProtocol(transport, WithCanUseToolCallback(callback))

	err := protocol.Start(ctx)
	assertControlNoError(t, err)
	defer func() { _ = protocol.Close() }()

	request := map[string]any{
		"type":       MessageTypeControlRequest,
		"request_id": "req_perm_3",
		"request": map[string]any{
			"subtype":   SubtypeCanUseTool,
			"tool_name": "Bash",
			"input":     map[string]any{"command": "rm -rf /"},
		},
	}

	err = protocol.HandleIncomingMessage(ctx, request)
	assertControlNoError(t, err)

	transport.mu.Lock()
	defer transport.mu.Unlock()

	var resp SDKControlResponse
	err = json.Unmarshal(transport.writtenData[0], &resp)
	assertControlNoError(t, err)

	respData, ok := resp.Response.Response.(map[string]any)
	if !ok {
		t.Fatal("response should be a map")
	}
	assertControlEqual(t, "deny", respData["behavior"])
	assertControlEqual(t, true, respData["interrupt"])
}

func testPermissionAllowWithUpdatedInput(t *testing.T) {
	t.Helper()

	ctx, cancel := setupControlTestContext(t, 5*time.Second)
	defer cancel()

	transport := newControlMockTransport()

	// Create callback that modifies the input
	callback := func(_ context.Context, _ string, input map[string]any, _ ToolPermissionContext) (PermissionResult, error) {
		// Modify the file path to a safe location
		modifiedInput := make(map[string]any)
		for k, v := range input {
			modifiedInput[k] = v
		}
		modifiedInput["file_path"] = "/tmp/safe/test.txt"

		return PermissionResultAllow{
			Behavior:     "allow",
			UpdatedInput: modifiedInput,
		}, nil
	}

	protocol := NewProtocol(transport, WithCanUseToolCallback(callback))

	err := protocol.Start(ctx)
	assertControlNoError(t, err)
	defer func() { _ = protocol.Close() }()

	request := map[string]any{
		"type":       MessageTypeControlRequest,
		"request_id": "req_perm_4",
		"request": map[string]any{
			"subtype":   SubtypeCanUseTool,
			"tool_name": "Write",
			"input":     map[string]any{"file_path": "/etc/passwd", "content": "test"},
		},
	}

	err = protocol.HandleIncomingMessage(ctx, request)
	assertControlNoError(t, err)

	transport.mu.Lock()
	defer transport.mu.Unlock()

	var resp SDKControlResponse
	err = json.Unmarshal(transport.writtenData[0], &resp)
	assertControlNoError(t, err)

	respData, ok := resp.Response.Response.(map[string]any)
	if !ok {
		t.Fatal("response should be a map")
	}
	assertControlEqual(t, "allow", respData["behavior"])

	updatedInput, ok := respData["updatedInput"].(map[string]any)
	if !ok {
		t.Fatal("updatedInput should be a map")
	}
	assertControlEqual(t, "/tmp/safe/test.txt", updatedInput["file_path"])
}

func testPermissionAllowWithUpdatedPermissions(t *testing.T) {
	t.Helper()

	ctx, cancel := setupControlTestContext(t, 5*time.Second)
	defer cancel()

	transport := newControlMockTransport()

	// Create callback that returns updated permissions
	callback := func(_ context.Context, _ string, _ map[string]any, _ ToolPermissionContext) (PermissionResult, error) {
		return PermissionResultAllow{
			Behavior: "allow",
			UpdatedPermissions: []PermissionUpdate{
				{
					Type: PermissionUpdateTypeAddRules,
					Rules: []PermissionRuleValue{
						{ToolName: "Read", RuleContent: ptrString("/tmp/*")},
					},
				},
			},
		}, nil
	}

	protocol := NewProtocol(transport, WithCanUseToolCallback(callback))

	err := protocol.Start(ctx)
	assertControlNoError(t, err)
	defer func() { _ = protocol.Close() }()

	request := map[string]any{
		"type":       MessageTypeControlRequest,
		"request_id": "req_perm_5",
		"request": map[string]any{
			"subtype":   SubtypeCanUseTool,
			"tool_name": "Read",
			"input":     map[string]any{},
		},
	}

	err = protocol.HandleIncomingMessage(ctx, request)
	assertControlNoError(t, err)

	transport.mu.Lock()
	defer transport.mu.Unlock()

	var resp SDKControlResponse
	err = json.Unmarshal(transport.writtenData[0], &resp)
	assertControlNoError(t, err)

	respData, ok := resp.Response.Response.(map[string]any)
	if !ok {
		t.Fatal("response should be a map")
	}
	assertControlEqual(t, "allow", respData["behavior"])

	// Verify updatedPermissions is present
	updatedPerms, ok := respData["updatedPermissions"].([]any)
	if !ok {
		t.Fatal("updatedPermissions should be an array")
	}
	if len(updatedPerms) != 1 {
		t.Fatalf("expected 1 permission update, got %d", len(updatedPerms))
	}
}

func testPermissionCallbackError(t *testing.T) {
	t.Helper()

	ctx, cancel := setupControlTestContext(t, 5*time.Second)
	defer cancel()

	transport := newControlMockTransport()

	// Create callback that returns an error
	callback := func(_ context.Context, _ string, _ map[string]any, _ ToolPermissionContext) (PermissionResult, error) {
		return nil, fmt.Errorf("callback error: database connection failed")
	}

	protocol := NewProtocol(transport, WithCanUseToolCallback(callback))

	err := protocol.Start(ctx)
	assertControlNoError(t, err)
	defer func() { _ = protocol.Close() }()

	request := map[string]any{
		"type":       MessageTypeControlRequest,
		"request_id": "req_perm_6",
		"request": map[string]any{
			"subtype":   SubtypeCanUseTool,
			"tool_name": "Query",
			"input":     map[string]any{},
		},
	}

	err = protocol.HandleIncomingMessage(ctx, request)
	assertControlNoError(t, err)

	transport.mu.Lock()
	defer transport.mu.Unlock()

	var resp SDKControlResponse
	err = json.Unmarshal(transport.writtenData[0], &resp)
	assertControlNoError(t, err)

	// Should be an error response
	assertControlEqual(t, ResponseSubtypeError, resp.Response.Subtype)
	if resp.Response.Error == "" {
		t.Error("expected error message in response")
	}
}

func testPermissionNoCallbackRegistered(t *testing.T) {
	t.Helper()

	ctx, cancel := setupControlTestContext(t, 5*time.Second)
	defer cancel()

	transport := newControlMockTransport()

	// Create protocol WITHOUT a callback
	protocol := NewProtocol(transport)

	err := protocol.Start(ctx)
	assertControlNoError(t, err)
	defer func() { _ = protocol.Close() }()

	request := map[string]any{
		"type":       MessageTypeControlRequest,
		"request_id": "req_perm_7",
		"request": map[string]any{
			"subtype":   SubtypeCanUseTool,
			"tool_name": "AnyTool",
			"input":     map[string]any{},
		},
	}

	err = protocol.HandleIncomingMessage(ctx, request)
	assertControlNoError(t, err)

	transport.mu.Lock()
	defer transport.mu.Unlock()

	var resp SDKControlResponse
	err = json.Unmarshal(transport.writtenData[0], &resp)
	assertControlNoError(t, err)

	// Should deny by default (secure default)
	respData, ok := resp.Response.Response.(map[string]any)
	if !ok {
		t.Fatal("response should be a map")
	}
	assertControlEqual(t, "deny", respData["behavior"])
}

func testPermissionCallbackPanicRecovery(t *testing.T) {
	t.Helper()

	ctx, cancel := setupControlTestContext(t, 5*time.Second)
	defer cancel()

	transport := newControlMockTransport()

	// Create callback that panics
	callback := func(_ context.Context, _ string, _ map[string]any, _ ToolPermissionContext) (PermissionResult, error) {
		panic("unexpected panic in callback")
	}

	protocol := NewProtocol(transport, WithCanUseToolCallback(callback))

	err := protocol.Start(ctx)
	assertControlNoError(t, err)
	defer func() { _ = protocol.Close() }()

	request := map[string]any{
		"type":       MessageTypeControlRequest,
		"request_id": "req_perm_8",
		"request": map[string]any{
			"subtype":   SubtypeCanUseTool,
			"tool_name": "PanicTool",
			"input":     map[string]any{},
		},
	}

	// Should not panic - should recover and return error response
	err = protocol.HandleIncomingMessage(ctx, request)
	assertControlNoError(t, err)

	transport.mu.Lock()
	defer transport.mu.Unlock()

	var resp SDKControlResponse
	err = json.Unmarshal(transport.writtenData[0], &resp)
	assertControlNoError(t, err)

	// Should be an error response due to panic
	assertControlEqual(t, ResponseSubtypeError, resp.Response.Subtype)
}

// TestPermissionTypeSerialization tests JSON serialization of permission types.
func TestPermissionTypeSerialization(t *testing.T) {
	t.Run("marshal_allow_result", testMarshalPermissionAllowResult)
	t.Run("marshal_deny_result", testMarshalPermissionDenyResult)
	t.Run("marshal_permission_update", testMarshalPermissionUpdate)
	t.Run("marshal_permission_rule_value", testMarshalPermissionRuleValue)
}

func testMarshalPermissionAllowResult(t *testing.T) {
	t.Helper()

	result := PermissionResultAllow{
		Behavior:     "allow",
		UpdatedInput: map[string]any{"file_path": "/safe/path"},
	}

	data, err := json.Marshal(result)
	assertControlNoError(t, err)

	var parsed map[string]any
	err = json.Unmarshal(data, &parsed)
	assertControlNoError(t, err)

	assertControlEqual(t, "allow", parsed["behavior"])

	// Verify camelCase field name
	if _, ok := parsed["updatedInput"]; !ok {
		t.Error("expected 'updatedInput' field (camelCase)")
	}
}

func testMarshalPermissionDenyResult(t *testing.T) {
	t.Helper()

	result := PermissionResultDeny{
		Behavior:  "deny",
		Message:   "not allowed",
		Interrupt: true,
	}

	data, err := json.Marshal(result)
	assertControlNoError(t, err)

	var parsed map[string]any
	err = json.Unmarshal(data, &parsed)
	assertControlNoError(t, err)

	assertControlEqual(t, "deny", parsed["behavior"])
	assertControlEqual(t, "not allowed", parsed["message"])
	assertControlEqual(t, true, parsed["interrupt"])
}

func testMarshalPermissionUpdate(t *testing.T) {
	t.Helper()

	update := PermissionUpdate{
		Type: PermissionUpdateTypeAddRules,
		Rules: []PermissionRuleValue{
			{ToolName: "Read", RuleContent: ptrString("/tmp/*")},
		},
	}

	data, err := json.Marshal(update)
	assertControlNoError(t, err)

	var parsed map[string]any
	err = json.Unmarshal(data, &parsed)
	assertControlNoError(t, err)

	assertControlEqual(t, "addRules", parsed["type"])

	rules, ok := parsed["rules"].([]any)
	if !ok {
		t.Fatal("rules should be an array")
	}
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}

	rule := rules[0].(map[string]any)
	// Verify camelCase field names
	assertControlEqual(t, "Read", rule["toolName"])
	assertControlEqual(t, "/tmp/*", rule["ruleContent"])
}

func testMarshalPermissionRuleValue(t *testing.T) {
	t.Helper()

	rule := PermissionRuleValue{
		ToolName:    "Write",
		RuleContent: ptrString("allow /home/*"),
	}

	data, err := json.Marshal(rule)
	assertControlNoError(t, err)

	var parsed map[string]any
	err = json.Unmarshal(data, &parsed)
	assertControlNoError(t, err)

	// Verify camelCase field names in JSON
	assertControlEqual(t, "Write", parsed["toolName"])
	assertControlEqual(t, "allow /home/*", parsed["ruleContent"])
}

// ptrString is a helper to create a pointer to a string.
func ptrString(s string) *string {
	return &s
}

// =============================================================================
// RewindFiles Request Serialization Tests (Issue #32)
// =============================================================================

func TestRewindFilesRequestSerialization(t *testing.T) {
	t.Run("marshal_rewind_files_request", testMarshalRewindFilesRequest)
}

func testMarshalRewindFilesRequest(t *testing.T) {
	t.Helper()

	req := SDKControlRequest{
		Type:      MessageTypeControlRequest,
		RequestID: "req_1_abc123",
		Request: RewindFilesRequest{
			Subtype:       SubtypeRewindFiles,
			UserMessageID: "msg-uuid-12345",
		},
	}

	data, err := json.Marshal(req)
	assertControlNoError(t, err)

	var parsed map[string]any
	err = json.Unmarshal(data, &parsed)
	assertControlNoError(t, err)

	assertControlEqual(t, "control_request", parsed["type"])
	assertControlEqual(t, "req_1_abc123", parsed["request_id"])

	request, ok := parsed["request"].(map[string]any)
	if !ok {
		t.Fatal("request field should be an object")
	}
	assertControlEqual(t, "rewind_files", request["subtype"])
	assertControlEqual(t, "msg-uuid-12345", request["user_message_id"])
}

// =============================================================================
// Phase 9: HandleControlInitErr Tests (Issue #110)
// =============================================================================

func TestHandleControlInitErr(t *testing.T) {
	t.Run("unblocks_send_control_request", testInitErrUnblocksSendControlRequest)
	t.Run("non_blocking_when_no_receiver", testInitErrNonBlockingNoReceiver)
	t.Run("only_first_error_delivered", testInitErrOnlyFirstDelivered)
}

func testInitErrUnblocksSendControlRequest(t *testing.T) {
	t.Helper()

	ctx, cancel := setupControlTestContext(t, 5*time.Second)
	defer cancel()

	transport := newControlMockTransport()
	protocol := NewProtocol(transport)

	err := protocol.Start(ctx)
	assertControlNoError(t, err)
	defer func() { _ = protocol.Close() }()

	// Inject an init error after a short delay (simulates early result message)
	go func() {
		time.Sleep(50 * time.Millisecond)
		protocol.HandleControlInitErr(fmt.Errorf("No conversation found with session ID: abc-123"))
	}()

	// SendControlRequest should unblock with the init error instead of timing out
	_, err = protocol.SendControlRequest(ctx, InitializeRequest{
		Subtype: SubtypeInitialize,
	}, 2*time.Second)

	if err == nil {
		t.Fatal("expected error from SendControlRequest")
	}
	if !strings.Contains(err.Error(), "No conversation found") {
		t.Errorf("expected session ID error, got: %v", err)
	}
}

func testInitErrNonBlockingNoReceiver(t *testing.T) {
	t.Helper()

	transport := newControlMockTransport()
	protocol := NewProtocol(transport)

	// HandleControlInitErr should not block even with no receiver
	done := make(chan struct{})
	go func() {
		protocol.HandleControlInitErr(fmt.Errorf("test error"))
		close(done)
	}()

	select {
	case <-done:
		// Non-blocking - good
	case <-time.After(1 * time.Second):
		t.Fatal("HandleControlInitErr blocked without a receiver")
	}
}

func testInitErrOnlyFirstDelivered(t *testing.T) {
	t.Helper()

	transport := newControlMockTransport()
	protocol := NewProtocol(transport)

	// Send two errors - only the first should be buffered (chan size 1)
	protocol.HandleControlInitErr(fmt.Errorf("first error"))
	protocol.HandleControlInitErr(fmt.Errorf("second error"))

	// Drain the channel - should get only the first
	select {
	case err := <-protocol.initErrChan:
		if !strings.Contains(err.Error(), "first error") {
			t.Errorf("expected first error, got: %v", err)
		}
	default:
		t.Fatal("expected an error in initErrChan")
	}

	// Channel should be empty now
	select {
	case err := <-protocol.initErrChan:
		t.Errorf("expected empty channel, got: %v", err)
	default:
		// Good - empty
	}
}
