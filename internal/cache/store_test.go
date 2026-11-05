package cache

import (
	"path/filepath"
	"testing"
)

func TestFileStore_SetThenGet(t *testing.T) {
	store := NewFileStore(filepath.Join(t.TempDir(), "cache"))
	key := Key("model", "system", "prompt")

	if _, ok := store.Get(key); ok {
		t.Fatal("expected miss before Set")
	}

	if err := store.Set(key, "the summary"); err != nil {
		t.Fatalf("Set returned error: %v", err)
	}

	got, ok := store.Get(key)
	if !ok {
		t.Fatal("expected hit after Set")
	}
	if got != "the summary" {
		t.Fatalf("Get = %q, want %q", got, "the summary")
	}
}

func TestKey_DifferentInputsDifferentKeys(t *testing.T) {
	a := Key("model", "ab", "c")
	b := Key("model", "a", "bc")
	if a == b {
		t.Fatal("expected different keys for different part boundaries")
	}
}
