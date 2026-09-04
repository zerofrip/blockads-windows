# Porting Status

| Component | Android | Windows | Status | Notes |
|-----------|---------|---------|--------|-------|
| Architecture audit docs | N/A | docs/windows/* | IMPLEMENTED | Phase 0 |
| Portable mmap | unix mmap | windows MapView | TESTED | Phase 1 done |
| Trie/Bloom load | Yes | Same files | TESTED | mapped_file_test |
| Custom rules (Go) | Kotlin DomainChecker | Pure Go | IMPLEMENTED | Android still uses Kotlin |
| StartStandalone DNS | Root Proxy | Phase 2 core | IMPLEMENTED | Controller wraps StartStandalone |
| DnsConfigurator | VpnService DNS | Native IP Helper | TESTED | Get/SetInterfaceDnsSettings; VM-validated |
| Adapter policy | UID/app bypass | Eligibility policy | TESTED | Ethernet/Wi-Fi + Hyper-V guest NIC |
| blockads-cli | N/A | CLI | TESTED | production IPC client; --dev-direct explicit |
| Windows Service | Foreground svc | SCM service | TESTED | SCM install/start/stop; VM-validated |
| Desktop UI | Compose | WPF | IMPLEMENTED | Phase 4 — docs/windows/PHASE4_WPF.md |
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

