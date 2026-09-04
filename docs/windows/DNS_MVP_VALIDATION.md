# DNS MVP Validation

**Status:** Phase 3 runtime validation executed on Windows 11 build 26200 (see `PHASE3_RUNTIME_VALIDATION.md`). System DNS mutation paths verified with compare-and-restore; DoH traffic inspection and reboot persistence remain open.

Environment under test: Windows 11 Pro 10.0.26200 x64; Go 1.27.0; branch `windows`.

## Listener

| Check | Result | Notes |
|-------|--------|-------|
| Bind `127.0.0.1:53` UDP/TCP | PASS | Via enable path after port free |
| Bind `[::1]:53` when IPv6 present | UNVERIFIED | Listener claimed in status; dedicated bind conflict not run |
| Port 53 occupied → fail safe + report owner | PASS | Enable failed; DNS unchanged |

## Resolution paths

| Check | Result | Notes |
|-------|--------|-------|
| `nslookup` blocked domain | PASS | `doubleclick.net` → `0.0.0.0` via 127.0.0.1 |
| `nslookup` allowed domain | PASS | `example.com` |
| `Resolve-DnsName` | PASS | Allow + sinkhole |
| Win32 `DnsQuery_W` / ordinary apps | PASS | System resolver via interface DNS |
| Browser resolution | UNVERIFIED | Manual |
| IPv4-only adapter | PASS | Ethernet |
| Dual-stack adapter | PARTIAL | AAAA observed via nslookup |
| Multiple active eligible adapters | N/A | Only Ethernet eligible/up |

## Safety

| Check | Result | Notes |
|-------|--------|-------|
| Disable compare-and-restore | PASS | DHCP/empty NameServer restored to `192.168.1.1` |
| External DNS change not overwritten | PASS | Kept `1.1.1.1`; DEGRADED/conflict |
| Crash recovery ownership | PASS | Restart Recover restored owned localhost DNS |
| Adapter appear/remove | UNVERIFIED | Gate L follow-up |

## Network change mechanism selected

Phase 3: `NotifyIpInterfaceChange` + debounce in `netwatch` (runtime flap counts: not measured this session).

## Known insufficient configurations / bugs fixed in validation

- **BUG:** `GetInterfaceDnsSettings` used `IP_ADAPTER_ADDRESSES.NetworkGuid` (network profile) instead of `AdapterName` (interface GUID). Fixed to use `AdapterName`.
- **BUG:** Windows mmap prevented delete/rename of mapped filter files; filter pipeline moved to versioned immutable files + `CloseFilters`.
- DoH upstream not proven by traffic capture this session (UDP upstream configured).

