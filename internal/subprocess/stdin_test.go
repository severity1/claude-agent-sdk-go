package subprocess

import (
	"bytes"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// TestStdinWritesAreSerialized verifies that concurrent writers (user
// messages and control responses) never interleave their lines, even over a
// pipe that does not serialize writes itself.
func TestStdinWritesAreSerialized(t *testing.T) {
	pipe := &byteByBytePipe{}
	writer := newStdinWriter(pipe)

	const writers = 8
	const lineLen = 2000
	var wg sync.WaitGroup
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(letter byte) {
			defer wg.Done()
			line := append(bytes.Repeat([]byte{letter}, lineLen), '\n')
			if _, err := writer.Write(line); err != nil {
				t.Errorf("Write: %v", err)
			}
		}(byte('a' + i))
	}
	wg.Wait()

	lines := strings.Split(strings.TrimSuffix(pipe.String(), "\n"), "\n")
	if len(lines) != writers {
		t.Fatalf("got %d lines, want %d", len(lines), writers)
		return
	}
	for _, line := range lines {
		if len(line) != lineLen || strings.Trim(line, line[:1]) != "" {
			t.Errorf("interleaved line: %.40q...", line)
		}
	}
}

// TestStdinWriterClosedRejectsWrites verifies that Write and EndInput after
// Close fail or do nothing, and that Close is idempotent.
func TestStdinWriterClosedRejectsWrites(t *testing.T) {
	pipe := &byteByBytePipe{}
	writer := newStdinWriter(pipe)
	writer.Close()
	writer.Close()

	if _, err := writer.Write([]byte("x\n")); err == nil {
		t.Error("Write after Close returned no error")
	}
	if err := writer.EndInput(); err != nil {
		t.Errorf("EndInput after Close: %v", err)
	}
}

// byteByBytePipe writes one byte at a time and yields between bytes, so
// unsynchronized concurrent writes interleave.
type byteByBytePipe struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (p *byteByBytePipe) Write(data []byte) (int, error) {
	for _, b := range data {
		p.mu.Lock()
		p.buf.WriteByte(b)
		p.mu.Unlock()
		runtime.Gosched()
	}
	return len(data), nil
}

func (p *byteByBytePipe) Close() error { return nil }

func (p *byteByBytePipe) String() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.buf.String()
}
