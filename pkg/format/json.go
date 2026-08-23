package format

import (
	"encoding/json"
	"os"

	"github.com/sebishogun/verifoxx2/pkg/policy"
)

// OutputPack represents the top-level machine-readable JSON structure for results.
type OutputPack struct {
	SchemaVersion int                       `json:"schema_version"`
	PolicyName    string                    `json:"policy_name"`
	PolicyVersion string                    `json:"policy_version"`
	Results       []policy.EvaluationResult `json:"results"`
}

// LoadRequests loads a slice of Request objects from a JSON file.
func LoadRequests(filePath string) ([]policy.Request, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	var reqs []policy.Request
	if err := json.Unmarshal(data, &reqs); err != nil {
		return nil, err
	}
	return reqs, nil
}

// LoadEvidence loads evidence records into a map keyed by evidence ID.
func LoadEvidence(filePath string) (map[string]policy.Evidence, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	var evList []policy.Evidence
	if err := json.Unmarshal(data, &evList); err != nil {
		return nil, err
	}
	evMap := make(map[string]policy.Evidence)
	for _, ev := range evList {
		evMap[ev.ID] = ev
	}
	return evMap, nil
}

// WriteResults writes evaluation results to a machine-readable JSON file.
func WriteResults(filePath string, results []policy.EvaluationResult) error {
	pack := OutputPack{
		SchemaVersion: 1,
		PolicyName:    "verifoxx-policy",
		PolicyVersion: "1.0.0",
		Results:       results,
	}
	data, err := json.MarshalIndent(pack, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filePath, data, 0644)
}
