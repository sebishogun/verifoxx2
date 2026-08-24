package jsonio

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/sebishogun/verifoxx2/internal/eval"
	"github.com/sebishogun/verifoxx2/internal/input"
	"github.com/sebishogun/verifoxx2/internal/program"
	"github.com/sebishogun/verifoxx2/internal/result"
	"github.com/sebishogun/verifoxx2/internal/schema"
)

const resultAssumption = "The supplied structured fields faithfully represent the underlying request and evidence records."

type OutputPack struct {
	SchemaVersion int                `json:"schema_version"`
	PolicyName    string             `json:"policy_name"`
	PolicyVersion string             `json:"policy_version"`
	Results       []EvaluationResult `json:"results"`
}

type EvaluationResult struct {
	RequestID                    string        `json:"request_id"`
	Decision                     string        `json:"decision"`
	Rationale                    string        `json:"rationale"`
	RequirementsApplied          []string      `json:"requirements_applied"`
	EvidenceUsed                 []string      `json:"evidence_used"`
	MissingOrConflictingEvidence []string      `json:"missing_or_conflicting_evidence"`
	Assumptions                  []string      `json:"assumptions"`
	UnresolvedUncertainty        []string      `json:"unresolved_uncertainty"`
	Remediation                  []Remediation `json:"remediation,omitempty"`
}

type Remediation struct {
	Action       string `json:"action"`
	EvidenceKind string `json:"evidence_kind,omitempty"`
	Description  string `json:"description,omitempty"`
	Field        string `json:"field,omitempty"`
	Value        string `json:"value,omitempty"`
}

type outputOps struct {
	encode func(io.Writer, OutputPack) error
	close  func(*os.File) error
	chmod  func(string, os.FileMode) error
	rename func(string, string) error
	remove func(string) error
}

func newOutputOps() outputOps {
	return outputOps{
		encode: EncodeResults,
		close:  (*os.File).Close,
		chmod:  os.Chmod,
		rename: os.Rename,
		remove: os.Remove,
	}
}

func MaterializeInto(dst *OutputPack, compiled program.Program, batch eval.Batch, numeric result.Batch, requests []input.Request, evidence []input.Evidence) error {
	if dst == nil {
		return fmt.Errorf("output destination is nil")
	}
	if err := compiled.Validate(); err != nil {
		return fmt.Errorf("materialize program: %w", err)
	}
	rows := int(batch.Rows)
	if len(requests) != rows {
		return fmt.Errorf("request count %d does not match batch row count %d", len(requests), rows)
	}
	if len(evidence) != len(batch.EvidenceKinds) {
		return fmt.Errorf("evidence count %d does not match batch evidence count %d", len(evidence), len(batch.EvidenceKinds))
	}
	for _, column := range []struct {
		name string
		len  int
	}{
		{"OutcomeIDs", len(numeric.OutcomeIDs)},
		{"DriverKinds", len(numeric.DriverKinds)},
		{"DriverRequirementIDs", len(numeric.DriverRequirementIDs)},
		{"DriverClauseIDs", len(numeric.DriverClauseIDs)},
		{"DriverReasonIDs", len(numeric.DriverReasonIDs)},
		{"DriverFieldIDs", len(numeric.DriverFieldIDs)},
		{"DriverEvidenceEdgeIDs", len(numeric.DriverEvidenceEdgeIDs)},
		{"IssueIDs", len(numeric.IssueIDs)},
	} {
		if column.len != rows {
			return fmt.Errorf("numeric %s length %d does not match row count %d", column.name, column.len, rows)
		}
	}
	if err := validateOutputCSR("RequirementOffsets", numeric.RequirementOffsets, len(numeric.RequirementIDs), rows); err != nil {
		return err
	}
	if err := validateOutputCSR("EvidenceOffsets", numeric.EvidenceOffsets, len(numeric.EvidenceRefs), rows); err != nil {
		return err
	}
	if err := validateOutputCSR("RemediationOffsets", numeric.RemediationOffsets, len(numeric.RemediationIDs), rows); err != nil {
		return err
	}

	pack := OutputPack{
		SchemaVersion: 1,
		PolicyName:    compiled.Name,
		PolicyVersion: compiled.Version,
		Results:       make([]EvaluationResult, rows),
	}
	for row := 0; row < rows; row++ {
		materialized, err := materializeRow(compiled, batch, numeric, requests, evidence, row)
		if err != nil {
			return fmt.Errorf("materialize row %d request %q: %w", row, requests[row].ID, err)
		}
		pack.Results[row] = materialized
	}
	*dst = pack
	return nil
}

func materializeRow(compiled program.Program, batch eval.Batch, numeric result.Batch, requests []input.Request, evidence []input.Evidence, row int) (EvaluationResult, error) {
	outcome := numeric.OutcomeIDs[row]
	decision, ok := outcomeName(outcome)
	if !ok {
		return EvaluationResult{}, fmt.Errorf("invalid outcome ID %d", outcome)
	}
	resultValue := EvaluationResult{
		RequestID:                    requests[row].ID,
		Decision:                     decision,
		RequirementsApplied:          make([]string, 0, int(numeric.RequirementOffsets[row+1]-numeric.RequirementOffsets[row])),
		EvidenceUsed:                 make([]string, 0, int(numeric.EvidenceOffsets[row+1]-numeric.EvidenceOffsets[row])),
		MissingOrConflictingEvidence: []string{},
		Assumptions:                  []string{resultAssumption},
		UnresolvedUncertainty:        []string{},
	}
	for i := numeric.RequirementOffsets[row]; i < numeric.RequirementOffsets[row+1]; i++ {
		id := numeric.RequirementIDs[i]
		if !id.Valid() || int(id) > len(compiled.RequirementSymbols) {
			return EvaluationResult{}, fmt.Errorf("invalid requirement ID %d", id)
		}
		resultValue.RequirementsApplied = append(resultValue.RequirementsApplied, compiled.Symbol(compiled.RequirementSymbols[id-1]))
	}
	for i := numeric.EvidenceOffsets[row]; i < numeric.EvidenceOffsets[row+1]; i++ {
		ref := numeric.EvidenceRefs[i]
		if ref == 0 || int(ref) > len(evidence) {
			return EvaluationResult{}, fmt.Errorf("invalid evidence ref %d", ref)
		}
		resultValue.EvidenceUsed = append(resultValue.EvidenceUsed, evidence[ref-1].ID)
	}

	rationale, detail, uncertainty, err := materializeDriver(compiled, batch, numeric, requests, row)
	if err != nil {
		return EvaluationResult{}, err
	}
	resultValue.Rationale = rationale
	if detail != "" {
		resultValue.MissingOrConflictingEvidence = append(resultValue.MissingOrConflictingEvidence, detail)
	}
	if uncertainty != "" {
		resultValue.UnresolvedUncertainty = append(resultValue.UnresolvedUncertainty, uncertainty)
	}

	remediationCount := int(numeric.RemediationOffsets[row+1] - numeric.RemediationOffsets[row])
	if remediationCount != 0 {
		resultValue.Remediation = make([]Remediation, 0, remediationCount)
	}
	for i := numeric.RemediationOffsets[row]; i < numeric.RemediationOffsets[row+1]; i++ {
		id := numeric.RemediationIDs[i]
		if !id.Valid() || int(id) > len(compiled.Remediations) {
			return EvaluationResult{}, fmt.Errorf("invalid remediation ID %d", id)
		}
		spec := compiled.Remediations[id-1]
		item := Remediation{}
		switch spec.Action {
		case program.RemediationAddEvidence:
			item.Action = "add_evidence"
			item.EvidenceKind = evidenceKindName(compiled, spec.EvidenceKind)
			item.Description = explanationText(compiled, spec.Description)
		case program.RemediationSetField:
			item.Action = "set_field"
			item.Field = schema.FieldName(spec.Field)
			item.Value = compiled.Symbol(spec.Value)
		default:
			return EvaluationResult{}, fmt.Errorf("invalid remediation action %d", spec.Action)
		}
		resultValue.Remediation = append(resultValue.Remediation, item)
	}
	return resultValue, nil
}

func materializeDriver(compiled program.Program, batch eval.Batch, numeric result.Batch, requests []input.Request, row int) (rationale, detail, uncertainty string, err error) {
	switch numeric.DriverKinds[row] {
	case result.DriverNone:
		if numeric.OutcomeIDs[row] != schema.OutcomeApprove {
			return "", "", "", fmt.Errorf("non-Approve outcome has no driver")
		}
		return "The request satisfies all applicable requirements and supporting evidence.", "", "", nil
	case result.DriverSemantic:
		problem, problemErr := semanticProblem(requests[row], numeric.DriverFieldIDs[row])
		if problemErr != nil {
			return "", "", "", problemErr
		}
		return "The request contains unknown or missing semantic values: " + problem + ".", "", "The request's semantic values cannot be validated against the policy.", nil
	case result.DriverMissingReference:
		edgeID := numeric.DriverEvidenceEdgeIDs[row]
		if edgeID == 0 {
			return "", "", "", fmt.Errorf("missing-reference driver has no edge")
		}
		edge := edgeID - 1
		start, end := batch.EvidenceRefOffsets[row], batch.EvidenceRefOffsets[row+1]
		if edge < start || edge >= end {
			return "", "", "", fmt.Errorf("missing-reference edge %d is outside row range [%d,%d)", edge, start, end)
		}
		relative := int(edge - start)
		if relative >= len(requests[row].EvidenceIDs) {
			return "", "", "", fmt.Errorf("missing-reference position %d exceeds request evidence IDs", relative)
		}
		message := "Referenced evidence ID " + requests[row].EvidenceIDs[relative] + " is missing from the evidence pack."
		return message, message, "The referenced evidence record cannot be located, so its state remains unresolved.", nil
	case result.DriverNoApplicableRequirement:
		return "No applicable policy requirement exists for this request.", "", "Which policy requirement governs this request cannot be determined.", nil
	case result.DriverClause:
		clauseID := numeric.DriverClauseIDs[row]
		reason := numeric.DriverReasonIDs[row]
		if !clauseID.Valid() || int(clauseID) > len(compiled.ClauseSymbols) || !reason.Valid() || reason > schema.ReasonCount {
			return "", "", "", fmt.Errorf("invalid clause driver (%d, %d)", clauseID, reason)
		}
		clauseIndex := int(clauseID) - 1
		resolutionIndex := clauseIndex*int(schema.ReasonCount) + int(reason) - 1
		rationale = explanationText(compiled, compiled.ClauseExplanationIDs[resolutionIndex])
		if rationale == "" {
			return "", "", "", fmt.Errorf("clause %d reason %d has no explanation", clauseID, reason)
		}
		detail, uncertainty = clauseEvidenceDetail(compiled, clauseIndex, reason)
		return rationale, detail, uncertainty, nil
	default:
		return "", "", "", fmt.Errorf("invalid driver kind %d", numeric.DriverKinds[row])
	}
}

func semanticProblem(request input.Request, field schema.FieldID) (string, error) {
	switch field {
	case schema.FieldRequester:
		return "requester is missing", nil
	case schema.FieldTrustLevel:
		return "trust_level " + strconv.Quote(request.TrustLevel) + " is unknown", nil
	case schema.FieldAction:
		return "action " + strconv.Quote(request.Action) + " is unknown", nil
	case schema.FieldOutputKind:
		return "output_kind " + strconv.Quote(request.OutputKind) + " is unknown", nil
	case schema.FieldDataset:
		return "dataset " + strconv.Quote(request.Dataset) + " is unknown", nil
	case schema.FieldEnvironment:
		return "environment " + strconv.Quote(request.Environment) + " is unknown", nil
	case schema.FieldUsageLimit:
		return "usage_limit " + strconv.Quote(request.UsageLimit) + " is unknown", nil
	default:
		return "", fmt.Errorf("invalid semantic field ID %d", field)
	}
}

func clauseEvidenceDetail(compiled program.Program, clauseIndex int, reason schema.ReasonID) (detail, uncertainty string) {
	start := int(compiled.ClauseEvidenceKindStarts[clauseIndex])
	count := int(compiled.ClauseEvidenceKindCounts[clauseIndex])
	if count == 0 {
		return "", ""
	}
	evidenceKind := evidenceKindName(compiled, compiled.ClauseEvidenceKinds[start])
	if reason == schema.ReasonFalse && clauseConstrainsEnvironment(compiled, clauseIndex) {
		return evidenceKind + " is not verified for the request environment.", "The request's execution environment cannot be established as the required environment."
	}
	switch reason {
	case schema.ReasonMissing:
		return "Required " + evidenceKind + " evidence is missing.", "Whether the required " + evidenceKind + " evidence exists remains unresolved."
	case schema.ReasonConflict:
		return evidenceKind + " evidence has conflicting state.", "The " + evidenceKind + " evidence is conflicting and cannot be resolved."
	case schema.ReasonStale:
		return evidenceKind + " evidence is stale.", "The " + evidenceKind + " evidence is stale and cannot be relied upon."
	case schema.ReasonInvalidEvidence:
		return evidenceKind + " evidence is invalid or does not match required qualifiers.", "The " + evidenceKind + " evidence does not satisfy the required qualifiers."
	case schema.ReasonUnclear:
		return evidenceKind + " evidence is unclear or incomplete.", "The " + evidenceKind + " evidence is unclear or incomplete."
	case schema.ReasonUnverifiable:
		return evidenceKind + " evidence cannot be verified.", "The " + evidenceKind + " evidence cannot be verified."
	default:
		return "", ""
	}
}

func clauseConstrainsEnvironment(compiled program.Program, clauseIndex int) bool {
	root := compiled.ClauseAssertionRoots[clauseIndex]
	return instructionUsesField(compiled, root, schema.FieldEnvironment)
}

func instructionUsesField(compiled program.Program, instruction schema.InstructionID, field schema.FieldID) bool {
	index := int(instruction) - 1
	if compiled.Opcodes[index] == program.OpEqual || compiled.Opcodes[index] == program.OpIn {
		return compiled.Fields[index] == field
	}
	start := int(compiled.OperandStarts[index])
	end := start + int(compiled.OperandCounts[index])
	for _, operand := range compiled.Operands[start:end] {
		if instructionUsesField(compiled, operand, field) {
			return true
		}
	}
	return false
}

func explanationText(compiled program.Program, id schema.ExplanationID) string {
	if !id.Valid() || int(id) > len(compiled.ExplanationSymbols) {
		return ""
	}
	return compiled.Symbol(compiled.ExplanationSymbols[id-1])
}

func evidenceKindName(compiled program.Program, id schema.EvidenceKindID) string {
	if !id.Valid() || int(id) > len(compiled.EvidenceKindSymbols) {
		return ""
	}
	return compiled.Symbol(compiled.EvidenceKindSymbols[id-1])
}

func outcomeName(id schema.OutcomeID) (string, bool) {
	switch id {
	case schema.OutcomeApprove:
		return "Approve", true
	case schema.OutcomeReject:
		return "Reject", true
	case schema.OutcomeRevise:
		return "Revise", true
	case schema.OutcomeEscalate:
		return "Escalate", true
	default:
		return "", false
	}
}

func validateOutputCSR(name string, offsets []uint32, values, rows int) error {
	if len(offsets) != rows+1 {
		return fmt.Errorf("%s length %d does not match rows %d", name, len(offsets), rows)
	}
	previous := uint32(0)
	for i, offset := range offsets {
		if offset < previous || uint64(offset) > uint64(values) {
			return fmt.Errorf("%s[%d] = %d is invalid for %d values", name, i, offset, values)
		}
		previous = offset
	}
	if int(previous) != values {
		return fmt.Errorf("%s final offset %d does not match %d values", name, previous, values)
	}
	return nil
}

func EncodeResults(writer io.Writer, pack OutputPack) error {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(pack); err != nil {
		return fmt.Errorf("encode results: %w", err)
	}
	return nil
}

func WriteResults(path string, pack OutputPack) error {
	return writeResults(path, pack, newOutputOps())
}

func writeResults(path string, pack OutputPack, ops outputOps) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return fmt.Errorf("create result directory %s: %w", directory, err)
	}
	temporary, err := os.CreateTemp(directory, ".verifoxx-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary result in %s: %w", directory, err)
	}
	temporaryPath := temporary.Name()
	cleanup := func() {
		_ = ops.close(temporary)
		_ = ops.remove(temporaryPath)
	}
	if err := ops.encode(temporary, pack); err != nil {
		cleanup()
		return fmt.Errorf("write temporary result %s: %w", temporaryPath, err)
	}
	if err := ops.close(temporary); err != nil {
		_ = ops.remove(temporaryPath)
		return fmt.Errorf("close temporary result %s: %w", temporaryPath, err)
	}
	if err := ops.chmod(temporaryPath, 0o644); err != nil {
		_ = ops.remove(temporaryPath)
		return fmt.Errorf("set result permissions %s: %w", temporaryPath, err)
	}
	if err := ops.rename(temporaryPath, path); err != nil {
		_ = ops.remove(temporaryPath)
		return fmt.Errorf("replace result %s: %w", path, err)
	}
	return nil
}
