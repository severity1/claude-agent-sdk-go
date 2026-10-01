package subprocess

import (
	"context"
	"io"
	"sync"
)

// ProtocolAdapter adapts subprocess stdin/stdout for use with control.Protocol.
// It implements the control.Transport interface to enable the control protocol
// to send requests via subprocess stdin.
//
// Note: The Read() method returns a closed channel because we don't use the
// protocol's built-in readLoop. Instead, subprocess.Transport routes control
// messages to protocol.HandleIncomingMessageAsync() from handleStdout().
type ProtocolAdapter struct {
	stdin    io.Writer
	mu       sync.Mutex
	closed   bool
	readChan chan []byte
}

// NewProtocolAdapter creates a new adapter that wraps subprocess stdin for
// the control protocol.
func NewProtocolAdapter(stdin io.Writer) *ProtocolAdapter {
	// Create a closed channel for Read() - we handle message routing externally
	readChan := make(chan []byte)
	close(readChan)

	return &ProtocolAdapter{
		stdin:    stdin,
		readChan: readChan,
	}
}

// Write sends data to the subprocess stdin.
// This is called by Protocol.SendControlRequest() to send control requests.
func (pa *ProtocolAdapter) Write(ctx context.Context, data []byte) error {
	// Check context before proceeding
	if ctx.Err() != nil {
		return ctx.Err()
	}

	pa.mu.Lock()
	defer pa.mu.Unlock()

	if pa.closed {
		return io.ErrClosedPipe
	}

	if pa.stdin == nil {
		return io.ErrClosedPipe
	}

	_, err := pa.stdin.Write(data)
	return err
}

// Read returns a channel for reading data from the subprocess.
// This channel is pre-closed because we don't use the protocol's built-in
// readLoop - instead, subprocess.Transport routes control messages to
// protocol.HandleIncomingMessageAsync() from handleStdout().
func (pa *ProtocolAdapter) Read(_ context.Context) <-chan []byte {
	return pa.readChan
}

// Close closes the adapter.
// Note: This does NOT close the underlying stdin - that's managed by Transport.
func (pa *ProtocolAdapter) Close() error {
	pa.mu.Lock()
	defer pa.mu.Unlock()

	pa.closed = true
	// Don't close stdin here - Transport manages that
	return nil
}
