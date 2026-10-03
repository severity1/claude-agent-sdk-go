package claudecode

import (
	"github.com/severity1/claude-agent-sdk-go/internal/session"
)

// SDKSessionInfo holds metadata about a session.
type SDKSessionInfo = session.SDKSessionInfo

// SessionMessage represents a message from a session transcript.
type SessionMessage = session.Message

// SessionContentBlock represents a typed content block from a session message.
type SessionContentBlock = session.ContentBlock

// SessionMessageContent is a sum type representing the content of a session message.
type SessionMessageContent = session.MessageContent

// SessionContentType discriminates the SessionMessageContent union.
type SessionContentType = session.ContentType

// SessionContentType constants.
const (
	SessionContentTypeString = session.ContentTypeString
	SessionContentTypeBlocks = session.ContentTypeBlocks
)

// Session content block type constants.
const (
	SessionBlockTypeText                         = session.BlockTypeText
	SessionBlockTypeThinking                     = session.BlockTypeThinking
	SessionBlockTypeRedactedThinking             = session.BlockTypeRedactedThinking
	SessionBlockTypeToolUse                      = session.BlockTypeToolUse
	SessionBlockTypeServerToolUse                = session.BlockTypeServerToolUse
	SessionBlockTypeToolResult                   = session.BlockTypeToolResult
	SessionBlockTypeImage                        = session.BlockTypeImage
	SessionBlockTypeWebSearchToolResult          = session.BlockTypeWebSearchToolResult
	SessionBlockTypeWebFetchToolResult           = session.BlockTypeWebFetchToolResult
	SessionBlockTypeCodeExecutionToolResult      = session.BlockTypeCodeExecutionToolResult
	SessionBlockTypeBashCodeExecutionToolResult  = session.BlockTypeBashCodeExecutionToolResult
	SessionBlockTypeTextEditorCodeExecToolResult = session.BlockTypeTextEditorCodeExecToolResult
	SessionBlockTypeToolSearchToolResult         = session.BlockTypeToolSearchToolResult
	SessionBlockTypeContainerUpload              = session.BlockTypeContainerUpload
)

// SessionOption configures session query behavior.
type SessionOption = session.Option

// WithSessionDirectory scopes the query to a specific project directory.
// When omitted, sessions across all projects are searched.
var WithSessionDirectory = session.WithSessionDirectory

// WithSessionLimit sets the maximum number of results to return.
var WithSessionLimit = session.WithSessionLimit

// WithSessionOffset skips the first n messages (GetSessionMessages only).
var WithSessionOffset = session.WithSessionOffset

// WithIncludeWorktrees controls whether git worktree directories are included
// when searching for sessions. Defaults to true. Only has effect when a
// directory is specified via WithSessionDirectory.
var WithIncludeWorktrees = session.WithIncludeWorktrees

// ListSessions returns metadata for sessions, sorted by LastModified descending.
// Use WithSessionDirectory to scope to a specific project, or omit to list all.
//
// Example:
//
//	// List 10 most recent sessions in a project
//	sessions, err := claudecode.ListSessions(
//	    claudecode.WithSessionDirectory("/path/to/project"),
//	    claudecode.WithSessionLimit(10),
//	)
//
//	// List all sessions across all projects
//	sessions, err := claudecode.ListSessions()
func ListSessions(opts ...SessionOption) ([]SDKSessionInfo, error) {
	return session.ListSessions(opts...)
}

// GetSessionMessages reads user and assistant messages from a session transcript.
//
// Example:
//
//	messages, err := claudecode.GetSessionMessages(sessionID,
//	    claudecode.WithSessionDirectory("/path/to/project"),
//	    claudecode.WithSessionLimit(20),
//	)
func GetSessionMessages(sessionID string, opts ...SessionOption) ([]SessionMessage, error) {
	return session.GetMessages(sessionID, opts...)
}

// GetSessionInfo returns metadata for a single session by ID.
// Returns nil (not an error) if the session is not found.
//
// Example:
//
//	info, err := claudecode.GetSessionInfo(sessionID)
//	if info != nil {
//	    fmt.Println(info.Summary)
//	}
func GetSessionInfo(sessionID string, opts ...SessionOption) (*SDKSessionInfo, error) {
	return session.GetSessionInfo(sessionID, opts...)
}

// RenameSession sets the title of a session. It appends a custom-title entry
// to the session JSONL file, so the last call wins. The title is trimmed and
// must not be empty. It returns an error when the session file is not found.
//
// Example:
//
//	err := claudecode.RenameSession(sessionID, "Refactor auth",
//	    claudecode.WithSessionDirectory("/path/to/project"),
//	)
func RenameSession(sessionID, title string, opts ...SessionOption) error {
	return session.RenameSession(sessionID, title, opts...)
}

// TagSession sets the tag of a session. A nil tag clears it. The tag is
// Unicode-sanitized (invisible and format characters are removed) and
// trimmed, and must not be empty after that. The last call wins.
//
// Example:
//
//	tag := "experiment"
//	err := claudecode.TagSession(sessionID, &tag)
//
//	// Clear the tag
//	err = claudecode.TagSession(sessionID, nil)
func TagSession(sessionID string, tag *string, opts ...SessionOption) error {
	return session.TagSession(sessionID, tag, opts...)
}
