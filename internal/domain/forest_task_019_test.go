package domain

import "testing"

func TestOutboxJobCloneOwnsPayload(t *testing.T) {
	original := OutboxJob{ID: "job_ownership", Payload: []byte("immutable")}
	clone := original.Clone()
	clone.Payload[0] = 'X'
	if original.Payload[0] != 'i' {
		t.Fatalf("payload alias leaked")
	}
}
