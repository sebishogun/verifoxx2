package policy

import (
	"fmt"
)

// Evaluator executes policy evaluation for requests against evidence maps.
type Evaluator struct{}

// NewEvaluator creates a new instance of the semantic policy evaluator.
func NewEvaluator() *Evaluator {
	return &Evaluator{}
}

// Evaluate evaluates a request given an available map of evidence records.
func (e *Evaluator) Evaluate(req Request, evidenceMap map[string]Evidence) EvaluationResult {
	result := EvaluationResult{
		RequestID:                     req.ID,
		RequirementsApplied:           []string{},
		EvidenceUsed:                  []string{},
		MissingOrConflictingEvidence: []string{},
		Assumptions: []string{
			"The supplied structured fields faithfully represent the underlying request and evidence records.",
		},
		UnresolvedUncertainty: []string{},
		Remediation:           []Remediation{},
	}

	// Index attached evidence items for this request
	attachedEvidence := make(map[string]Evidence)
	for _, id := range req.EvidenceIDs {
		if ev, ok := evidenceMap[id]; ok {
			attachedEvidence[id] = ev
		}
	}

	// Rule 1: Disclosure Restriction (Non-Negotiable)
	// Individual-level row exports from protected dataset are strictly forbidden.
	if req.Action == ActionRowLevelExport {
		result.RequirementsApplied = append(result.RequirementsApplied, "R1", "R2")
		result.Decision = DecisionReject
		result.Rationale = "The requested individual-record export violates R1's non-negotiable disclosure restriction."
		for id := range attachedEvidence {
			result.EvidenceUsed = append(result.EvidenceUsed, id)
		}
		return result
	}

	// Rule 2: Execution Environment Verification
	// Must run in approved local environment with verified attestation.
	result.RequirementsApplied = append(result.RequirementsApplied, "R1", "R2")

	envAttested := false
	for id, ev := range attachedEvidence {
		if ev.Type == "execution_environment_attestation" {
			if ev.Subject == string(EnvLocalApproved) && ev.Status == "verified" && ev.AttestationState == "valid" {
				envAttested = true
				result.EvidenceUsed = append(result.EvidenceUsed, id)
			}
		}
	}

	if req.Environment != EnvLocalApproved || !envAttested {
		result.Decision = DecisionEscalate
		result.Rationale = "The approved local execution environment cannot be verified because the required attestation is missing or unverified."
		result.MissingOrConflictingEvidence = append(result.MissingOrConflictingEvidence, "E2 execution_environment_attestation is missing or unverified for the request environment.")
		result.UnresolvedUncertainty = append(result.UnresolvedUncertainty, "The request's execution environment lacks a verified approved-local attestation.")
		for id := range attachedEvidence {
			if !contains(result.EvidenceUsed, id) {
				result.EvidenceUsed = append(result.EvidenceUsed, id)
			}
		}
		return result
	}

	// Check for conflicting or stale evidence across all attached evidence items
	for id, ev := range attachedEvidence {
		if ev.Status == "conflicting" || ev.TimestampState == "conflicting" || ev.ReviewerState == "one_valid_one_revoked" {
			result.Decision = DecisionEscalate
			result.Rationale = "The pre-execution approval record is conflicting, so the request cannot be decided automatically."
			result.MissingOrConflictingEvidence = append(result.MissingOrConflictingEvidence, fmt.Sprintf("%s %s has conflicting approval state.", id, ev.Type))
			result.UnresolvedUncertainty = append(result.UnresolvedUncertainty, "The valid pre-execution approval state cannot be established from conflicting evidence.")
			for _, eid := range req.EvidenceIDs {
				if !contains(result.EvidenceUsed, eid) {
					result.EvidenceUsed = append(result.EvidenceUsed, eid)
				}
			}
			return result
		}
	}

	// Rule 3: Usage Limit Adjustment for Internal Teams
	if req.UsageLimit == UsageAboveStandardLimit {
		result.RequirementsApplied = append(result.RequirementsApplied, "R3")
		
		hasAdjustmentApproval := false
		for id, ev := range attachedEvidence {
			if ev.Type == "usage_limit_adjustment" {
				if ev.Status == "approved" && ev.Scope == "trusted_internal_only" && ev.AdjustmentType == string(UsageAboveStandardLimit) {
					hasAdjustmentApproval = true
					result.EvidenceUsed = append(result.EvidenceUsed, id)
				}
			}
		}

		if !hasAdjustmentApproval {
			result.Decision = DecisionRevise
			result.Rationale = "The above-standard usage request can be corrected by providing the required scoped usage-adjustment approval."
			result.MissingOrConflictingEvidence = append(result.MissingOrConflictingEvidence, "E3 usage_limit_adjustment is missing from the request.")
			result.UnresolvedUncertainty = append(result.UnresolvedUncertainty, "Whether a qualifying usage adjustment will be approved remains unresolved.")
			result.Remediation = append(result.Remediation, Remediation{
				Action:       "add_evidence",
				EvidenceKind: "usage_limit_adjustment",
				Description:  "Provide a valid usage_limit_adjustment approval record scoped for trusted_internal_only.",
			})
			return result
		}
	}

	// Check Pre-execution Approval Record (R1 requirement)
	hasValidApproval := false
	for id, ev := range attachedEvidence {
		if ev.Type == "approval_record" {
			if ev.Status == "valid" && ev.Timing == "before_execution" && ev.TimestampState == "current" {
				hasValidApproval = true
				if !contains(result.EvidenceUsed, id) {
					result.EvidenceUsed = append(result.EvidenceUsed, id)
				}
			}
		}
	}

	if !hasValidApproval {
		result.Decision = DecisionEscalate
		result.Rationale = "Pre-execution approval evidence is missing or invalid."
		result.MissingOrConflictingEvidence = append(result.MissingOrConflictingEvidence, "Valid pre-execution approval record is missing.")
		return result
	}

	// If all requirements satisfied
	result.Decision = DecisionApprove
	result.Rationale = "The request satisfies all relevant requirements and supporting evidence."
	return result
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
