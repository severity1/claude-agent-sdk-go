package shared

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestOptionsDefaults tests Options struct default values using table-driven approach
func TestOptionsDefaults(t *testing.T) {
	options := NewOptions()

	tests := []struct {
		name     string
		field    interface{}
		expected interface{}
	}{
		{"MaxThinkingTokens", options.MaxThinkingTokens, 0},
		{"Thinking_nil", options.Thinking == nil, true},
		{"ContinueConversation", options.ContinueConversation, false},
		{"MaxTurns", options.MaxTurns, 0},
		{"AllowedTools_initialized", options.AllowedTools == nil, false},
		{"AllowedTools_empty", len(options.AllowedTools), 0},
		{"DisallowedTools_initialized", options.DisallowedTools == nil, false},
		{"DisallowedTools_empty", len(options.DisallowedTools), 0},
		{"Betas_initialized", options.Betas == nil, false},
		{"Betas_empty", len(options.Betas), 0},
		{"AddDirs_initialized", options.AddDirs == nil, false},
		{"AddDirs_empty", len(options.AddDirs), 0},
		{"McpServers_initialized", options.McpServers == nil, false},
		{"McpServers_empty", len(options.McpServers), 0},
		{"ExtraArgs_initialized", options.ExtraArgs == nil, false},
		{"ExtraArgs_empty", len(options.ExtraArgs), 0},
		{"ExtraEnv_initialized", options.ExtraEnv == nil, false},
		{"ExtraEnv_empty", len(options.ExtraEnv), 0},
		{"ForkSession", options.ForkSession, false},
		// nil keeps the CLI default sources (Python setting_sources=None).
		{"SettingSources_nil", options.SettingSources == nil, true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertOptionsField(t, test.field, test.expected, test.name)
		})
	}

	// Test nil pointer fields
	nilTests := []struct {
		name  string
		check func() bool
	}{
		{"SystemPrompt", func() bool { return options.SystemPrompt == nil }},
		{"AppendSystemPrompt", func() bool { return options.AppendSystemPrompt == nil }},
		{"Model", func() bool { return options.Model == nil }},
		{"PermissionMode", func() bool { return options.PermissionMode == nil }},
		{"PermissionPromptToolName", func() bool { return options.PermissionPromptToolName == nil }},
		{"Resume", func() bool { return options.Resume == nil }},
		{"Settings", func() bool { return options.Settings == nil }},
		{"Cwd", func() bool { return options.Cwd == nil }},
	}

	for _, test := range nilTests {
		t.Run(test.name+"_nil", func(t *testing.T) {
			if !test.check() {
				t.Errorf("Expected %s to be nil", test.name)
			}
		})
	}
}

// TestOptionsValidation tests critical validation edge cases
func TestOptionsValidation(t *testing.T) {
	tests := []struct {
		name    string
		setup   func() *Options
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid_options",
			setup: func() *Options {
				opts := NewOptions()
				opts.AllowedTools = []string{"Read", "Write"}
				return opts
			},
			wantErr: false,
		},
		{
			name: "negative_thinking_tokens",
			setup: func() *Options {
				opts := NewOptions()
				opts.MaxThinkingTokens = -100
				return opts
			},
			wantErr: true,
			errMsg:  "MaxThinkingTokens must be non-negative, got -100",
		},
		{
			name: "conflicting_tools",
			setup: func() *Options {
				opts := NewOptions()
				opts.AllowedTools = []string{"Read", "Write"}
				opts.DisallowedTools = []string{"Write", "Bash"}
				return opts
			},
			wantErr: true,
			errMsg:  "tool 'Write' cannot be in both AllowedTools and DisallowedTools",
		},
		{
			name: "negative_max_turns",
			setup: func() *Options {
				opts := NewOptions()
				opts.MaxTurns = -5
				return opts
			},
			wantErr: true,
			errMsg:  "MaxTurns must be non-negative, got -5",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			options := test.setup()
			err := options.Validate()
			assertValidationError(t, err, test.wantErr, test.errMsg)
		})
	}
}

// TestMcpServerTypes tests MCP server configuration interface compliance
func TestMcpServerTypes(t *testing.T) {
	tests := []struct {
		name         string
		config       McpServerConfig
		expectedType McpServerType
	}{
		{
			name: "stdio_server",
			config: &McpStdioServerConfig{
				Type:    McpServerTypeStdio,
				Command: "node",
				Args:    []string{"server.js"},
			},
			expectedType: McpServerTypeStdio,
		},
		{
			name: "sse_server",
			config: &McpSSEServerConfig{
				Type: McpServerTypeSSE,
				URL:  "https://example.com/sse",
			},
			expectedType: McpServerTypeSSE,
		},
		{
			name: "http_server",
			config: &McpHTTPServerConfig{
				Type: McpServerTypeHTTP,
				URL:  "https://example.com/http",
			},
			expectedType: McpServerTypeHTTP,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertMcpServerType(t, test.config, test.expectedType)
		})
	}
}

// TestMcpServerConfigAlwaysLoad verifies that AlwaysLoad is wired through all
// four MCP server config structs and round-trips through JSON correctly.
// A false (default) value must be omitted from the marshaled output thanks to
// the omitempty tag, so the CLI sees only servers that have explicitly opted in.
func TestMcpServerConfigAlwaysLoad(t *testing.T) {
	t.Run("defaults_to_false", func(t *testing.T) {
		stdio := &McpStdioServerConfig{Type: McpServerTypeStdio, Command: "node"}
		sse := &McpSSEServerConfig{Type: McpServerTypeSSE, URL: "https://example.com/sse"}
		http := &McpHTTPServerConfig{Type: McpServerTypeHTTP, URL: "https://example.com/http"}
		sdk := &McpSdkServerConfig{Type: McpServerTypeSdk, Name: "test"}
		if stdio.AlwaysLoad || sse.AlwaysLoad || http.AlwaysLoad || sdk.AlwaysLoad {
			t.Error("AlwaysLoad should default to false on all MCP server config types")
		}
	})

	tests := []struct {
		name   string
		config McpServerConfig
	}{
		{
			name: "stdio",
			config: &McpStdioServerConfig{
				Type:       McpServerTypeStdio,
				Command:    "node",
				Args:       []string{"server.js"},
				AlwaysLoad: true,
			},
		},
		{
			name: "sse",
			config: &McpSSEServerConfig{
				Type:       McpServerTypeSSE,
				URL:        "https://example.com/sse",
				AlwaysLoad: true,
			},
		},
		{
			name: "http",
			config: &McpHTTPServerConfig{
				Type:       McpServerTypeHTTP,
				URL:        "https://example.com/http",
				AlwaysLoad: true,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name+"_marshals_alwaysLoad_when_true", func(t *testing.T) {
			data, err := json.Marshal(test.config)
			if err != nil {
				t.Fatalf("Marshal failed: %v", err)
			}
			if !strings.Contains(string(data), `"alwaysLoad":true`) {
				t.Errorf("Expected JSON to contain alwaysLoad:true, got %s", data)
			}
		})
	}

	for _, test := range tests {
		t.Run(test.name+"_omits_alwaysLoad_when_false", func(t *testing.T) {
			// Construct a copy with AlwaysLoad cleared.
			var data []byte
			var err error
			switch c := test.config.(type) {
			case *McpStdioServerConfig:
				dup := *c
				dup.AlwaysLoad = false
				data, err = json.Marshal(&dup)
			case *McpSSEServerConfig:
				dup := *c
				dup.AlwaysLoad = false
				data, err = json.Marshal(&dup)
			case *McpHTTPServerConfig:
				dup := *c
				dup.AlwaysLoad = false
				data, err = json.Marshal(&dup)
			}
			if err != nil {
				t.Fatalf("Marshal failed: %v", err)
			}
			if strings.Contains(string(data), "alwaysLoad") {
				t.Errorf("Expected alwaysLoad to be omitted when false, got %s", data)
			}
		})
	}

	t.Run("stdio_round_trip", func(t *testing.T) {
		original := &McpStdioServerConfig{
			Type:       McpServerTypeStdio,
			Command:    "node",
			AlwaysLoad: true,
		}
		data, err := json.Marshal(original)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}
		var decoded McpStdioServerConfig
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}
		if !decoded.AlwaysLoad {
			t.Error("AlwaysLoad lost in round-trip")
		}
	})
}

// TestPermissionModeConstants tests permission mode constant values
func TestPermissionModeConstants(t *testing.T) {
	tests := []struct {
		mode     PermissionMode
		expected string
	}{
		{PermissionModeDefault, "default"},
		{PermissionModeAcceptEdits, "acceptEdits"},
		{PermissionModePlan, "plan"},
		{PermissionModeBypassPermissions, "bypassPermissions"},
	}

	for _, test := range tests {
		t.Run(string(test.mode), func(t *testing.T) {
			assertOptionsField(t, string(test.mode), test.expected, "PermissionMode")
		})
	}
}

// TestSettingSourceConstants tests setting source constant values
func TestSettingSourceConstants(t *testing.T) {
	tests := []struct {
		source   SettingSource
		expected string
	}{
		{SettingSourceUser, "user"},
		{SettingSourceProject, "project"},
		{SettingSourceLocal, "local"},
	}

	for _, test := range tests {
		t.Run(string(test.source), func(t *testing.T) {
			assertOptionsField(t, string(test.source), test.expected, "SettingSource")
		})
	}
}

// Helper functions

// assertOptionsField verifies field values with proper error reporting
func assertOptionsField(t *testing.T, actual, expected interface{}, fieldName string) {
	t.Helper()
	// Handle nil pointer comparisons properly
	if expected == nil {
		if actual != nil {
			t.Errorf("Expected %s = nil, got %v", fieldName, actual)
		}
		return
	}
	if actual != expected {
		t.Errorf("Expected %s = %v, got %v", fieldName, expected, actual)
	}
}

// assertValidationError verifies validation error behavior
func assertValidationError(t *testing.T, err error, wantErr bool, expectedMsg string) {
	t.Helper()
	if (err != nil) != wantErr {
		t.Errorf("error = %v, wantErr %v", err, wantErr)
		return
	}
	if wantErr && expectedMsg != "" && err.Error() != expectedMsg {
		t.Errorf("error = %v, expected message %q", err, expectedMsg)
	}
}

// assertMcpServerType verifies MCP server configuration types
func assertMcpServerType(t *testing.T, config McpServerConfig, expectedType McpServerType) {
	t.Helper()
	if config.GetType() != expectedType {
		t.Errorf("Expected server type %s, got %s", expectedType, config.GetType())
	}
}

// TestSandboxSettingsDefaults tests that Sandbox is nil by default
func TestSandboxSettingsDefaults(t *testing.T) {
	options := NewOptions()

	if options.Sandbox != nil {
		t.Errorf("Expected Sandbox to be nil by default, got %+v", options.Sandbox)
	}
}

// TestSandboxSettingsTypes tests sandbox type definitions and JSON serialization
func TestSandboxSettingsTypes(t *testing.T) {
	// Test that all sandbox types are properly defined
	sandbox := &SandboxSettings{
		Enabled:                   true,
		AutoAllowBashIfSandboxed:  true,
		ExcludedCommands:          []string{"docker", "git"},
		AllowUnsandboxedCommands:  false,
		EnableWeakerNestedSandbox: false,
		Network: &SandboxNetworkConfig{
			AllowUnixSockets:    []string{"/var/run/docker.sock"},
			AllowAllUnixSockets: false,
			AllowLocalBinding:   true,
		},
		IgnoreViolations: &SandboxIgnoreViolations{
			File:    []string{"/tmp/*"},
			Network: []string{"localhost"},
		},
	}

	// Verify fields are accessible and correct
	if !sandbox.Enabled {
		t.Error("Expected Enabled to be true")
	}
	if !sandbox.AutoAllowBashIfSandboxed {
		t.Error("Expected AutoAllowBashIfSandboxed to be true")
	}
	if len(sandbox.ExcludedCommands) != 2 {
		t.Errorf("Expected 2 ExcludedCommands, got %d", len(sandbox.ExcludedCommands))
	}
	if sandbox.Network == nil {
		t.Error("Expected Network to be set")
	}
	if sandbox.Network != nil && !sandbox.Network.AllowLocalBinding {
		t.Error("Expected Network.AllowLocalBinding to be true")
	}
	if sandbox.IgnoreViolations == nil {
		t.Error("Expected IgnoreViolations to be set")
	}
}
