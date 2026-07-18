package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAtomicWriteFileReplacesContentAndMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "value.json")
	if err := atomicWriteFile(path, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := atomicWriteFile(path, []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new" {
		t.Fatalf("content=%q", data)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Fatalf("mode=%o", info.Mode().Perm())
	}
}

func TestHistoryAddDoesNotOverwriteCorruptFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.json")
	if err := os.WriteFile(path, []byte("not-json"), 0644); err != nil {
		t.Fatal(err)
	}
	store := NewHistoryStore(dir)
	if err := store.Add(&SubmittedVideo{YouTubeID: "video"}); err == nil {
		t.Fatal("expected corrupt history error")
	}
	data, _ := os.ReadFile(path)
	if string(data) != "not-json" {
		t.Fatalf("corrupt history was overwritten: %q", data)
	}
}
