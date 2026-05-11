// Package control hook callback handling and registration.
// This file contains hook lifecycle event processing for PreToolUse, PostToolUse, etc.
package control

import (
	"context"
	"encoding/json"
	"fmt"
)

// handleHookCallbackRequest processes a hook callback request from CLI.
// Follows the same pattern as handleCanUseToolRequest with panic recovery.
func (p *Protocol) handleHookCallbackRequest(ctx context.Context, requestID string, request map[string]any) error {
	// Parse callback ID
	callbackID, _ := request["callback_id"].(string)
	if callbackID == "" {
		return p.sendErrorResponse(ctx, requestID, "missing callback_id")
	}

	// Parse hook event name from input
	inputData, _ := request["input"].(map[string]any)
	if inputData == nil {
		inputData = make(map[string]any)
	}

	eventName, _ := inputData["hook_event_name"].(string)
	event := HookEvent(eventName)

	// Parse input based on event type
	input := p.parseHookInput(event, inputData)

	// Parse tool_use_id if present
	var toolUseID *string
	if id, ok := request["tool_use_id"].(string); ok {
		toolUseID = &id
	}

	// Get callback (thread-safe read)
	p.hookCallbacksMu.RLock()
	callback, exists := p.hookCallbacks[callbackID]
	p.hookCallbacksMu.RUnlock()

	if !exists {
		return p.sendErrorResponse(ctx, requestID, fmt.Sprintf("callback not found: %s", callbackID))
	}

	// Create hook context
	hookCtx := HookContext{Signal: ctx}

	// Invoke callback with panic recovery (matches permission callback pattern)
	var result HookJSONOutput
	var callbackErr error
	func() {
		defer func() {
			if r := recover(); r != nil {
				callbackErr = fmt.Errorf("hook callback panicked: %v", r)
			}
		}()
		result, callbackErr = callback(ctx, input, toolUseID, hookCtx)
	}()

	if callbackErr != nil {
		return p.sendErrorResponse(ctx, requestID, fmt.Sprintf("callback error: %v", callbackErr))
	}

	return p.sendHookResponse(ctx, requestID, result)
}

// parseHookInput creates the appropriate typed input based on event type.
// Returns the strongly-typed input struct for the callback.
func (p *Protocol) parseHookInput(event HookEvent, inputData map[string]any) any {
	// Parse base fields
	base := BaseHookInput{
		SessionID:      getString(inputData, "session_id"),
		TranscriptPath: getString(inputData, "transcript_path"),
		Cwd:            getString(inputData, "cwd"),
		PermissionMode: getString(inputData, "permission_mode"),
	}

	switch event {
	case HookEventPreToolUse:
		return &PreToolUseHookInput{
			BaseHookInput: base,
			HookEventName: "PreToolUse",
			ToolName:      getString(inputData, "tool_name"),
			ToolInput:     getMap(inputData, "tool_input"),
		}
	case HookEventPostToolUse:
		return &PostToolUseHookInput{
			BaseHookInput: base,
			HookEventName: "PostToolUse",
			ToolName:      getString(inputData, "tool_name"),
			ToolInput:     getMap(inputData, "tool_input"),
			ToolResponse:  inputData["tool_response"],
		}
	case HookEventPostToolUseFailure:
		return &PostToolUseFailureHookInput{
			BaseHookInput: base,
			HookEventName: "PostToolUseFailure",
			ToolName:      getString(inputData, "tool_name"),
			ToolInput:     getMap(inputData, "tool_input"),
			ToolUseID:     getString(inputData, "tool_use_id"),
			Error:         getString(inputData, "error"),
			IsInterrupt:   getBoolPtr(inputData, "is_interrupt"),
		}
	case HookEventUserPromptSubmit:
		return &UserPromptSubmitHookInput{
			BaseHookInput: base,
			HookEventName: "UserPromptSubmit",
			Prompt:        getString(inputData, "prompt"),
		}
	case HookEventStop:
		return &StopHookInput{
			BaseHookInput:  base,
			HookEventName:  "Stop",
			StopHookActive: getBool(inputData, "stop_hook_active"),
		}
	case HookEventSubagentStop:
		return &SubagentStopHookInput{
			BaseHookInput:  base,
			HookEventName:  "SubagentStop",
			StopHookActive: getBool(inputData, "stop_hook_active"),
		}
	case HookEventPreCompact:
		return &PreCompactHookInput{
			BaseHookInput:      base,
			HookEventName:      "PreCompact",
			Trigger:            getString(inputData, "trigger"),
			CustomInstructions: getStringPtr(inputData, "custom_instructions"),
		}
	default:
		// Forward compatibility - return raw input for unknown events
		return inputData
	}
}

// sendHookResponse sends a hook callback response back to CLI.
func (p *Protocol) sendHookResponse(ctx context.Context, requestID string, result HookJSONOutput) error {
	// Build response data from HookJSONOutput
	responseData := make(map[string]any)

	if result.Continue != nil {
		responseData["continue"] = *result.Continue
	}
	if result.SuppressOutput != nil {
		responseData["suppressOutput"] = *result.SuppressOutput
	}
	if result.StopReason != nil {
		responseData["stopReason"] = *result.StopReason
	}
	if result.Decision != nil {
		responseData["decision"] = *result.Decision
	}
	if result.SystemMessage != nil {
		responseData["systemMessage"] = *result.SystemMessage
	}
	if result.Reason != nil {
		responseData["reason"] = *result.Reason
	}
	if result.HookSpecificOutput != nil {
		responseData["hookSpecificOutput"] = result.HookSpecificOutput
	}

	response := SDKControlResponse{
		Type: MessageTypeControlResponse,
		Response: Response{
			Subtype:   ResponseSubtypeSuccess,
			RequestID: requestID,
			Response:  responseData,
		},
	}

	data, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("failed to marshal hook response: %w", err)
	}

	return p.transport.Write(ctx, append(data, '\n'))
}

// generateHookRegistrations creates hook registrations for initialization.
// This builds the hooks config to send to CLI during initialize.
func (p *Protocol) generateHookRegistrations() []HookRegistration {
	var registrations []HookRegistration

	if p.hooks == nil {
		return registrations
	}

	// Initialize callback map if needed
	p.hookCallbacksMu.Lock()
	if p.hookCallbacks == nil {
		p.hookCallbacks = make(map[string]HookCallback)
	}

	for _, matchers := range p.hooks {
		for _, matcher := range matchers {
			for _, callback := range matcher.Hooks {
				// Generate callback ID matching Python SDK format
				callbackID := fmt.Sprintf("hook_%d", p.nextHookCallback)
				p.nextHookCallback++

				// Store callback for later lookup
				p.hookCallbacks[callbackID] = callback

				registrations = append(registrations, HookRegistration{
					CallbackID: callbackID,
					Matcher:    matcher.Matcher,
					Timeout:    matcher.Timeout,
				})
			}
		}
	}
	p.hookCallbacksMu.Unlock()

	return registrations
}

// buildHooksConfig creates the hooks config for the initialize request.
// Format: {"PreToolUse": [{"matcher": "Bash", "hookCallbackIds": ["hook_0"]}], ...}
// This matches the Python SDK's format exactly for CLI compatibility.
func (p *Protocol) buildHooksConfig() map[string][]HookMatcherConfig {
	if p.hooks == nil {
		return nil
	}

	config := make(map[string][]HookMatcherConfig)

	// Initialize callback map if needed
	p.hookCallbacksMu.Lock()
	if p.hookCallbacks == nil {
		p.hookCallbacks = make(map[string]HookCallback)
	}

	for event, matchers := range p.hooks {
		eventName := string(event)
		var matcherConfigs []HookMatcherConfig

		for _, matcher := range matchers {
			// Generate callback IDs for each callback in this matcher
			var callbackIDs []string
			for _, callback := range matcher.Hooks {
				callbackID := fmt.Sprintf("hook_%d", p.nextHookCallback)
				p.nextHookCallback++

				// Store callback for later lookup
				p.hookCallbacks[callbackID] = callback
				callbackIDs = append(callbackIDs, callbackID)
			}

			matcherConfigs = append(matcherConfigs, HookMatcherConfig{
				Matcher:         matcher.Matcher,
				HookCallbackIDs: callbackIDs,
				Timeout:         matcher.Timeout,
			})
		}

		if len(matcherConfigs) > 0 {
			config[eventName] = matcherConfigs
		}
	}
	p.hookCallbacksMu.Unlock()

	return config
}

// Helper functions for parsing hook input fields

func getString(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func getStringPtr(m map[string]any, key string) *string {
	if v, ok := m[key].(string); ok {
		return &v
	}
	return nil
}

func getBool(m map[string]any, key string) bool {
	if v, ok := m[key].(bool); ok {
		return v
	}
	return false
}

func getBoolPtr(m map[string]any, key string) *bool {
	if v, ok := m[key].(bool); ok {
		return &v
	}
	return nil
}

func getMap(m map[string]any, key string) map[string]any {
	if v, ok := m[key].(map[string]any); ok {
		return v
	}
	return make(map[string]any)
}
