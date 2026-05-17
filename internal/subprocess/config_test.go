package subprocess

import (
	"context"
	"encoding/json"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/severity1/claude-agent-sdk-go/internal/shared"
)

// TestTransportEnvironmentSetup tests environment variable and platform compatibility
func TestTransportEnvironmentSetup(t *testing.T) {
	ctx, cancel := setupTransportTestContext(t, 10*time.Second)
	defer cancel()

	transport := setupTransportForTest(t, newTransportMockCLIWithOptions(WithEnvironmentCheck()))
	defer disconnectTransportSafely(t, transport)

	// Connection should succeed with proper environment setup
	connectTransportSafely(ctx, t, transport)
	assertTransportConnected(t, transport, true)

	// Test interrupt (platform-specific signals)
	if runtime.GOOS != windowsOS {
		err := transport.Interrupt(ctx)
		assertNoTransportError(t, err)
	}
}

// TestSubprocessEnvironmentVariables tests environment variable passing to subprocess
func TestSubprocessEnvironmentVariables(t *testing.T) {
	// Following client_test.go patterns for test organization
	setupSubprocessTestContext := func(t *testing.T) (context.Context, context.CancelFunc) {
		t.Helper()
		return context.WithTimeout(context.Background(), 5*time.Second)
	}

	tests := []struct {
		name     string
		options  *shared.Options
		validate func(t *testing.T, env []string)
	}{
		{
			name: "custom_env_vars_passed",
			options: &shared.Options{
				ExtraEnv: map[string]string{
					"TEST_VAR": "test_value",
					"DEBUG":    "1",
				},
			},
			validate: func(t *testing.T, env []string) {
				assertEnvContains(t, env, "TEST_VAR=test_value")
				assertEnvContains(t, env, "DEBUG=1")
				assertEnvContains(t, env, "CLAUDE_CODE_ENTRYPOINT=sdk-go")
			},
		},
		{
			name: "system_env_preserved",
			options: &shared.Options{
				ExtraEnv: map[string]string{"CUSTOM": "value"},
			},
			validate: func(t *testing.T, env []string) {
				// Verify system environment is preserved by checking for common env vars
				// Don't rely on PATH alone since it might have platform-specific casing
				systemEnvFound := false
				expectedEnvVars := []string{"PATH", "Path", "HOME", "USERPROFILE", "USER", "USERNAME"}

				for _, expectedVar := range expectedEnvVars {
					if os.Getenv(expectedVar) != "" {
						// Check if this env var exists in the subprocess environment
						for _, envVar := range env {
							if strings.HasPrefix(strings.ToUpper(envVar), strings.ToUpper(expectedVar)+"=") {
								systemEnvFound = true
								break
							}
						}
						if systemEnvFound {
							break
						}
					}
				}

				if !systemEnvFound {
					// Show first few env vars for debugging, but limit to avoid log spam
					envSample := env
					if len(envSample) > 5 {
						envSample = env[:5]
					}
					t.Errorf("Expected system environment to be preserved. System env has PATH=%q, subprocess env sample: %v",
						os.Getenv("PATH"), envSample)
				}

				assertEnvContains(t, env, "CUSTOM=value")
				assertEnvContains(t, env, "CLAUDE_CODE_ENTRYPOINT=sdk-go")
			},
		},
		{
			name: "nil_extra_env_works",
			options: &shared.Options{
				ExtraEnv: nil,
			},
			validate: func(t *testing.T, env []string) {
				assertEnvContains(t, env, "CLAUDE_CODE_ENTRYPOINT=sdk-go")
			},
		},
		{
			name: "empty_extra_env_works",
			options: &shared.Options{
				ExtraEnv: map[string]string{},
			},
			validate: func(t *testing.T, env []string) {
				assertEnvContains(t, env, "CLAUDE_CODE_ENTRYPOINT=sdk-go")
			},
		},
		{
			name: "proxy_configuration_example",
			options: &shared.Options{
				ExtraEnv: map[string]string{
					"HTTP_PROXY":  "http://proxy.example.com:8080",
					"HTTPS_PROXY": "http://proxy.example.com:8080",
					"NO_PROXY":    "localhost,127.0.0.1",
				},
			},
			validate: func(t *testing.T, env []string) {
				assertEnvContains(t, env, "HTTP_PROXY=http://proxy.example.com:8080")
				assertEnvContains(t, env, "HTTPS_PROXY=http://proxy.example.com:8080")
				assertEnvContains(t, env, "NO_PROXY=localhost,127.0.0.1")
				assertEnvContains(t, env, "CLAUDE_CODE_ENTRYPOINT=sdk-go")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := setupSubprocessTestContext(t)
			defer cancel()

			transport := New(newTransportMockCLI(), tt.options, "sdk-go")
			defer func() {
				if transport.IsConnected() {
					_ = transport.Close()
				}
			}()

			err := transport.Connect(ctx)
			assertNoTransportError(t, err)

			if transport.cmd != nil && transport.cmd.Env != nil {
				tt.validate(t, transport.cmd.Env)
			} else {
				t.Error("Expected command environment to be set")
			}
		})
	}
}

// TestTransportWorkingDirectory tests that working directory is set via exec.Cmd.Dir
func TestTransportWorkingDirectory(t *testing.T) {
	ctx, cancel := setupTransportTestContext(t, 5*time.Second)
	defer cancel()

	tests := []struct {
		name     string
		setup    func() *Transport
		validate func(*testing.T, *Transport)
	}{
		{
			name: "working_directory_set_via_cmd_dir",
			setup: func() *Transport {
				cwd := t.TempDir()
				options := &shared.Options{
					Cwd: &cwd,
				}
				return New(newTransportMockCLI(), options, "sdk-go")
			},
			validate: func(t *testing.T, transport *Transport) {
				t.Helper()
				if transport.cmd == nil {
					t.Fatal("Expected command to be initialized after Connect()")
				}
				expectedCwd := *transport.options.Cwd
				if transport.cmd.Dir != expectedCwd {
					t.Errorf("Expected cmd.Dir to be %s, got %s", expectedCwd, transport.cmd.Dir)
				}
			},
		},
		{
			name: "no_working_directory_when_cwd_nil",
			setup: func() *Transport {
				options := &shared.Options{
					Cwd: nil,
				}
				return New(newTransportMockCLI(), options, "sdk-go")
			},
			validate: func(t *testing.T, transport *Transport) {
				t.Helper()
				if transport.cmd == nil {
					t.Fatal("Expected command to be initialized after Connect()")
				}
				if transport.cmd.Dir != "" {
					t.Errorf("Expected cmd.Dir to be empty when Cwd is nil, got %s", transport.cmd.Dir)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := tt.setup()
			defer disconnectTransportSafely(t, transport)

			// Connect to initialize the command
			err := transport.Connect(ctx)
			assertNoTransportError(t, err)

			// Validate working directory was set correctly via cmd.Dir
			tt.validate(t, transport)
		})
	}
}

// TestTransportMcpServerConfiguration tests MCP server config file generation
func TestTransportMcpServerConfiguration(t *testing.T) {
	ctx, cancel := setupTransportTestContext(t, 5*time.Second)
	defer cancel()

	tests := []struct {
		name     string
		setup    func() *Transport
		validate func(*testing.T, *Transport)
	}{
		{
			name: "mcp_config_file_generated",
			setup: func() *Transport {
				mcpServers := map[string]shared.McpServerConfig{
					"test-server": &shared.McpStdioServerConfig{
						Type:    shared.McpServerTypeStdio,
						Command: "node",
						Args:    []string{"test-server.js"},
						Env:     map[string]string{"TEST_VAR": "test_value"},
					},
				}
				options := &shared.Options{
					McpServers: mcpServers,
				}
				return New(newTransportMockCLI(), options, "sdk-go")
			},
			validate: func(t *testing.T, transport *Transport) {
				t.Helper()
				if transport.mcpConfigFile == nil {
					t.Fatal("Expected MCP config file to be generated")
				}

				// Read and verify the generated config file
				configData, err := os.ReadFile(transport.mcpConfigFile.Name())
				if err != nil {
					t.Fatalf("Failed to read MCP config file: %v", err)
				}

				// Verify it's valid JSON
				var config map[string]interface{}
				if err := json.Unmarshal(configData, &config); err != nil {
					t.Fatalf("MCP config is not valid JSON: %v", err)
				}

				// Verify structure
				mcpServers, ok := config["mcpServers"].(map[string]interface{})
				if !ok {
					t.Fatal("MCP config missing 'mcpServers' key")
				}

				testServer, ok := mcpServers["test-server"].(map[string]interface{})
				if !ok {
					t.Fatal("MCP config missing 'test-server'")
				}

				if testServer["command"] != "node" {
					t.Errorf("Expected command 'node', got %v", testServer["command"])
				}
			},
		},
		{
			name: "mcp_config_file_cleaned_up",
			setup: func() *Transport {
				mcpServers := map[string]shared.McpServerConfig{
					"cleanup-test": &shared.McpStdioServerConfig{
						Type:    shared.McpServerTypeStdio,
						Command: "echo",
						Args:    []string{"test"},
					},
				}
				options := &shared.Options{
					McpServers: mcpServers,
				}
				return New(newTransportMockCLI(), options, "sdk-go")
			},
			validate: func(t *testing.T, transport *Transport) {
				t.Helper()
				if transport.mcpConfigFile == nil {
					t.Fatal("Expected MCP config file to be generated")
				}

				configPath := transport.mcpConfigFile.Name()

				// Verify file exists while transport is connected
				if _, err := os.Stat(configPath); os.IsNotExist(err) {
					t.Error("MCP config file should exist while transport is connected")
				}

				// Close transport and verify cleanup
				if err := transport.Close(); err != nil {
					t.Errorf("Failed to close transport: %v", err)
				}

				// Verify file is deleted after cleanup
				if _, err := os.Stat(configPath); !os.IsNotExist(err) {
					t.Error("MCP config file should be deleted after transport Close()")
				}
			},
		},
		{
			name: "no_mcp_config_when_servers_empty",
			setup: func() *Transport {
				options := &shared.Options{
					McpServers: nil,
				}
				return New(newTransportMockCLI(), options, "sdk-go")
			},
			validate: func(t *testing.T, transport *Transport) {
				t.Helper()
				if transport.mcpConfigFile != nil {
					t.Error("MCP config file should not be generated when McpServers is empty")
				}
			},
		},
		{
			name: "stdio_alwaysLoad_propagates_to_config_file",
			setup: func() *Transport {
				mcpServers := map[string]shared.McpServerConfig{
					"always-loaded": &shared.McpStdioServerConfig{
						Type:       shared.McpServerTypeStdio,
						Command:    "node",
						AlwaysLoad: true,
					},
					"deferred": &shared.McpStdioServerConfig{
						Type:    shared.McpServerTypeStdio,
						Command: "node",
					},
				}
				options := &shared.Options{McpServers: mcpServers}
				return New(newTransportMockCLI(), options, "sdk-go")
			},
			validate: func(t *testing.T, transport *Transport) {
				t.Helper()
				if transport.mcpConfigFile == nil {
					t.Fatal("Expected MCP config file to be generated")
				}
				configData, err := os.ReadFile(transport.mcpConfigFile.Name())
				if err != nil {
					t.Fatalf("Failed to read MCP config file: %v", err)
				}
				var config map[string]interface{}
				if err := json.Unmarshal(configData, &config); err != nil {
					t.Fatalf("MCP config is not valid JSON: %v", err)
				}
				servers := config["mcpServers"].(map[string]interface{})

				always := servers["always-loaded"].(map[string]interface{})
				if always["alwaysLoad"] != true {
					t.Errorf("Expected alwaysLoad=true on always-loaded server, got %v", always["alwaysLoad"])
				}

				deferred := servers["deferred"].(map[string]interface{})
				if _, present := deferred["alwaysLoad"]; present {
					t.Errorf("Expected alwaysLoad to be omitted when false, got %v", deferred["alwaysLoad"])
				}
			},
		},
		{
			name: "sdk_alwaysLoad_propagates_to_config_file",
			setup: func() *Transport {
				mcpServers := map[string]shared.McpServerConfig{
					"sdk-server": &shared.McpSdkServerConfig{
						Type:       shared.McpServerTypeSdk,
						Name:       "sdk-server",
						AlwaysLoad: true,
					},
					"sdk-deferred": &shared.McpSdkServerConfig{
						Type: shared.McpServerTypeSdk,
						Name: "sdk-deferred",
					},
				}
				options := &shared.Options{McpServers: mcpServers}
				return New(newTransportMockCLI(), options, "sdk-go")
			},
			validate: func(t *testing.T, transport *Transport) {
				t.Helper()
				if transport.mcpConfigFile == nil {
					t.Fatal("Expected MCP config file to be generated")
				}
				configData, err := os.ReadFile(transport.mcpConfigFile.Name())
				if err != nil {
					t.Fatalf("Failed to read MCP config file: %v", err)
				}
				var config map[string]interface{}
				if err := json.Unmarshal(configData, &config); err != nil {
					t.Fatalf("MCP config is not valid JSON: %v", err)
				}
				servers := config["mcpServers"].(map[string]interface{})

				always := servers["sdk-server"].(map[string]interface{})
				if always["alwaysLoad"] != true {
					t.Errorf("Expected alwaysLoad=true on sdk-server, got %v", always["alwaysLoad"])
				}
				if always["type"] != string(shared.McpServerTypeSdk) {
					t.Errorf("Expected type=%q, got %v", shared.McpServerTypeSdk, always["type"])
				}

				deferred := servers["sdk-deferred"].(map[string]interface{})
				if _, present := deferred["alwaysLoad"]; present {
					t.Errorf("Expected alwaysLoad to be omitted when false, got %v", deferred["alwaysLoad"])
				}
			},
		},
		{
			name: "mcp_config_added_to_extra_args",
			setup: func() *Transport {
				mcpServers := map[string]shared.McpServerConfig{
					"args-test": &shared.McpStdioServerConfig{
						Type:    shared.McpServerTypeStdio,
						Command: "test",
					},
				}
				options := &shared.Options{
					McpServers: mcpServers,
					ExtraArgs:  map[string]*string{"existing": stringPtr("value")},
				}
				return New(newTransportMockCLI(), options, "sdk-go")
			},
			validate: func(t *testing.T, transport *Transport) {
				t.Helper()
				// Verify the command includes --mcp-config flag
				if transport.cmd == nil {
					t.Fatal("Expected command to be initialized")
				}

				cmdStr := strings.Join(transport.cmd.Args, " ")
				if !strings.Contains(cmdStr, "--mcp-config") {
					t.Errorf("Expected command to contain --mcp-config flag, got: %s", cmdStr)
				}

				// Verify original ExtraArgs was not mutated
				if transport.options.ExtraArgs["mcp-config"] != nil {
					t.Error("Original options.ExtraArgs should not be mutated")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := tt.setup()
			// Note: cleanup is handled in the validate function for cleanup test
			if tt.name != "mcp_config_file_cleaned_up" {
				defer disconnectTransportSafely(t, transport)
			}

			// Connect to trigger MCP config generation
			err := transport.Connect(ctx)
			assertNoTransportError(t, err)

			// Run validation
			tt.validate(t, transport)
		})
	}
}

// assertEnvContains checks if environment slice contains a key=value pair
func assertEnvContains(t *testing.T, env []string, expected string) {
	t.Helper()
	for _, e := range env {
		if e == expected {
			return
		}
	}
	t.Errorf("Environment missing %s. Available: %v", expected, env)
}

func stringPtr(s string) *string {
	return &s
}
