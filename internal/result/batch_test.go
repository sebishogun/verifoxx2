package result

import "testing"

func TestBatchEnsureProvidesRequestedCapacity(t *testing.T) {
	var batch Batch
	batch.Ensure(5, 15, 20, 10)
	if cap(batch.OutcomeIDs) < 5 || cap(batch.RequirementOffsets) < 6 || cap(batch.RequirementIDs) < 15 || cap(batch.EvidenceRefs) < 20 || cap(batch.RemediationIDs) < 10 {
		t.Fatalf("Ensure() capacities are insufficient: %+v", batch)
	}
	batch.Ensure(1, 1, 1, 1)
	if len(batch.OutcomeIDs) != 1 || len(batch.RequirementOffsets) != 2 || len(batch.RequirementIDs) != 0 || len(batch.EvidenceRefs) != 0 || len(batch.RemediationIDs) != 0 {
		t.Fatalf("Ensure() active lengths = outcomes %d requirement offsets %d requirements %d evidence %d remediation %d", len(batch.OutcomeIDs), len(batch.RequirementOffsets), len(batch.RequirementIDs), len(batch.EvidenceRefs), len(batch.RemediationIDs))
	}
}
