package eval

import (
	"fmt"
	"strings"

	"github.com/sebishogun/verifoxx2/internal/input"
	"github.com/sebishogun/verifoxx2/internal/program"
	"github.com/sebishogun/verifoxx2/internal/schema"
)

type Limits struct {
	MaxRows     uint32
	MaxEvidence uint32
	MaxEdges    uint32
}

func DefaultLimits() Limits {
	return Limits{MaxRows: 1 << 20, MaxEvidence: 1 << 20, MaxEdges: 1 << 22}
}

type Builder struct {
	program *program.Program
	limits  Limits
}

func NewBuilder(compiled *program.Program, limits Limits) Builder {
	return Builder{program: compiled, limits: limits}
}

func (builder Builder) BuildInto(dst *Batch, requests []input.Request, evidence []input.Evidence) error {
	if dst == nil {
		return fmt.Errorf("batch destination is nil")
	}
	if builder.program == nil {
		return fmt.Errorf("compiled program is nil")
	}
	if uint64(len(requests)) > uint64(builder.limits.MaxRows) {
		return fmt.Errorf("request row count %d exceeds limit %d", len(requests), builder.limits.MaxRows)
	}
	if uint64(len(evidence)) > uint64(builder.limits.MaxEvidence) {
		return fmt.Errorf("evidence record count %d exceeds limit %d", len(evidence), builder.limits.MaxEvidence)
	}

	requestIDs := make(map[string]struct{}, len(requests))
	edgeCount := uint64(0)
	for i := range requests {
		id := requests[i].ID
		if strings.TrimSpace(id) == "" || strings.TrimSpace(id) != id {
			return fmt.Errorf("request %d has malformed or empty id %q", i, id)
		}
		if _, exists := requestIDs[id]; exists {
			return fmt.Errorf("duplicate request id %q", id)
		}
		requestIDs[id] = struct{}{}
		edgeCount += uint64(len(requests[i].EvidenceIDs))
		if edgeCount > uint64(builder.limits.MaxEdges) {
			return fmt.Errorf("evidence edge count %d exceeds limit %d", edgeCount, builder.limits.MaxEdges)
		}
		for j, ref := range requests[i].EvidenceIDs {
			if strings.TrimSpace(ref) == "" || strings.TrimSpace(ref) != ref {
				return fmt.Errorf("request %q evidence_ids[%d] is malformed", id, j)
			}
		}
	}

	evidenceByID := make(map[string]uint32, len(evidence))
	for i := range evidence {
		id := evidence[i].ID
		if strings.TrimSpace(id) == "" || strings.TrimSpace(id) != id {
			return fmt.Errorf("evidence %d has malformed or empty id %q", i, id)
		}
		if _, exists := evidenceByID[id]; exists {
			return fmt.Errorf("duplicate evidence id %q", id)
		}
		evidenceByID[id] = uint32(i + 1)
	}

	rows := len(requests)
	words := (rows + 63) / 64
	fieldCount := int(schema.FieldCount - 1)
	if uint64(fieldCount)*uint64(rows) > uint64(^uint(0)>>1) || uint64(fieldCount)*uint64(words) > uint64(^uint(0)>>1) {
		return fmt.Errorf("batch dimensions overflow addressable memory")
	}
	valuesLen := fieldCount * rows
	planesLen := fieldCount * words

	dst.Values = resizeClear(dst.Values, valuesLen)
	dst.Present = resizeClear(dst.Present, planesLen)
	dst.Valid = resizeClear(dst.Valid, planesLen)
	dst.SemanticIssueMasks = resizeClear(dst.SemanticIssueMasks, rows)
	dst.EvidenceKinds = resizeClear(dst.EvidenceKinds, len(evidence))
	dst.EvidenceStatus = resizeClear(dst.EvidenceStatus, len(evidence))
	dst.EvidenceTiming = resizeClear(dst.EvidenceTiming, len(evidence))
	dst.EvidenceReviewer = resizeClear(dst.EvidenceReviewer, len(evidence))
	dst.EvidenceTimestampState = resizeClear(dst.EvidenceTimestampState, len(evidence))
	dst.EvidenceSubject = resizeClear(dst.EvidenceSubject, len(evidence))
	dst.EvidenceAttestationState = resizeClear(dst.EvidenceAttestationState, len(evidence))
	dst.EvidenceScope = resizeClear(dst.EvidenceScope, len(evidence))
	dst.EvidenceAdjustmentType = resizeClear(dst.EvidenceAdjustmentType, len(evidence))
	dst.EvidencePresent = resizeClear(dst.EvidencePresent, len(evidence))
	dst.EvidenceFlags = resizeClear(dst.EvidenceFlags, len(evidence))
	dst.EvidenceRefOffsets = resizeClear(dst.EvidenceRefOffsets, rows+1)
	dst.EvidenceRefs = resizeClear(dst.EvidenceRefs, int(edgeCount))
	dst.Rows = uint32(rows)
	dst.Words = uint32(words)

	for row := range requests {
		request := &requests[row]
		builder.setRequestField(dst, row, schema.FieldRequester, request.Requester, schema.ValidFieldValue(schema.FieldRequester, request.Requester))
		builder.setRequestField(dst, row, schema.FieldTrustLevel, request.TrustLevel, schema.ValidFieldValue(schema.FieldTrustLevel, request.TrustLevel))
		builder.setRequestField(dst, row, schema.FieldAction, request.Action, schema.ValidFieldValue(schema.FieldAction, request.Action))
		builder.setRequestField(dst, row, schema.FieldOutputKind, request.OutputKind, schema.ValidFieldValue(schema.FieldOutputKind, request.OutputKind))
		builder.setRequestField(dst, row, schema.FieldDataset, request.Dataset, schema.ValidFieldValue(schema.FieldDataset, request.Dataset))
		builder.setRequestField(dst, row, schema.FieldEnvironment, request.Environment, schema.ValidFieldValue(schema.FieldEnvironment, request.Environment))
		builder.setRequestField(dst, row, schema.FieldUsageLimit, request.UsageLimit, schema.ValidFieldValue(schema.FieldUsageLimit, request.UsageLimit))
	}
	for i := range evidence {
		builder.setEvidence(dst, i, &evidence[i])
	}
	edge := 0
	for row := range requests {
		dst.EvidenceRefOffsets[row] = uint32(edge)
		for _, ref := range requests[row].EvidenceIDs {
			dst.EvidenceRefs[edge] = evidenceByID[ref]
			edge++
		}
	}
	dst.EvidenceRefOffsets[rows] = uint32(edge)
	return nil
}

func (builder Builder) setRequestField(dst *Batch, row int, field schema.FieldID, value string, semanticallyValid bool) {
	valueIndex := (int(field)-1)*int(dst.Rows) + row
	if symbol, ok := builder.program.LookupSymbol(value); ok {
		dst.Values[valueIndex] = symbol
	}
	wordIndex := (int(field)-1)*int(dst.Words) + row/64
	bit := uint64(1) << (row & 63)
	if value != "" {
		dst.Present[wordIndex] |= bit
	}
	if value != "" && semanticallyValid {
		dst.Valid[wordIndex] |= bit
	} else {
		dst.SemanticIssueMasks[row] |= uint16(1) << (field - 1)
	}
}

func (builder Builder) setEvidence(dst *Batch, index int, evidence *input.Evidence) {
	if symbol, ok := builder.program.LookupSymbol(evidence.Type); ok {
		for i, kindSymbol := range builder.program.EvidenceKindSymbols {
			if symbol == kindSymbol {
				dst.EvidenceKinds[index] = schema.EvidenceKindID(i + 1)
				break
			}
		}
	}
	builder.setEvidenceSymbol(dst.EvidenceStatus, dst.EvidencePresent, index, EvidencePresentStatus, evidence.Status)
	builder.setEvidenceSymbol(dst.EvidenceTiming, dst.EvidencePresent, index, EvidencePresentTiming, evidence.Timing)
	builder.setEvidenceSymbol(dst.EvidenceReviewer, dst.EvidencePresent, index, EvidencePresentReviewer, evidence.Reviewer)
	builder.setEvidenceSymbol(dst.EvidenceTimestampState, dst.EvidencePresent, index, EvidencePresentTimestampState, evidence.TimestampState)
	builder.setEvidenceSymbol(dst.EvidenceSubject, dst.EvidencePresent, index, EvidencePresentSubject, evidence.Subject)
	builder.setEvidenceSymbol(dst.EvidenceAttestationState, dst.EvidencePresent, index, EvidencePresentAttestationState, evidence.AttestationState)
	builder.setEvidenceSymbol(dst.EvidenceScope, dst.EvidencePresent, index, EvidencePresentScope, evidence.Scope)
	builder.setEvidenceSymbol(dst.EvidenceAdjustmentType, dst.EvidencePresent, index, EvidencePresentAdjustmentType, evidence.AdjustmentType)
	if evidence.Status == "conflicting" || evidence.TimestampState == "conflicting" || evidence.ReviewerState == "one_valid_one_revoked" {
		dst.EvidenceFlags[index] |= EvidenceFlagConflict
	}
	if evidence.TimestampState == "stale" {
		dst.EvidenceFlags[index] |= EvidenceFlagStale
	}
	if evidence.Status == "revoked" || evidence.Status == "expired" || evidence.ReviewerState == "revoked" {
		dst.EvidenceFlags[index] |= EvidenceFlagRevoked
	}
}

func (builder Builder) setEvidenceSymbol(column []schema.SymbolID, present []uint16, index int, bit uint16, value string) {
	if value == "" {
		return
	}
	present[index] |= bit
	if symbol, ok := builder.program.LookupSymbol(value); ok {
		column[index] = symbol
	}
}

func resizeClear[T any](slice []T, length int) []T {
	if cap(slice) < length {
		slice = make([]T, length)
	} else {
		slice = slice[:length]
		clear(slice)
	}
	return slice
}
