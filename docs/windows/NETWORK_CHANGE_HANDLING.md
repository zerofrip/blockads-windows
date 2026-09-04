# Network Change Handling

## Goal

Keep BlockAds DNS ownership correct when adapters appear, disappear, or reconnect — without aggressive polling.

## Candidate Windows APIs

| API | Signals | Phase 3 use |
|-----|---------|-------------|
| `NotifyIpInterfaceChange` | Interface add/remove/up/down | **Primary** |
| `NotifyUnicastIpAddressChange` | Address assignment | Secondary (debounce with interface) |
| `NotifyRouteChange2` | Routing changes | **Not required** for DNS-only MVP |

## Policy

1. Debounce events (~500ms–1s coalescing)
2. Re-enumerate adapters (`GetAdaptersAddresses`)
3. Re-evaluate eligibility (`DNS_ADAPTER_POLICY`)
4. If filtering **enabled**:
   - New eligible Up adapter → snapshot → persist → ApplyLocalhost
   - Missing adapter → drop ownership entry (no restore attempt)
   - Still present but DNS ≠ Applied → mark conflict; do not overwrite
5. Persist recovery state atomically

## Non-goals

- Do not subscribe to every ETW/network trace
- Do not poll every N ms as the primary mechanism
- Do not touch VPN/Wintun/WSL/Hyper-V adapters

## Implementation status

Controller exposes `ReevaluateAdapters(ctx)` for event callbacks.

Windows service registers `NotifyIpInterfaceChange` when filtering is active (see `internal/netwatch`).
