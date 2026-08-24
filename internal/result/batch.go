package result

import (
	"fmt"

	"github.com/sebishogun/verifoxx2/internal/schema"
)

type DriverKind uint8

const (
	DriverNone DriverKind = iota
	DriverClause
	DriverSemantic
	DriverMissingReference
	DriverNoApplicableRequirement
)

type Batch struct {
	OutcomeIDs            []schema.OutcomeID
	DriverKinds           []DriverKind
	DriverRequirementIDs  []schema.RequirementID
	DriverClauseIDs       []schema.ClauseID
	DriverReasonIDs       []schema.ReasonID
	DriverFieldIDs        []schema.FieldID
	DriverEvidenceEdgeIDs []uint32
	IssueIDs              []schema.ReasonID
	RequirementOffsets    []uint32
	RequirementIDs        []schema.RequirementID
	EvidenceOffsets       []uint32
	EvidenceRefs          []uint32
	RemediationOffsets    []uint32
	RemediationIDs        []schema.RemediationID
}

type CapacityError struct {
	Column string
	Have   int
	Need   int
}

func (err *CapacityError) Error() string {
	return fmt.Sprintf("result column %s capacity %d is below required %d", err.Column, err.Have, err.Need)
}

func (batch *Batch) Ensure(rows uint32, requirementCapacity, evidenceCapacity, remediationCapacity int) {
	rowCount := int(rows)
	batch.OutcomeIDs = ensureLength(batch.OutcomeIDs, rowCount)
	batch.DriverKinds = ensureLength(batch.DriverKinds, rowCount)
	batch.DriverRequirementIDs = ensureLength(batch.DriverRequirementIDs, rowCount)
	batch.DriverClauseIDs = ensureLength(batch.DriverClauseIDs, rowCount)
	batch.DriverReasonIDs = ensureLength(batch.DriverReasonIDs, rowCount)
	batch.DriverFieldIDs = ensureLength(batch.DriverFieldIDs, rowCount)
	batch.DriverEvidenceEdgeIDs = ensureLength(batch.DriverEvidenceEdgeIDs, rowCount)
	batch.IssueIDs = ensureLength(batch.IssueIDs, rowCount)
	batch.RequirementOffsets = ensureLength(batch.RequirementOffsets, rowCount+1)
	batch.EvidenceOffsets = ensureLength(batch.EvidenceOffsets, rowCount+1)
	batch.RemediationOffsets = ensureLength(batch.RemediationOffsets, rowCount+1)
	batch.RequirementIDs = ensureEmptyCapacity(batch.RequirementIDs, requirementCapacity)
	batch.EvidenceRefs = ensureEmptyCapacity(batch.EvidenceRefs, evidenceCapacity)
	batch.RemediationIDs = ensureEmptyCapacity(batch.RemediationIDs, remediationCapacity)
}

func (batch *Batch) CheckCapacity(rows uint32, requirementCapacity, evidenceCapacity, remediationCapacity int) error {
	rowCount := int(rows)
	for _, column := range []struct {
		name string
		cap  int
		need int
	}{
		{"OutcomeIDs", cap(batch.OutcomeIDs), rowCount},
		{"DriverKinds", cap(batch.DriverKinds), rowCount},
		{"DriverRequirementIDs", cap(batch.DriverRequirementIDs), rowCount},
		{"DriverClauseIDs", cap(batch.DriverClauseIDs), rowCount},
		{"DriverReasonIDs", cap(batch.DriverReasonIDs), rowCount},
		{"DriverFieldIDs", cap(batch.DriverFieldIDs), rowCount},
		{"DriverEvidenceEdgeIDs", cap(batch.DriverEvidenceEdgeIDs), rowCount},
		{"IssueIDs", cap(batch.IssueIDs), rowCount},
		{"RequirementOffsets", cap(batch.RequirementOffsets), rowCount + 1},
		{"EvidenceOffsets", cap(batch.EvidenceOffsets), rowCount + 1},
		{"RemediationOffsets", cap(batch.RemediationOffsets), rowCount + 1},
		{"RequirementIDs", cap(batch.RequirementIDs), requirementCapacity},
		{"EvidenceRefs", cap(batch.EvidenceRefs), evidenceCapacity},
		{"RemediationIDs", cap(batch.RemediationIDs), remediationCapacity},
	} {
		if column.cap < column.need {
			return &CapacityError{Column: column.name, Have: column.cap, Need: column.need}
		}
	}
	return nil
}

func (batch *Batch) Reset(rows uint32) {
	rowCount := int(rows)
	batch.OutcomeIDs = resetLength(batch.OutcomeIDs, rowCount)
	batch.DriverKinds = resetLength(batch.DriverKinds, rowCount)
	batch.DriverRequirementIDs = resetLength(batch.DriverRequirementIDs, rowCount)
	batch.DriverClauseIDs = resetLength(batch.DriverClauseIDs, rowCount)
	batch.DriverReasonIDs = resetLength(batch.DriverReasonIDs, rowCount)
	batch.DriverFieldIDs = resetLength(batch.DriverFieldIDs, rowCount)
	batch.DriverEvidenceEdgeIDs = resetLength(batch.DriverEvidenceEdgeIDs, rowCount)
	batch.IssueIDs = resetLength(batch.IssueIDs, rowCount)
	batch.RequirementOffsets = resetLength(batch.RequirementOffsets, rowCount+1)
	batch.EvidenceOffsets = resetLength(batch.EvidenceOffsets, rowCount+1)
	batch.RemediationOffsets = resetLength(batch.RemediationOffsets, rowCount+1)
	batch.RequirementIDs = batch.RequirementIDs[:0]
	batch.EvidenceRefs = batch.EvidenceRefs[:0]
	batch.RemediationIDs = batch.RemediationIDs[:0]
}

func ensureLength[T any](slice []T, length int) []T {
	if cap(slice) < length {
		return make([]T, length)
	}
	return resetLength(slice, length)
}

func ensureEmptyCapacity[T any](slice []T, capacity int) []T {
	if cap(slice) < capacity {
		return make([]T, 0, capacity)
	}
	return slice[:0]
}

func resetLength[T any](slice []T, length int) []T {
	slice = slice[:length]
	clear(slice)
	return slice
}
