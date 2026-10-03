package control

import (
	"encoding/json"
	"testing"
	"time"
)

// TestInitializeRequestIncludesAgents pins the wire shape: when WithAgents is
// configured, the initialize request carries an `agents` field; otherwise
// the key is omitted (omitempty).
func TestInitializeRequestIncludesAgents(t *testing.T) {
	tests := []struct {
		name       string
		agents     map[string]any
		wantAgents bool
	}{
		{
			name:       "nil_agents_omits_key",
			agents:     nil,
			wantAgents: false,
		},
		{
			name:       "empty_agents_omits_key",
			agents:     map[string]any{},
			wantAgents: false,
		},
		{
			name: "populated_agents_serializes",
			agents: map[string]any{
				"reviewer": map[string]any{
					"description": "Reviews code",
					"prompt":      "You are a reviewer.",
					"tools":       []string{"Read", "Grep"},
					"model":       "sonnet",
				},
			},
			wantAgents: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := setupControlTestContext(t, 3*time.Second)
			defer cancel()

			transport := newControlMockTransport()
			opts := []ProtocolOption{}
			if tc.agents != nil {
				opts = append(opts, WithAgents(tc.agents))
			}
			protocol := NewProtocol(transport, opts...)
			if err := protocol.Start(ctx); err != nil {
				t.Fatalf("protocol.Start: %v", err)
			}
			defer func() { _ = protocol.Close() }()

			// Respond to whatever initialize request shows up.
			go func() {
				req, ok := transport.waitForFirstWrite(time.Now().Add(2 * time.Second))
				if !ok {
					return
				}
				transport.injectResponse(req.RequestID, map[string]any{
					"supported_commands": []string{"initialize"},
				})
			}()

			if _, err := protocol.Initialize(ctx); err != nil {
				t.Fatalf("Initialize returned error: %v", err)
			}

			transport.mu.Lock()
			if len(transport.writtenData) == 0 {
				transport.mu.Unlock()
				t.Fatal("Initialize sent no data")
			}
			raw := transport.writtenData[0]
			transport.mu.Unlock()

			var envelope SDKControlRequest
			if err := json.Unmarshal(raw, &envelope); err != nil {
				t.Fatalf("unmarshal envelope: %v", err)
			}
			inner, ok := envelope.Request.(map[string]any)
			if !ok {
				t.Fatalf("inner request type %T, want map[string]any", envelope.Request)
			}
			_, hasAgents := inner["agents"]
			if hasAgents != tc.wantAgents {
				t.Errorf("agents key present=%v, want %v (wire body: %s)", hasAgents, tc.wantAgents, raw)
			}
			if tc.wantAgents {
				agentsMap, ok := inner["agents"].(map[string]any)
				if !ok {
					t.Fatalf("agents field type %T, want map[string]any", inner["agents"])
				}
				if _, ok := agentsMap["reviewer"]; !ok {
					t.Errorf("expected `reviewer` key in agents map, got %v", agentsMap)
				}
			}
		})
	}
}

// TestInitializeAlwaysCalled verifies that Protocol.Initialize sends an
// initialize request even when there are no hooks or agents configured,
// so the unified Connect path can always perform the handshake.
func TestInitializeAlwaysCalled(t *testing.T) {
	ctx, cancel := setupControlTestContext(t, 3*time.Second)
	defer cancel()

	transport := newControlMockTransport()
	protocol := NewProtocol(transport)
	if err := protocol.Start(ctx); err != nil {
		t.Fatalf("protocol.Start: %v", err)
	}
	defer func() { _ = protocol.Close() }()

	go func() {
		req, ok := transport.waitForFirstWrite(time.Now().Add(2 * time.Second))
		if !ok {
			return
		}
		transport.injectResponse(req.RequestID, map[string]any{
			"supported_commands": []string{"initialize"},
		})
	}()

	if _, err := protocol.Initialize(ctx); err != nil {
		t.Fatalf("Initialize returned error: %v", err)
	}

	if got := transport.getWriteCount(); got == 0 {
		t.Errorf("expected at least one write for the initialize request, got %d", got)
	}
}

// TestInitializeRequestAlwaysEmitsHooksKey pins the wire shape: the
// initialize request body always carries a `"hooks"` key, rendered as
// `null` when no hooks are registered (never omitted).
func TestInitializeRequestAlwaysEmitsHooksKey(t *testing.T) {
	ctx, cancel := setupControlTestContext(t, 3*time.Second)
	defer cancel()

	transport := newControlMockTransport()
	protocol := NewProtocol(transport)
	if err := protocol.Start(ctx); err != nil {
		t.Fatalf("protocol.Start: %v", err)
	}
	defer func() { _ = protocol.Close() }()

	go func() {
		req, ok := transport.waitForFirstWrite(time.Now().Add(2 * time.Second))
		if !ok {
			return
		}
		transport.injectResponse(req.RequestID, map[string]any{
			"supported_commands": []string{"initialize"},
		})
	}()

	if _, err := protocol.Initialize(ctx); err != nil {
		t.Fatalf("Initialize returned error: %v", err)
	}

	transport.mu.Lock()
	if len(transport.writtenData) == 0 {
		transport.mu.Unlock()
		t.Fatal("Initialize sent no data")
	}
	raw := transport.writtenData[0]
	transport.mu.Unlock()

	var envelope SDKControlRequest
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	inner, ok := envelope.Request.(map[string]any)
	if !ok {
		t.Fatalf("inner request type %T, want map[string]any", envelope.Request)
	}
	hooksVal, hasHooks := inner["hooks"]
	if !hasHooks {
		t.Fatalf("initialize request missing `hooks` key (wire body: %s)", raw)
	}
	if hooksVal != nil {
		t.Errorf("initialize request `hooks` = %v (%T), want null", hooksVal, hooksVal)
	}
}

// TestInitializeRequestSkills pins Python query.py: skills is sent only for a
// list, and an empty list stays on the wire as `[]` (disable all Skills).
func TestInitializeRequestSkills(t *testing.T) {
	tests := []struct {
		name       string
		skills     []string
		setSkills  bool
		wantSkills string
	}{
		{name: "unset_omits_key"},
		{name: "list_serializes", skills: []string{"pdf", "docx"}, setSkills: true, wantSkills: `["pdf","docx"]`},
		{name: "empty_list_serializes", skills: []string{}, setSkills: true, wantSkills: `[]`},
		{name: "nil_list_serializes_empty", skills: nil, setSkills: true, wantSkills: `[]`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var opts []ProtocolOption
			if tc.setSkills {
				opts = append(opts, WithSkills(tc.skills))
			}
			inner, raw := captureInitializeRequest(t, opts...)

			got, hasSkills := inner["skills"]
			if !tc.setSkills {
				if hasSkills {
					t.Errorf("skills key present, want absent (wire body: %s)", raw)
				}
				return
			}
			if !hasSkills {
				t.Fatalf("skills key absent, want %s (wire body: %s)", tc.wantSkills, raw)
			}
			encoded, err := json.Marshal(got)
			if err != nil {
				t.Fatalf("marshal skills: %v", err)
			}
			if string(encoded) != tc.wantSkills {
				t.Errorf("skills = %s, want %s", encoded, tc.wantSkills)
			}
		})
	}
}

// captureInitializeRequest runs Initialize against a mock transport and
// returns the inner request map and the raw wire bytes.
func captureInitializeRequest(t *testing.T, opts ...ProtocolOption) (map[string]any, []byte) {
	t.Helper()
	ctx, cancel := setupControlTestContext(t, 3*time.Second)
	defer cancel()

	transport := newControlMockTransport()
	protocol := NewProtocol(transport, opts...)
	if err := protocol.Start(ctx); err != nil {
		t.Fatalf("protocol.Start: %v", err)
	}
	defer func() { _ = protocol.Close() }()

	go func() {
		req, ok := transport.waitForFirstWrite(time.Now().Add(2 * time.Second))
		if !ok {
			return
		}
		transport.injectResponse(req.RequestID, map[string]any{
			"supported_commands": []string{"initialize"},
		})
	}()

	if _, err := protocol.Initialize(ctx); err != nil {
		t.Fatalf("Initialize returned error: %v", err)
	}

	transport.mu.Lock()
	if len(transport.writtenData) == 0 {
		transport.mu.Unlock()
		t.Fatal("Initialize sent no data")
	}
	raw := transport.writtenData[0]
	transport.mu.Unlock()

	var envelope SDKControlRequest
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	inner, ok := envelope.Request.(map[string]any)
	if !ok {
		t.Fatalf("inner request type %T, want map[string]any", envelope.Request)
	}
	return inner, raw
}
