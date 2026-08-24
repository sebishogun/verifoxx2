package jsonio

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/sebishogun/verifoxx2/internal/ast"
)

const MaxPolicyBytes = 1 << 20

func DecodePolicy(reader io.Reader, source string) (ast.Policy, error) {
	var policy ast.Policy
	data, err := io.ReadAll(io.LimitReader(reader, MaxPolicyBytes+1))
	if err != nil {
		return policy, fmt.Errorf("read %s: %w", source, err)
	}
	if len(data) > MaxPolicyBytes {
		return policy, fmt.Errorf("policy %s exceeds %d bytes", source, MaxPolicyBytes)
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&policy); err != nil {
		return policy, fmt.Errorf("decode %s: %w", source, err)
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return policy, fmt.Errorf("decode %s: trailing JSON value", source)
		}
		return policy, fmt.Errorf("decode %s: trailing JSON: %w", source, err)
	}
	if err := policy.Validate(); err != nil {
		return ast.Policy{}, fmt.Errorf("validate %s: %w", source, err)
	}
	return policy, nil
}

func LoadPolicy(path string) (ast.Policy, error) {
	file, err := os.Open(path)
	if err != nil {
		return ast.Policy{}, fmt.Errorf("open policy %s: %w", path, err)
	}
	defer file.Close()
	return DecodePolicy(file, path)
}
