package compile

import (
	"github.com/sebishogun/verifoxx2/internal/ast"
	"github.com/sebishogun/verifoxx2/internal/program"
	"github.com/sebishogun/verifoxx2/internal/schema"
)

type Diagnostic struct {
	Path    string
	Message string
}

type compiler struct {
	program        program.Program
	symbols        interner
	evidenceKinds  map[string]schema.EvidenceKindID
	explanationIDs map[string]schema.ExplanationID
}

func Compile(source ast.Policy) (program.Program, []Diagnostic) {
	if err := source.Validate(); err != nil {
		return program.Program{}, []Diagnostic{{Path: "policy", Message: err.Error()}}
	}
	c := compiler{
		program: program.Program{
			Name:              source.Name,
			Version:           source.Version,
			OutcomePrecedence: []uint8{1, 4, 2, 3},
		},
		symbols:        newInterner(),
		evidenceKinds:  make(map[string]schema.EvidenceKindID),
		explanationIDs: make(map[string]schema.ExplanationID),
	}

	for i := range source.Requirements {
		requirement := &source.Requirements[i]
		c.program.RequirementSymbols = append(c.program.RequirementSymbols, c.symbols.intern(requirement.ID))
		root := c.compileExpression(&requirement.Applicability, nil)
		c.program.RequirementApplicabilityRoots = append(c.program.RequirementApplicabilityRoots, root)
		c.program.RequirementClauseStarts = append(c.program.RequirementClauseStarts, uint32(len(c.program.RequirementClauseIDs)))
		c.program.RequirementClauseCounts = append(c.program.RequirementClauseCounts, uint16(len(requirement.Clauses)))

		for j := range requirement.Clauses {
			clause := &requirement.Clauses[j]
			clauseID := schema.ClauseID(len(c.program.ClauseSymbols) + 1)
			c.program.RequirementClauseIDs = append(c.program.RequirementClauseIDs, clauseID)
			c.program.ClauseSymbols = append(c.program.ClauseSymbols, c.symbols.intern(clause.ID))

			clauseEvidenceKinds := make([]schema.EvidenceKindID, 0, 2)
			assertionRoot := c.compileExpression(&clause.Assertion, &clauseEvidenceKinds)
			c.program.ClauseAssertionRoots = append(c.program.ClauseAssertionRoots, assertionRoot)
			c.program.ClauseEvidenceKindStarts = append(c.program.ClauseEvidenceKindStarts, uint32(len(c.program.ClauseEvidenceKinds)))
			c.program.ClauseEvidenceKindCounts = append(c.program.ClauseEvidenceKindCounts, uint16(len(clauseEvidenceKinds)))
			c.program.ClauseEvidenceKinds = append(c.program.ClauseEvidenceKinds, clauseEvidenceKinds...)

			outcomes, explanations := clauseStates(clause)
			for state := range outcomes {
				c.program.ClauseResolutionOutcomeIDs = append(c.program.ClauseResolutionOutcomeIDs, outcomeID(outcomes[state]))
				c.program.ClauseExplanationIDs = append(c.program.ClauseExplanationIDs, c.explanationID(explanations[state]))
			}

			c.program.ClauseRemediationStarts = append(c.program.ClauseRemediationStarts, uint32(len(c.program.ClauseRemediationIDs)))
			c.program.ClauseRemediationCounts = append(c.program.ClauseRemediationCounts, uint16(len(clause.Remediations)))
			for k := range clause.Remediations {
				remediationID := c.compileRemediation(&clause.Remediations[k])
				c.program.ClauseRemediationIDs = append(c.program.ClauseRemediationIDs, remediationID)
			}
		}
	}
	c.symbols.freeze(&c.program)
	if err := c.program.Validate(); err != nil {
		return program.Program{}, []Diagnostic{{Path: "program", Message: err.Error()}}
	}
	return c.program, nil
}

func (c *compiler) compileExpression(expr *ast.Expression, evidenceKinds *[]schema.EvidenceKindID) schema.InstructionID {
	var operands []schema.InstructionID
	if expr.Op == ast.OperatorAll {
		operands = make([]schema.InstructionID, len(expr.Children))
		for i := range expr.Children {
			operands[i] = c.compileExpression(&expr.Children[i], evidenceKinds)
		}
	}

	opcode := program.OpInvalid
	field := schema.FieldInvalid
	value := schema.SymbolID(0)
	setStart := uint32(len(c.program.SetValues))
	setCount := uint16(0)
	evidenceSpecIndex := uint16(0)
	switch expr.Op {
	case ast.OperatorAll:
		opcode = program.OpAll
	case ast.OperatorEqual:
		opcode = program.OpEqual
		field, _ = schema.LookupField(expr.Field)
		value = c.symbols.intern(expr.Value)
	case ast.OperatorIn:
		opcode = program.OpIn
		field, _ = schema.LookupField(expr.Field)
		setCount = uint16(len(expr.Values))
		for _, item := range expr.Values {
			c.program.SetValues = append(c.program.SetValues, c.symbols.intern(item))
		}
	case ast.OperatorEvidenceMatches:
		opcode = program.OpEvidenceMatches
		kind := c.evidenceKind(expr.Evidence.Kind)
		c.appendUniqueEvidenceKind(evidenceKinds, kind)
		c.program.EvidenceSpecs = append(c.program.EvidenceSpecs, program.EvidenceSpec{
			Kind:             kind,
			Status:           c.symbols.intern(expr.Evidence.Status),
			Timing:           c.symbols.intern(expr.Evidence.Timing),
			Reviewer:         c.symbols.intern(expr.Evidence.Reviewer),
			TimestampState:   c.symbols.intern(expr.Evidence.TimestampState),
			Subject:          c.symbols.intern(expr.Evidence.Subject),
			AttestationState: c.symbols.intern(expr.Evidence.AttestationState),
			Scope:            c.symbols.intern(expr.Evidence.Scope),
			AdjustmentType:   c.symbols.intern(expr.Evidence.AdjustmentType),
		})
		evidenceSpecIndex = uint16(len(c.program.EvidenceSpecs))
	}

	operandStart := uint32(len(c.program.Operands))
	c.program.Operands = append(c.program.Operands, operands...)
	c.program.Opcodes = append(c.program.Opcodes, opcode)
	c.program.Fields = append(c.program.Fields, field)
	c.program.Values = append(c.program.Values, value)
	c.program.OperandStarts = append(c.program.OperandStarts, operandStart)
	c.program.OperandCounts = append(c.program.OperandCounts, uint16(len(operands)))
	c.program.SetStarts = append(c.program.SetStarts, setStart)
	c.program.SetCounts = append(c.program.SetCounts, setCount)
	c.program.EvidenceSpecIndexes = append(c.program.EvidenceSpecIndexes, evidenceSpecIndex)
	return schema.InstructionID(len(c.program.Opcodes))
}

func (c *compiler) appendUniqueEvidenceKind(dst *[]schema.EvidenceKindID, kind schema.EvidenceKindID) {
	if dst == nil {
		return
	}
	for _, existing := range *dst {
		if existing == kind {
			return
		}
	}
	*dst = append(*dst, kind)
}

func (c *compiler) evidenceKind(name string) schema.EvidenceKindID {
	if id, ok := c.evidenceKinds[name]; ok {
		return id
	}
	id := schema.EvidenceKindID(len(c.program.EvidenceKindSymbols) + 1)
	c.evidenceKinds[name] = id
	c.program.EvidenceKindSymbols = append(c.program.EvidenceKindSymbols, c.symbols.intern(name))
	return id
}

func (c *compiler) explanationID(text string) schema.ExplanationID {
	if text == "" {
		return 0
	}
	if id, ok := c.explanationIDs[text]; ok {
		return id
	}
	id := schema.ExplanationID(len(c.program.ExplanationSymbols) + 1)
	c.explanationIDs[text] = id
	c.program.ExplanationSymbols = append(c.program.ExplanationSymbols, c.symbols.intern(text))
	return id
}

func (c *compiler) compileRemediation(source *ast.Remediation) schema.RemediationID {
	spec := program.RemediationSpec{Description: c.explanationID(source.Description)}
	switch source.Action {
	case ast.RemediationAddEvidence:
		spec.Action = program.RemediationAddEvidence
		spec.EvidenceKind = c.evidenceKind(source.EvidenceKind)
	case ast.RemediationSetField:
		spec.Action = program.RemediationSetField
		spec.Field, _ = schema.LookupField(source.Field)
		spec.Value = c.symbols.intern(source.Value)
	}
	c.program.Remediations = append(c.program.Remediations, spec)
	return schema.RemediationID(len(c.program.Remediations))
}

func outcomeID(outcome ast.Outcome) schema.OutcomeID {
	switch outcome {
	case ast.OutcomeApprove:
		return schema.OutcomeApprove
	case ast.OutcomeReject:
		return schema.OutcomeReject
	case ast.OutcomeRevise:
		return schema.OutcomeRevise
	case ast.OutcomeEscalate:
		return schema.OutcomeEscalate
	default:
		return schema.OutcomeInvalid
	}
}

func clauseStates(clause *ast.Clause) ([8]ast.Outcome, [8]string) {
	return [8]ast.Outcome{
		clause.Resolution.Satisfied,
		clause.Resolution.False,
		clause.Resolution.Missing,
		clause.Resolution.Invalid,
		clause.Resolution.Stale,
		clause.Resolution.Unclear,
		clause.Resolution.Unverifiable,
		clause.Resolution.Conflict,
	}, [8]string{
		clause.Explanations.Satisfied,
		clause.Explanations.False,
		clause.Explanations.Missing,
		clause.Explanations.Invalid,
		clause.Explanations.Stale,
		clause.Explanations.Unclear,
		clause.Explanations.Unverifiable,
		clause.Explanations.Conflict,
	}
}
