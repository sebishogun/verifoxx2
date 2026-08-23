package policy

import (
	"testing"
)

func TestEvaluator_DynamicPolicyAST_RequestsR1ToR5(t *testing.T) {
	policyAST, err := LoadPolicyAST("../../policies/policy.json")
	if err != nil {
		t.Fatalf("Failed to load policy AST: %v", err)
	}

	evidenceMap := map[string]Evidence{
		"E1": {
			ID:             "E1",
			Type:           "approval_record",
			Status:         "valid",
			Timing:         "before_execution",
			Reviewer:       "designated_reviewer",
			TimestampState: "current",
		},
		"E2": {
			ID:               "E2",
			Type:             "execution_environment_attestation",
			Subject:          "local_approved_env",
			Status:           "verified",
			AttestationState: "valid",
		},
		"E3": {
			ID:             "E3",
			Type:           "usage_limit_adjustment",
			Status:         "approved",
			Scope:          "trusted_internal_only",
			AdjustmentType: "above_standard_limit",
			Reviewer:       "designated_reviewer",
			TimestampState: "current",
		},
		"E4": {
			ID:             "E4",
			Type:           "approval_record",
			Status:         "conflicting",
			Timing:         "before_execution",
			ReviewerState:  "one_valid_one_revoked",
			TimestampState: "conflicting",
		},
	}

	tests := []struct {
		name             string
		request          Request
		expectedDecision Decision
	}{
		{
			name: "R1: External partner valid aggregate request",
			request: Request{
				ID:          "R1",
				Requester:   "external_partner",
				TrustLevel:  TrustExternal,
				Action:      ActionAggregateAnalysis,
				OutputKind:  "aggregate_counts",
				Dataset:     "protected_dataset",
				Environment: EnvLocalApproved,
				UsageLimit:  UsageStandard,
				EvidenceIDs: []string{"E1", "E2"},
			},
			expectedDecision: DecisionApprove,
		},
		{
			name: "R2: External partner row level export violation",
			request: Request{
				ID:          "R2",
				Requester:   "external_partner",
				TrustLevel:  TrustExternal,
				Action:      ActionRowLevelExport,
				OutputKind:  "individual_records",
				Dataset:     "protected_dataset",
				Environment: EnvLocalApproved,
				UsageLimit:  UsageStandard,
				EvidenceIDs: []string{"E1", "E2"},
			},
			expectedDecision: DecisionReject,
		},
		{
			name: "R3: Internal team above standard limit missing E3 adjustment",
			request: Request{
				ID:          "R3",
				Requester:   "internal_team",
				TrustLevel:  TrustTrustedInternal,
				Action:      ActionAggregateAnalysis,
				OutputKind:  "aggregate_counts",
				Dataset:     "protected_dataset",
				Environment: EnvLocalApproved,
				UsageLimit:  UsageAboveStandardLimit,
				EvidenceIDs: []string{"E1", "E2"},
			},
			expectedDecision: DecisionRevise,
		},
		{
			name: "R4: External partner unverified remote environment",
			request: Request{
				ID:          "R4",
				Requester:   "external_partner",
				TrustLevel:  TrustExternal,
				Action:      ActionAggregateAnalysis,
				OutputKind:  "aggregate_counts",
				Dataset:     "protected_dataset",
				Environment: EnvUnverifiedRemote,
				UsageLimit:  UsageStandard,
				EvidenceIDs: []string{"E1"},
			},
			expectedDecision: DecisionEscalate,
		},
		{
			name: "R5: Internal team with conflicting E4 evidence",
			request: Request{
				ID:          "R5",
				Requester:   "internal_team",
				TrustLevel:  TrustTrustedInternal,
				Action:      ActionAggregateAnalysis,
				OutputKind:  "aggregate_counts",
				Dataset:     "protected_dataset",
				Environment: EnvLocalApproved,
				UsageLimit:  UsageAboveStandardLimit,
				EvidenceIDs: []string{"E2", "E3", "E4"},
			},
			expectedDecision: DecisionEscalate,
		},
	}

	evaluator := NewEvaluator(policyAST)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := evaluator.Evaluate(tt.request, evidenceMap)
			if res.Decision != tt.expectedDecision {
				t.Errorf("Expected decision %s, got %s for request %s", tt.expectedDecision, res.Decision, tt.request.ID)
			}
		})
	}
}
