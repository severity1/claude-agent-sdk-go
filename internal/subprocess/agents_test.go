package subprocess

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/severity1/claude-agent-sdk-go/internal/control"
	"github.com/severity1/claude-agent-sdk-go/internal/shared"
)

// TestAgentsToMapStripsEmptyFields pins the omit rule: description and
// prompt always emit, a nil slice is omitted, and an empty Model is omitted.
func TestAgentsToMapStripsEmptyFields(t *testing.T) {
	agents := map[string]shared.AgentDefinition{
		"full": {
			Description: "Full agent",
			Prompt:      "You are full.",
			Tools:       []string{"Read", "Grep"},
			Model:       shared.AgentModelSonnet,
		},
		"minimal": {
			Description: "Minimal agent",
			Prompt:      "You are minimal.",
		},
	}

	got := agentsToMap(agents)

	if len(got) != 2 {
		t.Fatalf("agentsToMap returned %d entries, want 2", len(got))
	}
	assertAgentJSON(t, got["full"], `{"description":"Full agent","model":"sonnet","prompt":"You are full.","tools":["Read","Grep"]}`)
	assertAgentJSON(t, got["minimal"], `{"description":"Minimal agent","prompt":"You are minimal."}`)
}

// TestAgentsToMapFields ports the serialization cases for every
// AgentDefinition field: camelCase keys, nil omitted, and empty or false
// values kept when set.
func TestAgentsToMapFields(t *testing.T) {
	var nilEffort *shared.AgentEffortLevel
	tests := []struct {
		name  string
		agent shared.AgentDefinition
		want  string
	}{
		{
			name:  "minimal_definition_omits_unset_fields",
			agent: shared.AgentDefinition{Description: "test", Prompt: "You are a test"},
			want:  `{"description":"test","prompt":"You are a test"}`,
		},
		{
			name:  "empty_tools_sends_empty_list",
			agent: shared.AgentDefinition{Description: "d", Prompt: "p", Tools: []string{}},
			want:  `{"description":"d","prompt":"p","tools":[]}`,
		},
		{
			name:  "skills_and_memory",
			agent: shared.AgentDefinition{Description: "d", Prompt: "p", Skills: []string{"skill-a", "skill-b"}, Memory: shared.AgentMemoryProject},
			want:  `{"description":"d","memory":"project","prompt":"p","skills":["skill-a","skill-b"]}`,
		},
		{
			name:  "empty_skills_sends_empty_list",
			agent: shared.AgentDefinition{Description: "d", Prompt: "p", Skills: []string{}},
			want:  `{"description":"d","prompt":"p","skills":[]}`,
		},
		{
			name:  "disallowed_tools_and_max_turns",
			agent: shared.AgentDefinition{Description: "d", Prompt: "p", DisallowedTools: []string{"Bash", "Write"}, MaxTurns: 10},
			want:  `{"description":"d","disallowedTools":["Bash","Write"],"maxTurns":10,"prompt":"p"}`,
		},
		{
			name:  "initial_prompt",
			agent: shared.AgentDefinition{Description: "d", Prompt: "p", InitialPrompt: "/review-pr 123"},
			want:  `{"description":"d","initialPrompt":"/review-pr 123","prompt":"p"}`,
		},
		{
			name:  "model_accepts_full_model_id",
			agent: shared.AgentDefinition{Description: "d", Prompt: "p", Model: "claude-opus-4-5"},
			want:  `{"description":"d","model":"claude-opus-4-5","prompt":"p"}`,
		},
		{
			name:  "background_true",
			agent: shared.AgentDefinition{Description: "d", Prompt: "p", Background: boolPtr(true)},
			want:  `{"background":true,"description":"d","prompt":"p"}`,
		},
		{
			name:  "background_false_is_sent",
			agent: shared.AgentDefinition{Description: "d", Prompt: "p", Background: boolPtr(false)},
			want:  `{"background":false,"description":"d","prompt":"p"}`,
		},
		{
			name:  "effort_named_level",
			agent: shared.AgentDefinition{Description: "d", Prompt: "p", Effort: shared.AgentEffortLevel(shared.EffortHigh)},
			want:  `{"description":"d","effort":"high","prompt":"p"}`,
		},
		{
			name:  "effort_xhigh_level",
			agent: shared.AgentDefinition{Description: "d", Prompt: "p", Effort: shared.AgentEffortLevel(shared.EffortXHigh)},
			want:  `{"description":"d","effort":"xhigh","prompt":"p"}`,
		},
		{
			name:  "effort_integer",
			agent: shared.AgentDefinition{Description: "d", Prompt: "p", Effort: shared.AgentEffortTokens(32000)},
			want:  `{"description":"d","effort":32000,"prompt":"p"}`,
		},
		{
			name:  "effort_typed_nil_pointer_omitted",
			agent: shared.AgentDefinition{Description: "d", Prompt: "p", Effort: nilEffort},
			want:  `{"description":"d","prompt":"p"}`,
		},
		{
			name:  "permission_mode",
			agent: shared.AgentDefinition{Description: "d", Prompt: "p", PermissionMode: shared.PermissionModeBypassPermissions},
			want:  `{"description":"d","permissionMode":"bypassPermissions","prompt":"p"}`,
		},
		{
			name: "mcp_servers_name_and_inline_config",
			agent: shared.AgentDefinition{Description: "d", Prompt: "p", McpServers: []shared.AgentMcpServer{
				{Name: "slack"},
				{Name: "local", Config: &shared.McpStdioServerConfig{Command: "python", Args: []string{"server.py"}}},
				{Name: "remote", Config: &shared.McpHTTPServerConfig{URL: "https://example.com/mcp"}},
			}},
			want: `{"description":"d","mcpServers":["slack",{"local":{"args":["server.py"],"command":"python","type":"stdio"}},{"remote":{"type":"http","url":"https://example.com/mcp"}}],"prompt":"p"}`,
		},
		{
			name:  "empty_mcp_servers_sends_empty_list",
			agent: shared.AgentDefinition{Description: "d", Prompt: "p", McpServers: []shared.AgentMcpServer{}},
			want:  `{"description":"d","mcpServers":[],"prompt":"p"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := agentsToMap(map[string]shared.AgentDefinition{"a": tt.agent})
			assertAgentJSON(t, got["a"], tt.want)
		})
	}
}

// TestInitializeRequestAgentsWireJSON pins the agents object exactly as the
// initialize control request sends it.
func TestInitializeRequestAgentsWireJSON(t *testing.T) {
	req := control.InitializeRequest{
		Subtype: control.SubtypeInitialize,
		Agents: agentsToMap(map[string]shared.AgentDefinition{
			"reviewer": {
				Description:     "Reviews code",
				Prompt:          "You are a reviewer.",
				Tools:           []string{},
				DisallowedTools: []string{"Bash"},
				MaxTurns:        5,
				Effort:          shared.AgentEffortLevel(shared.EffortLow),
			},
		}),
	}
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	want := `"agents":{"reviewer":{"description":"Reviews code","disallowedTools":["Bash"],"effort":"low","maxTurns":5,"prompt":"You are a reviewer.","tools":[]}}`
	if !strings.Contains(string(raw), want) {
		t.Errorf("initialize request = %s, want it to contain %s", raw, want)
	}
}

// TestBuildProtocolOptionsIncludesAgents pins the wiring: when Options.Agents
// is populated, buildProtocolOptions appends exactly one additional option
// (control.WithAgents) over the no-agents baseline. The end-to-end wire
// assertion lives in internal/control/initialize_agents_test.go; this unit
// test guards against accidental removal of the agents wiring in
// buildProtocolOptions even when other options are also present.
func TestBuildProtocolOptionsIncludesAgents(t *testing.T) {
	baseline := New("/usr/bin/claude", &shared.Options{}, "sdk-go").buildProtocolOptions()
	withAgents := New("/usr/bin/claude", &shared.Options{
		Agents: map[string]shared.AgentDefinition{
			"reviewer": {Description: "Reviews code", Prompt: "You are a reviewer."},
		},
	}, "sdk-go").buildProtocolOptions()

	if got, want := len(withAgents), len(baseline)+1; got != want {
		t.Errorf("buildProtocolOptions with agents = %d opts, baseline = %d; want exactly %d (baseline + WithAgents)",
			got, len(baseline), want)
	}
}

// TestBuildProtocolOptionsOmitsAgentsWhenEmpty verifies the inverse: when
// Options.Agents is nil/empty, no agents-specific option is emitted.
func TestBuildProtocolOptionsOmitsAgentsWhenEmpty(t *testing.T) {
	tests := []struct {
		name    string
		options *shared.Options
	}{
		{"nil_options", nil},
		{"nil_agents", &shared.Options{}},
		{"empty_agents", &shared.Options{Agents: map[string]shared.AgentDefinition{}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := New("/usr/bin/claude", tt.options, "sdk-go")
			opts := transport.buildProtocolOptions()
			if len(opts) != 0 {
				t.Errorf("expected no protocol options without agents, got %d", len(opts))
			}
		})
	}
}

// TestBuildProtocolOptionsSkills pins Python query.py: only a []string
// (also empty) adds the skills option; SkillsAll and nil add nothing.
func TestBuildProtocolOptionsSkills(t *testing.T) {
	tests := []struct {
		name    string
		skills  any
		wantOpt bool
	}{
		{"nil_skills", nil, false},
		{"skills_all", shared.SkillsAll, false},
		{"skills_list", []string{"pdf"}, true},
		{"skills_disabled", []string{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := New("/usr/bin/claude", &shared.Options{Skills: tt.skills}, "sdk-go").buildProtocolOptions()
			if got := len(opts) == 1; got != tt.wantOpt {
				t.Errorf("skills option present = %v (%d opts), want %v", got, len(opts), tt.wantOpt)
			}
		})
	}
}

func assertAgentJSON(t *testing.T, entry any, want string) {
	t.Helper()
	raw, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal agent entry: %v", err)
	}
	if string(raw) != want {
		t.Errorf("agent entry = %s, want %s", raw, want)
	}
}

func boolPtr(b bool) *bool { return &b }
