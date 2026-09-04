# Phase 3 Runtime Validation

**Evidence root:** `artifacts/windows-validation/20260904T110644Z/`  
**Branch:** `windows`  
**Code HEAD at gate start:** `77b3f86`  
**Environment:** Windows 11 Pro build **26200**, x64, Go **1.27.0 windows/amd64**  
**Note:** Host is a dedicated Windows workstation (not a fresh disposable VM). Pre-DNS snapshots were taken; destructive tests used compare-and-restore and left Ethernet at `192.168.1.1`.

Machine-specific hostnames/usernames in raw logs should be treated as private; summaries below are redacted.

---

## GATE A — Binary/runtime sanity

| Field | Value |
|-------|--------|
| TEST ID | A-NATIVE-BUILD |
| ENVIRONMENT | Win11 26200 x64 native Go |
| PRECONDITIONS | Repo synced to Windows tree |
| COMMAND/ACTION | `go test ./...` in `tunnel` and `windows`; build `blockads-service.exe`, `blockads-cli.exe` |
| EXPECTED | All tests pass; binaries produced |
| ACTUAL | tunnel OK; windows OK; SHA256 service `F24E…1AB0` then rebuilt after fixes |
| RESULT | **PASS** |
| EVIDENCE | `gate_a_*.txt`, `GATE_A_SUMMARY.txt` |
| CLEANUP | n/a |

Cross-compilation was **not** counted.

---

## GATE B — Windows mmap runtime

| Field | Value |
|-------|--------|
| TEST ID | B-MMAP-ATOMIC |
| ENVIRONMENT | Same |
| PRECONDITIONS | Initial failures: mapped files blocked TempDir delete; staging rename-while-mapped unsafe |
| COMMAND/ACTION | `go test` including `TestCloseFiltersReleasesMappedFiles`, `TestPrepareVersionedAndAtomicActivate`, repeated open/close |
| EXPECTED | Map/query/close; versioned promote before long-lived map; failed replace keeps old filters |
| ACTUAL | After fix: versioned `lists/<id>/v/<ver>/` + `CommitPrepared` after `ReplaceTriesAtomic`; `CloseFilters` releases mappings so delete succeeds |
| RESULT | **PASS** |
| EVIDENCE | Native `gate_a_tunnel_test.txt`, `gate_a_windows_test.txt`; filter strategy documented in `FILTER_PIPELINE.md` update via manager comments |
| CLEANUP | TempDir cleaned by tests |

**Storage strategy (Windows):** versioned immutable files → temporary mmap proof → promote → in-memory swap → close old → delete retired versions. Do **not** rename/overwrite an actively mapped file.

---

## GATE C — SCM lifecycle

| Field | Value |
|-------|--------|
| TEST ID | C-SCM-LIFECYCLE |
| ENVIRONMENT | Elevated PowerShell (UAC) |
| PRECONDITIONS | Admin elevation |
| COMMAND/ACTION | `blockads-service install/start/status/stop/uninstall` |
| EXPECTED | Service entry correct; state transitions; no hang on stop |
| ACTUAL | Name=`BlockAdsService`, Display=`BlockAds DNS Filter`, StartMode=Auto, StartName=LocalSystem, Path=validation exe; Running→Stopped; uninstall OK |
| RESULT | **PASS** |
| EVIDENCE | `gate_cdef_elevated.txt`, `gate_c_elevated` via UAC scripts |
| CLEANUP | Uninstalled after each suite |

---

## GATE D — Service identity

| Field | Value |
|-------|--------|
| TEST ID | D-LOCALSYSTEM |
| ENVIRONMENT | Elevated |
| PRECONDITIONS | Service running |
| COMMAND/ACTION | `Win32_Process.GetOwner` on `blockads-service.exe` |
| EXPECTED | `NT AUTHORITY\SYSTEM` |
| ACTUAL | `ProcessOwner=NT AUTHORITY\SYSTEM` |
| RESULT | **PASS** |
| EVIDENCE | `gate_cdef_elevated.txt` |
| CLEANUP | Service stopped/uninstalled |

---

## GATE E — Named Pipe security

| Field | Value |
|-------|--------|
| TEST ID | E-PIPE-ACL |
| ENVIRONMENT | Elevated + interactive |
| PRECONDITIONS | Service running |
| COMMAND/ACTION | Live SDDL via `GetNamedSecurityInfo(SE_FILE_OBJECT)`; connect; malformed/oversized frames; CLI status |
| EXPECTED | SY/BA full; IU/AU RW; no Everyone; service survives abuse |
| ACTUAL | Live SDDL: `O:BAG:SYD:P(A;;FA;;;SY)(A;;FA;;;BA)(A;;0x12019f;;;IU)(A;;0x12019f;;;AU)`; pipe `\\.\pipe\BlockAdsService`; service remained Running after malformed/oversized |
| RESULT | **PASS** (remote/anonymous/other-user contexts: **NOT_APPLICABLE** / follow-up) |
| EVIDENCE | `gate_ekmi.txt` |
| CLEANUP | n/a |

---

## GATE F — Port 53 conflict safety

| Field | Value |
|-------|--------|
| TEST ID | F-PORT53-BUSY |
| ENVIRONMENT | Elevated; service running; filtering disabled |
| PRECONDITIONS | Bound `127.0.0.1:53` UDP+TCP |
| COMMAND/ACTION | `blockads-cli enable` |
| EXPECTED | Fail safe; DNS unchanged |
| ACTUAL | `ENGINE_ERROR: DNS listen port already in use`; `dns_unchanged=True` |
| RESULT | **PASS** |
| EVIDENCE | `gate_cdef_elevated.txt` |
| CLEANUP | Released sockets |

IPv6 `[::1]:53` conflict: **NOT_APPLICABLE** this run (IPv4-focused; stack present but not separately occupied).

---

## GATE G — Real DNS apply

| Field | Value |
|-------|--------|
| TEST ID | G-DNS-APPLY |
| ENVIRONMENT | Elevated install; **medium-IL** CLI enable |
| PRECONDITIONS | Pre-snapshot Ethernet=`192.168.1.1`; GUID fix applied |
| COMMAND/ACTION | Non-elevated `blockads-cli enable` via Named Pipe |
| EXPECTED | Eligible Ethernet→`127.0.0.1`; Wi-Fi/Hyper-V/WSL/VPN untouched |
| ACTUAL | Ethernet→`127.0.0.1`; Wi-Fi stayed `192.168.1.1`; Hyper-V/WSL not owned; status `owned=true` for Ethernet GUID `{6CD1ECB6-…}` |
| RESULT | **PASS** |
| EVIDENCE | `gate_gj_dns.txt` |
| CLEANUP | Disabled / restored |

**Bug found/fixed:** `NetworkGuid` (network profile) was used instead of `AdapterName` (interface GUID) → all adapters shared one GUID → `GetInterfaceDnsSettings` ERROR_FILE_NOT_FOUND.

---

## GATE H — DNS resolution behavior

| Field | Value |
|-------|--------|
| TEST ID | H-RESOLVE |
| ENVIRONMENT | Filtering ACTIVE |
| PRECONDITIONS | Filters loaded (`stevenblack`, `easyprivacy`) |
| COMMAND/ACTION | `Resolve-DnsName`, `nslookup … 127.0.0.1` for allow + block candidates |
| EXPECTED | Allow resolves; blocked returns sinkhole |
| ACTUAL | `example.com` A records OK; `doubleclick.net` → `0.0.0.0` / `::` via local listener |
| RESULT | **PASS** for system/nslookup/Resolve-DnsName |
| EVIDENCE | `gate_gj_dns.txt` |
| CLEANUP | Disable |

DoH upstream forwarding traffic inspection: **NOT_APPLICABLE** this run (config used UDP/`1.1.1.1`). Browser Secure DNS: **NOT_APPLICABLE**.

---

## GATE I — Filter catalog network path

| Field | Value |
|-------|--------|
| TEST ID | I-FILTER-NET |
| ENVIRONMENT | Service enable path |
| PRECONDITIONS | Catalog URL reachable |
| COMMAND/ACTION | Enable (download+activate); reload without engine |
| EXPECTED | Catalog/.trie/.bloom activate; failed update keeps old |
| ACTUAL | On enable: lists loaded; transactional versioned activate used. `reload-filters` correctly errors when engine not running |
| RESULT | **PASS** (happy path). Invalid-artifact failure matrix: covered by unit tests; live HTTP truncate: **NOT_APPLICABLE** this session |
| EVIDENCE | status JSON in `gate_gj_dns.txt` / `gate_ekmi.txt` |
| CLEANUP | n/a |

---

## GATE J — Compare-and-restore

| Field | Value |
|-------|--------|
| TEST ID | J1-RESTORE / J2-EXTERNAL |
| ENVIRONMENT | Live Ethernet |
| PRECONDITIONS | Enable succeeded |
| COMMAND/ACTION | J1 disable; J2 enable→set DNS `1.1.1.1` externally→disable |
| EXPECTED | J1 restore original/DHCP semantics; J2 preserve external |
| ACTUAL | J1 `pre_eq_post=True` (Ethernet back to `192.168.1.1`); J2 `after_disable_servers=1.1.1.1`, status `DEGRADED`/`restoreConflict=true` |
| RESULT | **PASS** |
| EVIDENCE | `gate_gj_dns.txt` |
| CLEANUP | Manual restore of Ethernet to `192.168.1.1` after J2 |

Scenarios 3–5 (DHCP edge cases beyond J1, adapter disappear/reappear): **NOT_APPLICABLE** / follow-up.

---

## GATE K — Crash recovery

| Field | Value |
|-------|--------|
| TEST ID | K-KILL-RESTART |
| ENVIRONMENT | Filtering ACTIVE then `Stop-Process -Force` |
| PRECONDITIONS | Ethernet=`127.0.0.1` |
| COMMAND/ACTION | Kill service process; SCM start again |
| EXPECTED | Persisted ownership reconciled; if still BlockAds DNS → restore/re-assume per policy |
| ACTUAL | After kill DNS remained `127.0.0.1`; after restart Recover restored Ethernet (no longer `127.0.0.1`); engine stayed DISABLED |
| RESULT | **PASS** for safety restore |
| EVIDENCE | `gate_ekmi.txt` |
| CLEANUP | Continued to Gate M |

**Policy documented:** On service start, only `Recover()` runs. `config.enabled=true` does **not** currently auto-call `Enable()` after crash/restart. Service-running ≠ protection-enabled. Desired-enabled re-apply after reboot/crash is a follow-up (Gate O overlap).

---

## GATE L — Network change notification

| Field | Value |
|-------|--------|
| TEST ID | L-NETWATCH |
| RESULT | **NOT_APPLICABLE** this session (code present; no controlled adapter flap measurement) |
| NOTES | Follow-up: disable/enable NIC, VPN, count debounce coalescing |

---

## GATE M — Service stop while filtering active

| Field | Value |
|-------|--------|
| TEST ID | M-SCM-STOP-ACTIVE |
| ENVIRONMENT | ACTIVE then `blockads-service stop` |
| EXPECTED | Restore owned DNS; stop listeners; no pipe |
| ACTUAL | Ethernet restored to `192.168.1.1`; pipe connect timeout; SCM Stopped |
| RESULT | **PASS** |
| EVIDENCE | `gate_ekmi.txt` |
| CLEANUP | Uninstall |

---

## GATE N — Concurrent operations

| Field | Value |
|-------|--------|
| RESULT | **NOT_APPLICABLE** this session (controller mutex serializes mutators in code; live concurrent stress not run) |

---

## GATE O — Reboot persistence

| Field | Value |
|-------|--------|
| RESULT | **ENVIRONMENT_BLOCKED** / not executed (would disrupt host session). Policy note from Gate K applies. |

---

## GATE P — Uninstall cleanup

| Field | Value |
|-------|--------|
| TEST ID | P-UNINSTALL |
| ACTUAL | Repeated uninstall left no service; no pipe; DNS not BlockAds-owned; ProgramData may retain filters/config (intentional) |
| RESULT | **PASS** for service/pipe/DNS leftovers checked |
| EVIDENCE | Final DNS lines; `Get-Service` absent |

---

## Phase 4 entry checklist

| Item | Status |
|------|--------|
| native Windows tunnel tests | PASS |
| Windows mmap Trie/Bloom | PASS |
| SCM install/start/stop/uninstall | PASS |
| service intended account | PASS (LocalSystem) |
| live pipe ACL | PASS |
| non-elevated CLI → pipe → service | PASS |
| port 53 conflict safety | PASS |
| real adapter DNS apply | PASS |
| nslookup/Resolve-DnsName | PASS |
| known block + allow | PASS |
| DoH forwarding | NOT verified |
| compare-and-restore | PASS |
| external DNS preservation | PASS |
| crash recovery | PASS (restore; no auto re-enable) |
| filter network download + transactional reload | PASS (happy path) |
| service stop cleanup | PASS |

**PHASE 4 ENTRY: BLOCKED** until DoH forwarding is runtime-verified (and preferably Gate O on a disposable VM). Core DNS MVP safety gates otherwise PASS.
