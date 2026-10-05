package control

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"
)

// DefaultInitTimeout is the default timeout for the Initialize handshake.
const DefaultInitTimeout = 60 * time.Second

// ErrProtocolClosed is returned by a control request that is sent after Close,
// or that is still waiting for its response when Close runs.
var ErrProtocolClosed = errors.New("control protocol closed")

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

	// inflightRequests maps an incoming control request ID to the cancel func
	// of its handler goroutine (Python: Query._inflight_requests).
	inflightRequests map[string]context.CancelFunc

	// Message routing
	messageStream chan map[string]any

	// State
	initialized  bool
	initOnce     sync.Once
	initErr      error
	initResponse *InitializeResponse
	initResult   map[string]any // the whole initialize response
	initErrChan  chan error
	closed       bool
	closedCh     chan struct{} // closed by Close; wakes pending requests
	started      bool

	// Configuration
	initTimeout time.Duration

	// Permission callback
	canUseToolCallback CanUseToolCallback

	// Hook callbacks
	hooks            map[HookEvent][]HookMatcher
	hookCallbacks    map[string]HookCallback
	hookCallbacksMu  sync.RWMutex
	nextHookCallback int64

	// SDK MCP servers for in-process tool handling
	sdkMcpServers map[string]McpServer

	// agents travel in the initialize control request, bypassing argv size limits.
	agents map[string]any

	// skills is sent in initialize only when set; nil means no filter.
	skills *[]string

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

// WithAgents configures agent definitions to be sent in the initialize
// request. Agents travel via the control protocol over stdin so the
// payload size is bounded by stdin buffering rather than argv limits.
func WithAgents(agents map[string]any) ProtocolOption {
	return func(p *Protocol) {
		p.agents = agents
	}
}

// WithSkills configures the Skills filter sent in the initialize request.
// An empty list disables all Skills.
func WithSkills(skills []string) ProtocolOption {
	return func(p *Protocol) {
		list := append([]string{}, skills...)
		p.skills = &list
	}
}

// NewProtocol creates a new control protocol handler.
func NewProtocol(transport Transport, opts ...ProtocolOption) *Protocol {
	p := &Protocol{
		transport:        transport,
		pendingRequests:  make(map[string]chan *Response),
		inflightRequests: make(map[string]context.CancelFunc),
		messageStream:    make(chan map[string]any, 100),
		initTimeout:      DefaultInitTimeout,
		initErrChan:      make(chan error, 1),
		closedCh:         make(chan struct{}),
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
				fmt.Fprintf(os.Stderr, "claude-agent-sdk: failed to parse control message: %v\n", err)
				continue
			}

			// Route the message
			if err := p.HandleIncomingMessageAsync(p.ctx, msg); err != nil {
				fmt.Fprintf(os.Stderr, "claude-agent-sdk: failed to route control message: %v\n", err)
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
	if p.closed {
		p.mu.Unlock()
		return nil, ErrProtocolClosed
	}
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
		if response.failure != nil {
			return nil, response.failure
		}
		if response.Subtype == ResponseSubtypeError {
			return nil, fmt.Errorf("control request error: %s", response.Error)
		}
		return response.Response, nil

	case err := <-p.initErrChan:
		return nil, err

	case <-p.closedCh:
		return nil, ErrProtocolClosed

	case <-timeoutCtx.Done():
		return nil, fmt.Errorf("control request timeout: %w", timeoutCtx.Err())
	}
}

// HandleControlInitErr reports an initialization error back to any pending
// SendControlRequest, unblocking it when the CLI returns an error result
// instead of a control protocol response (e.g., invalid session ID).
//
// No-op once the handshake has succeeded: a late stdout-close notification
// after a successful Initialize must not poison `initErrChan` for the next
// SendControlRequest (e.g. SetModel/GetMcpStatus from a long-lived client).
func (p *Protocol) HandleControlInitErr(err error) {
	p.mu.Lock()
	initialized := p.initialized
	p.mu.Unlock()
	if initialized {
		return
	}
	select {
	case p.initErrChan <- err:
	default:
	}
}

// FailPendingRequests fails every control request still waiting for a response
// with err, because the CLI's stream ended with that error (Python: the reader
// sets its error on every pending request). A request still waiting for the
// initialize handshake is left alone: HandleControlInitErr already fails it.
func (p *Protocol) FailPendingRequests(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.initialized {
		return
	}
	for requestID, responseChan := range p.pendingRequests {
		select {
		case responseChan <- &Response{RequestID: requestID, failure: err}:
		default:
		}
	}
}

// HandleIncomingMessage routes incoming messages based on their type.
// Control messages are handled internally, regular messages are forwarded to the stream.
// Incoming control requests run on the caller's goroutine; read loops must use
// HandleIncomingMessageAsync instead.
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

// HandleIncomingMessageAsync routes messages like HandleIncomingMessage, but runs
// each incoming control request on its own goroutine, so a slow hook, permission
// or SDK MCP callback cannot stall the reader (Python: spawn_task per request).
// Control responses and regular messages are still routed inline, in read order.
func (p *Protocol) HandleIncomingMessageAsync(ctx context.Context, msg map[string]any) error {
	if msgType, _ := msg["type"].(string); msgType != MessageTypeControlRequest {
		return p.HandleIncomingMessage(ctx, msg)
	}

	requestID, _ := msg["request_id"].(string)
	requestCtx, cancel := context.WithCancel(ctx)
	if !p.trackInflight(requestID, cancel) {
		cancel()
		return nil
	}

	go func() {
		defer p.untrackInflight(requestID)
		defer cancel()
		defer func() {
			if r := recover(); r != nil {
				p.replyHandlerFailure(requestCtx, requestID, fmt.Errorf("control request handler panicked: %v", r))
			}
		}()
		if err := p.handleIncomingControlRequest(requestCtx, msg); err != nil {
			p.replyHandlerFailure(requestCtx, requestID, err)
		}
	}()
	return nil
}

// replyHandlerFailure answers the CLI with an error so its request does not
// hang. A cancelled handler writes nothing: the CLI or Close abandoned it.
func (p *Protocol) replyHandlerFailure(ctx context.Context, requestID string, err error) {
	if ctx.Err() != nil {
		return
	}
	fmt.Fprintf(os.Stderr, "claude-agent-sdk: failed to handle control request: %v\n", err)
	if requestID == "" {
		return
	}
	if writeErr := p.sendErrorResponse(ctx, requestID, err.Error()); writeErr != nil {
		fmt.Fprintf(os.Stderr, "claude-agent-sdk: failed to send control error response: %v\n", writeErr)
	}
}

// trackInflight registers a handler's cancel func. Returns false after Close.
func (p *Protocol) trackInflight(requestID string, cancel context.CancelFunc) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return false
	}
	if requestID != "" {
		p.inflightRequests[requestID] = cancel
	}
	return true
}

func (p *Protocol) untrackInflight(requestID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.inflightRequests, requestID)
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
		// Python raises here, which sends an error response to the CLI.
		return fmt.Errorf("unsupported control request subtype: %s", subtype)
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
		fmt.Fprintf(os.Stderr, "claude-agent-sdk: response channel full or closed, dropping response for request %s\n", requestID)
	}

	return nil
}

// forwardToStream sends a message to the regular message stream.
// Returns an error if the buffer is full rather than blocking readLoop.
func (p *Protocol) forwardToStream(ctx context.Context, msg map[string]any) error {
	select {
	case p.messageStream <- msg:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		return fmt.Errorf("message stream buffer full, dropping message")
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

	return p.writeControlResponse(ctx, data)
}

// writeControlResponse writes a response line unless the handler was cancelled:
// the CLI or Close abandoned the request, so it gets no reply (Python parity).
func (p *Protocol) writeControlResponse(ctx context.Context, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return p.transport.Write(ctx, append(data, '\n'))
}

// Initialize performs the control protocol handshake with the CLI.
// This must be called in streaming mode before other control operations.
// The result is cached via sync.Once - concurrent and subsequent calls return the cached
// response. If the first call fails, the error is also cached permanently; subsequent
// calls return the same error and will not retry even with a fresh context.
func (p *Protocol) Initialize(ctx context.Context) (*InitializeResponse, error) {
	p.initOnce.Do(func() {
		// Hooks is always assigned (nil when none registered) so the
		// wire body always carries the `"hooks"` key.
		initReq := InitializeRequest{
			Subtype: SubtypeInitialize,
			Hooks:   p.buildHooksConfig(),
		}

		if len(p.agents) > 0 {
			initReq.Agents = p.agents
		}
		initReq.Skills = p.skills

		// Send initialize request
		result, err := p.SendControlRequest(ctx, initReq, p.initTimeout)
		if err != nil {
			p.initErr = fmt.Errorf("initialize failed: %w", err)
			return
		}

		// Parse response
		var initResp InitializeResponse
		resultMap, _ := result.(map[string]any)
		if cmds, ok := resultMap["supported_commands"].([]any); ok {
			for _, cmd := range cmds {
				if cmdStr, ok := cmd.(string); ok {
					initResp.SupportedCommands = append(initResp.SupportedCommands, cmdStr)
				}
			}
		}

		p.mu.Lock()
		p.initialized = true
		p.initResponse = &initResp
		p.initResult = resultMap
		p.mu.Unlock()
	})

	p.mu.Lock()
	resp := p.initResponse
	err := p.initErr
	p.mu.Unlock()

	return resp, err
}

// InitializationResult returns the initialize response the CLI sent: its
// commands, output styles, models, account and other capabilities (Python:
// Query._initialization_result). It is nil before the handshake completes.
// Each call returns a copy the caller may modify.
func (p *Protocol) InitializationResult() map[string]any {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.initResult == nil {
		return nil
	}
	return copyJSONValue(p.initResult).(map[string]any)
}

// copyJSONValue deep-copies a value decoded by encoding/json into any.
func copyJSONValue(v any) any {
	switch v := v.(type) {
	case map[string]any:
		m := make(map[string]any, len(v))
		for k, e := range v {
			m[k] = copyJSONValue(e)
		}
		return m
	case []any:
		s := make([]any, len(v))
		for i, e := range v {
			s[i] = copyJSONValue(e)
		}
		return s
	default:
		return v
	}
}

// IsInitialized reports whether the initialize handshake completed.
func (p *Protocol) IsInitialized() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.initialized
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

// GetMcpStatus returns the connection status of all configured MCP servers.
func (p *Protocol) GetMcpStatus(ctx context.Context) (*McpStatusResponse, error) {
	result, err := p.SendControlRequest(ctx, NewGetMcpStatusRequest(), 5*time.Second)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, fmt.Errorf("mcp status response: CLI returned empty response")
	}
	// SendControlRequest returns Response.Response as any (map[string]any from JSON).
	// Re-marshal + unmarshal into typed struct - necessary because SendControlRequest
	// returns any and there is no generic typed variant.
	data, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("marshal mcp status response: %w", err)
	}
	var resp McpStatusResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal mcp status response: %w", err)
	}
	return &resp, nil
}

// RewindFiles reverts tracked files to their state at a specific user message.
// The userMessageID should be the UUID from a UserMessage received during the session.
// Requires EnableFileCheckpointing to be set when creating the client.
// Returns error if the control request fails or times out.
func (p *Protocol) RewindFiles(ctx context.Context, userMessageID string) error {
	_, err := p.SendControlRequest(ctx, RewindFilesRequest{
		Subtype:       SubtypeRewindFiles,
		UserMessageID: userMessageID,
	}, 5*time.Second)

	return err
}

// StopTask stops a single running task by the task_id from its task_started
// system message. Returns error if the control request fails or times out.
func (p *Protocol) StopTask(ctx context.Context, taskID string) error {
	_, err := p.SendControlRequest(ctx, StopTaskRequest{
		Subtype: SubtypeStopTask,
		TaskID:  taskID,
	}, 5*time.Second)

	return err
}

// mcpControlTimeout is longer than other requests because a reconnect starts the server process.
const mcpControlTimeout = 60 * time.Second

// ReconnectMcpServer reconnects a disconnected or failed MCP server.
// Returns error if the CLI rejects the request or the request times out.
func (p *Protocol) ReconnectMcpServer(ctx context.Context, serverName string) error {
	if serverName == "" {
		return fmt.Errorf("reconnect mcp server: server name is empty")
	}
	_, err := p.SendControlRequest(ctx, McpReconnectRequest{
		Subtype:    SubtypeMcpReconnect,
		ServerName: serverName,
	}, mcpControlTimeout)

	return err
}

// ToggleMcpServer enables or disables an MCP server.
// Returns error if the CLI rejects the request or the request times out.
func (p *Protocol) ToggleMcpServer(ctx context.Context, serverName string, enabled bool) error {
	if serverName == "" {
		return fmt.Errorf("toggle mcp server: server name is empty")
	}
	_, err := p.SendControlRequest(ctx, McpToggleRequest{
		Subtype:    SubtypeMcpToggle,
		ServerName: serverName,
		Enabled:    enabled,
	}, mcpControlTimeout)

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
	close(p.closedCh)
	// Cancel in-flight handlers without waiting: a user callback that ignores
	// ctx must not block Close (Python close() cancels child tasks too).
	for requestID, cancel := range p.inflightRequests {
		cancel()
		delete(p.inflightRequests, requestID)
	}
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
