# Test Plan — Windows Port

## Unit tests

- Filter matching (trie/bloom mmap load/query/close; repeated open/close)
- Custom rule precedence (characterization from Android, then Go implementation)
- Rule parser (block/allow/wildcard/invalid/empty)
- DNS request processing helpers
- Configuration serialization
- DNS recovery state machine (compare-and-restore, conflicts, adapter gone)
- Adapter eligibility policy

## Integration tests

- Local DNS listener (`StartStandalone`) UDP/TCP IPv4; IPv6 when available
- DoH forwarding
- Filtering with real trie fixtures
- Enable/disable lifecycle
- Settings persistence
- Port 53 conflict fails safely

## Windows-specific tests

- DNS snapshot / apply / compare-and-restore
- Crash recovery ownership checks
- Adapter appear/disappear without corrupting state
- IPv4 / IPv6 dual-stack adapters
- Sleep/resume, Wi-Fi↔Ethernet, VPN adapter appear/disappear (manual + automated where possible)
- No shell in production DNS path

## HTTPS (later)

- CA generate / verify / install consent / remove / rotate
- Browser intercept, passthrough, pinned bypass
- Uninstall cleanup

## CI

| Job | Runner | Scope |
|-----|--------|-------|
| Existing Android | ubuntu-latest | Unchanged |
| Go core | ubuntu-latest + windows-latest | `go test` in `tunnel/` (+ `windows/` later) |

Mark runtime-only Windows checks `UNVERIFIED_WINDOWS_RUNTIME` when no Windows host is available.
