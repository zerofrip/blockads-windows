# Porting Status

| Component | Android | Windows | Status | Notes |
|-----------|---------|---------|--------|-------|
| Architecture audit docs | N/A | docs/windows/* | IMPLEMENTED | Phase 0 |
| Portable mmap | unix mmap | windows MapView | TESTED | Phase 1 done |
| Trie/Bloom load | Yes | Same files | TESTED | mapped_file_test |
| Custom rules (Go) | Kotlin DomainChecker | Pure Go | IMPLEMENTED | Android still uses Kotlin |
| StartStandalone DNS | Root Proxy | Phase 2 core | IMPLEMENTED | Controller wraps StartStandalone |
| DnsConfigurator | VpnService DNS | Native IP Helper | IMPLEMENTED | Get/SetInterfaceDnsSettings; runtime UNVERIFIED |
| Adapter policy | UID/app bypass | Eligibility policy | IMPLEMENTED | Ethernet/Wi-Fi conservative |
| blockads-cli | N/A | CLI | IMPLEMENTED | production IPC client; --dev-direct explicit |
| Windows Service | Foreground svc | SCM service | IMPLEMENTED | SCM install/start/stop; runtime PENDING_DEDICATED |
| Desktop UI | Compose | WPF | NOT_STARTED | Phase 4 |
| Process identity | UID/package | PID/path | NOT_STARTED | Phase 5 |
| Packet tunnel | VpnService TUN | Wintun candidate | NOT_STARTED | Phase 6 — not locked |
| HTTPS MITM | Yes | Cert store | NOT_STARTED | Phase 7 |
| WireGuard | Yes | Isolated | NOT_STARTED | Phase 8 |
| Installer | APK | MSI (WiX) | NOT_STARTED | Phase 9 |
| Windows CI | ubuntu Android | windows-latest Go | IMPLEMENTED | windows-go.yml |

Status values: `NOT_STARTED` | `IN_PROGRESS` | `IMPLEMENTED` | `TESTED` | `BLOCKED` | `NOT_APPLICABLE`

| IPC protocol v1 | N/A | Named Pipe | IMPLEMENTED | docs/windows/IPC_PROTOCOL.md |
| Service config schema | DataStore | PROGRAMDATA JSON | IMPLEMENTED | versioned atomic writes |
| Filter download pipeline | FilterDownloadManager | Go filters.Manager | IMPLEMENTED | transactional ReplaceTriesAtomic |
| Network change watch | NetworkMonitor | NotifyIpInterfaceChange | IMPLEMENTED | debounce + ReevaluateAdapters |
