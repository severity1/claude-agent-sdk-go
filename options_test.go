package claudecode

import (
	"bytes"
	"context"
	"io"
	"os"
	"testing"
)

// Ensure context is used (for mock transport)
var _ = context.Background

// T015: Default Options Creation - Test functional options integration
func TestDefaultOptions(t *testing.T) {
	// Test that NewOptions() creates proper defaults via shared package
	options := NewOptions()

	// Verify that functional options work with shared types
	assertOptionsMaxThinkingTokens(t, options, 8000)

	// Test that we can apply functional options
	optionsWithPrompt := NewOptions(WithSystemPrompt("test prompt"))
	assertOptionsSystemPrompt(t, optionsWithPrompt, "test prompt")
}

// T016: Options with Tools
func TestOptionsWithTools(t *testing.T) {
	// Test Options with allowed_tools and disallowed_tools to match Python SDK
	options := NewOptions(
		WithAllowedTools("Read", "Write", "Edit"),
		WithDisallowedTools("Bash"),
	)

	// Verify allowed tools
	expectedAllowed := []string{"Read", "Write", "Edit"}
	assertOptionsStringSlice(t, options.AllowedTools, expectedAllowed, "AllowedTools")

	// Verify disallowed tools
	expectedDisallowed := []string{"Bash"}
	assertOptionsStringSlice(t, options.DisallowedTools, expectedDisallowed, "DisallowedTools")

	// Test with empty tools
	emptyOptions := NewOptions(
		WithAllowedTools(),
		WithDisallowedTools(),
	)
	assertOptionsStringSlice(t, emptyOptions.AllowedTools, []string{}, "AllowedTools")
	assertOptionsStringSlice(t, emptyOptions.DisallowedTools, []string{}, "DisallowedTools")
}

// TestWithBetasOption tests SDK beta features functional option
func TestWithBetasOption(t *testing.T) {
	tests := []struct {
		name     string
		setup    func() *Options
		expected []SdkBeta
	}{
		{
			name: "single_beta",
			setup: func() *Options {
				return NewOptions(WithBetas(SdkBetaContext1M))
			},
			expected: []SdkBeta{SdkBetaContext1M},
		},
		{
			name: "multiple_betas",
			setup: func() *Options {
				return NewOptions(WithBetas(SdkBetaContext1M, "other-beta"))
			},
			expected: []SdkBeta{SdkBetaContext1M, "other-beta"},
		},
		{
			name: "empty_betas",
			setup: func() *Options {
				return NewOptions(WithBetas())
			},
			expected: []SdkBeta{},
		},
		{
			name: "override_betas",
			setup: func() *Options {
				return NewOptions(
					WithBetas(SdkBetaContext1M),
					WithBetas("new-beta"), // Should replace, not append
				)
			},
			expected: []SdkBeta{"new-beta"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options := tt.setup()
			assertOptionsBetas(t, options.Betas, tt.expected)
		})
	}
}

// T017: Permission Mode Options
func TestPermissionModeOptions(t *testing.T) {
	// Test all permission modes using table-driven approach
	tests := []struct {
		name string
		mode PermissionMode
	}{
		{"default", PermissionModeDefault},
		{"accept_edits", PermissionModeAcceptEdits},
		{"plan", PermissionModePlan},
		{"bypass_permissions", PermissionModeBypassPermissions},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			options := NewOptions(WithPermissionMode(test.mode))
			assertOptionsPermissionMode(t, options, test.mode)
		})
	}
}

// T018: System Prompt Options
func TestSystemPromptOptions(t *testing.T) {
	// Test system_prompt and append_system_prompt
	systemPrompt := "You are a helpful assistant."
	appendPrompt := "Be concise."

	options := NewOptions(
		WithSystemPrompt(systemPrompt),
		WithAppendSystemPrompt(appendPrompt),
	)

	// Verify system prompt is set
	assertOptionsSystemPrompt(t, options, systemPrompt)
	assertOptionsAppendSystemPrompt(t, options, appendPrompt)

	// Test with only system prompt
	systemOnlyOptions := NewOptions(WithSystemPrompt("Only system prompt"))
	assertOptionsSystemPrompt(t, systemOnlyOptions, "Only system prompt")
	assertOptionsAppendSystemPromptNil(t, systemOnlyOptions)

	// Test with only append prompt
	appendOnlyOptions := NewOptions(WithAppendSystemPrompt("Only append prompt"))
	assertOptionsAppendSystemPrompt(t, appendOnlyOptions, "Only append prompt")
	assertOptionsSystemPromptNil(t, appendOnlyOptions)
}

// T019: Session Continuation Options
func TestSessionContinuationOptions(t *testing.T) {
	// Test continue_conversation and resume options
	sessionID := "session-123"

	options := NewOptions(
		WithContinueConversation(true),
		WithResume(sessionID),
	)

	// Verify continue conversation is set
	assertOptionsContinueConversation(t, options, true)
	assertOptionsResume(t, options, sessionID)

	// Test with continue_conversation false
	falseOptions := NewOptions(WithContinueConversation(false))
	assertOptionsContinueConversation(t, falseOptions, false)
	assertOptionsResumeNil(t, falseOptions)

	// Test with only resume
	resumeOnlyOptions := NewOptions(WithResume("another-session"))
	assertOptionsResume(t, resumeOnlyOptions, "another-session")
	assertOptionsContinueConversation(t, resumeOnlyOptions, false) // default
}

// T020: Model Specification Options
func TestModelSpecificationOptions(t *testing.T) {
	// Test model and permission_prompt_tool_name
	model := "claude-3-5-sonnet-20241022"
	toolName := "CustomTool"

	options := NewOptions(
		WithModel(model),
		WithPermissionPromptToolName(toolName),
	)

	// Verify model and tool name are set
	assertOptionsModel(t, options, model)
	assertOptionsPermissionPromptToolName(t, options, toolName)

	// Test with only model
	modelOnlyOptions := NewOptions(WithModel("claude-opus-4"))
	assertOptionsModel(t, modelOnlyOptions, "claude-opus-4")
	assertOptionsPermissionPromptToolNameNil(t, modelOnlyOptions)

	// Test with only permission prompt tool name
	toolOnlyOptions := NewOptions(WithPermissionPromptToolName("OnlyTool"))
	assertOptionsPermissionPromptToolName(t, toolOnlyOptions, "OnlyTool")
	assertOptionsModelNil(t, toolOnlyOptions)
}

// T021: Functional Options Pattern
func TestFunctionalOptionsPattern(t *testing.T) {
	// Test chaining multiple functional options to create a fluent API
	options := NewOptions(
		WithSystemPrompt("You are a helpful assistant"),
		WithAllowedTools("Read", "Write"),
		WithDisallowedTools("Bash"),
		WithPermissionMode(PermissionModeAcceptEdits),
		WithModel("claude-3-5-sonnet-20241022"),
		WithContinueConversation(true),
		WithResume("session-456"),
		WithCwd("/tmp/test"),
		WithAddDirs("/tmp/dir1", "/tmp/dir2"),
		WithMaxThinkingTokens(10000),
		WithPermissionPromptToolName("CustomPermissionTool"),
	)

	// Verify all options are correctly applied
	if options.SystemPrompt == nil || *options.SystemPrompt != "You are a helpful assistant" {
		t.Errorf("Expected SystemPrompt = %q, got %v", "You are a helpful assistant", options.SystemPrompt)
	}

	expectedAllowed := []string{"Read", "Write"}
	if len(options.AllowedTools) != len(expectedAllowed) {
		t.Errorf("Expected AllowedTools length = %d, got %d", len(expectedAllowed), len(options.AllowedTools))
	}

	expectedDisallowed := []string{"Bash"}
	if len(options.DisallowedTools) != len(expectedDisallowed) {
		t.Errorf("Expected DisallowedTools length = %d, got %d", len(expectedDisallowed), len(options.DisallowedTools))
	}

	if options.PermissionMode == nil || *options.PermissionMode != PermissionModeAcceptEdits {
		t.Errorf("Expected PermissionMode = %q, got %v", PermissionModeAcceptEdits, options.PermissionMode)
	}

	if options.Model == nil || *options.Model != "claude-3-5-sonnet-20241022" {
		t.Errorf("Expected Model = %q, got %v", "claude-3-5-sonnet-20241022", options.Model)
	}

	if options.ContinueConversation != true {
		t.Errorf("Expected ContinueConversation = true, got %v", options.ContinueConversation)
	}

	if options.Resume == nil || *options.Resume != "session-456" {
		t.Errorf("Expected Resume = %q, got %v", "session-456", options.Resume)
	}

	if options.Cwd == nil || *options.Cwd != "/tmp/test" {
		t.Errorf("Expected Cwd = %q, got %v", "/tmp/test", options.Cwd)
	}

	expectedAddDirs := []string{"/tmp/dir1", "/tmp/dir2"}
	if len(options.AddDirs) != len(expectedAddDirs) {
		t.Errorf("Expected AddDirs length = %d, got %d", len(expectedAddDirs), len(options.AddDirs))
	}

	if options.MaxThinkingTokens != 10000 {
		t.Errorf("Expected MaxThinkingTokens = 10000, got %d", options.MaxThinkingTokens)
	}

	if options.PermissionPromptToolName == nil || *options.PermissionPromptToolName != "CustomPermissionTool" {
		t.Errorf("Expected PermissionPromptToolName = %q, got %v", "CustomPermissionTool", options.PermissionPromptToolName)
	}
}

// T022: MCP Server Configuration
func TestMcpServerConfiguration(t *testing.T) {
	// Test all three MCP server configuration types: stdio, SSE, HTTP

	// Create MCP server configurations
	stdioConfig := &McpStdioServerConfig{
		Type:    McpServerTypeStdio,
		Command: "python",
		Args:    []string{"-m", "my_mcp_server"},
		Env:     map[string]string{"DEBUG": "1"},
	}

	sseConfig := &McpSSEServerConfig{
		Type:    McpServerTypeSSE,
		URL:     "http://localhost:8080/sse",
		Headers: map[string]string{"Authorization": "Bearer token123"},
	}

	httpConfig := &McpHTTPServerConfig{
		Type:    McpServerTypeHTTP,
		URL:     "http://localhost:8080/mcp",
		Headers: map[string]string{"Content-Type": "application/json"},
	}

	servers := map[string]McpServerConfig{
		"stdio_server": stdioConfig,
		"sse_server":   sseConfig,
		"http_server":  httpConfig,
	}

	options := NewOptions(WithMcpServers(servers))

	// Verify MCP servers are set
	if options.McpServers == nil {
		t.Error("Expected McpServers to be set, got nil")
	}

	if len(options.McpServers) != 3 {
		t.Errorf("Expected 3 MCP servers, got %d", len(options.McpServers))
	}

	// Test stdio server configuration
	stdioServer, exists := options.McpServers["stdio_server"]
	if !exists {
		t.Error("Expected stdio_server to exist")
	}
	if stdioServer.GetType() != McpServerTypeStdio {
		t.Errorf("Expected stdio server type = %q, got %q", McpServerTypeStdio, stdioServer.GetType())
	}

	stdioTyped, ok := stdioServer.(*McpStdioServerConfig)
	if !ok {
		t.Errorf("Expected *McpStdioServerConfig, got %T", stdioServer)
	} else {
		if stdioTyped.Command != "python" {
			t.Errorf("Expected Command = %q, got %q", "python", stdioTyped.Command)
		}
		if len(stdioTyped.Args) != 2 || stdioTyped.Args[0] != "-m" {
			t.Errorf("Expected Args = [-m my_mcp_server], got %v", stdioTyped.Args)
		}
		if stdioTyped.Env["DEBUG"] != "1" {
			t.Errorf("Expected Env[DEBUG] = %q, got %q", "1", stdioTyped.Env["DEBUG"])
		}
	}

	// Test SSE server configuration
	sseServer, exists := options.McpServers["sse_server"]
	if !exists {
		t.Error("Expected sse_server to exist")
	}
	if sseServer.GetType() != McpServerTypeSSE {
		t.Errorf("Expected SSE server type = %q, got %q", McpServerTypeSSE, sseServer.GetType())
	}

	sseTyped, ok := sseServer.(*McpSSEServerConfig)
	if !ok {
		t.Errorf("Expected *McpSSEServerConfig, got %T", sseServer)
	} else {
		if sseTyped.URL != "http://localhost:8080/sse" {
			t.Errorf("Expected URL = %q, got %q", "http://localhost:8080/sse", sseTyped.URL)
		}
		if sseTyped.Headers["Authorization"] != "Bearer token123" {
			t.Errorf("Expected Headers[Authorization] = %q, got %q", "Bearer token123", sseTyped.Headers["Authorization"])
		}
	}

	// Test HTTP server configuration
	httpServer, exists := options.McpServers["http_server"]
	if !exists {
		t.Error("Expected http_server to exist")
	}
	if httpServer.GetType() != McpServerTypeHTTP {
		t.Errorf("Expected HTTP server type = %q, got %q", McpServerTypeHTTP, httpServer.GetType())
	}

	httpTyped, ok := httpServer.(*McpHTTPServerConfig)
	if !ok {
		t.Errorf("Expected *McpHTTPServerConfig, got %T", httpServer)
	} else {
		if httpTyped.URL != "http://localhost:8080/mcp" {
			t.Errorf("Expected URL = %q, got %q", "http://localhost:8080/mcp", httpTyped.URL)
		}
		if httpTyped.Headers["Content-Type"] != "application/json" {
			t.Errorf("Expected Headers[Content-Type] = %q, got %q", "application/json", httpTyped.Headers["Content-Type"])
		}
	}
}

// T023: Extra Args Support
func TestExtraArgsSupport(t *testing.T) {
	// Test arbitrary CLI flag support via ExtraArgs map[string]*string

	// Create extra args - nil values represent boolean flags, non-nil represent flags with values
	debugFlag := "verbose"
	extraArgs := map[string]*string{
		"--debug":   &debugFlag,        // Flag with value: --debug=verbose
		"--verbose": nil,               // Boolean flag: --verbose
		"--output":  stringPtr("json"), // Flag with value: --output=json
		"--quiet":   nil,               // Boolean flag: --quiet
	}

	options := NewOptions(WithExtraArgs(extraArgs))

	// Verify extra args are set
	if options.ExtraArgs == nil {
		t.Error("Expected ExtraArgs to be set, got nil")
	}

	if len(options.ExtraArgs) != 4 {
		t.Errorf("Expected 4 extra args, got %d", len(options.ExtraArgs))
	}

	// Test flag with value
	debugValue, exists := options.ExtraArgs["--debug"]
	if !exists {
		t.Error("Expected --debug flag to exist")
	}
	if debugValue == nil {
		t.Error("Expected --debug to have a value, got nil")
		return
	}
	if *debugValue != "verbose" {
		t.Errorf("Expected --debug = %q, got %q", "verbose", *debugValue)
	}

	// Test boolean flag
	verboseValue, exists := options.ExtraArgs["--verbose"]
	if !exists {
		t.Error("Expected --verbose flag to exist")
	}
	if verboseValue != nil {
		t.Errorf("Expected --verbose to be boolean flag (nil), got %v", verboseValue)
	}

	// Test another flag with value
	outputValue, exists := options.ExtraArgs["--output"]
	if !exists {
		t.Error("Expected --output flag to exist")
	}
	if outputValue == nil {
		t.Error("Expected --output to have a value, got nil")
		return
	}
	if *outputValue != "json" {
		t.Errorf("Expected --output = %q, got %q", "json", *outputValue)
	}

	// Test another boolean flag
	quietValue, exists := options.ExtraArgs["--quiet"]
	if !exists {
		t.Error("Expected --quiet flag to exist")
	}
	if quietValue != nil {
		t.Errorf("Expected --quiet to be boolean flag (nil), got %v", quietValue)
	}

	// Test empty extra args
	emptyOptions := NewOptions(WithExtraArgs(map[string]*string{}))
	if emptyOptions.ExtraArgs == nil {
		t.Error("Expected ExtraArgs to be initialized, got nil")
	}
	if len(emptyOptions.ExtraArgs) != 0 {
		t.Errorf("Expected empty ExtraArgs, got %v", emptyOptions.ExtraArgs)
	}
}

// T024: Options Validation
func TestOptionsValidationIntegration(t *testing.T) {
	// Test that validation works through functional options API (detailed tests in internal/shared)
	validOptions := NewOptions(
		WithAllowedTools("Read", "Write"),
		WithMaxThinkingTokens(8000),
		WithSystemPrompt("Valid prompt"),
	)
	assertOptionsValidationError(t, validOptions, false, "valid options should pass validation")

	// Test that functional options can create invalid options that validation catches
	invalidOptions := NewOptions(WithMaxThinkingTokens(-100))
	assertOptionsValidationError(t, invalidOptions, true, "negative max thinking tokens should fail validation")
}

// T025: NewOptions Constructor
func TestNewOptionsConstructor(t *testing.T) {
	// Test Options creation with functional options applied correctly with defaults

	// Test NewOptions with no arguments should return defaults
	defaultOptions := NewOptions()
	assertOptionsMaxThinkingTokens(t, defaultOptions, 8000)
	assertOptionsStringSlice(t, defaultOptions.AllowedTools, []string{}, "AllowedTools")

	// Test NewOptions with single functional option
	singleOptionOptions := NewOptions(WithSystemPrompt("Single option test"))
	assertOptionsSystemPrompt(t, singleOptionOptions, "Single option test")
	// Should still have defaults for other fields
	assertOptionsMaxThinkingTokens(t, singleOptionOptions, 8000)

	// Test NewOptions with multiple functional options applied in order
	multipleOptions := NewOptions(
		WithMaxThinkingTokens(5000),               // Override default
		WithAllowedTools("Read"),                  // Add tools
		WithSystemPrompt("First prompt"),          // Set system prompt
		WithMaxThinkingTokens(12000),              // Override again (should win)
		WithAllowedTools("Read", "Write", "Edit"), // Override tools (should win)
		WithSystemPrompt("Second prompt"),         // Override again (should win)
		WithDisallowedTools("Bash"),
		WithPermissionMode(PermissionModeAcceptEdits),
		WithContinueConversation(true),
		WithMaxTurns(5),                        // Test WithMaxTurns
		WithSettings("/path/to/settings.json"), // Test WithSettings
	)

	// Verify options are applied in order (later options override earlier ones)
	assertOptionsMaxThinkingTokens(t, multipleOptions, 12000) // final override
	assertOptionsStringSlice(t, multipleOptions.AllowedTools, []string{"Read", "Write", "Edit"}, "AllowedTools")
	assertOptionsSystemPrompt(t, multipleOptions, "Second prompt") // final override
	assertOptionsStringSlice(t, multipleOptions.DisallowedTools, []string{"Bash"}, "DisallowedTools")
	assertOptionsPermissionMode(t, multipleOptions, PermissionModeAcceptEdits)
	assertOptionsContinueConversation(t, multipleOptions, true)
	assertOptionsMaxTurns(t, multipleOptions, 5)
	assertOptionsSettings(t, multipleOptions, "/path/to/settings.json")

	// Test that unmodified fields retain defaults
	assertOptionsResumeNil(t, multipleOptions)
	assertOptionsCwdNil(t, multipleOptions)

	// Test that maps are properly initialized even with options
	if multipleOptions.McpServers == nil {
		t.Error("Expected McpServers to be initialized, got nil")
	} else {
		assertOptionsMapInitialized(t, len(multipleOptions.McpServers), "McpServers")
	}

	if multipleOptions.ExtraArgs == nil {
		t.Error("Expected ExtraArgs to be initialized, got nil")
	} else {
		assertOptionsMapInitialized(t, len(multipleOptions.ExtraArgs), "ExtraArgs")
	}
}

// TestWithCLIPath tests the WithCLIPath option function
func TestWithCLIPath(t *testing.T) {
	tests := []struct {
		name     string
		cliPath  string
		expected *string
	}{
		{
			name:     "valid_cli_path",
			cliPath:  "/usr/local/bin/claude",
			expected: stringPtr("/usr/local/bin/claude"),
		},
		{
			name:     "relative_cli_path",
			cliPath:  "./claude",
			expected: stringPtr("./claude"),
		},
		{
			name:     "empty_cli_path",
			cliPath:  "",
			expected: stringPtr(""),
		},
		{
			name:     "windows_cli_path",
			cliPath:  "C:\\Program Files\\Claude\\claude.exe",
			expected: stringPtr("C:\\Program Files\\Claude\\claude.exe"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			options := NewOptions(WithCLIPath(test.cliPath))

			if options.CLIPath == nil && test.expected != nil {
				t.Errorf("Expected CLIPath to be set to %q, got nil", *test.expected)
			}

			if options.CLIPath != nil && test.expected == nil {
				t.Errorf("Expected CLIPath to be nil, got %q", *options.CLIPath)
			}

			if options.CLIPath != nil && test.expected != nil && *options.CLIPath != *test.expected {
				t.Errorf("Expected CLIPath %q, got %q", *test.expected, *options.CLIPath)
			}
		})
	}

	// Test integration with other options
	t.Run("cli_path_with_other_options", func(t *testing.T) {
		options := NewOptions(
			WithCLIPath("/custom/claude"),
			WithSystemPrompt("Test system prompt"),
			WithModel("claude-sonnet-3-5-20241022"),
		)

		if options.CLIPath == nil || *options.CLIPath != "/custom/claude" {
			t.Errorf("Expected CLIPath to be preserved with other options")
		}

		assertOptionsSystemPrompt(t, options, "Test system prompt")
		assertOptionsModel(t, options, "claude-sonnet-3-5-20241022")
	})
}

// TestWithTransport tests the WithTransport option function
func TestWithTransport(t *testing.T) {
	// Create a mock transport for testing
	mockTransport := &mockTransportForOptions{}

	t.Run("transport_marker_in_extra_args", func(t *testing.T) {
		options := NewOptions(WithTransport(mockTransport))

		if options.ExtraArgs == nil {
			t.Fatal("Expected ExtraArgs to be initialized")
		}

		marker, exists := options.ExtraArgs["__transport_marker__"]
		if !exists {
			t.Error("Expected transport marker to be set in ExtraArgs")
		}

		if marker == nil || *marker != customTransportMarker {
			t.Errorf("Expected transport marker value 'custom_transport', got %v", marker)
		}
	})

	t.Run("transport_with_existing_extra_args", func(t *testing.T) {
		options := NewOptions(
			WithExtraArgs(map[string]*string{"existing": stringPtr("value")}),
			WithTransport(mockTransport),
		)

		if options.ExtraArgs == nil {
			t.Fatal("Expected ExtraArgs to be preserved")
		}

		// Check existing arg is preserved
		existing, exists := options.ExtraArgs["existing"]
		if !exists || existing == nil || *existing != "value" {
			t.Error("Expected existing ExtraArgs to be preserved")
		}

		// Check transport marker is added
		marker, exists := options.ExtraArgs["__transport_marker__"]
		if !exists || marker == nil || *marker != customTransportMarker {
			t.Error("Expected transport marker to be added to existing ExtraArgs")
		}
	})

	t.Run("transport_with_nil_extra_args", func(t *testing.T) {
		// Create options with nil ExtraArgs
		options := &Options{}

		// Apply WithTransport option
		WithTransport(mockTransport)(options)

		if options.ExtraArgs == nil {
			t.Error("Expected ExtraArgs to be initialized")
		}

		marker, exists := options.ExtraArgs["__transport_marker__"]
		if !exists || marker == nil || *marker != customTransportMarker {
			t.Error("Expected transport marker to be set when ExtraArgs was nil")
		}
	})

	t.Run("multiple_transport_calls", func(t *testing.T) {
		anotherMockTransport := &mockTransportForOptions{}

		options := NewOptions(
			WithTransport(mockTransport),
			WithTransport(anotherMockTransport), // Should overwrite
		)

		// Should only have one transport marker (last one wins)
		marker, exists := options.ExtraArgs["__transport_marker__"]
		if !exists || marker == nil || *marker != customTransportMarker {
			t.Error("Expected last transport to set the marker")
		}
	})
}

// Helper Functions - following client_test.go patterns

// assertOptionsMaxThinkingTokens verifies MaxThinkingTokens value
func assertOptionsMaxThinkingTokens(t *testing.T, options *Options, expected int) {
	t.Helper()
	if options.MaxThinkingTokens != expected {
		t.Errorf("Expected MaxThinkingTokens = %d, got %d", expected, options.MaxThinkingTokens)
	}
}

// assertOptionsSystemPrompt verifies SystemPrompt value
func assertOptionsSystemPrompt(t *testing.T, options *Options, expected string) {
	t.Helper()
	if options.SystemPrompt == nil {
		t.Error("Expected SystemPrompt to be set, got nil")
		return
	}
	actual := *options.SystemPrompt
	if actual != expected {
		t.Errorf("Expected SystemPrompt = %q, got %q", expected, actual)
	}
}

// assertOptionsSystemPromptNil verifies SystemPrompt is nil
func assertOptionsSystemPromptNil(t *testing.T, options *Options) {
	t.Helper()
	if options.SystemPrompt != nil {
		t.Errorf("Expected SystemPrompt = nil, got %v", *options.SystemPrompt)
	}
}

// assertOptionsAppendSystemPrompt verifies AppendSystemPrompt value
func assertOptionsAppendSystemPrompt(t *testing.T, options *Options, expected string) {
	t.Helper()
	if options.AppendSystemPrompt == nil {
		t.Error("Expected AppendSystemPrompt to be set, got nil")
		return
	}
	if *options.AppendSystemPrompt != expected {
		t.Errorf("Expected AppendSystemPrompt = %q, got %q", expected, *options.AppendSystemPrompt)
	}
}

// assertOptionsAppendSystemPromptNil verifies AppendSystemPrompt is nil
func assertOptionsAppendSystemPromptNil(t *testing.T, options *Options) {
	t.Helper()
	if options.AppendSystemPrompt != nil {
		t.Errorf("Expected AppendSystemPrompt = nil, got %v", *options.AppendSystemPrompt)
	}
}

// assertOptionsStringSlice verifies string slice values
func assertOptionsStringSlice(t *testing.T, actual, expected []string, fieldName string) {
	t.Helper()
	if len(actual) != len(expected) {
		t.Errorf("Expected %s length = %d, got %d", fieldName, len(expected), len(actual))
		return
	}
	for i, expectedVal := range expected {
		if i >= len(actual) || actual[i] != expectedVal {
			t.Errorf("Expected %s[%d] = %q, got %q", fieldName, i, expectedVal, actual[i])
		}
	}
}

// assertOptionsBetas verifies Betas slice values
func assertOptionsBetas(t *testing.T, actual, expected []SdkBeta) {
	t.Helper()
	if len(actual) != len(expected) {
		t.Errorf("Expected Betas length = %d, got %d", len(expected), len(actual))
		return
	}
	for i, exp := range expected {
		if actual[i] != exp {
			t.Errorf("Expected Betas[%d] = %q, got %q", i, exp, actual[i])
		}
	}
}

// assertOptionsPermissionMode verifies PermissionMode value
func assertOptionsPermissionMode(t *testing.T, options *Options, expected PermissionMode) {
	t.Helper()
	if options.PermissionMode == nil {
		t.Error("Expected PermissionMode to be set, got nil")
		return
	}
	if *options.PermissionMode != expected {
		t.Errorf("Expected PermissionMode = %q, got %q", expected, *options.PermissionMode)
	}
}

// assertOptionsContinueConversation verifies ContinueConversation value
func assertOptionsContinueConversation(t *testing.T, options *Options, expected bool) {
	t.Helper()
	if options.ContinueConversation != expected {
		t.Errorf("Expected ContinueConversation = %v, got %v", expected, options.ContinueConversation)
	}
}

// assertOptionsResume verifies Resume value
func assertOptionsResume(t *testing.T, options *Options, expected string) {
	t.Helper()
	if options.Resume == nil {
		t.Error("Expected Resume to be set, got nil")
		return
	}
	if *options.Resume != expected {
		t.Errorf("Expected Resume = %q, got %q", expected, *options.Resume)
	}
}

// assertOptionsResumeNil verifies Resume is nil
func assertOptionsResumeNil(t *testing.T, options *Options) {
	t.Helper()
	if options.Resume != nil {
		t.Errorf("Expected Resume = nil, got %v", *options.Resume)
	}
}

// assertOptionsModel verifies Model value
func assertOptionsModel(t *testing.T, options *Options, expected string) {
	t.Helper()
	if options.Model == nil {
		t.Error("Expected Model to be set, got nil")
		return
	}
	if *options.Model != expected {
		t.Errorf("Expected Model = %q, got %q", expected, *options.Model)
	}
}

// assertOptionsModelNil verifies Model is nil
func assertOptionsModelNil(t *testing.T, options *Options) {
	t.Helper()
	if options.Model != nil {
		t.Errorf("Expected Model = nil, got %v", *options.Model)
	}
}

// assertOptionsPermissionPromptToolName verifies PermissionPromptToolName value
func assertOptionsPermissionPromptToolName(t *testing.T, options *Options, expected string) {
	t.Helper()
	if options.PermissionPromptToolName == nil {
		t.Error("Expected PermissionPromptToolName to be set, got nil")
		return
	}
	if *options.PermissionPromptToolName != expected {
		t.Errorf("Expected PermissionPromptToolName = %q, got %q", expected, *options.PermissionPromptToolName)
	}
}

// assertOptionsPermissionPromptToolNameNil verifies PermissionPromptToolName is nil
func assertOptionsPermissionPromptToolNameNil(t *testing.T, options *Options) {
	t.Helper()
	if options.PermissionPromptToolName != nil {
		t.Errorf("Expected PermissionPromptToolName = nil, got %v", *options.PermissionPromptToolName)
	}
}

// assertOptionsCwdNil verifies Cwd is nil
func assertOptionsCwdNil(t *testing.T, options *Options) {
	t.Helper()
	if options.Cwd != nil {
		t.Errorf("Expected Cwd = nil, got %v", *options.Cwd)
	}
}

// assertOptionsMaxTurns verifies MaxTurns value
func assertOptionsMaxTurns(t *testing.T, options *Options, expected int) {
	t.Helper()
	if options.MaxTurns != expected {
		t.Errorf("Expected MaxTurns = %d, got %d", expected, options.MaxTurns)
	}
}

// assertOptionsSettings verifies Settings value
func assertOptionsSettings(t *testing.T, options *Options, expected string) {
	t.Helper()
	if options.Settings == nil {
		t.Error("Expected Settings to be set, got nil")
		return
	}
	if *options.Settings != expected {
		t.Errorf("Expected Settings = %q, got %q", expected, *options.Settings)
	}
}

// assertOptionsMapInitialized verifies a map field is initialized but empty
func assertOptionsMapInitialized(t *testing.T, actualLen int, fieldName string) {
	t.Helper()
	if actualLen != 0 {
		t.Errorf("Expected %s = {} (empty but initialized), got length %d", fieldName, actualLen)
	}
}

// assertOptionsValidationError verifies validation returns error
func assertOptionsValidationError(t *testing.T, options *Options, shouldError bool, description string) {
	t.Helper()
	err := options.Validate()
	if shouldError && err == nil {
		t.Errorf("%s: expected validation error, got nil", description)
	}
	if !shouldError && err != nil {
		t.Errorf("%s: expected no validation error, got: %v", description, err)
	}
}

// Helper function for creating string pointers
func stringPtr(s string) *string {
	return &s
}

// mockTransportForOptions is a minimal mock transport for testing options
type mockTransportForOptions struct{}

func (m *mockTransportForOptions) Connect(_ context.Context) error { return nil }
func (m *mockTransportForOptions) SendMessage(_ context.Context, _ StreamMessage) error {
	return nil
}

func (m *mockTransportForOptions) ReceiveMessages(_ context.Context) (<-chan Message, <-chan error) {
	return nil, nil
}
func (m *mockTransportForOptions) Interrupt(_ context.Context) error           { return nil }
func (m *mockTransportForOptions) SetModel(_ context.Context, _ *string) error { return nil }
func (m *mockTransportForOptions) SetPermissionMode(_ context.Context, _ PermissionMode) error {
	return nil
}
func (m *mockTransportForOptions) RewindFiles(_ context.Context, _ string) error { return nil }
func (m *mockTransportForOptions) GetMcpStatus(_ context.Context) (*McpStatusResponse, error) {
	return &McpStatusResponse{}, nil
}
func (m *mockTransportForOptions) Close() error                   { return nil }
func (m *mockTransportForOptions) GetValidator() *StreamValidator { return &StreamValidator{} }

// TestWithEnvOptions tests environment variable functional options following table-driven pattern
func TestWithEnvOptions(t *testing.T) {
	tests := []struct {
		name      string
		setup     func() *Options
		expected  map[string]string
		wantPanic bool
	}{
		{
			name: "single_env_var",
			setup: func() *Options {
				return NewOptions(WithEnvVar("DEBUG", "1"))
			},
			expected: map[string]string{"DEBUG": "1"},
		},
		{
			name: "multiple_env_vars",
			setup: func() *Options {
				return NewOptions(WithEnv(map[string]string{
					"HTTP_PROXY": "http://proxy:8080",
					"CUSTOM_VAR": "value",
				}))
			},
			expected: map[string]string{
				"HTTP_PROXY": "http://proxy:8080",
				"CUSTOM_VAR": "value",
			},
		},
		{
			name: "merge_with_env_and_envvar",
			setup: func() *Options {
				return NewOptions(
					WithEnv(map[string]string{"VAR1": "val1"}),
					WithEnvVar("VAR2", "val2"),
				)
			},
			expected: map[string]string{
				"VAR1": "val1",
				"VAR2": "val2",
			},
		},
		{
			name: "override_existing",
			setup: func() *Options {
				return NewOptions(
					WithEnvVar("KEY", "original"),
					WithEnvVar("KEY", "updated"),
				)
			},
			expected: map[string]string{"KEY": "updated"},
		},
		{
			name: "empty_env_map",
			setup: func() *Options {
				return NewOptions(WithEnv(map[string]string{}))
			},
			expected: map[string]string{},
		},
		{
			name: "nil_env_map_initializes",
			setup: func() *Options {
				opts := &Options{} // ExtraEnv is nil
				WithEnvVar("TEST", "value")(opts)
				return opts
			},
			expected: map[string]string{"TEST": "value"},
		},
		{
			name: "proxy_configuration_example",
			setup: func() *Options {
				return NewOptions(
					WithEnv(map[string]string{
						"HTTP_PROXY":  "http://proxy.example.com:8080",
						"HTTPS_PROXY": "http://proxy.example.com:8080",
						"NO_PROXY":    "localhost,127.0.0.1",
					}),
				)
			},
			expected: map[string]string{
				"HTTP_PROXY":  "http://proxy.example.com:8080",
				"HTTPS_PROXY": "http://proxy.example.com:8080",
				"NO_PROXY":    "localhost,127.0.0.1",
			},
		},
		{
			name: "path_override_example",
			setup: func() *Options {
				return NewOptions(
					WithEnvVar("PATH", "/custom/bin:/usr/bin"),
				)
			},
			expected: map[string]string{
				"PATH": "/custom/bin:/usr/bin",
			},
		},
		{
			name: "nil_env_map_to_WithEnv",
			setup: func() *Options {
				opts := &Options{} // ExtraEnv is nil
				WithEnv(map[string]string{"TEST": "value"})(opts)
				return opts
			},
			expected: map[string]string{"TEST": "value"},
		},
		{
			name: "nil_map_passed_to_WithEnv",
			setup: func() *Options {
				return NewOptions(WithEnv(nil))
			},
			expected: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options := tt.setup()
			assertEnvVars(t, options.ExtraEnv, tt.expected)
		})
	}
}

// TestWithEnvIntegration tests environment variable options integration with other options
func TestWithEnvIntegration(t *testing.T) {
	options := NewOptions(
		WithSystemPrompt("You are a helpful assistant"),
		WithEnvVar("DEBUG", "1"),
		WithModel("claude-3-5-sonnet-20241022"),
		WithEnv(map[string]string{
			"HTTP_PROXY": "http://proxy:8080",
			"CUSTOM":     "value",
		}),
		WithEnvVar("OVERRIDE", "final"),
	)

	// Test that env vars are correctly set
	expected := map[string]string{
		"DEBUG":      "1",
		"HTTP_PROXY": "http://proxy:8080",
		"CUSTOM":     "value",
		"OVERRIDE":   "final",
	}
	assertEnvVars(t, options.ExtraEnv, expected)

	// Test that other options are preserved
	assertOptionsSystemPrompt(t, options, "You are a helpful assistant")
	assertOptionsModel(t, options, "claude-3-5-sonnet-20241022")
}

// Helper function following client_test.go patterns
func assertEnvVars(t *testing.T, actual, expected map[string]string) {
	t.Helper()
	if len(actual) != len(expected) {
		t.Errorf("Expected %d env vars, got %d. Expected: %v, Actual: %v",
			len(expected), len(actual), expected, actual)
		return
	}
	for k, v := range expected {
		if actual[k] != v {
			t.Errorf("Expected %s=%s, got %s=%s", k, v, k, actual[k])
		}
	}
}

// T026: MaxBudgetUSD Option
func TestMaxBudgetUSDOption(t *testing.T) {
	tests := []struct {
		name     string
		budget   float64
		expected float64
	}{
		{"positive_budget", 10.50, 10.50},
		{"zero_budget", 0.0, 0.0},
		{"large_budget", 1000.00, 1000.00},
		{"decimal_precision", 99.99, 99.99},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options := NewOptions(WithMaxBudgetUSD(tt.budget))
			assertOptionsMaxBudgetUSD(t, options, tt.expected)
		})
	}

	// Test nil case
	t.Run("nil_by_default", func(t *testing.T) {
		options := NewOptions()
		assertOptionsMaxBudgetUSDNil(t, options)
	})
}

// T027: FallbackModel Option
func TestFallbackModelOption(t *testing.T) {
	tests := []struct {
		name     string
		model    string
		expected string
	}{
		{"sonnet_model", "claude-3-5-sonnet-20241022", "claude-3-5-sonnet-20241022"},
		{"opus_model", "claude-opus-4", "claude-opus-4"},
		{"empty_model", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options := NewOptions(WithFallbackModel(tt.model))
			assertOptionsFallbackModel(t, options, tt.expected)
		})
	}

	// Test nil case
	t.Run("nil_by_default", func(t *testing.T) {
		options := NewOptions()
		assertOptionsFallbackModelNil(t, options)
	})
}

// T028: User Option
func TestUserOption(t *testing.T) {
	tests := []struct {
		name     string
		user     string
		expected string
	}{
		{"standard_user", "user-123", "user-123"},
		{"email_user", "user@example.com", "user@example.com"},
		{"empty_user", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options := NewOptions(WithUser(tt.user))
			assertOptionsUser(t, options, tt.expected)
		})
	}

	// Test nil case
	t.Run("nil_by_default", func(t *testing.T) {
		options := NewOptions()
		assertOptionsUserNil(t, options)
	})
}

// T029: MaxBufferSize Option
func TestMaxBufferSizeOption(t *testing.T) {
	tests := []struct {
		name     string
		size     int
		expected int
	}{
		{"default_1mb", 1048576, 1048576},
		{"custom_2mb", 2097152, 2097152},
		{"zero_size", 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options := NewOptions(WithMaxBufferSize(tt.size))
			assertOptionsMaxBufferSize(t, options, tt.expected)
		})
	}

	// Test nil case
	t.Run("nil_by_default", func(t *testing.T) {
		options := NewOptions()
		assertOptionsMaxBufferSizeNil(t, options)
	})
}

// T030: New Options Integration Test
func TestNewConfigOptionsIntegration(t *testing.T) {
	// Test all new options together with existing options
	options := NewOptions(
		WithSystemPrompt("You are a helpful assistant"),
		WithModel("claude-3-5-sonnet-20241022"),
		WithMaxBudgetUSD(50.00),
		WithFallbackModel("claude-opus-4"),
		WithUser("user-test-123"),
		WithMaxBufferSize(2097152),
	)

	// Verify new options
	assertOptionsMaxBudgetUSD(t, options, 50.00)
	assertOptionsFallbackModel(t, options, "claude-opus-4")
	assertOptionsUser(t, options, "user-test-123")
	assertOptionsMaxBufferSize(t, options, 2097152)

	// Verify existing options still work
	assertOptionsSystemPrompt(t, options, "You are a helpful assistant")
	assertOptionsModel(t, options, "claude-3-5-sonnet-20241022")
}

// Helper functions for new options

// assertOptionsMaxBudgetUSD verifies MaxBudgetUSD value
func assertOptionsMaxBudgetUSD(t *testing.T, options *Options, expected float64) {
	t.Helper()
	if options.MaxBudgetUSD == nil {
		t.Error("Expected MaxBudgetUSD to be set, got nil")
		return
	}
	if *options.MaxBudgetUSD != expected {
		t.Errorf("Expected MaxBudgetUSD = %f, got %f", expected, *options.MaxBudgetUSD)
	}
}

// assertOptionsMaxBudgetUSDNil verifies MaxBudgetUSD is nil
func assertOptionsMaxBudgetUSDNil(t *testing.T, options *Options) {
	t.Helper()
	if options.MaxBudgetUSD != nil {
		t.Errorf("Expected MaxBudgetUSD = nil, got %f", *options.MaxBudgetUSD)
	}
}

// assertOptionsFallbackModel verifies FallbackModel value
func assertOptionsFallbackModel(t *testing.T, options *Options, expected string) {
	t.Helper()
	if options.FallbackModel == nil {
		t.Error("Expected FallbackModel to be set, got nil")
		return
	}
	if *options.FallbackModel != expected {
		t.Errorf("Expected FallbackModel = %q, got %q", expected, *options.FallbackModel)
	}
}

// assertOptionsFallbackModelNil verifies FallbackModel is nil
func assertOptionsFallbackModelNil(t *testing.T, options *Options) {
	t.Helper()
	if options.FallbackModel != nil {
		t.Errorf("Expected FallbackModel = nil, got %q", *options.FallbackModel)
	}
}

// assertOptionsUser verifies User value
func assertOptionsUser(t *testing.T, options *Options, expected string) {
	t.Helper()
	if options.User == nil {
		t.Error("Expected User to be set, got nil")
		return
	}
	if *options.User != expected {
		t.Errorf("Expected User = %q, got %q", expected, *options.User)
	}
}

// assertOptionsUserNil verifies User is nil
func assertOptionsUserNil(t *testing.T, options *Options) {
	t.Helper()
	if options.User != nil {
		t.Errorf("Expected User = nil, got %q", *options.User)
	}
}

// assertOptionsMaxBufferSize verifies MaxBufferSize value
func assertOptionsMaxBufferSize(t *testing.T, options *Options, expected int) {
	t.Helper()
	if options.MaxBufferSize == nil {
		t.Error("Expected MaxBufferSize to be set, got nil")
		return
	}
	if *options.MaxBufferSize != expected {
		t.Errorf("Expected MaxBufferSize = %d, got %d", expected, *options.MaxBufferSize)
	}
}

// assertOptionsMaxBufferSizeNil verifies MaxBufferSize is nil
func assertOptionsMaxBufferSizeNil(t *testing.T, options *Options) {
	t.Helper()
	if options.MaxBufferSize != nil {
		t.Errorf("Expected MaxBufferSize = nil, got %d", *options.MaxBufferSize)
	}
}

// T031: Tools Preset Option
func TestWithToolsPreset(t *testing.T) {
	tests := []struct {
		name           string
		preset         string
		expectedType   string
		expectedPreset string
	}{
		{
			name:           "claude_code_preset",
			preset:         "claude_code",
			expectedType:   "preset",
			expectedPreset: "claude_code",
		},
		{
			name:           "custom_preset",
			preset:         "custom_preset",
			expectedType:   "preset",
			expectedPreset: "custom_preset",
		},
		{
			name:           "empty_preset",
			preset:         "",
			expectedType:   "preset",
			expectedPreset: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options := NewOptions(WithToolsPreset(tt.preset))
			assertOptionsToolsPreset(t, options, tt.expectedType, tt.expectedPreset)
		})
	}
}

// T032: WithClaudeCodeTools Convenience Function
func TestWithClaudeCodeTools(t *testing.T) {
	options := NewOptions(WithClaudeCodeTools())
	assertOptionsToolsPreset(t, options, "preset", "claude_code")
}

// T033: WithTools List Option
func TestWithToolsList(t *testing.T) {
	tests := []struct {
		name     string
		tools    []string
		expected []string
	}{
		{
			name:     "multiple_tools",
			tools:    []string{"Read", "Write", "Edit"},
			expected: []string{"Read", "Write", "Edit"},
		},
		{
			name:     "single_tool",
			tools:    []string{"Read"},
			expected: []string{"Read"},
		},
		{
			name:     "empty_tools",
			tools:    []string{},
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options := NewOptions(WithTools(tt.tools...))
			assertOptionsToolsList(t, options, tt.expected)
		})
	}
}

// T034: Tools Option Override Behavior
func TestToolsOptionOverride(t *testing.T) {
	// Test that later Tools options override earlier ones
	t.Run("preset_overrides_list", func(t *testing.T) {
		options := NewOptions(
			WithTools("Read", "Write"),
			WithToolsPreset("claude_code"),
		)
		assertOptionsToolsPreset(t, options, "preset", "claude_code")
	})

	t.Run("list_overrides_preset", func(t *testing.T) {
		options := NewOptions(
			WithToolsPreset("claude_code"),
			WithTools("Read", "Write"),
		)
		assertOptionsToolsList(t, options, []string{"Read", "Write"})
	})
}

// T035: Tools Option Nil by Default
func TestToolsOptionNilByDefault(t *testing.T) {
	options := NewOptions()
	assertOptionsToolsNil(t, options)
}

// Helper functions for Tools options

// assertOptionsToolsPreset verifies Tools contains a ToolsPreset
func assertOptionsToolsPreset(t *testing.T, options *Options, expectedType, expectedPreset string) {
	t.Helper()
	if options.Tools == nil {
		t.Error("Expected Tools to be set, got nil")
		return
	}
	preset, ok := options.Tools.(ToolsPreset)
	if !ok {
		t.Errorf("Expected Tools to be ToolsPreset, got %T", options.Tools)
		return
	}
	if preset.Type != expectedType {
		t.Errorf("Expected ToolsPreset.Type = %q, got %q", expectedType, preset.Type)
	}
	if preset.Preset != expectedPreset {
		t.Errorf("Expected ToolsPreset.Preset = %q, got %q", expectedPreset, preset.Preset)
	}
}

// assertOptionsToolsList verifies Tools contains a []string
func assertOptionsToolsList(t *testing.T, options *Options, expected []string) {
	t.Helper()
	if options.Tools == nil {
		if len(expected) == 0 {
			// Empty list case - Tools can be nil or empty slice
			return
		}
		t.Error("Expected Tools to be set, got nil")
		return
	}
	tools, ok := options.Tools.([]string)
	if !ok {
		t.Errorf("Expected Tools to be []string, got %T", options.Tools)
		return
	}
	if len(tools) != len(expected) {
		t.Errorf("Expected Tools length = %d, got %d", len(expected), len(tools))
		return
	}
	for i, exp := range expected {
		if tools[i] != exp {
			t.Errorf("Expected Tools[%d] = %q, got %q", i, exp, tools[i])
		}
	}
}

// assertOptionsToolsNil verifies Tools is nil
func assertOptionsToolsNil(t *testing.T, options *Options) {
	t.Helper()
	if options.Tools != nil {
		t.Errorf("Expected Tools = nil, got %v", options.Tools)
	}
}

// TestWithPluginsOption tests plugin configuration functional options
func TestWithPluginsOption(t *testing.T) {
	tests := []struct {
		name     string
		setup    func() *Options
		expected []SdkPluginConfig
	}{
		{
			name: "single_local_plugin",
			setup: func() *Options {
				return NewOptions(WithPlugins([]SdkPluginConfig{
					{Type: SdkPluginTypeLocal, Path: "/path/to/plugin"},
				}))
			},
			expected: []SdkPluginConfig{
				{Type: SdkPluginTypeLocal, Path: "/path/to/plugin"},
			},
		},
		{
			name: "multiple_plugins",
			setup: func() *Options {
				return NewOptions(WithPlugins([]SdkPluginConfig{
					{Type: SdkPluginTypeLocal, Path: "/path/to/plugin1"},
					{Type: SdkPluginTypeLocal, Path: "/path/to/plugin2"},
				}))
			},
			expected: []SdkPluginConfig{
				{Type: SdkPluginTypeLocal, Path: "/path/to/plugin1"},
				{Type: SdkPluginTypeLocal, Path: "/path/to/plugin2"},
			},
		},
		{
			name: "empty_plugins",
			setup: func() *Options {
				return NewOptions(WithPlugins([]SdkPluginConfig{}))
			},
			expected: []SdkPluginConfig{},
		},
		{
			name: "nil_plugins_replaces_with_nil",
			setup: func() *Options {
				return NewOptions(WithPlugins(nil))
			},
			expected: nil,
		},
		{
			name: "override_plugins",
			setup: func() *Options {
				return NewOptions(
					WithPlugins([]SdkPluginConfig{
						{Type: SdkPluginTypeLocal, Path: "/first"},
					}),
					WithPlugins([]SdkPluginConfig{
						{Type: SdkPluginTypeLocal, Path: "/second"},
					}),
				)
			},
			expected: []SdkPluginConfig{
				{Type: SdkPluginTypeLocal, Path: "/second"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options := tt.setup()
			assertOptionsPlugins(t, options.Plugins, tt.expected)
		})
	}
}

// TestWithPluginOption tests single plugin append functional option
func TestWithPluginOption(t *testing.T) {
	tests := []struct {
		name     string
		setup    func() *Options
		expected []SdkPluginConfig
	}{
		{
			name: "append_single_plugin",
			setup: func() *Options {
				return NewOptions(WithPlugin(SdkPluginConfig{
					Type: SdkPluginTypeLocal,
					Path: "/path/to/plugin",
				}))
			},
			expected: []SdkPluginConfig{
				{Type: SdkPluginTypeLocal, Path: "/path/to/plugin"},
			},
		},
		{
			name: "append_multiple_plugins",
			setup: func() *Options {
				return NewOptions(
					WithPlugin(SdkPluginConfig{Type: SdkPluginTypeLocal, Path: "/plugin1"}),
					WithPlugin(SdkPluginConfig{Type: SdkPluginTypeLocal, Path: "/plugin2"}),
					WithPlugin(SdkPluginConfig{Type: SdkPluginTypeLocal, Path: "/plugin3"}),
				)
			},
			expected: []SdkPluginConfig{
				{Type: SdkPluginTypeLocal, Path: "/plugin1"},
				{Type: SdkPluginTypeLocal, Path: "/plugin2"},
				{Type: SdkPluginTypeLocal, Path: "/plugin3"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options := tt.setup()
			assertOptionsPlugins(t, options.Plugins, tt.expected)
		})
	}
}

// TestWithLocalPluginOption tests local plugin convenience function
func TestWithLocalPluginOption(t *testing.T) {
	tests := []struct {
		name     string
		setup    func() *Options
		expected []SdkPluginConfig
	}{
		{
			name: "single_local_plugin",
			setup: func() *Options {
				return NewOptions(WithLocalPlugin("/path/to/plugin"))
			},
			expected: []SdkPluginConfig{
				{Type: SdkPluginTypeLocal, Path: "/path/to/plugin"},
			},
		},
		{
			name: "multiple_local_plugins",
			setup: func() *Options {
				return NewOptions(
					WithLocalPlugin("/plugin1"),
					WithLocalPlugin("/plugin2"),
				)
			},
			expected: []SdkPluginConfig{
				{Type: SdkPluginTypeLocal, Path: "/plugin1"},
				{Type: SdkPluginTypeLocal, Path: "/plugin2"},
			},
		},
		{
			name: "empty_path",
			setup: func() *Options {
				return NewOptions(WithLocalPlugin(""))
			},
			expected: []SdkPluginConfig{
				{Type: SdkPluginTypeLocal, Path: ""},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options := tt.setup()
			assertOptionsPlugins(t, options.Plugins, tt.expected)
		})
	}
}

// TestPluginsDefaultEmpty tests that Plugins is initialized as empty slice
func TestPluginsDefaultEmpty(t *testing.T) {
	options := NewOptions()
	if options.Plugins == nil {
		t.Error("Expected Plugins to be initialized, got nil")
	}
	if len(options.Plugins) != 0 {
		t.Errorf("Expected empty Plugins, got %v", options.Plugins)
	}
}

// TestPluginTypeConstant tests SdkPluginTypeLocal constant value
func TestPluginTypeConstant(t *testing.T) {
	if SdkPluginTypeLocal != "local" {
		t.Errorf("Expected SdkPluginTypeLocal = %q, got %q", "local", SdkPluginTypeLocal)
	}
}

// TestPluginsMixedWithOtherOptions tests plugins work with other options
func TestPluginsMixedWithOtherOptions(t *testing.T) {
	options := NewOptions(
		WithSystemPrompt("Test prompt"),
		WithLocalPlugin("/path/to/plugin1"),
		WithModel("claude-3-sonnet"),
		WithLocalPlugin("/path/to/plugin2"),
		WithBetas(SdkBetaContext1M),
	)

	// Verify plugins
	expectedPlugins := []SdkPluginConfig{
		{Type: SdkPluginTypeLocal, Path: "/path/to/plugin1"},
		{Type: SdkPluginTypeLocal, Path: "/path/to/plugin2"},
	}
	assertOptionsPlugins(t, options.Plugins, expectedPlugins)

	// Verify other options are preserved
	assertOptionsSystemPrompt(t, options, "Test prompt")
	assertOptionsModel(t, options, "claude-3-sonnet")
	assertOptionsBetas(t, options.Betas, []SdkBeta{SdkBetaContext1M})
}

// assertOptionsPlugins verifies Plugins slice values
func assertOptionsPlugins(t *testing.T, actual, expected []SdkPluginConfig) {
	t.Helper()
	if len(actual) != len(expected) {
		t.Errorf("Expected Plugins length = %d, got %d. Actual: %v", len(expected), len(actual), actual)
		return
	}
	for i, exp := range expected {
		if actual[i].Type != exp.Type {
			t.Errorf("Expected Plugins[%d].Type = %q, got %q", i, exp.Type, actual[i].Type)
		}
		if actual[i].Path != exp.Path {
			t.Errorf("Expected Plugins[%d].Path = %q, got %q", i, exp.Path, actual[i].Path)
		}
	}
}

// TestSessionManagementOptions tests fork_session and setting_sources options
func TestSessionManagementOptions(t *testing.T) {
	t.Run("fork_session", func(t *testing.T) {
		tests := []struct {
			name     string
			value    bool
			expected bool
		}{
			{"enabled", true, true},
			{"disabled", false, false},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				options := NewOptions(WithForkSession(tt.value))
				assertOptionsForkSession(t, options, tt.expected)
			})
		}
	})

	t.Run("setting_sources", func(t *testing.T) {
		tests := []struct {
			name     string
			sources  []SettingSource
			expected []SettingSource
		}{
			{"single_source", []SettingSource{SettingSourceUser}, []SettingSource{SettingSourceUser}},
			{"all_sources", []SettingSource{SettingSourceUser, SettingSourceProject, SettingSourceLocal},
				[]SettingSource{SettingSourceUser, SettingSourceProject, SettingSourceLocal}},
			{"empty_sources", []SettingSource{}, []SettingSource{}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				options := NewOptions(WithSettingSources(tt.sources...))
				assertOptionsSettingSources(t, options, tt.expected)
			})
		}
	})

	t.Run("override_behavior", func(t *testing.T) {
		options := NewOptions(
			WithSettingSources(SettingSourceUser),
			WithSettingSources(SettingSourceProject, SettingSourceLocal),
		)
		// Later call should replace, not append
		assertOptionsSettingSources(t, options, []SettingSource{SettingSourceProject, SettingSourceLocal})
	})

	t.Run("nil_by_default", func(t *testing.T) {
		options := NewOptions()
		assertOptionsForkSession(t, options, false)
		if options.SettingSources == nil {
			t.Error("Expected SettingSources to be initialized, got nil")
		}
		if len(options.SettingSources) != 0 {
			t.Errorf("Expected empty SettingSources, got %v", options.SettingSources)
		}
	})

	t.Run("integration_with_other_options", func(t *testing.T) {
		options := NewOptions(
			WithResume("session-123"),
			WithForkSession(true),
			WithSettingSources(SettingSourceUser, SettingSourceProject),
		)
		assertOptionsResume(t, options, "session-123")
		assertOptionsForkSession(t, options, true)
		assertOptionsSettingSources(t, options, []SettingSource{SettingSourceUser, SettingSourceProject})
	})
}

// assertOptionsForkSession verifies ForkSession value
func assertOptionsForkSession(t *testing.T, options *Options, expected bool) {
	t.Helper()
	if options.ForkSession != expected {
		t.Errorf("Expected ForkSession = %v, got %v", expected, options.ForkSession)
	}
}

// assertOptionsSettingSources verifies SettingSources slice values
func assertOptionsSettingSources(t *testing.T, options *Options, expected []SettingSource) {
	t.Helper()
	if len(options.SettingSources) != len(expected) {
		t.Errorf("Expected SettingSources length = %d, got %d", len(expected), len(options.SettingSources))
		return
	}
	for i, exp := range expected {
		if options.SettingSources[i] != exp {
			t.Errorf("Expected SettingSources[%d] = %q, got %q", i, exp, options.SettingSources[i])
		}
	}
}

// T036: Debug Writer Options - Issue #12
func TestWithDebugWriter(t *testing.T) {
	tests := []struct {
		name     string
		setup    func() *Options
		validate func(t *testing.T, options *Options)
	}{
		{
			name: "custom_writer",
			setup: func() *Options {
				var buf bytes.Buffer
				return NewOptions(WithDebugWriter(&buf))
			},
			validate: func(t *testing.T, options *Options) {
				t.Helper()
				if options.DebugWriter == nil {
					t.Error("Expected DebugWriter to be set, got nil")
				}
			},
		},
		{
			name: "debug_stderr",
			setup: func() *Options {
				return NewOptions(WithDebugStderr())
			},
			validate: func(t *testing.T, options *Options) {
				t.Helper()
				if options.DebugWriter != os.Stderr {
					t.Errorf("Expected DebugWriter to be os.Stderr, got %v", options.DebugWriter)
				}
			},
		},
		{
			name: "debug_disabled",
			setup: func() *Options {
				return NewOptions(WithDebugDisabled())
			},
			validate: func(t *testing.T, options *Options) {
				t.Helper()
				if options.DebugWriter != io.Discard {
					t.Errorf("Expected DebugWriter to be io.Discard, got %v", options.DebugWriter)
				}
			},
		},
		{
			name: "nil_by_default",
			setup: func() *Options {
				return NewOptions()
			},
			validate: func(t *testing.T, options *Options) {
				t.Helper()
				if options.DebugWriter != nil {
					t.Errorf("Expected DebugWriter to be nil by default, got %v", options.DebugWriter)
				}
			},
		},
		{
			name: "override_behavior",
			setup: func() *Options {
				var buf1, buf2 bytes.Buffer
				return NewOptions(
					WithDebugWriter(&buf1),
					WithDebugWriter(&buf2), // Should override
				)
			},
			validate: func(t *testing.T, options *Options) {
				t.Helper()
				if options.DebugWriter == nil {
					t.Error("Expected DebugWriter to be set after override")
				}
			},
		},
		{
			name: "nil_writer_explicit",
			setup: func() *Options {
				return NewOptions(WithDebugWriter(nil))
			},
			validate: func(t *testing.T, options *Options) {
				t.Helper()
				if options.DebugWriter != nil {
					t.Errorf("Expected DebugWriter to be nil when explicitly set, got %v", options.DebugWriter)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options := tt.setup()
			tt.validate(t, options)
		})
	}
}

// TestWithDebugWriterIntegration tests debug writer with other options
func TestWithDebugWriterIntegration(t *testing.T) {
	var debugBuf bytes.Buffer

	options := NewOptions(
		WithSystemPrompt("You are a helpful assistant"),
		WithDebugWriter(&debugBuf),
		WithModel("claude-3-5-sonnet-20241022"),
		WithPermissionMode(PermissionModeAcceptEdits),
	)

	// Verify debug writer is set
	if options.DebugWriter == nil {
		t.Error("Expected DebugWriter to be set")
	}

	// Verify other options are preserved
	assertOptionsSystemPrompt(t, options, "You are a helpful assistant")
	assertOptionsModel(t, options, "claude-3-5-sonnet-20241022")
	assertOptionsPermissionMode(t, options, PermissionModeAcceptEdits)
}

// TestDebugWriterConvenienceFunctions tests the convenience functions
func TestDebugWriterConvenienceFunctions(t *testing.T) {
	t.Run("WithDebugStderr_returns_os_stderr", func(t *testing.T) {
		options := NewOptions(WithDebugStderr())
		if options.DebugWriter != os.Stderr {
			t.Errorf("Expected os.Stderr, got %T", options.DebugWriter)
		}
	})

	t.Run("WithDebugDisabled_returns_io_discard", func(t *testing.T) {
		options := NewOptions(WithDebugDisabled())
		if options.DebugWriter != io.Discard {
			t.Errorf("Expected io.Discard, got %T", options.DebugWriter)
		}
	})
}

// T037: OutputFormat Option - Structured Output Support (Issue #29)
func TestWithOutputFormat(t *testing.T) {
	tests := []struct {
		name     string
		setup    func() *Options
		validate func(*testing.T, *Options)
	}{
		{
			name: "json_schema_with_full_schema",
			setup: func() *Options {
				schema := map[string]any{
					"type": "object",
					"properties": map[string]any{
						"name":  map[string]any{"type": "string"},
						"count": map[string]any{"type": "integer"},
					},
					"required": []string{"name"},
				}
				return NewOptions(WithOutputFormat(OutputFormatJSONSchema(schema)))
			},
			validate: func(t *testing.T, opts *Options) {
				assertOutputFormatType(t, opts, "json_schema")
				assertOutputFormatHasSchema(t, opts)
			},
		},
		{
			name: "nil_output_format_by_default",
			setup: func() *Options {
				return NewOptions()
			},
			validate: func(t *testing.T, opts *Options) {
				assertOutputFormatNil(t, opts)
			},
		},
		{
			name: "empty_schema",
			setup: func() *Options {
				return NewOptions(WithOutputFormat(OutputFormatJSONSchema(map[string]any{})))
			},
			validate: func(t *testing.T, opts *Options) {
				assertOutputFormatType(t, opts, "json_schema")
			},
		},
		{
			name: "nil_output_format_option",
			setup: func() *Options {
				return NewOptions(WithOutputFormat(nil))
			},
			validate: func(t *testing.T, opts *Options) {
				assertOutputFormatNil(t, opts)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := tt.setup()
			tt.validate(t, opts)
		})
	}
}

// T038: WithJSONSchema Convenience Function
func TestWithJSONSchema(t *testing.T) {
	tests := []struct {
		name     string
		schema   map[string]any
		validate func(*testing.T, *Options)
	}{
		{
			name: "simple_object_schema",
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"result": map[string]any{"type": "string"},
				},
			},
			validate: func(t *testing.T, opts *Options) {
				assertOutputFormatType(t, opts, "json_schema")
				assertOutputFormatHasSchema(t, opts)
			},
		},
		{
			name:   "nil_schema",
			schema: nil,
			validate: func(t *testing.T, opts *Options) {
				assertOutputFormatNil(t, opts)
			},
		},
		{
			name:   "empty_schema",
			schema: map[string]any{},
			validate: func(t *testing.T, opts *Options) {
				assertOutputFormatType(t, opts, "json_schema")
				// Empty schema is valid
				if opts.OutputFormat.Schema == nil {
					t.Error("Expected Schema to be set (even if empty)")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := NewOptions(WithJSONSchema(tt.schema))
			tt.validate(t, opts)
		})
	}
}

// T039: OutputFormat Override Behavior
func TestOutputFormatOverride(t *testing.T) {
	firstSchema := map[string]any{"type": "string"}
	secondSchema := map[string]any{"type": "object"}

	opts := NewOptions(
		WithJSONSchema(firstSchema),
		WithJSONSchema(secondSchema),
	)

	assertOutputFormatType(t, opts, "json_schema")
	// Second schema should override first
	if opts.OutputFormat.Schema["type"] != "object" {
		t.Errorf("Expected schema type 'object', got %v", opts.OutputFormat.Schema["type"])
	}
}

// T040: OutputFormat Integration with Other Options
func TestOutputFormatIntegration(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"answer": map[string]any{"type": "string"},
		},
	}

	opts := NewOptions(
		WithSystemPrompt("You are helpful"),
		WithJSONSchema(schema),
		WithModel("claude-3-5-sonnet-20241022"),
		WithPermissionMode(PermissionModeAcceptEdits),
	)

	// Verify all options are set correctly
	assertOptionsSystemPrompt(t, opts, "You are helpful")
	assertOutputFormatType(t, opts, "json_schema")
	assertOptionsModel(t, opts, "claude-3-5-sonnet-20241022")
	assertOptionsPermissionMode(t, opts, PermissionModeAcceptEdits)
}

// Helper functions for OutputFormat tests

// assertOutputFormatNil verifies OutputFormat is nil
func assertOutputFormatNil(t *testing.T, opts *Options) {
	t.Helper()
	if opts.OutputFormat != nil {
		t.Errorf("Expected OutputFormat = nil, got %v", opts.OutputFormat)
	}
}

// assertOutputFormatType verifies OutputFormat.Type value
func assertOutputFormatType(t *testing.T, opts *Options, expectedType string) {
	t.Helper()
	if opts.OutputFormat == nil {
		t.Error("Expected OutputFormat to be set, got nil")
		return
	}
	if opts.OutputFormat.Type != expectedType {
		t.Errorf("Expected OutputFormat.Type = %q, got %q", expectedType, opts.OutputFormat.Type)
	}
}

// assertOutputFormatHasSchema verifies OutputFormat.Schema is set
func assertOutputFormatHasSchema(t *testing.T, opts *Options) {
	t.Helper()
	if opts.OutputFormat == nil {
		t.Error("Expected OutputFormat to be set, got nil")
		return
	}
	if opts.OutputFormat.Schema == nil {
		t.Error("Expected OutputFormat.Schema to be set")
	}
}

// T041: Sandbox Settings Option
func TestWithSandbox(t *testing.T) {
	tests := []struct {
		name     string
		sandbox  *SandboxSettings
		validate func(*testing.T, *Options)
	}{
		{
			name: "full_sandbox_settings",
			sandbox: &SandboxSettings{
				Enabled:                  true,
				AutoAllowBashIfSandboxed: true,
				ExcludedCommands:         []string{"docker", "git"},
				AllowUnsandboxedCommands: false,
				Network: &SandboxNetworkConfig{
					AllowUnixSockets:  []string{"/var/run/docker.sock"},
					AllowLocalBinding: true,
				},
				IgnoreViolations: &SandboxIgnoreViolations{
					File:    []string{"/tmp/*"},
					Network: []string{"localhost"},
				},
				EnableWeakerNestedSandbox: false,
			},
			validate: func(t *testing.T, options *Options) {
				t.Helper()
				assertOptionsSandboxNotNil(t, options)
				assertOptionsSandboxEnabled(t, options, true)
				assertOptionsSandboxAutoAllow(t, options, true)
				assertOptionsSandboxExcludedCommands(t, options, []string{"docker", "git"})
				assertOptionsSandboxAllowUnsandboxed(t, options, false)
				assertOptionsSandboxNetworkNotNil(t, options)
				assertOptionsSandboxNetworkAllowLocalBinding(t, options, true)
				assertOptionsSandboxIgnoreViolationsNotNil(t, options)
			},
		},
		{
			name: "minimal_sandbox_enabled",
			sandbox: &SandboxSettings{
				Enabled: true,
			},
			validate: func(t *testing.T, options *Options) {
				t.Helper()
				assertOptionsSandboxNotNil(t, options)
				assertOptionsSandboxEnabled(t, options, true)
			},
		},
		{
			name:    "nil_sandbox",
			sandbox: nil,
			validate: func(t *testing.T, options *Options) {
				t.Helper()
				assertOptionsSandboxNil(t, options)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options := NewOptions(WithSandbox(tt.sandbox))
			tt.validate(t, options)
		})
	}
}

// T042: Sandbox Enabled Convenience Option
func TestWithSandboxEnabled(t *testing.T) {
	tests := []struct {
		name     string
		enabled  bool
		expected bool
	}{
		{"enabled", true, true},
		{"disabled", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options := NewOptions(WithSandboxEnabled(tt.enabled))
			assertOptionsSandboxNotNil(t, options)
			assertOptionsSandboxEnabled(t, options, tt.expected)
		})
	}
}

// T043: Auto Allow Bash Convenience Option
func TestWithAutoAllowBashIfSandboxed(t *testing.T) {
	tests := []struct {
		name      string
		autoAllow bool
		expected  bool
	}{
		{"enabled", true, true},
		{"disabled", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options := NewOptions(WithAutoAllowBashIfSandboxed(tt.autoAllow))
			assertOptionsSandboxNotNil(t, options)
			assertOptionsSandboxAutoAllow(t, options, tt.expected)
		})
	}
}

// T044: Sandbox Excluded Commands Option
func TestWithSandboxExcludedCommands(t *testing.T) {
	tests := []struct {
		name     string
		commands []string
		expected []string
	}{
		{"single_command", []string{"docker"}, []string{"docker"}},
		{"multiple_commands", []string{"docker", "git", "npm"}, []string{"docker", "git", "npm"}},
		{"empty_commands", []string{}, []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options := NewOptions(WithSandboxExcludedCommands(tt.commands...))
			assertOptionsSandboxNotNil(t, options)
			assertOptionsSandboxExcludedCommands(t, options, tt.expected)
		})
	}
}

// T045: Sandbox Network Configuration Option
func TestWithSandboxNetwork(t *testing.T) {
	tests := []struct {
		name     string
		network  *SandboxNetworkConfig
		validate func(*testing.T, *Options)
	}{
		{
			name: "full_network_config",
			network: &SandboxNetworkConfig{
				AllowUnixSockets:    []string{"/var/run/docker.sock", "/tmp/socket"},
				AllowAllUnixSockets: false,
				AllowLocalBinding:   true,
				HTTPProxyPort:       intPtr(8080),
				SOCKSProxyPort:      intPtr(1080),
			},
			validate: func(t *testing.T, options *Options) {
				t.Helper()
				assertOptionsSandboxNotNil(t, options)
				assertOptionsSandboxNetworkNotNil(t, options)
				assertOptionsSandboxNetworkAllowLocalBinding(t, options, true)
				assertOptionsSandboxNetworkUnixSockets(t, options, []string{"/var/run/docker.sock", "/tmp/socket"})
			},
		},
		{
			name:    "nil_network",
			network: nil,
			validate: func(t *testing.T, options *Options) {
				t.Helper()
				assertOptionsSandboxNotNil(t, options)
				assertOptionsSandboxNetworkNil(t, options)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options := NewOptions(WithSandboxNetwork(tt.network))
			tt.validate(t, options)
		})
	}
}

// T046: Sandbox Options Composition
func TestSandboxOptionsComposition(t *testing.T) {
	// Test that multiple sandbox options compose correctly
	options := NewOptions(
		WithSandboxEnabled(true),
		WithAutoAllowBashIfSandboxed(true),
		WithSandboxExcludedCommands("docker", "git"),
		WithSandboxNetwork(&SandboxNetworkConfig{
			AllowLocalBinding: true,
		}),
	)

	assertOptionsSandboxNotNil(t, options)
	assertOptionsSandboxEnabled(t, options, true)
	assertOptionsSandboxAutoAllow(t, options, true)
	assertOptionsSandboxExcludedCommands(t, options, []string{"docker", "git"})
	assertOptionsSandboxNetworkAllowLocalBinding(t, options, true)
}

// T047: Sandbox Option Override Behavior
func TestSandboxOptionOverride(t *testing.T) {
	// Test that WithSandbox replaces previous sandbox settings
	t.Run("full_replace", func(t *testing.T) {
		options := NewOptions(
			WithSandboxEnabled(true),
			WithSandbox(&SandboxSettings{
				Enabled:                  false,
				AutoAllowBashIfSandboxed: true,
			}),
		)
		assertOptionsSandboxEnabled(t, options, false)
		assertOptionsSandboxAutoAllow(t, options, true)
	})

	// Test that convenience options initialize nil sandbox
	t.Run("initialize_nil_sandbox", func(t *testing.T) {
		options := NewOptions()
		assertOptionsSandboxNil(t, options)

		options = NewOptions(WithSandboxEnabled(true))
		assertOptionsSandboxNotNil(t, options)
		assertOptionsSandboxEnabled(t, options, true)
	})
}

// T048: Sandbox Integration with Other Options
func TestSandboxIntegrationWithOtherOptions(t *testing.T) {
	options := NewOptions(
		WithSystemPrompt("You are a helpful assistant"),
		WithModel("claude-3-5-sonnet-20241022"),
		WithSandbox(&SandboxSettings{
			Enabled:                  true,
			AutoAllowBashIfSandboxed: true,
		}),
		WithPermissionMode(PermissionModeAcceptEdits),
	)

	// Verify sandbox settings preserved alongside other options
	assertOptionsSystemPrompt(t, options, "You are a helpful assistant")
	assertOptionsModel(t, options, "claude-3-5-sonnet-20241022")
	assertOptionsPermissionMode(t, options, PermissionModeAcceptEdits)
	assertOptionsSandboxNotNil(t, options)
	assertOptionsSandboxEnabled(t, options, true)
}

// T049: Sandbox Nil by Default
func TestSandboxNilByDefault(t *testing.T) {
	options := NewOptions()
	assertOptionsSandboxNil(t, options)
}

// Helper functions for sandbox options

// assertOptionsSandboxNil verifies Sandbox is nil
func assertOptionsSandboxNil(t *testing.T, options *Options) {
	t.Helper()
	if options.Sandbox != nil {
		t.Errorf("Expected Sandbox = nil, got %+v", options.Sandbox)
	}
}

// assertOptionsSandboxNotNil verifies Sandbox is not nil
func assertOptionsSandboxNotNil(t *testing.T, options *Options) {
	t.Helper()
	if options.Sandbox == nil {
		t.Error("Expected Sandbox to be set, got nil")
	}
}

// assertOptionsSandboxEnabled verifies Sandbox.Enabled value
func assertOptionsSandboxEnabled(t *testing.T, options *Options, expected bool) {
	t.Helper()
	if options.Sandbox == nil {
		t.Error("Expected Sandbox to be set, got nil")
		return
	}
	if options.Sandbox.Enabled != expected {
		t.Errorf("Expected Sandbox.Enabled = %v, got %v", expected, options.Sandbox.Enabled)
	}
}

// assertOptionsSandboxAutoAllow verifies Sandbox.AutoAllowBashIfSandboxed value
func assertOptionsSandboxAutoAllow(t *testing.T, options *Options, expected bool) {
	t.Helper()
	if options.Sandbox == nil {
		t.Error("Expected Sandbox to be set, got nil")
		return
	}
	if options.Sandbox.AutoAllowBashIfSandboxed != expected {
		t.Errorf("Expected Sandbox.AutoAllowBashIfSandboxed = %v, got %v", expected, options.Sandbox.AutoAllowBashIfSandboxed)
	}
}

// assertOptionsSandboxAllowUnsandboxed verifies Sandbox.AllowUnsandboxedCommands value
func assertOptionsSandboxAllowUnsandboxed(t *testing.T, options *Options, expected bool) {
	t.Helper()
	if options.Sandbox == nil {
		t.Error("Expected Sandbox to be set, got nil")
		return
	}
	if options.Sandbox.AllowUnsandboxedCommands != expected {
		t.Errorf("Expected Sandbox.AllowUnsandboxedCommands = %v, got %v", expected, options.Sandbox.AllowUnsandboxedCommands)
	}
}

// assertOptionsSandboxExcludedCommands verifies Sandbox.ExcludedCommands slice
func assertOptionsSandboxExcludedCommands(t *testing.T, options *Options, expected []string) {
	t.Helper()
	if options.Sandbox == nil {
		t.Error("Expected Sandbox to be set, got nil")
		return
	}
	if len(options.Sandbox.ExcludedCommands) != len(expected) {
		t.Errorf("Expected Sandbox.ExcludedCommands length = %d, got %d", len(expected), len(options.Sandbox.ExcludedCommands))
		return
	}
	for i, exp := range expected {
		if options.Sandbox.ExcludedCommands[i] != exp {
			t.Errorf("Expected Sandbox.ExcludedCommands[%d] = %q, got %q", i, exp, options.Sandbox.ExcludedCommands[i])
		}
	}
}

// assertOptionsSandboxNetworkNil verifies Sandbox.Network is nil
func assertOptionsSandboxNetworkNil(t *testing.T, options *Options) {
	t.Helper()
	if options.Sandbox == nil {
		t.Error("Expected Sandbox to be set, got nil")
		return
	}
	if options.Sandbox.Network != nil {
		t.Errorf("Expected Sandbox.Network = nil, got %+v", options.Sandbox.Network)
	}
}

// assertOptionsSandboxNetworkNotNil verifies Sandbox.Network is not nil
func assertOptionsSandboxNetworkNotNil(t *testing.T, options *Options) {
	t.Helper()
	if options.Sandbox == nil {
		t.Error("Expected Sandbox to be set, got nil")
		return
	}
	if options.Sandbox.Network == nil {
		t.Error("Expected Sandbox.Network to be set, got nil")
	}
}

// assertOptionsSandboxNetworkAllowLocalBinding verifies Network.AllowLocalBinding
func assertOptionsSandboxNetworkAllowLocalBinding(t *testing.T, options *Options, expected bool) {
	t.Helper()
	if options.Sandbox == nil || options.Sandbox.Network == nil {
		t.Error("Expected Sandbox.Network to be set, got nil")
		return
	}
	if options.Sandbox.Network.AllowLocalBinding != expected {
		t.Errorf("Expected Network.AllowLocalBinding = %v, got %v", expected, options.Sandbox.Network.AllowLocalBinding)
	}
}

// assertOptionsSandboxNetworkUnixSockets verifies Network.AllowUnixSockets
func assertOptionsSandboxNetworkUnixSockets(t *testing.T, options *Options, expected []string) {
	t.Helper()
	if options.Sandbox == nil || options.Sandbox.Network == nil {
		t.Error("Expected Sandbox.Network to be set, got nil")
		return
	}
	if len(options.Sandbox.Network.AllowUnixSockets) != len(expected) {
		t.Errorf("Expected Network.AllowUnixSockets length = %d, got %d", len(expected), len(options.Sandbox.Network.AllowUnixSockets))
		return
	}
	for i, exp := range expected {
		if options.Sandbox.Network.AllowUnixSockets[i] != exp {
			t.Errorf("Expected Network.AllowUnixSockets[%d] = %q, got %q", i, exp, options.Sandbox.Network.AllowUnixSockets[i])
		}
	}
}

// assertOptionsSandboxIgnoreViolationsNotNil verifies Sandbox.IgnoreViolations is not nil
func assertOptionsSandboxIgnoreViolationsNotNil(t *testing.T, options *Options) {
	t.Helper()
	if options.Sandbox == nil {
		t.Error("Expected Sandbox to be set, got nil")
		return
	}
	if options.Sandbox.IgnoreViolations == nil {
		t.Error("Expected Sandbox.IgnoreViolations to be set, got nil")
	}
}

// intPtr creates a pointer to an int
func intPtr(i int) *int {
	return &i
}

// T050: Agent Definition Options
func TestAgentDefinitionOptions(t *testing.T) {
	t.Run("single_agent", func(t *testing.T) {
		options := NewOptions(WithAgent("code-reviewer", AgentDefinition{
			Description: "Reviews code for best practices",
			Prompt:      "You are a code reviewer...",
			Tools:       []string{"Read", "Grep"},
			Model:       AgentModelSonnet,
		}))

		assertOptionsAgentsLength(t, options, 1)
		agent, exists := options.Agents["code-reviewer"]
		if !exists {
			t.Fatal("Expected code-reviewer agent to exist")
		}
		assertAgentDefinition(t, agent, "Reviews code for best practices", "You are a code reviewer...", []string{"Read", "Grep"}, AgentModelSonnet)
	})

	t.Run("multiple_agents", func(t *testing.T) {
		agents := map[string]AgentDefinition{
			"code-reviewer": {
				Description: "Reviews code",
				Prompt:      "You are a reviewer...",
				Tools:       []string{"Read"},
				Model:       AgentModelSonnet,
			},
			"test-writer": {
				Description: "Writes tests",
				Prompt:      "You are a tester...",
				Tools:       []string{"Write", "Bash"},
				Model:       AgentModelHaiku,
			},
		}
		options := NewOptions(WithAgents(agents))

		assertOptionsAgentsLength(t, options, 2)
		assertAgentExists(t, options, "code-reviewer")
		assertAgentExists(t, options, "test-writer")
	})

	t.Run("merge_agents", func(t *testing.T) {
		options := NewOptions(
			WithAgent("agent1", AgentDefinition{Description: "First", Prompt: "First prompt"}),
			WithAgent("agent2", AgentDefinition{Description: "Second", Prompt: "Second prompt"}),
		)

		assertOptionsAgentsLength(t, options, 2)
		assertAgentExists(t, options, "agent1")
		assertAgentExists(t, options, "agent2")
	})

	t.Run("override_same_name", func(t *testing.T) {
		options := NewOptions(
			WithAgent("agent", AgentDefinition{Description: "Original", Prompt: "Original prompt"}),
			WithAgent("agent", AgentDefinition{Description: "Updated", Prompt: "Updated prompt"}),
		)

		assertOptionsAgentsLength(t, options, 1)
		agent := options.Agents["agent"]
		if agent.Description != "Updated" {
			t.Errorf("Expected Description = %q, got %q", "Updated", agent.Description)
		}
	})

	t.Run("withagents_replaces_existing", func(t *testing.T) {
		options := NewOptions(
			WithAgent("agent1", AgentDefinition{Description: "First", Prompt: "First prompt"}),
			WithAgents(map[string]AgentDefinition{
				"agent2": {Description: "Second", Prompt: "Second prompt"},
			}),
		)

		// WithAgents should replace entirely
		assertOptionsAgentsLength(t, options, 1)
		assertAgentNotExists(t, options, "agent1")
		assertAgentExists(t, options, "agent2")
	})

	t.Run("nil_by_default", func(t *testing.T) {
		options := NewOptions()
		if options.Agents != nil {
			t.Errorf("Expected Agents = nil by default, got %v", options.Agents)
		}
	})

	t.Run("optional_fields", func(t *testing.T) {
		// Test agent with only required fields (description, prompt)
		options := NewOptions(WithAgent("minimal", AgentDefinition{
			Description: "Minimal agent",
			Prompt:      "You are minimal...",
		}))

		agent := options.Agents["minimal"]
		if len(agent.Tools) != 0 {
			t.Errorf("Expected empty Tools, got %v", agent.Tools)
		}
		if agent.Model != "" {
			t.Errorf("Expected empty Model, got %q", agent.Model)
		}
	})
}

// T051: Agent Model Constants
func TestAgentModelConstants(t *testing.T) {
	tests := []struct {
		name     string
		model    AgentModel
		expected string
	}{
		{"sonnet", AgentModelSonnet, "sonnet"},
		{"opus", AgentModelOpus, "opus"},
		{"haiku", AgentModelHaiku, "haiku"},
		{"inherit", AgentModelInherit, "inherit"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.model) != tt.expected {
				t.Errorf("Expected AgentModel%s = %q, got %q", tt.name, tt.expected, string(tt.model))
			}
		})
	}
}

// T052: Agent Options Integration
func TestAgentOptionsIntegration(t *testing.T) {
	options := NewOptions(
		WithSystemPrompt("System prompt"),
		WithModel("claude-3-5-sonnet-20241022"),
		WithAgent("code-reviewer", AgentDefinition{
			Description: "Reviews code",
			Prompt:      "You are a reviewer...",
			Tools:       []string{"Read", "Grep"},
			Model:       AgentModelSonnet,
		}),
		WithSettingSources(SettingSourceProject),
	)

	// Verify agents work with other options
	assertOptionsSystemPrompt(t, options, "System prompt")
	assertOptionsModel(t, options, "claude-3-5-sonnet-20241022")
	assertOptionsAgentsLength(t, options, 1)
	assertOptionsSettingSources(t, options, []SettingSource{SettingSourceProject})
}

// Helper functions for agent options

// assertOptionsAgentsLength verifies the number of agents
func assertOptionsAgentsLength(t *testing.T, options *Options, expected int) {
	t.Helper()
	if len(options.Agents) != expected {
		t.Errorf("Expected Agents length = %d, got %d", expected, len(options.Agents))
	}
}

// assertAgentExists verifies an agent exists in the map
func assertAgentExists(t *testing.T, options *Options, name string) {
	t.Helper()
	if _, exists := options.Agents[name]; !exists {
		t.Errorf("Expected agent %q to exist", name)
	}
}

// assertAgentNotExists verifies an agent does not exist in the map
func assertAgentNotExists(t *testing.T, options *Options, name string) {
	t.Helper()
	if _, exists := options.Agents[name]; exists {
		t.Errorf("Expected agent %q to not exist", name)
	}
}

// assertAgentDefinition verifies an agent's fields
func assertAgentDefinition(t *testing.T, agent AgentDefinition, description, prompt string, tools []string, model AgentModel) {
	t.Helper()
	if agent.Description != description {
		t.Errorf("Expected Description = %q, got %q", description, agent.Description)
	}
	if agent.Prompt != prompt {
		t.Errorf("Expected Prompt = %q, got %q", prompt, agent.Prompt)
	}
	if len(agent.Tools) != len(tools) {
		t.Errorf("Expected Tools length = %d, got %d", len(tools), len(agent.Tools))
	} else {
		for i, tool := range tools {
			if agent.Tools[i] != tool {
				t.Errorf("Expected Tools[%d] = %q, got %q", i, tool, agent.Tools[i])
			}
		}
	}
	if agent.Model != model {
		t.Errorf("Expected Model = %q, got %q", model, agent.Model)
	}
}

// TestIncludePartialMessagesOption tests partial message streaming functional option
func TestIncludePartialMessagesOption(t *testing.T) {
	tests := []struct {
		name     string
		setup    func() *Options
		expected bool
	}{
		{
			name: "enable_partial_messages",
			setup: func() *Options {
				return NewOptions(WithIncludePartialMessages(true))
			},
			expected: true,
		},
		{
			name: "disable_partial_messages",
			setup: func() *Options {
				return NewOptions(WithIncludePartialMessages(false))
			},
			expected: false,
		},
		{
			name: "convenience_function",
			setup: func() *Options {
				return NewOptions(WithPartialStreaming())
			},
			expected: true,
		},
		{
			name: "default_is_false",
			setup: func() *Options {
				return NewOptions()
			},
			expected: false,
		},
		{
			name: "override_partial_messages",
			setup: func() *Options {
				return NewOptions(
					WithIncludePartialMessages(true),
					WithIncludePartialMessages(false), // Should override
				)
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options := tt.setup()
			assertOptionsIncludePartialMessages(t, options, tt.expected)
		})
	}
}

// assertOptionsIncludePartialMessages verifies IncludePartialMessages field
func assertOptionsIncludePartialMessages(t *testing.T, options *Options, expected bool) {
	t.Helper()
	if options.IncludePartialMessages != expected {
		t.Errorf("expected IncludePartialMessages = %v, got %v", expected, options.IncludePartialMessages)
	}
}

// T053: Stderr Callback Option - Issue #53
func TestWithStderrCallback(t *testing.T) {
	tests := []struct {
		name     string
		setup    func() *Options
		validate func(t *testing.T, options *Options)
	}{
		{
			name: "callback_set",
			setup: func() *Options {
				return NewOptions(WithStderrCallback(func(_ string) {}))
			},
			validate: func(t *testing.T, options *Options) {
				t.Helper()
				if options.StderrCallback == nil {
					t.Error("Expected StderrCallback to be set, got nil")
				}
			},
		},
		{
			name: "nil_by_default",
			setup: func() *Options {
				return NewOptions()
			},
			validate: func(t *testing.T, options *Options) {
				t.Helper()
				if options.StderrCallback != nil {
					t.Error("Expected StderrCallback to be nil by default")
				}
			},
		},
		{
			name: "callback_invocation",
			setup: func() *Options {
				return NewOptions(WithStderrCallback(func(_ string) {
					// Verify callback is invocable by checking it compiles
				}))
			},
			validate: func(t *testing.T, options *Options) {
				t.Helper()
				if options.StderrCallback == nil {
					t.Fatal("Expected StderrCallback to be set")
				}
				// Invoke the callback to verify it works
				options.StderrCallback("test line")
			},
		},
		{
			name: "callback_captures_lines",
			setup: func() *Options {
				var received []string
				return NewOptions(WithStderrCallback(func(line string) {
					received = append(received, line)
				}))
			},
			validate: func(t *testing.T, options *Options) {
				t.Helper()
				if options.StderrCallback == nil {
					t.Fatal("Expected StderrCallback to be set")
				}
				// Invoke multiple times
				options.StderrCallback("line1")
				options.StderrCallback("line2")
				options.StderrCallback("line3")
			},
		},
		{
			name: "override_previous_callback",
			setup: func() *Options {
				return NewOptions(
					WithStderrCallback(func(_ string) { /* first */ }),
					WithStderrCallback(func(_ string) { /* second wins */ }),
				)
			},
			validate: func(t *testing.T, options *Options) {
				t.Helper()
				if options.StderrCallback == nil {
					t.Error("Expected StderrCallback to be set after override")
				}
			},
		},
		{
			name: "nil_callback_explicit",
			setup: func() *Options {
				return NewOptions(WithStderrCallback(nil))
			},
			validate: func(t *testing.T, options *Options) {
				t.Helper()
				if options.StderrCallback != nil {
					t.Error("Expected StderrCallback to be nil when explicitly set")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options := tt.setup()
			tt.validate(t, options)
		})
	}
}

// TestStderrCallbackIntegration tests stderr callback with other options
func TestStderrCallbackIntegration(t *testing.T) {
	var debugBuf bytes.Buffer

	options := NewOptions(
		WithSystemPrompt("You are a helpful assistant"),
		WithDebugWriter(&debugBuf),
		WithStderrCallback(func(_ string) {}),
		WithModel("claude-3-5-sonnet-20241022"),
		WithPermissionMode(PermissionModeAcceptEdits),
	)

	// Verify stderr callback is set
	if options.StderrCallback == nil {
		t.Error("Expected StderrCallback to be set")
	}

	// Verify debug writer is also set (they are independent)
	if options.DebugWriter == nil {
		t.Error("Expected DebugWriter to also be set")
	}

	// Verify other options are preserved
	assertOptionsSystemPrompt(t, options, "You are a helpful assistant")
	assertOptionsModel(t, options, "claude-3-5-sonnet-20241022")
	assertOptionsPermissionMode(t, options, PermissionModeAcceptEdits)
}

// TestStderrCallbackIndependentOfDebugWriter tests that both can coexist
func TestStderrCallbackIndependentOfDebugWriter(t *testing.T) {
	t.Run("both_set", func(t *testing.T) {
		var buf bytes.Buffer
		options := NewOptions(
			WithDebugWriter(&buf),
			WithStderrCallback(func(_ string) {}),
		)

		if options.DebugWriter == nil {
			t.Error("Expected DebugWriter to be set")
		}
		if options.StderrCallback == nil {
			t.Error("Expected StderrCallback to be set")
		}
	})

	t.Run("only_callback", func(t *testing.T) {
		options := NewOptions(WithStderrCallback(func(_ string) {}))

		if options.DebugWriter != nil {
			t.Error("Expected DebugWriter to be nil")
		}
		if options.StderrCallback == nil {
			t.Error("Expected StderrCallback to be set")
		}
	})

	t.Run("only_debugwriter", func(t *testing.T) {
		var buf bytes.Buffer
		options := NewOptions(WithDebugWriter(&buf))

		if options.DebugWriter == nil {
			t.Error("Expected DebugWriter to be set")
		}
		if options.StderrCallback != nil {
			t.Error("Expected StderrCallback to be nil")
		}
	})
}

// =============================================================================
// Permission Callback Option Tests (Issue #8)
// =============================================================================

// TestWithCanUseTool tests the permission callback option
func TestWithCanUseTool(t *testing.T) {
	tests := []struct {
		name     string
		setup    func() *Options
		validate func(t *testing.T, options *Options)
	}{
		{
			name: "callback_set",
			setup: func() *Options {
				return NewOptions(WithCanUseTool(func(
					_ context.Context,
					_ string,
					_ map[string]any,
					_ ToolPermissionContext,
				) (PermissionResult, error) {
					return NewPermissionResultAllow(), nil
				}))
			},
			validate: func(t *testing.T, options *Options) {
				t.Helper()
				if options.CanUseTool == nil {
					t.Error("Expected CanUseTool to be set, got nil")
				}
			},
		},
		{
			name: "nil_by_default",
			setup: func() *Options {
				return NewOptions()
			},
			validate: func(t *testing.T, options *Options) {
				t.Helper()
				if options.CanUseTool != nil {
					t.Error("Expected CanUseTool to be nil by default")
				}
			},
		},
		{
			name: "callback_invocation_allow",
			setup: func() *Options {
				return NewOptions(WithCanUseTool(func(
					_ context.Context,
					toolName string,
					_ map[string]any,
					_ ToolPermissionContext,
				) (PermissionResult, error) {
					if toolName == "Read" {
						return NewPermissionResultAllow(), nil
					}
					return NewPermissionResultDeny("denied"), nil
				}))
			},
			validate: func(t *testing.T, options *Options) {
				t.Helper()
				if options.CanUseTool == nil {
					t.Fatal("Expected CanUseTool to be set")
				}
				// Invoke the callback wrapper
				result, err := options.CanUseTool(
					context.Background(),
					"Read",
					map[string]any{"file_path": "/test.txt"},
					ToolPermissionContext{},
				)
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if result == nil {
					t.Error("Expected result, got nil")
				}
				// Check result is Allow type
				if allow, ok := result.(PermissionResultAllow); !ok {
					t.Errorf("Expected PermissionResultAllow, got %T", result)
				} else if allow.Behavior != "allow" {
					t.Errorf("Expected behavior 'allow', got %q", allow.Behavior)
				}
			},
		},
		{
			name: "callback_invocation_deny",
			setup: func() *Options {
				return NewOptions(WithCanUseTool(func(
					_ context.Context,
					_ string,
					_ map[string]any,
					_ ToolPermissionContext,
				) (PermissionResult, error) {
					return NewPermissionResultDeny("not allowed"), nil
				}))
			},
			validate: func(t *testing.T, options *Options) {
				t.Helper()
				if options.CanUseTool == nil {
					t.Fatal("Expected CanUseTool to be set")
				}
				result, err := options.CanUseTool(
					context.Background(),
					"Write",
					map[string]any{},
					ToolPermissionContext{},
				)
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				// Check result is Deny type
				if deny, ok := result.(PermissionResultDeny); !ok {
					t.Errorf("Expected PermissionResultDeny, got %T", result)
				} else {
					if deny.Behavior != "deny" {
						t.Errorf("Expected behavior 'deny', got %q", deny.Behavior)
					}
					if deny.Message != "not allowed" {
						t.Errorf("Expected message 'not allowed', got %q", deny.Message)
					}
				}
			},
		},
		{
			name: "callback_receives_parameters",
			setup: func() *Options {
				var receivedTool string
				var receivedInput map[string]any
				return NewOptions(WithCanUseTool(func(
					_ context.Context,
					toolName string,
					input map[string]any,
					_ ToolPermissionContext,
				) (PermissionResult, error) {
					receivedTool = toolName
					receivedInput = input
					// Verify parameters were passed correctly
					if receivedTool != "Edit" {
						return NewPermissionResultDeny("wrong tool"), nil
					}
					if receivedInput["file_path"] != "/test.go" {
						return NewPermissionResultDeny("wrong input"), nil
					}
					return NewPermissionResultAllow(), nil
				}))
			},
			validate: func(t *testing.T, options *Options) {
				t.Helper()
				if options.CanUseTool == nil {
					t.Fatal("Expected CanUseTool to be set")
				}
				result, err := options.CanUseTool(
					context.Background(),
					"Edit",
					map[string]any{"file_path": "/test.go"},
					ToolPermissionContext{},
				)
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if _, ok := result.(PermissionResultAllow); !ok {
					t.Errorf("Expected PermissionResultAllow (params matched), got %T", result)
				}
			},
		},
		{
			name: "override_previous_callback",
			setup: func() *Options {
				return NewOptions(
					WithCanUseTool(func(
						_ context.Context,
						_ string,
						_ map[string]any,
						_ ToolPermissionContext,
					) (PermissionResult, error) {
						return NewPermissionResultDeny("first"), nil
					}),
					WithCanUseTool(func(
						_ context.Context,
						_ string,
						_ map[string]any,
						_ ToolPermissionContext,
					) (PermissionResult, error) {
						return NewPermissionResultAllow(), nil // second wins
					}),
				)
			},
			validate: func(t *testing.T, options *Options) {
				t.Helper()
				if options.CanUseTool == nil {
					t.Fatal("Expected CanUseTool to be set after override")
				}
				result, _ := options.CanUseTool(
					context.Background(),
					"Test",
					nil,
					ToolPermissionContext{},
				)
				// Second callback should win (returns Allow)
				if _, ok := result.(PermissionResultAllow); !ok {
					t.Errorf("Expected second callback to win (Allow), got %T", result)
				}
			},
		},
		{
			name: "nil_callback_explicit",
			setup: func() *Options {
				return NewOptions(WithCanUseTool(nil))
			},
			validate: func(t *testing.T, options *Options) {
				t.Helper()
				if options.CanUseTool != nil {
					t.Error("Expected CanUseTool to be nil when explicitly set")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options := tt.setup()
			tt.validate(t, options)
		})
	}
}

// TestCanUseToolIntegration tests permission callback with other options
func TestCanUseToolIntegration(t *testing.T) {
	options := NewOptions(
		WithSystemPrompt("You are a helpful assistant"),
		WithModel("claude-3-5-sonnet-20241022"),
		WithPermissionMode(PermissionModeAcceptEdits),
		WithCanUseTool(func(
			_ context.Context,
			_ string,
			_ map[string]any,
			_ ToolPermissionContext,
		) (PermissionResult, error) {
			return NewPermissionResultAllow(), nil
		}),
	)

	// Verify permission callback is set
	if options.CanUseTool == nil {
		t.Error("Expected CanUseTool to be set")
	}

	// Verify other options are preserved
	assertOptionsSystemPrompt(t, options, "You are a helpful assistant")
	assertOptionsModel(t, options, "claude-3-5-sonnet-20241022")
	assertOptionsPermissionMode(t, options, PermissionModeAcceptEdits)
}

// TestPermissionResultConstructors tests the permission result helper functions
func TestPermissionResultConstructors(t *testing.T) {
	t.Run("NewPermissionResultAllow", func(t *testing.T) {
		result := NewPermissionResultAllow()
		if result.Behavior != "allow" {
			t.Errorf("Expected behavior 'allow', got %q", result.Behavior)
		}
		if result.UpdatedInput != nil {
			t.Error("Expected UpdatedInput to be nil by default")
		}
		if len(result.UpdatedPermissions) != 0 {
			t.Error("Expected UpdatedPermissions to be empty by default")
		}
	})

	t.Run("NewPermissionResultDeny", func(t *testing.T) {
		result := NewPermissionResultDeny("access denied")
		if result.Behavior != "deny" {
			t.Errorf("Expected behavior 'deny', got %q", result.Behavior)
		}
		if result.Message != "access denied" {
			t.Errorf("Expected message 'access denied', got %q", result.Message)
		}
		if result.Interrupt {
			t.Error("Expected Interrupt to be false by default")
		}
	})
}

// TestCanUseToolTypeConversion tests the type conversion fallback in WithCanUseTool
func TestCanUseToolTypeConversion(t *testing.T) {
	t.Run("non_matching_permCtx_type", func(t *testing.T) {
		// This tests the fallback when permCtx is not a ToolPermissionContext
		options := NewOptions(WithCanUseTool(func(
			_ context.Context,
			_ string,
			_ map[string]any,
			permCtx ToolPermissionContext,
		) (PermissionResult, error) {
			// Should receive empty ToolPermissionContext when type doesn't match
			if len(permCtx.Suggestions) != 0 {
				return NewPermissionResultDeny("unexpected suggestions"), nil
			}
			return NewPermissionResultAllow(), nil
		}))

		if options.CanUseTool == nil {
			t.Fatal("Expected CanUseTool to be set")
		}

		// Pass a non-matching type (string instead of ToolPermissionContext)
		// The wrapper should convert it to empty ToolPermissionContext
		result, err := options.CanUseTool(
			context.Background(),
			"Read",
			nil,
			"invalid_type", // Not a ToolPermissionContext
		)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if _, ok := result.(PermissionResultAllow); !ok {
			t.Errorf("Expected PermissionResultAllow, got %T", result)
		}
	})
}

// TestPermissionUpdateTypeConstants tests that constants are correctly exported
func TestPermissionUpdateTypeConstants(t *testing.T) {
	// Verify constants are accessible and have expected values
	tests := []struct {
		constant PermissionUpdateType
		expected string
	}{
		{PermissionUpdateTypeAddRules, "addRules"},
		{PermissionUpdateTypeReplaceRules, "replaceRules"},
		{PermissionUpdateTypeRemoveRules, "removeRules"},
		{PermissionUpdateTypeSetMode, "setMode"},
		{PermissionUpdateTypeAddDirectories, "addDirectories"},
		{PermissionUpdateTypeRemoveDirectories, "removeDirectories"},
	}

	for _, tt := range tests {
		if string(tt.constant) != tt.expected {
			t.Errorf("Expected %q, got %q", tt.expected, string(tt.constant))
		}
	}
}

// =============================================================================
// Hook Options Tests (Issue #9)
// =============================================================================

// TestHookEventConstants tests that hook event constants are correctly exported
func TestHookEventConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant HookEvent
		expected string
	}{
		{"pre_tool_use", HookEventPreToolUse, "PreToolUse"},
		{"post_tool_use", HookEventPostToolUse, "PostToolUse"},
		{"user_prompt_submit", HookEventUserPromptSubmit, "UserPromptSubmit"},
		{"stop", HookEventStop, "Stop"},
		{"subagent_stop", HookEventSubagentStop, "SubagentStop"},
		{"pre_compact", HookEventPreCompact, "PreCompact"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.constant) != tt.expected {
				t.Errorf("HookEvent constant %s = %q, want %q", tt.name, tt.constant, tt.expected)
			}
		})
	}
}

// TestWithHooks tests bulk hook registration
func TestWithHooks(t *testing.T) {
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
			{Matcher: "Write|Edit", Hooks: []HookCallback{callback}},
		},
		HookEventPostToolUse: {
			{Matcher: "", Hooks: []HookCallback{callback}},
		},
	}

	options := NewOptions(WithHooks(hooks))

	// Verify hooks are stored correctly
	if options.Hooks == nil {
		t.Fatal("Expected Hooks to be set")
	}

	storedHooks, ok := options.Hooks.(map[HookEvent][]HookMatcher)
	if !ok {
		t.Fatalf("Expected Hooks to be map[HookEvent][]HookMatcher, got %T", options.Hooks)
	}

	if len(storedHooks[HookEventPreToolUse]) != 2 {
		t.Errorf("Expected 2 PreToolUse matchers, got %d", len(storedHooks[HookEventPreToolUse]))
	}

	if len(storedHooks[HookEventPostToolUse]) != 1 {
		t.Errorf("Expected 1 PostToolUse matcher, got %d", len(storedHooks[HookEventPostToolUse]))
	}
}

// TestWithHook tests incremental hook addition
func TestWithHook(t *testing.T) {
	callback := func(
		_ context.Context,
		_ any,
		_ *string,
		_ HookContext,
	) (HookJSONOutput, error) {
		return HookJSONOutput{}, nil
	}

	options := NewOptions(
		WithHook(HookEventPreToolUse, "Bash", callback),
		WithHook(HookEventPreToolUse, "Write", callback),
		WithHook(HookEventPostToolUse, "", callback), // Empty matcher = all tools
	)

	if options.Hooks == nil {
		t.Fatal("Expected Hooks to be set")
	}

	storedHooks, ok := options.Hooks.(map[HookEvent][]HookMatcher)
	if !ok {
		t.Fatalf("Expected Hooks to be map[HookEvent][]HookMatcher, got %T", options.Hooks)
	}

	// Both Bash and Write hooks should be added to PreToolUse
	if len(storedHooks[HookEventPreToolUse]) != 2 {
		t.Errorf("Expected 2 PreToolUse matchers, got %d", len(storedHooks[HookEventPreToolUse]))
	}

	// PostToolUse should have 1 matcher
	if len(storedHooks[HookEventPostToolUse]) != 1 {
		t.Errorf("Expected 1 PostToolUse matcher, got %d", len(storedHooks[HookEventPostToolUse]))
	}

	// Verify matcher values
	if storedHooks[HookEventPreToolUse][0].Matcher != "Bash" {
		t.Errorf("First PreToolUse matcher = %q, want %q", storedHooks[HookEventPreToolUse][0].Matcher, "Bash")
	}
	if storedHooks[HookEventPreToolUse][1].Matcher != "Write" {
		t.Errorf("Second PreToolUse matcher = %q, want %q", storedHooks[HookEventPreToolUse][1].Matcher, "Write")
	}
	if storedHooks[HookEventPostToolUse][0].Matcher != "" {
		t.Errorf("PostToolUse matcher = %q, want empty string", storedHooks[HookEventPostToolUse][0].Matcher)
	}
}

// TestWithPreToolUseHook tests the convenience function
func TestWithPreToolUseHook(t *testing.T) {
	callback := func(
		_ context.Context,
		_ any,
		_ *string,
		_ HookContext,
	) (HookJSONOutput, error) {
		return HookJSONOutput{}, nil
	}

	options := NewOptions(WithPreToolUseHook("Bash", callback))

	if options.Hooks == nil {
		t.Fatal("Expected Hooks to be set")
	}

	storedHooks, ok := options.Hooks.(map[HookEvent][]HookMatcher)
	if !ok {
		t.Fatalf("Expected Hooks to be map[HookEvent][]HookMatcher, got %T", options.Hooks)
	}

	if len(storedHooks[HookEventPreToolUse]) != 1 {
		t.Errorf("Expected 1 PreToolUse matcher, got %d", len(storedHooks[HookEventPreToolUse]))
	}

	if storedHooks[HookEventPreToolUse][0].Matcher != "Bash" {
		t.Errorf("Matcher = %q, want %q", storedHooks[HookEventPreToolUse][0].Matcher, "Bash")
	}
}

// TestWithPostToolUseHook tests the convenience function
func TestWithPostToolUseHook(t *testing.T) {
	callback := func(
		_ context.Context,
		_ any,
		_ *string,
		_ HookContext,
	) (HookJSONOutput, error) {
		return HookJSONOutput{}, nil
	}

	options := NewOptions(WithPostToolUseHook("", callback)) // Empty = all tools

	if options.Hooks == nil {
		t.Fatal("Expected Hooks to be set")
	}

	storedHooks, ok := options.Hooks.(map[HookEvent][]HookMatcher)
	if !ok {
		t.Fatalf("Expected Hooks to be map[HookEvent][]HookMatcher, got %T", options.Hooks)
	}

	if len(storedHooks[HookEventPostToolUse]) != 1 {
		t.Errorf("Expected 1 PostToolUse matcher, got %d", len(storedHooks[HookEventPostToolUse]))
	}
}

// TestHookOptionsWithOtherOptions tests that hook options work with other options
func TestHookOptionsWithOtherOptions(t *testing.T) {
	callback := func(
		_ context.Context,
		_ any,
		_ *string,
		_ HookContext,
	) (HookJSONOutput, error) {
		return HookJSONOutput{}, nil
	}

	options := NewOptions(
		WithSystemPrompt("You are a helpful assistant"),
		WithModel("claude-3-5-sonnet-20241022"),
		WithHook(HookEventPreToolUse, "Bash", callback),
		WithPermissionMode(PermissionModeAcceptEdits),
	)

	// Verify hooks are set
	if options.Hooks == nil {
		t.Error("Expected Hooks to be set")
	}

	// Verify other options are preserved
	assertOptionsSystemPrompt(t, options, "You are a helpful assistant")
	assertOptionsModel(t, options, "claude-3-5-sonnet-20241022")
	assertOptionsPermissionMode(t, options, PermissionModeAcceptEdits)
}

// TestHookMatcherWithTimeout tests HookMatcher with timeout
func TestHookMatcherWithTimeout(t *testing.T) {
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

	options := NewOptions(WithHooks(hooks))

	storedHooks, ok := options.Hooks.(map[HookEvent][]HookMatcher)
	if !ok {
		t.Fatalf("Expected Hooks to be map[HookEvent][]HookMatcher, got %T", options.Hooks)
	}

	if storedHooks[HookEventPreToolUse][0].Timeout == nil {
		t.Fatal("Expected Timeout to be set")
	}

	if *storedHooks[HookEventPreToolUse][0].Timeout != 30.0 {
		t.Errorf("Timeout = %v, want 30.0", *storedHooks[HookEventPreToolUse][0].Timeout)
	}
}

// TestEmptyHooks tests empty hooks map
func TestEmptyHooks(t *testing.T) {
	options := NewOptions(WithHooks(map[HookEvent][]HookMatcher{}))

	if options.Hooks == nil {
		t.Fatal("Expected Hooks to be set (even if empty)")
	}

	storedHooks, ok := options.Hooks.(map[HookEvent][]HookMatcher)
	if !ok {
		t.Fatalf("Expected Hooks to be map[HookEvent][]HookMatcher, got %T", options.Hooks)
	}

	if len(storedHooks) != 0 {
		t.Errorf("Expected empty hooks map, got %d entries", len(storedHooks))
	}
}

// TestMultipleCallbacksPerMatcher tests multiple callbacks in a single matcher
func TestMultipleCallbacksPerMatcher(t *testing.T) {
	callback1 := func(_ context.Context, _ any, _ *string, _ HookContext) (HookJSONOutput, error) {
		return HookJSONOutput{}, nil
	}
	callback2 := func(_ context.Context, _ any, _ *string, _ HookContext) (HookJSONOutput, error) {
		return HookJSONOutput{}, nil
	}

	hooks := map[HookEvent][]HookMatcher{
		HookEventPreToolUse: {
			{Matcher: "Bash", Hooks: []HookCallback{callback1, callback2}},
		},
	}

	options := NewOptions(WithHooks(hooks))

	storedHooks, ok := options.Hooks.(map[HookEvent][]HookMatcher)
	if !ok {
		t.Fatalf("Expected Hooks to be map[HookEvent][]HookMatcher, got %T", options.Hooks)
	}

	if len(storedHooks[HookEventPreToolUse][0].Hooks) != 2 {
		t.Errorf("Expected 2 callbacks, got %d", len(storedHooks[HookEventPreToolUse][0].Hooks))
	}
}

// =============================================================================
// File Checkpointing Options Tests (Issue #32)
// =============================================================================

// TestFileCheckpointingOptions tests file checkpointing option functions
func TestFileCheckpointingOptions(t *testing.T) {
	t.Run("with_enable_file_checkpointing_true", func(t *testing.T) {
		opts := NewOptions(WithEnableFileCheckpointing(true))
		if !opts.EnableFileCheckpointing {
			t.Error("expected EnableFileCheckpointing to be true")
		}
	})

	t.Run("with_enable_file_checkpointing_false", func(t *testing.T) {
		opts := NewOptions(WithEnableFileCheckpointing(false))
		if opts.EnableFileCheckpointing {
			t.Error("expected EnableFileCheckpointing to be false")
		}
	})

	t.Run("with_file_checkpointing_convenience", func(t *testing.T) {
		opts := NewOptions(WithFileCheckpointing())
		if !opts.EnableFileCheckpointing {
			t.Error("expected EnableFileCheckpointing to be true")
		}
	})

	t.Run("default_is_disabled", func(t *testing.T) {
		opts := NewOptions()
		if opts.EnableFileCheckpointing {
			t.Error("expected EnableFileCheckpointing to be false by default")
		}
	})
}

// =============================================================================
// SDK MCP Server Options Tests (Issue #7)
// =============================================================================

// TestWithSdkMcpServer tests the SDK MCP server option function
func TestWithSdkMcpServer(t *testing.T) {
	t.Run("initializes_nil_map", func(t *testing.T) {
		// Test the nil map initialization branch by applying option directly
		// to an empty Options struct (not via NewOptions which pre-initializes)
		server := CreateSDKMcpServer("calculator", "1.0.0")
		opt := WithSdkMcpServer("calc", server)

		opts := &Options{} // McpServers is nil
		opt(opts)          // Apply option directly

		if opts.McpServers == nil {
			t.Fatal("expected McpServers to be initialized")
		}
		if len(opts.McpServers) != 1 {
			t.Errorf("expected 1 server, got %d", len(opts.McpServers))
		}
		if opts.McpServers["calc"] != server {
			t.Error("expected server to be stored under 'calc' key")
		}
	})

	t.Run("adds_server_via_new_options", func(t *testing.T) {
		server := CreateSDKMcpServer("calculator", "1.0.0")
		opts := NewOptions(WithSdkMcpServer("calc", server))

		if opts.McpServers == nil {
			t.Fatal("expected McpServers to be initialized")
		}
		if len(opts.McpServers) != 1 {
			t.Errorf("expected 1 server, got %d", len(opts.McpServers))
		}
		if opts.McpServers["calc"] != server {
			t.Error("expected server to be stored under 'calc' key")
		}
	})

	t.Run("adds_server_to_existing_map", func(t *testing.T) {
		server1 := CreateSDKMcpServer("calc1", "1.0.0")
		server2 := CreateSDKMcpServer("calc2", "1.0.0")

		opts := NewOptions(
			WithSdkMcpServer("first", server1),
			WithSdkMcpServer("second", server2),
		)

		if len(opts.McpServers) != 2 {
			t.Errorf("expected 2 servers, got %d", len(opts.McpServers))
		}
		if opts.McpServers["first"] != server1 {
			t.Error("expected server1 to be stored under 'first' key")
		}
		if opts.McpServers["second"] != server2 {
			t.Error("expected server2 to be stored under 'second' key")
		}
	})

	t.Run("server_has_correct_type", func(t *testing.T) {
		server := CreateSDKMcpServer("test", "1.0.0")
		opts := NewOptions(WithSdkMcpServer("test", server))

		stored := opts.McpServers["test"]
		sdkServer, ok := stored.(*McpSdkServerConfig)
		if !ok {
			t.Fatalf("expected *McpSdkServerConfig, got %T", stored)
		}
		if sdkServer.Type != McpServerTypeSdk {
			t.Errorf("expected type %q, got %q", McpServerTypeSdk, sdkServer.Type)
		}
	})
}
