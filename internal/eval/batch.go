package eval

import "github.com/sebishogun/verifoxx2/internal/schema"

const (
	EvidencePresentStatus uint16 = 1 << iota
	EvidencePresentTiming
	EvidencePresentReviewer
	EvidencePresentTimestampState
	EvidencePresentSubject
	EvidencePresentAttestationState
	EvidencePresentScope
	EvidencePresentAdjustmentType
)

const (
	EvidenceFlagConflict uint8 = 1 << iota
	EvidenceFlagStale
	EvidenceFlagRevoked
)

type Batch struct {
	Rows  uint32
	Words uint32

	Values             []schema.SymbolID
	Present            []uint64
	Valid              []uint64
	SemanticIssueMasks []uint16

	EvidenceKinds            []schema.EvidenceKindID
	EvidenceStatus           []schema.SymbolID
	EvidenceTiming           []schema.SymbolID
	EvidenceReviewer         []schema.SymbolID
	EvidenceTimestampState   []schema.SymbolID
	EvidenceSubject          []schema.SymbolID
	EvidenceAttestationState []schema.SymbolID
	EvidenceScope            []schema.SymbolID
	EvidenceAdjustmentType   []schema.SymbolID
	EvidencePresent          []uint16
	EvidenceFlags            []uint8

	EvidenceRefOffsets []uint32
	EvidenceRefs       []uint32
}

func (b Batch) FieldValues(field schema.FieldID) []schema.SymbolID {
	if !field.Valid() || field >= schema.FieldCount || b.Rows == 0 {
		return nil
	}
	start := (int(field) - 1) * int(b.Rows)
	end := start + int(b.Rows)
	if start < 0 || end > len(b.Values) {
		return nil
	}
	return b.Values[start:end]
}

func (b Batch) FieldPresent(field schema.FieldID) []uint64 {
	return b.fieldPlane(b.Present, field)
}

func (b Batch) FieldValid(field schema.FieldID) []uint64 {
	return b.fieldPlane(b.Valid, field)
}

func (b Batch) fieldPlane(plane []uint64, field schema.FieldID) []uint64 {
	if !field.Valid() || field >= schema.FieldCount || b.Words == 0 {
		return nil
	}
	start := (int(field) - 1) * int(b.Words)
	end := start + int(b.Words)
	if start < 0 || end > len(plane) {
		return nil
	}
	return plane[start:end]
}
