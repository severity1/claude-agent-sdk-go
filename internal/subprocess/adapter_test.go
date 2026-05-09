package subprocess

import (
	"context"
	"testing"
	"time"

	"github.com/severity1/claude-agent-sdk-go/internal/control"
	"github.com/severity1/claude-agent-sdk-go/internal/shared"
)

// noopControlTransport is a minimal control.Transport used to drive a
// control.Protocol during adapter tests without needing a real subprocess.
// Writes are captured; the Read channel is closed so the protocol's readLoop
// exits cleanly on Close.
type noopControlTransport struct {
	writes [][]byte
	readCh chan []byte
}

func newNoopControlTransport() *noopControlTransport {
	return &noopControlTransport{readCh: make(chan []byte)}
}

func (n *noopControlTransport) Write(_ context.Context, data []byte) error {
	n.writes = append(n.writes, append([]byte(nil), data...))
	return nil
}

func (n *noopControlTransport) Read(_ context.Context) <-chan []byte { return n.readCh }

func (n *noopControlTransport) Close() error {
	select {
	case <-n.readCh:
	default:
		close(n.readCh)
	}
	return nil
}

// TestCanUseToolAdapter_PreservesToolUseIDAndAgentID exercises the subprocess
// adapter at buildProtocolOptions(): a control.ToolPermissionContext carrying
// ToolUseID and AgentID flows through the any-typed Options.CanUseTool bridge
// and lands in the user callback with both fields intact. This is the one
// adapter hop not covered by internal/control/protocol_test.go, which stops
// at the control layer. A silent regression in the bridge would leave hook
// callbacks unable to identify which tool call or subagent is being gated.
func TestCanUseToolAdapter_PreservesToolUseIDAndAgentID(t *testing.T) {
	var gotPermCtx any
	userCallback := func(_ context.Context, _ string, _ map[string]any, permCtx any) (any, error) {
		gotPermCtx = permCtx
		return control.NewPermissionResultAllow(), nil
	}

	tr := &Transport{options: &shared.Options{CanUseTool: userCallback}}
	opts := tr.buildProtocolOptions()
	if len(opts) == 0 {
		t.Fatal("buildProtocolOptions returned no options when CanUseTool is set")
		return
	}

	mock := newNoopControlTransport()
	p := control.NewProtocol(mock, opts...)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := p.Start(ctx); err != nil {
		t.Fatalf("protocol start: %v", err)
		return
	}
	defer func() { _ = p.Close() }()

	request := map[string]any{
		"type":       control.MessageTypeControlRequest,
		"request_id": "req_adapter_1",
		"request": map[string]any{
			"subtype":     control.SubtypeCanUseTool,
			"tool_name":   "Read",
			"input":       map[string]any{"file_path": "/tmp/x"},
			"tool_use_id": "toolu_01ABCDEF",
			"agent_id":    "agent_42",
		},
	}
	if err := p.HandleIncomingMessage(ctx, request); err != nil {
		t.Fatalf("HandleIncomingMessage: %v", err)
		return
	}

	tpc, ok := gotPermCtx.(control.ToolPermissionContext)
	if !ok {
		t.Fatalf("user callback received permCtx of type %T; expected control.ToolPermissionContext", gotPermCtx)
		return
	}
	if tpc.ToolUseID == nil || *tpc.ToolUseID != "toolu_01ABCDEF" {
		t.Errorf("ToolUseID = %v, want %q", tpc.ToolUseID, "toolu_01ABCDEF")
	}
	if tpc.AgentID == nil || *tpc.AgentID != "agent_42" {
		t.Errorf("AgentID = %v, want %q", tpc.AgentID, "agent_42")
	}

	_ = mock.writes
}
