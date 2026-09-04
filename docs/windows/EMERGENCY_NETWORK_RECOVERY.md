# Emergency network recovery (Windows)

Use this when Windows has no Internet and you suspect BlockAds left adapter DNS on `127.0.0.1` / `::1` without a working local resolver.

**Prefer the smallest change.** Do not run a global network reset first.

## Provenance matters

| Situation | What to run |
|---|---|
| BlockAds ownership journal still present | `blockads-service emergency-restore` (default) |
| Localhost DNS but **no** BlockAds journal (may be another app) | Do **not** auto-reset; inspect first. Only if you are sure: `emergency-restore --force-unproven-localhost` |

Default emergency-restore restores **proven BlockAds-owned** adapters only and prints a plan classifying proven / unproven / unrelated.

## 1. Prefer BlockAds emergency-restore

```powershell
& "C:\Path\To\blockads-service.exe" emergency-restore
```

Forced unproven path (never default):

```powershell
& "C:\Path\To\blockads-service.exe" emergency-restore --force-unproven-localhost
```

Does **not** start filtering, bind port 53, download filters, or require DoH.

## 2. Manual recovery (no BlockAds required)

```powershell
Get-DnsClientServerAddress -AddressFamily IPv4 |
  Where-Object { $_.ServerAddresses -contains '127.0.0.1' } |
  ForEach-Object {
    Write-Host "Resetting DNS on $($_.InterfaceAlias)"
    Set-DnsClientServerAddress -InterfaceAlias $_.InterfaceAlias -ResetServerAddresses
  }
```

Optional IPv6:

```powershell
Get-DnsClientServerAddress -AddressFamily IPv6 |
  Where-Object { $_.ServerAddresses -contains '::1' } |
  ForEach-Object {
    Set-DnsClientServerAddress -InterfaceAlias $_.InterfaceAlias -ResetServerAddresses
  }
```

```powershell
Stop-Service BlockAdsService -ErrorAction SilentlyContinue
Resolve-DnsName example.com -Type A -DnsOnly
```

## 3. What not to do first

- Do **not** start with global Network Reset.
- Do **not** use `--force-unproven-localhost` if another local DNS product may own `127.0.0.1`.

## 4. After recovery

Leave filtering disabled until a disposable-VM retest is authorized.
