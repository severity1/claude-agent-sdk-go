package control

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// DefaultInitTimeout is the default timeout for the Initialize handshake.
const DefaultInitTimeout = 60 * time.Second

// Transport abstracts the I/O operations for the control protocol.
// This allows testing with mock transports.
type Transport interface {
	// Write sends data to the CLI stdin.
	Write(ctx context.Context, data []byte) error
	// Read returns a channel that receives data from CLI stdout.
	Read(ctx context.Context) <-chan []byte
	// Close closes the transport.
	Close() error
}

// Protocol manages the bidirectional control protocol with Claude CLI.
// It handles request/response correlation, message routing, and initialization.
type Protocol struct {
	mu        sync.Mutex
	transport Transport

	// Request correlation
	pendingRequests map[string]chan *Response
	requestCounter  int64

	// Message routing
	messageStream chan map[string]any

	// State
	initialized  bool
	initResponse *InitializeResponse
	initErrChan  chan error
	closed       bool
	started      bool

	// Configuration
	initTimeout time.Duration

	// Permission callback (Issue #8)
	canUseToolCallback CanUseToolCallback

	// Hook callbacks (Issue #9)
	hooks            map[HookEvent][]HookMatcher
	hookCallbacks    map[string]HookCallback
	hookCallbacksMu  sync.RWMutex
	nextHookCallback int64

	// SDK MCP servers for in-process tool handling (Issue #7)
	sdkMcpServers map[string]McpServer

	// Background goroutine management
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// ProtocolOption configures Protocol behavior.
type ProtocolOption func(*Protocol)

// WithInitTimeout sets the initialization timeout.
func WithInitTimeout(timeout time.Duration) ProtocolOption {
	return func(p *Protocol) {
		p.initTimeout = timeout
	}
}

// WithCanUseToolCallback sets the permission callback for tool usage requests.
// The callback is invoked when CLI requests permission to use a tool.
func WithCanUseToolCallback(callback CanUseToolCallback) ProtocolOption {
	return func(p *Protocol) {
		p.canUseToolCallback = callback
	}
}

// WithHooks sets the hook configuration for lifecycle events.
// Hooks are registered during initialization and invoked by the CLI.
func WithHooks(hooks map[HookEvent][]HookMatcher) ProtocolOption {
	return func(p *Protocol) {
		p.hooks = hooks
	}
}

// WithHookCallbacks sets pre-registered hook callbacks by ID.
// This is primarily used for testing.
func WithHookCallbacks(callbacks map[string]HookCallback) ProtocolOption {
	return func(p *Protocol) {
		p.hookCallbacks = callbacks
	}
}

// WithSdkMcpServers configures SDK MCP servers for in-process tool handling.
// The servers map is keyed by server name.
func WithSdkMcpServers(servers map[string]McpServer) ProtocolOption {
	return func(p *Protocol) {
		p.sdkMcpServers = servers
	}
}

// NewProtocol creates a new control protocol handler.
func NewProtocol(transport Transport, opts ...ProtocolOption) *Protocol {
	p := &Protocol{
		transport:       transport,
		pendingRequests: make(map[string]chan *Response),
		messageStream:   make(chan map[string]any, 100),
		initTimeout:     DefaultInitTimeout,
		initErrChan:     make(chan error, 1),
	}

	for _, opt := range opts {
		opt(p)
	}

	return p
}

// Start begins the message reading goroutine.
// This must be called before sending any control requests.
func (p *Protocol) Start(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.started {
		return nil
	}

	p.ctx, p.cancel = context.WithCancel(ctx)
	p.started = true

	// Start background message reader
	p.wg.Add(1)
	go p.readLoop()

	return nil
}

// readLoop continuously reads from transport and routes messages.
func (p *Protocol) readLoop() {
	defer p.wg.Done()

	readChan := p.transport.Read(p.ctx)

	for {
		select {
		case <-p.ctx.Done():
			return
		case data, ok := <-readChan:
			if !ok {
				return
			}

			// Parse the incoming message
			var msg map[string]any
			if err := json.Unmarshal(data, &msg); err != nil {
				// Log parse error but continue
				continue
			}

			// Route the message
			if err := p.HandleIncomingMessage(p.ctx, msg); err != nil {
				// Log routing error but continue
				continue
			}
		}
	}
}

// generateRequestID creates a unique request ID matching Python SDK format.
// Format: req_{counter}_{random_hex}
func (p *Protocol) generateRequestID() string {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.requestCounter++

	// Generate 4 random bytes as hex
	randomBytes := make([]byte, 4)
	_, _ = rand.Read(randomBytes)

	return fmt.Sprintf("req_%d_%x", p.requestCounter, randomBytes)
}

// SendControlRequest sends a control request and waits for the response.
// It uses the request ID for correlation with the matching response.
func (p *Protocol) SendControlRequest(ctx context.Context, request any, timeout time.Duration) (any, error) {
	requestID := p.generateRequestID()

	// Create response channel
	responseChan := make(chan *Response, 1)

	p.mu.Lock()
	p.pendingRequests[requestID] = responseChan
	p.mu.Unlock()

	// Cleanup on exit
	defer func() {
		p.mu.Lock()
		delete(p.pendingRequests, requestID)
		p.mu.Unlock()
	}()

	// Build control request envelope
	controlReq := SDKControlRequest{
		Type:      MessageTypeControlRequest,
		RequestID: requestID,
		Request:   request,
	}

	// Serialize and send
	data, err := json.Marshal(controlReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal control request: %w", err)
	}

	// Add newline for JSON lines protocol
	data = append(data, '\n')

	if err := p.transport.Write(ctx, data); err != nil {
		return nil, fmt.Errorf("failed to send control request: %w", err)
	}

	// Wait for response with timeout
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	select {
	case response := <-responseChan:
		if response.Subtype == ResponseSubtypeError {
			return nil, fmt.Errorf("control request error: %s", response.Error)
		}
		return response.Response, nil

	case err := <-p.initErrChan:
		return nil, err

	case <-timeoutCtx.Done():
		return nil, fmt.Errorf("control request timeout: %w", timeoutCtx.Err())
	}
}

// HandleControlInitErr reports an initialization error back to any pending
// SendControlRequest, unblocking it when the CLI returns an error result
// instead of a control protocol response (e.g., invalid session ID).
func (p *Protocol) HandleControlInitErr(err error) {
	select {
	case p.initErrChan <- err:
	default:
	}
}

// HandleIncomingMessage routes incoming messages based on their type.
// Control messages are handled internally, regular messages are forwarded to the stream.
func (p *Protocol) HandleIncomingMessage(ctx context.Context, msg map[string]any) error {
	msgType, ok := msg["type"].(string)
	if !ok {
		// No type field - forward to stream for compatibility
		return p.forwardToStream(ctx, msg)
	}

	switch msgType {
	case MessageTypeControlResponse:
		return p.handleControlResponse(ctx, msg)
	case MessageTypeControlRequest:
		// Incoming control request from CLI (e.g., hook callback, permission check)
		return p.handleIncomingControlRequest(ctx, msg)
	default:
		// Regular SDK message - forward to stream
		return p.forwardToStream(ctx, msg)
	}
}

// handleIncomingControlRequest routes incoming control requests from CLI.
func (p *Protocol) handleIncomingControlRequest(ctx context.Context, msg map[string]any) error {
	request, ok := msg["request"].(map[string]any)
	if !ok {
		return fmt.Errorf("invalid control request: missing request field")
	}

	subtype, _ := request["subtype"].(string)
	requestID, _ := msg["request_id"].(string)

	switch subtype {
	case SubtypeCanUseTool:
		return p.handleCanUseToolRequest(ctx, requestID, request)
	case SubtypeHookCallback:
		return p.handleHookCallbackRequest(ctx, requestID, request)
	case SubtypeMcpMessage:
		return p.handleMcpMessageRequest(ctx, requestID, request)
	default:
		// Unknown subtype - ignore for forward compatibility
		return nil
	}
}

// handleControlResponse routes a control response to the waiting request.
func (p *Protocol) handleControlResponse(_ context.Context, msg map[string]any) error {
	responseData, ok := msg["response"].(map[string]any)
	if !ok {
		return fmt.Errorf("invalid control response: missing response field")
	}

	requestID, ok := responseData["request_id"].(string)
	if !ok {
		return fmt.Errorf("invalid control response: missing request_id")
	}

	p.mu.Lock()
	responseChan, exists := p.pendingRequests[requestID]
	p.mu.Unlock()

	if !exists {
		// Response for unknown request - ignore (could be stale or from another session)
		return nil
	}

	response := &Response{
		RequestID: requestID,
	}

	if subtype, ok := responseData["subtype"].(string); ok {
		response.Subtype = subtype
	}

	if response.Subtype == ResponseSubtypeError {
		if errMsg, ok := responseData["error"].(string); ok {
			response.Error = errMsg
		}
	} else {
		response.Response = responseData["response"]
	}

	// Send response to waiting goroutine (non-blocking)
	select {
	case responseChan <- response:
	default:
		// Channel full or closed - ignore
	}

	return nil
}

// forwardToStream sends a message to the regular message stream.
func (p *Protocol) forwardToStream(ctx context.Context, msg map[string]any) error {
	select {
	case p.messageStream <- msg:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// sendErrorResponse sends an error response back to CLI.
// This is a shared utility used by hooks, MCP, and permissions handlers.
func (p *Protocol) sendErrorResponse(ctx context.Context, requestID string, errMsg string) error {
	response := SDKControlResponse{
		Type: MessageTypeControlResponse,
		Response: Response{
			Subtype:   ResponseSubtypeError,
			RequestID: requestID,
			Error:     errMsg,
		},
	}

	data, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("failed to marshal error response: %w", err)
	}

	return p.transport.Write(ctx, append(data, '\n'))
}

// Initialize performs the control protocol handshake with the CLI.
// This must be called in streaming mode before other control operations.
// The result is cached - subsequent calls return the cached response.
func (p *Protocol) Initialize(ctx context.Context) (*InitializeResponse, error) {
	p.mu.Lock()
	if p.initialized {
		resp := p.initResponse
		p.mu.Unlock()
		return resp, nil
	}
	p.mu.Unlock()

	// Build initialize request with hooks configuration
	initReq := InitializeRequest{
		Subtype: SubtypeInitialize,
	}

	// Generate hook registrations and build hooks config
	if p.hooks != nil {
		initReq.Hooks = p.buildHooksConfig()
	}

	// Send initialize request
	result, err := p.SendControlRequest(ctx, initReq, p.initTimeout)

	if err != nil {
		return nil, fmt.Errorf("initialize failed: %w", err)
	}

	// Parse response
	var initResp InitializeResponse
	if resultMap, ok := result.(map[string]any); ok {
		if cmds, ok := resultMap["supported_commands"].([]any); ok {
			for _, cmd := range cmds {
				if cmdStr, ok := cmd.(string); ok {
					initResp.SupportedCommands = append(initResp.SupportedCommands, cmdStr)
				}
			}
		}
	}

	p.mu.Lock()
	p.initialized = true
	p.initResponse = &initResp
	p.mu.Unlock()

	return &initResp, nil
}

// Interrupt sends an interrupt control request to the CLI.
func (p *Protocol) Interrupt(ctx context.Context) error {
	_, err := p.SendControlRequest(ctx, InterruptRequest{
		Subtype: SubtypeInterrupt,
	}, 5*time.Second)

	return err
}

// SetModel changes the AI model during a streaming session.
// Pass nil to reset to the default model.
// Returns error if the control request fails or times out.
func (p *Protocol) SetModel(ctx context.Context, model *string) error {
	_, err := p.SendControlRequest(ctx, SetModelRequest{
		Subtype: SubtypeSetModel,
		Model:   model,
	}, 5*time.Second)

	return err
}

// SetPermissionMode changes the permission mode during a streaming session.
// Valid modes: "default", "accept_edits", "plan", "bypass_permissions"
// Returns error if the control request fails or times out.
func (p *Protocol) SetPermissionMode(ctx context.Context, mode string) error {
	_, err := p.SendControlRequest(ctx, SetPermissionModeRequest{
		Subtype: SubtypeSetPermissionMode,
		Mode:    mode,
	}, 5*time.Second)

	return err
}

// RewindFiles reverts tracked files to their state at a specific user message.
// The userMessageID should be the UUID from a UserMessage received during the session.
// Requires EnableFileCheckpointing to be set when creating the client.
// Returns error if the control request fails or times out.
//
// This method matches Python SDK's rewind_files behavior exactly:
// - Uses "rewind_files" subtype
// - Sends user_message_id in the request
// - Uses standard 5-second timeout
func (p *Protocol) RewindFiles(ctx context.Context, userMessageID string) error {
	_, err := p.SendControlRequest(ctx, RewindFilesRequest{
		Subtype:       SubtypeRewindFiles,
		UserMessageID: userMessageID,
	}, 5*time.Second)

	return err
}

// ReceiveMessages returns a channel for receiving regular (non-control) messages.
func (p *Protocol) ReceiveMessages() <-chan map[string]any {
	return p.messageStream
}

// IsClosed returns whether the protocol has been closed.
func (p *Protocol) IsClosed() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.closed
}

// Close shuts down the protocol handler.
func (p *Protocol) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	p.mu.Unlock()

	// Cancel background goroutines
	if p.cancel != nil {
		p.cancel()
	}

	// Wait for goroutines to finish
	p.wg.Wait()

	// Close message stream
	close(p.messageStream)

	return nil
}

// setPendingRequest adds a pending request for testing purposes.
func (p *Protocol) setPendingRequest(requestID string, responseChan chan *Response) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pendingRequests[requestID] = responseChan
}
