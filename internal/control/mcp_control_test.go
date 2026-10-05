package control

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestProtocolMcpServerControl(t *testing.T) {
	reconnect := func(ctx context.Context, p *Protocol) error { return p.ReconnectMcpServer(ctx, "my-server") }
	toggleOff := func(ctx context.Context, p *Protocol) error { return p.ToggleMcpServer(ctx, "my-server", false) }
	toggleOn := func(ctx context.Context, p *Protocol) error { return p.ToggleMcpServer(ctx, "my-server", true) }
	success := func(m *controlMockTransport, id string) { m.injectResponse(id, nil) }
	failure := func(m *controlMockTransport, id string) { m.injectErrorResponse(id, "server not found") }

	tests := []struct {
		name     string
		call     func(context.Context, *Protocol) error
		respond  func(m *controlMockTransport, requestID string)
		wantErr  string
		wantWire string
	}{
		{
			name:     "reconnect",
			call:     reconnect,
			respond:  success,
			wantWire: `{"subtype":"mcp_reconnect","serverName":"my-server"}`,
		},
		{
			name:     "reconnect_error_response",
			call:     reconnect,
			respond:  failure,
			wantErr:  "server not found",
			wantWire: `{"subtype":"mcp_reconnect","serverName":"my-server"}`,
		},
		{
			name:     "toggle_disable_sends_false",
			call:     toggleOff,
			respond:  success,
			wantWire: `{"subtype":"mcp_toggle","serverName":"my-server","enabled":false}`,
		},
		{
			name:     "toggle_enable",
			call:     toggleOn,
			respond:  success,
			wantWire: `{"subtype":"mcp_toggle","serverName":"my-server","enabled":true}`,
		},
		{
			name:     "toggle_error_response",
			call:     toggleOn,
			respond:  failure,
			wantErr:  "server not found",
			wantWire: `{"subtype":"mcp_toggle","serverName":"my-server","enabled":true}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := setupControlTestContext(t, 5*time.Second)
			defer cancel()

			transport := newControlMockTransport()
			protocol := NewProtocol(transport)
			assertControlNoError(t, protocol.Start(ctx))
			defer func() { _ = protocol.Close() }()

			wire := make(chan string, 1)
			go func() {
				data, ok := transport.waitForWrite(time.Now().Add(4*time.Second), func([]byte) bool { return true })
				if !ok {
					wire <- ""
					return
				}
				var raw struct {
					RequestID string          `json:"request_id"`
					Request   json.RawMessage `json:"request"`
				}
				_ = json.Unmarshal(data, &raw)
				wire <- string(raw.Request)
				tt.respond(transport, raw.RequestID)
			}()

			err := tt.call(ctx, protocol)

			assertMcpControlError(t, err, tt.wantErr)
			if got := <-wire; got != tt.wantWire {
				t.Errorf("request = %s, want %s", got, tt.wantWire)
			}
		})
	}
}

func TestProtocolMcpServerControlRejectsEmptyName(t *testing.T) {
	ctx, cancel := setupControlTestContext(t, 5*time.Second)
	defer cancel()

	transport := newControlMockTransport()
	protocol := NewProtocol(transport)
	assertControlNoError(t, protocol.Start(ctx))
	defer func() { _ = protocol.Close() }()

	assertMcpControlError(t, protocol.ReconnectMcpServer(ctx, ""), "server name")
	assertMcpControlError(t, protocol.ToggleMcpServer(ctx, "", true), "server name")
	if _, ok := transport.waitForWrite(time.Now().Add(100*time.Millisecond), func([]byte) bool { return true }); ok {
		t.Error("wrote a control request for an empty server name, want none")
	}
}

func TestProtocolMcpServerControlContextCancel(t *testing.T) {
	ctx, cancel := setupControlTestContext(t, 5*time.Second)
	defer cancel()

	transport := newControlMockTransport()
	protocol := NewProtocol(transport)
	assertControlNoError(t, protocol.Start(ctx))
	defer func() { _ = protocol.Close() }()

	callCtx, callCancel := context.WithCancel(ctx)
	go func() {
		// Cancel after the request is written; nothing answers it.
		transport.waitForWrite(time.Now().Add(4*time.Second), func([]byte) bool { return true })
		callCancel()
	}()

	err := protocol.ReconnectMcpServer(callCtx, "my-server")
	if err == nil {
		t.Fatal("ReconnectMcpServer error = nil after cancel, want an error")
		return
	}
	if !strings.Contains(err.Error(), context.Canceled.Error()) {
		t.Errorf("ReconnectMcpServer error = %v, want context canceled", err)
	}
}

func assertMcpControlError(t *testing.T, err error, want string) {
	t.Helper()
	if want == "" {
		assertControlNoError(t, err)
		return
	}
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("error = %v, want one containing %q", err, want)
	}
}
