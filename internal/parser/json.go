// Package parser provides JSON message parsing functionality with speculative parsing and buffer management.
package parser

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/severity1/claude-agent-sdk-go/internal/shared"
)

const (
	// MaxBufferSize is the maximum buffer size to prevent memory exhaustion (1MB).
	MaxBufferSize = 1024 * 1024
)

// Parser handles JSON message parsing with speculative parsing and buffer management.
type Parser struct {
	buffer        strings.Builder
	maxBufferSize int
	mu            sync.Mutex // Thread safety
}

// New creates a new JSON parser with default buffer size.
func New() *Parser {
	return &Parser{
		maxBufferSize: MaxBufferSize,
	}
}

// NewWithSize creates a new JSON parser with a custom maximum buffer size.
func NewWithSize(maxBufferSize int) *Parser {
	return &Parser{
		maxBufferSize: maxBufferSize,
	}
}

// ProcessLine processes a line of JSON input with speculative parsing.
// Handles multiple JSON objects on single line and embedded newlines.
func (p *Parser) ProcessLine(line string) ([]shared.Message, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	line = strings.TrimSpace(line)
	if line == "" {
		return nil, nil
	}

	var messages []shared.Message

	// Handle multiple JSON objects on single line by splitting on newlines
	jsonLines := strings.Split(line, "\n")
	for _, jsonLine := range jsonLines {
		jsonLine = strings.TrimSpace(jsonLine)
		if jsonLine == "" {
			continue
		}

		// Process each JSON line with speculative parsing (unlocked version)
		msg, err := p.processJSONLineUnlocked(jsonLine)
		if err != nil {
			return messages, err
		}
		if msg != nil {
			messages = append(messages, msg)
		}
	}

	return messages, nil
}

// ParseMessage parses a raw JSON object into the appropriate Message type.
// Implements type discrimination based on the "type" field. An unknown type
// returns a nil Message and a nil error, so a newer CLI does not break the stream.
func (p *Parser) ParseMessage(data map[string]any) (shared.Message, error) {
	msgType, ok := data["type"].(string)
	if !ok {
		return nil, shared.NewMessageParseError("missing or invalid type field", data)
	}

	switch msgType {
	case shared.MessageTypeUser:
		return p.parseUserMessage(data)
	case shared.MessageTypeAssistant:
		return p.parseAssistantMessage(data)
	case shared.MessageTypeSystem:
		return p.parseSystemMessage(data)
	case shared.MessageTypeResult:
		return p.parseResultMessage(data)
	case shared.MessageTypeControlRequest, shared.MessageTypeControlResponse:
		// Control messages are passed through as raw data for the control protocol handler
		return &shared.RawControlMessage{
			MessageType: msgType,
			Data:        data,
		}, nil
	case shared.MessageTypeStreamEvent:
		return p.parseStreamEventMessage(data)
	case shared.MessageTypeRateLimitEvent:
		return p.parseRateLimitEventMessage(data)
	case shared.MessageTypeConversationReset:
		return parseConversationResetMessage(data)
	default:
		return nil, nil
	}
}

// Reset clears the internal buffer.
func (p *Parser) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.buffer.Reset()
}

// BufferSize returns the current buffer size.
func (p *Parser) BufferSize() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.buffer.Len()
}

// NewBufferOverflowError returns the error for a stdout message longer than
// limit bytes. The text matches the Python SDK, so callers can match on it.
func NewBufferOverflowError(limit int, cause error) *shared.JSONDecodeError {
	return shared.NewJSONDecodeError(
		fmt.Sprintf("JSON message exceeded maximum buffer size of %d bytes", limit),
		0,
		cause,
	)
}

// processJSONLine attempts to parse accumulated buffer as JSON using speculative parsing.
// This is the core of the speculative parsing strategy from the Python SDK.
func (p *Parser) processJSONLine(jsonLine string) (shared.Message, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.processJSONLineUnlocked(jsonLine)
}

// processJSONLineUnlocked is the unlocked version of processJSONLine.
// Must be called with mutex already held.
func (p *Parser) processJSONLineUnlocked(jsonLine string) (shared.Message, error) {
	p.buffer.WriteString(jsonLine)

	// Check buffer size limit
	if p.buffer.Len() > p.maxBufferSize {
		bufferSize := p.buffer.Len()
		p.buffer.Reset()
		return nil, NewBufferOverflowError(
			p.maxBufferSize,
			fmt.Errorf("buffer size %d exceeds limit %d", bufferSize, p.maxBufferSize),
		)
	}

	// Attempt speculative JSON parsing
	var rawData map[string]any
	bufferContent := p.buffer.String()

	if err := json.Unmarshal([]byte(bufferContent), &rawData); err != nil {
		// JSON is incomplete - continue accumulating
		// This is NOT an error condition in speculative parsing!
		return nil, nil
	}

	// Successfully parsed complete JSON - reset buffer and parse message
	p.buffer.Reset()
	return p.ParseMessage(rawData)
}

// parseUserMessage parses a user message from raw JSON data.
func (p *Parser) parseUserMessage(data map[string]any) (*shared.UserMessage, error) {
	messageData, ok := data["message"].(map[string]any)
	if !ok {
		return nil, shared.NewMessageParseError("user message missing message field", data)
	}

	content := messageData["content"]
	if content == nil {
		return nil, shared.NewMessageParseError("user message missing content field", data)
	}

	// Extract optional top-level fields (following Python SDK pattern)
	var uuid *string
	if u, ok := data["uuid"].(string); ok {
		uuid = &u
	}

	var parentToolUseID *string
	if ptid, ok := data["parent_tool_use_id"].(string); ok {
		parentToolUseID = &ptid
	}

	var toolUseResult map[string]any
	if tur, ok := data["tool_use_result"].(map[string]any); ok {
		toolUseResult = tur
	}

	// Handle both string content and array of content blocks
	switch c := content.(type) {
	case string:
		// String content - create directly
		return &shared.UserMessage{
			Content:         c,
			UUID:            uuid,
			ParentToolUseID: parentToolUseID,
			ToolUseResult:   toolUseResult,
		}, nil
	case []any:
		// Array of content blocks
		blocks, err := p.parseContentBlocks(c)
		if err != nil {
			return nil, err
		}
		return &shared.UserMessage{
			Content:         blocks,
			UUID:            uuid,
			ParentToolUseID: parentToolUseID,
			ToolUseResult:   toolUseResult,
		}, nil
	default:
		return nil, shared.NewMessageParseError("invalid user message content type", data)
	}
}

// parseAssistantMessage parses an assistant message from raw JSON data.
func (p *Parser) parseAssistantMessage(data map[string]any) (*shared.AssistantMessage, error) {
	messageData, ok := data["message"].(map[string]any)
	if !ok {
		return nil, shared.NewMessageParseError("assistant message missing message field", data)
	}

	contentArray, ok := messageData["content"].([]any)
	if !ok {
		return nil, shared.NewMessageParseError("assistant message content must be array", data)
	}

	model, ok := messageData["model"].(string)
	if !ok {
		return nil, shared.NewMessageParseError("assistant message missing model field", data)
	}

	blocks, err := p.parseContentBlocks(contentArray)
	if err != nil {
		return nil, err
	}

	// Parse optional error field from top-level data, not the nested message object.
	// Wire format: {"type":"assistant","error":"rate_limit","message":{...}}.
	var errorPtr *shared.AssistantMessageError
	if errorStr, ok := data["error"].(string); ok {
		errType := shared.AssistantMessageError(errorStr)
		errorPtr = &errType
	}

	// parent_tool_use_id is set on assistant messages produced inside a subagent
	// (Agent/Task tool). Lives at the top-level of the raw event.
	var parentToolUseID *string
	if ptid, ok := data["parent_tool_use_id"].(string); ok {
		parentToolUseID = &ptid
	}

	// usage is nested under the message object (unlike ResultMessage, where it
	// is top-level): {"type":"assistant","message":{...,"usage":{...}}}.
	var usage *map[string]any
	if u, ok := messageData["usage"].(map[string]any); ok {
		usage = &u
	}

	return &shared.AssistantMessage{
		Content:         blocks,
		Model:           model,
		Error:           errorPtr,
		ParentToolUseID: parentToolUseID,
		Usage:           usage,
	}, nil
}

// parseSystemMessage parses a system message from raw JSON data. A
// task_started, task_progress or task_notification message that lacks a
// required field is a parse error; the typed forms come from the
// SystemMessage's AsTask* methods.
func (p *Parser) parseSystemMessage(data map[string]any) (*shared.SystemMessage, error) {
	subtype, ok := data["subtype"].(string)
	if !ok {
		return nil, shared.NewMessageParseError("system message missing subtype field", data)
	}

	msg := &shared.SystemMessage{
		Subtype: subtype,
		Data:    data, // Preserve all original data
	}
	if err := shared.ValidateTaskMessage(msg); err != nil {
		return nil, err
	}
	return msg, nil
}

// parseResultMessage parses a result message from raw JSON data.
func (p *Parser) parseResultMessage(data map[string]any) (*shared.ResultMessage, error) {
	result := &shared.ResultMessage{}

	// Required fields with validation
	if subtype, ok := data["subtype"].(string); ok {
		result.Subtype = subtype
	} else {
		return nil, shared.NewMessageParseError("result message missing subtype field", data)
	}

	if durationMS, ok := data["duration_ms"].(float64); ok {
		result.DurationMs = int(durationMS)
	} else {
		return nil, shared.NewMessageParseError("result message missing or invalid duration_ms field", data)
	}

	if durationAPIMS, ok := data["duration_api_ms"].(float64); ok {
		result.DurationAPIMs = int(durationAPIMS)
	} else {
		return nil, shared.NewMessageParseError("result message missing or invalid duration_api_ms field", data)
	}

	if isError, ok := data["is_error"].(bool); ok {
		result.IsError = isError
	} else {
		return nil, shared.NewMessageParseError("result message missing or invalid is_error field", data)
	}

	if numTurns, ok := data["num_turns"].(float64); ok {
		result.NumTurns = int(numTurns)
	} else {
		return nil, shared.NewMessageParseError("result message missing or invalid num_turns field", data)
	}

	if sessionID, ok := data["session_id"].(string); ok {
		result.SessionID = sessionID
	} else {
		return nil, shared.NewMessageParseError("result message missing session_id field", data)
	}

	parseResultOptionalFields(result, data)
	return result, nil
}

// parseResultOptionalFields sets the fields that can be absent; a wrong type leaves the field unset.
func parseResultOptionalFields(result *shared.ResultMessage, data map[string]any) {
	if stopReason, ok := data["stop_reason"].(string); ok {
		result.StopReason = &stopReason
	}

	if totalCostUSD, ok := data["total_cost_usd"].(float64); ok {
		result.TotalCostUSD = &totalCostUSD
	}

	if usage, ok := data["usage"].(map[string]any); ok {
		result.Usage = &usage
	}

	if resultData, ok := data["result"]; ok {
		if resultStr, ok := resultData.(string); ok {
			result.Result = &resultStr
		}
	}

	// Parse structured_output (any JSON value)
	if structuredOutput, exists := data["structured_output"]; exists {
		result.StructuredOutput = structuredOutput
	}

	// Parse errors array
	if errorsRaw, ok := data["errors"].([]any); ok {
		for _, e := range errorsRaw {
			if s, ok := e.(string); ok {
				result.Errors = append(result.Errors, s)
			}
		}
	}
}

// parseContentBlocks parses content blocks and drops blocks of an unknown type.
func (p *Parser) parseContentBlocks(raw []any) ([]shared.ContentBlock, error) {
	blocks := make([]shared.ContentBlock, 0, len(raw))
	for i, blockData := range raw {
		block, err := p.parseContentBlock(blockData)
		if err != nil {
			return nil, fmt.Errorf("failed to parse content block %d: %w", i, err)
		}
		if block != nil {
			blocks = append(blocks, block)
		}
	}
	return blocks, nil
}

// parseContentBlock parses a content block based on its type field. An
// unknown type returns a nil block and a nil error.
func (p *Parser) parseContentBlock(blockData any) (shared.ContentBlock, error) {
	data, ok := blockData.(map[string]any)
	if !ok {
		return nil, shared.NewMessageParseError("content block must be an object", blockData)
	}

	blockType, ok := data["type"].(string)
	if !ok {
		return nil, shared.NewMessageParseError("content block missing type field", data)
	}

	switch blockType {
	case shared.ContentBlockTypeText:
		return p.parseTextBlock(data)
	case shared.ContentBlockTypeThinking:
		return p.parseThinkingBlock(data)
	case shared.ContentBlockTypeToolUse:
		return p.parseToolUseBlock(data)
	case shared.ContentBlockTypeToolResult:
		return p.parseToolResultBlock(data)
	case shared.ContentBlockTypeServerToolUse:
		return parseServerToolUseBlock(data)
	case shared.ContentBlockTypeAdvisorToolResult:
		return parseServerToolResultBlock(data)
	default:
		return nil, nil
	}
}

func (p *Parser) parseTextBlock(data map[string]any) (shared.ContentBlock, error) {
	text, ok := data["text"].(string)
	if !ok {
		return nil, shared.NewMessageParseError("text block missing text field", data)
	}
	return &shared.TextBlock{Text: text}, nil
}

func (p *Parser) parseThinkingBlock(data map[string]any) (shared.ContentBlock, error) {
	thinking, ok := data["thinking"].(string)
	if !ok {
		return nil, shared.NewMessageParseError("thinking block missing thinking field", data)
	}
	signature, _ := data["signature"].(string) // Optional field
	return &shared.ThinkingBlock{
		Thinking:  thinking,
		Signature: signature,
	}, nil
}

func (p *Parser) parseToolUseBlock(data map[string]any) (shared.ContentBlock, error) {
	id, ok := data["id"].(string)
	if !ok {
		return nil, shared.NewMessageParseError("tool_use block missing id field", data)
	}
	name, ok := data["name"].(string)
	if !ok {
		return nil, shared.NewMessageParseError("tool_use block missing name field", data)
	}
	input, _ := data["input"].(map[string]any) // Optional field
	if input == nil {
		input = make(map[string]any)
	}
	return &shared.ToolUseBlock{
		ToolUseID: id,
		Name:      name,
		Input:     input,
	}, nil
}

func parseServerToolUseBlock(data map[string]any) (shared.ContentBlock, error) {
	id, ok := data["id"].(string)
	if !ok {
		return nil, shared.NewMessageParseError("server_tool_use block missing id field", data)
	}
	name, ok := data["name"].(string)
	if !ok {
		return nil, shared.NewMessageParseError("server_tool_use block missing name field", data)
	}
	input, ok := data["input"].(map[string]any)
	if !ok {
		return nil, shared.NewMessageParseError("server_tool_use block missing input field", data)
	}
	return &shared.ServerToolUseBlock{
		MessageType: shared.ContentBlockTypeServerToolUse,
		ID:          id,
		Name:        shared.ServerToolName(name),
		Input:       input,
	}, nil
}

func parseServerToolResultBlock(data map[string]any) (shared.ContentBlock, error) {
	toolUseID, ok := data["tool_use_id"].(string)
	if !ok {
		return nil, shared.NewMessageParseError("advisor_tool_result block missing tool_use_id field", data)
	}
	content, ok := data["content"].(map[string]any)
	if !ok {
		return nil, shared.NewMessageParseError("advisor_tool_result block missing content field", data)
	}
	return &shared.ServerToolResultBlock{
		MessageType: shared.ContentBlockTypeAdvisorToolResult,
		ToolUseID:   toolUseID,
		Content:     content,
	}, nil
}

func (p *Parser) parseToolResultBlock(data map[string]any) (shared.ContentBlock, error) {
	toolUseID, ok := data["tool_use_id"].(string)
	if !ok {
		return nil, shared.NewMessageParseError("tool_result block missing tool_use_id field", data)
	}

	var isError *bool
	if isErrorValue, exists := data["is_error"]; exists {
		if b, ok := isErrorValue.(bool); ok {
			isError = &b
		}
	}

	return &shared.ToolResultBlock{
		ToolUseID: toolUseID,
		Content:   data["content"],
		IsError:   isError,
	}, nil
}

func parseConversationResetMessage(data map[string]any) (*shared.ConversationResetMessage, error) {
	fields := [3]string{"new_conversation_id", "uuid", "session_id"}
	var values [3]string
	for i, field := range fields {
		value, ok := data[field].(string)
		if !ok {
			return nil, shared.NewMessageParseError(
				fmt.Sprintf("conversation_reset message missing %s field", field), data)
		}
		values[i] = value
	}
	return &shared.ConversationResetMessage{
		MessageType:       shared.MessageTypeConversationReset,
		NewConversationID: values[0],
		UUID:              values[1],
		SessionID:         values[2],
	}, nil
}

// parseRateLimitEventMessage parses a rate_limit_event message from raw JSON
// data. Tolerant on optional fields: only the rate_limit_info object is
// required; uuid / session_id are best-effort copies because the CLI does
// not always include them depending on session state.
func (p *Parser) parseRateLimitEventMessage(data map[string]any) (*shared.RateLimitEventMessage, error) {
	infoRaw, ok := data["rate_limit_info"].(map[string]any)
	if !ok {
		return nil, shared.NewMessageParseError("rate_limit_event missing rate_limit_info field", data)
	}

	info := shared.RateLimitInfo{}
	if s, ok := infoRaw["status"].(string); ok {
		info.Status = s
	}
	if f, ok := infoRaw["resetsAt"].(float64); ok {
		info.ResetsAt = int64(f)
	}
	if s, ok := infoRaw["rateLimitType"].(string); ok {
		info.RateLimitType = s
	}
	if s, ok := infoRaw["overageStatus"].(string); ok {
		info.OverageStatus = s
	}
	if f, ok := infoRaw["overageResetsAt"].(float64); ok {
		info.OverageResetsAt = int64(f)
	}
	if b, ok := infoRaw["isUsingOverage"].(bool); ok {
		info.IsUsingOverage = b
	}
	if f, ok := infoRaw["utilization"].(float64); ok {
		info.Utilization = &f
	}
	if s, ok := infoRaw["overageDisabledReason"].(string); ok {
		info.OverageDisabledReason = s
	}
	info.Raw = infoRaw

	msg := &shared.RateLimitEventMessage{
		RateLimitInfo: info,
	}
	if s, ok := data["uuid"].(string); ok {
		msg.UUID = s
	}
	if s, ok := data["session_id"].(string); ok {
		msg.SessionID = s
	}
	return msg, nil
}

// parseStreamEventMessage parses a stream event message from raw JSON data.
func (p *Parser) parseStreamEventMessage(data map[string]any) (*shared.StreamEvent, error) {
	uuid, ok := data["uuid"].(string)
	if !ok {
		return nil, shared.NewMessageParseError("stream_event missing uuid field", data)
	}

	sessionID, ok := data["session_id"].(string)
	if !ok {
		return nil, shared.NewMessageParseError("stream_event missing session_id field", data)
	}

	event, ok := data["event"].(map[string]any)
	if !ok {
		return nil, shared.NewMessageParseError("stream_event missing event field", data)
	}

	var parentToolUseID *string
	if ptid, ok := data["parent_tool_use_id"].(string); ok {
		parentToolUseID = &ptid
	}

	return &shared.StreamEvent{
		UUID:            uuid,
		SessionID:       sessionID,
		Event:           event,
		ParentToolUseID: parentToolUseID,
	}, nil
}

// ParseMessages is a convenience function to parse multiple JSON lines.
func ParseMessages(lines []string) ([]shared.Message, error) {
	parser := New()
	var allMessages []shared.Message

	for i, line := range lines {
		messages, err := parser.ProcessLine(line)
		if err != nil {
			return allMessages, fmt.Errorf("error parsing line %d: %w", i, err)
		}
		allMessages = append(allMessages, messages...)
	}

	return allMessages, nil
}
