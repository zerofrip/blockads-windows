package tunnel

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReplaceTriesAtomicKeepsOldOnFailure(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "hosts.txt")
	trieOK := filepath.Join(dir, "ok.trie")
	bloomOK := filepath.Join(dir, "ok.bloom")
	if err := os.WriteFile(input, []byte("||good.example^\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := CompileFilterList(input, trieOK, bloomOK); err != nil {
		t.Fatal(err)
	}

	e := NewEngine()
	defer e.CloseFilters()
	if err := e.ReplaceTriesAtomic(trieOK, "", bloomOK, ""); err != nil {
		t.Fatal(err)
	}
	if !e.IsDomainBlocked("good.example") {
		t.Fatal("expected blocked")
	}

	err := e.ReplaceTriesAtomic(filepath.Join(dir, "missing.trie"), "", bloomOK, "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !e.IsDomainBlocked("good.example") {
		t.Fatal("old filter must remain after failed replace")
	}
	if err := e.ReplaceTriesAtomic(trieOK, "", bloomOK, ""); err != nil {
		t.Fatal(err)
	}
}

func TestReplaceTriesAtomicSuccess(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "hosts.txt")
	triePath := filepath.Join(dir, "a.trie")
	bloomPath := filepath.Join(dir, "a.bloom")
	_ = os.WriteFile(input, []byte("||ads.blocked.test^\n"), 0o644)
	if _, err := CompileFilterList(input, triePath, bloomPath); err != nil {
		t.Fatal(err)
	}
	e := NewEngine()
	defer e.CloseFilters()
	if err := e.ReplaceTriesAtomic(triePath, "", bloomPath, ""); err != nil {
		t.Fatal(err)
	}
	if !e.IsDomainBlocked("ads.blocked.test") {
		t.Fatal("expected blocked")
	}
	if e.IsDomainBlocked("clean.example") {
		t.Fatal("clean should not be blocked")
	}
}

func TestCloseFiltersReleasesMappedFiles(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "hosts.txt")
	triePath := filepath.Join(dir, "c.trie")
	bloomPath := filepath.Join(dir, "c.bloom")
	_ = os.WriteFile(input, []byte("||release.test^\n"), 0o644)
	if _, err := CompileFilterList(input, triePath, bloomPath); err != nil {
		t.Fatal(err)
	}
	e := NewEngine()
	if err := e.ReplaceTriesAtomic(triePath, "", bloomPath, ""); err != nil {
		t.Fatal(err)
	}
	e.CloseFilters()
	// Windows: deleting previously mapped files must succeed after CloseFilters.
	if err := os.Remove(triePath); err != nil {
		t.Fatalf("remove trie after close: %v", err)
	}
	if err := os.Remove(bloomPath); err != nil {
		t.Fatalf("remove bloom after close: %v", err)
	}
}
