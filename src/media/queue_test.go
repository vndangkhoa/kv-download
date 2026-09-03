package media

import (
	"testing"
)

func TestQueueMaxWorkers(t *testing.T) {
	qm := NewQueueManager(3)
	if qm.GetMaxWorkers() != 3 {
		t.Errorf("expected maxWorkers=3, got %d", qm.GetMaxWorkers())
	}

	qm.SetMaxWorkers(5)
	if qm.GetMaxWorkers() != 5 {
		t.Errorf("expected maxWorkers=5, got %d", qm.GetMaxWorkers())
	}

	// Test boundary constraints
	qm.SetMaxWorkers(0)
	if qm.GetMaxWorkers() != 1 {
		t.Errorf("expected maxWorkers=1 for non-positive input, got %d", qm.GetMaxWorkers())
	}

	qm.SetMaxWorkers(100)
	if qm.GetMaxWorkers() != 16 {
		t.Errorf("expected maxWorkers=16 for excessive input, got %d", qm.GetMaxWorkers())
	}
}
