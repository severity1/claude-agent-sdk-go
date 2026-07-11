// Package session provides functions for reading Claude Code session data from disk.
//
// Sessions are stored as JSONL files at ~/.claude/projects/<encoded-cwd>/<session-id>.jsonl.
// The encoded-cwd replaces every non-alphanumeric character with "-".
package session

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// errSessionNotFound is a sentinel error returned by findSessionFile when
// the session JSONL file does not exist in any project directory.
var errSessionNotFound = errors.New("session not found")

// SDKSessionInfo holds metadata about a session.
type SDKSessionInfo struct {
	SessionID    string  `json:"session_id"`
	Summary      string  `json:"summary"`
	LastModified int64   `json:"last_modified"`
	FileSize     *int64  `json:"file_size,omitempty"`
	CustomTitle  *string `json:"custom_title,omitempty"`
	AITitle      *string `json:"ai_title,omitempty"`
	FirstPrompt  *string `json:"first_prompt,omitempty"`
	GitBranch    *string `json:"git_branch,omitempty"`
	Cwd          *string `json:"cwd,omitempty"`
	Tag          *string `json:"tag,omitempty"`
	CreatedAt    *int64  `json:"created_at,omitempty"`
}

// ContentType discriminates the MessageContent union.
type ContentType int

const (
	// ContentTypeString indicates the message content is a plain string.
	ContentTypeString ContentType = iota + 1
	// ContentTypeBlocks indicates the message content is an array of content blocks.
	ContentTypeBlocks
)

// MessageContent is a sum type representing the content of a session message.
// Kind indicates which field is populated.
type MessageContent struct {
	Kind   ContentType
	String string         // populated when Kind == ContentTypeString
	Blocks []ContentBlock // populated when Kind == ContentTypeBlocks
}

// Block type constants for ContentBlock.Type.
const (
	BlockTypeText                         = "text"
	BlockTypeThinking                     = "thinking"
	BlockTypeRedactedThinking             = "redacted_thinking"
	BlockTypeToolUse                      = "tool_use"
	BlockTypeServerToolUse                = "server_tool_use"
	BlockTypeToolResult                   = "tool_result"
	BlockTypeImage                        = "image"
	BlockTypeWebSearchToolResult          = "web_search_tool_result"
	BlockTypeWebFetchToolResult           = "web_fetch_tool_result"
	BlockTypeCodeExecutionToolResult      = "code_execution_tool_result"
	BlockTypeBashCodeExecutionToolResult  = "bash_code_execution_tool_result"
	BlockTypeTextEditorCodeExecToolResult = "text_editor_code_execution_tool_result"
	BlockTypeToolSearchToolResult         = "tool_search_tool_result"
	BlockTypeContainerUpload              = "container_upload"
)

// ContentBlock represents a typed content block from a session message.
// The Type field discriminates the variant. Unknown types are preserved in Raw.
type ContentBlock struct {
	// Type discriminates the block variant.
	// Use the BlockType* constants to compare against known types.
	Type string `json:"type"`

	// Raw holds the full original map for all block types (always populated).
	Raw map[string]any `json:"-"`

	// text
	Text string `json:"text,omitempty"`

	// thinking
	Thinking  string `json:"thinking,omitempty"`
	Signature string `json:"signature,omitempty"`

	// redacted_thinking
	Data string `json:"data,omitempty"`

	// tool_use, server_tool_use
	ID    string         `json:"id,omitempty"`
	Name  string         `json:"name,omitempty"`
	Input map[string]any `json:"input,omitempty"`

	// tool_result
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   any    `json:"content,omitempty"` // string or nested blocks
	IsError   *bool  `json:"is_error,omitempty"`

	// image
	Source map[string]any `json:"source,omitempty"`
}

// Message represents a message from a session transcript.
type Message struct {
	Type      string `json:"type"` // "user", "assistant", etc. (there are many other types beyond just these two)
	UUID      string `json:"uuid"`
	SessionID string `json:"session_id"`
	// *should* be true for system-injected messages,
	// but nothing in claude api enforces it,
	// so some implementations like claude-vscode inject messages without this.
	IsMeta          bool            `json:"is_meta"`
	RawMessage      map[string]any  `json:"message"`                      // raw message data
	Content         *MessageContent `json:"-"`                            // parsed content
	ParentToolUseID *string         `json:"parent_tool_use_id,omitempty"` // reserved
}

// Option configures session query behavior.
type Option func(*sessionOpts)

type sessionOpts struct {
	directory        string
	limit            int
	offset           int
	includeWorktrees *bool // nil means default (true)
}

func defaultOpts() sessionOpts {
	return sessionOpts{}
}

// includeWorktreesEnabled returns whether worktree scanning is enabled.
// Defaults to true when not explicitly set.
func (o sessionOpts) includeWorktreesEnabled() bool {
	if o.includeWorktrees == nil {
		return true
	}
	return *o.includeWorktrees
}

// WithSessionDirectory scopes the query to a specific project directory.
// When omitted, sessions across all projects are searched.
func WithSessionDirectory(dir string) Option {
	return func(o *sessionOpts) {
		o.directory = dir
	}
}

// WithSessionLimit sets the maximum number of results to return.
func WithSessionLimit(n int) Option {
	return func(o *sessionOpts) {
		o.limit = n
	}
}

// WithSessionOffset skips the first n messages (GetMessages only).
func WithSessionOffset(n int) Option {
	return func(o *sessionOpts) {
		o.offset = n
	}
}

// WithIncludeWorktrees controls whether git worktree directories are included
// when searching for sessions. Defaults to true. Only has effect when a
// directory is specified via WithSessionDirectory.
func WithIncludeWorktrees(include bool) Option {
	return func(o *sessionOpts) {
		o.includeWorktrees = &include
	}
}

// ListSessions returns metadata for sessions, sorted by LastModified descending.
// Unreadable project directories and individual session files are silently skipped
// to provide best-effort results (matches the Python SDK's behavior).
func ListSessions(opts ...Option) ([]SDKSessionInfo, error) {
	o := defaultOpts()
	for _, fn := range opts {
		fn(&o)
	}

	dirs, err := projectDirsForOpts(o)
	if err != nil {
		return nil, err
	}

	var sessions []SDKSessionInfo
	for _, dir := range dirs {
		infos, err := listSessionsInDir(dir)
		if err != nil {
			continue // skip unreadable directories
		}
		sessions = append(sessions, infos...)
	}

	// Deduplicate sessions that appear in multiple worktree project dirs.
	sessions = deduplicateBySessionID(sessions)

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].LastModified > sessions[j].LastModified
	})

	if o.limit > 0 && len(sessions) > o.limit {
		sessions = sessions[:o.limit]
	}

	return sessions, nil
}

// GetMessages reads user and assistant messages from a session transcript.
func GetMessages(sessionID string, opts ...Option) ([]Message, error) {
	o := defaultOpts()
	for _, fn := range opts {
		fn(&o)
	}

	path, err := findSessionFile(sessionID, o)
	if err != nil {
		return nil, err
	}

	entries, err := parseJSONLFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading session %s: %w", sessionID, err)
	}

	messages := buildMessages(sessionID, entries)

	if o.offset > 0 {
		if o.offset >= len(messages) {
			return nil, nil
		}
		messages = messages[o.offset:]
	}

	if o.limit > 0 && len(messages) > o.limit {
		messages = messages[:o.limit]
	}

	return messages, nil
}

// GetSessionInfo returns metadata for a single session by ID.
// Returns nil (not an error) if the session is not found.
func GetSessionInfo(sessionID string, opts ...Option) (*SDKSessionInfo, error) {
	o := defaultOpts()
	for _, fn := range opts {
		fn(&o)
	}

	path, err := findSessionFile(sessionID, o)
	if errors.Is(err, errSessionNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return buildSessionInfoFromFile(sessionID, path)
}

// configDir returns the Claude configuration directory.
func configDir() (string, error) {
	if dir := os.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".claude"), nil
}

// worktreeTimeout is the maximum time to wait for `git worktree list --porcelain`.
const worktreeTimeout = 5 * time.Second

// getWorktreePaths runs `git worktree list --porcelain` in the given directory
// and returns all worktree paths. Returns an empty slice (not an error) if git
// is not available, the directory is not a git repo, or the command fails.
// Matches the Python SDK's _get_worktree_paths.
func getWorktreePaths(dir string) []string {
	ctx, cancel := context.WithTimeout(context.Background(), worktreeTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "worktree", "list", "--porcelain")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	var paths []string
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "worktree ") {
			p := strings.TrimPrefix(line, "worktree ")
			if p != "" {
				paths = append(paths, p)
			}
		}
	}
	return paths
}

// encodeCwd encodes a directory path by replacing non-alphanumeric characters with "-".
func encodeCwd(cwd string) string {
	var b strings.Builder
	b.Grow(len(cwd))
	for _, r := range cwd {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	return b.String()
}

// projectDirsForOpts returns the project directories to search based on options.
// When a directory is specified and includeWorktrees is enabled, it also includes
// project directories for all git worktree paths.
func projectDirsForOpts(o sessionOpts) ([]string, error) {
	cfgDir, err := configDir()
	if err != nil {
		return nil, err
	}
	projectsDir := filepath.Join(cfgDir, "projects")

	if o.directory != "" {
		abs, err := filepath.Abs(o.directory)
		if err != nil {
			return nil, fmt.Errorf("resolving directory: %w", err)
		}

		// Collect all candidate directories: user's dir first, then worktrees.
		candidatePaths := []string{abs}
		if o.includeWorktreesEnabled() {
			candidatePaths = append(candidatePaths, getWorktreePaths(abs)...)
		}

		// Encode each candidate path and collect existing project dirs.
		seen := make(map[string]bool)
		var dirs []string
		for _, p := range candidatePaths {
			encoded := encodeCwd(p)
			dir := filepath.Join(projectsDir, encoded)
			if seen[dir] {
				continue
			}
			seen[dir] = true
			if _, err := os.Stat(dir); err == nil {
				dirs = append(dirs, dir)
			}
		}
		if len(dirs) == 0 {
			return nil, nil
		}
		return dirs, nil
	}

	// List all project directories.
	names, err := readDirNames(projectsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading projects directory: %w", err)
	}

	var dirs []string
	for _, name := range names {
		full := filepath.Join(projectsDir, name)
		fi, err := os.Stat(full)
		if err != nil {
			continue
		}
		if fi.IsDir() {
			dirs = append(dirs, full)
		}
	}
	return dirs, nil
}

// readDirNames is a replacement for os.ReadDir which is vulnerable to GO-2026-4602.
// See https://pkg.go.dev/vuln/GO-2026-4602. Remove once upgraded to go1.26.0 or later.
func readDirNames(path string) (names []string, err error) {
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("closing directory: %w", cerr)
		}
	}()
	return f.Readdirnames(-1)
}

// deduplicateBySessionID deduplicates sessions by SessionID, keeping the entry
// with the newest LastModified value. Matches the Python SDK's _deduplicate_by_session_id.
func deduplicateBySessionID(sessions []SDKSessionInfo) []SDKSessionInfo {
	if len(sessions) == 0 {
		return sessions
	}
	best := make(map[string]SDKSessionInfo, len(sessions))
	for _, s := range sessions {
		if existing, ok := best[s.SessionID]; !ok || s.LastModified > existing.LastModified {
			best[s.SessionID] = s
		}
	}
	deduped := make([]SDKSessionInfo, 0, len(best))
	for _, s := range best {
		deduped = append(deduped, s)
	}
	return deduped
}

// listSessionsInDir lists all sessions in a single project directory.
// Individual session files that fail to parse (corrupt JSONL, permission errors)
// are silently skipped to provide best-effort results.
func listSessionsInDir(dir string) ([]SDKSessionInfo, error) {
	names, err := readDirNames(dir)
	if err != nil {
		return nil, err
	}

	var sessions []SDKSessionInfo
	for _, name := range names {
		if !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		sessionID := strings.TrimSuffix(name, ".jsonl")
		path := filepath.Join(dir, name)

		info, err := buildSessionInfoFromFile(sessionID, path)
		if err != nil {
			continue // skip unreadable sessions
		}
		sessions = append(sessions, *info)
	}
	return sessions, nil
}

// findSessionFile locates the JSONL file for a session ID.
func findSessionFile(sessionID string, o sessionOpts) (string, error) {
	dirs, err := projectDirsForOpts(o)
	if err != nil {
		return "", err
	}

	filename := sessionID + ".jsonl"
	for _, dir := range dirs {
		path := filepath.Join(dir, filename)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("%w: %s", errSessionNotFound, sessionID)
}

// metadataReadSize is the size of head/tail chunks for metadata extraction.
// Matches the Python SDK's LITE_READ_BUF_SIZE.
const metadataReadSize int64 = 64 * 1024

// buildSessionInfoFromFile builds SDKSessionInfo by reading a JSONL file.
// Uses head/tail reads for efficiency — only reads the first and last 64KB
// of the file rather than parsing the entire JSONL.
func buildSessionInfoFromFile(sessionID, path string) (*SDKSessionInfo, error) {
	path = filepath.Clean(path)
	fileInfo, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	entries, err := parseJSONLHeadTail(path, metadataReadSize)
	if err != nil {
		return nil, err
	}

	return buildSessionInfo(sessionID, entries, fileInfo), nil
}

// jsonlEntry represents a parsed line from a JSONL file.
type jsonlEntry struct {
	entryType string
	raw       map[string]any
}

// parseJSONLFile reads and parses all lines from a JSONL file.
func parseJSONLFile(path string) (entries []jsonlEntry, err error) {
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("closing session file: %w", cerr)
		}
	}()

	return parseJSONLFromReader(f)
}

// maxFirstPromptLen is the maximum length of the first prompt (truncated with ellipsis).
const maxFirstPromptLen = 200

// skipFirstPromptPattern matches auto-generated or system messages that should
// be skipped when extracting the first meaningful user prompt.
// Matches the Python SDK's _SKIP_FIRST_PROMPT_PATTERN.
// permalink:
// https://github.com/anthropics/claude-agent-sdk-python/blob/acd62ea8f7bba637f604e0bb09c21b86176ccb49/src/claude_agent_sdk/_internal/sessions.py#L40
var skipFirstPromptPattern = regexp.MustCompile(
	`^(?:<local-command-stdout>|<session-start-hook>|<tick>|<goal>|` +
		`\[Request interrupted by user[^\]]*\]|` +
		`\s*<ide_opened_file>[\s\S]*</ide_opened_file>\s*$|` +
		`\s*<ide_selection>[\s\S]*</ide_selection>\s*$)`,
)

// commandNamePattern matches <command-name>...</command-name> tags.
var commandNamePattern = regexp.MustCompile(`<command-name>(.*?)</command-name>`)

// extractFirstPrompt extracts the first meaningful user prompt from JSONL entries.
// Skips tool_result messages, isMeta, isCompactSummary, command-name messages,
// and auto-generated patterns. Truncates to maxFirstPromptLen characters.
func extractFirstPrompt(entries []jsonlEntry) *string {
	for _, e := range entries {
		if e.entryType != entryTypeUser {
			continue
		}
		if meta, ok := e.raw["isMeta"].(bool); ok && meta {
			continue
		}
		if cs, ok := e.raw["isCompactSummary"].(bool); ok && cs {
			continue
		}

		rawMsg, ok := e.raw["message"].(map[string]any)
		if !ok {
			continue
		}

		text := extractTextFromMessage(rawMsg)
		if text == "" {
			continue
		}

		// Skip command-name messages.
		if commandNamePattern.MatchString(text) {
			continue
		}

		// Skip auto-generated patterns.
		if skipFirstPromptPattern.MatchString(text) {
			continue
		}

		// Collapse newlines and trim.
		text = strings.Join(strings.Fields(text), " ")
		if text == "" {
			continue
		}

		if len(text) > maxFirstPromptLen {
			text = text[:maxFirstPromptLen]
		}
		return &text
	}
	return nil
}

// extractTextFromMessage returns the text content of a message.
// For string content, returns it directly. For block content, returns
// the first text block's text. Returns "" if the content is only tool_result blocks.
func extractTextFromMessage(msg map[string]any) string {
	content, ok := msg["content"]
	if !ok {
		return ""
	}

	// String content.
	if s, ok := content.(string); ok {
		return s
	}

	// Block content — extract first text block, but skip if only tool_result.
	blocks, ok := content.([]any)
	if !ok {
		return ""
	}

	hasNonToolResult := false
	var firstText string
	for _, block := range blocks {
		b, ok := block.(map[string]any)
		if !ok {
			continue
		}
		typ, _ := b["type"].(string)
		if typ != "tool_result" {
			hasNonToolResult = true
		}
		if typ == "text" && firstText == "" {
			if t, ok := b["text"].(string); ok {
				firstText = t
			}
		}
	}

	if !hasNonToolResult {
		return "" // skip tool_result-only messages
	}
	return firstText
}

// parseJSONLHeadTail reads the first and last bufSize bytes of a JSONL file,
// parsing complete lines from each chunk. For files smaller than 2*bufSize,
// the entire file is read (equivalent to parseJSONLFile).
//
// This is used for metadata extraction (ListSessions, GetSessionInfo) where
// reading the full file is unnecessary — session metadata is in the head
// (timestamps, cwd, gitBranch, first_prompt) and tail (titles, tags).
func parseJSONLHeadTail(path string, bufSize int64) (entries []jsonlEntry, err error) {
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("closing session file: %w", cerr)
		}
	}()

	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	fileSize := fi.Size()

	// Small file — read everything.
	if fileSize <= 2*bufSize {
		return parseJSONLFromReader(f)
	}

	// Read head chunk.
	headBuf := make([]byte, bufSize)
	if _, err := io.ReadFull(f, headBuf); err != nil {
		return nil, fmt.Errorf("reading head: %w", err)
	}

	// Parse complete lines from head (discard last partial line).
	entries = parseLinesFromBytes(headBuf, true)

	// Read tail chunk.
	tailOffset := fileSize - bufSize
	if _, err := f.Seek(tailOffset, 0); err != nil {
		return nil, fmt.Errorf("seeking to tail: %w", err)
	}
	tailBuf := make([]byte, bufSize)
	if _, err := io.ReadFull(f, tailBuf); err != nil {
		return nil, fmt.Errorf("reading tail: %w", err)
	}

	// Parse complete lines from tail (discard first partial line).
	tailEntries := parseLinesFromBytes(tailBuf, false)
	entries = append(entries, tailEntries...)

	return entries, nil
}

// parseJSONLFromReader reads all lines from an already-opened file.
// Lines can be arbitrarily long (a base64-embedded document attachment
// exceeds bufio.Scanner's default-style caps), so read with bufio.Reader,
// which grows to fit each line instead of erroring.
func parseJSONLFromReader(f *os.File) ([]jsonlEntry, error) {
	r := bufio.NewReaderSize(f, 64*1024)

	var entries []jsonlEntry
	for {
		line, rerr := r.ReadBytes('\n')
		line = bytes.TrimRight(line, "\r\n")
		if len(line) > 0 {
			var raw map[string]any
			if err := json.Unmarshal(line, &raw); err == nil {
				typ, _ := raw["type"].(string)
				entries = append(entries, jsonlEntry{entryType: typ, raw: raw})
			}
			// skip malformed lines
		}
		if rerr != nil {
			if errors.Is(rerr, io.EOF) {
				return entries, nil
			}
			return nil, fmt.Errorf("scanning JSONL file: %w", rerr)
		}
	}
}

// parseLinesFromBytes parses complete JSONL lines from a byte buffer.
// If isHead is true, discards the last partial line (tail of head chunk).
// If isHead is false, discards the first partial line (start of tail chunk).
func parseLinesFromBytes(buf []byte, isHead bool) []jsonlEntry {
	var entries []jsonlEntry
	start := 0

	if !isHead {
		// Skip first partial line in tail chunk.
		idx := bytes.IndexByte(buf, '\n')
		if idx < 0 {
			return nil // no complete line
		}
		start = idx + 1
	}

	for start < len(buf) {
		end := start + bytes.IndexByte(buf[start:], '\n')
		if end < start {
			// No newline found — this is the last (potentially partial) line.
			if isHead {
				break // discard partial last line in head
			}
			end = len(buf)
		}

		line := buf[start:end]
		start = end + 1

		if len(line) == 0 {
			continue
		}
		var raw map[string]any
		if err := json.Unmarshal(line, &raw); err != nil {
			continue
		}
		typ, _ := raw["type"].(string)
		entries = append(entries, jsonlEntry{entryType: typ, raw: raw})
	}
	return entries
}

// extractMetadataFromEntries populates session metadata fields by scanning JSONL entries.
// Extracts titles, tags, cwd, gitBranch, and createdAt timestamp.
func extractMetadataFromEntries(info *SDKSessionInfo, entries []jsonlEntry) {
	for _, e := range entries {
		extractEntryMetadata(info, e)
	}
	info.FirstPrompt = extractFirstPrompt(entries)
}

// extractEntryMetadata extracts metadata from a single JSONL entry into info.
func extractEntryMetadata(info *SDKSessionInfo, e jsonlEntry) {
	switch e.entryType {
	case "custom-title":
		if title, ok := e.raw["customTitle"].(string); ok && title != "" {
			info.CustomTitle = &title
		}
	case "ai-title":
		if title, ok := e.raw["aiTitle"].(string); ok && title != "" {
			info.AITitle = &title
		}
	case "tag":
		if tag, ok := e.raw["tag"].(string); ok {
			if tag == "" {
				info.Tag = nil // cleared
			} else {
				info.Tag = &tag
			}
		}
	case entryTypeUser:
		// Track cwd and gitBranch (last one wins)
		if branch, ok := e.raw["gitBranch"].(string); ok && branch != "" {
			info.GitBranch = &branch
		}
		if cwd, ok := e.raw["cwd"].(string); ok && cwd != "" {
			info.Cwd = &cwd
		}
	}

	extractCreatedAt(info, e)
}

// extractCreatedAt sets CreatedAt from the first entry that carries a valid
// RFC3339 timestamp. Once set, subsequent calls are no-ops.
func extractCreatedAt(info *SDKSessionInfo, e jsonlEntry) {
	if info.CreatedAt != nil {
		return
	}
	ts, ok := e.raw["timestamp"].(string)
	if !ok || ts == "" {
		return
	}
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return
	}
	ms := t.UnixMilli()
	info.CreatedAt = &ms
}

// determineSummary sets the Summary field based on available metadata.
// Priority: custom_title > ai_title > first_prompt > timestamp fallback > session ID.
func determineSummary(info *SDKSessionInfo) {
	switch {
	case info.CustomTitle != nil:
		info.Summary = *info.CustomTitle
	case info.AITitle != nil:
		info.Summary = *info.AITitle
	case info.FirstPrompt != nil:
		info.Summary = *info.FirstPrompt
	case info.CreatedAt != nil:
		info.Summary = fmt.Sprintf("New session - %s", time.UnixMilli(*info.CreatedAt).UTC().Format(time.RFC3339))
	default:
		info.Summary = info.SessionID
	}
}

// buildSessionInfo constructs SDKSessionInfo from parsed JSONL entries.
func buildSessionInfo(sessionID string, entries []jsonlEntry, fileInfo os.FileInfo) *SDKSessionInfo {
	info := &SDKSessionInfo{
		SessionID:    sessionID,
		LastModified: fileInfo.ModTime().UnixMilli(),
	}

	fileSize := fileInfo.Size()
	info.FileSize = &fileSize

	extractMetadataFromEntries(info, entries)
	determineSummary(info)

	return info
}

// parseMessageContent parses the content field of a message into a typed MessageContent.
func parseMessageContent(msg map[string]any) *MessageContent {
	content, ok := msg["content"]
	if !ok {
		return nil
	}

	// String content.
	if s, ok := content.(string); ok {
		return &MessageContent{
			Kind:   ContentTypeString,
			String: s,
		}
	}

	// Content block array.
	blocks, ok := content.([]any)
	if !ok {
		return nil
	}

	parsed := make([]ContentBlock, 0, len(blocks))
	for _, block := range blocks {
		b, ok := block.(map[string]any)
		if !ok {
			continue
		}
		parsed = append(parsed, parseContentBlock(b))
	}

	return &MessageContent{
		Kind:   ContentTypeBlocks,
		Blocks: parsed,
	}
}

// parseContentBlock parses a single content block from a raw map.
// Known fields are extracted into typed struct fields; Raw is always populated.
func parseContentBlock(raw map[string]any) ContentBlock {
	cb := ContentBlock{
		Raw: raw,
	}

	cb.Type, _ = raw["type"].(string)

	switch cb.Type {
	case BlockTypeText:
		cb.Text, _ = raw["text"].(string)

	case BlockTypeThinking:
		cb.Thinking, _ = raw["thinking"].(string)
		cb.Signature, _ = raw["signature"].(string)

	case BlockTypeRedactedThinking:
		cb.Data, _ = raw["data"].(string)

	case BlockTypeToolUse, BlockTypeServerToolUse:
		cb.ID, _ = raw["id"].(string)
		cb.Name, _ = raw["name"].(string)
		if input, ok := raw["input"].(map[string]any); ok {
			cb.Input = input
		}

	case BlockTypeToolResult:
		cb.ToolUseID, _ = raw["tool_use_id"].(string)
		cb.Content = raw["content"]
		if isErr, ok := raw["is_error"].(bool); ok {
			cb.IsError = &isErr
		}

	case BlockTypeImage:
		if source, ok := raw["source"].(map[string]any); ok {
			cb.Source = source
		}
	}

	return cb
}

// Entry type constants for JSONL entries.
const (
	entryTypeUser      = "user"
	entryTypeAssistant = "assistant"
)

// transcriptEntryTypes are the JSONL entry types that carry uuid + parentUuid
// chain links, matching the Python SDK's _TRANSCRIPT_ENTRY_TYPES.
var transcriptEntryTypes = map[string]bool{
	entryTypeUser: true, entryTypeAssistant: true, "progress": true, "system": true, "attachment": true,
}

// isTranscriptEntry returns true if the entry is a transcript message type with a uuid.
func isTranscriptEntry(e jsonlEntry) bool {
	if !transcriptEntryTypes[e.entryType] {
		return false
	}
	uuid, _ := e.raw["uuid"].(string)
	return uuid != ""
}

// isVisibleMessage returns true if the entry should be included in returned messages.
// Matches the Python SDK's _is_visible_message filter.
func isVisibleMessage(e jsonlEntry) bool {
	if e.entryType != entryTypeUser && e.entryType != entryTypeAssistant {
		return false
	}
	if meta, ok := e.raw["isMeta"].(bool); ok && meta {
		return false
	}
	if sc, ok := e.raw["isSidechain"].(bool); ok && sc {
		return false
	}
	if tn, ok := e.raw["teamName"].(string); ok && tn != "" {
		return false
	}
	return true
}

// entryUUID returns the uuid of a JSONL entry, or "".
func entryUUID(e jsonlEntry) string {
	uuid, _ := e.raw["uuid"].(string)
	return uuid
}

// entryParentUUID returns the parentUuid of a JSONL entry, or "".
func entryParentUUID(e jsonlEntry) string {
	parent, _ := e.raw["parentUuid"].(string)
	return parent
}

// leaf represents a user/assistant entry at the end of a conversation branch,
// used during chain reconstruction to pick the best (main) conversation path.
type leaf struct {
	idx         int
	isSidechain bool
	isTeamName  bool
	isMeta      bool
}

// findTerminals returns the indices of transcript entries that have no children
// (i.e., no other entry lists their uuid as a parentUuid).
func findTerminals(transcriptEntries []jsonlEntry) []int {
	childrenOf := make(map[string]bool, len(transcriptEntries))
	for _, e := range transcriptEntries {
		if parent := entryParentUUID(e); parent != "" {
			childrenOf[parent] = true
		}
	}

	var terminals []int
	for i, e := range transcriptEntries {
		if !childrenOf[entryUUID(e)] {
			terminals = append(terminals, i)
		}
	}
	return terminals
}

// findLeaves walks back from each terminal via parentUuid to find the nearest
// user/assistant entry, collecting metadata about sidechain/teamName/isMeta status.
func findLeaves(transcriptEntries []jsonlEntry, terminals []int, byUUID map[string]int) []leaf {
	var leaves []leaf
	for _, termIdx := range terminals {
		if l, ok := walkToLeaf(transcriptEntries, termIdx, byUUID); ok {
			leaves = append(leaves, l)
		}
	}
	return leaves
}

// walkToLeaf walks backwards from a terminal entry via parentUuid links until it
// finds a user or assistant entry, returning it as a leaf. Returns ok=false if
// no qualifying entry is found.
func walkToLeaf(transcriptEntries []jsonlEntry, startIdx int, byUUID map[string]int) (leaf, bool) {
	seen := make(map[string]bool)
	cur := startIdx
	for cur >= 0 {
		uuid := entryUUID(transcriptEntries[cur])
		if seen[uuid] {
			break
		}
		seen[uuid] = true
		e := transcriptEntries[cur]
		if e.entryType == entryTypeUser || e.entryType == entryTypeAssistant {
			sc, _ := e.raw["isSidechain"].(bool)
			tn, _ := e.raw["teamName"].(string)
			meta, _ := e.raw["isMeta"].(bool)
			return leaf{idx: cur, isSidechain: sc, isTeamName: tn != "", isMeta: meta}, true
		}
		parent := entryParentUUID(e)
		if parent == "" {
			break
		}
		parentIdx, ok := byUUID[parent]
		if !ok {
			break
		}
		cur = parentIdx
	}
	return leaf{}, false
}

// pickBestLeaf selects the best leaf from candidates: prefers entries that are
// not sidechain/teamName/isMeta, breaking ties by highest file position (index).
func pickBestLeaf(leaves []leaf) leaf {
	var mainLeaves []leaf
	for _, l := range leaves {
		if !l.isSidechain && !l.isTeamName && !l.isMeta {
			mainLeaves = append(mainLeaves, l)
		}
	}
	candidates := mainLeaves
	if len(candidates) == 0 {
		candidates = leaves
	}
	best := candidates[0]
	for _, c := range candidates[1:] {
		if c.idx > best.idx {
			best = c
		}
	}
	return best
}

// walkChainToRoot walks from the entry at startIdx to the root via parentUuid
// links, collecting entries along the way. Returns entries in chronological order
// (root first).
func walkChainToRoot(transcriptEntries []jsonlEntry, startIdx int, byUUID map[string]int) []jsonlEntry {
	var chain []jsonlEntry
	seen := make(map[string]bool)
	cur := startIdx
	for cur >= 0 {
		e := transcriptEntries[cur]
		uuid := entryUUID(e)
		if seen[uuid] {
			break
		}
		seen[uuid] = true
		chain = append(chain, e)
		parent := entryParentUUID(e)
		if parent == "" {
			break
		}
		parentIdx, ok := byUUID[parent]
		if !ok {
			break
		}
		cur = parentIdx
	}

	// Reverse to chronological order.
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	return chain
}

// buildConversationChain reconstructs the main conversation chain from transcript
// entries by walking parentUuid links, matching the Python SDK's _build_conversation_chain.
//
// Algorithm:
//  1. Index transcript entries by uuid
//  2. Find terminals (entries with no children)
//  3. From each terminal, walk back to find the nearest user/assistant leaf
//  4. Pick the best leaf: not sidechain/teamName/isMeta, highest file position
//  5. Walk from leaf to root via parentUuid, reverse to chronological order
func buildConversationChain(transcriptEntries []jsonlEntry, byUUID map[string]int) []jsonlEntry {
	if len(transcriptEntries) == 0 {
		return nil
	}

	terminals := findTerminals(transcriptEntries)
	leaves := findLeaves(transcriptEntries, terminals, byUUID)
	if len(leaves) == 0 {
		return nil
	}

	best := pickBestLeaf(leaves)
	return walkChainToRoot(transcriptEntries, best.idx, byUUID)
}

// buildMessages extracts user and assistant messages from JSONL entries.
// If entries contain parentUuid fields, it reconstructs the conversation chain.
// Otherwise, it falls back to a flat scan with visibility filtering.
func buildMessages(sessionID string, entries []jsonlEntry) []Message {
	// Check if any entry has parentUuid — determines chain vs flat-scan path.
	hasParentUUID := false
	var transcriptEntries []jsonlEntry
	byUUID := make(map[string]int)

	for _, e := range entries {
		if !isTranscriptEntry(e) {
			continue
		}
		idx := len(transcriptEntries)
		transcriptEntries = append(transcriptEntries, e)
		byUUID[entryUUID(e)] = idx
		if entryParentUUID(e) != "" {
			hasParentUUID = true
		}
	}

	var visible []jsonlEntry
	if hasParentUUID {
		chain := buildConversationChain(transcriptEntries, byUUID)
		for _, e := range chain {
			if isVisibleMessage(e) {
				visible = append(visible, e)
			}
		}
	} else {
		// Flat-scan fallback for sessions without parentUuid.
		for _, e := range entries {
			if isVisibleMessage(e) {
				visible = append(visible, e)
			}
		}
	}

	messages := make([]Message, 0, len(visible))
	for _, e := range visible {
		msg := Message{
			Type:      e.entryType,
			SessionID: sessionID,
		}
		if uuid, ok := e.raw["uuid"].(string); ok {
			msg.UUID = uuid
		}
		if meta, ok := e.raw["isMeta"].(bool); ok && meta {
			msg.IsMeta = true
		}
		if rawMsg, ok := e.raw["message"].(map[string]any); ok {
			msg.RawMessage = rawMsg
			msg.Content = parseMessageContent(rawMsg)
		}

		messages = append(messages, msg)
	}
	return messages
}
