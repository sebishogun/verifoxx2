package framed

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"strings"
	"testing"
)

func encodedFrame(payload string) []byte {
	frame := make([]byte, 4+len(payload))
	binary.BigEndian.PutUint32(frame[:4], uint32(len(payload)))
	copy(frame[4:], payload)
	return frame
}

type fragmentedReader struct{ reader *bytes.Reader }

func (reader fragmentedReader) Read(dst []byte) (int, error) {
	if len(dst) > 1 {
		dst = dst[:1]
	}
	return reader.reader.Read(dst)
}

func TestFrameReaderReadsFragmentedFramesAndCleanEOF(t *testing.T) {
	data := append(encodedFrame("first payload"), encodedFrame("two")...)
	source := fragmentedReader{reader: bytes.NewReader(data)}
	var reader FrameReader

	payload, ok, err := reader.Read(source)
	if err != nil || !ok || string(payload) != "first payload" {
		t.Fatalf("first Read = (%q, %v, %v)", payload, ok, err)
	}
	storage := &payload[0]
	payload, ok, err = reader.Read(source)
	if err != nil || !ok || string(payload) != "two" {
		t.Fatalf("second Read = (%q, %v, %v)", payload, ok, err)
	}
	if &payload[0] != storage {
		t.Fatal("FrameReader did not reuse payload storage")
	}
	payload, ok, err = reader.Read(source)
	if err != nil || ok || payload != nil {
		t.Fatalf("EOF Read = (%q, %v, %v), want (nil, false, nil)", payload, ok, err)
	}
}

func TestFrameReaderRejectsTruncatedAndOversizedFrames(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want string
	}{
		{"partial header", []byte{0, 0}, "header"},
		{"partial payload", append([]byte{0, 0, 0, 4}, 'x'), "payload"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var reader FrameReader
			if _, _, err := reader.Read(bytes.NewReader(tt.data)); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Read error = %v, want %q context", err, tt.want)
			}
		})
	}

	var header [4]byte
	binary.BigEndian.PutUint32(header[:], MaxInputPayload+1)
	var reader FrameReader
	if _, _, err := reader.Read(bytes.NewReader(header[:])); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversized Read error = %v", err)
	}
	if cap(reader.payload) != 0 {
		t.Fatalf("oversized frame grew payload capacity to %d", cap(reader.payload))
	}
}

type shortWriter struct {
	buffer bytes.Buffer
	limit  int
	err    error
}

func (writer *shortWriter) Write(src []byte) (int, error) {
	if writer.err != nil {
		return 0, writer.err
	}
	if len(src) > writer.limit {
		src = src[:writer.limit]
	}
	return writer.buffer.Write(src)
}

func TestFrameWriterHandlesShortWritesAndErrors(t *testing.T) {
	destination := &shortWriter{limit: 2}
	var writer FrameWriter
	if err := writer.Write(destination, []byte("payload")); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(destination.buffer.Bytes(), encodedFrame("payload")) {
		t.Fatalf("written frame = %v", destination.buffer.Bytes())
	}

	want := errors.New("write failed")
	if err := writer.Write(&shortWriter{limit: 1, err: want}, []byte("x")); !errors.Is(err, want) {
		t.Fatalf("Write error = %v, want %v", err, want)
	}
	if err := writer.Write(io.Discard, make([]byte, MaxOutputPayload+1)); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversized Write error = %v", err)
	}
}
