# Porting Status

| Component | Android | Windows | Status | Notes |
|-----------|---------|---------|--------|-------|
| Architecture audit docs | N/A | docs/windows/* | IMPLEMENTED | Phase 0 |
| Portable mmap | unix mmap | windows MapView | NOT_STARTED | Phase 1 P1 |
| Trie/Bloom load | Yes | Same files | NOT_STARTED | Depends on mmap |
| Custom rules (Go) | Kotlin DomainChecker | Pure Go | NOT_STARTED | After characterization |
| StartStandalone DNS | Root Proxy | Phase 2 core | NOT_STARTED | Reuse as-is |
| DnsConfigurator | VpnService DNS | Native IP Helper | NOT_STARTED | Compare-and-restore |
| Adapter policy | UID/app bypass | Eligibility policy | NOT_STARTED | DNS_ADAPTER_POLICY |
| blockads-cli | N/A | CLI | NOT_STARTED | Phase 2 |
| Windows Service | Foreground svc | SCM service | NOT_STARTED | Phase 3 |
| Desktop UI | Compose | WPF | NOT_STARTED | Phase 4 |
| Process identity | UID/package | PID/path | NOT_STARTED | Phase 5 |
| Packet tunnel | VpnService TUN | Wintun candidate | NOT_STARTED | Phase 6 — not locked |
| HTTPS MITM | Yes | Cert store | NOT_STARTED | Phase 7 |
| WireGuard | Yes | Isolated | NOT_STARTED | Phase 8 |
| Installer | APK | MSI (WiX) | NOT_STARTED | Phase 9 |
| Windows CI | ubuntu Android | windows-latest Go | NOT_STARTED | Phase 1 |

Status values: `NOT_STARTED` | `IN_PROGRESS` | `IMPLEMENTED` | `TESTED` | `BLOCKED` | `NOT_APPLICABLE`
