package disc

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"testing"
)

func remoteTitleFixture(t *testing.T, payload []byte, guard func(context.Context) error, waitErr error) *remoteTitleStream {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	// Only a short synthetic payload is placed in the pipe before consumption.
	if _, err = writer.Write(payload); err != nil {
		reader.Close()
		writer.Close()
		t.Fatal(err)
	}
	writer.Close()
	done := make(chan struct{})
	close(done)
	s := &remoteTitleStream{ctx: context.Background(), file: reader, guard: guard, stop: func() {}, done: done, waitErr: waitErr}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestRemoteTitleStreamNeverReportsFailedProducerAsSuccessfulEOF(t *testing.T) {
	s := remoteTitleFixture(t, nil, func(context.Context) error { return nil }, errors.New("producer failed"))
	_, err := s.Read(make([]byte, 10))
	if !errors.Is(err, ErrProtocol) || errors.Is(err, io.EOF) {
		t.Fatal("a failed producer became a successful end-of-title")
	}
}

func TestRemoteTitleStreamRetirementDoesNotCopyBufferedBytes(t *testing.T) {
	retired, calls := errors.New("original caller retired"), 0
	s := remoteTitleFixture(t, []byte("media"), func(context.Context) error {
		calls++
		if calls > 1 {
			return retired
		}
		return nil
	}, nil)
	out := []byte("xxxxx")
	n, err := s.Read(out)
	if n != 0 || !errors.Is(err, retired) || !bytes.Equal(out, []byte("xxxxx")) {
		t.Fatal("retired buffered media reached the consumer")
	}
}

func TestRemoteTitleStreamPreservesSuccessfulTailThenEOF(t *testing.T) {
	s := remoteTitleFixture(t, []byte("media"), func(context.Context) error { return nil }, nil)
	out, err := io.ReadAll(s)
	if err != nil || !bytes.Equal(out, []byte("media")) {
		t.Fatal("a successful source tail was lost")
	}
}

func TestRemoteTitleStreamRequiresOriginalAdmission(t *testing.T) {
	_, _, err := (Tool{Binary: "/must-not-launch"}).StreamRemoteBluRay(context.Background(), nil, Probe{}, Title{}, 0, nil)
	if !errors.Is(err, ErrRemoteUnqualified) {
		t.Fatal("missing original admission was not refused")
	}
}
