# INCIDENT 2026-09-04 — Windows network loss during BlockAds validation

**Severity:** HIGH — host Internet connectivity broken until manual recovery  
**Branch:** `windows`  
**Committed HEAD at analysis:** `6025a09` (no commits after baseline; post-baseline work was uncommitted)  
**Evidence directory:** `artifacts/windows-incidents/20260904-network-loss/`  
**Real host DNS modified during this analysis:** NO

This document records **only observed evidence**. Missing facts are marked unknown.

---

## 1. Observed symptoms (user report + post-incident capture)

| Observation | Source |
|---|---|
| Windows lost Internet while Cursor implemented/tested BlockAds | User report |
| Reboot did **not** restore connectivity | User report |
| Manual recovery: stop BlockAds + reset active physical adapter DNS to automatic/DHCP | User report |
| Connectivity restored after manual DHCP reset | User report + `cleanup.txt` / `current_dns_readonly.txt` |
| Post-manual-recovery Ethernet DNS = `192.168.1.1` | `current_dns_readonly.txt` |
| `BlockAdsService` Stopped, StartType Automatic, LocalSystem | `scm/service.txt` |
| ProgramData `config.json` has `"enabled": false` | `programdata/config.json` |
| Recovery journal directory empty (no `recovery.json`) | `programdata/state/` listing |

---

## 2. Smoking-gun validation log (observed)

From `validation-logs/q1fail_L_O_prep.txt` / `q1fail_L_O_KEY.txt`:

| Marker | Observed value |
|---|---|
| After Gate L disable (`L_disable`) | `engine.state=DISABLED`, `filteringEnabled=false`, `dns.adapters=null`, `recoveryRequired=false` |
| `L_dns_restored` | **`127.0.0.1`** (not restored to prior upstream) |
| `O1_pre_status` | `filteringEnabled=false`, engine DISABLED, adapters null |
| `O1_pre_dns` | **`イーサネット\|127.0.0.1`** while filtering off / no ownership journal |

Interpretation constrained to evidence:

- Adapter IPv4 DNS remained **BlockAds localhost** after Disable reported DISABLED with **empty ownership**.
- Service restart for O1 prep did **not** clear that localhost DNS.
- A subsequent reboot with Automatic start + empty journal + `enabled=false` would have **no Recover ownership** and **no Enable**, leaving dead localhost → matches “reboot did not restore.”

---

## 3. Timeline (from logs / scripts; UTC timestamps in status JSON)

| Time (approx, from status `startedAt`) | Activity | Evidence |
|---|---|---|
| Pre-incident baseline | HEAD `6025a09`; Gates A–K/M/P previously PASS | git evidence; prior validation docs |
| ~12:19:37Z | DoH-failure enable path ran; filtering ACTIVE; Ethernet owned | `q1fail_L_O_prep.txt` enable block |
| ~12:19:43Z | Good DoH re-enable + **Gate L** (Ethernet disable/enable while filtering) | `L_cb_delta=36`, `L_re_delta=4`, `L_dns=127.0.0.1`, owned=True |
| ~12:19:43Z +15s | `cli disable` → status DISABLED, adapters null | `L_disable` |
| Same moment | Effective DNS still `127.0.0.1` | **`L_dns_restored=127.0.0.1`** |
| ~12:20:01Z | Service restarted for O1 prep; filtering still false | `O1_pre_status` |
| Same | Ethernet still `127.0.0.1` | **`O1_pre_dns`** |
| Later (user) | Reboot failed to restore; manual DHCP recovery | User report |
| Analysis capture | Host DNS healthy again; service Stopped; journal empty | incident evidence tree |

**Validation activity when connectivity was lost:** Gate L network-change sequence and/or the subsequent Disable that left localhost DNS with cleared journal (exact first user-visible failure moment not separately logged).

---

## 4. Hypothesis ranking (after evidence)

### H1 — PROBABLE / nearly proven: Original poisoned across adapter flap + false “successful” restore

`ReevaluateAdapters` (pre-fix) when an adapter temporarily disappeared:

1. **Dropped** ownership journal entries for missing adapters (destroying pre-BlockAds `Original`).
2. On reappear, created new ownership with **`Original = current snapshot`**.
3. If current was already `127.0.0.1`, Original became localhost.
4. `Disable` → `CompareAndRestore` “restored” Original=`127.0.0.1`, cleared journal → **`L_dns_restored=127.0.0.1`** with `adapters=null`.

Consistent with Gate L (`L_re_delta=4`) immediately before the bad Disable.

### H2 — PROBABLE contributor: no post-restore verification

Even a DHCP/`SetInterfaceDnsSettings(NULL)` path that returned success without clearing static `127.0.0.1` would clear the journal. Evidence cannot distinguish H1 vs H2 alone; H1 fits Gate L better because Original-poisoning makes Restore a no-op that still “succeeds.”

### H3 — PROBABLE for reboot persistence: empty journal + Automatic service + desired disabled

With journal gone and `enabled=false`, Startup Recover is a no-op; orphan localhost is not cleared (pre-fix). Reboot cannot heal DNS.

### H4 — Secondary: `ReevaluateAdapters` applied DNS without Enable’s health gate

Could enlarge blast radius; not required to explain the Disable outcome above.

### Ruled less likely as sole cause (insufficient evidence)

- Port 53 bind failure at the Disable moment (status still showed listener addresses; engine DISABLED after intentional stop).
- DoH-only upstream failure alone (nslookup to 127.0.0.1 still resolved during bad-DoH test via fallback).

---

## 5. Call graph — production paths to `SetInterfaceDnsSettings` / `ApplyLocalhost`

| Path | Trigger | Preconditions (pre-fix → post-fix) | Listener health required? |
|---|---|---|---|
| `Enable` → `applyLocalhostGuardedLocked` → `ApplyLocalhost` | CLI/IPC enable; Startup when desired | Port bind + health + eligible adapters; **now** `mayApplyLocalDns` | YES |
| `ReevaluateAdapters` → apply | `NotifyIpInterfaceChange` debounce | Was: Active only. **Now:** Active + `acceptMut` + `mayApplyLocalDns`; refuse Original=localhost; keep missing ownership | YES (now) |
| `CompareAndRestore` / `Restore` | Disable, Recover, watchdog, emergency-restore, Enable rollback | Compare-and-restore only if current matches Applied; **now** post-restore verify rejects remaining localhost | N/A (restore) |
| `EmergencyRestore` | Proven ownership restore; unproven only with `--force-unproven-localhost` | N/A |

There must be **no** production Apply that bypasses `mayApplyLocalDns` / `applyLocalhostGuardedLocked`.

---

## 6. Safety properties DNS-1 / DNS-2

See `docs/windows/SAFETY_DNS1.md`.

## 7. Orphan / provenance policy (hardening)

Automatic restore is allowed only for **OWNED** / **PROBABLE_OWNED**.

`UNPROVEN_LOCALHOST` is **not** auto-mutated (another local DNS product may use `127.0.0.1`).
Diagnostics: `RECOVERY_REQUIRED` + `suspectedUnprovenLocalhost`.
Explicit: `emergency-restore` (proven only) or `--force-unproven-localhost`.

## 8. Related code paths (post-fix)

| Path | Behavior |
|---|---|
| `ReevaluateAdapters` | Keep ownership across flaps; never re-snapshot Original; refuse localhost Original |
| `CompareAndRestore` | Verified readback required before clearing provenance |
| Watchdog | Health fail → restore; success=DEGRADED/NONE; failure=RECOVERY_REQUIRED+retain |

Real host DNS was **not** mutated during analysis or this hardening pass.

