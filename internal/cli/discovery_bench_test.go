package cli

import (
	"testing"

	"github.com/severity1/claude-agent-sdk-go/internal/shared"
)

// sink prevents dead code elimination by the compiler.
var sink any

// BenchmarkFindCLI measures CLI binary discovery performance.
// Note: This benchmark involves filesystem operations (exec.LookPath, os.Stat).
func BenchmarkFindCLI(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		result, err := FindCLI()
		sink = result
		sink = err
	}
}

// BenchmarkBuildCommand measures command construction performance.
func BenchmarkBuildCommand(b *testing.B) {
	cliPath := "/usr/bin/claude"
	model := "claude-sonnet-4-5"
	prompt := "You are a helpful assistant"
	cwd := "/home/user/project"

	tests := []struct {
		name    string
		options *shared.Options
	}{
		{
			name:    "nil_options",
			options: nil,
		},
		{
			name:    "empty_options",
			options: &shared.Options{},
		},
		{
			name: "minimal",
			options: &shared.Options{
				Model: &model,
			},
		},
		{
			name: "with_tools",
			options: &shared.Options{
				Model:        &model,
				AllowedTools: []string{"Read", "Write", "Bash", "Glob", "Grep"},
			},
		},
		{
			name: "full",
			options: &shared.Options{
				Model:             &model,
				SystemPrompt:      &prompt,
				Cwd:               &cwd,
				AllowedTools:      []string{"Read", "Write", "Bash"},
				DisallowedTools:   []string{"Edit"},
				MaxThinkingTokens: 16000,
				McpServers: map[string]shared.McpServerConfig{
					"aws": &shared.McpStdioServerConfig{
						Type:    shared.McpServerTypeStdio,
						Command: "aws-mcp",
						Args:    []string{"--profile", "default"},
					},
				},
			},
		},
	}

	for _, tc := range tests {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				result := BuildCommand(cliPath, tc.options)
				sink = result
			}
		})
	}
}

// BenchmarkGetCommonCLILocations measures platform-specific location discovery.
func BenchmarkGetCommonCLILocations(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		result := getCommonCLILocations()
		sink = result
	}
}
