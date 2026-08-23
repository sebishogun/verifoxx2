package policy

// Decision represents the final outcome of evaluating a request against policy requirements.
type Decision string

const (
	DecisionApprove  Decision = "Approve"
	DecisionReject   Decision = "Reject"
	DecisionRevise   Decision = "Revise"
	DecisionEscalate Decision = "Escalate"
)

// RequesterTrust represents the trust level of the requester.
type RequesterTrust string

const (
	TrustExternal        RequesterTrust = "external"
	TrustTrustedInternal RequesterTrust = "trusted_internal"
)

// ActionKind represents the type of data processing requested.
type ActionKind string

const (
	ActionAggregateAnalysis ActionKind = "aggregate_analysis"
	ActionRowLevelExport    ActionKind = "row_level_export"
)

// UsageLimit represents the requested usage capacity.
type UsageLimit string

const (
	UsageStandard           UsageLimit = "standard"
	UsageAboveStandardLimit UsageLimit = "above_standard_limit"
)

// EnvironmentType represents the execution environment.
type EnvironmentType string

const (
	EnvLocalApproved    EnvironmentType = "local_approved_env"
	EnvUnverifiedRemote EnvironmentType = "unverified_remote_env"
)

// Request defines an incoming request to process protected dataset items.
type Request struct {
	ID          string          `json:"id"`
	Requester   string          `json:"requester"`
	TrustLevel  RequesterTrust  `json:"trust_level"`
	Action      ActionKind      `json:"action"`
	OutputKind  string          `json:"output_kind"`
	Dataset     string          `json:"dataset"`
	Environment EnvironmentType `json:"environment"`
	UsageLimit  UsageLimit      `json:"usage_limit"`
	EvidenceIDs []string        `json:"evidence_ids"`
}

// Evidence represents an attestation or approval document attached to a request.
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

// Remediation details a bounded change required to make a request acceptable under Revise.
type Remediation struct {
	Action       string `json:"action"`
	EvidenceKind string `json:"evidence_kind,omitempty"`
	Description  string `json:"description,omitempty"`
}

// EvaluationResult represents the machine-readable outcome for a single request.
type EvaluationResult struct {
	RequestID                     string        `json:"request_id"`
	Decision                      Decision      `json:"decision"`
	Rationale                     string        `json:"rationale"`
	RequirementsApplied           []string      `json:"requirements_applied"`
	EvidenceUsed                  []string      `json:"evidence_used"`
	MissingOrConflictingEvidence []string      `json:"missing_or_conflicting_evidence"`
	Assumptions                   []string      `json:"assumptions"`
	UnresolvedUncertainty         []string      `json:"unresolved_uncertainty"`
	Remediation                   []Remediation `json:"remediation,omitempty"`
}
