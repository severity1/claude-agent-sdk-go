package subprocess

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/severity1/claude-agent-sdk-go/internal/cli"
	"github.com/severity1/claude-agent-sdk-go/internal/control"
	"github.com/severity1/claude-agent-sdk-go/internal/shared"
)

// generateMcpConfigFile creates a temporary MCP config file from options.McpServers.
// Returns the file path. The file is stored in t.mcpConfigFile for cleanup.
func (t *Transport) generateMcpConfigFile() (string, error) {
	// Build servers map, stripping Instance field from SDK servers for CLI serialization
	// The CLI doesn't need the Go instance - it routes mcp_message requests to the SDK
	serversForCLI := make(map[string]any)
	for name, config := range t.options.McpServers {
		if sdkConfig, ok := config.(*shared.McpSdkServerConfig); ok {
			// SDK servers: only send type and name to CLI (the Go Instance
			// stays in-process). AlwaysLoad must be propagated explicitly
			// since we're not relying on struct json tags here.
			entry := map[string]any{
				"type": string(sdkConfig.Type),
				"name": sdkConfig.Name,
			}
			if sdkConfig.AlwaysLoad {
				entry["alwaysLoad"] = true
			}
			serversForCLI[name] = entry
		} else {
			// External servers: pass as-is
			serversForCLI[name] = config
		}
	}

	// Create the MCP config structure matching Claude CLI expected format
	mcpConfig := map[string]interface{}{
		"mcpServers": serversForCLI,
	}

	// Marshal to JSON
	configData, err := json.MarshalIndent(mcpConfig, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal MCP config: %w", err)
	}

	// Create temporary file
	tmpFile, err := os.CreateTemp("", "claude_mcp_config_*.json")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}

	// Write config data
	if _, err := tmpFile.Write(configData); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpFile.Name())
		return "", fmt.Errorf("failed to write MCP config: %w", err)
	}

	// Sync to ensure data is written
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpFile.Name())
		return "", fmt.Errorf("failed to sync MCP config file: %w", err)
	}

	// Store for cleanup later
	t.mcpConfigFile = tmpFile

	return tmpFile.Name(), nil
}

// GetValidator returns the stream validator for diagnostic purposes.
// This allows clients to check for validation issues like missing tool results.
func (t *Transport) GetValidator() *shared.StreamValidator {
	return t.validator
}

// SetModel changes the AI model during an active session.
func (t *Transport) SetModel(ctx context.Context, model *string) error {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if !t.connected {
		return fmt.Errorf("transport not connected")
	}
	if t.protocol == nil {
		return fmt.Errorf("internal error: transport connected but control protocol is nil")
	}

	return t.protocol.SetModel(ctx, model)
}

// SetPermissionMode changes the permission mode during an active session.
func (t *Transport) SetPermissionMode(ctx context.Context, mode shared.PermissionMode) error {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if !t.connected {
		return fmt.Errorf("transport not connected")
	}
	if t.protocol == nil {
		return fmt.Errorf("internal error: transport connected but control protocol is nil")
	}

	return t.protocol.SetPermissionMode(ctx, string(mode))
}

// RewindFiles reverts tracked files to their state at a specific user message.
// Requires file checkpointing to have been enabled when creating the client.
func (t *Transport) RewindFiles(ctx context.Context, userMessageID string) error {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if !t.connected {
		return fmt.Errorf("transport not connected")
	}
	if t.protocol == nil {
		return fmt.Errorf("internal error: transport connected but control protocol is nil")
	}

	return t.protocol.RewindFiles(ctx, userMessageID)
}

// GetMcpStatus returns the connection status of all configured MCP servers.
func (t *Transport) GetMcpStatus(ctx context.Context) (*control.McpStatusResponse, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if !t.connected {
		return nil, fmt.Errorf("transport not connected")
	}
	if t.protocol == nil {
		return nil, fmt.Errorf("internal error: transport connected but control protocol is nil")
	}

	return t.protocol.GetMcpStatus(ctx)
}

// buildProtocolOptions constructs control protocol options from transport configuration.
func (t *Transport) buildProtocolOptions() []control.ProtocolOption {
	var opts []control.ProtocolOption
	if t.options == nil {
		return opts
	}

	if t.options.CanUseTool != nil {
		opts = append(opts, control.WithCanUseToolCallback(t.canUseToolAdapter()))
	}
	if hooksOpt := t.hooksProtocolOption(); hooksOpt != nil {
		opts = append(opts, hooksOpt)
	}
	if mcpOpt := t.sdkMcpServersProtocolOption(); mcpOpt != nil {
		opts = append(opts, mcpOpt)
	}
	if len(t.options.Agents) > 0 {
		opts = append(opts, control.WithAgents(agentsToMap(t.options.Agents)))
	}
	return opts
}

// canUseToolAdapter wraps the user-facing CanUseTool callback (which uses
// `any`-typed permCtx/result to avoid import cycles) in a strongly-typed
// shim acceptable to control.WithCanUseToolCallback.
func (t *Transport) canUseToolAdapter() control.CanUseToolCallback {
	optionsCallback := t.options.CanUseTool
	return func(ctx context.Context, toolName string, input map[string]any, permCtx control.ToolPermissionContext) (control.PermissionResult, error) {
		result, err := optionsCallback(ctx, toolName, input, permCtx)
		if err != nil {
			return nil, err
		}
		if pr, ok := result.(control.PermissionResult); ok {
			return pr, nil
		}
		fmt.Fprintf(os.Stderr, "claude-agent-sdk: CanUseTool callback returned unexpected type %T, denying\n", result)
		return control.NewPermissionResultDeny("invalid permission result type"), nil
	}
}

// hooksProtocolOption returns a ProtocolOption wiring up the hook map, or
// nil when no hooks are configured or the type cast fails (with a warning).
func (t *Transport) hooksProtocolOption() control.ProtocolOption {
	if t.options.Hooks == nil {
		return nil
	}
	hooks, ok := t.options.Hooks.(map[control.HookEvent][]control.HookMatcher)
	if !ok {
		fmt.Fprintf(os.Stderr, "claude-agent-sdk: Hooks option has unexpected type %T, hooks will not be registered\n", t.options.Hooks)
		return nil
	}
	return control.WithHooks(hooks)
}

// sdkMcpServersProtocolOption builds the in-process SDK MCP server map and
// returns a ProtocolOption, or nil if no SDK servers are configured.
func (t *Transport) sdkMcpServersProtocolOption() control.ProtocolOption {
	if len(t.options.McpServers) == 0 {
		return nil
	}
	sdkServers := make(map[string]control.McpServer)
	for name, config := range t.options.McpServers {
		if sdkConfig, ok := config.(*shared.McpSdkServerConfig); ok && sdkConfig.Instance != nil {
			sdkServers[name] = sdkConfig.Instance
		}
	}
	if len(sdkServers) == 0 {
		return nil
	}
	return control.WithSdkMcpServers(sdkServers)
}

// agentsToMap converts the typed Options.Agents map into the
// map[string]any shape consumed by the control protocol, stripping empty
// optional fields (Python SDK omitempty parity).
func agentsToMap(agents map[string]shared.AgentDefinition) map[string]any {
	out := make(map[string]any, len(agents))
	for name, agent := range agents {
		entry := map[string]any{
			"description": agent.Description,
			"prompt":      agent.Prompt,
		}
		if len(agent.Tools) > 0 {
			entry["tools"] = agent.Tools
		}
		if agent.Model != "" {
			entry["model"] = string(agent.Model)
		}
		out[name] = entry
	}
	return out
}

// buildEnvironment constructs the environment variables for the subprocess.
func (t *Transport) buildEnvironment() []string {
	env := os.Environ()

	// Set entrypoint to identify SDK to CLI
	env = append(env, "CLAUDE_CODE_ENTRYPOINT="+t.entrypoint)

	// Enable file checkpointing if requested (matches Python SDK)
	if t.options != nil && t.options.EnableFileCheckpointing {
		env = append(env, "CLAUDE_CODE_ENABLE_SDK_FILE_CHECKPOINTING=true")
	}

	// Add user-specified environment variables
	if t.options != nil && t.options.ExtraEnv != nil {
		for key, value := range t.options.ExtraEnv {
			env = append(env, fmt.Sprintf("%s=%s", key, value))
		}
	}

	return env
}

// prepareMcpConfig generates MCP config file if needed and returns modified options.
// Returns the original options unchanged if no MCP servers are configured.
func (t *Transport) prepareMcpConfig() (*shared.Options, error) {
	if t.options == nil || len(t.options.McpServers) == 0 {
		return t.options, nil
	}

	mcpConfigPath, err := t.generateMcpConfigFile()
	if err != nil {
		return nil, fmt.Errorf("failed to generate MCP config file: %w", err)
	}

	// Create shallow copy with mcp-config in ExtraArgs
	optsCopy := *t.options
	if optsCopy.ExtraArgs == nil {
		optsCopy.ExtraArgs = make(map[string]*string)
	} else {
		extraArgsCopy := make(map[string]*string, len(optsCopy.ExtraArgs)+1)
		for k, v := range optsCopy.ExtraArgs {
			extraArgsCopy[k] = v
		}
		optsCopy.ExtraArgs = extraArgsCopy
	}
	optsCopy.ExtraArgs["mcp-config"] = &mcpConfigPath
	return &optsCopy, nil
}

// emitCLIVersionWarning performs a non-blocking CLI version check and emits
// a warning via StderrCallback if the CLI version is outdated.
func (t *Transport) emitCLIVersionWarning(ctx context.Context) {
	if warning := cli.CheckCLIVersion(ctx, t.cliPath); warning != "" {
		if t.options != nil && t.options.StderrCallback != nil {
			t.options.StderrCallback(warning)
		}
	}
}
