package eval

import (
	"fmt"

	"github.com/sebishogun/verifoxx2/internal/program"
	"github.com/sebishogun/verifoxx2/internal/schema"
)

func EvaluateInstructions(compiled program.Program, batch Batch, context *Context) error {
	if context == nil {
		return fmt.Errorf("evaluation context is nil")
	}
	words := (batch.Rows + 63) / 64
	instructions := uint32(len(compiled.Opcodes))
	truthLength := int(uint64(words) * uint64(instructions))
	reasonLength := int(uint64(schema.ReasonCount) * uint64(words) * uint64(instructions))
	if context.Rows != batch.Rows || context.Words != words || context.Instructions != instructions || len(context.Positive) != truthLength || len(context.Negative) != truthLength || len(context.Reasons) != reasonLength {
		return fmt.Errorf("evaluation context capacity does not match %d rows and %d instructions", batch.Rows, instructions)
	}
	fieldCount := int(schema.FieldCount - 1)
	if len(batch.Values) != fieldCount*int(batch.Rows) || len(batch.Present) != fieldCount*int(words) || len(batch.Valid) != fieldCount*int(words) || len(batch.SemanticIssueMasks) != int(batch.Rows) {
		return fmt.Errorf("batch request columns do not match declared row shape")
	}
	if err := validateEvidenceBatch(batch); err != nil {
		return err
	}
	clear(context.Positive)
	clear(context.Negative)
	clear(context.Reasons)
	if batch.Rows == 0 {
		return nil
	}

	for index, opcode := range compiled.Opcodes {
		instruction := schema.InstructionID(index + 1)
		switch opcode {
		case program.OpEqual:
			evaluateEqual(compiled, batch, context, instruction, index)
		case program.OpIn:
			evaluateIn(compiled, batch, context, instruction, index)
		case program.OpAll:
			evaluateAll(compiled, batch, context, instruction, index)
		case program.OpEvidenceMatches:
			evaluateEvidence(compiled, batch, context, instruction, index)
		default:
			return fmt.Errorf("instruction %d has invalid opcode %d", instruction, opcode)
		}
	}
	return nil
}

func evaluateEqual(compiled program.Program, batch Batch, context *Context, instruction schema.InstructionID, index int) {
	field := compiled.Fields[index]
	values := batch.FieldValues(field)
	present := batch.FieldPresent(field)
	valid := batch.FieldValid(field)
	positive := context.PositivePlane(instruction)
	negative := context.NegativePlane(instruction)
	want := compiled.Values[index]
	for row := uint32(0); row < batch.Rows; row++ {
		switch {
		case !hasRow(present, row):
			setReason(context, schema.ReasonMissing, instruction, row)
		case !hasRow(valid, row):
			setReason(context, schema.ReasonUnclear, instruction, row)
		case values[row] == want:
			setRow(positive, row)
			setReason(context, schema.ReasonSatisfied, instruction, row)
		default:
			setRow(negative, row)
			setReason(context, schema.ReasonFalse, instruction, row)
		}
	}
}

func evaluateIn(compiled program.Program, batch Batch, context *Context, instruction schema.InstructionID, index int) {
	field := compiled.Fields[index]
	values := batch.FieldValues(field)
	present := batch.FieldPresent(field)
	valid := batch.FieldValid(field)
	positive := context.PositivePlane(instruction)
	negative := context.NegativePlane(instruction)
	setStart := int(compiled.SetStarts[index])
	set := compiled.SetValues[setStart : setStart+int(compiled.SetCounts[index])]
	for row := uint32(0); row < batch.Rows; row++ {
		if !hasRow(present, row) {
			setReason(context, schema.ReasonMissing, instruction, row)
			continue
		}
		if !hasRow(valid, row) {
			setReason(context, schema.ReasonUnclear, instruction, row)
			continue
		}
		matched := false
		for _, candidate := range set {
			if values[row] == candidate {
				matched = true
				break
			}
		}
		if matched {
			setRow(positive, row)
			setReason(context, schema.ReasonSatisfied, instruction, row)
		} else {
			setRow(negative, row)
			setReason(context, schema.ReasonFalse, instruction, row)
		}
	}
}

func evaluateAll(compiled program.Program, batch Batch, context *Context, instruction schema.InstructionID, index int) {
	positive := context.PositivePlane(instruction)
	negative := context.NegativePlane(instruction)
	operandStart := int(compiled.OperandStarts[index])
	operands := compiled.Operands[operandStart : operandStart+int(compiled.OperandCounts[index])]
	for word := range positive {
		allPositive := ^uint64(0)
		anyNegative := uint64(0)
		for _, operand := range operands {
			allPositive &= context.PositivePlane(operand)[word]
			anyNegative |= context.NegativePlane(operand)[word]
		}
		positive[word] = allPositive
		negative[word] = anyNegative
	}
	mask := tailMask(batch.Rows)
	positive[len(positive)-1] &= mask
	negative[len(negative)-1] &= mask

	priority := [...]schema.ReasonID{
		schema.ReasonConflict,
		schema.ReasonStale,
		schema.ReasonInvalidEvidence,
		schema.ReasonUnclear,
		schema.ReasonMissing,
		schema.ReasonUnverifiable,
	}
	for row := uint32(0); row < batch.Rows; row++ {
		pos := hasRow(positive, row)
		neg := hasRow(negative, row)
		switch {
		case pos && neg:
			setReason(context, schema.ReasonConflict, instruction, row)
		case neg:
			setReason(context, schema.ReasonFalse, instruction, row)
		case pos:
			setReason(context, schema.ReasonSatisfied, instruction, row)
		default:
			selected := schema.ReasonUnverifiable
			found := false
			for _, reason := range priority {
				for _, operand := range operands {
					if hasRow(context.ReasonPlane(reason, operand), row) {
						selected = reason
						found = true
						break
					}
				}
				if found {
					break
				}
			}
			setReason(context, selected, instruction, row)
		}
	}
}
