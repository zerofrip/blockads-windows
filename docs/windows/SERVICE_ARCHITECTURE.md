# Service Architecture

## Preferred process model

```text
BlockAds.exe          UI / tray / settings (user session, no permanent elevation)
        │ Named Pipe (authenticated)
        ▼
BlockAdsService.exe   Windows Service — owns DNS, engine, recovery state
```

Phase 2 ships `blockads-cli` that can perform the same operations (may require elevation for DNS changes) before the full service split lands in Phase 3.

## Responsibilities

| Component | Owns |
|-----------|------|
| Service | `Engine` lifecycle, filter load, DNS snapshot/apply/restore, recovery persistence, IPC server |
| UI / CLI | User intent, display logs/stats, never mute safety invariants |
| `%PROGRAMDATA%\BlockAds\` | Service config, recovery state, shared filter cache |
| `%LOCALAPPDATA%\BlockAds\` | Per-user UI preferences only |

## Lifecycle operations

| Operation | Behavior |
|-----------|----------|
| Install | Create service, start type Automatic (delayed OK), ACL Named Pipe |
| Remove | Disable → restore DNS (compare-and-restore) → stop → delete service |
| Start / Stop / Restart | SCM + internal state machine |
| Crash recovery | SCM restart + recovery record ownership checks |
| Upgrade | Stop → replace binaries → recover DNS if needed → start |

## State machine (engine controller)

```text
DISABLED → STARTING → ACTIVE
                ↘ DEGRADED
ACTIVE → STOPPING → DISABLED
Any failure mid-start → rollback → DISABLED or RECOVERY_REQUIRED
Unclean exit → next start enters RECOVERY_REQUIRED then reconcile
```

## IPC authorization

- Named Pipe with security descriptor limited to Administrators + interactive user (or LOCAL SERVICE ↔ user SID)
- Reject unauthenticated remote clients (local-only pipe)
- Commands: status, enable, disable, recover, reload-filters — no secret material in replies

## Multi-user

- One machine-wide filtering service
- UI instances per session; service is singleton
- Settings that affect filtering live in PROGRAMDATA

## Logging

| Log | Location | Default |
|-----|----------|---------|
| Service / engine | `%PROGRAMDATA%\BlockAds\logs\` | Info; rotation |
| DNS query log | Same, gated by user setting | Off / privacy-first |

Never upload browsing data.
