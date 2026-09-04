# UI Architecture

## Role

`BlockAds.Windows` is a non-elevated WPF client. The service remains the sole privileged controller.

## IPC

| Item | Value |
|------|--------|
| Protocol | v1 |
| Pipe | `\\.\pipe\BlockAdsService` |
| Framing | newline-delimited UTF-8 JSON |
| Max message | 256 KiB |
| Methods used | `ping`, `status`, `enable`, `disable`, `reload_filters`, `get_stats` |

Client: `Services/BlockAdsIpcClient.cs`  
Connect timeout 3s, request timeout 30s, bounded reads.

## State mapping

`ProtectionStateMapper` maps service DTOs → UI states:

| UI state | Rule (summary) |
|----------|----------------|
| Service unavailable | Pipe/status unreachable |
| Recovery required | `recoveryRequired` or engine `RECOVERY_REQUIRED` |
| Degraded | engine `DEGRADED`, or ACTIVE without ownership |
| Active | ACTIVE + filtering + healthy listener + OWNED |
| Starting / Stopping | transitional |
| Off | DISABLED / not filtering |

`desiredProtection == ENABLED` alone never implies Active.

## Tray

Menu: Open, Protection label, Enable, Disable, Refresh, Exit UI.  
Exit UI → cancel polling, dispose tray, release UI mutex — **no** disable.

## Preferences

`%LocalAppData%\BlockAds\ui-preferences.json` — window geometry / minimize-to-tray only.  
Authoritative protection state is never stored in UI preferences.

## Testing

- Unit: `BlockAds.Windows.Tests` (mapper + ViewModel lifecycle)  
- VM: enable/disable DNS readback, close-while-active, tray exit, service restart reconnect
