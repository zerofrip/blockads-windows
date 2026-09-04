# Feature Parity Map

Statuses: `SUPPORTED` | `PLANNED` | `DIFFERENT_DESIGN` | `NOT_APPLICABLE` | `BLOCKED`

| Android | Windows | Status | Notes |
|---------|---------|--------|-------|
| MainActivity | Main desktop window | PLANNED (P4) | WPF |
| Foreground service | Windows Service | PLANNED (P3) | CLI elevates in P2 |
| Notification | Tray + toast | PLANNED (P4) | |
| QS tile | Tray quick toggle | PLANNED (P4) | |
| Home widget | — | NOT_APPLICABLE | |
| Boot receiver | Service Automatic | PLANNED (P3) | |
| App selection (packages) | Executable/process selection | PLANNED (P5) | |
| PackageManager | Process / AppX resolver | PLANNED (P5) | |
| VpnService permission | Service install + admin | DIFFERENT_DESIGN | |
| CA install Settings | Certificate Store consent UI | PLANNED (P7) | |
| VPN DNS filtering | DNS-only localhost | PLANNED (P2) | MVP |
| Full tunnel + HTTPS | Wintun/etc candidate | PLANNED (P6–7) | Backend not locked |
| Root / iptables | — | NOT_APPLICABLE | |
| Filter lists / trie | Same Go engine + downloads | PLANNED (P2) | |
| Custom rules | Pure-Go checker | PLANNED (P1) | Match Android precedence |
| DoH providers | `resolver.go` | PLANNED (P2) | |
| WireGuard profiles | Isolated outbound | PLANNED (P8) | |
| Import/export | JSON backup | PLANNED | |
| Android TV | — | NOT_APPLICABLE | |
| Battery optimization | — | NOT_APPLICABLE | |
| Crash reporting opt-in | Optional later | PLANNED | Local-first |
