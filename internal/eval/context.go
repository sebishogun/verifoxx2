package eval

import (
	"github.com/sebishogun/verifoxx2/internal/program"
	"github.com/sebishogun/verifoxx2/internal/schema"
)

type Context struct {
	Positive []uint64
	Negative []uint64
	Reasons  []uint64

	Rows         uint32
	Words        uint32
	Instructions uint32
}

func (context *Context) Ensure(compiled program.Program, rows uint32) {
	words := (rows + 63) / 64
	instructions := uint32(len(compiled.Opcodes))
	truthLength := int(uint64(words) * uint64(instructions))
	reasonLength := int(uint64(schema.ReasonCount) * uint64(words) * uint64(instructions))
	context.Positive = resizeClear(context.Positive, truthLength)
	context.Negative = resizeClear(context.Negative, truthLength)
	context.Reasons = resizeClear(context.Reasons, reasonLength)
	context.Rows = rows
	context.Words = words
	context.Instructions = instructions
}

func (context *Context) PositivePlane(instruction schema.InstructionID) []uint64 {
	return context.instructionPlane(context.Positive, instruction)
}

func (context *Context) NegativePlane(instruction schema.InstructionID) []uint64 {
	return context.instructionPlane(context.Negative, instruction)
}

func (context *Context) ReasonPlane(reason schema.ReasonID, instruction schema.InstructionID) []uint64 {
	if !reason.Valid() || reason > schema.ReasonCount || !instruction.Valid() || uint32(instruction) > context.Instructions || context.Words == 0 {
		return nil
	}
	start := ((int(reason)-1)*int(context.Instructions) + int(instruction) - 1) * int(context.Words)
	return context.Reasons[start : start+int(context.Words)]
}

func (context *Context) instructionPlane(storage []uint64, instruction schema.InstructionID) []uint64 {
	if !instruction.Valid() || uint32(instruction) > context.Instructions || context.Words == 0 {
		return nil
	}
	start := (int(instruction) - 1) * int(context.Words)
	return storage[start : start+int(context.Words)]
}
