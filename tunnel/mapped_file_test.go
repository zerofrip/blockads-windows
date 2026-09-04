package tunnel

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCompileLoadTrieBloomParity(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "hosts.txt")
	triePath := filepath.Join(dir, "test.trie")
	bloomPath := filepath.Join(dir, "test.bloom")

	content := "" +
		"# comment\n" +
		"||ads.example.com^\n" +
		"0.0.0.0 tracker.evil.com\n" +
		"clean.example.org\n" + // plain domain → blocked list entry
		"@@||should-skip.com^\n" + // exception skipped by compiler
		"\n"
	if err := os.WriteFile(input, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	n, err := CompileFilterList(input, triePath, bloomPath)
	if err != nil {
		t.Fatalf("CompileFilterList: %v", err)
	}
	if n < 2 {
		t.Fatalf("expected >=2 domains compiled, got %d", n)
	}

	trie, err := LoadMmapTrie(triePath)
	if err != nil {
		t.Fatalf("LoadMmapTrie: %v", err)
	}
	defer trie.Close()

	bloom, err := LoadBloomFilter(bloomPath)
	if err != nil {
		t.Fatalf("LoadBloomFilter: %v", err)
	}
	defer bloom.Close()

	cases := []struct {
		domain  string
		blocked bool
	}{
		{"ads.example.com", true},
		{"sub.ads.example.com", true}, // parent match
		{"tracker.evil.com", true},
		{"clean.example.org", true},
		{"example.com", false},
		{"google.com", false},
		{"should-skip.com", false},
	}

	for _, tc := range cases {
		inTrie := trie.ContainsOrParent(tc.domain)
		if inTrie != tc.blocked {
			t.Errorf("trie %q: got %v want %v", tc.domain, inTrie, tc.blocked)
		}
		if tc.blocked {
			if !bloom.MightContainDomainOrParent(tc.domain) {
				t.Errorf("bloom should maybe-contain blocked domain %q", tc.domain)
			}
		}
	}
}

func TestMappedTrieBloomRepeatedOpenClose(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "hosts.txt")
	triePath := filepath.Join(dir, "test.trie")
	bloomPath := filepath.Join(dir, "test.bloom")
	if err := os.WriteFile(input, []byte("||repeat.ads.test^\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := CompileFilterList(input, triePath, bloomPath); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 50; i++ {
		trie, err := LoadMmapTrie(triePath)
		if err != nil {
			t.Fatalf("iter %d LoadMmapTrie: %v", i, err)
		}
		if !trie.ContainsOrParent("repeat.ads.test") {
			trie.Close()
			t.Fatalf("iter %d: domain not found", i)
		}
		trie.Close()
		trie.Close() // idempotent

		bloom, err := LoadBloomFilter(bloomPath)
		if err != nil {
			t.Fatalf("iter %d LoadBloomFilter: %v", i, err)
		}
		if !bloom.MightContain("repeat.ads.test") {
			bloom.Close()
			t.Fatalf("iter %d: bloom miss", i)
		}
		bloom.Close()
		bloom.Close()
	}
}

func TestMapReadOnlyRejectsEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.bin")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := mapReadOnly(f, 0); err == nil {
		t.Fatal("expected error for size 0")
	}
}
