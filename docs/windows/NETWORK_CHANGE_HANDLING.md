# Network Change Handling

## Goal

Keep BlockAds DNS ownership correct when adapters appear, disappear, or reconnect — without aggressive polling — **without violating SAFETY PROPERTY DNS-1 / DNS-2**.

## Candidate Windows APIs

| API | Signals | Phase 3 use |
|-----|---------|-------------|
| `NotifyIpInterfaceChange` | Interface add/remove/up/down | **Primary** |
| `NotifyUnicastIpAddressChange` | Address assignment | Secondary (debounce with interface) |
| `NotifyRouteChange2` | Routing changes | **Not required** for DNS-only MVP |

## Policy (post incident 2026-09-04 + hardening)

1. Debounce events (~500ms–1s coalescing)
2. Re-enumerate adapters (`GetAdaptersAddresses`)
3. Re-evaluate eligibility (`DNS_ADAPTER_POLICY`)
4. If filtering **runtime ACTIVE** and `mayApplyLocalDns` authorizes:
   - Already-owned GUID → confirm `LastConfirmedAt`; **never** rewrite `OriginalDns` (DNS-2)
   - New eligible Up adapter with `CanBecomeOriginal` → APPLYING → guarded Apply → OWNED
   - Class `SUSPECT_LOCALHOST_ORPHAN` → refuse new ownership
   - Temporarily missing adapter → **keep** ownership journal (preserve Original across flaps)
   - Still present but DNS ≠ Applied → conflict; do not overwrite
5. If engine is STOPPING / unhealthy / not ACTIVE → network callbacks must **not** apply DNS
6. Persist recovery state atomically; clear provenance only after **verified** restore readback
7. Listener health while owning DNS is enforced by the DNS-1 watchdog (not NIC polling)

## Incident lesson

Dropping ownership when an adapter flaps, then re-snapshotting `Original` from already-applied `127.0.0.1`, made Disable “restore” localhost and clear the journal — leaving Windows offline after reboot. See `INCIDENT_20260904_NETWORK_LOSS.md`.

## Non-goals

- Do not subscribe to every ETW/network trace
- Do not poll every N ms as the primary mechanism
- Do not touch VPN/Wintun/WSL/Hyper-V adapters

## Implementation status

Controller exposes `ReevaluateAdapters(ctx)` for event callbacks.

Windows service registers `NotifyIpInterfaceChange` when filtering is active (see `internal/netwatch`).
