# Filter Pipeline (Windows)

## Android source of truth

Catalog URL (from `FilterListRepository`):

```text
https://raw.githubusercontent.com/pass-with-high-score/blockads-default-filter/refs/heads/main/output/filter_lists.json
```

Each entry (remote model):

| Field | Use |
|-------|-----|
| `id` | Stable list identity |
| `name` | Display |
| `category` | `AD` / `SECURITY` |
| `isEnabled` | Default enable |
| `bloomUrl` | Precompiled bloom |
| `trieUrl` | Precompiled trie |
| `cssUrl` / `scriptletsUrl` | HTTPS filtering (later) |
| `ruleCount` | Metadata |

Android downloads precompiled `.trie` / `.bloom` via `FilterDownloadManager` into app files; Go `SetTries` mmap-loads CSV path lists.

Windows **reuses the same catalog and artifacts**. No separate Windows filter format.

## Windows storage

```text
%PROGRAMDATA%\BlockAds\filters\
  catalog.json
  lists\<id>\
    current.trie
    current.bloom
    meta.json
  staging\<id>\...
```

## Download transaction

```text
HTTPS GET (timeout, size limit)
  → staging temp file
  → validate magic/version/size
  → candidate LoadMmapTrie / LoadBloomFilter
  → Engine.ReplaceTriesAtomic (all lists)
  → atomic rename staging → current
  → close old mappings only after success
```

On failure: keep previous active set; report `FILTER_ERROR`.

## Validation

- Trie magic `0x54524945`, version `2`
- Bloom magic `0x424C4F4D`, version `1`
- Non-truncated header/body sizes
- Optional upstream hashes when catalog provides them (future)

## Region defaults

Android region-aware defaults are UI-side. Phase 3 Windows enables catalog entries with `isEnabled: true` from the remote JSON; region heuristics can follow in UI Phase 4.
