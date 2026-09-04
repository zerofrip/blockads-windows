# Evidence index — 2026-09-04 network loss

Collected without mutating host DNS.

| Path | Contents |
|---|---|
| `git/` | HEAD, status, diff vs `6025a09` |
| `programdata/config.json` | Post-incident config (`enabled: false`) |
| `programdata/state/` | Empty (no recovery journal present) |
| `scm/service.txt` | SCM query (Stopped / Automatic) |
| `scm/system_events.json` | Related system events capture (as available) |
| `validation-logs/` | Gate L / O prep scripts and logs (smoking gun: `L_dns_restored=127.0.0.1`) |
| `current_dns_readonly.txt` | Post-manual-recovery DNS read-only snapshot |
| `validation-logs/pktmon_omitted.txt` | Note that huge pktmon dump was not copied |

Narrative: `docs/windows/INCIDENT_20260904_NETWORK_LOSS.md`
