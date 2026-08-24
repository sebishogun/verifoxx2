package program

import (
	"fmt"
	"strings"

	"github.com/sebishogun/verifoxx2/internal/schema"
)

func (p Program) Validate() error {
	if strings.TrimSpace(p.Name) == "" || strings.TrimSpace(p.Version) == "" {
		return fmt.Errorf("program name and version are required")
	}
	instructionCount := len(p.Opcodes)
	if instructionCount == 0 {
		return fmt.Errorf("program has no instructions")
	}
	for _, column := range []struct {
		name string
		len  int
	}{
		{"Fields", len(p.Fields)},
		{"Values", len(p.Values)},
		{"OperandStarts", len(p.OperandStarts)},
		{"OperandCounts", len(p.OperandCounts)},
		{"SetStarts", len(p.SetStarts)},
		{"SetCounts", len(p.SetCounts)},
		{"EvidenceSpecIndexes", len(p.EvidenceSpecIndexes)},
	} {
		if column.len != instructionCount {
			return fmt.Errorf("%s length %d does not match Opcodes length %d", column.name, column.len, instructionCount)
		}
	}

	if err := p.validateSymbols(); err != nil {
		return err
	}
	for i, op := range p.Opcodes {
		if !op.Valid() {
			return fmt.Errorf("Opcodes[%d] has invalid opcode %d", i, op)
		}
		if !validRange(p.OperandStarts[i], uint32(p.OperandCounts[i]), len(p.Operands)) {
			return fmt.Errorf("instruction %d operand range (%d,%d) exceeds Operands length %d", i, p.OperandStarts[i], p.OperandCounts[i], len(p.Operands))
		}
		if !validRange(p.SetStarts[i], uint32(p.SetCounts[i]), len(p.SetValues)) {
			return fmt.Errorf("instruction %d set range (%d,%d) exceeds SetValues length %d", i, p.SetStarts[i], p.SetCounts[i], len(p.SetValues))
		}

		switch op {
		case OpEqual:
			if !validField(p.Fields[i]) {
				return fmt.Errorf("Fields[%d] has invalid field ID %d", i, p.Fields[i])
			}
			if !p.validSymbol(p.Values[i], false) {
				return fmt.Errorf("Values[%d] has invalid symbol ID %d", i, p.Values[i])
			}
			if p.OperandCounts[i] != 0 || p.SetCounts[i] != 0 || p.EvidenceSpecIndexes[i] != 0 {
				return fmt.Errorf("Equal instruction %d has unrelated operands, set, or evidence spec", i)
			}
		case OpIn:
			if !validField(p.Fields[i]) {
				return fmt.Errorf("Fields[%d] has invalid field ID %d", i, p.Fields[i])
			}
			if p.SetCounts[i] == 0 {
				return fmt.Errorf("In instruction %d has an empty set", i)
			}
			start := int(p.SetStarts[i])
			for j := 0; j < int(p.SetCounts[i]); j++ {
				if !p.validSymbol(p.SetValues[start+j], false) {
					return fmt.Errorf("SetValues[%d] has invalid symbol ID %d", start+j, p.SetValues[start+j])
				}
			}
			if p.OperandCounts[i] != 0 || p.Values[i] != 0 || p.EvidenceSpecIndexes[i] != 0 {
				return fmt.Errorf("In instruction %d has unrelated operands, value, or evidence spec", i)
			}
		case OpEvidenceMatches:
			index := p.EvidenceSpecIndexes[i]
			if index == 0 || int(index) > len(p.EvidenceSpecs) {
				return fmt.Errorf("EvidenceSpecIndexes[%d] has invalid index %d", i, index)
			}
			if p.OperandCounts[i] != 0 || p.SetCounts[i] != 0 || p.Fields[i] != 0 || p.Values[i] != 0 {
				return fmt.Errorf("EvidenceMatches instruction %d has unrelated operands, set, field, or value", i)
			}
		case OpAll:
			if p.OperandCounts[i] == 0 {
				return fmt.Errorf("All instruction %d has no operands", i)
			}
			start := int(p.OperandStarts[i])
			for j := 0; j < int(p.OperandCounts[i]); j++ {
				operand := p.Operands[start+j]
				if !operand.Valid() || int(operand) >= i+1 {
					return fmt.Errorf("Operands[%d] instruction %d must precede consumer %d", start+j, operand, i+1)
				}
			}
			if p.SetCounts[i] != 0 || p.Fields[i] != 0 || p.Values[i] != 0 || p.EvidenceSpecIndexes[i] != 0 {
				return fmt.Errorf("All instruction %d has unrelated set, field, value, or evidence spec", i)
			}
		}
	}

	for i, spec := range p.EvidenceSpecs {
		if !spec.Kind.Valid() || int(spec.Kind) > len(p.EvidenceKindSymbols) {
			return fmt.Errorf("EvidenceSpecs[%d].Kind has invalid ID %d", i, spec.Kind)
		}
		for name, symbol := range map[string]schema.SymbolID{
			"Status": spec.Status, "Timing": spec.Timing, "Reviewer": spec.Reviewer,
			"TimestampState": spec.TimestampState, "Subject": spec.Subject,
			"AttestationState": spec.AttestationState, "Scope": spec.Scope,
			"AdjustmentType": spec.AdjustmentType,
		} {
			if !p.validSymbol(symbol, true) {
				return fmt.Errorf("EvidenceSpecs[%d].%s has invalid symbol ID %d", i, name, symbol)
			}
		}
	}
	for i, symbol := range p.EvidenceKindSymbols {
		if !p.validSymbol(symbol, false) {
			return fmt.Errorf("EvidenceKindSymbols[%d] has invalid symbol ID %d", i, symbol)
		}
	}
	if err := p.validateRequirementsAndClauses(); err != nil {
		return err
	}
	if err := p.validateCatalogues(); err != nil {
		return err
	}
	return nil
}

func (p Program) validateSymbols() error {
	if len(p.SymbolStarts) == 0 || len(p.SymbolStarts) != len(p.SymbolLengths) {
		return fmt.Errorf("SymbolStarts length %d does not match nonzero SymbolLengths length %d", len(p.SymbolStarts), len(p.SymbolLengths))
	}
	expectedStart := uint64(0)
	for i := range p.SymbolStarts {
		start := uint64(p.SymbolStarts[i])
		length := uint64(p.SymbolLengths[i])
		end := start + length
		if start != expectedStart || end < start || end > uint64(len(p.SymbolBytes)) || length == 0 {
			return fmt.Errorf("SymbolStarts[%d]/SymbolLengths[%d] range (%d,%d) is invalid for SymbolBytes length %d", i, i, start, length, len(p.SymbolBytes))
		}
		expectedStart = end
	}
	if expectedStart != uint64(len(p.SymbolBytes)) {
		return fmt.Errorf("symbol ranges cover %d bytes, want %d", expectedStart, len(p.SymbolBytes))
	}
	return nil
}

func (p Program) validateRequirementsAndClauses() error {
	requirementCount := len(p.RequirementSymbols)
	if requirementCount == 0 {
		return fmt.Errorf("program has no requirements")
	}
	for _, column := range []struct {
		name string
		len  int
	}{
		{"RequirementApplicabilityRoots", len(p.RequirementApplicabilityRoots)},
		{"RequirementClauseStarts", len(p.RequirementClauseStarts)},
		{"RequirementClauseCounts", len(p.RequirementClauseCounts)},
	} {
		if column.len != requirementCount {
			return fmt.Errorf("%s length %d does not match requirement count %d", column.name, column.len, requirementCount)
		}
	}
	clauseCount := len(p.ClauseSymbols)
	if clauseCount == 0 {
		return fmt.Errorf("program has no clauses")
	}
	for _, column := range []struct {
		name string
		len  int
	}{
		{"ClauseAssertionRoots", len(p.ClauseAssertionRoots)},
		{"ClauseRemediationStarts", len(p.ClauseRemediationStarts)},
		{"ClauseRemediationCounts", len(p.ClauseRemediationCounts)},
		{"ClauseEvidenceKindStarts", len(p.ClauseEvidenceKindStarts)},
		{"ClauseEvidenceKindCounts", len(p.ClauseEvidenceKindCounts)},
	} {
		if column.len != clauseCount {
			return fmt.Errorf("%s length %d does not match clause count %d", column.name, column.len, clauseCount)
		}
	}
	wantStates := clauseCount * int(schema.ReasonCount)
	if len(p.ClauseResolutionOutcomeIDs) != wantStates {
		return fmt.Errorf("ClauseResolutionOutcomeIDs length %d, want %d", len(p.ClauseResolutionOutcomeIDs), wantStates)
	}
	if len(p.ClauseExplanationIDs) != wantStates {
		return fmt.Errorf("ClauseExplanationIDs length %d, want %d", len(p.ClauseExplanationIDs), wantStates)
	}

	seenClauses := make([]bool, clauseCount)
	for i := 0; i < requirementCount; i++ {
		if !p.validSymbol(p.RequirementSymbols[i], false) {
			return fmt.Errorf("RequirementSymbols[%d] has invalid symbol ID %d", i, p.RequirementSymbols[i])
		}
		if !validInstruction(p.RequirementApplicabilityRoots[i], len(p.Opcodes)) {
			return fmt.Errorf("RequirementApplicabilityRoots[%d] has invalid instruction ID %d", i, p.RequirementApplicabilityRoots[i])
		}
		if !validRange(p.RequirementClauseStarts[i], uint32(p.RequirementClauseCounts[i]), len(p.RequirementClauseIDs)) {
			return fmt.Errorf("requirement %d clause range (%d,%d) exceeds RequirementClauseIDs length %d", i, p.RequirementClauseStarts[i], p.RequirementClauseCounts[i], len(p.RequirementClauseIDs))
		}
		start := int(p.RequirementClauseStarts[i])
		for j := 0; j < int(p.RequirementClauseCounts[i]); j++ {
			id := p.RequirementClauseIDs[start+j]
			if !id.Valid() || int(id) > clauseCount {
				return fmt.Errorf("RequirementClauseIDs[%d] has invalid clause ID %d", start+j, id)
			}
			if seenClauses[int(id)-1] {
				return fmt.Errorf("RequirementClauseIDs[%d] repeats clause ID %d", start+j, id)
			}
			seenClauses[int(id)-1] = true
		}
	}
	for i, seen := range seenClauses {
		if !seen {
			return fmt.Errorf("clause ID %d is unreferenced", i+1)
		}
	}

	for i := 0; i < clauseCount; i++ {
		if !p.validSymbol(p.ClauseSymbols[i], false) {
			return fmt.Errorf("ClauseSymbols[%d] has invalid symbol ID %d", i, p.ClauseSymbols[i])
		}
		if !validInstruction(p.ClauseAssertionRoots[i], len(p.Opcodes)) {
			return fmt.Errorf("ClauseAssertionRoots[%d] has invalid instruction ID %d", i, p.ClauseAssertionRoots[i])
		}
		if !validRange(p.ClauseRemediationStarts[i], uint32(p.ClauseRemediationCounts[i]), len(p.ClauseRemediationIDs)) {
			return fmt.Errorf("clause %d remediation range is invalid", i)
		}
		if !validRange(p.ClauseEvidenceKindStarts[i], uint32(p.ClauseEvidenceKindCounts[i]), len(p.ClauseEvidenceKinds)) {
			return fmt.Errorf("clause %d evidence-kind range is invalid", i)
		}
		for state := 0; state < int(schema.ReasonCount); state++ {
			index := i*int(schema.ReasonCount) + state
			outcome := p.ClauseResolutionOutcomeIDs[index]
			if !outcome.Valid() || outcome > schema.OutcomeCount {
				return fmt.Errorf("ClauseResolutionOutcomeIDs[%d] has invalid outcome ID %d", index, outcome)
			}
			explanation := p.ClauseExplanationIDs[index]
			if !p.validExplanation(explanation, outcome == schema.OutcomeApprove) {
				return fmt.Errorf("ClauseExplanationIDs[%d] has invalid explanation ID %d for outcome %d", index, explanation, outcome)
			}
		}
	}
	for i, id := range p.ClauseRemediationIDs {
		if !id.Valid() || int(id) > len(p.Remediations) {
			return fmt.Errorf("ClauseRemediationIDs[%d] has invalid remediation ID %d", i, id)
		}
	}
	for i, id := range p.ClauseEvidenceKinds {
		if !id.Valid() || int(id) > len(p.EvidenceKindSymbols) {
			return fmt.Errorf("ClauseEvidenceKinds[%d] has invalid evidence kind ID %d", i, id)
		}
	}
	return nil
}

func (p Program) validateCatalogues() error {
	for i, symbol := range p.ExplanationSymbols {
		if !p.validSymbol(symbol, false) {
			return fmt.Errorf("ExplanationSymbols[%d] has invalid symbol ID %d", i, symbol)
		}
	}
	for i, remediation := range p.Remediations {
		switch remediation.Action {
		case RemediationAddEvidence:
			if !remediation.EvidenceKind.Valid() || int(remediation.EvidenceKind) > len(p.EvidenceKindSymbols) || remediation.Field != 0 || remediation.Value != 0 {
				return fmt.Errorf("Remediations[%d] has invalid add-evidence payload", i)
			}
		case RemediationSetField:
			if !validField(remediation.Field) || !p.validSymbol(remediation.Value, false) || remediation.EvidenceKind != 0 {
				return fmt.Errorf("Remediations[%d] has invalid set-field payload", i)
			}
		default:
			return fmt.Errorf("Remediations[%d] has invalid action %d", i, remediation.Action)
		}
		if !p.validExplanation(remediation.Description, true) {
			return fmt.Errorf("Remediations[%d] has invalid description ID %d", i, remediation.Description)
		}
	}
	if len(p.OutcomePrecedence) != int(schema.OutcomeCount) {
		return fmt.Errorf("OutcomePrecedence length %d, want %d", len(p.OutcomePrecedence), schema.OutcomeCount)
	}
	seen := make([]bool, len(p.OutcomePrecedence)+1)
	for i, rank := range p.OutcomePrecedence {
		if rank == 0 || int(rank) > len(p.OutcomePrecedence) {
			return fmt.Errorf("OutcomePrecedence[%d] has invalid rank %d", i, rank)
		}
		if seen[rank] {
			return fmt.Errorf("OutcomePrecedence[%d] has duplicate precedence rank %d", i, rank)
		}
		seen[rank] = true
	}
	return nil
}

func (p Program) validSymbol(id schema.SymbolID, allowZero bool) bool {
	return (allowZero && id == 0) || (id.Valid() && int(id) <= len(p.SymbolStarts))
}

func (p Program) validExplanation(id schema.ExplanationID, allowZero bool) bool {
	return (allowZero && id == 0) || (id.Valid() && int(id) <= len(p.ExplanationSymbols))
}

func validField(id schema.FieldID) bool {
	return id.Valid() && id < schema.FieldCount
}

func validInstruction(id schema.InstructionID, count int) bool {
	return id.Valid() && int(id) <= count
}

func validRange(start, count uint32, total int) bool {
	end := uint64(start) + uint64(count)
	return end >= uint64(start) && end <= uint64(total)
}
