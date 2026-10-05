package shared

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
)

// AgentDefinition defines a programmatic subagent. A nil slice is not sent;
// a non-nil empty slice is sent as []. The JSON tags show the wire keys; the
// transport builds the wire map itself.
type AgentDefinition struct {
	// Description is a brief description of the agent's purpose.
	Description string `json:"description"`

	// Prompt is the agent's system prompt.
	Prompt string `json:"prompt"`

	// Tools lists the tools available to the agent. Nil inherits the
	// default tools; an empty slice gives the agent no tools.
	Tools []string `json:"tools,omitempty"`

	// DisallowedTools lists the tools the agent cannot use.
	DisallowedTools []string `json:"disallowedTools,omitempty"`

	// Model is a model alias (AgentModelSonnet, ...) or a full model ID.
	Model AgentModel `json:"model,omitempty"`

	// Skills lists the skills available to the agent.
	Skills []string `json:"skills,omitempty"`

	// Memory selects the memory scope of the agent; "" means unset.
	Memory AgentMemory `json:"memory,omitempty"`

	// McpServers lists the MCP servers of the agent, by name or inline.
	McpServers []AgentMcpServer `json:"-"`

	// InitialPrompt is sent as the first user turn of the agent.
	InitialPrompt string `json:"initialPrompt,omitempty"`

	// MaxTurns limits the turns of the agent; 0 means unset.
	MaxTurns int `json:"maxTurns,omitempty"`

	// Background runs the agent in the background; nil means unset.
	Background *bool `json:"background,omitempty"`

	// Effort is an AgentEffortLevel or AgentEffortTokens; nil means unset.
	Effort AgentEffort `json:"-"`

	// PermissionMode sets the permission mode of the agent; "" means unset.
	// The CLI validates the value, so modes this SDK does not list still pass.
	PermissionMode PermissionMode `json:"permissionMode,omitempty"`
}

// AgentMemory selects the memory scope of an agent.
type AgentMemory string

const (
	// AgentMemoryUser keeps agent memory in the user scope.
	AgentMemoryUser AgentMemory = "user"
	// AgentMemoryProject keeps agent memory in the project scope.
	AgentMemoryProject AgentMemory = "project"
	// AgentMemoryLocal keeps agent memory in the local scope.
	AgentMemoryLocal AgentMemory = "local"
)

// AgentMcpServer names an MCP server for an agent. A nil Config refers to a
// server by name; a non-nil Config defines the server inline.
type AgentMcpServer struct {
	Name   string
	Config McpServerConfig
}

// AgentEffort is the effort of an agent. The implementations are
// AgentEffortLevel and AgentEffortTokens.
type AgentEffort interface {
	agentEffortValue() any
}

// AgentEffortLevel sets the effort of an agent as a level. The CLI validates
// the value, so levels this SDK does not list still pass.
type AgentEffortLevel EffortLevel

// AgentEffortTokens sets the effort of an agent as a token count.
type AgentEffortTokens int

// An empty level is unset, like the other string fields of AgentDefinition.
func (e AgentEffortLevel) agentEffortValue() any {
	if e == "" {
		return nil
	}
	return string(e)
}

func (e AgentEffortTokens) agentEffortValue() any { return int(e) }

// AgentEffortValue returns the wire value of effort, or nil when effort is
// nil, a typed nil pointer or an empty level.
func AgentEffortValue(effort AgentEffort) any {
	if effort == nil {
		return nil
	}
	if v := reflect.ValueOf(effort); v.Kind() == reflect.Ptr && v.IsNil() {
		return nil
	}
	return effort.agentEffortValue()
}

// ValidateAgents returns an error for an agent definition that the SDK
// cannot send to the CLI.
func ValidateAgents(agents map[string]AgentDefinition) error {
	names := make([]string, 0, len(agents))
	for name := range agents {
		names = append(names, name)
	}
	// Sorted so that the same options always give the same error.
	sort.Strings(names)
	for _, name := range names {
		if err := validateAgent(agents[name]); err != nil {
			return fmt.Errorf("agent %q: %w", name, err)
		}
	}
	return nil
}

func validateAgent(agent AgentDefinition) error {
	if agent.MaxTurns < 0 {
		return fmt.Errorf("maxTurns must be non-negative, got %d", agent.MaxTurns)
	}
	if tokens, ok := AgentEffortValue(agent.Effort).(int); ok && tokens < 0 {
		return fmt.Errorf("effort tokens must be non-negative, got %d", tokens)
	}
	for i, server := range agent.McpServers {
		if server.Name == "" {
			return fmt.Errorf("mcpServers[%d] has an empty name", i)
		}
		// The CLI cannot run the in-process Instance, so an SDK server
		// must be set in Options.McpServers and named here.
		if _, ok := server.Config.(*McpSdkServerConfig); ok {
			return fmt.Errorf("mcpServers[%d] %q is an SDK server: add it to McpServers and refer to it by name", i, server.Name)
		}
		if server.Config != nil && !isJSONObject(server.Config) {
			return fmt.Errorf("mcpServers[%d] %q has a config that is not a JSON object", i, server.Name)
		}
	}
	return nil
}

func isJSONObject(v any) bool {
	raw, err := json.Marshal(v)
	if err != nil {
		return false
	}
	var m map[string]any
	return json.Unmarshal(raw, &m) == nil && m != nil
}
