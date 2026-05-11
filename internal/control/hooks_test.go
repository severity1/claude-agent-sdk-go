package control

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"
)

// Test constants for hook callback tests.
const (
	testDecisionBlock = "block"
)

// =============================================================================
// Hook Callback Handler Tests
// =============================================================================

func TestHookCallbackHandler_PreToolUse(t *testing.T) {
	ctx, cancel := setupHookTestContext(t, 5*time.Second)
	defer cancel()

	transport := newHookMockTransport()

	// Track callback invocation
	var receivedInput any
	var receivedToolUseID *string
	callbackCalled := false

	callback := func(
		_ context.Context,
		input any,
		toolUseID *string,
		_ HookContext,
	) (HookJSONOutput, error) {
		callbackCalled = true
		receivedInput = input
		receivedToolUseID = toolUseID
		continueVal := true
		return HookJSONOutput{Continue: &continueVal}, nil
	}

	// Register callback
	hookCallbacks := map[string]HookCallback{
		"hook_0": callback,
	}

	protocol := NewProtocol(transport, WithHookCallbacks(hookCallbacks))

	err := protocol.Start(ctx)
	assertHookNoError(t, err)
	defer func() { _ = protocol.Close() }()

	// Simulate incoming hook_callback request from CLI
	toolUseID := "tool_123"
	request := map[string]any{
		"type":       MessageTypeControlRequest,
		"request_id": "req_hook_1",
		"request": map[string]any{
			"subtype":         SubtypeHookCallback,
			"callback_id":     "hook_0",
			"hook_event_name": "PreToolUse",
			"input": map[string]any{
				"session_id":      "test-session",
				"transcript_path": "/tmp/transcript.json",
				"cwd":             "/home/user",
				"permission_mode": "default",
				"hook_event_name": "PreToolUse",
				"tool_name":       "Bash",
				"tool_input":      map[string]any{"command": "ls -la"},
			},
			"tool_use_id": toolUseID,
		},
	}

	err = protocol.HandleIncomingMessage(ctx, request)
	assertHookNoError(t, err)

	if !callbackCalled {
		t.Fatal("Expected callback to be called")
	}

	// Verify input was parsed correctly
	preToolInput, ok := receivedInput.(*PreToolUseHookInput)
	if !ok {
		t.Fatalf("Expected *PreToolUseHookInput, got %T", receivedInput)
	}

	if preToolInput.ToolName != "Bash" {
		t.Errorf("ToolName = %q, want %q", preToolInput.ToolName, "Bash")
	}

	if receivedToolUseID == nil || *receivedToolUseID != toolUseID {
		t.Errorf("toolUseID = %v, want %q", receivedToolUseID, toolUseID)
	}

	// Verify response was sent
	assertHookResponseSent(t, transport, "req_hook_1", ResponseSubtypeSuccess)
}

func TestHookCallbackHandler_PostToolUse(t *testing.T) {
	ctx, cancel := setupHookTestContext(t, 5*time.Second)
	defer cancel()

	transport := newHookMockTransport()

	var receivedInput any
	callbackCalled := false

	callback := func(
		_ context.Context,
		input any,
		_ *string,
		_ HookContext,
	) (HookJSONOutput, error) {
		callbackCalled = true
		receivedInput = input
		return HookJSONOutput{}, nil
	}

	hookCallbacks := map[string]HookCallback{
		"hook_0": callback,
	}

	protocol := NewProtocol(transport, WithHookCallbacks(hookCallbacks))

	err := protocol.Start(ctx)
	assertHookNoError(t, err)
	defer func() { _ = protocol.Close() }()

	request := map[string]any{
		"type":       MessageTypeControlRequest,
		"request_id": "req_hook_2",
		"request": map[string]any{
			"subtype":         SubtypeHookCallback,
			"callback_id":     "hook_0",
			"hook_event_name": "PostToolUse",
			"input": map[string]any{
				"session_id":      "test-session",
				"transcript_path": "/tmp/transcript.json",
				"cwd":             "/home/user",
				"hook_event_name": "PostToolUse",
				"tool_name":       "Bash",
				"tool_input":      map[string]any{"command": "ls -la"},
				"tool_response":   "file1.txt\nfile2.txt",
			},
		},
	}

	err = protocol.HandleIncomingMessage(ctx, request)
	assertHookNoError(t, err)

	if !callbackCalled {
		t.Fatal("Expected callback to be called")
	}

	postToolInput, ok := receivedInput.(*PostToolUseHookInput)
	if !ok {
		t.Fatalf("Expected *PostToolUseHookInput, got %T", receivedInput)
	}

	if postToolInput.ToolResponse != "file1.txt\nfile2.txt" {
		t.Errorf("ToolResponse = %v, want %q", postToolInput.ToolResponse, "file1.txt\nfile2.txt")
	}
}

func TestHookCallbackHandler_PostToolUseFailure(t *testing.T) {
	ctx, cancel := setupHookTestContext(t, 5*time.Second)
	defer cancel()

	transport := newHookMockTransport()

	var receivedInput any
	callbackCalled := false

	callback := func(
		_ context.Context,
		input any,
		_ *string,
		_ HookContext,
	) (HookJSONOutput, error) {
		callbackCalled = true
		receivedInput = input
		return HookJSONOutput{}, nil
	}

	hookCallbacks := map[string]HookCallback{
		"hook_0": callback,
	}

	protocol := NewProtocol(transport, WithHookCallbacks(hookCallbacks))

	err := protocol.Start(ctx)
	assertHookNoError(t, err)
	defer func() { _ = protocol.Close() }()

	request := map[string]any{
		"type":       MessageTypeControlRequest,
		"request_id": "req_hook_ptuf",
		"request": map[string]any{
			"subtype":         SubtypeHookCallback,
			"callback_id":     "hook_0",
			"hook_event_name": "PostToolUseFailure",
			"input": map[string]any{
				"session_id":      "test-session",
				"transcript_path": "/tmp/transcript.json",
				"cwd":             "/home/user",
				"hook_event_name": "PostToolUseFailure",
				"tool_name":       "Bash",
				"tool_input":      map[string]any{"command": "ls -la"},
				"tool_use_id":     "tool_use_abc",
				"error":           "exit status 1",
				"is_interrupt":    true,
			},
		},
	}

	err = protocol.HandleIncomingMessage(ctx, request)
	assertHookNoError(t, err)

	if !callbackCalled {
		t.Fatal("Expected callback to be called")
	}

	failureInput, ok := receivedInput.(*PostToolUseFailureHookInput)
	if !ok {
		t.Fatalf("Expected *PostToolUseFailureHookInput, got %T", receivedInput)
		return
	}

	if failureInput.HookEventName != "PostToolUseFailure" {
		t.Errorf("HookEventName = %q, want %q", failureInput.HookEventName, "PostToolUseFailure")
	}
	if failureInput.ToolName != "Bash" {
		t.Errorf("ToolName = %q, want %q", failureInput.ToolName, "Bash")
	}
	if failureInput.ToolUseID != "tool_use_abc" {
		t.Errorf("ToolUseID = %q, want %q", failureInput.ToolUseID, "tool_use_abc")
	}
	if failureInput.Error != "exit status 1" {
		t.Errorf("Error = %q, want %q", failureInput.Error, "exit status 1")
	}
	if failureInput.IsInterrupt == nil || !*failureInput.IsInterrupt {
		t.Errorf("IsInterrupt = %v, want pointer to true", failureInput.IsInterrupt)
	}
	if failureInput.ToolInput["command"] != "ls -la" {
		t.Errorf("ToolInput[command] = %v, want %q", failureInput.ToolInput["command"], "ls -la")
	}
	// Base hook fields propagated
	if failureInput.SessionID != "test-session" {
		t.Errorf("SessionID = %q, want %q", failureInput.SessionID, "test-session")
	}
}

func TestHookCallbackHandler_PostToolUseFailureNoInterrupt(t *testing.T) {
	// Verifies that when the input map omits "is_interrupt", the parsed struct's
	// IsInterrupt is nil (Python's NotRequired[bool] semantics).
	ctx, cancel := setupHookTestContext(t, 5*time.Second)
	defer cancel()

	transport := newHookMockTransport()

	var receivedInput any
	callback := func(
		_ context.Context,
		input any,
		_ *string,
		_ HookContext,
	) (HookJSONOutput, error) {
		receivedInput = input
		return HookJSONOutput{}, nil
	}

	hookCallbacks := map[string]HookCallback{
		"hook_0": callback,
	}

	protocol := NewProtocol(transport, WithHookCallbacks(hookCallbacks))

	err := protocol.Start(ctx)
	assertHookNoError(t, err)
	defer func() { _ = protocol.Close() }()

	request := map[string]any{
		"type":       MessageTypeControlRequest,
		"request_id": "req_hook_ptuf_nointr",
		"request": map[string]any{
			"subtype":         SubtypeHookCallback,
			"callback_id":     "hook_0",
			"hook_event_name": "PostToolUseFailure",
			"input": map[string]any{
				"session_id":      "test-session",
				"transcript_path": "/tmp/transcript.json",
				"cwd":             "/home/user",
				"hook_event_name": "PostToolUseFailure",
				"tool_name":       "Bash",
				"tool_input":      map[string]any{"command": "ls"},
				"tool_use_id":     "tool_use_abc",
				"error":           "boom",
				// is_interrupt deliberately omitted
			},
		},
	}

	err = protocol.HandleIncomingMessage(ctx, request)
	assertHookNoError(t, err)

	failureInput, ok := receivedInput.(*PostToolUseFailureHookInput)
	if !ok {
		t.Fatalf("Expected *PostToolUseFailureHookInput, got %T", receivedInput)
		return
	}

	if failureInput.IsInterrupt != nil {
		t.Errorf("IsInterrupt = %v, want nil when key absent", *failureInput.IsInterrupt)
	}
}

func TestHookCallbackHandler_BlockDecision(t *testing.T) {
	ctx, cancel := setupHookTestContext(t, 5*time.Second)
	defer cancel()

	transport := newHookMockTransport()

	callback := func(
		_ context.Context,
		_ any,
		_ *string,
		_ HookContext,
	) (HookJSONOutput, error) {
		decision := testDecisionBlock
		reason := "Dangerous command detected"
		return HookJSONOutput{
			Decision: &decision,
			Reason:   &reason,
		}, nil
	}

	hookCallbacks := map[string]HookCallback{
		"hook_0": callback,
	}

	protocol := NewProtocol(transport, WithHookCallbacks(hookCallbacks))

	err := protocol.Start(ctx)
	assertHookNoError(t, err)
	defer func() { _ = protocol.Close() }()

	request := map[string]any{
		"type":       MessageTypeControlRequest,
		"request_id": "req_hook_3",
		"request": map[string]any{
			"subtype":         SubtypeHookCallback,
			"callback_id":     "hook_0",
			"hook_event_name": "PreToolUse",
			"input": map[string]any{
				"session_id":      "test-session",
				"transcript_path": "/tmp/transcript.json",
				"cwd":             "/home/user",
				"hook_event_name": "PreToolUse",
				"tool_name":       "Bash",
				"tool_input":      map[string]any{"command": "rm -rf /"},
			},
		},
	}

	err = protocol.HandleIncomingMessage(ctx, request)
	assertHookNoError(t, err)

	// Verify response contains block decision
	transport.mu.Lock()
	defer transport.mu.Unlock()

	if len(transport.writtenData) == 0 {
		t.Fatal("Expected response to be written")
	}

	var resp SDKControlResponse
	err = json.Unmarshal(transport.writtenData[len(transport.writtenData)-1], &resp)
	assertHookNoError(t, err)

	responseData, ok := resp.Response.Response.(map[string]any)
	if !ok {
		t.Fatal("Response should be a map")
	}

	if responseData["decision"] != testDecisionBlock {
		t.Errorf("decision = %v, want %q", responseData["decision"], testDecisionBlock)
	}
	if responseData["reason"] != "Dangerous command detected" {
		t.Errorf("reason = %v, want %q", responseData["reason"], "Dangerous command detected")
	}
}

func TestHookCallbackHandler_PanicRecovery(t *testing.T) {
	ctx, cancel := setupHookTestContext(t, 5*time.Second)
	defer cancel()

	transport := newHookMockTransport()

	callback := func(
		_ context.Context,
		_ any,
		_ *string,
		_ HookContext,
	) (HookJSONOutput, error) {
		panic("simulated panic in callback")
	}

	hookCallbacks := map[string]HookCallback{
		"hook_0": callback,
	}

	protocol := NewProtocol(transport, WithHookCallbacks(hookCallbacks))

	err := protocol.Start(ctx)
	assertHookNoError(t, err)
	defer func() { _ = protocol.Close() }()

	request := map[string]any{
		"type":       MessageTypeControlRequest,
		"request_id": "req_hook_4",
		"request": map[string]any{
			"subtype":         SubtypeHookCallback,
			"callback_id":     "hook_0",
			"hook_event_name": "PreToolUse",
			"input": map[string]any{
				"session_id":      "test-session",
				"transcript_path": "/tmp/transcript.json",
				"cwd":             "/home/user",
				"hook_event_name": "PreToolUse",
				"tool_name":       "Bash",
				"tool_input":      map[string]any{"command": "ls"},
			},
		},
	}

	// Should not panic - error should be recovered and sent as error response
	err = protocol.HandleIncomingMessage(ctx, request)
	assertHookNoError(t, err)

	// Verify error response was sent
	assertHookResponseSent(t, transport, "req_hook_4", ResponseSubtypeError)
}

func TestHookCallbackHandler_NotFound(t *testing.T) {
	ctx, cancel := setupHookTestContext(t, 5*time.Second)
	defer cancel()

	transport := newHookMockTransport()

	// Empty callback map - no callbacks registered
	hookCallbacks := map[string]HookCallback{}

	protocol := NewProtocol(transport, WithHookCallbacks(hookCallbacks))

	err := protocol.Start(ctx)
	assertHookNoError(t, err)
	defer func() { _ = protocol.Close() }()

	request := map[string]any{
		"type":       MessageTypeControlRequest,
		"request_id": "req_hook_5",
		"request": map[string]any{
			"subtype":         SubtypeHookCallback,
			"callback_id":     "nonexistent_hook",
			"hook_event_name": "PreToolUse",
			"input": map[string]any{
				"session_id":      "test-session",
				"transcript_path": "/tmp/transcript.json",
				"cwd":             "/home/user",
				"hook_event_name": "PreToolUse",
				"tool_name":       "Bash",
				"tool_input":      map[string]any{"command": "ls"},
			},
		},
	}

	err = protocol.HandleIncomingMessage(ctx, request)
	assertHookNoError(t, err)

	// Verify error response was sent for missing callback
	assertHookResponseSent(t, transport, "req_hook_5", ResponseSubtypeError)
}

func TestHookCallbackHandler_CallbackError(t *testing.T) {
	ctx, cancel := setupHookTestContext(t, 5*time.Second)
	defer cancel()

	transport := newHookMockTransport()

	callback := func(
		_ context.Context,
		_ any,
		_ *string,
		_ HookContext,
	) (HookJSONOutput, error) {
		return HookJSONOutput{}, fmt.Errorf("callback processing failed")
	}

	hookCallbacks := map[string]HookCallback{
		"hook_0": callback,
	}

	protocol := NewProtocol(transport, WithHookCallbacks(hookCallbacks))

	err := protocol.Start(ctx)
	assertHookNoError(t, err)
	defer func() { _ = protocol.Close() }()

	request := map[string]any{
		"type":       MessageTypeControlRequest,
		"request_id": "req_hook_6",
		"request": map[string]any{
			"subtype":         SubtypeHookCallback,
			"callback_id":     "hook_0",
			"hook_event_name": "PreToolUse",
			"input": map[string]any{
				"session_id":      "test-session",
				"transcript_path": "/tmp/transcript.json",
				"cwd":             "/home/user",
				"hook_event_name": "PreToolUse",
				"tool_name":       "Bash",
				"tool_input":      map[string]any{"command": "ls"},
			},
		},
	}

	err = protocol.HandleIncomingMessage(ctx, request)
	assertHookNoError(t, err)

	// Verify error response was sent
	assertHookResponseSent(t, transport, "req_hook_6", ResponseSubtypeError)
}

func TestHookCallbackHandler_UserPromptSubmit(t *testing.T) {
	ctx, cancel := setupHookTestContext(t, 5*time.Second)
	defer cancel()

	transport := newHookMockTransport()

	var receivedInput any
	callbackCalled := false

	callback := func(
		_ context.Context,
		input any,
		_ *string,
		_ HookContext,
	) (HookJSONOutput, error) {
		callbackCalled = true
		receivedInput = input
		return HookJSONOutput{}, nil
	}

	hookCallbacks := map[string]HookCallback{
		"hook_0": callback,
	}

	protocol := NewProtocol(transport, WithHookCallbacks(hookCallbacks))

	err := protocol.Start(ctx)
	assertHookNoError(t, err)
	defer func() { _ = protocol.Close() }()

	request := map[string]any{
		"type":       MessageTypeControlRequest,
		"request_id": "req_hook_7",
		"request": map[string]any{
			"subtype":         SubtypeHookCallback,
			"callback_id":     "hook_0",
			"hook_event_name": "UserPromptSubmit",
			"input": map[string]any{
				"session_id":      "test-session",
				"transcript_path": "/tmp/transcript.json",
				"cwd":             "/home/user",
				"hook_event_name": "UserPromptSubmit",
				"prompt":          "Help me fix this bug",
			},
		},
	}

	err = protocol.HandleIncomingMessage(ctx, request)
	assertHookNoError(t, err)

	if !callbackCalled {
		t.Fatal("Expected callback to be called")
	}

	promptInput, ok := receivedInput.(*UserPromptSubmitHookInput)
	if !ok {
		t.Fatalf("Expected *UserPromptSubmitHookInput, got %T", receivedInput)
	}

	if promptInput.Prompt != "Help me fix this bug" {
		t.Errorf("Prompt = %q, want %q", promptInput.Prompt, "Help me fix this bug")
	}
}

func TestHookCallbackHandler_StopHook(t *testing.T) {
	ctx, cancel := setupHookTestContext(t, 5*time.Second)
	defer cancel()

	transport := newHookMockTransport()

	var receivedInput any
	callbackCalled := false

	callback := func(
		_ context.Context,
		input any,
		_ *string,
		_ HookContext,
	) (HookJSONOutput, error) {
		callbackCalled = true
		receivedInput = input
		return HookJSONOutput{}, nil
	}

	hookCallbacks := map[string]HookCallback{
		"hook_0": callback,
	}

	protocol := NewProtocol(transport, WithHookCallbacks(hookCallbacks))

	err := protocol.Start(ctx)
	assertHookNoError(t, err)
	defer func() { _ = protocol.Close() }()

	request := map[string]any{
		"type":       MessageTypeControlRequest,
		"request_id": "req_hook_8",
		"request": map[string]any{
			"subtype":         SubtypeHookCallback,
			"callback_id":     "hook_0",
			"hook_event_name": "Stop",
			"input": map[string]any{
				"session_id":       "test-session",
				"transcript_path":  "/tmp/transcript.json",
				"cwd":              "/home/user",
				"hook_event_name":  "Stop",
				"stop_hook_active": true,
			},
		},
	}

	err = protocol.HandleIncomingMessage(ctx, request)
	assertHookNoError(t, err)

	if !callbackCalled {
		t.Fatal("Expected callback to be called")
	}

	stopInput, ok := receivedInput.(*StopHookInput)
	if !ok {
		t.Fatalf("Expected *StopHookInput, got %T", receivedInput)
	}

	if !stopInput.StopHookActive {
		t.Error("StopHookActive should be true")
	}
}

func TestHookCallbackHandler_ThreadSafe(t *testing.T) {
	ctx, cancel := setupHookTestContext(t, 10*time.Second)
	defer cancel()

	transport := newHookMockTransport()

	var callCount int
	var mu sync.Mutex

	callback := func(
		_ context.Context,
		_ any,
		_ *string,
		_ HookContext,
	) (HookJSONOutput, error) {
		mu.Lock()
		callCount++
		mu.Unlock()
		return HookJSONOutput{}, nil
	}

	hookCallbacks := map[string]HookCallback{
		"hook_0": callback,
	}

	protocol := NewProtocol(transport, WithHookCallbacks(hookCallbacks))

	err := protocol.Start(ctx)
	assertHookNoError(t, err)
	defer func() { _ = protocol.Close() }()

	// Send multiple concurrent requests
	var wg sync.WaitGroup
	numRequests := 10

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			request := map[string]any{
				"type":       MessageTypeControlRequest,
				"request_id": fmt.Sprintf("req_hook_%d", idx),
				"request": map[string]any{
					"subtype":         SubtypeHookCallback,
					"callback_id":     "hook_0",
					"hook_event_name": "PreToolUse",
					"input": map[string]any{
						"session_id":      "test-session",
						"transcript_path": "/tmp/transcript.json",
						"cwd":             "/home/user",
						"hook_event_name": "PreToolUse",
						"tool_name":       "Bash",
						"tool_input":      map[string]any{"command": "ls"},
					},
				},
			}
			_ = protocol.HandleIncomingMessage(ctx, request)
		}(i)
	}

	wg.Wait()

	mu.Lock()
	if callCount != numRequests {
		t.Errorf("Expected %d callbacks, got %d", numRequests, callCount)
	}
	mu.Unlock()
}

// =============================================================================
// Initialize with Hooks Tests
// =============================================================================

func TestGenerateHookRegistrations(t *testing.T) {
	callback := func(
		_ context.Context,
		_ any,
		_ *string,
		_ HookContext,
	) (HookJSONOutput, error) {
		return HookJSONOutput{}, nil
	}

	hooks := map[HookEvent][]HookMatcher{
		HookEventPreToolUse: {
			{Matcher: "Bash", Hooks: []HookCallback{callback}},
			{Matcher: "Write|Edit", Hooks: []HookCallback{callback, callback}},
		},
		HookEventPostToolUse: {
			{Matcher: "", Hooks: []HookCallback{callback}}, // Empty matcher = all tools
		},
	}

	transport := newHookMockTransport()
	protocol := NewProtocol(transport, WithHooks(hooks))

	registrations := protocol.generateHookRegistrations()

	// Should generate registrations for all callbacks
	// PreToolUse: 1 callback from first matcher + 2 callbacks from second matcher = 3
	// PostToolUse: 1 callback = 1
	// Total: 4 registrations
	if len(registrations) != 4 {
		t.Errorf("Expected 4 registrations, got %d", len(registrations))
	}

	// Verify callback ID format matches Python SDK: "hook_{counter}"
	for _, reg := range registrations {
		if len(reg.CallbackID) == 0 {
			t.Error("CallbackID should not be empty")
		}
		// Check format starts with "hook_"
		if reg.CallbackID[:5] != "hook_" {
			t.Errorf("CallbackID should start with 'hook_', got %q", reg.CallbackID)
		}
	}
}

func TestInitializeWithHooks(t *testing.T) {
	ctx, cancel := setupHookTestContext(t, 5*time.Second)
	defer cancel()

	callback := func(
		_ context.Context,
		_ any,
		_ *string,
		_ HookContext,
	) (HookJSONOutput, error) {
		return HookJSONOutput{}, nil
	}

	timeout := 30.0
	hooks := map[HookEvent][]HookMatcher{
		HookEventPreToolUse: {
			{Matcher: "Bash", Hooks: []HookCallback{callback}, Timeout: &timeout},
		},
	}

	transport := newHookMockTransport()
	protocol := NewProtocol(transport, WithHooks(hooks))

	err := protocol.Start(ctx)
	assertHookNoError(t, err)
	defer func() { _ = protocol.Close() }()

	// Set up auto-response for initialize
	go func() {
		time.Sleep(50 * time.Millisecond)
		transport.mu.Lock()
		if len(transport.writtenData) > 0 {
			var req SDKControlRequest
			if err := json.Unmarshal(transport.writtenData[0], &req); err == nil {
				transport.mu.Unlock()
				transport.injectResponse(req.RequestID, map[string]any{
					"supported_commands": []string{"hook_callback"},
				})
				return
			}
		}
		transport.mu.Unlock()
	}()

	_, err = protocol.Initialize(ctx)
	assertHookNoError(t, err)

	// Verify initialize request contained hooks
	transport.mu.Lock()
	defer transport.mu.Unlock()

	if len(transport.writtenData) == 0 {
		t.Fatal("Expected initialize request to be written")
	}

	var initReq SDKControlRequest
	err = json.Unmarshal(transport.writtenData[0], &initReq)
	assertHookNoError(t, err)

	request, ok := initReq.Request.(map[string]any)
	if !ok {
		t.Fatal("Request should be a map")
	}

	hooksConfig, ok := request["hooks"].(map[string]any)
	if !ok {
		t.Fatal("hooks should be present in initialize request")
	}

	preToolUseHooks, ok := hooksConfig["PreToolUse"].([]any)
	if !ok {
		t.Fatal("PreToolUse hooks should be present")
	}

	if len(preToolUseHooks) != 1 {
		t.Errorf("Expected 1 PreToolUse hook matcher, got %d", len(preToolUseHooks))
	}
}

// =============================================================================
// Mock Transport for Hook Tests
// =============================================================================

type hookMockTransport struct {
	mu          sync.Mutex
	writtenData [][]byte
	readChan    chan []byte
	closed      bool
}

func newHookMockTransport() *hookMockTransport {
	return &hookMockTransport{
		writtenData: make([][]byte, 0),
		readChan:    make(chan []byte, 100),
	}
}

func (t *hookMockTransport) Write(_ context.Context, data []byte) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return fmt.Errorf("transport closed")
	}

	// Store a copy to avoid data races
	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)
	t.writtenData = append(t.writtenData, dataCopy)
	return nil
}

func (t *hookMockTransport) Read(_ context.Context) <-chan []byte {
	return t.readChan
}

func (t *hookMockTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.closed {
		t.closed = true
		close(t.readChan)
	}
	return nil
}

func (t *hookMockTransport) injectResponse(requestID string, response map[string]any) {
	resp := SDKControlResponse{
		Type: MessageTypeControlResponse,
		Response: Response{
			Subtype:   ResponseSubtypeSuccess,
			RequestID: requestID,
			Response:  response,
		},
	}

	data, _ := json.Marshal(resp)
	t.readChan <- data
}

// =============================================================================
// Helper Functions
// =============================================================================

func setupHookTestContext(t *testing.T, timeout time.Duration) (context.Context, context.CancelFunc) {
	t.Helper()
	return context.WithTimeout(context.Background(), timeout)
}

func assertHookNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
}

func assertHookResponseSent(t *testing.T, transport *hookMockTransport, requestID string, expectedSubtype string) {
	t.Helper()

	transport.mu.Lock()
	defer transport.mu.Unlock()

	if len(transport.writtenData) == 0 {
		t.Fatal("Expected response to be written")
	}

	// Find the response for this request ID
	for _, data := range transport.writtenData {
		var resp SDKControlResponse
		if err := json.Unmarshal(data, &resp); err != nil {
			continue
		}

		if resp.Response.RequestID == requestID {
			if resp.Response.Subtype != expectedSubtype {
				t.Errorf("Response subtype = %q, want %q", resp.Response.Subtype, expectedSubtype)
			}
			return
		}
	}

	t.Errorf("No response found for request ID %q", requestID)
}
