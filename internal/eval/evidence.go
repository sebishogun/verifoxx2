package eval

import (
	"fmt"

	"github.com/sebishogun/verifoxx2/internal/program"
	"github.com/sebishogun/verifoxx2/internal/schema"
)

func validateEvidenceBatch(batch Batch) error {
	evidenceCount := len(batch.EvidenceKinds)
	for _, column := range []struct {
		name string
		len  int
	}{
		{"EvidenceStatus", len(batch.EvidenceStatus)},
		{"EvidenceTiming", len(batch.EvidenceTiming)},
		{"EvidenceReviewer", len(batch.EvidenceReviewer)},
		{"EvidenceTimestampState", len(batch.EvidenceTimestampState)},
		{"EvidenceSubject", len(batch.EvidenceSubject)},
		{"EvidenceAttestationState", len(batch.EvidenceAttestationState)},
		{"EvidenceScope", len(batch.EvidenceScope)},
		{"EvidenceAdjustmentType", len(batch.EvidenceAdjustmentType)},
		{"EvidencePresent", len(batch.EvidencePresent)},
		{"EvidenceFlags", len(batch.EvidenceFlags)},
	} {
		if column.len != evidenceCount {
			return fmt.Errorf("%s length %d does not match evidence count %d", column.name, column.len, evidenceCount)
		}
	}
	if len(batch.EvidenceRefOffsets) != int(batch.Rows)+1 {
		return fmt.Errorf("EvidenceRefOffsets length %d does not match row count %d", len(batch.EvidenceRefOffsets), batch.Rows)
	}
	previous := uint32(0)
	for i, offset := range batch.EvidenceRefOffsets {
		if offset < previous || uint64(offset) > uint64(len(batch.EvidenceRefs)) {
			return fmt.Errorf("EvidenceRefOffsets[%d] = %d is not monotonic within %d edges", i, offset, len(batch.EvidenceRefs))
		}
		previous = offset
	}
	if previous != uint32(len(batch.EvidenceRefs)) {
		return fmt.Errorf("final evidence offset %d does not equal edge count %d", previous, len(batch.EvidenceRefs))
	}
	for i, ref := range batch.EvidenceRefs {
		if uint64(ref) > uint64(evidenceCount) {
			return fmt.Errorf("EvidenceRefs[%d] = %d exceeds evidence count %d", i, ref, evidenceCount)
		}
	}
	return nil
}

func evaluateEvidence(compiled program.Program, batch Batch, context *Context, instruction schema.InstructionID, index int) {
	spec := &compiled.EvidenceSpecs[int(compiled.EvidenceSpecIndexes[index])-1]
	positive := context.PositivePlane(instruction)
	negative := context.NegativePlane(instruction)
	for row := uint32(0); row < batch.Rows; row++ {
		start := batch.EvidenceRefOffsets[row]
		end := batch.EvidenceRefOffsets[row+1]
		conflict, stale, revoked := false, false, false
		unclear, satisfied, wrong := false, false, false
		for edge := start; edge < end; edge++ {
			ref := batch.EvidenceRefs[edge]
			if ref == 0 {
				continue
			}
			evidenceIndex := int(ref - 1)
			if batch.EvidenceKinds[evidenceIndex] != spec.Kind {
				continue
			}
			flags := batch.EvidenceFlags[evidenceIndex]
			switch {
			case flags&EvidenceFlagConflict != 0:
				conflict = true
			case flags&EvidenceFlagStale != 0:
				stale = true
			case flags&EvidenceFlagRevoked != 0:
				revoked = true
			default:
				switch classifyEvidenceQualifiers(batch, evidenceIndex, spec) {
				case qualifierSatisfied:
					satisfied = true
				case qualifierUnclear:
					unclear = true
				case qualifierWrong:
					wrong = true
				}
			}
		}

		reason := schema.ReasonMissing
		switch {
		case conflict:
			reason = schema.ReasonConflict
			setRow(positive, row)
			setRow(negative, row)
		case stale:
			reason = schema.ReasonStale
		case revoked:
			reason = schema.ReasonInvalidEvidence
		case unclear:
			reason = schema.ReasonUnclear
		case satisfied:
			reason = schema.ReasonSatisfied
			setRow(positive, row)
		case wrong:
			reason = schema.ReasonInvalidEvidence
		}
		setReason(context, reason, instruction, row)
	}
}

type qualifierResult uint8

const (
	qualifierSatisfied qualifierResult = iota
	qualifierUnclear
	qualifierWrong
)

func classifyEvidenceQualifiers(batch Batch, index int, spec *program.EvidenceSpec) qualifierResult {
	unclear := false
	if qualifierMismatch(batch.EvidenceStatus[index], batch.EvidencePresent[index]&EvidencePresentStatus != 0, spec.Status, &unclear) ||
		qualifierMismatch(batch.EvidenceTiming[index], batch.EvidencePresent[index]&EvidencePresentTiming != 0, spec.Timing, &unclear) ||
		qualifierMismatch(batch.EvidenceReviewer[index], batch.EvidencePresent[index]&EvidencePresentReviewer != 0, spec.Reviewer, &unclear) ||
		qualifierMismatch(batch.EvidenceTimestampState[index], batch.EvidencePresent[index]&EvidencePresentTimestampState != 0, spec.TimestampState, &unclear) ||
		qualifierMismatch(batch.EvidenceSubject[index], batch.EvidencePresent[index]&EvidencePresentSubject != 0, spec.Subject, &unclear) ||
		qualifierMismatch(batch.EvidenceAttestationState[index], batch.EvidencePresent[index]&EvidencePresentAttestationState != 0, spec.AttestationState, &unclear) ||
		qualifierMismatch(batch.EvidenceScope[index], batch.EvidencePresent[index]&EvidencePresentScope != 0, spec.Scope, &unclear) ||
		qualifierMismatch(batch.EvidenceAdjustmentType[index], batch.EvidencePresent[index]&EvidencePresentAdjustmentType != 0, spec.AdjustmentType, &unclear) {
		return qualifierWrong
	}
	if unclear {
		return qualifierUnclear
	}
	return qualifierSatisfied
}

func qualifierMismatch(have schema.SymbolID, present bool, want schema.SymbolID, unclear *bool) bool {
	if want == 0 {
		return false
	}
	if !present {
		*unclear = true
		return false
	}
	return have != want
}
