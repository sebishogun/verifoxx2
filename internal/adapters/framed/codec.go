package framed

import (
	"encoding/binary"
	"fmt"
	"io"
)

const (
	MaxInputPayload  = 16 << 20
	MaxOutputPayload = 64 << 20
)

// FrameReader owns its returned payload. The next Read may overwrite it.
type FrameReader struct {
	header  [4]byte
	payload []byte
}

func (reader *FrameReader) Read(source io.Reader) ([]byte, bool, error) {
	read, err := io.ReadFull(source, reader.header[:])
	if err != nil {
		if err == io.EOF && read == 0 {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("read frame header: %w", err)
	}
	size := binary.BigEndian.Uint32(reader.header[:])
	if size > MaxInputPayload {
		return nil, false, fmt.Errorf("frame payload length %d exceeds limit %d", size, MaxInputPayload)
	}
	if cap(reader.payload) < int(size) {
		reader.payload = make([]byte, int(size))
	} else {
		reader.payload = reader.payload[:int(size)]
	}
	if _, err := io.ReadFull(source, reader.payload); err != nil {
		return nil, false, fmt.Errorf("read frame payload: %w", err)
	}
	return reader.payload, true, nil
}

type FrameWriter struct {
	header [4]byte
}

func (writer *FrameWriter) Write(destination io.Writer, payload []byte) error {
	if len(payload) > MaxOutputPayload {
		return fmt.Errorf("frame payload length %d exceeds limit %d", len(payload), MaxOutputPayload)
	}
	binary.BigEndian.PutUint32(writer.header[:], uint32(len(payload)))
	if err := writeFull(destination, writer.header[:]); err != nil {
		return fmt.Errorf("write frame header: %w", err)
	}
	if err := writeFull(destination, payload); err != nil {
		return fmt.Errorf("write frame payload: %w", err)
	}
	return nil
}

func writeFull(destination io.Writer, payload []byte) error {
	for len(payload) != 0 {
		written, err := destination.Write(payload)
		if written < 0 || written > len(payload) {
			return fmt.Errorf("invalid write count %d for %d bytes", written, len(payload))
		}
		payload = payload[written:]
		if err != nil {
			return err
		}
		if written == 0 {
			return io.ErrShortWrite
		}
	}
	return nil
}
