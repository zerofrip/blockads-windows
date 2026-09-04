# Service Security — Named Pipe

## Pipe name

```text
\\.\pipe\BlockAdsService
```

## Security descriptor (SDDL)

```text
D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;GRGW;;;IU)(A;;GRGW;;;S-1-5-11)
```

| ACE | SID | Access |
|-----|-----|--------|
| SY | LocalSystem | Generic All |
| BA | Built-in Administrators | Generic All |
| IU | Interactive Users | Read/Write (connect + message) |
| S-1-5-11 | Authenticated Users | Read/Write (local authenticated) |

Explicit denials are implicit for:

- Anonymous (`AN`)
- Network users remotely (Named Pipes are local-only via `\\.\pipe\`; remote `\\server\pipe\` is not published)

Do **not** use a world-writable DACL (`WD` / Everyone GA).

## Impersonation

Phase 3 does **not** require client impersonation for `enable`/`disable`.

The service runs privileged and performs DNS mutations itself after accepting an authenticated local pipe client.

Future hardening may inspect the client token (session ID / elevation) for `recover` / config writes.

## Method privilege classes

| Method | Client elevation | Service action |
|--------|------------------|----------------|
| status, ping, test_dns, get_stats | Normal interactive user | Read-only |
| enable, disable, reload_filters | Normal interactive user | Privileged inside service |
| recover | Normal user accepted; admin recommended | Privileged restore |

The CLI must **not** be elevated for normal enable/disable when the service is installed.

## Single ownership

| Mechanism | Purpose |
|-----------|---------|
| SCM unique service name `BlockAdsService` | One service registration |
| Named mutex `Global\BlockAdsController` | Prevent second controller (service + --dev-direct) |
| Exclusive pipe listen | Second listener fails |

PID files alone are not used.
