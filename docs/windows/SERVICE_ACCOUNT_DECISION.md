# Service Account Decision

## Candidates

| Account | DNS mutate | Bind :53 | ProgramData | DoH egress | Future CA | Future Wintun |
|---------|------------|----------|-------------|------------|-----------|---------------|
| LocalSystem | Yes | Yes | Yes | Yes | Yes | Yes |
| LocalService | Often No | Maybe localhost | Limited | Yes | Hard | No |
| NetworkService | Often No | Unreliable | Limited | Yes | Hard | No |
| Virtual service account | Needs ACL grants | Needs grants | Needs ACL | Yes | Needs grants | Needs grants |

## APIs used now (Phase 2–3)

- `GetAdaptersAddresses`
- `GetInterfaceDnsSettings` / `SetInterfaceDnsSettings` (admin-level)
- Bind `127.0.0.1:53` / `[::1]:53`
- `%PROGRAMDATA%\BlockAds` state
- Named Pipe server
- HTTPS filter download / DoH

## Decision (Phase 3)

**Run as `LocalSystem`.**

Rationale: `SetInterfaceDnsSettings` and privileged port 53 binding are not reliably available to LocalService without fragile machine-wide ACL surgery. Least-privilege is deferred until a virtual service account with explicit capability grants is proven on Win10/11.

## Revisit triggers

- Phase 6 Wintun / driver install may keep LocalSystem or require install-time privilege only
- Phase 7 certificate store writes: still LocalSystem or explicit admin consent UI
- If Microsoft documents a supported non-SYSTEM path for per-interface DNS mutation, re-evaluate

## Configuration

Installer sets service account to LocalSystem. Do not grant interactive desktop.
