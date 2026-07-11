package invocation

import "testing"

func TestStoreReturnsSnapshots(t *testing.T) {
	store := NewStore()
	created := store.Create("agent")
	store.Complete(created.ID, map[string]any{"ok": true})

	if created.Status != StatusPending {
		t.Fatalf("Create returned mutable store state: %q", created.Status)
	}
	completed, ok := store.Get(created.ID)
	if !ok || completed.Status != StatusCompleted || completed.Result["ok"] != true {
		t.Fatalf("completed snapshot = %#v, found = %v", completed, ok)
	}
	completed.Status = StatusFailed
	again, _ := store.Get(created.ID)
	if again.Status != StatusCompleted {
		t.Fatalf("caller mutation changed store status to %q", again.Status)
	}
}

func TestStoreFailureAndMissingInvocation(t *testing.T) {
	store := NewStore()
	created := store.Create("agent")
	store.Fail(created.ID, "provider unavailable")
	failed, ok := store.Get(created.ID)
	if !ok || failed.Status != StatusFailed || failed.Error != "provider unavailable" || failed.CompletedAt == nil {
		t.Fatalf("failed snapshot = %#v, found = %v", failed, ok)
	}
	if missing, ok := store.Get("missing"); ok || missing != nil {
		t.Fatalf("missing invocation = %#v, found = %v", missing, ok)
	}
}
