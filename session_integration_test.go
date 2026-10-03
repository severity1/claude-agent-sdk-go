//go:build session_integration

package claudecode_test

import (
	"context"
	"os/exec"
	"testing"
	"time"

	claudecode "github.com/severity1/claude-agent-sdk-go"
)

// TestIntegrationSessionRoundTrip runs a real Query against the Claude CLI,
// captures the session ID from the ResultMessage, then reads the session
// back using GetSessionInfo and GetSessionMessages.
//
// Requires: Claude CLI installed and authenticated.
// Run with: go test -tags integration -run TestIntegrationSessionRoundTrip -v
func TestIntegrationSessionRoundTrip(t *testing.T) {
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("claude CLI not found in PATH; skipping E2E test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	prompt := "Respond with exactly: hello from session test"

	iter, err := claudecode.Query(ctx, prompt,
		claudecode.WithMaxTurns(1),
		claudecode.WithPermissionMode(claudecode.PermissionModePlan),
	)
	if err != nil {
		t.Fatalf("Query() error: %v", err)
	}
	defer iter.Close()

	var sessionID string
	var gotResult bool
	for {
		msg, err := iter.Next(ctx)
		if err != nil {
			if err == claudecode.ErrNoMoreMessages {
				break
			}
			t.Fatalf("Next() error: %v", err)
		}

		if result, ok := msg.(*claudecode.ResultMessage); ok {
			sessionID = result.SessionID
			gotResult = true
			t.Logf("ResultMessage: subtype=%s session_id=%s", result.Subtype, result.SessionID)
		}
	}

	if !gotResult {
		t.Fatal("never received a ResultMessage")
	}
	if sessionID == "" {
		t.Fatal("session_id is empty on ResultMessage")
	}

	// Now read the session back using the session utilities.
	info, err := claudecode.GetSessionInfo(sessionID)
	if err != nil {
		t.Fatalf("GetSessionInfo(%s) error: %v", sessionID, err)
	}
	if info == nil {
		t.Fatalf("GetSessionInfo(%s) returned nil — session file not found on disk", sessionID)
	}

	t.Logf("Session info: id=%s summary=%q cwd=%v branch=%v",
		info.SessionID, info.Summary, info.Cwd, info.GitBranch)

	if info.SessionID != sessionID {
		t.Errorf("SessionID = %q, want %q", info.SessionID, sessionID)
	}
	if info.FileSize == nil || *info.FileSize == 0 {
		t.Error("FileSize should be non-zero for a completed session")
	}
	if info.CreatedAt == nil {
		t.Error("CreatedAt should be set")
	}
	// Read messages.
	msgs, err := claudecode.GetSessionMessages(sessionID)
	if err != nil {
		t.Fatalf("GetSessionMessages(%s) error: %v", sessionID, err)
	}

	t.Logf("Message count: %d", len(msgs))

	if len(msgs) == 0 {
		t.Fatal("expected at least one message")
	}

	// Verify we have at least one user and one assistant message.
	var hasUser, hasAssistant bool
	for _, m := range msgs {
		if m.Type == "user" {
			hasUser = true
		}
		if m.Type == "assistant" {
			hasAssistant = true
		}
		if m.SessionID != sessionID {
			t.Errorf("message SessionID = %q, want %q", m.SessionID, sessionID)
		}
	}
	if !hasUser {
		t.Error("no user messages found")
	}
	if !hasAssistant {
		t.Error("no assistant messages found")
	}

	// Verify the session appears in ListSessions.
	sessions, err := claudecode.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions() error: %v", err)
	}

	var found bool
	for _, s := range sessions {
		if s.SessionID == sessionID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("session %s not found in ListSessions() output", sessionID)
	}

	// Rename and tag the real session, then read both back.
	if err := claudecode.RenameSession(sessionID, "session integration test"); err != nil {
		t.Fatalf("RenameSession(%s) error: %v", sessionID, err)
	}
	tag := "integration"
	if err := claudecode.TagSession(sessionID, &tag); err != nil {
		t.Fatalf("TagSession(%s) error: %v", sessionID, err)
	}
	info, err = claudecode.GetSessionInfo(sessionID)
	if err != nil || info == nil {
		t.Fatalf("GetSessionInfo(%s) = %v, %v", sessionID, info, err)
	}
	if info.CustomTitle == nil || *info.CustomTitle != "session integration test" {
		t.Errorf("CustomTitle = %v, want session integration test", info.CustomTitle)
	}
	if info.Tag == nil || *info.Tag != tag {
		t.Errorf("Tag = %v, want %s", info.Tag, tag)
	}
}
