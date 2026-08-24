package ast

type Policy struct {
	Name         string        `json:"name"`
	Version      string        `json:"version"`
	Requirements []Requirement `json:"requirements"`
}

type Requirement struct {
	ID            string     `json:"id"`
	Description   string     `json:"description"`
	NonNegotiable bool       `json:"non_negotiable"`
	Applicability Expression `json:"applicability"`
	Clauses       []Clause   `json:"clauses"`
}

type Clause struct {
	ID           string        `json:"id"`
	Assertion    Expression    `json:"assertion"`
	Resolution   Resolution    `json:"resolution"`
	Explanations Explanations  `json:"explanations,omitempty"`
	Remediations []Remediation `json:"remediations,omitempty"`
}

type Outcome string

const (
	OutcomeApprove  Outcome = "Approve"
	OutcomeReject   Outcome = "Reject"
	OutcomeRevise   Outcome = "Revise"
	OutcomeEscalate Outcome = "Escalate"
)

type Resolution struct {
	Satisfied    Outcome `json:"satisfied"`
	False        Outcome `json:"false"`
	Missing      Outcome `json:"missing"`
	Invalid      Outcome `json:"invalid"`
	Stale        Outcome `json:"stale"`
	Unclear      Outcome `json:"unclear"`
	Unverifiable Outcome `json:"unverifiable"`
	Conflict     Outcome `json:"conflict"`
}

type Explanations struct {
	Satisfied    string `json:"satisfied,omitempty"`
	False        string `json:"false,omitempty"`
	Missing      string `json:"missing,omitempty"`
	Invalid      string `json:"invalid,omitempty"`
	Stale        string `json:"stale,omitempty"`
	Unclear      string `json:"unclear,omitempty"`
	Unverifiable string `json:"unverifiable,omitempty"`
	Conflict     string `json:"conflict,omitempty"`
}

type RemediationAction string

const (
	RemediationAddEvidence RemediationAction = "add_evidence"
	RemediationSetField    RemediationAction = "set_field"
)

type Remediation struct {
	Action       RemediationAction `json:"action"`
	EvidenceKind string            `json:"evidence_kind,omitempty"`
	Description  string            `json:"description,omitempty"`
	Field        string            `json:"field,omitempty"`
	Value        string            `json:"value,omitempty"`
}
