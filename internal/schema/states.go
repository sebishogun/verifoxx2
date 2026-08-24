package schema

const (
	ReasonInvalid ReasonID = iota
	ReasonSatisfied
	ReasonFalse
	ReasonMissing
	ReasonInvalidEvidence
	ReasonStale
	ReasonUnclear
	ReasonUnverifiable
	ReasonConflict
	ReasonCount = ReasonConflict
)

const (
	OutcomeInvalid OutcomeID = iota
	OutcomeApprove
	OutcomeReject
	OutcomeRevise
	OutcomeEscalate
	OutcomeCount = OutcomeEscalate
)
