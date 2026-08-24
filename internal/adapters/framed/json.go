package framed

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"unicode/utf8"

	"github.com/sebishogun/verifoxx2/internal/adapters/jsonio"
	decisioninput "github.com/sebishogun/verifoxx2/internal/input"
)

const MaxErrorMessageBytes = 1024

type Input struct {
	Requests []decisioninput.Request  `json:"requests"`
	Evidence []decisioninput.Evidence `json:"evidence"`
}

type ResponseError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Response struct {
	OK     bool               `json:"ok"`
	Output *jsonio.OutputPack `json:"output,omitempty"`
	Error  *ResponseError     `json:"error,omitempty"`
}

type JSONCodec struct {
	input         Input
	reader        bytes.Reader
	response      Response
	responseError ResponseError
	output        boundedBuffer
}

func NewJSONCodec(maxResponseBytes int) *JSONCodec {
	if maxResponseBytes <= 0 || maxResponseBytes > MaxOutputPayload {
		maxResponseBytes = MaxOutputPayload
	}
	return &JSONCodec{output: boundedBuffer{limit: maxResponseBytes}}
}

// Decode returns codec-owned input that is overwritten by the next call.
func (codec *JSONCodec) Decode(payload []byte) (*Input, error) {
	if len(payload) > MaxInputPayload {
		return nil, fmt.Errorf("JSON payload length %d exceeds limit %d", len(payload), MaxInputPayload)
	}
	document := bytes.TrimSpace(payload)
	if len(document) == 0 || document[0] != '{' {
		return nil, fmt.Errorf("decode frame input: expected JSON object")
	}
	codec.resetInput()
	if cap(codec.input.Requests) == 0 {
		codec.input.Requests = make([]decisioninput.Request, 0, 1)
	}
	if cap(codec.input.Evidence) == 0 {
		codec.input.Evidence = make([]decisioninput.Evidence, 0, 1)
	}
	requestMarker := &codec.input.Requests[:1][0]
	evidenceMarker := &codec.input.Evidence[:1][0]
	codec.reader.Reset(payload)
	decoder := json.NewDecoder(&codec.reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&codec.input); err != nil {
		codec.resetInput()
		return nil, fmt.Errorf("decode frame input: %w", err)
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		codec.resetInput()
		if err == nil {
			return nil, fmt.Errorf("decode frame input: trailing JSON value")
		}
		return nil, fmt.Errorf("decode frame input trailing JSON: %w", err)
	}
	if codec.input.Requests == nil || codec.input.Evidence == nil {
		codec.resetInput()
		return nil, fmt.Errorf("decode frame input: requests and evidence must be arrays")
	}
	requestsMissing := len(codec.input.Requests) == 0 && cap(codec.input.Requests) != 0 && &codec.input.Requests[:1][0] == requestMarker
	evidenceMissing := len(codec.input.Evidence) == 0 && cap(codec.input.Evidence) != 0 && &codec.input.Evidence[:1][0] == evidenceMarker
	if requestsMissing || evidenceMissing {
		codec.resetInput()
		return nil, fmt.Errorf("decode frame input: requests and evidence are required")
	}
	return &codec.input, nil
}

// EncodeSuccess returns codec-owned bytes that are overwritten by the next
// encode call.
func (codec *JSONCodec) EncodeSuccess(output *jsonio.OutputPack) ([]byte, error) {
	codec.response = Response{OK: true, Output: output}
	return codec.encodeResponse()
}

func (codec *JSONCodec) EncodeError(code, message string) ([]byte, error) {
	codec.responseError.Code = code
	codec.responseError.Message = truncateUTF8(message, MaxErrorMessageBytes)
	codec.response = Response{Error: &codec.responseError}
	return codec.encodeResponse()
}

func (codec *JSONCodec) encodeResponse() ([]byte, error) {
	codec.output.Reset()
	encoder := json.NewEncoder(&codec.output)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(codec.response); err != nil {
		return nil, fmt.Errorf("encode frame response: %w", err)
	}
	if size := len(codec.output.data); size != 0 && codec.output.data[size-1] == '\n' {
		codec.output.data = codec.output.data[:size-1]
	}
	return codec.output.data, nil
}

func (codec *JSONCodec) resetInput() {
	for i := range codec.input.Requests {
		evidenceIDs := codec.input.Requests[i].EvidenceIDs
		clear(evidenceIDs)
		codec.input.Requests[i] = decisioninput.Request{EvidenceIDs: evidenceIDs[:0]}
	}
	codec.input.Requests = codec.input.Requests[:0]
	clear(codec.input.Evidence)
	codec.input.Evidence = codec.input.Evidence[:0]
}

type boundedBuffer struct {
	data  []byte
	limit int
}

func (buffer *boundedBuffer) Reset() {
	buffer.data = buffer.data[:0]
}

func (buffer *boundedBuffer) Write(src []byte) (int, error) {
	if len(src) > buffer.limit-len(buffer.data) {
		return 0, fmt.Errorf("encoded response exceeds limit %d", buffer.limit)
	}
	buffer.data = append(buffer.data, src...)
	return len(src), nil
}

func truncateUTF8(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	value = value[:limit]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}
