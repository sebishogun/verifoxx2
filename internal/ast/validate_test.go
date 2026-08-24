package ast

import (
	"strings"
	"testing"
)

func approveResolution() Resolution {
	return Resolution{
		Satisfied:    OutcomeApprove,
		False:        OutcomeApprove,
		Missing:      OutcomeApprove,
		Invalid:      OutcomeApprove,
		Stale:        OutcomeApprove,
		Unclear:      OutcomeApprove,
		Unverifiable: OutcomeApprove,
		Conflict:     OutcomeApprove,
	}
}

func validPolicy() Policy {
	return Policy{
		Name:    "test-policy",
		Version: "1.0.0",
		Requirements: []Requirement{{
			ID: "R1",
			Applicability: Expression{
				Op:    OperatorEqual,
				Field: "dataset",
				Value: "protected_dataset",
			},
			Clauses: []Clause{{
				ID: "R1_C1",
				Assertion: Expression{
					Op:     OperatorIn,
					Field:  "action",
					Values: []string{"aggregate_analysis"},
				},
				Resolution: approveResolution(),
			}},
		}},
	}
}

func requireValidationError(t *testing.T, policy Policy, want string) {
	t.Helper()
	err := policy.Validate()
	if err == nil {
		t.Fatalf("Validate() succeeded, want error containing %q", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("Validate() error = %q, want substring %q", err, want)
	}
}

func TestPolicyValidateAcceptsGenericExpressions(t *testing.T) {
	policy := validPolicy()
	policy.Requirements[0].Applicability = Expression{
		Op: OperatorAll,
		Children: []Expression{
			{Op: OperatorEqual, Field: "dataset", Value: "protected_dataset"},
			{Op: OperatorIn, Field: "trust_level", Values: []string{"external", "trusted_internal"}},
		},
	}
	policy.Requirements[0].Clauses[0].Assertion = Expression{
		Op: OperatorAll,
		Children: []Expression{
			{Op: OperatorEqual, Field: "environment", Value: "local_approved_env"},
			{
				Op: OperatorEvidenceMatches,
				Evidence: EvidencePredicate{
					Kind:             "execution_environment_attestation",
					Status:           "verified",
					Subject:          "local_approved_env",
					AttestationState: "valid",
				},
			},
		},
	}

	if err := policy.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestPolicyValidateRejectsMalformedExpressions(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Policy)
		want string
	}{
		{
			name: "unsupported operator",
			edit: func(p *Policy) { p.Requirements[0].Clauses[0].Assertion.Op = Operator("any") },
			want: "unsupported operator",
		},
		{
			name: "all without children",
			edit: func(p *Policy) { p.Requirements[0].Clauses[0].Assertion = Expression{Op: OperatorAll} },
			want: "all requires at least one child",
		},
		{
			name: "equal with values",
			edit: func(p *Policy) {
				p.Requirements[0].Clauses[0].Assertion = Expression{Op: OperatorEqual, Field: "action", Value: "aggregate_analysis", Values: []string{"row_level_export"}}
			},
			want: "equal does not allow values",
		},
		{
			name: "in without values",
			edit: func(p *Policy) {
				p.Requirements[0].Clauses[0].Assertion = Expression{Op: OperatorIn, Field: "action"}
			},
			want: "in requires at least one value",
		},
		{
			name: "empty in value",
			edit: func(p *Policy) {
				p.Requirements[0].Clauses[0].Assertion = Expression{Op: OperatorIn, Field: "action", Values: []string{""}}
			},
			want: "empty value",
		},
		{
			name: "unknown field",
			edit: func(p *Policy) { p.Requirements[0].Clauses[0].Assertion.Field = "mystery" },
			want: "unknown field",
		},
		{
			name: "unknown field value",
			edit: func(p *Policy) { p.Requirements[0].Clauses[0].Assertion.Values = []string{"delete_everything"} },
			want: "unknown value",
		},
		{
			name: "evidence in applicability",
			edit: func(p *Policy) {
				p.Requirements[0].Applicability = Expression{Op: OperatorEvidenceMatches, Evidence: EvidencePredicate{Kind: "approval_record"}}
			},
			want: "evidence_matches is not allowed in applicability",
		},
		{
			name: "evidence without kind",
			edit: func(p *Policy) {
				p.Requirements[0].Clauses[0].Assertion = Expression{Op: OperatorEvidenceMatches}
			},
			want: "evidence_matches requires evidence.kind",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := validPolicy()
			tt.edit(&policy)
			requireValidationError(t, policy, tt.want)
		})
	}
}

func TestPolicyValidateRejectsDuplicateAndEmptyIDs(t *testing.T) {
	policy := validPolicy()
	policy.Requirements[0].ID = "   "
	requireValidationError(t, policy, "empty id")

	policy = validPolicy()
	policy.Requirements = append(policy.Requirements, policy.Requirements[0])
	requireValidationError(t, policy, "duplicate requirement id")

	policy = validPolicy()
	policy.Requirements[0].Clauses = append(policy.Requirements[0].Clauses, policy.Requirements[0].Clauses[0])
	requireValidationError(t, policy, "duplicate clause id")
}

func TestPolicyValidateRequiresCompleteResolutionAndExplanations(t *testing.T) {
	policy := validPolicy()
	policy.Requirements[0].Clauses[0].Resolution.Conflict = ""
	requireValidationError(t, policy, "resolution.conflict")

	policy = validPolicy()
	policy.Requirements[0].Clauses[0].Resolution.Missing = OutcomeEscalate
	requireValidationError(t, policy, "explanations.missing")

	policy.Requirements[0].Clauses[0].Explanations.Missing = "Evidence is missing."
	if err := policy.Validate(); err != nil {
		t.Fatalf("Validate() with bounded explanation error = %v", err)
	}
}

func TestPolicyValidateRequiresStructuredReferencedRemediation(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Clause)
		want string
	}{
		{
			name: "revise without remediation",
			edit: func(c *Clause) {
				c.Resolution.False = OutcomeRevise
				c.Explanations.False = "Change the request."
			},
			want: "Revise requires remediation",
		},
		{
			name: "unreferenced remediation",
			edit: func(c *Clause) {
				c.Remediations = []Remediation{{Action: RemediationSetField, Field: "usage_limit", Value: "standard"}}
			},
			want: "remediation without a Revise resolution",
		},
		{
			name: "set field unknown",
			edit: func(c *Clause) {
				c.Resolution.False = OutcomeRevise
				c.Explanations.False = "Change the request."
				c.Remediations = []Remediation{{Action: RemediationSetField, Field: "mystery", Value: "standard"}}
			},
			want: "unknown field",
		},
		{
			name: "add evidence without kind",
			edit: func(c *Clause) {
				c.Resolution.False = OutcomeRevise
				c.Explanations.False = "Add evidence."
				c.Remediations = []Remediation{{Action: RemediationAddEvidence}}
			},
			want: "requires evidence_kind",
		},
		{
			name: "add evidence unknown kind",
			edit: func(c *Clause) {
				c.Resolution.False = OutcomeRevise
				c.Explanations.False = "Add evidence."
				c.Remediations = []Remediation{{Action: RemediationAddEvidence, EvidenceKind: "manager_note"}}
			},
			want: "unknown evidence_kind",
		},
		{
			name: "set field unknown value",
			edit: func(c *Clause) {
				c.Resolution.False = OutcomeRevise
				c.Explanations.False = "Change the request."
				c.Remediations = []Remediation{{Action: RemediationSetField, Field: "usage_limit", Value: "unbounded"}}
			},
			want: "unknown value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := validPolicy()
			tt.edit(&policy.Requirements[0].Clauses[0])
			requireValidationError(t, policy, tt.want)
		})
	}
}

func TestPolicyValidateBoundsExpressionDepth(t *testing.T) {
	policy := validPolicy()
	expr := Expression{Op: OperatorEqual, Field: "dataset", Value: "protected_dataset"}
	for range MaxExpressionDepth {
		expr = Expression{Op: OperatorAll, Children: []Expression{expr}}
	}
	policy.Requirements[0].Clauses[0].Assertion = expr
	requireValidationError(t, policy, "maximum depth")
}

func TestPolicyValidateBoundsExpressionCountAndText(t *testing.T) {
	policy := validPolicy()
	children := make([]Expression, MaxExpressionNodes)
	for i := range children {
		children[i] = Expression{Op: OperatorEqual, Field: "dataset", Value: "protected_dataset"}
	}
	policy.Requirements[0].Clauses[0].Assertion = Expression{Op: OperatorAll, Children: children}
	requireValidationError(t, policy, "maximum policy expression count")

	policy = validPolicy()
	policy.Requirements[0].Clauses[0].Resolution.Missing = OutcomeEscalate
	policy.Requirements[0].Clauses[0].Explanations.Missing = strings.Repeat("x", MaxExplanationBytes+1)
	requireValidationError(t, policy, "exceeds 1024 bytes")

	policy = validPolicy()
	clause := &policy.Requirements[0].Clauses[0]
	clause.Resolution.False = OutcomeRevise
	clause.Explanations.False = "Change it."
	clause.Remediations = []Remediation{{
		Action: RemediationAddEvidence, EvidenceKind: "approval_record",
		Description: strings.Repeat("x", MaxTextBytes+1),
	}}
	requireValidationError(t, policy, "exceeds 4096 bytes")
}
