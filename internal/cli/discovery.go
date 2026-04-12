// Package cli provides CLI discovery and command building functionality.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/severity1/claude-agent-sdk-go/internal/shared"
)

const windowsOS = "windows"

// MinimumCLIVersion is the minimum supported Claude Code CLI version.
// Features may not work correctly with older versions.
const MinimumCLIVersion = "2.0.76"

// versionRegex matches semantic version X.Y.Z (mimics Python SDK regex).
var versionRegex = regexp.MustCompile(`([0-9]+\.[0-9]+\.[0-9]+)`)

// DiscoveryPaths defines the standard search paths for Claude CLI.
var DiscoveryPaths = []string{
	// Will be populated with dynamic paths in FindCLI()
}

// FindCLI searches for the Claude CLI binary in standard locations.
func FindCLI() (string, error) {
	// 1. Check PATH first - most common case
	if path, err := exec.LookPath("claude"); err == nil {
		return path, nil
	}

	// 2. Check platform-specific common locations
	locations := getCommonCLILocations()

	for _, location := range locations {
		if info, err := os.Stat(location); err == nil && !info.IsDir() {
			// Verify it's executable (Unix-like systems)
			if runtime.GOOS != windowsOS {
				if info.Mode()&0o111 == 0 {
					continue // Not executable
				}
			}
			return location, nil
		}
	}

	// 3. Check Node.js dependency
	if _, err := exec.LookPath("node"); err != nil {
		return "", shared.NewCLINotFoundError("",
			"Claude Code requires Node.js, which is not installed.\n\n"+
				"Install Node.js from: https://nodejs.org/\n\n"+
				"After installing Node.js, install Claude Code:\n"+
				"  npm install -g @anthropic-ai/claude-code")
	}

	// 4. Provide installation guidance
	return "", shared.NewCLINotFoundError("",
		"Claude Code not found. Install with:\n"+
			"  npm install -g @anthropic-ai/claude-code\n\n"+
			"If already installed locally, try:\n"+
			`  export PATH="$HOME/node_modules/.bin:$PATH"`+"\n\n"+
			"Or specify the path when creating client")
}

// getCommonCLILocations returns platform-specific CLI search locations
func getCommonCLILocations() []string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		// Fallback to current directory if home directory can't be determined
		homeDir = "."
	}

	var locations []string

	switch runtime.GOOS {
	case windowsOS:
		locations = []string{
			filepath.Join(homeDir, "AppData", "Roaming", "npm", "claude.cmd"),
			filepath.Join("C:", "Program Files", "nodejs", "claude.cmd"),
			filepath.Join(homeDir, ".npm-global", "claude.cmd"),
			filepath.Join(homeDir, "node_modules", ".bin", "claude.cmd"),
		}
	default: // Unix-like systems
		locations = []string{
			filepath.Join(homeDir, ".npm-global", "bin", "claude"),
			"/usr/local/bin/claude",
			filepath.Join(homeDir, ".local", "bin", "claude"),
			filepath.Join(homeDir, "node_modules", ".bin", "claude"),
			filepath.Join(homeDir, ".yarn", "bin", "claude"),
			"/opt/homebrew/bin/claude",       // macOS Homebrew ARM
			"/usr/local/homebrew/bin/claude", // macOS Homebrew Intel
		}
	}

	return locations
}

// BuildCommand constructs the CLI command with all necessary flags.
func BuildCommand(cliPath string, options *shared.Options, closeStdin bool) []string {
	cmd := []string{cliPath}

	// Base arguments - always include these
	cmd = append(cmd, "--output-format", "stream-json", "--verbose")

	// Input mode configuration
	if closeStdin {
		// One-shot mode (Query function)
		cmd = append(cmd, "--print")
	} else {
		// Streaming mode (Client interface)
		cmd = append(cmd, "--input-format", "stream-json")
	}

	// Add all configuration options as CLI flags
	if options != nil {
		cmd = addOptionsToCommand(cmd, options)
	}

	return cmd
}

// BuildCommandWithPrompt constructs the CLI command for one-shot queries with prompt as argument.
func BuildCommandWithPrompt(cliPath string, options *shared.Options, prompt string) []string {
	cmd := []string{cliPath}

	// Base arguments - always include these
	cmd = append(cmd, "--output-format", "stream-json", "--verbose")

	// Add all configuration options as CLI flags before the prompt
	if options != nil {
		cmd = addOptionsToCommand(cmd, options)
	}

	// Prompt must be last so the CLI parses all flags (e.g. --mcp-config) correctly
	cmd = append(cmd, "--print", prompt)

	return cmd
}

// addOptionsToCommand adds all Options fields as CLI flags
func addOptionsToCommand(cmd []string, options *shared.Options) []string {
	cmd = addToolControlFlags(cmd, options)
	cmd = addToolsFlag(cmd, options)
	cmd = addModelAndPromptFlags(cmd, options)
	cmd = addPermissionFlags(cmd, options)
	cmd = addSessionFlags(cmd, options)
	cmd = addAgentFlags(cmd, options)
	cmd = addFileSystemFlags(cmd, options)
	cmd = addMCPFlags(cmd, options)
	cmd = addPluginsFlag(cmd, options)
	cmd = addBetasFlag(cmd, options)
	cmd = addSandboxFlags(cmd, options)
	cmd = addOutputFormatFlags(cmd, options)
	cmd = addExtraFlags(cmd, options)
	return cmd
}

func addToolControlFlags(cmd []string, options *shared.Options) []string {
	if len(options.AllowedTools) > 0 {
		cmd = append(cmd, "--allowed-tools", strings.Join(options.AllowedTools, ","))
	}
	if len(options.DisallowedTools) > 0 {
		cmd = append(cmd, "--disallowed-tools", strings.Join(options.DisallowedTools, ","))
	}
	return cmd
}

func addToolsFlag(cmd []string, options *shared.Options) []string {
	if options.Tools == nil {
		return cmd
	}

	switch v := options.Tools.(type) {
	case []string:
		// Pass --tools "" for explicitly empty list (disables all tools)
		// vs nil which means "use default tools"
		cmd = append(cmd, "--tools", strings.Join(v, ","))
	case shared.ToolsPreset:
		// Serialize as JSON for preset
		data, err := json.Marshal(v)
		if err == nil {
			cmd = append(cmd, "--tools", string(data))
		}
	}
	return cmd
}

func addModelAndPromptFlags(cmd []string, options *shared.Options) []string {
	if options.SystemPrompt != nil {
		cmd = append(cmd, "--system-prompt", *options.SystemPrompt)
	}
	if options.AppendSystemPrompt != nil {
		cmd = append(cmd, "--append-system-prompt", *options.AppendSystemPrompt)
	}
	if options.Model != nil {
		cmd = append(cmd, "--model", *options.Model)
	}
	if options.FallbackModel != nil {
		cmd = append(cmd, "--fallback-model", *options.FallbackModel)
	}
	if options.MaxBudgetUSD != nil {
		cmd = append(cmd, "--max-budget-usd", fmt.Sprintf("%.2f", *options.MaxBudgetUSD))
	}
	// NOTE: --max-thinking-tokens not supported by current CLI version
	// if options.MaxThinkingTokens > 0 {
	//	cmd = append(cmd, "--max-thinking-tokens", fmt.Sprintf("%d", options.MaxThinkingTokens))
	// }
	// NOTE: User and MaxBufferSize are internal SDK options without CLI flag mappings
	return cmd
}

func addPermissionFlags(cmd []string, options *shared.Options) []string {
	if options.PermissionMode != nil {
		cmd = append(cmd, "--permission-mode", string(*options.PermissionMode))
	}
	if options.PermissionPromptToolName != nil {
		cmd = append(cmd, "--permission-prompt-tool", *options.PermissionPromptToolName)
	}
	return cmd
}

func addSessionFlags(cmd []string, options *shared.Options) []string {
	if options.ContinueConversation {
		cmd = append(cmd, "--continue")
	}
	if options.Resume != nil {
		cmd = append(cmd, "--resume", *options.Resume)
	}
	if options.MaxTurns > 0 {
		cmd = append(cmd, "--max-turns", fmt.Sprintf("%d", options.MaxTurns))
	}
	// Only add --settings here if Sandbox is nil
	// When Sandbox is set, addSandboxFlags() handles merging both into one --settings flag
	if options.Settings != nil && options.Sandbox == nil {
		cmd = append(cmd, "--settings", *options.Settings)
	}
	if options.ForkSession {
		cmd = append(cmd, "--fork-session")
	}
	// Always pass --setting-sources (Python SDK parity)
	// Empty slice results in empty string value
	sourcesValue := ""
	if len(options.SettingSources) > 0 {
		strs := make([]string, len(options.SettingSources))
		for i, s := range options.SettingSources {
			strs[i] = string(s)
		}
		sourcesValue = strings.Join(strs, ",")
	}
	cmd = append(cmd, "--setting-sources", sourcesValue)
	if options.IncludePartialMessages {
		cmd = append(cmd, "--include-partial-messages")
	}
	return cmd
}

func addAgentFlags(cmd []string, options *shared.Options) []string {
	if len(options.Agents) == 0 {
		return cmd
	}

	// Convert to map[string]map[string]any, filtering nil/empty fields
	// This matches Python SDK behavior of omitting None values
	agentsMap := make(map[string]map[string]any)
	for name, agent := range options.Agents {
		agentMap := map[string]any{
			"description": agent.Description,
			"prompt":      agent.Prompt,
		}
		if len(agent.Tools) > 0 {
			agentMap["tools"] = agent.Tools
		}
		if agent.Model != "" {
			agentMap["model"] = string(agent.Model)
		}
		agentsMap[name] = agentMap
	}

	data, err := json.Marshal(agentsMap)
	if err != nil {
		return cmd // Skip on serialization error
	}

	return append(cmd, "--agents", string(data))
}

func addFileSystemFlags(cmd []string, options *shared.Options) []string {
	// Note: Working directory is set via exec.Cmd.Dir in transport layer, not as a CLI flag
	for _, dir := range options.AddDirs {
		cmd = append(cmd, "--add-dir", dir)
	}
	return cmd
}

func addMCPFlags(cmd []string, _ *shared.Options) []string {
	// Note: MCP server configuration is handled by the Transport layer.
	// When options.McpServers is set, Transport generates a temporary config file
	// and adds it to ExtraArgs as "--mcp-config", which is then added by addExtraFlags().
	// This function is kept for potential future direct MCP flag support.
	return cmd
}

func addBetasFlag(cmd []string, options *shared.Options) []string {
	if len(options.Betas) > 0 {
		betaStrs := make([]string, len(options.Betas))
		for i, beta := range options.Betas {
			betaStrs[i] = string(beta)
		}
		cmd = append(cmd, "--betas", strings.Join(betaStrs, ","))
	}
	return cmd
}

func addPluginsFlag(cmd []string, options *shared.Options) []string {
	for _, plugin := range options.Plugins {
		if plugin.Type == shared.SdkPluginTypeLocal {
			cmd = append(cmd, "--plugin-dir", plugin.Path)
		}
		// Note: Future plugin types would be handled here
	}
	return cmd
}

func addSandboxFlags(cmd []string, options *shared.Options) []string {
	if options.Sandbox == nil {
		return cmd
	}

	// Start with existing settings if present, otherwise create empty map
	var settingsMap map[string]interface{}
	if options.Settings != nil {
		if err := json.Unmarshal([]byte(*options.Settings), &settingsMap); err != nil {
			// If existing settings are invalid JSON, start fresh
			settingsMap = make(map[string]interface{})
		}
	} else {
		settingsMap = make(map[string]interface{})
	}

	// Add sandbox to merged settings
	settingsMap["sandbox"] = options.Sandbox

	data, err := json.Marshal(settingsMap)
	if err != nil {
		// This should never happen with our simple types
		// If it does, skip sandbox settings (but existing settings are also skipped in this case)
		return cmd
	}

	cmd = append(cmd, "--settings", string(data))
	return cmd
}

func addOutputFormatFlags(cmd []string, options *shared.Options) []string {
	if options.OutputFormat == nil || options.OutputFormat.Schema == nil {
		return cmd
	}

	// Serialize schema to JSON for CLI flag
	schemaData, err := json.Marshal(options.OutputFormat.Schema)
	if err != nil {
		// Silently skip on marshal error (shouldn't happen with valid schemas)
		return cmd
	}

	return append(cmd, "--json-schema", string(schemaData))
}

func addExtraFlags(cmd []string, options *shared.Options) []string {
	for flag, value := range options.ExtraArgs {
		if value == nil {
			// Boolean flag
			cmd = append(cmd, "--"+flag)
		} else {
			// Flag with value
			cmd = append(cmd, "--"+flag, *value)
		}
	}
	return cmd
}

// ValidateNodeJS checks if Node.js is available.
func ValidateNodeJS() error {
	if _, err := exec.LookPath("node"); err != nil {
		return shared.NewCLINotFoundError("node",
			"Node.js is required for Claude CLI but was not found.\n\n"+
				"Install Node.js from: https://nodejs.org/\n\n"+
				"After installing Node.js, install Claude Code:\n"+
				"  npm install -g @anthropic-ai/claude-code")
	}
	return nil
}

// ValidateWorkingDirectory checks if the working directory exists and is valid.
func ValidateWorkingDirectory(cwd string) error {
	if cwd == "" {
		return nil // No validation needed if no cwd specified
	}

	info, err := os.Stat(cwd)
	if os.IsNotExist(err) {
		return shared.NewConnectionError(
			fmt.Sprintf("working directory does not exist: %s", cwd),
			err,
		)
	}
	if err != nil {
		return fmt.Errorf("failed to check working directory: %w", err)
	}

	if !info.IsDir() {
		return shared.NewConnectionError(
			fmt.Sprintf("working directory path is not a directory: %s", cwd),
			nil,
		)
	}

	return nil
}

// CheckCLIVersion checks if CLI version is below minimum and returns a warning.
// Mimics Python SDK _check_claude_version() behavior.
// Non-blocking - errors are silently ignored.
func CheckCLIVersion(ctx context.Context, cliPath string) (warning string) {
	if os.Getenv("CLAUDE_AGENT_SDK_SKIP_VERSION_CHECK") != "" {
		return ""
	}

	// 2-second timeout (matches Python SDK)
	checkCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	// Run CLI with -v flag (matches Python SDK)
	cmd := exec.CommandContext(checkCtx, cliPath, "-v")
	output, err := cmd.Output()
	if err != nil {
		return "" // Silently ignore errors (matches Python SDK)
	}

	versionOutput := strings.TrimSpace(string(output))

	// Extract version with regex (matches Python SDK)
	match := versionRegex.FindStringSubmatch(versionOutput)
	if len(match) < 2 {
		return "" // No valid version found
	}
	version := match[1]

	// Compare version parts (matches Python SDK list comparison)
	if compareVersionParts(version, MinimumCLIVersion) < 0 {
		return fmt.Sprintf(
			"Warning: Claude Code version %s is unsupported in the Agent SDK. "+
				"Minimum required version is %s. "+
				"Some features may not work correctly.",
			version, MinimumCLIVersion,
		)
	}

	return ""
}

// compareVersionParts compares two X.Y.Z versions.
// Returns -1 if v1 < v2, 0 if equal, 1 if v1 > v2.
// Mimics Python SDK: [int(x) for x in version.split(".")] comparison.
func compareVersionParts(v1, v2 string) int {
	p1 := strings.Split(v1, ".")
	p2 := strings.Split(v2, ".")

	for i := 0; i < 3; i++ {
		n1, n2 := 0, 0
		if i < len(p1) {
			n1, _ = strconv.Atoi(p1[i])
		}
		if i < len(p2) {
			n2, _ = strconv.Atoi(p2[i])
		}
		if n1 < n2 {
			return -1
		}
		if n1 > n2 {
			return 1
		}
	}
	return 0
}
