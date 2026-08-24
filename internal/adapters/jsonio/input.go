package jsonio

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/sebishogun/verifoxx2/internal/input"
)

const (
	MaxInputBytes   = 8 << 20
	MaxInputRecords = 1 << 20
)

func DecodeRequests(reader io.Reader, source string) ([]input.Request, error) {
	requests, err := decodeInput[[]input.Request](reader, source)
	if err != nil {
		return nil, err
	}
	if len(requests) > MaxInputRecords {
		return nil, fmt.Errorf("requests %s contain %d records, limit %d", source, len(requests), MaxInputRecords)
	}
	seen := make(map[string]struct{}, len(requests))
	for i := range requests {
		id := requests[i].ID
		if strings.TrimSpace(id) == "" || strings.TrimSpace(id) != id {
			return nil, fmt.Errorf("requests %s: request %d has malformed or empty id", source, i)
		}
		if _, exists := seen[id]; exists {
			return nil, fmt.Errorf("requests %s: duplicate request id %q", source, id)
		}
		seen[id] = struct{}{}
	}
	return requests, nil
}

func DecodeEvidence(reader io.Reader, source string) ([]input.Evidence, error) {
	evidence, err := decodeInput[[]input.Evidence](reader, source)
	if err != nil {
		return nil, err
	}
	if len(evidence) > MaxInputRecords {
		return nil, fmt.Errorf("evidence %s contains %d records, limit %d", source, len(evidence), MaxInputRecords)
	}
	seen := make(map[string]struct{}, len(evidence))
	for i := range evidence {
		id := evidence[i].ID
		if strings.TrimSpace(id) == "" || strings.TrimSpace(id) != id {
			return nil, fmt.Errorf("evidence %s: evidence %d has malformed or empty id", source, i)
		}
		if _, exists := seen[id]; exists {
			return nil, fmt.Errorf("evidence %s: duplicate evidence id %q", source, id)
		}
		seen[id] = struct{}{}
	}
	return evidence, nil
}

func LoadRequests(path string) ([]input.Request, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open requests %s: %w", path, err)
	}
	defer file.Close()
	return DecodeRequests(file, path)
}

func LoadEvidence(path string) ([]input.Evidence, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open evidence %s: %w", path, err)
	}
	defer file.Close()
	return DecodeEvidence(file, path)
}

func decodeInput[T any](reader io.Reader, source string) (T, error) {
	var value T
	data, err := io.ReadAll(io.LimitReader(reader, MaxInputBytes+1))
	if err != nil {
		return value, fmt.Errorf("read %s: %w", source, err)
	}
	if len(data) > MaxInputBytes {
		return value, fmt.Errorf("input %s exceeds %d bytes", source, MaxInputBytes)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return value, fmt.Errorf("decode %s: %w", source, err)
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return value, fmt.Errorf("decode %s: trailing JSON value", source)
		}
		return value, fmt.Errorf("decode %s: trailing JSON: %w", source, err)
	}
	return value, nil
}
