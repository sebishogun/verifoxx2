package ast

import (
	"fmt"
	"strings"

	"github.com/sebishogun/verifoxx2/internal/schema"
)

const (
	MaxExpressionDepth  = 32
	MaxExpressionNodes  = 4096
	MaxExplanationBytes = 1024
	MaxTextBytes        = 4096
)

type resolutionState struct {
	name        string
	outcome     Outcome
	explanation string
}

func (p *Policy) Validate() error {
	if p == nil {
		return fmt.Errorf("policy is nil")
	}
	if strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("policy name is required")
	}
	if strings.TrimSpace(p.Version) == "" {
		return fmt.Errorf("policy version is required")
	}
	if len(p.Requirements) == 0 {
		return fmt.Errorf("policy must declare at least one requirement")
	}

	requirementIDs := make(map[string]struct{}, len(p.Requirements))
	clauseIDs := make(map[string]struct{})
	nodeCount := 0
	for i := range p.Requirements {
		requirement := &p.Requirements[i]
		path := fmt.Sprintf("requirements[%d]", i)
		id := strings.TrimSpace(requirement.ID)
		if id == "" {
			return fmt.Errorf("%s has an empty id", path)
		}
		if _, exists := requirementIDs[id]; exists {
			return fmt.Errorf("%s has duplicate requirement id %q", path, id)
		}
		requirementIDs[id] = struct{}{}
		if len(requirement.Clauses) == 0 {
			return fmt.Errorf("%s must declare at least one clause", path)
		}
		if err := validateExpression(&requirement.Applicability, path+".applicability", 1, &nodeCount, false); err != nil {
			return err
		}

		for j := range requirement.Clauses {
			clause := &requirement.Clauses[j]
			clausePath := fmt.Sprintf("%s.clauses[%d]", path, j)
			clauseID := strings.TrimSpace(clause.ID)
			if clauseID == "" {
				return fmt.Errorf("%s has an empty id", clausePath)
			}
			if _, exists := clauseIDs[clauseID]; exists {
				return fmt.Errorf("%s has duplicate clause id %q", clausePath, clauseID)
			}
			clauseIDs[clauseID] = struct{}{}
			if err := validateExpression(&clause.Assertion, clausePath+".assertion", 1, &nodeCount, true); err != nil {
				return err
			}
			if err := validateClauseResolution(clause, clausePath); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateExpression(expr *Expression, path string, depth int, nodes *int, allowEvidence bool) error {
	if depth > MaxExpressionDepth {
		return fmt.Errorf("%s exceeds maximum depth %d", path, MaxExpressionDepth)
	}
	*nodes++
	if *nodes > MaxExpressionNodes {
		return fmt.Errorf("%s exceeds maximum policy expression count %d", path, MaxExpressionNodes)
	}

	switch expr.Op {
	case OperatorAll:
		if len(expr.Children) == 0 {
			return fmt.Errorf("%s: all requires at least one child", path)
		}
		if expr.Field != "" || expr.Value != "" || len(expr.Values) != 0 || expr.Evidence != (EvidencePredicate{}) {
			return fmt.Errorf("%s: all allows only children", path)
		}
		for i := range expr.Children {
			if err := validateExpression(&expr.Children[i], fmt.Sprintf("%s.children[%d]", path, i), depth+1, nodes, allowEvidence); err != nil {
				return err
			}
		}
	case OperatorEqual:
		field, ok := schema.LookupField(expr.Field)
		if !ok {
			return fmt.Errorf("%s: equal has unknown field %q", path, expr.Field)
		}
		if strings.TrimSpace(expr.Value) == "" {
			return fmt.Errorf("%s: equal requires a non-empty value", path)
		}
		if !schema.ValidFieldValue(field, expr.Value) {
			return fmt.Errorf("%s: equal has unknown value %q for field %q", path, expr.Value, expr.Field)
		}
		if len(expr.Values) != 0 {
			return fmt.Errorf("%s: equal does not allow values", path)
		}
		if len(expr.Children) != 0 || expr.Evidence != (EvidencePredicate{}) {
			return fmt.Errorf("%s: equal allows only field and value", path)
		}
	case OperatorIn:
		field, ok := schema.LookupField(expr.Field)
		if !ok {
			return fmt.Errorf("%s: in has unknown field %q", path, expr.Field)
		}
		if len(expr.Values) == 0 {
			return fmt.Errorf("%s: in requires at least one value", path)
		}
		if expr.Value != "" || len(expr.Children) != 0 || expr.Evidence != (EvidencePredicate{}) {
			return fmt.Errorf("%s: in allows only field and values", path)
		}
		seen := make(map[string]struct{}, len(expr.Values))
		for i, value := range expr.Values {
			if strings.TrimSpace(value) == "" {
				return fmt.Errorf("%s.values[%d] is an empty value", path, i)
			}
			if _, exists := seen[value]; exists {
				return fmt.Errorf("%s has duplicate value %q", path, value)
			}
			if !schema.ValidFieldValue(field, value) {
				return fmt.Errorf("%s.values[%d] has unknown value %q for field %q", path, i, value, expr.Field)
			}
			seen[value] = struct{}{}
		}
	case OperatorEvidenceMatches:
		if !allowEvidence {
			return fmt.Errorf("%s: evidence_matches is not allowed in applicability", path)
		}
		if strings.TrimSpace(expr.Evidence.Kind) == "" {
			return fmt.Errorf("%s: evidence_matches requires evidence.kind", path)
		}
		if !schema.ValidEvidenceKind(expr.Evidence.Kind) {
			return fmt.Errorf("%s: evidence_matches has unknown evidence kind %q", path, expr.Evidence.Kind)
		}
		if expr.Field != "" || expr.Value != "" || len(expr.Values) != 0 || len(expr.Children) != 0 {
			return fmt.Errorf("%s: evidence_matches allows only evidence", path)
		}
	default:
		return fmt.Errorf("%s has unsupported operator %q", path, expr.Op)
	}
	return nil
}

func validateClauseResolution(clause *Clause, path string) error {
	states := [...]resolutionState{
		{"satisfied", clause.Resolution.Satisfied, clause.Explanations.Satisfied},
		{"false", clause.Resolution.False, clause.Explanations.False},
		{"missing", clause.Resolution.Missing, clause.Explanations.Missing},
		{"invalid", clause.Resolution.Invalid, clause.Explanations.Invalid},
		{"stale", clause.Resolution.Stale, clause.Explanations.Stale},
		{"unclear", clause.Resolution.Unclear, clause.Explanations.Unclear},
		{"unverifiable", clause.Resolution.Unverifiable, clause.Explanations.Unverifiable},
		{"conflict", clause.Resolution.Conflict, clause.Explanations.Conflict},
	}
	hasRevise := false
	for _, state := range states {
		if !validOutcome(state.outcome) {
			return fmt.Errorf("%s.resolution.%s has invalid outcome %q", path, state.name, state.outcome)
		}
		if len(state.explanation) > MaxExplanationBytes {
			return fmt.Errorf("%s.explanations.%s exceeds %d bytes", path, state.name, MaxExplanationBytes)
		}
		if state.outcome != OutcomeApprove && strings.TrimSpace(state.explanation) == "" {
			return fmt.Errorf("%s.explanations.%s is required for non-Approve outcome %s", path, state.name, state.outcome)
		}
		if state.outcome == OutcomeRevise {
			hasRevise = true
		}
	}
	if hasRevise && len(clause.Remediations) == 0 {
		return fmt.Errorf("%s: Revise requires remediation", path)
	}
	if !hasRevise && len(clause.Remediations) != 0 {
		return fmt.Errorf("%s has remediation without a Revise resolution", path)
	}
	for i := range clause.Remediations {
		if err := validateRemediation(&clause.Remediations[i], fmt.Sprintf("%s.remediations[%d]", path, i)); err != nil {
			return err
		}
	}
	return nil
}

func validateRemediation(remediation *Remediation, path string) error {
	if len(remediation.Description) > MaxTextBytes {
		return fmt.Errorf("%s.description exceeds %d bytes", path, MaxTextBytes)
	}
	switch remediation.Action {
	case RemediationAddEvidence:
		if strings.TrimSpace(remediation.EvidenceKind) == "" {
			return fmt.Errorf("%s action add_evidence requires evidence_kind", path)
		}
		if !schema.ValidEvidenceKind(remediation.EvidenceKind) {
			return fmt.Errorf("%s action add_evidence has unknown evidence_kind %q", path, remediation.EvidenceKind)
		}
		if remediation.Field != "" || remediation.Value != "" {
			return fmt.Errorf("%s action add_evidence does not allow field or value", path)
		}
	case RemediationSetField:
		field, ok := schema.LookupField(remediation.Field)
		if !ok {
			return fmt.Errorf("%s action set_field has unknown field %q", path, remediation.Field)
		}
		if strings.TrimSpace(remediation.Value) == "" {
			return fmt.Errorf("%s action set_field requires a non-empty value", path)
		}
		if !schema.ValidFieldValue(field, remediation.Value) {
			return fmt.Errorf("%s action set_field has unknown value %q for field %q", path, remediation.Value, remediation.Field)
		}
		if remediation.EvidenceKind != "" {
			return fmt.Errorf("%s action set_field does not allow evidence_kind", path)
		}
	default:
		return fmt.Errorf("%s has unsupported remediation action %q", path, remediation.Action)
	}
	return nil
}

func validOutcome(outcome Outcome) bool {
	switch outcome {
	case OutcomeApprove, OutcomeReject, OutcomeRevise, OutcomeEscalate:
		return true
	default:
		return false
	}
}
