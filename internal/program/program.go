package program

import "github.com/sebishogun/verifoxx2/internal/schema"

type EvidenceSpec struct {
	Kind             schema.EvidenceKindID
	Status           schema.SymbolID
	Timing           schema.SymbolID
	Reviewer         schema.SymbolID
	TimestampState   schema.SymbolID
	Subject          schema.SymbolID
	AttestationState schema.SymbolID
	Scope            schema.SymbolID
	AdjustmentType   schema.SymbolID
}

type RemediationAction uint8

const (
	RemediationInvalid RemediationAction = iota
	RemediationAddEvidence
	RemediationSetField
)

type RemediationSpec struct {
	Action       RemediationAction
	EvidenceKind schema.EvidenceKindID
	Description  schema.ExplanationID
	Field        schema.FieldID
	Value        schema.SymbolID
}

type Program struct {
	Name    string
	Version string

	Opcodes             []Opcode
	Fields              []schema.FieldID
	Values              []schema.SymbolID
	OperandStarts       []uint32
	OperandCounts       []uint16
	Operands            []schema.InstructionID
	SetStarts           []uint32
	SetCounts           []uint16
	SetValues           []schema.SymbolID
	EvidenceSpecIndexes []uint16
	EvidenceSpecs       []EvidenceSpec

	EvidenceKindSymbols []schema.SymbolID

	RequirementSymbols            []schema.SymbolID
	RequirementApplicabilityRoots []schema.InstructionID
	RequirementClauseStarts       []uint32
	RequirementClauseCounts       []uint16
	RequirementClauseIDs          []schema.ClauseID

	ClauseSymbols              []schema.SymbolID
	ClauseAssertionRoots       []schema.InstructionID
	ClauseResolutionOutcomeIDs []schema.OutcomeID
	ClauseExplanationIDs       []schema.ExplanationID
	ClauseRemediationStarts    []uint32
	ClauseRemediationCounts    []uint16
	ClauseRemediationIDs       []schema.RemediationID
	ClauseEvidenceKindStarts   []uint32
	ClauseEvidenceKindCounts   []uint16
	ClauseEvidenceKinds        []schema.EvidenceKindID

	Remediations       []RemediationSpec
	ExplanationSymbols []schema.SymbolID
	OutcomePrecedence  []uint8

	SymbolBytes   []byte
	SymbolStarts  []uint32
	SymbolLengths []uint32
}
