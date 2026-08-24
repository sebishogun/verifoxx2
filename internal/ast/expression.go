package ast

type Operator string

const (
	OperatorAll             Operator = "all"
	OperatorEqual           Operator = "equal"
	OperatorIn              Operator = "in"
	OperatorEvidenceMatches Operator = "evidence_matches"
)

type Expression struct {
	Op       Operator          `json:"op"`
	Field    string            `json:"field,omitempty"`
	Value    string            `json:"value,omitempty"`
	Values   []string          `json:"values,omitempty"`
	Children []Expression      `json:"children,omitempty"`
	Evidence EvidencePredicate `json:"evidence,omitempty"`
}

type EvidencePredicate struct {
	Kind             string `json:"kind"`
	Status           string `json:"status,omitempty"`
	Timing           string `json:"timing,omitempty"`
	Reviewer         string `json:"reviewer,omitempty"`
	TimestampState   string `json:"timestamp_state,omitempty"`
	Subject          string `json:"subject,omitempty"`
	AttestationState string `json:"attestation_state,omitempty"`
	Scope            string `json:"scope,omitempty"`
	AdjustmentType   string `json:"adjustment_type,omitempty"`
}
