# Phase 4 — WPF UI

**Status:** Implemented (see validation evidence under `artifacts/windows-validation/`)  
**Stack:** WPF, .NET 8, Windows x64  
**Project:** `windows/ui/BlockAds.Windows`

## Architecture

```text
WPF UI / Tray  →  Named Pipe IPC v1  →  BlockAdsService  →  Controller
```

The UI is non-elevated, IPC-only, and thin. It never mutates DNS, SCM, journals, routes, or port 53.

## Lifecycle independence

- Closing the main window hides to the tray (default).
- **Exit UI** exits only the WPF process.
- Exit / close never calls `disable`, never stops the service, never restores DNS.

## Protection toggle

1. Disable control / show transitional state  
2. Await IPC `enable` / `disable`  
3. Fetch authoritative `status`  
4. Render result (no optimistic lying toggle)

## Polling

- Default ~3s while UI is running  
- `status` only — never enable/disable/reload/repair  
- One in-flight poll per UI instance

## Single instance

Per-user mutex: `Local\BlockAds.Windows.UI`  
Does not use the service controller mutex.

## Recovery required

Rendered as a distinct protection state with guidance text.  
UI does **not** invoke `emergency-restore`.
