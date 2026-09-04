# Implementation Plan

## Branch policy

- All Windows work on `windows` only
- Never force-push; never modify `main` for port work
- Small commits with prefixes: `docs(windows):`, `refactor(core):`, `feat(windows):`, …

## Phase 0 — Audit (this commit)

Deliverables under `docs/windows/`:

- ARCHITECTURE_AUDIT.md
- NETWORK_BACKEND_ANALYSIS.md
- SERVICE_ARCHITECTURE.md
- UI_TECHNOLOGY_DECISION.md
- FEATURE_PARITY.md
- TEST_PLAN.md
- IMPLEMENTATION_PLAN.md
- PORTING_STATUS.md
- DNS_ADAPTER_POLICY.md

## Phase 1 — Core portability

Priority order (mandatory):

1. Portable mmap abstraction (`mapReadOnly` / `mappedFile`) — `mmap_unix.go` + `mmap_windows.go`
2. Trie/Bloom deterministic compile→mmap→query→close tests (+ repeated open/close)
3. Windows Go CI job
4. Custom-rule characterization tests (Android semantics)
5. Pure-Go custom rule checker only after (4)

Constraints:

- No `ProcessResolver` refactor
- No heavy `StartStandalone` rewrite
- Preserve gomobile API
- Android behavior unchanged

## Phase 2 — DNS MVP

Order:

1. Listener wrapper around `StartStandalone(53)`
2. Port-53 conflict detection (do not kill other services)
3. Adapter enumeration + eligibility policy
4. Native DNS snapshot (`GetInterfaceDnsSettings` family)
5. Persisted recovery model (GUID/LUID, applied vs original, session, checksum)
6. Apply localhost DNS
7. Compare-and-restore (never blind restore)
8. Crash recovery
9. CLI: `status|enable|disable|recover|test-dns`
10. Integration validation doc `DNS_MVP_VALIDATION.md`

Transactional enable: bind+healthcheck **before** mutating system DNS; rollback on failure.

## Phase 3+ (after MVP gate)

Service + Named Pipe → WPF UI → ProcessResolver → tunnel backend evaluation → HTTPS → WireGuard → installer.

## Safety invariants

Never leave broken DNS, orphaned routes/adapters/CA/services after clean uninstall.  
Compare-and-restore only when current DNS equals BlockAds-applied state for that adapter identity.

**DNS-1 (mandatory):** see `docs/windows/SAFETY_DNS1.md` — never leave adapters on BlockAds localhost without a healthy local listener; never overwrite externally changed DNS; never capture ownership Original from localhost.

