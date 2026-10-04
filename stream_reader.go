package claudecode

import (
	"context"
	"sync"
)

// streamReader reads one transport stream. The transport sends a terminal
// error to errChan after every message it sent to msgChan, so the reader
// delivers every buffered message before that error (Python: ProcessError is
// raised after read_messages has yielded everything). It is shared by
// successive ReceiveResponse iterators because an error read while messages
// were still buffered must survive until a later call asks for it.
type streamReader struct {
	msgChan <-chan Message
	errChan <-chan error

	mu      sync.Mutex
	pending error
}

func newStreamReader(msgChan <-chan Message, errChan <-chan error) *streamReader {
	return &streamReader{msgChan: msgChan, errChan: errChan}
}

// next returns the next message. Errors from aux, if any, are returned at
// once. At the end of the stream it returns the terminal error, or
// ErrNoMoreMessages when there is none.
func (s *streamReader) next(ctx context.Context, aux <-chan error) (Message, error) {
	errChan := s.errChan
	for {
		if msg, err := s.drainToPending(); msg != nil || err != nil {
			return msg, err
		}
		select {
		case msg, ok := <-s.msgChan:
			if !ok {
				return nil, s.endOfStream()
			}
			return msg, nil
		case err, ok := <-errChan:
			if !ok {
				errChan = nil
				continue
			}
			s.setPending(err)
		case err, ok := <-aux:
			if !ok {
				aux = nil
				continue
			}
			return nil, err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// drainToPending returns a message that is still buffered, or else the
// pending error. The error was sent after those messages, so a message that
// is not buffered now is never coming before it. It returns (nil, nil) when
// neither is there, for example when another iterator took the error first.
func (s *streamReader) drainToPending() (Message, error) {
	select {
	case msg, ok := <-s.msgChan:
		if ok {
			return msg, nil
		}
	default:
	}
	return nil, s.takePending()
}

// endOfStream is the error for a closed msgChan. The transport sends its
// terminal error before it closes msgChan, so one may still sit in errChan.
func (s *streamReader) endOfStream() error {
	if err := s.takePending(); err != nil {
		return err
	}
	select {
	case err, ok := <-s.errChan:
		if ok {
			return err
		}
	default:
	}
	return ErrNoMoreMessages
}

func (s *streamReader) setPending(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending = err
}

func (s *streamReader) takePending() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	err := s.pending
	s.pending = nil
	return err
}
