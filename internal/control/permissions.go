// Package control permission callback handling.
// This file processes can_use_tool requests and permission responses.
package control

import (
	"context"
	"encoding/json"
	"fmt"
)

// handleCanUseToolRequest processes a permission check request from CLI.
// Follows StderrCallback pattern: synchronous with panic recovery.
func (p *Protocol) handleCanUseToolRequest(ctx context.Context, requestID string, request map[string]any) error {
	// Parse request fields
	toolName, _ := request["tool_name"].(string)
	if toolName == "" {
		return p.sendErrorResponse(ctx, requestID, "missing tool_name")
	}

	input, _ := request["input"].(map[string]any)
	if input == nil {
		input = make(map[string]any)
	}

	// Parse context fields from request. tool_use_id and agent_id are
	// populated by the CLI on every can_use_tool request (Python PR #754);
	// forwarding them lets callbacks distinguish concurrent tool calls and
	// attribute requests to the originating subagent.
	var permCtx ToolPermissionContext
	if suggestions, ok := request["permission_suggestions"].([]any); ok {
		permCtx.Suggestions = parsePermissionSuggestions(suggestions)
	}
	if s, ok := request["tool_use_id"].(string); ok && s != "" {
		permCtx.ToolUseID = &s
	}
	if s, ok := request["agent_id"].(string); ok && s != "" {
		permCtx.AgentID = &s
	}

	// Get callback (thread-safe read)
	p.mu.Lock()
	callback := p.canUseToolCallback
	p.mu.Unlock()

	// No callback = deny (secure default)
	if callback == nil {
		return p.sendPermissionResponse(ctx, requestID, NewPermissionResultDeny("no permission callback registered"))
	}

	// Invoke callback synchronously with panic recovery (matches StderrCallback pattern)
	var result PermissionResult
	var err error
	func() {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("permission callback panicked: %v", r)
			}
		}()
		result, err = callback(ctx, toolName, input, permCtx)
	}()

	if err != nil {
		return p.sendErrorResponse(ctx, requestID, fmt.Sprintf("callback error: %v", err))
	}

	return p.sendPermissionResponse(ctx, requestID, result)
}

// sendPermissionResponse sends a permission result back to CLI.
//
// The PermissionResult is marshaled directly so that
// PermissionResultAllow/Deny.MarshalJSON runs and hard-codes the
// "behavior" discriminator on the wire regardless of struct state.
func (p *Protocol) sendPermissionResponse(ctx context.Context, requestID string, result PermissionResult) error {
	switch result.(type) {
	case PermissionResultAllow, PermissionResultDeny:
		// OK - handled by MarshalJSON.
	default:
		return fmt.Errorf("unknown permission result type: %T", result)
	}

	response := SDKControlResponse{
		Type: MessageTypeControlResponse,
		Response: Response{
			Subtype:   ResponseSubtypeSuccess,
			RequestID: requestID,
			Response:  result,
		},
	}

	data, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("failed to marshal permission response: %w", err)
	}

	return p.transport.Write(ctx, append(data, '\n'))
}

// parsePermissionSuggestions converts raw JSON to PermissionUpdate slice.
// Invalid or unrecognized items are silently skipped for forward compatibility
// with future CLI versions that may introduce new fields or formats.
func parsePermissionSuggestions(raw []any) []PermissionUpdate {
	var suggestions []PermissionUpdate
	for _, item := range raw {
		if m, ok := item.(map[string]any); ok {
			update := PermissionUpdate{}
			if t, ok := m["type"].(string); ok {
				update.Type = PermissionUpdateType(t)
			}
			if rules, ok := m["rules"].([]any); ok {
				for _, rule := range rules {
					if ruleMap, ok := rule.(map[string]any); ok {
						rv := PermissionRuleValue{}
						if tn, ok := ruleMap["toolName"].(string); ok {
							rv.ToolName = tn
						}
						if rc, ok := ruleMap["ruleContent"].(string); ok {
							rv.RuleContent = &rc
						}
						update.Rules = append(update.Rules, rv)
					}
				}
			}
			if b, ok := m["behavior"].(string); ok {
				update.Behavior = &b
			}
			if mode, ok := m["mode"].(string); ok {
				update.Mode = &mode
			}
			if dirs, ok := m["directories"].([]any); ok {
				for _, d := range dirs {
					if ds, ok := d.(string); ok {
						update.Directories = append(update.Directories, ds)
					}
				}
			}
			if dest, ok := m["destination"].(string); ok {
				update.Destination = &dest
			}
			suggestions = append(suggestions, update)
		}
	}
	return suggestions
}
