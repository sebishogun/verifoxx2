package schema

type FieldID uint16
type NodeID uint32
type InstructionID uint32
type SymbolID uint32
type ValueID uint32
type EvidenceKindID uint16
type OutcomeID uint16
type ReasonID uint8
type RequirementID uint16
type ClauseID uint16
type RemediationID uint16
type ExplanationID uint16

func (id FieldID) Valid() bool        { return id != 0 }
func (id NodeID) Valid() bool         { return id != 0 }
func (id InstructionID) Valid() bool  { return id != 0 }
func (id SymbolID) Valid() bool       { return id != 0 }
func (id ValueID) Valid() bool        { return id != 0 }
func (id EvidenceKindID) Valid() bool { return id != 0 }
func (id OutcomeID) Valid() bool      { return id != 0 }
func (id ReasonID) Valid() bool       { return id != 0 }
func (id RequirementID) Valid() bool  { return id != 0 }
func (id ClauseID) Valid() bool       { return id != 0 }
func (id RemediationID) Valid() bool  { return id != 0 }
func (id ExplanationID) Valid() bool  { return id != 0 }
