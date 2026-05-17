// Package control MCP message routing for SDK MCP servers.
// This file handles JSONRPC method dispatch for tools/list, tools/call, etc.
package control

import (
	"context"
	"encoding/json"
	"fmt"
)

// handleMcpMessageRequest routes MCP JSONRPC messages to SDK servers.
// Follows handleCanUseToolRequest pattern with panic recovery.
func (p *Protocol) handleMcpMessageRequest(ctx context.Context, requestID string, request map[string]any) error {
	serverName := getString(request, "server_name")
	if serverName == "" {
		return p.sendErrorResponse(ctx, requestID, "missing server_name")
	}

	message, _ := request["message"].(map[string]any)
	if message == nil {
		return p.sendErrorResponse(ctx, requestID, "missing message")
	}

	// Thread-safe server lookup
	p.mu.Lock()
	server, exists := p.sdkMcpServers[serverName]
	p.mu.Unlock()

	if !exists {
		return p.sendMcpErrorResponse(ctx, requestID, message, -32601,
			fmt.Sprintf("server '%s' not found", serverName))
	}

	// Route JSONRPC method with panic recovery
	var mcpResponse map[string]any
	var routeErr error
	func() {
		defer func() {
			if r := recover(); r != nil {
				routeErr = fmt.Errorf("MCP handler panicked: %v", r)
			}
		}()
		mcpResponse, routeErr = p.routeMcpMethod(ctx, server, message)
	}()

	if routeErr != nil {
		return p.sendMcpErrorResponse(ctx, requestID, message, -32603, routeErr.Error())
	}

	return p.sendMcpResponse(ctx, requestID, mcpResponse)
}

// routeMcpMethod dispatches JSONRPC methods to server handlers.
func (p *Protocol) routeMcpMethod(ctx context.Context, server McpServer, msg map[string]any) (map[string]any, error) {
	method := getString(msg, "method")
	params, _ := msg["params"].(map[string]any)
	msgID := msg["id"]

	switch method {
	case "initialize":
		return map[string]any{
			"jsonrpc": "2.0",
			"id":      msgID,
			"result": map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo": map[string]any{
					"name":    server.Name(),
					"version": server.Version(),
				},
			},
		}, nil

	case "tools/list":
		tools, err := server.ListTools(ctx)
		if err != nil {
			return nil, err
		}
		return buildToolsListResult(tools, msgID)

	case "tools/call":
		if params == nil {
			params = make(map[string]any)
		}
		name := getString(params, "name")
		args, _ := params["arguments"].(map[string]any)
		if args == nil {
			args = make(map[string]any)
		}

		result, err := server.CallTool(ctx, name, args)
		if err != nil {
			return nil, err
		}

		content := make([]map[string]any, len(result.Content))
		for i, c := range result.Content {
			item := map[string]any{"type": c.Type}
			switch c.Type {
			case "text":
				item["text"] = c.Text
			case "image":
				item["data"] = c.Data
				item["mimeType"] = c.MimeType
			}
			content[i] = item
		}

		respData := map[string]any{"content": content}
		if result.IsError {
			respData["isError"] = true
		}
		return map[string]any{
			"jsonrpc": "2.0",
			"id":      msgID,
			"result":  respData,
		}, nil

	case "notifications/initialized":
		// Notification - no response required per JSONRPC spec
		return map[string]any{"jsonrpc": "2.0", "result": map[string]any{}}, nil

	default:
		return nil, fmt.Errorf("method '%s' not found", method)
	}
}

// sendMcpResponse sends an MCP success response.
func (p *Protocol) sendMcpResponse(ctx context.Context, requestID string, mcpResp map[string]any) error {
	response := SDKControlResponse{
		Type: MessageTypeControlResponse,
		Response: Response{
			Subtype:   ResponseSubtypeSuccess,
			RequestID: requestID,
			Response:  map[string]any{"mcp_response": mcpResp},
		},
	}
	data, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("failed to marshal MCP response: %w", err)
	}
	return p.transport.Write(ctx, append(data, '\n'))
}

// buildToolsListResult builds the JSONRPC tools/list response payload from a
// slice of tool definitions. Kept standalone from the routeMcpMethod dispatch
// switch so the switch stays within the gocyclo budget, and so additions to
// the response shape do not perturb method-dispatch logic.
func buildToolsListResult(tools []McpToolDefinition, msgID any) (map[string]any, error) {
	toolsData := make([]map[string]any, len(tools))
	for i, t := range tools {
		entry := map[string]any{
			"name":        t.Name,
			"description": t.Description,
			"inputSchema": t.InputSchema,
		}
		if t.Annotations != nil {
			annMap, err := annotationsToMap(t.Annotations)
			if err != nil {
				return nil, err
			}
			entry["annotations"] = annMap
		}
		toolsData[i] = entry
	}
	return map[string]any{
		"jsonrpc": "2.0",
		"id":      msgID,
		"result":  map[string]any{"tools": toolsData},
	}, nil
}

// annotationsToMap converts a ToolAnnotations value to a map[string]any with
// json `omitempty` honored, so only the fields the caller set appear on the
// wire. Empty fields are omitted from the resulting map.
func annotationsToMap(ann *ToolAnnotations) (map[string]any, error) {
	raw, err := json.Marshal(ann)
	if err != nil {
		return nil, fmt.Errorf("marshal annotations: %w", err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("unmarshal annotations: %w", err)
	}
	return out, nil
}

// sendMcpErrorResponse sends an MCP JSONRPC error response.
func (p *Protocol) sendMcpErrorResponse(ctx context.Context, requestID string, msg map[string]any, code int, message string) error {
	errorResp := map[string]any{
		"jsonrpc": "2.0",
		"id":      msg["id"],
		"error": map[string]any{
			"code":    code,
			"message": message,
		},
	}
	return p.sendMcpResponse(ctx, requestID, errorResp)
}
