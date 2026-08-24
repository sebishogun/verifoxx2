package schema

import "testing"

type validID interface {
	Valid() bool
}

func TestTypedIDsReserveZero(t *testing.T) {
	tests := []struct {
		name string
		zero validID
		one  validID
	}{
		{"FieldID", FieldID(0), FieldID(1)},
		{"NodeID", NodeID(0), NodeID(1)},
		{"InstructionID", InstructionID(0), InstructionID(1)},
		{"SymbolID", SymbolID(0), SymbolID(1)},
		{"ValueID", ValueID(0), ValueID(1)},
		{"EvidenceKindID", EvidenceKindID(0), EvidenceKindID(1)},
		{"OutcomeID", OutcomeID(0), OutcomeID(1)},
		{"ReasonID", ReasonID(0), ReasonID(1)},
		{"RequirementID", RequirementID(0), RequirementID(1)},
		{"ClauseID", ClauseID(0), ClauseID(1)},
		{"RemediationID", RemediationID(0), RemediationID(1)},
		{"ExplanationID", ExplanationID(0), ExplanationID(1)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.zero.Valid() {
				t.Fatal("zero ID is valid")
			}
			if !tt.one.Valid() {
				t.Fatal("nonzero ID is invalid")
			}
		})
	}
}

func TestLookupField(t *testing.T) {
	tests := []struct {
		name string
		want FieldID
	}{
		{"requester", FieldRequester},
		{"trust_level", FieldTrustLevel},
		{"action", FieldAction},
		{"output_kind", FieldOutputKind},
		{"dataset", FieldDataset},
		{"environment", FieldEnvironment},
		{"usage_limit", FieldUsageLimit},
	}

	for _, tt := range tests {
		got, ok := LookupField(tt.name)
		if !ok || got != tt.want {
			t.Fatalf("LookupField(%q) = (%d, %v), want (%d, true)", tt.name, got, ok, tt.want)
		}
		if gotName := FieldName(got); gotName != tt.name {
			t.Fatalf("FieldName(%d) = %q, want %q", got, gotName, tt.name)
		}
	}

	if got, ok := LookupField("unknown"); ok || got.Valid() {
		t.Fatalf("LookupField(unknown) = (%d, %v), want invalid", got, ok)
	}
	if got := FieldName(FieldID(99)); got != "" {
		t.Fatalf("FieldName(99) = %q, want empty", got)
	}
}

func TestValidFieldValueAndEvidenceKindUseExerciseVocabulary(t *testing.T) {
	tests := []struct {
		field FieldID
		value string
		want  bool
	}{
		{FieldRequester, "external_partner", true},
		{FieldRequester, "", false},
		{FieldTrustLevel, "external", true},
		{FieldTrustLevel, "unknown", false},
		{FieldAction, "row_level_export", true},
		{FieldAction, "delete", false},
		{FieldOutputKind, "aggregate_counts", true},
		{FieldDataset, "protected_dataset", true},
		{FieldEnvironment, "unverified_remote_env", true},
		{FieldUsageLimit, "above_standard_limit", true},
		{FieldInvalid, "external", false},
	}
	for _, tt := range tests {
		if got := ValidFieldValue(tt.field, tt.value); got != tt.want {
			t.Fatalf("ValidFieldValue(%d, %q) = %v, want %v", tt.field, tt.value, got, tt.want)
		}
	}
	for _, kind := range []string{"approval_record", "execution_environment_attestation", "usage_limit_adjustment"} {
		if !ValidEvidenceKind(kind) {
			t.Fatalf("ValidEvidenceKind(%q) = false", kind)
		}
	}
	if ValidEvidenceKind("manager_note") {
		t.Fatal("ValidEvidenceKind(manager_note) = true")
	}
}
