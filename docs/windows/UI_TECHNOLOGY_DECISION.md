# UI Technology Decision

## Candidates

| Option | Android reuse | Native look | Size / startup | Tray | Maintenance |
|--------|---------------|-------------|----------------|------|-------------|
| Compose Multiplatform | Low (VpnService/Room/Koin-bound screens) | Fair | Large runtime | Possible | New stack for team |
| WinUI 3 | None | Excellent (Win11) | Medium | Yes | Windows App SDK complexity |
| **WPF** | None | Excellent (desktop idioms) | Small–medium | Excellent | Mature; WireGuard-class apps |
| Avalonia | None | Good | Medium | Yes | Cross-platform future |
| Tauri | None | Web-skinned | Small | Yes | Extra web layer |
| Pure Go GUI | None | Weak | Small | Limited | Poor a11y |

## Evidence against Compose Multiplatform

- `HomeViewModel`, VPN permission flows, `PackageManager` app lists, Room DAOs, WorkManager — not portable
- Cosmetic CMP desktop would still rewrite most screens
- Adds Kotlin/Native or JVM desktop footprint beside Go service

## Decision

| Phase | Choice |
|-------|--------|
| 2 | **CLI only** (`blockads-cli`) — no GUI |
| 4 | **WPF (.NET)** for dashboard, lists, rules, DNS, logs, tray |
| Future | Revisit Avalonia only if multi-OS desktop becomes a goal |

Rationale: native Windows interaction patterns, strong tray/service-control ecosystem, no false reuse of Android Compose.
