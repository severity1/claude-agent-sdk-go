package control

import (
	"encoding/json"
	"testing"
	"time"
)

// TestInitializeRequestIncludesAgents pins the Python SDK PR #468 wire shape:
// when WithAgents is configured, the initialize request carries an `agents`
// field; otherwise the key is omitted (omitempty).
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
