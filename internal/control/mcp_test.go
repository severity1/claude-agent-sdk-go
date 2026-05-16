// Package control provides the SDK control protocol for bidirectional communication with Claude CLI.
package control

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"
)

// TestMcpMessageRouting tests the MCP message routing logic.
func TestMcpMessageRouting(t *testing.T) {
	tests := []struct {
		name      string
		method    string
		params    map[string]any
		wantError bool
	}{
		{"initialize", "initialize", nil, false},
		{"tools_list", "tools/list", nil, false},
		{"tools_call", "tools/call", map[string]any{"name": "test", "arguments": map[string]any{}}, false},
		{"notifications_initialized", "notifications/initialized", nil, false},
		{"unknown_method", "unknown/method", nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := setupMcpTestContext(t, 5*time.Second)
			defer cancel()

			server := newMockMcpServer("test", "1.0.0")
			p := NewProtocol(newMcpMockTransport(), WithSdkMcpServers(map[string]McpServer{"test": server}))

			msg := map[string]any{
				"jsonrpc": "2.0",
				"id":      1,
				"method":  tt.method,
			}
			if tt.params != nil {
				msg["params"] = tt.params
			}

			result, err := p.routeMcpMethod(ctx, server, msg)

			if tt.wantError && err == nil {
				t.Error("Expected error, got nil")
				return
			}
			if !tt.wantError && err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}
			if tt.wantError {
				return
			}

			if result["jsonrpc"] != "2.0" {
				t.Errorf("jsonrpc = %v, want %q", result["jsonrpc"], "2.0")
			}
		})
	}
}

// TestMcpInitializeResponse tests the initialize method response format.
func TestMcpInitializeResponse(t *testing.T) {
	ctx, cancel := setupMcpTestContext(t, 5*time.Second)
	defer cancel()

	server := newMockMcpServer("myserver", "2.0.0")
	p := NewProtocol(newMcpMockTransport(), WithSdkMcpServers(map[string]McpServer{"test": server}))

	msg := map[string]any{
		"jsonrpc": "2.0",
		"id":      42,
		"method":  "initialize",
	}

	result, err := p.routeMcpMethod(ctx, server, msg)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Check response structure
	if result["id"] != 42 {
		t.Errorf("id = %v, want 42", result["id"])
	}

	resultData, ok := result["result"].(map[string]any)
	if !ok {
		t.Fatal("Expected result to be a map")
	}

	if resultData["protocolVersion"] != "2024-11-05" {
		t.Errorf("protocolVersion = %v, want %q", resultData["protocolVersion"], "2024-11-05")
	}

	serverInfo, ok := resultData["serverInfo"].(map[string]any)
	if !ok {
		t.Fatal("Expected serverInfo to be a map")
	}

	if serverInfo["name"] != "myserver" {
		t.Errorf("serverInfo.name = %v, want %q", serverInfo["name"], "myserver")
	}
	if serverInfo["version"] != "2.0.0" {
		t.Errorf("serverInfo.version = %v, want %q", serverInfo["version"], "2.0.0")
	}
}

// TestMcpToolsListResponse tests the tools/list method response format.
func TestMcpToolsListResponse(t *testing.T) {
	ctx, cancel := setupMcpTestContext(t, 5*time.Second)
	defer cancel()

	server := newMockMcpServer("test", "1.0.0")
	server.tools = []McpToolDefinition{
		{Name: "add", Description: "Add numbers", InputSchema: map[string]any{"type": "object"}},
		{Name: "sub", Description: "Subtract numbers", InputSchema: map[string]any{"type": "object"}},
	}

	p := NewProtocol(newMcpMockTransport(), WithSdkMcpServers(map[string]McpServer{"test": server}))

	msg := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/list",
	}

	result, err := p.routeMcpMethod(ctx, server, msg)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	resultData, ok := result["result"].(map[string]any)
	if !ok {
		t.Fatal("Expected result to be a map")
	}

	tools, ok := resultData["tools"].([]map[string]any)
	if !ok {
		t.Fatal("Expected tools to be a slice of maps")
	}

	if len(tools) != 2 {
		t.Errorf("Expected 2 tools, got %d", len(tools))
	}
}

// TestMcpToolsListResponseWithAnnotations verifies that tools/list response
// includes the "annotations" key only for tools whose Annotations field is
// non-nil. A nil pointer omits the key; a non-nil but empty value still
// emits the key with an empty map (all fields are pointers with omitempty,
// so an empty struct produces no map entries but the key itself is present).
func TestMcpToolsListResponseWithAnnotations(t *testing.T) {
	ctx, cancel := setupMcpTestContext(t, 5*time.Second)
	defer cancel()

	title := "Annotated"
	readOnly := true
	server := newMockMcpServer("test", "1.0.0")
	server.tools = []McpToolDefinition{
		{
			Name:        "with_ann",
			Description: "Has partial annotations",
			InputSchema: map[string]any{"type": "object"},
			Annotations: &ToolAnnotations{
				Title:        &title,
				ReadOnlyHint: &readOnly,
			},
		},
		{
			Name:        "no_ann",
			Description: "No annotations",
			InputSchema: map[string]any{"type": "object"},
		},
		{
			Name:        "empty_ann",
			Description: "Non-nil but all-fields-unset annotations",
			InputSchema: map[string]any{"type": "object"},
			Annotations: &ToolAnnotations{},
		},
	}

	p := NewProtocol(newMcpMockTransport(), WithSdkMcpServers(map[string]McpServer{"test": server}))

	msg := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/list",
	}

	result, err := p.routeMcpMethod(ctx, server, msg)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	resultData, ok := result["result"].(map[string]any)
	if !ok {
		t.Fatal("Expected result to be a map")
	}
	tools, ok := resultData["tools"].([]map[string]any)
	if !ok {
		t.Fatal("Expected tools to be a slice of maps")
	}
	if len(tools) != 3 {
		t.Fatalf("Expected 3 tools, got %d", len(tools))
	}

	for _, td := range tools {
		switch td["name"] {
		case "with_ann":
			ann, ok := td["annotations"].(map[string]any)
			if !ok {
				t.Errorf("with_ann.annotations missing or wrong type: %T (%v)", td["annotations"], td["annotations"])
				continue
			}
			if ann["title"] != "Annotated" {
				t.Errorf("with_ann.annotations.title = %v, want %q", ann["title"], "Annotated")
			}
			if ann["readOnlyHint"] != true {
				t.Errorf("with_ann.annotations.readOnlyHint = %v, want true", ann["readOnlyHint"])
			}
			// Other fields should be omitted (not present in map).
			for _, k := range []string{"destructiveHint", "idempotentHint", "openWorldHint"} {
				if _, present := ann[k]; present {
					t.Errorf("with_ann.annotations.%s should be omitted, got %v", k, ann[k])
				}
			}
		case "no_ann":
			if _, present := td["annotations"]; present {
				t.Errorf("no_ann should omit annotations key, got %v", td["annotations"])
			}
		case "empty_ann":
			ann, ok := td["annotations"].(map[string]any)
			if !ok {
				t.Errorf("empty_ann.annotations missing or wrong type: %T (%v)", td["annotations"], td["annotations"])
				continue
			}
			if len(ann) != 0 {
				t.Errorf("empty_ann.annotations should be empty map, got %d keys: %v", len(ann), ann)
			}
		}
	}
}

// TestMcpToolsListResponseAnnotationsAllFields verifies wire format when all
// five MCP-spec annotation fields are populated, including exact camelCase
// JSON key naming.
func TestMcpToolsListResponseAnnotationsAllFields(t *testing.T) {
	ctx, cancel := setupMcpTestContext(t, 5*time.Second)
	defer cancel()

	title := "Full"
	readOnly := true
	destructive := false
	idempotent := true
	openWorld := false

	server := newMockMcpServer("test", "1.0.0")
	server.tools = []McpToolDefinition{
		{
			Name:        "full",
			Description: "All fields set",
			InputSchema: map[string]any{"type": "object"},
			Annotations: &ToolAnnotations{
				Title:           &title,
				ReadOnlyHint:    &readOnly,
				DestructiveHint: &destructive,
				IdempotentHint:  &idempotent,
				OpenWorldHint:   &openWorld,
			},
		},
	}

	p := NewProtocol(newMcpMockTransport(), WithSdkMcpServers(map[string]McpServer{"test": server}))

	msg := map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/list"}
	result, err := p.routeMcpMethod(ctx, server, msg)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	resultData, ok := result["result"].(map[string]any)
	if !ok {
		t.Fatal("Expected result to be a map")
	}
	tools, ok := resultData["tools"].([]map[string]any)
	if !ok {
		t.Fatal("Expected tools to be a slice of maps")
	}
	ann, ok := tools[0]["annotations"].(map[string]any)
	if !ok {
		t.Fatalf("expected annotations map, got %T (%v)", tools[0]["annotations"], tools[0]["annotations"])
	}

	wantKeys := map[string]any{
		"title":           "Full",
		"readOnlyHint":    true,
		"destructiveHint": false,
		"idempotentHint":  true,
		"openWorldHint":   false,
	}
	for k, want := range wantKeys {
		if got := ann[k]; got != want {
			t.Errorf("annotations[%q] = %v (%T), want %v", k, got, got, want)
		}
	}
	if len(ann) != len(wantKeys) {
		t.Errorf("annotations has %d keys (%v), want exactly %d (%v)", len(ann), ann, len(wantKeys), wantKeys)
	}
}

// TestMcpToolsCallResponse tests the tools/call method response format.
func TestMcpToolsCallResponse(t *testing.T) {
	ctx, cancel := setupMcpTestContext(t, 5*time.Second)
	defer cancel()

	server := newMockMcpServer("test", "1.0.0")
	server.callResult = &McpToolResult{
		Content: []McpContent{{Type: "text", Text: "42"}},
	}

	p := NewProtocol(newMcpMockTransport(), WithSdkMcpServers(map[string]McpServer{"test": server}))

	msg := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      "add",
			"arguments": map[string]any{"a": 1, "b": 2},
		},
	}

	result, err := p.routeMcpMethod(ctx, server, msg)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	resultData, ok := result["result"].(map[string]any)
	if !ok {
		t.Fatal("Expected result to be a map")
	}

	content, ok := resultData["content"].([]map[string]any)
	if !ok {
		t.Fatal("Expected content to be a slice of maps")
	}

	if len(content) != 1 {
		t.Errorf("Expected 1 content item, got %d", len(content))
		return
	}

	if content[0]["type"] != "text" {
		t.Errorf("content[0].type = %v, want %q", content[0]["type"], "text")
	}
	if content[0]["text"] != "42" {
		t.Errorf("content[0].text = %v, want %q", content[0]["text"], "42")
	}
}

// TestMcpToolsCallIsError tests the isError flag in tool call results.
func TestMcpToolsCallIsError(t *testing.T) {
	ctx, cancel := setupMcpTestContext(t, 5*time.Second)
	defer cancel()

	server := newMockMcpServer("test", "1.0.0")
	server.callResult = &McpToolResult{
		Content: []McpContent{{Type: "text", Text: "error occurred"}},
		IsError: true,
	}

	p := NewProtocol(newMcpMockTransport(), WithSdkMcpServers(map[string]McpServer{"test": server}))

	msg := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      "failing",
			"arguments": map[string]any{},
		},
	}

	result, err := p.routeMcpMethod(ctx, server, msg)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	resultData, ok := result["result"].(map[string]any)
	if !ok {
		t.Fatal("Expected result to be a map")
	}

	isError, ok := resultData["isError"].(bool)
	if !ok || !isError {
		t.Error("Expected isError = true")
	}
}

// TestMcpImageContent tests image content in tool results.
func TestMcpImageContent(t *testing.T) {
	ctx, cancel := setupMcpTestContext(t, 5*time.Second)
	defer cancel()

	server := newMockMcpServer("test", "1.0.0")
	server.callResult = &McpToolResult{
		Content: []McpContent{{Type: "image", Data: "base64data", MimeType: "image/png"}},
	}

	p := NewProtocol(newMcpMockTransport(), WithSdkMcpServers(map[string]McpServer{"test": server}))

	msg := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      "image",
			"arguments": map[string]any{},
		},
	}

	result, err := p.routeMcpMethod(ctx, server, msg)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	resultData, ok := result["result"].(map[string]any)
	if !ok {
		t.Fatal("Expected result to be a map")
	}

	content, ok := resultData["content"].([]map[string]any)
	if !ok || len(content) == 0 {
		t.Fatal("Expected at least 1 content item")
	}

	if content[0]["type"] != "image" {
		t.Errorf("content[0].type = %v, want %q", content[0]["type"], "image")
	}
	if content[0]["data"] != "base64data" {
		t.Errorf("content[0].data = %v, want %q", content[0]["data"], "base64data")
	}
	if content[0]["mimeType"] != "image/png" {
		t.Errorf("content[0].mimeType = %v, want %q", content[0]["mimeType"], "image/png")
	}
}

// TestMcpServerNotFound tests error handling when server is not found.
func TestMcpServerNotFound(t *testing.T) {
	ctx, cancel := setupMcpTestContext(t, 5*time.Second)
	defer cancel()

	transport := newMcpMockTransport()
	p := NewProtocol(transport, WithSdkMcpServers(map[string]McpServer{}))

	request := map[string]any{
		"server_name": "nonexistent",
		"message": map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"method":  "initialize",
		},
	}

	err := p.handleMcpMessageRequest(ctx, "req_1", request)
	if err != nil {
		t.Fatalf("Unexpected error (should send error response): %v", err)
	}

	// Check that error response was sent
	if len(transport.sentData) == 0 {
		t.Fatal("Expected error response to be sent")
	}

	var response SDKControlResponse
	if err := json.Unmarshal(transport.sentData[0], &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	// The response should contain an MCP error
	respData, ok := response.Response.Response.(map[string]any)
	if !ok {
		t.Fatal("Expected response data to be a map")
	}

	mcpResp, ok := respData["mcp_response"].(map[string]any)
	if !ok {
		t.Fatal("Expected mcp_response to be a map")
	}

	if mcpResp["error"] == nil {
		t.Error("Expected error in MCP response")
	}
}

// TestMcpMissingServerName tests error handling when server_name is missing.
func TestMcpMissingServerName(t *testing.T) {
	ctx, cancel := setupMcpTestContext(t, 5*time.Second)
	defer cancel()

	transport := newMcpMockTransport()
	p := NewProtocol(transport)

	request := map[string]any{
		"message": map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"method":  "initialize",
		},
	}

	err := p.handleMcpMessageRequest(ctx, "req_1", request)
	if err != nil {
		t.Fatalf("Unexpected error (should send error response): %v", err)
	}

	// Check that error response was sent
	if len(transport.sentData) == 0 {
		t.Fatal("Expected error response to be sent")
	}
}

// TestMcpMissingMessage tests error handling when message is missing.
func TestMcpMissingMessage(t *testing.T) {
	ctx, cancel := setupMcpTestContext(t, 5*time.Second)
	defer cancel()

	transport := newMcpMockTransport()
	p := NewProtocol(transport)

	request := map[string]any{
		"server_name": "test",
	}

	err := p.handleMcpMessageRequest(ctx, "req_1", request)
	if err != nil {
		t.Fatalf("Unexpected error (should send error response): %v", err)
	}

	// Check that error response was sent
	if len(transport.sentData) == 0 {
		t.Fatal("Expected error response to be sent")
	}
}

// TestMcpPanicRecovery tests that panics in handlers are recovered.
func TestMcpPanicRecovery(t *testing.T) {
	ctx, cancel := setupMcpTestContext(t, 5*time.Second)
	defer cancel()

	server := newMockMcpServer("test", "1.0.0")
	server.callPanic = true

	transport := newMcpMockTransport()
	p := NewProtocol(transport, WithSdkMcpServers(map[string]McpServer{"test": server}))

	request := map[string]any{
		"server_name": "test",
		"message": map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"method":  "tools/call",
			"params": map[string]any{
				"name":      "panic",
				"arguments": map[string]any{},
			},
		},
	}

	// This should not panic
	err := p.handleMcpMessageRequest(ctx, "req_1", request)
	if err != nil {
		t.Fatalf("Unexpected error (should send error response): %v", err)
	}

	// Check that error response was sent
	if len(transport.sentData) == 0 {
		t.Fatal("Expected error response to be sent after panic recovery")
	}
}

// TestWithSdkMcpServers tests the protocol option.
func TestWithSdkMcpServers(t *testing.T) {
	server := newMockMcpServer("test", "1.0.0")
	servers := map[string]McpServer{"myserver": server}

	p := NewProtocol(newMcpMockTransport(), WithSdkMcpServers(servers))

	if p.sdkMcpServers == nil {
		t.Fatal("sdkMcpServers is nil")
	}
	if _, ok := p.sdkMcpServers["myserver"]; !ok {
		t.Error("myserver not found in sdkMcpServers")
	}
}

// mockMcpServer implements McpServer for testing.
type mockMcpServer struct {
	mu         sync.RWMutex
	name       string
	version    string
	tools      []McpToolDefinition
	callResult *McpToolResult
	callErr    error
	callPanic  bool
}

func newMockMcpServer(name, version string) *mockMcpServer {
	return &mockMcpServer{
		name:    name,
		version: version,
		tools:   []McpToolDefinition{},
		callResult: &McpToolResult{
			Content: []McpContent{{Type: "text", Text: "ok"}},
		},
	}
}

func (m *mockMcpServer) Name() string {
	return m.name
}

func (m *mockMcpServer) Version() string {
	return m.version
}

func (m *mockMcpServer) ListTools(_ context.Context) ([]McpToolDefinition, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.tools, nil
}

func (m *mockMcpServer) CallTool(_ context.Context, _ string, _ map[string]any) (*McpToolResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.callPanic {
		panic("intentional panic for testing")
	}

	if m.callErr != nil {
		return nil, m.callErr
	}
	return m.callResult, nil
}

// mcpMockTransport implements Transport for MCP tests.
type mcpMockTransport struct {
	mu       sync.Mutex
	sentData [][]byte
	readChan chan []byte
}

func newMcpMockTransport() *mcpMockTransport {
	return &mcpMockTransport{
		sentData: [][]byte{},
		readChan: make(chan []byte, 10),
	}
}

func (m *mcpMockTransport) Write(_ context.Context, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Make a copy to avoid mutation issues
	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)
	m.sentData = append(m.sentData, dataCopy)
	return nil
}

func (m *mcpMockTransport) Read(_ context.Context) <-chan []byte {
	return m.readChan
}

func (m *mcpMockTransport) Close() error {
	return nil
}

// setupMcpTestContext creates a context with timeout for MCP tests.
func setupMcpTestContext(t *testing.T, timeout time.Duration) (context.Context, context.CancelFunc) {
	t.Helper()
	return context.WithTimeout(context.Background(), timeout)
}
