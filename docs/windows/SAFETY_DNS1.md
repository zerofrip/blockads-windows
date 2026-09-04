# SAFETY PROPERTIES — DNS-1 / DNS-2

## DNS-1 — Healthy listener required

```text
SAFETY PROPERTY DNS-1

BlockAds MUST NOT leave a Windows adapter configured to use
BlockAds-controlled localhost DNS unless a healthy BlockAds DNS
listener is available.

If listener health cannot be maintained or restored, BlockAds MUST
compare-and-restore DNS configuration that it still owns.

Externally modified DNS configuration MUST never be overwritten.
```

## DNS-2 — Ownership Original immutability

```text
SAFETY PROPERTY DNS-2 OWNERSHIP ORIGINAL IMMUTABILITY

Once BlockAds establishes ownership for a stable adapter identity,
OriginalDns must remain immutable for the lifetime of that ownership session.

Temporary adapter disappearance/reappearance MUST NOT cause OriginalDns
to be re-snapshotted.

Reappearance must reconcile against the existing ownership record.
```

Stable identity is **GUID** (authoritative) with optional **LUID**. Display name and transient interface index alone are never ownership keys.

## Ownership state machine

```text
UNOWNED → APPLYING → OWNED
OWNED → RESTORING → UNOWNED          (verified restore)
OWNED → RESTORING → RECOVERY_REQUIRED (restore failed; provenance retained)
OWNED → RECOVERY_REQUIRED
```

Forbidden:

```text
UNOWNED → OWNED                         (must Apply through APPLYING)
RECOVERY_REQUIRED → OWNED               (requires new healthy Enable transaction)
```

## Provenance model

| Level | Meaning | Auto restore? |
|---|---|---|
| `OWNED` | Valid journal/session proves BlockAds applied localhost | YES (compare-and-restore) |
| `PROBABLE_OWNED` | Durable marker/session present but journal incomplete | YES (documented; same path) |
| `UNPROVEN_LOCALHOST` | Localhost DNS with no trustworthy BlockAds evidence | **NO** |

`UNPROVEN_LOCALHOST` → diagnostic `RECOVERY_REQUIRED` + status `suspectedUnprovenLocalhost`.  
Mutation only via explicit `blockads-service emergency-restore --force-unproven-localhost`.

At snapshot boundaries, current DNS class `SUSPECT_LOCALHOST_ORPHAN` / BlockAds-applied **must not** become `OriginalDns`.

## Restore is a verified transaction

```text
Restore API call
→ read back actual DNS
→ MatchesRestoredSemantic(original, after)
→ only then clear ownership provenance
```

API success alone is not restore success. DHCP/automatic compares by automatic semantics.

## Watchdog

- Interval **5s**, **3** consecutive health failures
- Restore **success** → `runtime=DEGRADED`, `dnsOwnership=NONE`, auto-reapply forbidden
- Restore **failure** → `runtime=RECOVERY_REQUIRED`, provenance retained, `lastRestoreError` set

## Emergency recovery

Default: proven BlockAds-owned adapters only.  
Forced unproven: `--force-unproven-localhost` (never default).

See `EMERGENCY_NETWORK_RECOVERY.md`.
