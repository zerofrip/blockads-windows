# Architecture Audit — BlockAds Windows Port

**Branch:** `windows`  
**Audit date:** 2026-09-04  
**Source base:** `main` @ a997619 (Android BlockAds)

Classification legend:

| Class | Meaning |
|-------|---------|
| **A** | Platform-independent / directly reusable |
| **B** | Reusable after minor abstraction |
| **C** | Android-specific / must be replaced |
| **D** | Not relevant on Windows |
| **E** | Requires further investigation |

---

## 1. Filtering & DNS engine

| Subsystem | Class | Concrete sources | Notes |
|-----------|-------|------------------|-------|
| DNS filtering (TUN path) | A | `tunnel/engine.go` (`handleDNSQuery`), `tunnel/interceptor.go`, `tunnel/packet.go` | Packet path needs TUN; logic is portable |
| DNS filtering (standalone) | A | `tunnel/engine.go` (`StartStandalone`, `ServeDNS`, `serveDNS`) | **Primary Phase 2 reuse**; binds `127.0.0.1` / `[::1]` |
| Domain Trie matching | B | `tunnel/trie.go` | Matching logic portable; **mmap uses unix only** → abstract |
| Bloom pre-filter | B | `tunnel/bloom.go` | Same mmap issue as Trie |
| Filter-list compiler | A | `tunnel/compiler.go` (`CompileFilterList`, `parseDomainLine`) | Hosts / `\|\|domain^` / plain domain |
| EasyList / AdGuard domain rules | A | `compiler.go` `parseDomainLine`; cosmetic/scriptlets separate | Domain-blocking subset only in compiler |
| Precompiled trie/bloom download | B | `app/.../FilterListRepository.kt`, `FilterDownloadManager.kt` | Format reusable; download/persistence must be Go on Windows |
| Custom block/allow rules | B | `FilterListRepository.kt` (`hasCustomRule`, `isBlocked`), `CustomRuleParser.kt`, `CustomDnsRuleDao.kt` | Precedence traced; pure-Go port after characterization tests |
| Allow/whitelist domains | B | `WhitelistDomainDao.kt`, `FilterListRepository.loadWhitelist` | Treated as allow override (same as custom allow) |
| Logging (DNS query) | B | `tunnel/engine.go` `LogCallback`, `DnsLogDao.kt` | Callback interface reusable; persistence platform-specific |
| Filter list auto-update | C→B | `FilterUpdateWorker.kt`, `FilterUpdateScheduler.kt` | Semantics reusable; WorkManager → Windows timer/service |
| Settings model | B | `AppPreferences.kt` (DataStore), Room entities | Need JSON/cross-platform config; fields map cleanly |
| Import/export | B | `SettingsBackup.kt`, settings UI | Format can be shared; I/O differs |

### Custom-rule precedence (traced)

From `FilterListRepository.hasCustomRule` + `engine.serveDNS` / `handleDNSQuery`:

1. **Custom allow** (and whitelist) → forward (override `0`) — checked before tries  
2. **Custom block** → block (override `1`)  
3. **Security trie** then **ad trie**  
4. Kotlin `IsBlocked` fallback (currently only custom/whitelist paths; tries already in Go)  
5. Upstream forward  

Parent/wildcard walk: exact domain, then each parent, then `*.parent` at each level (`checkDomainAndParents`).

---

## 2. Go tunnel / networking

| Subsystem | Class | Concrete sources | Notes |
|-----------|-------|------------------|-------|
| Go tunnel engine | A | `tunnel/engine.go` | Keep gomobile API |
| gVisor netstack | A | `tcp_ip_stack.go`, `tcp_stack_handlers.go`, tun2socks/gvisor deps | Phase 6 |
| TCP handling | A | `tcp_ip_stack.go`, `fulltunnel.go` | Needs TUN backend |
| UDP handling | A | same + DNS interceptor | |
| DNS interception (TUN) | A | `interceptor.go` | |
| DoH / DoT / DoQ / Plain | A | `tunnel/resolver.go` | `SocketProtector` for dial; standalone uses `nil` protector |
| WireGuard outbound | B | `wireguard.go`, `outbound_wireguard.go`, `wireguard_bind.go` | Mostly portable; Windows routing isolation needed |
| HTTPS MITM | B | `mitm_*.go` | Engine portable; CA trust store is platform |
| CA generation | A | `mitm_ca.go` | PEM files on disk |
| CA installation | C | Android Settings / user cert flow | Windows Certificate Store |
| Scriptlet injection | A | `scriptlet_parser.go`, `scriptlet_runtime.go`, `mitm_inject.go` | |
| Cosmetic filtering | A | `mitm_filter.go`, cosmetic CSS paths from repo | |
| Passthrough domains | A | `mitm_trust.go`, curated list in Go/Kotlin | |
| Socket protection | B | `SocketProtector` in `engine.go`; Android `VpnService.protect` | Windows: no-op in DNS-only; route bypass in tunnel mode |
| UID resolver | C→E | `uid_resolver.go`, `GoTunnelAdapter.setupUidResolver` | Keep as-is until Phase 5; future `ProcessResolver` |
| Application identification | C | `AppNameResolver.kt`, `AppResolver` / `AppUidResolver` | Windows process identity later |
| VPN lifecycle | C | `AdBlockVpnService.kt` | Replace with Service + DNS/tunnel controllers |
| Full-tunnel mode | A/B | `fulltunnel.go`, always-on in recent Android | Phase 6 with Wintun candidate |

---

## 3. Android platform surface

| Subsystem | Class | Concrete sources |
|-----------|-------|------------------|
| Android foreground service | C | `AdBlockVpnService.kt`, `RootProxyService.kt`, `NotificationHelper.kt` |
| Boot auto-start | C | `BootReceiver.kt` → Windows Service start type |
| Android notification | C | `NotificationHelper.kt` → tray / toast |
| Quick Settings | D | `AdBlockTileService.kt` |
| Widgets | D | `AdBlockWidgetProvider.kt`, `WidgetToggleReceiver.kt` |
| Battery optimization | D | onboarding / `BatteryMonitor.kt` |
| Root / iptables mode | D | `IptablesManager.kt`, `RootProxyService.kt`, libsu |
| Compose UI | C | `app/.../ui/**` |
| Android TV | D | `blockadstv/` |
| DI / Room / WorkManager | C | `AppModule.kt`, `AppDatabase.kt`, workers |

**Note:** Root Proxy’s standalone DNS path is the closest architectural cousin to Windows DNS-only mode (`startStandalone(15353)` + redirect). Windows uses DNS Client configuration instead of iptables.

---

## 4. Build / CI / packaging

| Item | Class | Sources |
|------|-------|---------|
| Gradle modules | C | `settings.gradle.kts` (`:app`, `:blockadstv`) |
| gomobile AAR | C | `scripts/build_tunnel.sh`, `app/libs/tunnel.aar` |
| GitHub Actions | B | `.github/workflows/ci.yml`, `deploy.yml`, `update_tunnel.yml` — add Windows job |
| License | A | GPL-3.0 `LICENSE` |

---

## 5. mmap portability gap (Phase 1 P1)

`tunnel/trie.go` and `tunnel/bloom.go` call `unix.Mmap` / `unix.Munmap` directly.

**Required:** single `mappedFile` abstraction in `mmap_unix.go` / `mmap_windows.go`; zero OS calls in trie/bloom.

---

## 6. Least-disruptive target layout

Do **not** reorganize into `core/` / `platform/` trees yet.

```text
tunnel/          # shared engine (portable mmap, later custom rules)
app/             # Android unchanged
blockadstv/      # Android TV unchanged
windows/         # NEW: service, CLI, DNS config, later UI
docs/windows/    # port docs
```

---

## 7. Future abstractions (document only until needed)

| Future capability | Do not introduce in Phase 1 |
|-------------------|----------------------------|
| `ProcessResolver` | Phase 5 |
| Packet `NetworkInterceptor` | Phase 6 |
| Windows `CertificateManager` | Phase 7 |

Keep `UIDResolver`, `AppUidResolver`, `AppResolver`, `SocketProtector` unchanged through Phase 1–2.
