package input

type Request struct {
	ID          string   `json:"id"`
	Requester   string   `json:"requester"`
	TrustLevel  string   `json:"trust_level"`
	Action      string   `json:"action"`
	OutputKind  string   `json:"output_kind"`
	Dataset     string   `json:"dataset"`
	Environment string   `json:"environment"`
	UsageLimit  string   `json:"usage_limit"`
	EvidenceIDs []string `json:"evidence_ids"`
}

type Evidence struct {
	ID               string `json:"id"`
	Type             string `json:"type"`
	Status           string `json:"status,omitempty"`
	Timing           string `json:"timing,omitempty"`
	Reviewer         string `json:"reviewer,omitempty"`
	ReviewerState    string `json:"reviewer_state,omitempty"`
	TimestampState   string `json:"timestamp_state,omitempty"`
	Subject          string `json:"subject,omitempty"`
	AttestationState string `json:"attestation_state,omitempty"`
	Scope            string `json:"scope,omitempty"`
	AdjustmentType   string `json:"adjustment_type,omitempty"`
}
