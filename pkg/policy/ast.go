package policy

import (
	"encoding/json"
	"os"
)

// PolicyAST defines the top-level intermediate semantic representation of a policy document.
type PolicyAST struct {
	Name         string           `json:"name"`
	Version      string           `json:"version"`
	Requirements []RequirementAST `json:"requirements"`
}

// RequirementAST defines a requirement statement in the policy AST.
type RequirementAST struct {
	ID            string      `json:"id"`
	Description   string      `json:"description"`
	NonNegotiable bool        `json:"non_negotiable"`
	Clauses       []ClauseAST `json:"clauses"`
}

// ClauseAST defines a semantic clause condition inside a requirement.
type ClauseAST struct {
	ID                       string       `json:"id"`
	Kind                     string       `json:"kind"`
	DisallowedActions        []string     `json:"disallowed_actions,omitempty"`
	RejectionRationale       string       `json:"rejection_rationale,omitempty"`
	RequiredEnv              string       `json:"required_env,omitempty"`
	EvidenceType             string       `json:"evidence_type,omitempty"`
	RequiredStatus           string       `json:"required_status,omitempty"`
	RequiredAttestationState string       `json:"required_attestation_state,omitempty"`
	RequiredTiming           string       `json:"required_timing,omitempty"`
	RequiredTimestampState   string       `json:"required_timestamp_state,omitempty"`
	RequiredScope            string       `json:"required_scope,omitempty"`
	AboveLimitValue          string       `json:"above_limit_value,omitempty"`
	AllowedTrustLevel        string       `json:"allowed_trust_level,omitempty"`
	EscalationRationale      string       `json:"escalation_rationale,omitempty"`
	RevisionRationale        string       `json:"revision_rationale,omitempty"`
	Remediation              *Remediation `json:"remediation,omitempty"`
}

// LoadPolicyAST reads and parses a policy JSON file into a PolicyAST intermediate representation.
func LoadPolicyAST(filePath string) (*PolicyAST, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	var policy PolicyAST
	if err := json.Unmarshal(data, &policy); err != nil {
		return nil, err
	}
	return &policy, nil
}
