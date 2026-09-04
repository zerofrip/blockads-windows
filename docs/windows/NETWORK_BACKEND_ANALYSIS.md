# Network Backend Analysis

**Goal:** Choose Windows interception strategies that preserve BlockAds behavior while minimizing privileged kernel code.

## Options evaluated

### 1. DNS Client configuration → localhost listener (DNS-only)

| Criterion | Assessment |
|-----------|------------|
| Admin | Yes (change interface DNS; bind port 53) |
| Driver | None |
| License | N/A (Win32 APIs) |
| Install complexity | Low (service + DNS change) |
| Overhead | Minimal (DNS path only) |
| IPv4 / IPv6 | Both (set IPv4 and IPv6 DNS servers) |
| UDP / TCP DNS | Via `StartStandalone` |
| DNS interception | Yes (system resolver) |
| Per-process ID | No (DNS-only) |
| VPN coexistence | Partial — must **exclude** VPN/virtual adapters (see DNS_ADAPTER_POLICY) |
| WireGuard coexistence | Do not touch WG/Wintun adapters |
| HTTPS filtering later | Not sufficient alone |
| Signing | No driver signing |
| Reuse Go engine | **Yes** — `Engine.StartStandalone` |
| Maintainability | High |

**Production APIs:** `GetAdaptersAddresses`, `GetInterfaceDnsSettings`, `SetInterfaceDnsSettings` (`DNS_INTERFACE_SETTINGS`). No `netsh`/PowerShell in production path.

**API availability (verified against Microsoft docs):**

| API | Minimum |
|-----|---------|
| `GetAdaptersAddresses` | Vista+ |
| `GetInterfaceDnsSettings` / `SetInterfaceDnsSettings` | **Windows 10 Build 19041+** (2004 / 20H1) |

Windows 10 builds older than 19041 get a documented `ErrNotSupported` from the platform configurator (no silent fallback to `netsh`). Primary target Win11 / secondary Win10 x64 assumes 19041+.

### 2. Wintun + existing gVisor/tun2socks path

| Criterion | Assessment |
|-----------|------------|
| Admin | Yes |
| Driver | Wintun (signed kernel driver) |
| License | Wintun GPL-2.0 — **compatibility with GPL-3.0 app must be re-validated at Phase 6** |
| Overhead | Full userspace stack |
| Reuse Go engine | Best match to Android TUN `Engine.Start(fd)` |
| Per-process | Needs Windows flow→PID mapping |
| VPN coexistence | Competing default routes — careful design |

Already a transitive dependency via `golang.zx2c4.com/wireguard`. **Candidate for Phase 6, not irreversible.**

### 3. WinDivert

| Criterion | Assessment |
|-----------|------------|
| Driver | Yes (WinDivert) |
| License | LGPL-3.0 (generally OK with GPL-3.0 if dynamic/linked carefully) |
| Reuse netstack | Divert-to-userspace ≠ TUN fd; more glue |
| Signing | Driver signing required for distribution |

Phase 6 fallback if Wintun path fails license or coexistence tests.

### 4. Windows Filtering Platform (WFP)

| Criterion | Assessment |
|-----------|------------|
| Kernel callout | High complexity / signing if custom callout |
| User-mode | Useful for metadata / some redirects |
| Maintainability | Steep learning curve |

Avoid custom callout drivers in early phases. May use user-mode WFP for process metadata in Phase 5.

### 5. NRPT / DNS client policy

Useful supplement for domain-specific DNS, not a full substitute for system-wide ad blocking. May help enterprise edge cases; not Phase 2 primary.

---

## Decision matrix

| Capability | DNS-only (Phase 2) | Wintun+netstack | WinDivert | WFP callout |
|------------|--------------------|-----------------|-----------|-------------|
| System-wide ad DNS block | Best | Yes | Yes | Yes |
| No kernel driver | Yes | No | No | Often No |
| Reuse StartStandalone | Yes | Partial | No | No |
| Reuse full tunnel MITM | No | Best | Medium | Medium |
| Privilege surface | Admin service | Admin + driver | Admin + driver | High |
| Phase | **2 (chosen)** | **6 candidate** | 6 alt | Later |

## Chosen architecture

```text
Phase 2: DNS-only
  BlockAdsService/CLI
    → Engine.StartStandalone(53)
    → DnsConfigurator (native IP Helper / NetIO)
    → conservative adapter eligibility

Phase 6: re-evaluate Wintun vs WinDivert vs user-mode WFP
  with benchmarks; do not lock now
```

## Explicit non-goals for Phase 2

- No WinDivert/Wintun packaging
- No packet tunnelling
- No shell-based DNS configuration in production

