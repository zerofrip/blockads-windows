package filters

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	tunnel "github.com/nqmgaming/blockads-tunnel"
)

func TestPrepareVersionedAndAtomicActivate(t *testing.T) {
	dir := t.TempDir()
	triePath := filepath.Join(dir, "src.trie")
	bloomPath := filepath.Join(dir, "src.bloom")
	input := filepath.Join(dir, "hosts.txt")
	_ = os.WriteFile(input, []byte("||ads.test^\n"), 0o644)
	if _, err := tunnel.CompileFilterList(input, triePath, bloomPath); err != nil {
		t.Fatal(err)
	}

	trieBytes, _ := os.ReadFile(triePath)
	bloomBytes, _ := os.ReadFile(bloomPath)

	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	catalog := `[{"id":"test","name":"Test","isEnabled":true,"category":"AD","bloomUrl":"` + srv.URL + `/b.bloom","trieUrl":"` + srv.URL + `/t.trie","ruleCount":1}]`
	mux.HandleFunc("/cat.json", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(catalog)) })
	mux.HandleFunc("/t.trie", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(trieBytes) })
	mux.HandleFunc("/b.bloom", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(bloomBytes) })

	m := NewManager(filepath.Join(dir, "filters"))
	ctx := context.Background()
	entries, err := m.FetchCatalog(ctx, srv.URL+"/cat.json")
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := m.PrepareVersioned(ctx, entries, nil)
	if err != nil {
		t.Fatal(err)
	}
	e := tunnel.NewEngine()
	defer e.CloseFilters()
	if err := e.ReplaceTriesAtomic(prepared.AdTrieCSV, prepared.SecTrieCSV, prepared.AdBloomCSV, prepared.SecBloomCSV); err != nil {
		t.Fatal(err)
	}
	m.CommitPrepared(prepared)
	if !e.IsDomainBlocked("ads.test") {
		t.Fatal("expected blocked")
	}
	bad := filepath.Join(dir, "bad.trie")
	_ = os.WriteFile(bad, []byte("nope"), 0o644)
	if err := e.ReplaceTriesAtomic(bad, "", prepared.AdBloomCSV, ""); err == nil {
		t.Fatal("expected failure")
	}
	if !e.IsDomainBlocked("ads.test") {
		t.Fatal("old filter must remain")
	}

	// Second prepare creates a new version while old mapping stays valid until swap.
	prepared2, err := m.PrepareVersioned(ctx, entries, nil)
	if err != nil {
		t.Fatal(err)
	}
	if prepared2.AdTrieCSV == prepared.AdTrieCSV {
		t.Fatal("expected distinct versioned paths")
	}
	if err := e.ReplaceTriesAtomic(prepared2.AdTrieCSV, prepared2.SecTrieCSV, prepared2.AdBloomCSV, prepared2.SecBloomCSV); err != nil {
		t.Fatal(err)
	}
	m.CommitPrepared(prepared2)
	// After close, retiring old version dirs must succeed on Windows.
	e.CloseFilters()
	m.RetireVersions(prepared2.RetireDirs)
	for _, d := range prepared2.RetireDirs {
		if _, err := os.Stat(d); !os.IsNotExist(err) {
			t.Fatalf("retired dir still present: %s (%v)", d, err)
		}
	}
}
