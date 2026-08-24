package eval

import (
	"fmt"

	"github.com/sebishogun/verifoxx2/internal/program"
	"github.com/sebishogun/verifoxx2/internal/result"
	"github.com/sebishogun/verifoxx2/internal/schema"
)

type Evaluator struct {
	program *program.Program
}

func NewEvaluator(compiled *program.Program) Evaluator {
	return Evaluator{program: compiled}
}

func (evaluator Evaluator) Program() program.Program {
	if evaluator.program == nil {
		return program.Program{}
	}
	return *evaluator.program
}

func (evaluator Evaluator) EvaluateInto(context *Context, batch Batch, dst *result.Batch) error {
	if evaluator.program == nil {
		return fmt.Errorf("compiled program is nil")
	}
	if dst == nil {
		return fmt.Errorf("result destination is nil")
	}
	rows := uint64(batch.Rows)
	requirementCapacity := rows * uint64(len(evaluator.program.RequirementSymbols))
	remediationCapacity := rows * uint64(len(evaluator.program.Remediations))
	maxInt := uint64(^uint(0) >> 1)
	if requirementCapacity > maxInt || remediationCapacity > maxInt {
		return fmt.Errorf("result capacity calculation overflows addressable memory")
	}
	if err := dst.CheckCapacity(batch.Rows, int(requirementCapacity), len(batch.EvidenceRefs), int(remediationCapacity)); err != nil {
		return err
	}
	if err := EvaluateInstructions(*evaluator.program, batch, context); err != nil {
		return err
	}

	dst.Reset(batch.Rows)
	for row := uint32(0); row < batch.Rows; row++ {
		evaluator.resolveRow(context, batch, dst, row)
	}
	return nil
}

func (evaluator Evaluator) resolveRow(context *Context, batch Batch, dst *result.Batch, row uint32) {
	compiled := evaluator.program
	winnerOutcome := schema.OutcomeApprove
	winnerKind := result.DriverNone
	winnerRequirement := schema.RequirementID(0)
	winnerClause := schema.ClauseID(0)
	winnerReason := schema.ReasonSatisfied
	winnerField := schema.FieldID(0)
	winnerEdge := uint32(0)
	winnerRank := compiled.OutcomePrecedence[int(schema.OutcomeApprove)-1]

	semanticMask := batch.SemanticIssueMasks[row]
	for field := schema.FieldRequester; field < schema.FieldCount; field++ {
		if semanticMask&(uint16(1)<<(field-1)) == 0 {
			continue
		}
		reason := schema.ReasonUnclear
		if !hasRow(batch.FieldPresent(field), row) {
			reason = schema.ReasonMissing
		}
		rank := compiled.OutcomePrecedence[int(schema.OutcomeEscalate)-1]
		if rank > winnerRank {
			winnerOutcome = schema.OutcomeEscalate
			winnerKind = result.DriverSemantic
			winnerReason = reason
			winnerField = field
			winnerRank = rank
		}
		break
	}

	edgeStart := batch.EvidenceRefOffsets[row]
	edgeEnd := batch.EvidenceRefOffsets[row+1]
	for edge := edgeStart; edge < edgeEnd; edge++ {
		if batch.EvidenceRefs[edge] != 0 {
			continue
		}
		rank := compiled.OutcomePrecedence[int(schema.OutcomeEscalate)-1]
		if rank > winnerRank {
			winnerOutcome = schema.OutcomeEscalate
			winnerKind = result.DriverMissingReference
			winnerReason = schema.ReasonMissing
			winnerEdge = edge + 1
			winnerRank = rank
		}
	}

	dst.RequirementOffsets[row] = uint32(len(dst.RequirementIDs))
	applicableCount := 0
	for requirementIndex, root := range compiled.RequirementApplicabilityRoots {
		if !hasRow(context.PositivePlane(root), row) || hasRow(context.NegativePlane(root), row) {
			continue
		}
		requirementID := schema.RequirementID(requirementIndex + 1)
		dst.RequirementIDs = append(dst.RequirementIDs, requirementID)
		applicableCount++
		clauseStart := int(compiled.RequirementClauseStarts[requirementIndex])
		clauseEnd := clauseStart + int(compiled.RequirementClauseCounts[requirementIndex])
		for _, clauseID := range compiled.RequirementClauseIDs[clauseStart:clauseEnd] {
			clauseIndex := int(clauseID) - 1
			reason := instructionReason(context, compiled.ClauseAssertionRoots[clauseIndex], row)
			resolutionIndex := clauseIndex*int(schema.ReasonCount) + int(reason) - 1
			outcome := compiled.ClauseResolutionOutcomeIDs[resolutionIndex]
			rank := compiled.OutcomePrecedence[int(outcome)-1]
			if rank > winnerRank {
				winnerOutcome = outcome
				winnerKind = result.DriverClause
				winnerRequirement = requirementID
				winnerClause = clauseID
				winnerReason = reason
				winnerField = 0
				winnerEdge = 0
				winnerRank = rank
			}
		}
	}
	dst.RequirementOffsets[row+1] = uint32(len(dst.RequirementIDs))
	if applicableCount == 0 {
		rank := compiled.OutcomePrecedence[int(schema.OutcomeEscalate)-1]
		if rank > winnerRank {
			winnerOutcome = schema.OutcomeEscalate
			winnerKind = result.DriverNoApplicableRequirement
			winnerReason = schema.ReasonUnverifiable
			winnerRank = rank
		}
	}

	dst.EvidenceOffsets[row] = uint32(len(dst.EvidenceRefs))
	for edge := edgeStart; edge < edgeEnd; edge++ {
		ref := batch.EvidenceRefs[edge]
		if ref == 0 || !evaluator.evidenceRequiredForRow(batch, dst, row, ref) {
			continue
		}
		duplicate := false
		for i := dst.EvidenceOffsets[row]; i < uint32(len(dst.EvidenceRefs)); i++ {
			if dst.EvidenceRefs[i] == ref {
				duplicate = true
				break
			}
		}
		if !duplicate {
			dst.EvidenceRefs = append(dst.EvidenceRefs, ref)
		}
	}
	dst.EvidenceOffsets[row+1] = uint32(len(dst.EvidenceRefs))

	dst.RemediationOffsets[row] = uint32(len(dst.RemediationIDs))
	if winnerOutcome == schema.OutcomeRevise && winnerKind == result.DriverClause {
		clauseIndex := int(winnerClause) - 1
		start := int(compiled.ClauseRemediationStarts[clauseIndex])
		end := start + int(compiled.ClauseRemediationCounts[clauseIndex])
		dst.RemediationIDs = append(dst.RemediationIDs, compiled.ClauseRemediationIDs[start:end]...)
	}
	dst.RemediationOffsets[row+1] = uint32(len(dst.RemediationIDs))

	dst.OutcomeIDs[row] = winnerOutcome
	dst.DriverKinds[row] = winnerKind
	dst.DriverRequirementIDs[row] = winnerRequirement
	dst.DriverClauseIDs[row] = winnerClause
	dst.DriverReasonIDs[row] = winnerReason
	dst.DriverFieldIDs[row] = winnerField
	dst.DriverEvidenceEdgeIDs[row] = winnerEdge
	if winnerOutcome != schema.OutcomeApprove {
		dst.IssueIDs[row] = winnerReason
	}
}

func (evaluator Evaluator) evidenceRequiredForRow(batch Batch, dst *result.Batch, row, ref uint32) bool {
	kind := batch.EvidenceKinds[ref-1]
	if !kind.Valid() {
		return false
	}
	compiled := evaluator.program
	for i := dst.RequirementOffsets[row]; i < dst.RequirementOffsets[row+1]; i++ {
		requirementIndex := int(dst.RequirementIDs[i]) - 1
		clauseStart := int(compiled.RequirementClauseStarts[requirementIndex])
		clauseEnd := clauseStart + int(compiled.RequirementClauseCounts[requirementIndex])
		for _, clauseID := range compiled.RequirementClauseIDs[clauseStart:clauseEnd] {
			clauseIndex := int(clauseID) - 1
			kindStart := int(compiled.ClauseEvidenceKindStarts[clauseIndex])
			kindEnd := kindStart + int(compiled.ClauseEvidenceKindCounts[clauseIndex])
			for _, requiredKind := range compiled.ClauseEvidenceKinds[kindStart:kindEnd] {
				if kind == requiredKind {
					return true
				}
			}
		}
	}
	return false
}

func instructionReason(context *Context, instruction schema.InstructionID, row uint32) schema.ReasonID {
	for reason := schema.ReasonSatisfied; reason <= schema.ReasonCount; reason++ {
		if hasRow(context.ReasonPlane(reason, instruction), row) {
			return reason
		}
	}
	return schema.ReasonUnverifiable
}
