package subprocess

import (
	"io"
	"sync"
)

// stdinWriter is the single writer for the CLI stdin. Every write takes
// writeMu so JSON lines from user messages and control responses never
// interleave (Python: _write_lock). Close does not take writeMu, so it can
// unblock a write that waits on a full pipe.
type stdinWriter struct {
	pipe    io.WriteCloser
	writeMu sync.Mutex

	stateMu sync.Mutex
	closed  bool
}

func newStdinWriter(pipe io.WriteCloser) *stdinWriter {
	return &stdinWriter{pipe: pipe}
}

// Write writes one complete line to stdin. It implements io.Writer so the
// protocol adapter can share the same lock.
func (w *stdinWriter) Write(data []byte) (int, error) {
	w.writeMu.Lock()
	defer w.writeMu.Unlock()
	if w.isClosed() {
		return 0, io.ErrClosedPipe
	}
	return w.pipe.Write(data)
}

// EndInput closes stdin after any write in progress, so a line is never cut.
func (w *stdinWriter) EndInput() error {
	w.writeMu.Lock()
	defer w.writeMu.Unlock()
	if !w.markClosed() {
		return nil
	}
	return w.pipe.Close()
}

// Close closes stdin without waiting for a write in progress. The close runs
// on its own goroutine because some platforms keep a pipe handle until the
// pending write fails, which happens when the CLI exits.
func (w *stdinWriter) Close() {
	if !w.markClosed() {
		return
	}
	go func() { _ = w.pipe.Close() }()
}

func (w *stdinWriter) isClosed() bool {
	w.stateMu.Lock()
	defer w.stateMu.Unlock()
	return w.closed
}

// markClosed sets the closed flag and reports whether this call set it.
func (w *stdinWriter) markClosed() bool {
	w.stateMu.Lock()
	defer w.stateMu.Unlock()
	if w.closed {
		return false
	}
	w.closed = true
	return true
}
