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
	path, lastResort := findOnPath(runtime.GOOS, exec.LookPath)
	if path != "" {
		return path, nil
	}

	// 2. Check platform-specific common locations
	for _, location := range getCommonCLILocations() {
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

	// A shim found on PATH goes to Connect, which explains why it refuses to run it.
	if lastResort != "" {
		return lastResort, nil
	}

	// npm's Windows install is a claude.cmd shim, which Connect refuses, so do not recommend it.
	if runtime.GOOS == windowsOS {
		return "", shared.NewCLINotFoundError("",
			"Claude Code not found. Install the native claude.exe with (PowerShell):\n"+
				"  irm https://claude.ai/install.ps1 | iex\n\n"+
				"Or specify the path to a claude.exe with WithCLIPath.\n\n"+
				"(npm install -g @anthropic-ai/claude-code produces a claude.cmd shim, "+
				"which this SDK refuses to run on Windows.)")
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

// findOnPath returns the CLI to use from PATH, or on Windows a non-native hit to keep as a last resort.
// A shim in an early PATH directory can shadow a native claude.exe in a later one (Python _find_cli).
func findOnPath(goos string, lookPath func(string) (string, error)) (use, lastResort string) {
	hit, err := lookPath("claude")
	if err != nil {
		return "", ""
	}
	if goos != windowsOS || isWindowsNativeExe(hit) {
		return hit, ""
	}
	// PATHEXT can turn the claude.exe probe into "claude.exe.cmd", so check it too.
	if exe, err := lookPath("claude.exe"); err == nil && isWindowsNativeExe(exe) {
		return exe, ""
	}
	return "", hit
}

// isWindowsNativeExe reports whether the final path component names a .exe or .com image.
// It only picks a discovery result; RejectWindowsBatchCLI is the security check.
func isWindowsNativeExe(path string) bool {
	components := strings.Split(strings.ReplaceAll(path, `\`, "/"), "/")
	name := strings.ToLower(strings.TrimRight(components[len(components)-1], ". "))
	return strings.HasSuffix(name, ".exe") || strings.HasSuffix(name, ".com")
}

// isWindowsBatchPath reports whether any path component names a .bat or .cmd file.
// Plain string logic (no filepath) so it gives the same result on every OS. Every component is
// checked because Win32 path normalization ("..", trailing dots, stream specs) can make any of
// them the file that runs; no real claude.exe lives under a directory named like a batch file.
func isWindowsBatchPath(path string) bool {
	for _, component := range strings.Split(strings.ReplaceAll(path, `\`, "/"), "/") {
		for _, segment := range strings.Split(component, ":") {
			name := strings.ToLower(strings.TrimRight(segment, ". "))
			if strings.HasSuffix(name, ".bat") || strings.HasSuffix(name, ".cmd") {
				return true
			}
		}
	}
	return false
}

// RejectWindowsBatchCLI refuses a .bat or .cmd CLI path on Windows (Python #1127).
// Windows runs a batch file through cmd.exe, which re-parses the arguments, and no
// reliable cmd.exe escaping exists (CVE-2024-27980, "BatBadBut").
func RejectWindowsBatchCLI(goos, path string) error {
	if goos != windowsOS || !isWindowsBatchPath(path) {
		return nil
	}
	return shared.NewConnectionError(fmt.Sprintf(
		"refusing to execute batch script %s: Windows runs .bat/.cmd files via cmd.exe, "+
			"which can execute commands injected through CLI arguments, and no reliable escaping "+
			"for cmd.exe exists. Use a native claude executable instead: install Claude Code "+
			"natively (irm https://claude.ai/install.ps1 | iex), or point WithCLIPath at a claude.exe", path), nil)
}

// getCommonCLILocations returns platform-specific CLI search locations
func getCommonCLILocations() []string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		// Fallback to current directory if home directory can't be determined
		homeDir = "."
	}
	return commonCLILocations(runtime.GOOS, homeDir)
}

// commonCLILocations returns the CLI search locations for goos.
func commonCLILocations(goos, homeDir string) []string {
	if goos == windowsOS {
		// Only the native installer's claude.exe: npm's claude.cmd shim is refused at Connect.
		return []string{filepath.Join(homeDir, ".local", "bin", "claude.exe")}
	}
	return []string{
		filepath.Join(homeDir, ".npm-global", "bin", "claude"),
		"/usr/local/bin/claude",
		filepath.Join(homeDir, ".local", "bin", "claude"),
		filepath.Join(homeDir, "node_modules", ".bin", "claude"),
		filepath.Join(homeDir, ".yarn", "bin", "claude"),
		"/opt/homebrew/bin/claude",       // macOS Homebrew ARM
		"/usr/local/homebrew/bin/claude", // macOS Homebrew Intel
	}
}

// BuildCommand constructs the CLI command with all necessary flags.
// Always uses streaming mode (--input-format stream-json); prompts are
// written to stdin after the initialize handshake instead of via --print.
func BuildCommand(cliPath string, options *shared.Options) []string {
	cmd := []string{cliPath}

	cmd = append(cmd, "--output-format", "stream-json", "--verbose")
	cmd = append(cmd, "--input-format", "stream-json")

	if options != nil {
		cmd = addOptionsToCommand(cmd, options)
	}

	return cmd
}

// addOptionsToCommand adds all Options fields as CLI flags
func addOptionsToCommand(cmd []string, options *shared.Options) []string {
	// Apply Skills option by transforming AllowedTools and SettingSources before
	// any flags are emitted. Matches the Python SDK's _apply_skills_defaults.
	if options.Skills != nil {
		copied := *options
		copied.AllowedTools, copied.SettingSources = applySkillsDefaults(options)
		options = &copied
	}

	cmd = addToolControlFlags(cmd, options)
	cmd = addToolsFlag(cmd, options)
	cmd = addModelAndPromptFlags(cmd, options)
	cmd = addPermissionFlags(cmd, options)
	cmd = addSessionFlags(cmd, options)
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
	if options.Effort != nil {
		cmd = append(cmd, "--effort", *options.Effort)
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
		// One token: the CLI's --resume takes an optional value, so a dash-leading value would parse as a flag.
		cmd = append(cmd, "--resume="+*options.Resume)
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

// applySkillsDefaults computes the effective AllowedTools and SettingSources for
// the Skills option without mutating the input. When Skills is "all", appends the
// bare "Skill" tool; when it is a []string, appends "Skill(name)" for each entry.
// When Skills is non-nil and SettingSources is unset, defaults SettingSources to
// [user, project] so the CLI discovers installed Skills. Mirrors the Python SDK's
// _apply_skills_defaults in subprocess_cli.py.
func applySkillsDefaults(options *shared.Options) ([]string, []shared.SettingSource) {
	allowedTools := append([]string(nil), options.AllowedTools...)
	settingSources := options.SettingSources

	switch s := options.Skills.(type) {
	case nil:
		return allowedTools, settingSources
	case string:
		if s == shared.SkillsAll && !containsString(allowedTools, "Skill") {
			allowedTools = append(allowedTools, "Skill")
		}
	case []string:
		for _, name := range s {
			pattern := fmt.Sprintf("Skill(%s)", name)
			if !containsString(allowedTools, pattern) {
				allowedTools = append(allowedTools, pattern)
			}
		}
	}

	if settingSources == nil {
		settingSources = []shared.SettingSource{
			shared.SettingSourceUser,
			shared.SettingSourceProject,
		}
	}
	return allowedTools, settingSources
}

func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
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
