package main

import (
	"fmt"
	"io"

	"github.com/sebishogun/verifoxx2/internal/adapters/framed"
	"github.com/sebishogun/verifoxx2/internal/engine"
)

func runStream(runtime *engine.Engine, stdin io.Reader, stdout, stderr io.Writer) int {
	processor := newStreamProcessor(runtime)
	var reader framed.FrameReader

	for {
		payload, ok, err := reader.Read(stdin)
		if err != nil {
			fmt.Fprintf(stderr, "Error reading stream: %v\n", err)
			return 1
		}
		if !ok {
			return 0
		}

		if err := processor.Process(payload, stdout); err != nil {
			fmt.Fprintf(stderr, "Error processing stream frame: %v\n", err)
			return 1
		}
	}
}

type streamProcessor struct {
	session *engine.Session
	codec   *framed.JSONCodec
	writer  framed.FrameWriter
}

func newStreamProcessor(runtime *engine.Engine) *streamProcessor {
	return &streamProcessor{
		session: runtime.NewSession(),
		codec:   framed.NewJSONCodec(framed.MaxOutputPayload),
	}
}

func (processor *streamProcessor) Process(payload []byte, destination io.Writer) error {
	frameInput, err := processor.codec.Decode(payload)
	if err != nil {
		return processor.writeInputError(destination, err)
	}
	output, err := processor.session.Evaluate(frameInput.Requests, frameInput.Evidence)
	if err != nil {
		if engine.IsInputError(err) {
			return processor.writeInputError(destination, err)
		}
		return fmt.Errorf("evaluate: %w", err)
	}
	response, err := processor.codec.EncodeSuccess(output)
	if err != nil {
		return fmt.Errorf("encode response: %w", err)
	}
	if err := processor.writer.Write(destination, response); err != nil {
		return fmt.Errorf("write response: %w", err)
	}
	return nil
}

func (processor *streamProcessor) writeInputError(destination io.Writer, inputError error) error {
	response, err := processor.codec.EncodeError("invalid_input", inputError.Error())
	if err != nil {
		return fmt.Errorf("encode input error response: %w", err)
	}
	if err := processor.writer.Write(destination, response); err != nil {
		return fmt.Errorf("write input error response: %w", err)
	}
	return nil
}
