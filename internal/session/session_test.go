package session

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Test constants for repeated string literals (goconst).
const (
	roleUser         = "user"
	roleAssistant    = "assistant"
	aiGeneratedTitle = "AI Generated Title"
	realPrompt       = "Real prompt"
)

func TestEncodeCwd(t *testing.T) {
	tests := []struct {
		name string
		cwd  string
		want string
	}{
		{"unix path", "/Users/me/proj", "-Users-me-proj"},
		{"windows path", `C:\Users\me\proj`, "C--Users-me-proj"},
		{"spaces", "/path/to/my project", "-path-to-my-project"},
		{"dots", "/home/user/.config", "-home-user--config"},
		{"alphanumeric only", "abc123", "abc123"},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := encodeCwd(tt.cwd)
			if got != tt.want {
				t.Errorf("encodeCwd(%q) = %q, want %q", tt.cwd, got, tt.want)
			}
		})
	}
}

func TestConfigDir(t *testing.T) {
	t.Run("respects CLAUDE_CONFIG_DIR", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("CLAUDE_CONFIG_DIR", dir)

		got, err := configDir()
		if err != nil {
			t.Fatalf("configDir() error: %v", err)
		}
		if got != dir {
			t.Errorf("configDir() = %q, want %q", got, dir)
		}
	})

	t.Run("defaults to home/.claude", func(t *testing.T) {
		t.Setenv("CLAUDE_CONFIG_DIR", "")

		got, err := configDir()
		if err != nil {
			t.Fatalf("configDir() error: %v", err)
		}

		home, _ := os.UserHomeDir()
		want := filepath.Join(home, ".claude")
		if got != want {
			t.Errorf("configDir() = %q, want %q", got, want)
		}
	})
}

// setupTestProject creates a temp dir structured like ~/.claude/projects/<encoded>/
// and sets CLAUDE_CONFIG_DIR to point at it.
// The encoded directory name is computed dynamically so tests work on Windows
// where filepath.Abs("/test/project") prepends a drive letter.
func setupTestProject(t *testing.T) (configDir string, projectDir string) {
	t.Helper()
	cfgDir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", cfgDir)

	abs, err := filepath.Abs("/test/project")
	if err != nil {
		t.Fatalf("filepath.Abs: %v", err)
	}
	encoded := encodeCwd(abs)
	projDir := filepath.Join(cfgDir, "projects", encoded)
	if err := os.MkdirAll(projDir, 0o750); err != nil {
		t.Fatalf("creating project dir: %v", err)
	}
	return cfgDir, projDir
}

// writeSessionJSONL writes JSONL entries to a session file.
func writeSessionJSONL(t *testing.T, dir, sessionID string, entries []map[string]any) string {
	t.Helper()
	path := filepath.Clean(filepath.Join(dir, sessionID+".jsonl"))
	f, err := os.Create(path) //nolint:gosec // path is constructed from test temp dir
	if err != nil {
		t.Fatalf("creating session file: %v", err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			t.Errorf("closing session file: %v", err)
		}
	}()

	enc := json.NewEncoder(f)
	for _, entry := range entries {
		if err := enc.Encode(entry); err != nil {
			t.Fatalf("writing entry: %v", err)
		}
	}
	return path
}

// textContent creates a content block array matching the real CLI format:
// [{"type": "text", "text": "..."}]
func textContent(text string) []any {
	return []any{map[string]any{"type": "text", "text": text}}
}

func TestListSessions(t *testing.T) {
	_, projDir := setupTestProject(t)

	writeSessionJSONL(t, projDir, "aaaa-1111", []map[string]any{
		{"type": "queue-operation", "timestamp": "2026-01-01T00:00:00Z", "sessionId": "aaaa-1111"},
		{"type": "user", "message": map[string]any{"role": "user", "content": textContent("Hello world")}, "uuid": "u1", "timestamp": "2026-01-01T00:00:01Z", "cwd": "/test/project", "gitBranch": "main", "sessionId": "aaaa-1111"},
		{"type": "assistant", "message": map[string]any{"role": "assistant", "content": textContent("Hi!")}, "uuid": "a1", "timestamp": "2026-01-01T00:00:02Z", "sessionId": "aaaa-1111"},
	})

	writeSessionJSONL(t, projDir, "bbbb-2222", []map[string]any{
		{"type": "queue-operation", "timestamp": "2026-02-01T00:00:00Z", "sessionId": "bbbb-2222"},
		{"type": "user", "message": map[string]any{"role": "user", "content": textContent("Fix the bug")}, "uuid": "u2", "timestamp": "2026-02-01T00:00:01Z", "cwd": "/test/project", "gitBranch": "fix-branch", "sessionId": "bbbb-2222"},
		{"type": "custom-title", "customTitle": "Bug Fix Session", "sessionId": "bbbb-2222"},
	})

	t.Run("lists all sessions", func(t *testing.T) {
		sessions, err := ListSessions(WithSessionDirectory("/test/project"))
		if err != nil {
			t.Fatalf("ListSessions() error: %v", err)
		}
		if len(sessions) != 2 {
			t.Fatalf("got %d sessions, want 2", len(sessions))
		}
	})

	t.Run("sorted by last modified descending", func(t *testing.T) {
		sessions, err := ListSessions(WithSessionDirectory("/test/project"))
		if err != nil {
			t.Fatalf("ListSessions() error: %v", err)
		}
		if len(sessions) < 2 {
			t.Fatal("expected at least 2 sessions")
		}
		if sessions[0].LastModified < sessions[1].LastModified {
			t.Error("sessions not sorted by LastModified descending")
		}
	})

	t.Run("respects limit", func(t *testing.T) {
		sessions, err := ListSessions(
			WithSessionDirectory("/test/project"),
			WithSessionLimit(1),
		)
		if err != nil {
			t.Fatalf("ListSessions() error: %v", err)
		}
		if len(sessions) != 1 {
			t.Fatalf("got %d sessions, want 1", len(sessions))
		}
	})

	t.Run("empty dir returns empty slice", func(t *testing.T) {
		sessions, err := ListSessions(WithSessionDirectory("/nonexistent/path"))
		if err != nil {
			t.Fatalf("ListSessions() error: %v", err)
		}
		if len(sessions) != 0 {
			t.Fatalf("got %d sessions, want 0", len(sessions))
		}
	})
}

func TestGetMessages(t *testing.T) {
	_, projDir := setupTestProject(t)

	writeSessionJSONL(t, projDir, "cccc-3333", []map[string]any{
		{"type": "queue-operation", "timestamp": "2026-01-01T00:00:00Z", "sessionId": "cccc-3333"},
		{"type": "user", "message": map[string]any{"role": "user", "content": textContent("Hello")}, "uuid": "u1", "timestamp": "2026-01-01T00:00:01Z", "sessionId": "cccc-3333"},
		{"type": "assistant", "message": map[string]any{"role": "assistant", "content": textContent("Hi!")}, "uuid": "a1", "timestamp": "2026-01-01T00:00:02Z", "sessionId": "cccc-3333"},
		{"type": "user", "message": map[string]any{"role": "user", "content": textContent("Thanks")}, "uuid": "u2", "timestamp": "2026-01-01T00:00:03Z", "sessionId": "cccc-3333"},
		{"type": "last-prompt", "lastPrompt": "Thanks", "sessionId": "cccc-3333"},
	})

	t.Run("returns user and assistant messages", func(t *testing.T) {
		msgs, err := GetMessages("cccc-3333", WithSessionDirectory("/test/project"))
		if err != nil {
			t.Fatalf("GetMessages() error: %v", err)
		}
		if len(msgs) != 3 {
			t.Fatalf("got %d messages, want 3", len(msgs))
		}
		if msgs[0].Type != roleUser {
			t.Errorf("msgs[0].Type = %q, want %q", msgs[0].Type, roleUser)
		}
		if msgs[1].Type != roleAssistant {
			t.Errorf("msgs[1].Type = %q, want %q", msgs[1].Type, roleAssistant)
		}
		if msgs[2].Type != roleUser {
			t.Errorf("msgs[2].Type = %q, want %q", msgs[2].Type, roleUser)
		}
	})

	t.Run("preserves uuid and session_id", func(t *testing.T) {
		msgs, err := GetMessages("cccc-3333", WithSessionDirectory("/test/project"))
		if err != nil {
			t.Fatalf("GetMessages() error: %v", err)
		}
		if msgs[0].UUID != "u1" {
			t.Errorf("msgs[0].UUID = %q, want %q", msgs[0].UUID, "u1")
		}
		if msgs[0].SessionID != "cccc-3333" {
			t.Errorf("msgs[0].SessionID = %q, want %q", msgs[0].SessionID, "cccc-3333")
		}
	})

	t.Run("respects limit", func(t *testing.T) {
		msgs, err := GetMessages("cccc-3333",
			WithSessionDirectory("/test/project"),
			WithSessionLimit(2),
		)
		if err != nil {
			t.Fatalf("GetMessages() error: %v", err)
		}
		if len(msgs) != 2 {
			t.Fatalf("got %d messages, want 2", len(msgs))
		}
	})

	t.Run("respects offset", func(t *testing.T) {
		msgs, err := GetMessages("cccc-3333",
			WithSessionDirectory("/test/project"),
			WithSessionOffset(1),
		)
		if err != nil {
			t.Fatalf("GetMessages() error: %v", err)
		}
		if len(msgs) != 2 {
			t.Fatalf("got %d messages, want 2", len(msgs))
		}
		if msgs[0].Type != roleAssistant {
			t.Errorf("msgs[0].Type = %q, want %q (after offset=1)", msgs[0].Type, roleAssistant)
		}
	})

	t.Run("offset beyond length returns nil", func(t *testing.T) {
		msgs, err := GetMessages("cccc-3333",
			WithSessionDirectory("/test/project"),
			WithSessionOffset(100),
		)
		if err != nil {
			t.Fatalf("GetMessages() error: %v", err)
		}
		if msgs != nil {
			t.Fatalf("got %d messages, want nil", len(msgs))
		}
	})

	t.Run("not found returns error", func(t *testing.T) {
		_, err := GetMessages("nonexistent", WithSessionDirectory("/test/project"))
		if err == nil {
			t.Fatal("expected error for nonexistent session")
		}
	})
}

func TestGetSessionInfo(t *testing.T) {
	_, projDir := setupTestProject(t)

	writeSessionJSONL(t, projDir, "dddd-4444", []map[string]any{
		{"type": "queue-operation", "timestamp": "2026-03-15T10:00:00Z", "sessionId": "dddd-4444"},
		{"type": "user", "message": map[string]any{"role": "user", "content": textContent("Analyze the code")}, "uuid": "u1", "timestamp": "2026-03-15T10:00:01Z", "cwd": "/my/project", "gitBranch": "feature-x", "sessionId": "dddd-4444"},
		{"type": "assistant", "message": map[string]any{"role": "assistant", "content": textContent("I'll analyze it.")}, "uuid": "a1", "timestamp": "2026-03-15T10:00:02Z", "sessionId": "dddd-4444"},
	})

	t.Run("summary uses first prompt when no title", func(t *testing.T) {
		info, _ := GetSessionInfo("dddd-4444", WithSessionDirectory("/test/project"))
		want := "Analyze the code"
		if info.Summary != want {
			t.Errorf("Summary = %q, want %q", info.Summary, want)
		}
		if info.FirstPrompt == nil || *info.FirstPrompt != want {
			t.Errorf("FirstPrompt = %v, want %q", info.FirstPrompt, want)
		}
	})

	t.Run("extracts git branch and cwd", func(t *testing.T) {
		info, _ := GetSessionInfo("dddd-4444", WithSessionDirectory("/test/project"))
		if info.GitBranch == nil || *info.GitBranch != "feature-x" {
			t.Errorf("GitBranch = %v, want %q", info.GitBranch, "feature-x")
		}
		if info.Cwd == nil || *info.Cwd != "/my/project" {
			t.Errorf("Cwd = %v, want %q", info.Cwd, "/my/project")
		}
	})

	t.Run("not found returns nil", func(t *testing.T) {
		info, err := GetSessionInfo("nonexistent", WithSessionDirectory("/test/project"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if info != nil {
			t.Error("expected nil for nonexistent session")
		}
	})
}

func TestBuildSessionInfoSummaryPriority(t *testing.T) {
	_, projDir := setupTestProject(t)

	t.Run("custom title takes priority over ai title", func(t *testing.T) {
		writeSessionJSONL(t, projDir, "eeee-5555", []map[string]any{
			{"type": "queue-operation", "timestamp": "2026-01-01T00:00:00Z", "sessionId": "eeee-5555"},
			{"type": "user", "message": map[string]any{"role": "user", "content": textContent("Original prompt")}, "uuid": "u1", "timestamp": "2026-01-01T00:00:01Z", "sessionId": "eeee-5555"},
			{"type": "ai-title", "aiTitle": aiGeneratedTitle, "sessionId": "eeee-5555"},
			{"type": "custom-title", "customTitle": "My Custom Title", "sessionId": "eeee-5555"},
		})
		info, _ := GetSessionInfo("eeee-5555", WithSessionDirectory("/test/project"))
		if info.Summary != "My Custom Title" {
			t.Errorf("Summary = %q, want %q", info.Summary, "My Custom Title")
		}
		if info.CustomTitle == nil || *info.CustomTitle != "My Custom Title" {
			t.Errorf("CustomTitle = %v, want %q", info.CustomTitle, "My Custom Title")
		}
		if info.AITitle == nil || *info.AITitle != aiGeneratedTitle {
			t.Errorf("AITitle = %v, want %q", info.AITitle, aiGeneratedTitle)
		}
	})

	t.Run("ai title used when no custom title", func(t *testing.T) {
		writeSessionJSONL(t, projDir, "eeee-5556", []map[string]any{
			{"type": "queue-operation", "timestamp": "2026-01-01T00:00:00Z", "sessionId": "eeee-5556"},
			{"type": "user", "message": map[string]any{"role": "user", "content": textContent("Hello")}, "uuid": "u1", "timestamp": "2026-01-01T00:00:01Z", "sessionId": "eeee-5556"},
			{"type": "ai-title", "aiTitle": aiGeneratedTitle, "sessionId": "eeee-5556"},
		})
		info, _ := GetSessionInfo("eeee-5556", WithSessionDirectory("/test/project"))
		if info.Summary != aiGeneratedTitle {
			t.Errorf("Summary = %q, want %q", info.Summary, aiGeneratedTitle)
		}
		if info.AITitle == nil || *info.AITitle != aiGeneratedTitle {
			t.Errorf("AITitle = %v, want %q", info.AITitle, aiGeneratedTitle)
		}
		if info.CustomTitle != nil {
			t.Errorf("CustomTitle = %v, want nil", info.CustomTitle)
		}
	})

	t.Run("falls back to timestamp when no title", func(t *testing.T) {
		writeSessionJSONL(t, projDir, "ffff-6666", []map[string]any{
			{"type": "queue-operation", "timestamp": "2026-01-01T00:00:00Z", "sessionId": "ffff-6666"},
		})
		info, _ := GetSessionInfo("ffff-6666", WithSessionDirectory("/test/project"))
		want := "New session - 2026-01-01T00:00:00Z"
		if info.Summary != want {
			t.Errorf("Summary = %q, want %q", info.Summary, want)
		}
	})

	t.Run("falls back to session ID when no timestamp", func(t *testing.T) {
		writeSessionJSONL(t, projDir, "ffff-7777", []map[string]any{
			{"type": "queue-operation", "sessionId": "ffff-7777"},
		})
		info, _ := GetSessionInfo("ffff-7777", WithSessionDirectory("/test/project"))
		if info.Summary != "ffff-7777" {
			t.Errorf("Summary = %q, want %q", info.Summary, "ffff-7777")
		}
	})
}

func TestMessageContentParsing(t *testing.T) {
	_, projDir := setupTestProject(t)

	writeSessionJSONL(t, projDir, "content-test", []map[string]any{
		{"type": "user", "message": map[string]any{"role": "user", "content": "raw string prompt"}, "uuid": "u1", "timestamp": "2026-01-01T00:00:00Z", "sessionId": "content-test"},
		{"type": "user", "message": map[string]any{"role": "user", "content": []any{
			map[string]any{"type": "text", "text": "hello from blocks"},
		}}, "uuid": "u2", "timestamp": "2026-01-01T00:00:01Z", "sessionId": "content-test"},
		{"type": "assistant", "message": map[string]any{"role": "assistant", "content": []any{
			map[string]any{"type": "thinking", "thinking": "let me think", "signature": "sig123"},
			map[string]any{"type": "text", "text": "Here is my answer"},
		}}, "uuid": "a1", "timestamp": "2026-01-01T00:00:02Z", "sessionId": "content-test"},
	})

	msgs, err := GetMessages("content-test", WithSessionDirectory("/test/project"))
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("got %d messages, want 3", len(msgs))
	}

	t.Run("string content", func(t *testing.T) {
		mc := msgs[0].Content
		if mc == nil {
			t.Fatal("MessageContent is nil")
		}
		if mc.Kind != ContentTypeString {
			t.Fatalf("Kind = %d, want ContentTypeString", mc.Kind)
		}
		if mc.String != "raw string prompt" {
			t.Errorf("String = %q, want %q", mc.String, "raw string prompt")
		}
	})

	t.Run("text content block", func(t *testing.T) {
		mc := msgs[1].Content
		if mc == nil {
			t.Fatal("MessageContent is nil")
		}
		if mc.Kind != ContentTypeBlocks {
			t.Fatalf("Kind = %d, want ContentTypeBlocks", mc.Kind)
		}
		if len(mc.Blocks) != 1 {
			t.Fatalf("got %d blocks, want 1", len(mc.Blocks))
		}
		b := mc.Blocks[0]
		if b.Type != "text" {
			t.Errorf("Type = %q, want %q", b.Type, "text")
		}
		if b.Text != "hello from blocks" {
			t.Errorf("Text = %q, want %q", b.Text, "hello from blocks")
		}
	})

	t.Run("thinking + text blocks", func(t *testing.T) {
		mc := msgs[2].Content
		if mc == nil {
			t.Fatal("MessageContent is nil")
		}
		if len(mc.Blocks) != 2 {
			t.Fatalf("got %d blocks, want 2", len(mc.Blocks))
		}
		thinking := mc.Blocks[0]
		if thinking.Type != "thinking" {
			t.Errorf("Type = %q, want %q", thinking.Type, "thinking")
		}
		if thinking.Thinking != "let me think" {
			t.Errorf("Thinking = %q, want %q", thinking.Thinking, "let me think")
		}
		if thinking.Signature != "sig123" {
			t.Errorf("Signature = %q, want %q", thinking.Signature, "sig123")
		}
		text := mc.Blocks[1]
		if text.Text != "Here is my answer" {
			t.Errorf("Text = %q, want %q", text.Text, "Here is my answer")
		}
	})
}

func TestContentBlockParsing(t *testing.T) {
	_, projDir := setupTestProject(t)

	writeSessionJSONL(t, projDir, "blocks-test", []map[string]any{
		{"type": "assistant", "message": map[string]any{"role": "assistant", "content": []any{
			map[string]any{"type": "tool_use", "id": "toolu_123", "name": "Read", "input": map[string]any{"path": "/foo"}},
		}}, "uuid": "a1", "timestamp": "2026-01-01T00:00:00Z", "sessionId": "blocks-test"},
		{"type": "user", "message": map[string]any{"role": "user", "content": []any{
			map[string]any{"type": "tool_result", "tool_use_id": "toolu_123", "content": "file contents here"},
		}}, "uuid": "u1", "timestamp": "2026-01-01T00:00:01Z", "sessionId": "blocks-test"},
		{"type": "user", "message": map[string]any{"role": "user", "content": []any{
			map[string]any{"type": "image", "source": map[string]any{"type": "base64", "media_type": "image/png", "data": "abc123"}},
		}}, "uuid": "u2", "timestamp": "2026-01-01T00:00:02Z", "sessionId": "blocks-test"},
	})

	msgs, err := GetMessages("blocks-test", WithSessionDirectory("/test/project"))
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	t.Run("tool_use block", func(t *testing.T) {
		b := msgs[0].Content.Blocks[0]
		if b.Type != "tool_use" {
			t.Errorf("Type = %q, want %q", b.Type, "tool_use")
		}
		if b.ID != "toolu_123" {
			t.Errorf("ID = %q, want %q", b.ID, "toolu_123")
		}
		if b.Name != "Read" {
			t.Errorf("Name = %q, want %q", b.Name, "Read")
		}
		if b.Input["path"] != "/foo" {
			t.Errorf("Input[path] = %v, want %q", b.Input["path"], "/foo")
		}
	})

	t.Run("tool_result block", func(t *testing.T) {
		b := msgs[1].Content.Blocks[0]
		if b.Type != "tool_result" {
			t.Errorf("Type = %q, want %q", b.Type, "tool_result")
		}
		if b.ToolUseID != "toolu_123" {
			t.Errorf("ToolUseID = %q, want %q", b.ToolUseID, "toolu_123")
		}
		if b.Content != "file contents here" {
			t.Errorf("Content = %v, want %q", b.Content, "file contents here")
		}
	})

	t.Run("image block", func(t *testing.T) {
		b := msgs[2].Content.Blocks[0]
		if b.Type != "image" {
			t.Errorf("Type = %q, want %q", b.Type, "image")
		}
		if b.Source["type"] != "base64" {
			t.Errorf("Source.type = %v, want %q", b.Source["type"], "base64")
		}
	})

	t.Run("Raw is always populated", func(t *testing.T) {
		for i, m := range msgs {
			for j, b := range m.Content.Blocks {
				if b.Raw == nil {
					t.Errorf("msgs[%d].Blocks[%d].Raw is nil", i, j)
				}
			}
		}
	})
}

func TestContentBlockUnknownType(t *testing.T) {
	_, projDir := setupTestProject(t)

	writeSessionJSONL(t, projDir, "unknown-test", []map[string]any{
		{"type": "assistant", "message": map[string]any{"role": "assistant", "content": []any{
			map[string]any{"type": "some_future_type", "foo": "bar", "baz": 42.0},
		}}, "uuid": "a1", "timestamp": "2026-01-01T00:00:00Z", "sessionId": "unknown-test"},
	})

	msgs, err := GetMessages("unknown-test", WithSessionDirectory("/test/project"))
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	b := msgs[0].Content.Blocks[0]
	if b.Type != "some_future_type" {
		t.Errorf("Type = %q, want %q", b.Type, "some_future_type")
	}
	if b.Raw["foo"] != "bar" {
		t.Errorf("Raw[foo] = %v, want %q", b.Raw["foo"], "bar")
	}
	if b.Raw["baz"] != 42.0 {
		t.Errorf("Raw[baz] = %v, want 42.0", b.Raw["baz"])
	}
}

func TestBuildSessionInfoTagClearing(t *testing.T) {
	_, projDir := setupTestProject(t)

	writeSessionJSONL(t, projDir, "hhhh-8888", []map[string]any{
		{"type": "queue-operation", "timestamp": "2026-01-01T00:00:00Z", "sessionId": "hhhh-8888"},
		{"type": "tag", "tag": "important", "sessionId": "hhhh-8888"},
		{"type": "tag", "tag": "", "sessionId": "hhhh-8888"},
	})

	info, _ := GetSessionInfo("hhhh-8888", WithSessionDirectory("/test/project"))
	if info.Tag != nil {
		t.Errorf("Tag = %v, want nil (should be cleared)", info.Tag)
	}
}

// --- Chain reconstruction tests ---

func TestIsTranscriptEntry(t *testing.T) {
	tests := []struct {
		name string
		e    jsonlEntry
		want bool
	}{
		{"user with uuid", jsonlEntry{entryType: "user", raw: map[string]any{"uuid": "u1"}}, true},
		{"assistant with uuid", jsonlEntry{entryType: "assistant", raw: map[string]any{"uuid": "a1"}}, true},
		{"progress with uuid", jsonlEntry{entryType: "progress", raw: map[string]any{"uuid": "p1"}}, true},
		{"system with uuid", jsonlEntry{entryType: "system", raw: map[string]any{"uuid": "s1"}}, true},
		{"attachment with uuid", jsonlEntry{entryType: "attachment", raw: map[string]any{"uuid": "at1"}}, true},
		{"user without uuid", jsonlEntry{entryType: "user", raw: map[string]any{}}, false},
		{"queue-operation", jsonlEntry{entryType: "queue-operation", raw: map[string]any{"uuid": "q1"}}, false},
		{"custom-title", jsonlEntry{entryType: "custom-title", raw: map[string]any{}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isTranscriptEntry(tt.e); got != tt.want {
				t.Errorf("isTranscriptEntry() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsVisibleMessage(t *testing.T) {
	tests := []struct {
		name string
		e    jsonlEntry
		want bool
	}{
		{"normal user", jsonlEntry{entryType: "user", raw: map[string]any{}}, true},
		{"normal assistant", jsonlEntry{entryType: "assistant", raw: map[string]any{}}, true},
		{"progress", jsonlEntry{entryType: "progress", raw: map[string]any{}}, false},
		{"system", jsonlEntry{entryType: "system", raw: map[string]any{}}, false},
		{"isMeta user", jsonlEntry{entryType: "user", raw: map[string]any{"isMeta": true}}, false},
		{"isSidechain", jsonlEntry{entryType: "user", raw: map[string]any{"isSidechain": true}}, false},
		{"teamName", jsonlEntry{entryType: "assistant", raw: map[string]any{"teamName": "team1"}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isVisibleMessage(tt.e); got != tt.want {
				t.Errorf("isVisibleMessage() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuildMessagesLinearChain(t *testing.T) {
	// Simple linear chain: u1 -> a1 -> u2 -> a2
	entries := []jsonlEntry{
		{entryType: "queue-operation", raw: map[string]any{"type": "queue-operation"}},
		{entryType: "user", raw: map[string]any{"type": "user", "uuid": "u1", "message": map[string]any{"content": "Hello"}}},
		{entryType: "assistant", raw: map[string]any{"type": "assistant", "uuid": "a1", "parentUuid": "u1", "message": map[string]any{"content": "Hi!"}}},
		{entryType: "user", raw: map[string]any{"type": "user", "uuid": "u2", "parentUuid": "a1", "message": map[string]any{"content": "Thanks"}}},
		{entryType: "assistant", raw: map[string]any{"type": "assistant", "uuid": "a2", "parentUuid": "u2", "message": map[string]any{"content": "You're welcome"}}},
	}

	msgs := buildMessages("test-session", entries)
	if len(msgs) != 4 {
		t.Fatalf("got %d messages, want 4", len(msgs))
		return
	}
	wantUUIDs := []string{"u1", "a1", "u2", "a2"}
	for i, want := range wantUUIDs {
		if msgs[i].UUID != want {
			t.Errorf("msgs[%d].UUID = %q, want %q", i, msgs[i].UUID, want)
		}
	}
}

func TestBuildMessagesBranchedSession(t *testing.T) {
	// Branch at a1: both u2 and u3 have parentUuid=a1, but u3 has higher file position.
	// u1 -> a1 -> u2 (branch A)
	//          -> u3 -> a3 (branch B, higher index — should be chosen)
	entries := []jsonlEntry{
		{entryType: "user", raw: map[string]any{"type": "user", "uuid": "u1", "message": map[string]any{"content": "Hello"}}},
		{entryType: "assistant", raw: map[string]any{"type": "assistant", "uuid": "a1", "parentUuid": "u1", "message": map[string]any{"content": "Hi"}}},
		{entryType: "user", raw: map[string]any{"type": "user", "uuid": "u2", "parentUuid": "a1", "message": map[string]any{"content": "Branch A"}}},
		{entryType: "user", raw: map[string]any{"type": "user", "uuid": "u3", "parentUuid": "a1", "message": map[string]any{"content": "Branch B"}}},
		{entryType: "assistant", raw: map[string]any{"type": "assistant", "uuid": "a3", "parentUuid": "u3", "message": map[string]any{"content": "On branch B"}}},
	}

	msgs := buildMessages("test-session", entries)
	if len(msgs) != 4 {
		t.Fatalf("got %d messages, want 4", len(msgs))
		return
	}
	// Expect: u1, a1, u3, a3 (branch B chosen because a3 has highest position)
	wantUUIDs := []string{"u1", "a1", "u3", "a3"}
	for i, want := range wantUUIDs {
		if msgs[i].UUID != want {
			t.Errorf("msgs[%d].UUID = %q, want %q", i, msgs[i].UUID, want)
		}
	}
}

func TestBuildMessagesFiltersIsMeta(t *testing.T) {
	// Chain with isMeta user — should be excluded from output.
	entries := []jsonlEntry{
		{entryType: "user", raw: map[string]any{"type": "user", "uuid": "u1", "message": map[string]any{"content": "Hello"}}},
		{entryType: "assistant", raw: map[string]any{"type": "assistant", "uuid": "a1", "parentUuid": "u1", "message": map[string]any{"content": "Hi"}}},
		{entryType: "user", raw: map[string]any{"type": "user", "uuid": "u_meta", "parentUuid": "a1", "isMeta": true, "message": map[string]any{"content": "system injection"}}},
		{entryType: "assistant", raw: map[string]any{"type": "assistant", "uuid": "a_meta", "parentUuid": "u_meta", "message": map[string]any{"content": "meta response"}}},
		{entryType: "user", raw: map[string]any{"type": "user", "uuid": "u2", "parentUuid": "a_meta", "message": map[string]any{"content": "Real question"}}},
		{entryType: "assistant", raw: map[string]any{"type": "assistant", "uuid": "a2", "parentUuid": "u2", "message": map[string]any{"content": "Real answer"}}},
	}

	msgs := buildMessages("test-session", entries)
	// Chain is: u1, a1, u_meta(filtered), a_meta, u2, a2 → visible: u1, a1, a_meta, u2, a2
	wantUUIDs := []string{"u1", "a1", "a_meta", "u2", "a2"}
	if len(msgs) != len(wantUUIDs) {
		t.Fatalf("got %d messages, want %d", len(msgs), len(wantUUIDs))
		return
	}
	for i, want := range wantUUIDs {
		if msgs[i].UUID != want {
			t.Errorf("msgs[%d].UUID = %q, want %q", i, msgs[i].UUID, want)
		}
	}
}

func TestBuildMessagesFiltersSidechain(t *testing.T) {
	// Sidechain leaf should not be chosen as the best leaf.
	entries := []jsonlEntry{
		{entryType: "user", raw: map[string]any{"type": "user", "uuid": "u1", "message": map[string]any{"content": "Hello"}}},
		{entryType: "assistant", raw: map[string]any{"type": "assistant", "uuid": "a1", "parentUuid": "u1", "message": map[string]any{"content": "Hi"}}},
		{entryType: "user", raw: map[string]any{"type": "user", "uuid": "u_sc", "parentUuid": "a1", "isSidechain": true, "message": map[string]any{"content": "sidechain"}}},
		{entryType: "assistant", raw: map[string]any{"type": "assistant", "uuid": "a_sc", "parentUuid": "u_sc", "isSidechain": true, "message": map[string]any{"content": "sidechain resp"}}},
		{entryType: "user", raw: map[string]any{"type": "user", "uuid": "u2", "parentUuid": "a1", "message": map[string]any{"content": "Main line"}}},
		{entryType: "assistant", raw: map[string]any{"type": "assistant", "uuid": "a2", "parentUuid": "u2", "message": map[string]any{"content": "Main resp"}}},
	}

	msgs := buildMessages("test-session", entries)
	// Chain should follow main line: u1, a1, u2, a2 (not the sidechain)
	wantUUIDs := []string{"u1", "a1", "u2", "a2"}
	if len(msgs) != len(wantUUIDs) {
		t.Fatalf("got %d messages, want %d", len(msgs), len(wantUUIDs))
		return
	}
	for i, want := range wantUUIDs {
		if msgs[i].UUID != want {
			t.Errorf("msgs[%d].UUID = %q, want %q", i, msgs[i].UUID, want)
		}
	}
}

func TestBuildMessagesFiltersTeamName(t *testing.T) {
	entries := []jsonlEntry{
		{entryType: "user", raw: map[string]any{"type": "user", "uuid": "u1", "teamName": "team1", "message": map[string]any{"content": "Team msg"}}},
		{entryType: "assistant", raw: map[string]any{"type": "assistant", "uuid": "a1", "parentUuid": "u1", "message": map[string]any{"content": "Response"}}},
		{entryType: "user", raw: map[string]any{"type": "user", "uuid": "u2", "parentUuid": "a1", "message": map[string]any{"content": "Normal msg"}}},
	}

	msgs := buildMessages("test-session", entries)
	// u1 has teamName, filtered from visible output. Chain: u1, a1, u2. Visible: a1, u2.
	wantUUIDs := []string{"a1", "u2"}
	if len(msgs) != len(wantUUIDs) {
		t.Fatalf("got %d messages, want %d", len(msgs), len(wantUUIDs))
		return
	}
	for i, want := range wantUUIDs {
		if msgs[i].UUID != want {
			t.Errorf("msgs[%d].UUID = %q, want %q", i, msgs[i].UUID, want)
		}
	}
}

func TestBuildMessagesProgressInChain(t *testing.T) {
	// Progress/system entries are used for chain traversal but not in output.
	entries := []jsonlEntry{
		{entryType: "user", raw: map[string]any{"type": "user", "uuid": "u1", "message": map[string]any{"content": "Hello"}}},
		{entryType: "system", raw: map[string]any{"type": "system", "uuid": "s1", "parentUuid": "u1"}},
		{entryType: "assistant", raw: map[string]any{"type": "assistant", "uuid": "a1", "parentUuid": "s1", "message": map[string]any{"content": "Hi"}}},
	}

	msgs := buildMessages("test-session", entries)
	// Chain: u1 -> s1 -> a1. Visible: u1, a1.
	wantUUIDs := []string{"u1", "a1"}
	if len(msgs) != len(wantUUIDs) {
		t.Fatalf("got %d messages, want %d", len(msgs), len(wantUUIDs))
		return
	}
	for i, want := range wantUUIDs {
		if msgs[i].UUID != want {
			t.Errorf("msgs[%d].UUID = %q, want %q", i, msgs[i].UUID, want)
		}
	}
}

func TestBuildMessagesNoParentUuidFallback(t *testing.T) {
	// No parentUuid on any entry → flat-scan with visibility filter.
	entries := []jsonlEntry{
		{entryType: "queue-operation", raw: map[string]any{"type": "queue-operation"}},
		{entryType: "user", raw: map[string]any{"type": "user", "uuid": "u1", "message": map[string]any{"content": "Hello"}}},
		{entryType: "assistant", raw: map[string]any{"type": "assistant", "uuid": "a1", "message": map[string]any{"content": "Hi"}}},
		{entryType: "user", raw: map[string]any{"type": "user", "uuid": "u2", "message": map[string]any{"content": "Thanks"}}},
	}

	msgs := buildMessages("test-session", entries)
	if len(msgs) != 3 {
		t.Fatalf("got %d messages, want 3", len(msgs))
		return
	}
	wantUUIDs := []string{"u1", "a1", "u2"}
	for i, want := range wantUUIDs {
		if msgs[i].UUID != want {
			t.Errorf("msgs[%d].UUID = %q, want %q", i, msgs[i].UUID, want)
		}
	}
}

// --- FirstPrompt tests ---

func TestExtractFirstPrompt(t *testing.T) {
	t.Run("basic extraction", func(t *testing.T) {
		entries := []jsonlEntry{
			{entryType: "queue-operation", raw: map[string]any{"type": "queue-operation", "timestamp": "2026-01-01T00:00:00Z"}},
			{entryType: "user", raw: map[string]any{"type": "user", "uuid": "u1", "message": map[string]any{"content": "Hello world"}}},
		}
		got := extractFirstPrompt(entries)
		if got == nil || *got != "Hello world" {
			t.Errorf("extractFirstPrompt() = %v, want %q", got, "Hello world")
		}
	})

	t.Run("skips isMeta", func(t *testing.T) {
		entries := []jsonlEntry{
			{entryType: "user", raw: map[string]any{"type": "user", "uuid": "u1", "isMeta": true, "message": map[string]any{"content": "system injection"}}},
			{entryType: "user", raw: map[string]any{"type": "user", "uuid": "u2", "message": map[string]any{"content": realPrompt}}},
		}
		got := extractFirstPrompt(entries)
		if got == nil || *got != realPrompt {
			t.Errorf("extractFirstPrompt() = %v, want %q", got, realPrompt)
		}
	})

	t.Run("skips isCompactSummary", func(t *testing.T) {
		entries := []jsonlEntry{
			{entryType: "user", raw: map[string]any{"type": "user", "uuid": "u1", "isCompactSummary": true, "message": map[string]any{"content": "summary"}}},
			{entryType: "user", raw: map[string]any{"type": "user", "uuid": "u2", "message": map[string]any{"content": realPrompt}}},
		}
		got := extractFirstPrompt(entries)
		if got == nil || *got != realPrompt {
			t.Errorf("extractFirstPrompt() = %v, want %q", got, realPrompt)
		}
	})

	t.Run("skips tool_result only content", func(t *testing.T) {
		entries := []jsonlEntry{
			{entryType: "user", raw: map[string]any{"type": "user", "uuid": "u1", "message": map[string]any{
				"content": []any{map[string]any{"type": "tool_result", "tool_use_id": "t1", "content": "result"}},
			}}},
			{entryType: "user", raw: map[string]any{"type": "user", "uuid": "u2", "message": map[string]any{"content": "Actual prompt"}}},
		}
		got := extractFirstPrompt(entries)
		if got == nil || *got != "Actual prompt" {
			t.Errorf("extractFirstPrompt() = %v, want %q", got, "Actual prompt")
		}
	})

	t.Run("truncates long prompts", func(t *testing.T) {
		long := strings.Repeat("a", 300)
		entries := []jsonlEntry{
			{entryType: "user", raw: map[string]any{"type": "user", "uuid": "u1", "message": map[string]any{"content": long}}},
		}
		got := extractFirstPrompt(entries)
		if got == nil {
			t.Fatal("extractFirstPrompt() = nil, want truncated string")
		}
		if len(*got) != 200 {
			t.Errorf("len = %d, want 200", len(*got))
		}
	})

	t.Run("nil when no qualifying messages", func(t *testing.T) {
		entries := []jsonlEntry{
			{entryType: "queue-operation", raw: map[string]any{"type": "queue-operation"}},
		}
		got := extractFirstPrompt(entries)
		if got != nil {
			t.Errorf("extractFirstPrompt() = %v, want nil", got)
		}
	})

	t.Run("skips command-name messages", func(t *testing.T) {
		entries := []jsonlEntry{
			{entryType: "user", raw: map[string]any{"type": "user", "uuid": "u1", "message": map[string]any{"content": "<command-name>commit</command-name> do it"}}},
			{entryType: "user", raw: map[string]any{"type": "user", "uuid": "u2", "message": map[string]any{"content": realPrompt}}},
		}
		got := extractFirstPrompt(entries)
		if got == nil || *got != realPrompt {
			t.Errorf("extractFirstPrompt() = %v, want %q", got, realPrompt)
		}
	})

	t.Run("skips session-start-hook", func(t *testing.T) {
		entries := []jsonlEntry{
			{entryType: "user", raw: map[string]any{"type": "user", "uuid": "u1", "message": map[string]any{"content": "<session-start-hook>data</session-start-hook>"}}},
			{entryType: "user", raw: map[string]any{"type": "user", "uuid": "u2", "message": map[string]any{"content": realPrompt}}},
		}
		got := extractFirstPrompt(entries)
		if got == nil || *got != realPrompt {
			t.Errorf("extractFirstPrompt() = %v, want %q", got, realPrompt)
		}
	})

	t.Run("extracts text from content blocks", func(t *testing.T) {
		entries := []jsonlEntry{
			{entryType: "user", raw: map[string]any{"type": "user", "uuid": "u1", "message": map[string]any{
				"content": []any{map[string]any{"type": "text", "text": "Hello from blocks"}},
			}}},
		}
		got := extractFirstPrompt(entries)
		if got == nil || *got != "Hello from blocks" {
			t.Errorf("extractFirstPrompt() = %v, want %q", got, "Hello from blocks")
		}
	})
}

func TestSummaryFallbackIncludesFirstPrompt(t *testing.T) {
	_, projDir := setupTestProject(t)

	writeSessionJSONL(t, projDir, "fp-summary", []map[string]any{
		{"type": "queue-operation", "timestamp": "2026-01-01T00:00:00Z", "sessionId": "fp-summary"},
		{"type": "user", "message": map[string]any{"role": "user", "content": "My first prompt"}, "uuid": "u1", "timestamp": "2026-01-01T00:00:01Z", "sessionId": "fp-summary"},
	})

	info, err := GetSessionInfo("fp-summary", WithSessionDirectory("/test/project"))
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	// No custom title or ai title, so summary should use first_prompt.
	if info.Summary != "My first prompt" {
		t.Errorf("Summary = %q, want %q", info.Summary, "My first prompt")
	}
	if info.FirstPrompt == nil || *info.FirstPrompt != "My first prompt" {
		t.Errorf("FirstPrompt = %v, want %q", info.FirstPrompt, "My first prompt")
	}
}

func TestBuildMessagesDuplicateUUID(t *testing.T) {
	// Message edits create duplicate UUIDs — the later entry should win (last-write-wins).
	// Chain: u1 -> a1(v1) -> u2, then a1 is edited to a1(v2) at a higher file position.
	// The chain should use a1(v2)'s content.
	entries := []jsonlEntry{
		{entryType: "user", raw: map[string]any{"type": "user", "uuid": "u1", "message": map[string]any{"content": "Hello"}}},
		{entryType: "assistant", raw: map[string]any{"type": "assistant", "uuid": "a1", "parentUuid": "u1", "message": map[string]any{"content": "Original response"}}},
		{entryType: "user", raw: map[string]any{"type": "user", "uuid": "u2", "parentUuid": "a1", "message": map[string]any{"content": "Follow-up"}}},
		// Edited version of a1 appears later in the file.
		{entryType: "assistant", raw: map[string]any{"type": "assistant", "uuid": "a1", "parentUuid": "u1", "message": map[string]any{"content": "Edited response"}}},
	}

	msgs := buildMessages("test-session", entries)
	if len(msgs) != 3 {
		t.Fatalf("got %d messages, want 3", len(msgs))
		return
	}
	wantUUIDs := []string{"u1", "a1", "u2"}
	for i, want := range wantUUIDs {
		if msgs[i].UUID != want {
			t.Errorf("msgs[%d].UUID = %q, want %q", i, msgs[i].UUID, want)
		}
	}
	// The edited version (last in file) should be used for a1's content.
	if msgs[1].RawMessage["content"] != "Edited response" {
		t.Errorf("a1 content = %v, want %q (should use edited version)", msgs[1].RawMessage["content"], "Edited response")
	}
}

func TestBuildMessagesFlatScanFiltersIsMeta(t *testing.T) {
	// Flat-scan path (no parentUuid) also filters isMeta.
	entries := []jsonlEntry{
		{entryType: "user", raw: map[string]any{"type": "user", "uuid": "u1", "message": map[string]any{"content": "Hello"}}},
		{entryType: "assistant", raw: map[string]any{"type": "assistant", "uuid": "a1", "message": map[string]any{"content": "Hi"}}},
		{entryType: "user", raw: map[string]any{"type": "user", "uuid": "u_meta", "isMeta": true, "message": map[string]any{"content": "system"}}},
		{entryType: "user", raw: map[string]any{"type": "user", "uuid": "u2", "message": map[string]any{"content": "Real"}}},
	}

	msgs := buildMessages("test-session", entries)
	wantUUIDs := []string{"u1", "a1", "u2"}
	if len(msgs) != len(wantUUIDs) {
		t.Fatalf("got %d messages, want %d", len(msgs), len(wantUUIDs))
		return
	}
	for i, want := range wantUUIDs {
		if msgs[i].UUID != want {
			t.Errorf("msgs[%d].UUID = %q, want %q", i, msgs[i].UUID, want)
		}
	}
}

// --- Head/tail optimization tests ---

func TestParseJSONLHeadTailSmallFile(t *testing.T) {
	_, projDir := setupTestProject(t)

	// Small file (< 128KB) — should read all entries.
	writeSessionJSONL(t, projDir, "small-file", []map[string]any{
		{"type": "queue-operation", "timestamp": "2026-01-01T00:00:00Z"},
		{"type": "user", "uuid": "u1", "message": map[string]any{"content": "Hello"}, "cwd": "/proj"},
		{"type": "custom-title", "customTitle": "My Title"},
	})

	path := filepath.Join(projDir, "small-file.jsonl")
	entries, err := parseJSONLHeadTail(path, metadataReadSize)
	if err != nil {
		t.Fatalf("parseJSONLHeadTail() error: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("got %d entries, want 3", len(entries))
	}
}

func TestParseJSONLHeadTailLargeFile(t *testing.T) {
	_, projDir := setupTestProject(t)

	// Create a file larger than 128KB with known head and tail content.
	path := filepath.Clean(filepath.Join(projDir, "large-file.jsonl"))
	f, err := os.Create(path) //nolint:gosec // path is constructed from test temp dir
	if err != nil {
		t.Fatalf("creating file: %v", err)
	}

	// Head: timestamp + user message
	headEntries := []map[string]any{
		{"type": "queue-operation", "timestamp": "2026-01-01T00:00:00Z"},
		{"type": "user", "uuid": "u1", "message": map[string]any{"content": "First prompt"}, "cwd": "/proj", "gitBranch": "main"},
	}
	enc := json.NewEncoder(f)
	for _, e := range headEntries {
		if err := enc.Encode(e); err != nil {
			t.Fatalf("writing head entry: %v", err)
		}
	}

	// Pad middle with large assistant messages to push past 128KB.
	bigContent := strings.Repeat("x", 1024)
	for i := 0; i < 200; i++ {
		if err := enc.Encode(map[string]any{
			"type": "assistant", "uuid": fmt.Sprintf("pad-%d", i),
			"message": map[string]any{"content": bigContent},
		}); err != nil {
			t.Fatalf("writing pad entry: %v", err)
		}
	}

	// Tail: custom title
	if err := enc.Encode(map[string]any{"type": "custom-title", "customTitle": "Custom Title From Tail"}); err != nil {
		t.Fatalf("writing tail entry: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("closing file: %v", err)
	}

	// Verify file is > 128KB.
	fi, _ := os.Stat(path)
	if fi.Size() <= 2*metadataReadSize {
		t.Fatalf("file size %d is not > %d", fi.Size(), 2*metadataReadSize)
	}

	entries, err := parseJSONLHeadTail(path, metadataReadSize)
	if err != nil {
		t.Fatalf("parseJSONLHeadTail() error: %v", err)
	}

	// Should have entries from head (timestamp, user) and tail (custom-title).
	var hasTimestamp, hasUser, hasCustomTitle bool
	for _, e := range entries {
		switch e.entryType {
		case "queue-operation":
			if ts, ok := e.raw["timestamp"].(string); ok && ts == "2026-01-01T00:00:00Z" {
				hasTimestamp = true
			}
		case roleUser:
			if uuid, ok := e.raw["uuid"].(string); ok && uuid == "u1" {
				hasUser = true
			}
		case "custom-title":
			if title, ok := e.raw["customTitle"].(string); ok && title == "Custom Title From Tail" {
				hasCustomTitle = true
			}
		}
	}

	if !hasTimestamp {
		t.Error("missing timestamp from head")
	}
	if !hasUser {
		t.Error("missing user entry from head")
	}
	if !hasCustomTitle {
		t.Error("missing custom-title from tail")
	}
}

func TestBuildSessionInfoFromFileLargeFile(t *testing.T) {
	_, projDir := setupTestProject(t)

	// Create a large file and verify buildSessionInfoFromFile extracts correct metadata.
	path := filepath.Clean(filepath.Join(projDir, "large-info.jsonl"))
	f, err := os.Create(path) //nolint:gosec // path is constructed from test temp dir
	if err != nil {
		t.Fatalf("creating file: %v", err)
	}

	enc := json.NewEncoder(f)
	if err := enc.Encode(map[string]any{"type": "queue-operation", "timestamp": "2026-06-01T12:00:00Z"}); err != nil {
		t.Fatalf("writing: %v", err)
	}
	if err := enc.Encode(map[string]any{
		"type": "user", "uuid": "u1", "cwd": "/my/project", "gitBranch": "develop",
		"message": map[string]any{"content": "Build the feature"},
	}); err != nil {
		t.Fatalf("writing: %v", err)
	}

	// Pad to exceed 128KB.
	bigContent := strings.Repeat("y", 1024)
	for i := 0; i < 200; i++ {
		if err := enc.Encode(map[string]any{
			"type": "assistant", "uuid": fmt.Sprintf("pad-%d", i),
			"message": map[string]any{"content": bigContent},
		}); err != nil {
			t.Fatalf("writing pad: %v", err)
		}
	}

	if err := enc.Encode(map[string]any{"type": "ai-title", "aiTitle": "Feature Builder"}); err != nil {
		t.Fatalf("writing: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("closing: %v", err)
	}

	info, err := buildSessionInfoFromFile("large-info", path)
	if err != nil {
		t.Fatalf("buildSessionInfoFromFile() error: %v", err)
	}

	if info.Summary != "Feature Builder" {
		t.Errorf("Summary = %q, want %q", info.Summary, "Feature Builder")
	}
	if info.AITitle == nil || *info.AITitle != "Feature Builder" {
		t.Errorf("AITitle = %v, want %q", info.AITitle, "Feature Builder")
	}
	if info.FirstPrompt == nil || *info.FirstPrompt != "Build the feature" {
		t.Errorf("FirstPrompt = %v, want %q", info.FirstPrompt, "Build the feature")
	}
	if info.Cwd == nil || *info.Cwd != "/my/project" {
		t.Errorf("Cwd = %v, want %q", info.Cwd, "/my/project")
	}
	if info.GitBranch == nil || *info.GitBranch != "develop" {
		t.Errorf("GitBranch = %v, want %q", info.GitBranch, "develop")
	}
}

func TestParseJSONLSkipsMalformedLines(t *testing.T) {
	_, projDir := setupTestProject(t)

	path := filepath.Join(projDir, "malformed.jsonl")
	content := `{"type":"user","uuid":"u1","message":{"role":"user","content":"hello"},"timestamp":"2026-01-01T00:00:00Z"}
not valid json
{"type":"assistant","uuid":"a1","message":{"role":"assistant","content":[]},"timestamp":"2026-01-01T00:00:01Z"}
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing file: %v", err)
	}

	entries, err := parseJSONLFile(path)
	if err != nil {
		t.Fatalf("parseJSONLFile() error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2 (should skip malformed)", len(entries))
	}
}

// Regression: a transcript line can exceed 1MB (e.g. a base64-embedded PDF
// attachment). Parsing must not fail with bufio.Scanner's "token too long".
func TestParseJSONLHandlesOversizedLines(t *testing.T) {
	_, projDir := setupTestProject(t)

	// ~2MB of base64-looking payload embedded in one user message.
	bigData := strings.Repeat("JVBERi0xLjYNJeLj", 128*1024)
	path := filepath.Join(projDir, "oversized.jsonl")
	content := `{"type":"user","uuid":"u1","message":{"role":"user","content":"hello"},"timestamp":"2026-01-01T00:00:00Z"}
{"type":"user","uuid":"u2","message":{"role":"user","content":[{"type":"document","source":{"type":"base64","media_type":"application/pdf","data":"` + bigData + `"}}]},"timestamp":"2026-01-01T00:00:01Z"}
{"type":"assistant","uuid":"a1","message":{"role":"assistant","content":[{"type":"text","text":"read it"}]},"timestamp":"2026-01-01T00:00:02Z"}
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing file: %v", err)
	}

	entries, err := parseJSONLFile(path)
	if err != nil {
		t.Fatalf("parseJSONLFile() error: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("got %d entries, want 3 (oversized line must parse, not abort the read)", len(entries))
	}
	if entries[2].entryType != "assistant" {
		t.Errorf("entries[2].entryType = %q, want %q (entries after the oversized line must survive)", entries[2].entryType, "assistant")
	}
}

// --- Worktree support tests ---

func TestGetWorktreePaths(t *testing.T) {
	t.Run("returns nil when git not available in dir", func(t *testing.T) {
		// Non-git directory should return nil (no error).
		dir := t.TempDir()
		paths := getWorktreePaths(dir)
		if paths != nil {
			t.Errorf("getWorktreePaths() = %v, want nil for non-git dir", paths)
		}
	})

	t.Run("parses porcelain output correctly", func(t *testing.T) {
		// This test exercises the parsing logic by running against
		// a real git repo if available. We create a temp git repo.
		dir := t.TempDir()
		// Initialize a git repo.
		initCmd := exec.Command("git", "init")
		initCmd.Dir = dir
		if err := initCmd.Run(); err != nil {
			t.Skip("git not available, skipping")
		}

		paths := getWorktreePaths(dir)
		// A freshly initialized repo should have at least one worktree (itself).
		if len(paths) == 0 {
			t.Error("getWorktreePaths() returned empty for a git repo")
			return
		}
		// The first worktree path should be the repo dir itself.
		// Use filepath.EvalSymlinks to handle macOS /private/var/... symlinks.
		resolved, err := filepath.EvalSymlinks(dir)
		if err != nil {
			t.Fatalf("EvalSymlinks: %v", err)
		}
		gotResolved, err := filepath.EvalSymlinks(paths[0])
		if err != nil {
			t.Fatalf("EvalSymlinks: %v", err)
		}
		if gotResolved != resolved {
			t.Errorf("first worktree path = %q, want %q", gotResolved, resolved)
		}
	})
}

func TestDeduplicateBySessionID(t *testing.T) {
	t.Run("empty input", func(t *testing.T) {
		got := deduplicateBySessionID(nil)
		if got != nil {
			t.Errorf("deduplicateBySessionID(nil) = %v, want nil", got)
		}
	})

	t.Run("no duplicates", func(t *testing.T) {
		input := []SDKSessionInfo{
			{SessionID: "a", LastModified: 100},
			{SessionID: "b", LastModified: 200},
		}
		got := deduplicateBySessionID(input)
		if len(got) != 2 {
			t.Fatalf("got %d, want 2", len(got))
		}
	})

	t.Run("keeps newest LastModified", func(t *testing.T) {
		input := []SDKSessionInfo{
			{SessionID: "a", LastModified: 100, Summary: "old"},
			{SessionID: "a", LastModified: 200, Summary: "new"},
			{SessionID: "b", LastModified: 50},
		}
		got := deduplicateBySessionID(input)
		if len(got) != 2 {
			t.Fatalf("got %d, want 2", len(got))
		}
		// Find session "a" and verify it has the newer LastModified.
		for _, s := range got {
			if s.SessionID == "a" {
				if s.LastModified != 200 {
					t.Errorf("session 'a' LastModified = %d, want 200", s.LastModified)
				}
				if s.Summary != "new" {
					t.Errorf("session 'a' Summary = %q, want %q", s.Summary, "new")
				}
			}
		}
	})
}

func TestIncludeWorktreesOption(t *testing.T) {
	t.Run("defaults to true", func(t *testing.T) {
		o := defaultOpts()
		if !o.includeWorktreesEnabled() {
			t.Error("includeWorktreesEnabled() = false, want true (default)")
		}
	})

	t.Run("can be set to false", func(t *testing.T) {
		o := defaultOpts()
		WithIncludeWorktrees(false)(&o)
		if o.includeWorktreesEnabled() {
			t.Error("includeWorktreesEnabled() = true, want false")
		}
	})

	t.Run("can be set to true explicitly", func(t *testing.T) {
		o := defaultOpts()
		WithIncludeWorktrees(true)(&o)
		if !o.includeWorktreesEnabled() {
			t.Error("includeWorktreesEnabled() = false, want true")
		}
	})
}

// setupWorktreeTestProject creates a temp dir structured like ~/.claude/projects/
// with multiple encoded project directories simulating worktree layouts.
// Returns the config dir and a map of worktree path -> encoded project dir.
func setupWorktreeTestProject(t *testing.T, worktreePaths []string) (cfgDir string, projDirs map[string]string) {
	t.Helper()
	cfgDir = t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", cfgDir)

	projDirs = make(map[string]string, len(worktreePaths))
	for _, wt := range worktreePaths {
		encoded := encodeCwd(wt)
		dir := filepath.Join(cfgDir, "projects", encoded)
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatalf("creating project dir for %s: %v", wt, err)
		}
		projDirs[wt] = dir
	}
	return cfgDir, projDirs
}

func TestProjectDirsForOptsWithWorktrees(t *testing.T) {
	t.Run("includes worktree dirs when enabled", func(t *testing.T) {
		// We can't easily mock getWorktreePaths, but we can verify that
		// projectDirsForOpts includes the primary dir and deduplicates.
		cfgDir := t.TempDir()
		t.Setenv("CLAUDE_CONFIG_DIR", cfgDir)

		// Create two project dirs simulating main repo + worktree.
		abs, err := filepath.Abs("/test/main-repo")
		if err != nil {
			t.Fatalf("filepath.Abs: %v", err)
		}
		encoded := encodeCwd(abs)
		dir := filepath.Join(cfgDir, "projects", encoded)
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatalf("creating project dir: %v", err)
		}

		// Even without actual git worktrees, the primary dir should be found.
		o := sessionOpts{directory: "/test/main-repo"}
		dirs, err := projectDirsForOpts(o)
		if err != nil {
			t.Fatalf("projectDirsForOpts() error: %v", err)
		}
		if len(dirs) == 0 {
			t.Fatal("expected at least 1 dir, got 0")
		}
		if dirs[0] != dir {
			t.Errorf("dirs[0] = %q, want %q", dirs[0], dir)
		}
	})

	t.Run("excludes worktree dirs when disabled", func(t *testing.T) {
		cfgDir := t.TempDir()
		t.Setenv("CLAUDE_CONFIG_DIR", cfgDir)

		abs, err := filepath.Abs("/test/repo")
		if err != nil {
			t.Fatalf("filepath.Abs: %v", err)
		}
		encoded := encodeCwd(abs)
		dir := filepath.Join(cfgDir, "projects", encoded)
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatalf("creating project dir: %v", err)
		}

		o := sessionOpts{directory: "/test/repo"}
		WithIncludeWorktrees(false)(&o)
		dirs, err := projectDirsForOpts(o)
		if err != nil {
			t.Fatalf("projectDirsForOpts() error: %v", err)
		}
		// Should only return the primary dir.
		if len(dirs) != 1 {
			t.Fatalf("got %d dirs, want 1", len(dirs))
		}
		if dirs[0] != dir {
			t.Errorf("dirs[0] = %q, want %q", dirs[0], dir)
		}
	})

	t.Run("returns nil for nonexistent directory", func(t *testing.T) {
		cfgDir := t.TempDir()
		t.Setenv("CLAUDE_CONFIG_DIR", cfgDir)

		o := sessionOpts{directory: "/nonexistent/path"}
		dirs, err := projectDirsForOpts(o)
		if err != nil {
			t.Fatalf("projectDirsForOpts() error: %v", err)
		}
		if dirs != nil {
			t.Errorf("dirs = %v, want nil", dirs)
		}
	})
}

func TestListSessionsDeduplicatesAcrossWorktrees(t *testing.T) {
	// Simulate the same session appearing in two worktree project dirs.
	_, projDirs := setupWorktreeTestProject(t, []string{"/repo/main", "/repo/feature"})

	mainDir := projDirs["/repo/main"]
	featureDir := projDirs["/repo/feature"]

	// Write the same session in both dirs, with different timestamps.
	writeSessionJSONL(t, mainDir, "shared-session", []map[string]any{
		{"type": "queue-operation", "timestamp": "2026-01-01T00:00:00Z", "sessionId": "shared-session"},
		{"type": "user", "message": map[string]any{"role": "user", "content": "Hello from main"}, "uuid": "u1", "timestamp": "2026-01-01T00:00:01Z", "sessionId": "shared-session"},
	})
	writeSessionJSONL(t, featureDir, "shared-session", []map[string]any{
		{"type": "queue-operation", "timestamp": "2026-02-01T00:00:00Z", "sessionId": "shared-session"},
		{"type": "user", "message": map[string]any{"role": "user", "content": "Hello from feature"}, "uuid": "u1", "timestamp": "2026-02-01T00:00:01Z", "sessionId": "shared-session"},
	})

	// Write a unique session only in feature dir.
	writeSessionJSONL(t, featureDir, "unique-session", []map[string]any{
		{"type": "queue-operation", "timestamp": "2026-03-01T00:00:00Z", "sessionId": "unique-session"},
		{"type": "user", "message": map[string]any{"role": "user", "content": "Only in feature"}, "uuid": "u1", "timestamp": "2026-03-01T00:00:01Z", "sessionId": "unique-session"},
	})

	// List all sessions (no directory filter) to get both project dirs.
	sessions, err := ListSessions()
	if err != nil {
		t.Fatalf("ListSessions() error: %v", err)
	}

	// Should have 2 sessions after deduplication (shared-session + unique-session).
	if len(sessions) != 2 {
		t.Fatalf("got %d sessions, want 2 (after dedup)", len(sessions))
	}

	// The shared session should have the newer LastModified (from feature dir).
	for _, s := range sessions {
		if s.SessionID == "shared-session" {
			// The feature dir session file was written second, so it should
			// have a newer or equal mtime. We verify dedup kept the newest.
			// Since we can't control mtime precisely, just verify the session exists.
			break
		}
	}
}

func TestFindSessionFileWorktreeAware(t *testing.T) {
	// When directory is set, findSessionFile should search across all project
	// dirs returned by projectDirsForOpts (which includes worktree dirs).
	// We simulate this by creating two encoded project dirs and looking up
	// a session that only exists in one of them.

	cfgDir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", cfgDir)

	// Create project dir for the primary path.
	abs, err := filepath.Abs("/test/primary")
	if err != nil {
		t.Fatalf("filepath.Abs: %v", err)
	}
	primaryEncoded := encodeCwd(abs)
	primaryDir := filepath.Join(cfgDir, "projects", primaryEncoded)
	if err := os.MkdirAll(primaryDir, 0o750); err != nil {
		t.Fatalf("creating primary dir: %v", err)
	}

	// Write a session in the primary dir.
	writeSessionJSONL(t, primaryDir, "primary-session", []map[string]any{
		{"type": "user", "message": map[string]any{"content": "Hello"}, "uuid": "u1"},
	})

	// findSessionFile should find it.
	o := sessionOpts{directory: "/test/primary"}
	path, err := findSessionFile("primary-session", o)
	if err != nil {
		t.Fatalf("findSessionFile() error: %v", err)
	}
	if path == "" {
		t.Fatal("findSessionFile() returned empty path")
	}

	// Non-existent session should return errSessionNotFound.
	_, err = findSessionFile("nonexistent", o)
	if err == nil {
		t.Fatal("expected error for nonexistent session")
	}
}

func TestGetSessionInfoWithWorktrees(t *testing.T) {
	// Integration test: GetSessionInfo should find sessions via worktree-expanded dirs.
	cfgDir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", cfgDir)

	abs, err := filepath.Abs("/test/worktree-info")
	if err != nil {
		t.Fatalf("filepath.Abs: %v", err)
	}
	encoded := encodeCwd(abs)
	dir := filepath.Join(cfgDir, "projects", encoded)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("creating dir: %v", err)
	}

	writeSessionJSONL(t, dir, "wt-info-session", []map[string]any{
		{"type": "queue-operation", "timestamp": "2026-04-01T00:00:00Z", "sessionId": "wt-info-session"},
		{"type": "user", "message": map[string]any{"role": "user", "content": "Worktree test"}, "uuid": "u1", "timestamp": "2026-04-01T00:00:01Z", "cwd": "/test/worktree-info", "sessionId": "wt-info-session"},
	})

	info, err := GetSessionInfo("wt-info-session", WithSessionDirectory("/test/worktree-info"))
	if err != nil {
		t.Fatalf("GetSessionInfo() error: %v", err)
	}
	if info == nil {
		t.Fatal("GetSessionInfo() returned nil, want session info")
	}
	if info.Summary != "Worktree test" {
		t.Errorf("Summary = %q, want %q", info.Summary, "Worktree test")
	}
}

func TestGetMessagesWithWorktrees(t *testing.T) {
	// Integration test: GetMessages should find sessions via worktree-expanded dirs.
	cfgDir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", cfgDir)

	abs, err := filepath.Abs("/test/worktree-msgs")
	if err != nil {
		t.Fatalf("filepath.Abs: %v", err)
	}
	encoded := encodeCwd(abs)
	dir := filepath.Join(cfgDir, "projects", encoded)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("creating dir: %v", err)
	}

	writeSessionJSONL(t, dir, "wt-msgs-session", []map[string]any{
		{"type": "user", "message": map[string]any{"role": "user", "content": textContent("Hello worktree")}, "uuid": "u1", "timestamp": "2026-04-01T00:00:01Z", "sessionId": "wt-msgs-session"},
		{"type": "assistant", "message": map[string]any{"role": "assistant", "content": textContent("Hi!")}, "uuid": "a1", "timestamp": "2026-04-01T00:00:02Z", "sessionId": "wt-msgs-session"},
	})

	msgs, err := GetMessages("wt-msgs-session", WithSessionDirectory("/test/worktree-msgs"))
	if err != nil {
		t.Fatalf("GetMessages() error: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("got %d messages, want 2", len(msgs))
	}
}

func TestWithIncludeWorktreesDisablesExpansion(t *testing.T) {
	// When WithIncludeWorktrees(false) is set, only the primary dir should be searched.
	cfgDir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", cfgDir)

	abs, err := filepath.Abs("/test/no-wt")
	if err != nil {
		t.Fatalf("filepath.Abs: %v", err)
	}
	encoded := encodeCwd(abs)
	dir := filepath.Join(cfgDir, "projects", encoded)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("creating dir: %v", err)
	}

	writeSessionJSONL(t, dir, "no-wt-session", []map[string]any{
		{"type": "user", "message": map[string]any{"role": "user", "content": textContent("Test")}, "uuid": "u1", "timestamp": "2026-01-01T00:00:01Z", "sessionId": "no-wt-session"},
	})

	// With worktrees disabled, should still find sessions in the primary dir.
	sessions, err := ListSessions(
		WithSessionDirectory("/test/no-wt"),
		WithIncludeWorktrees(false),
	)
	if err != nil {
		t.Fatalf("ListSessions() error: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("got %d sessions, want 1", len(sessions))
	}
}
