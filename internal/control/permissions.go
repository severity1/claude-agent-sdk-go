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

	// Parse suggestions from context
	var permCtx ToolPermissionContext
	if suggestions, ok := request["permission_suggestions"].([]any); ok {
		permCtx.Suggestions = parsePermissionSuggestions(suggestions)
	}

	// Get callback (thread-safe read)
	p.mu.Lock()
	callback := p.canUseToolCallback
	p.mu.Unlock()

	// No callback = deny (secure default)
	if callback == nil {
		return p.sendPermissionResponse(ctx, requestID, NewPermissionResultDeny("no permission callback registered"), nil)
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

	return p.sendPermissionResponse(ctx, requestID, result, input)
}

// sendPermissionResponse sends a permission result back to CLI.
func (p *Protocol) sendPermissionResponse(ctx context.Context, requestID string, result PermissionResult, originalInput map[string]any) error {
	// Build response based on result type
	var responseData map[string]any
	switch r := result.(type) {
	case PermissionResultAllow:
		responseData = map[string]any{"behavior": "allow"}
		// CLI requires updatedInput - use modified input if provided, else original
		if r.UpdatedInput != nil {
			responseData["updatedInput"] = r.UpdatedInput
		} else {
			responseData["updatedInput"] = originalInput
		}
		if len(r.UpdatedPermissions) > 0 {
			responseData["updatedPermissions"] = r.UpdatedPermissions
		}
	case PermissionResultDeny:
		responseData = map[string]any{"behavior": "deny"}
		if r.Message != "" {
			responseData["message"] = r.Message
		}
		if r.Interrupt {
			responseData["interrupt"] = r.Interrupt
		}
	default:
		return fmt.Errorf("unknown permission result type: %T", result)
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
