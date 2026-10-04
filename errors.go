package claudecode

import (
	"errors"

	"github.com/severity1/claude-agent-sdk-go/internal/control"
	"github.com/severity1/claude-agent-sdk-go/internal/shared"
)

// ErrNotConnected is wrapped by the *ConnectionError that Client methods
// return before Connect or after Disconnect. Match it with errors.Is.
var ErrNotConnected = errors.New("client not connected")

// ErrProtocolClosed is returned by a control request (for example Interrupt
// or SetModel) that still waits for its response when the connection closes.
// Match it with errors.Is.
var ErrProtocolClosed = control.ErrProtocolClosed

// notConnectedError is the error for a call on a client that is not connected.
func notConnectedError() error {
	return NewConnectionError("not connected, call Connect first", ErrNotConnected)
}

// SDKError represents the base interface for all SDK errors.
type SDKError = shared.SDKError

// BaseError provides common error functionality across the SDK.
type BaseError = shared.BaseError

// ConnectionError represents errors that occur during CLI connection.
type ConnectionError = shared.ConnectionError

// CLINotFoundError indicates that the Claude Code CLI was not found.
type CLINotFoundError = shared.CLINotFoundError

// ProcessError represents errors from the CLI process execution.
type ProcessError = shared.ProcessError

// JSONDecodeError represents JSON parsing errors from CLI responses.
type JSONDecodeError = shared.JSONDecodeError

// MessageParseError represents errors parsing message content.
type MessageParseError = shared.MessageParseError

// NewConnectionError creates a new connection error.
var NewConnectionError = shared.NewConnectionError

// NewCLINotFoundError creates a new CLI not found error.
var NewCLINotFoundError = shared.NewCLINotFoundError

// NewProcessError creates a new process error.
var NewProcessError = shared.NewProcessError

// NewJSONDecodeError creates a new JSON decode error.
var NewJSONDecodeError = shared.NewJSONDecodeError

// NewMessageParseError creates a new message parse error.
var NewMessageParseError = shared.NewMessageParseError

// Error type checking helpers (Go-specific, follows os.IsNotExist pattern).
// These use errors.As() internally to handle wrapped errors correctly.

// IsConnectionError reports whether err is or wraps a ConnectionError.
var IsConnectionError = shared.IsConnectionError

// IsCLINotFoundError reports whether err is or wraps a CLINotFoundError.
var IsCLINotFoundError = shared.IsCLINotFoundError

// IsProcessError reports whether err is or wraps a ProcessError.
var IsProcessError = shared.IsProcessError

// IsJSONDecodeError reports whether err is or wraps a JSONDecodeError.
var IsJSONDecodeError = shared.IsJSONDecodeError

// IsMessageParseError reports whether err is or wraps a MessageParseError.
var IsMessageParseError = shared.IsMessageParseError

// Error type extraction helpers (Go-specific).
// Returns typed pointer for field access, or nil if not matching type.

// AsConnectionError returns the error as a *ConnectionError if it is one,
// or nil otherwise.
var AsConnectionError = shared.AsConnectionError

// AsCLINotFoundError returns the error as a *CLINotFoundError if it is one,
// or nil otherwise.
var AsCLINotFoundError = shared.AsCLINotFoundError

// AsProcessError returns the error as a *ProcessError if it is one,
// or nil otherwise.
var AsProcessError = shared.AsProcessError

// AsJSONDecodeError returns the error as a *JSONDecodeError if it is one,
// or nil otherwise.
var AsJSONDecodeError = shared.AsJSONDecodeError

// AsMessageParseError returns the error as a *MessageParseError if it is one,
// or nil otherwise.
var AsMessageParseError = shared.AsMessageParseError
