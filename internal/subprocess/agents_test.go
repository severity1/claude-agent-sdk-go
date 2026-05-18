package subprocess

import (
	"testing"

	"github.com/severity1/claude-agent-sdk-go/internal/shared"
)

// TestAgentsToMapStripsEmptyFields verifies the Go-side stripping rule:
// description/prompt always emit; nil/empty Tools and empty Model are
// dropped from the per-agent map sent in the initialize request. (Note:
// this is stricter than Python's `if v is not None` rule, which would
// preserve empty list and empty string. See agentsToMap docstring.)
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

	full, ok := got["full"].(map[string]any)
	if !ok {
		t.Fatalf("`full` agent entry type %T, want map[string]any", got["full"])
	}
	if full["description"] != "Full agent" || full["prompt"] != "You are full." {
		t.Errorf("full agent description/prompt wrong: %+v", full)
	}
	if _, ok := full["tools"]; !ok {
		t.Error("full agent should retain tools")
	}
	if model, ok := full["model"].(string); !ok || model != "sonnet" {
		t.Errorf("full agent model = %v, want sonnet", full["model"])
	}

	minimal, ok := got["minimal"].(map[string]any)
	if !ok {
		t.Fatalf("`minimal` agent entry type %T, want map[string]any", got["minimal"])
	}
	if _, ok := minimal["tools"]; ok {
		t.Error("minimal agent should omit tools when empty")
	}
	if _, ok := minimal["model"]; ok {
		t.Error("minimal agent should omit model when empty")
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
