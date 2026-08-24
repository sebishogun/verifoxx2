package schema

import "strings"

const (
	FieldInvalid FieldID = iota
	FieldRequester
	FieldTrustLevel
	FieldAction
	FieldOutputKind
	FieldDataset
	FieldEnvironment
	FieldUsageLimit
	FieldCount
)

func LookupField(name string) (FieldID, bool) {
	switch name {
	case "requester":
		return FieldRequester, true
	case "trust_level":
		return FieldTrustLevel, true
	case "action":
		return FieldAction, true
	case "output_kind":
		return FieldOutputKind, true
	case "dataset":
		return FieldDataset, true
	case "environment":
		return FieldEnvironment, true
	case "usage_limit":
		return FieldUsageLimit, true
	default:
		return FieldInvalid, false
	}
}

func FieldName(id FieldID) string {
	switch id {
	case FieldRequester:
		return "requester"
	case FieldTrustLevel:
		return "trust_level"
	case FieldAction:
		return "action"
	case FieldOutputKind:
		return "output_kind"
	case FieldDataset:
		return "dataset"
	case FieldEnvironment:
		return "environment"
	case FieldUsageLimit:
		return "usage_limit"
	default:
		return ""
	}
}

func ValidFieldValue(field FieldID, value string) bool {
	switch field {
	case FieldRequester:
		return value != "" && strings.TrimSpace(value) == value
	case FieldTrustLevel:
		return value == "external" || value == "trusted_internal"
	case FieldAction:
		return value == "aggregate_analysis" || value == "row_level_export"
	case FieldOutputKind:
		return value == "aggregate_counts" || value == "individual_records"
	case FieldDataset:
		return value == "protected_dataset"
	case FieldEnvironment:
		return value == "local_approved_env" || value == "unverified_remote_env"
	case FieldUsageLimit:
		return value == "standard" || value == "above_standard_limit"
	default:
		return false
	}
}

func ValidEvidenceKind(kind string) bool {
	switch kind {
	case "approval_record", "execution_environment_attestation", "usage_limit_adjustment":
		return true
	default:
		return false
	}
}
