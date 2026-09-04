# DNS MVP Validation

**Status:** Template — fill with experimental results during Phase 2.

Environment under test: (OS build, adapters, IPv6 yes/no)

## Listener

| Check | Result | Notes |
|-------|--------|-------|
| Bind `127.0.0.1:53` UDP/TCP | UNVERIFIED_WINDOWS_RUNTIME | |
| Bind `[::1]:53` when IPv6 present | UNVERIFIED_WINDOWS_RUNTIME | |
| Port 53 occupied → fail safe + report owner | UNVERIFIED_WINDOWS_RUNTIME | Must not kill peer |

## Resolution paths

| Check | Result | Notes |
|-------|--------|-------|
| `nslookup` blocked domain | UNVERIFIED_WINDOWS_RUNTIME | |
| `nslookup` allowed domain | UNVERIFIED_WINDOWS_RUNTIME | |
| `Resolve-DnsName` | UNVERIFIED_WINDOWS_RUNTIME | |
| Win32 `DnsQuery_W` / ordinary apps | UNVERIFIED_WINDOWS_RUNTIME | |
| Browser resolution | UNVERIFIED_WINDOWS_RUNTIME | Manual |
| IPv4-only adapter | UNVERIFIED_WINDOWS_RUNTIME | |
| Dual-stack adapter | UNVERIFIED_WINDOWS_RUNTIME | |
| Multiple active eligible adapters | UNVERIFIED_WINDOWS_RUNTIME | |

## Safety

| Check | Result | Notes |
|-------|--------|-------|
| Disable compare-and-restore | UNVERIFIED_WINDOWS_RUNTIME | |
| External DNS change not overwritten | UNVERIFIED_WINDOWS_RUNTIME | |
| Crash recovery ownership | UNVERIFIED_WINDOWS_RUNTIME | |
| Adapter appear/remove | UNVERIFIED_WINDOWS_RUNTIME | |

## Network change mechanism selected

(To be filled: e.g. `NotifyIpInterfaceChange` callback vs re-eval timer.)

## Known insufficient configurations

Document any case where setting interface DNS to `127.0.0.1` / `::1` does **not** send queries to `StartStandalone` (NRPT override, MDM, DoH hard-coded apps, etc.).
