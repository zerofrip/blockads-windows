package filters

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	tunnel "github.com/nqmgaming/blockads-tunnel"
)

const (
	trieMagic    = 0x54524945
	trieVersion  = 2
	bloomMagic   = 0x424C4F4D
	bloomVersion = 1
	maxDownload  = 64 << 20
)

// CatalogEntry mirrors Android remote filter_lists.json entries.
type CatalogEntry struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	IsEnabled   bool   `json:"isEnabled"`
	Category    string `json:"category"`
	RuleCount   int    `json:"ruleCount"`
	BloomURL    string `json:"bloomUrl"`
	TrieURL     string `json:"trieUrl"`
	CSSURL      string `json:"cssUrl"`
	Scriptlets  string `json:"scriptletsUrl"`
}

type activeMeta struct {
	Version string `json:"version"`
	Trie    string `json:"trie"`
	Bloom   string `json:"bloom"`
}

// Manager owns filter download and activation under PROGRAMDATA.
// Storage uses versioned immutable directories so Windows can keep old mappings
// open while a new version is prepared:
//
//	filters/lists/<id>/v/<version>/{current.trie,current.bloom}
//	filters/lists/<id>/active.json
type Manager struct {
	Dir    string
	Client *http.Client
}

func NewManager(dir string) *Manager {
	return &Manager{
		Dir: dir,
		Client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

func (m *Manager) catalogPath() string { return filepath.Join(m.Dir, "catalog.json") }

func (m *Manager) FetchCatalog(ctx context.Context, url string) ([]CatalogEntry, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := m.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("catalog HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	var entries []CatalogEntry
	if err := json.Unmarshal(body, &entries); err != nil {
		return nil, err
	}
	_ = os.MkdirAll(m.Dir, 0o755)
	tmp := m.catalogPath() + ".tmp"
	if err := os.WriteFile(tmp, body, 0o600); err != nil {
		return nil, err
	}
	if err := os.Rename(tmp, m.catalogPath()); err != nil {
		return nil, err
	}
	return entries, nil
}

// LoadedPaths are immutable on-disk artifact paths ready for mmap.
type LoadedPaths struct {
	AdTrieCSV   string
	AdBloomCSV  string
	SecTrieCSV  string
	SecBloomCSV string
	ListIDs     []string
	// PendingActive is written only after a successful engine swap.
	PendingActive map[string]activeMeta
	// RetireDirs are previous version directories to delete AFTER the engine
	// has swapped mappings away from them.
	RetireDirs []string
}

// PrepareVersioned downloads, validates (temporary mmap), then promotes into
// a new immutable version directory BEFORE long-lived mapping.
func (m *Manager) PrepareVersioned(ctx context.Context, entries []CatalogEntry, enabledIDs []string) (LoadedPaths, error) {
	want := map[string]bool{}
	if len(enabledIDs) == 0 {
		for _, e := range entries {
			if e.IsEnabled {
				want[e.ID] = true
			}
		}
	} else {
		for _, id := range enabledIDs {
			want[id] = true
		}
	}

	paths := LoadedPaths{PendingActive: map[string]activeMeta{}}
	version := fmt.Sprintf("%d", time.Now().UnixNano())

	for _, e := range entries {
		if !want[e.ID] {
			continue
		}
		stageDir := filepath.Join(m.Dir, "staging", e.ID)
		_ = os.MkdirAll(stageDir, 0o755)
		stageTrie := filepath.Join(stageDir, "current.trie")
		stageBloom := filepath.Join(stageDir, "current.bloom")
		if err := m.downloadFile(ctx, e.TrieURL, stageTrie); err != nil {
			m.AbortPrepared(paths)
			return LoadedPaths{}, fmt.Errorf("%s trie: %w", e.ID, err)
		}
		if err := m.downloadFile(ctx, e.BloomURL, stageBloom); err != nil {
			m.AbortPrepared(paths)
			return LoadedPaths{}, fmt.Errorf("%s bloom: %w", e.ID, err)
		}
		if err := validateTrieFile(stageTrie); err != nil {
			m.AbortPrepared(paths)
			return LoadedPaths{}, fmt.Errorf("%s trie validate: %w", e.ID, err)
		}
		if err := validateBloomFile(stageBloom); err != nil {
			m.AbortPrepared(paths)
			return LoadedPaths{}, fmt.Errorf("%s bloom validate: %w", e.ID, err)
		}
		// Temporary mmap proof — must Close before rename/promote on Windows.
		if err := proveMmap(stageTrie, stageBloom); err != nil {
			m.AbortPrepared(paths)
			return LoadedPaths{}, fmt.Errorf("%s mmap proof: %w", e.ID, err)
		}

		verDir := filepath.Join(m.Dir, "lists", e.ID, "v", version)
		_ = os.MkdirAll(verDir, 0o755)
		finalTrie := filepath.Join(verDir, "current.trie")
		finalBloom := filepath.Join(verDir, "current.bloom")
		if err := moveFile(stageTrie, finalTrie); err != nil {
			m.AbortPrepared(paths)
			return LoadedPaths{}, err
		}
		if err := moveFile(stageBloom, finalBloom); err != nil {
			m.AbortPrepared(paths)
			return LoadedPaths{}, err
		}
		_ = os.RemoveAll(stageDir)

		prev := m.readActive(e.ID)
		meta := activeMeta{Version: version, Trie: finalTrie, Bloom: finalBloom}
		paths.PendingActive[e.ID] = meta
		if prev != nil && prev.Version != "" && prev.Version != version {
			paths.RetireDirs = append(paths.RetireDirs, filepath.Join(m.Dir, "lists", e.ID, "v", prev.Version))
		}

		if e.Category == "SECURITY" {
			paths.SecTrieCSV = joinCSV(paths.SecTrieCSV, finalTrie)
			paths.SecBloomCSV = joinCSV(paths.SecBloomCSV, finalBloom)
		} else {
			paths.AdTrieCSV = joinCSV(paths.AdTrieCSV, finalTrie)
			paths.AdBloomCSV = joinCSV(paths.AdBloomCSV, finalBloom)
		}
		paths.ListIDs = append(paths.ListIDs, e.ID)
	}
	return paths, nil
}

// CommitPrepared writes active.json and deletes retired version dirs after a
// successful in-memory filter swap.
func (m *Manager) CommitPrepared(paths LoadedPaths) {
	for id, meta := range paths.PendingActive {
		_ = m.writeActive(id, meta)
	}
	m.RetireVersions(paths.RetireDirs)
}

// AbortPrepared removes newly created version directories when activation fails.
func (m *Manager) AbortPrepared(paths LoadedPaths) {
	for _, meta := range paths.PendingActive {
		_ = os.RemoveAll(filepath.Dir(meta.Trie))
	}
}

// RetireVersions deletes previous version directories after they are no longer mapped.
func (m *Manager) RetireVersions(dirs []string) {
	for _, d := range dirs {
		_ = os.RemoveAll(d)
	}
}

// PathsFromCurrent returns CSV paths from active.json pointers.
func (m *Manager) PathsFromCurrent(entries []CatalogEntry, enabledIDs []string) (LoadedPaths, error) {
	want := map[string]bool{}
	for _, id := range enabledIDs {
		want[id] = true
	}
	if len(want) == 0 {
		for _, e := range entries {
			if e.IsEnabled {
				want[e.ID] = true
			}
		}
	}
	var paths LoadedPaths
	ids := enabledIDs
	if len(ids) == 0 {
		for _, e := range entries {
			if want[e.ID] {
				ids = append(ids, e.ID)
			}
		}
		// If no catalog, scan lists/
		if len(ids) == 0 {
			listRoot := filepath.Join(m.Dir, "lists")
			ents, _ := os.ReadDir(listRoot)
			for _, ent := range ents {
				if ent.IsDir() {
					ids = append(ids, ent.Name())
				}
			}
		}
	}
	byID := map[string]CatalogEntry{}
	for _, e := range entries {
		byID[e.ID] = e
	}
	for _, id := range ids {
		meta := m.readActive(id)
		if meta == nil {
			continue
		}
		sec := byID[id].Category == "SECURITY"
		if sec {
			paths.SecTrieCSV = joinCSV(paths.SecTrieCSV, meta.Trie)
			paths.SecBloomCSV = joinCSV(paths.SecBloomCSV, meta.Bloom)
		} else {
			paths.AdTrieCSV = joinCSV(paths.AdTrieCSV, meta.Trie)
			paths.AdBloomCSV = joinCSV(paths.AdBloomCSV, meta.Bloom)
		}
		paths.ListIDs = append(paths.ListIDs, id)
	}
	return paths, nil
}

func proveMmap(triePath, bloomPath string) error {
	tr, err := tunnel.LoadMmapTrie(triePath)
	if err != nil {
		return err
	}
	tr.Close()
	bl, err := tunnel.LoadBloomFilter(bloomPath)
	if err != nil {
		return err
	}
	bl.Close()
	return nil
}

func (m *Manager) activePath(id string) string {
	return filepath.Join(m.Dir, "lists", id, "active.json")
}

func (m *Manager) readActive(id string) *activeMeta {
	b, err := os.ReadFile(m.activePath(id))
	if err != nil {
		return nil
	}
	var meta activeMeta
	if json.Unmarshal(b, &meta) != nil {
		return nil
	}
	return &meta
}

func (m *Manager) writeActive(id string, meta activeMeta) error {
	dir := filepath.Join(m.Dir, "lists", id)
	_ = os.MkdirAll(dir, 0o755)
	b, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	tmp := m.activePath(id) + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, m.activePath(id))
}

func moveFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	// Cross-device fallback: copy then remove
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Remove(src)
}

func joinCSV(existing, path string) string {
	if existing == "" {
		return path
	}
	return existing + "," + path
}

func (m *Manager) downloadFile(ctx context.Context, url, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := m.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	tmp := dest + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	n, err := io.Copy(f, io.LimitReader(resp.Body, maxDownload+1))
	if err != nil {
		f.Close()
		return err
	}
	if n > maxDownload {
		f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("download exceeds size limit")
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, dest)
}

func validateTrieFile(path string) error {
	b, err := readHeader(path, 16)
	if err != nil {
		return err
	}
	if binary.BigEndian.Uint32(b[0:4]) != trieMagic {
		return fmt.Errorf("bad trie magic")
	}
	if binary.BigEndian.Uint32(b[4:8]) != trieVersion {
		return fmt.Errorf("bad trie version")
	}
	return nil
}

func validateBloomFile(path string) error {
	b, err := readHeader(path, 24)
	if err != nil {
		return err
	}
	if binary.BigEndian.Uint32(b[0:4]) != bloomMagic {
		return fmt.Errorf("bad bloom magic")
	}
	if binary.BigEndian.Uint32(b[4:8]) != bloomVersion {
		return fmt.Errorf("bad bloom version")
	}
	return nil
}

func readHeader(path string, n int) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	buf := make([]byte, n)
	if _, err := io.ReadFull(f, buf); err != nil {
		return nil, err
	}
	return buf, nil
}
