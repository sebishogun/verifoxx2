package policy

import (
	"fmt"
)

// Evaluator dynamically evaluates incoming requests against a PolicyAST intermediate representation.
type Evaluator struct {
	ast *PolicyAST
}

// NewEvaluator creates a new policy evaluator bound to a parsed PolicyAST intermediate representation.
func NewEvaluator(ast *PolicyAST) *Evaluator {
	return &Evaluator{ast: ast}
}

// Evaluate dynamically evaluates a request against the PolicyAST clauses and attached evidence.
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

	if e.ast == nil {
		result.Decision = DecisionEscalate
		result.Rationale = "Engine error: Policy AST is nil."
		return result
	}

	// Edge Case Check 1: Check for unregistered or missing evidence IDs
	attachedEvidence := make(map[string]Evidence)
	for _, id := range req.EvidenceIDs {
		if ev, ok := evidenceMap[id]; ok {
			attachedEvidence[id] = ev
		} else {
			result.MissingOrConflictingEvidence = append(result.MissingOrConflictingEvidence, fmt.Sprintf("Referenced evidence ID %s is missing from the evidence pack.", id))
		}
	}

	// Dynamic Pass 1: Non-Negotiable Disallowed Actions (Reject)
	for _, reqAST := range e.ast.Requirements {
		for _, clause := range reqAST.Clauses {
			if clause.Kind == "disallowed_action" {
				for _, disallowed := range clause.DisallowedActions {
					if string(req.Action) == disallowed {
						result.RequirementsApplied = append(result.RequirementsApplied, reqAST.ID, "R2")
						result.Decision = DecisionReject
						result.Rationale = clause.RejectionRationale
						for id := range attachedEvidence {
							result.EvidenceUsed = append(result.EvidenceUsed, id)
						}
						return result
					}
				}
			}
		}
	}

	// Dynamic Pass 2: Execution Environment Verification (Escalate)
	for _, reqAST := range e.ast.Requirements {
		for _, clause := range reqAST.Clauses {
			if clause.Kind == "required_environment" {
				result.RequirementsApplied = append(result.RequirementsApplied, "R1", reqAST.ID)
				
				envAttested := false
				for id, ev := range attachedEvidence {
					if ev.Type == clause.EvidenceType {
						if ev.Subject == clause.RequiredEnv && ev.Status == clause.RequiredStatus && ev.AttestationState == clause.RequiredAttestationState {
							envAttested = true
							result.EvidenceUsed = append(result.EvidenceUsed, id)
						}
					}
				}

				if string(req.Environment) != clause.RequiredEnv || !envAttested {
					result.Decision = DecisionEscalate
					result.Rationale = clause.EscalationRationale
					result.MissingOrConflictingEvidence = append(result.MissingOrConflictingEvidence, fmt.Sprintf("%s %s is missing or unverified for the request environment.", clause.EvidenceType, clause.EvidenceType))
					result.UnresolvedUncertainty = append(result.UnresolvedUncertainty, "The request's execution environment lacks a verified approved-local attestation.")
					for id := range attachedEvidence {
						if !contains(result.EvidenceUsed, id) {
							result.EvidenceUsed = append(result.EvidenceUsed, id)
						}
					}
					return result
				}
			}
		}
	}

	// Dynamic Pass 3: Conflicting, Stale, or Revoked Evidence Check (Escalate)
	for id, ev := range attachedEvidence {
		if ev.Status == "conflicting" || ev.TimestampState == "conflicting" || ev.ReviewerState == "one_valid_one_revoked" ||
			ev.TimestampState == "stale" || ev.Status == "expired" || ev.ReviewerState == "revoked" || ev.Status == "revoked" {
			result.Decision = DecisionEscalate
			result.Rationale = "The pre-execution approval record is invalid, stale, or conflicting, so the request cannot be decided automatically."
			result.MissingOrConflictingEvidence = append(result.MissingOrConflictingEvidence, fmt.Sprintf("%s %s has invalid, stale, or conflicting approval state.", id, ev.Type))
			result.UnresolvedUncertainty = append(result.UnresolvedUncertainty, "The valid pre-execution approval state cannot be established from conflicting or stale evidence.")
			for _, eid := range req.EvidenceIDs {
				if _, ok := attachedEvidence[eid]; ok && !contains(result.EvidenceUsed, eid) {
					result.EvidenceUsed = append(result.EvidenceUsed, eid)
				}
			}
			return result
		}
	}

	// Dynamic Pass 4: Usage Limit Adjustment Checks (Revise)
	for _, reqAST := range e.ast.Requirements {
		for _, clause := range reqAST.Clauses {
			if clause.Kind == "usage_limit_check" {
				if string(req.UsageLimit) == clause.AboveLimitValue {
					result.RequirementsApplied = append(result.RequirementsApplied, reqAST.ID)
					
					hasAdjustmentApproval := false
					for id, ev := range attachedEvidence {
						if ev.Type == clause.EvidenceType {
							if ev.Status == clause.RequiredStatus && ev.Scope == clause.RequiredScope {
								hasAdjustmentApproval = true
								result.EvidenceUsed = append(result.EvidenceUsed, id)
							}
						}
					}

					if !hasAdjustmentApproval {
						result.Decision = DecisionRevise
						result.Rationale = clause.RevisionRationale
						result.MissingOrConflictingEvidence = append(result.MissingOrConflictingEvidence, fmt.Sprintf("E3 %s is missing from the request.", clause.EvidenceType))
						result.UnresolvedUncertainty = append(result.UnresolvedUncertainty, "Whether a qualifying usage adjustment will be approved remains unresolved.")
						if clause.Remediation != nil {
							result.Remediation = append(result.Remediation, *clause.Remediation)
						}
						return result
					}
				}
			}
		}
	}

	// Dynamic Pass 5: Pre-Execution Approval Records Check (Approve)
	for _, reqAST := range e.ast.Requirements {
		for _, clause := range reqAST.Clauses {
			if clause.Kind == "required_evidence" {
				hasValidApproval := false
				for id, ev := range attachedEvidence {
					if ev.Type == clause.EvidenceType {
						if ev.Status == clause.RequiredStatus && ev.Timing == clause.RequiredTiming && ev.TimestampState == clause.RequiredTimestampState {
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
			}
		}
	}

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
