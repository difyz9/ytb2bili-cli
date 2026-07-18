package storage

import "testing"

func TestPrepareDoesNotPersistUntilPlanIsReady(t *testing.T) {
	store := NewTaskStore(t.TempDir())
	task := store.Prepare("shared-id", "https://example.com")
	if _, err := store.Get(task.ID); err == nil {
		t.Fatal("prepared task was persisted")
	}
	if err := store.Persist(task, []string{"download", "custom"}); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get("shared-id")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got.Steps["custom"]; !ok {
		t.Fatal("planned step was not persisted")
	}
}
