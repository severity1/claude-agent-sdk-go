package shared

import (
	"strings"
	"testing"
)

func TestValidateAgents(t *testing.T) {
	var nilEffort *AgentEffortTokens
	tests := []struct {
		name    string
		agents  map[string]AgentDefinition
		wantErr string
	}{
		{name: "nil_map", agents: nil},
		{name: "minimal_agent", agents: map[string]AgentDefinition{"a": {Description: "d", Prompt: "p"}}},
		{
			name: "all_fields_valid",
			agents: map[string]AgentDefinition{"a": {
				Description: "d",
				Prompt:      "p",
				McpServers: []AgentMcpServer{
					{Name: "slack"},
					{Name: "local", Config: &McpStdioServerConfig{Command: "python"}},
				},
				MaxTurns: 3,
				Effort:   AgentEffortTokens(32000),
			}},
		},
		{name: "typed_nil_effort", agents: map[string]AgentDefinition{"a": {Effort: nilEffort}}},
		{
			name:    "empty_mcp_server_name",
			agents:  map[string]AgentDefinition{"reviewer": {McpServers: []AgentMcpServer{{Name: ""}}}},
			wantErr: `agent "reviewer": mcpServers[0] has an empty name`,
		},
		{
			name: "inline_sdk_server",
			agents: map[string]AgentDefinition{"reviewer": {McpServers: []AgentMcpServer{
				{Name: "calc", Config: &McpSdkServerConfig{Type: McpServerTypeSdk, Name: "calc"}},
			}}},
			wantErr: `agent "reviewer": mcpServers[0] "calc" is an SDK server`,
		},
		{
			name: "typed_nil_inline_config",
			agents: map[string]AgentDefinition{"reviewer": {McpServers: []AgentMcpServer{
				{Name: "local", Config: (*McpStdioServerConfig)(nil)},
			}}},
			wantErr: `agent "reviewer": mcpServers[0] "local" has a config that is not a JSON object`,
		},
		{
			name:    "negative_max_turns",
			agents:  map[string]AgentDefinition{"reviewer": {MaxTurns: -1}},
			wantErr: `agent "reviewer": maxTurns must be non-negative, got -1`,
		},
		{
			name:    "negative_effort_tokens",
			agents:  map[string]AgentDefinition{"reviewer": {Effort: AgentEffortTokens(-5)}},
			wantErr: `agent "reviewer": effort tokens must be non-negative, got -5`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAgents(tt.agents)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("ValidateAgents() = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("ValidateAgents() = %v, want error containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestAgentEffortValue(t *testing.T) {
	var nilLevel *AgentEffortLevel
	tests := []struct {
		name   string
		effort AgentEffort
		want   any
	}{
		{"nil", nil, nil},
		{"typed_nil_pointer", nilLevel, nil},
		{"level", AgentEffortLevel(EffortHigh), "high"},
		{"xhigh_level", AgentEffortLevel(EffortXHigh), "xhigh"},
		{"unknown_level_passes_through", AgentEffortLevel("extreme"), "extreme"},
		{"empty_level_is_unset", AgentEffortLevel(""), nil},
		{"tokens", AgentEffortTokens(32000), 32000},
		{"zero_tokens", AgentEffortTokens(0), 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AgentEffortValue(tt.effort); got != tt.want {
				t.Errorf("AgentEffortValue() = %#v, want %#v", got, tt.want)
			}
		})
	}
}
