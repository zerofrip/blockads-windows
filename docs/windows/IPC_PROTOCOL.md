# BlockAds Windows IPC Protocol

**Version:** 1  
**Transport:** Named Pipe `\\.\pipe\BlockAdsService` (Windows)  
**Framing:** newline-delimited UTF-8 JSON with **hard max message size 262144 bytes**  
**Owner:** `BlockAdsService` only

## Message envelope

### Request

```json
{
  "version": 1,
  "id": "uuid-or-counter",
  "method": "status",
  "params": {}
}
```

### Response

```json
{
  "version": 1,
  "id": "uuid-or-counter",
  "ok": true,
  "result": {},
  "error": null
}
```

On failure:

```json
{
  "version": 1,
  "id": "uuid-or-counter",
  "ok": false,
  "result": null,
  "error": {
    "code": "SERVICE_UNAVAILABLE",
    "message": "human readable"
  }
}
```

## Rules

| Rule | Detail |
|------|--------|
| Protocol version | Must be `1`. Mismatch → `PROTOCOL_VERSION` |
| Request ID | Echoed; clients should unique-ify |
| Framing | One JSON object per line; line length ≤ 262144 |
| Oversized | Connection closed; `PROTOCOL_TOO_LARGE` if reply possible |
| Malformed JSON | `PROTOCOL_ERROR`; connection may close |
| Unknown method | `METHOD_UNKNOWN` |
| No shell/exec | Methods are an allow-list only |

## Methods (v1)

| Method | Params | Result | Privilege class |
|--------|--------|--------|-----------------|
| `ping` | `{}` | `{ "pong": true }` | authenticated user |
| `status` | `{}` | Status DTO | authenticated user |
| `enable` | `{}` | Status DTO | authenticated user (service elevates) |
| `disable` | `{}` | Status DTO | authenticated user |
| `recover` | `{}` | `{ "results": [...] }` | authenticated user (admin-recommended) |
| `test_dns` | `{ "domain": "example.com" }` | `{ "rcode", "answers", "blocked" }` | authenticated user |
| `reload_filters` | `{}` | `{ "loaded": true, "lists": [...] }` | authenticated user |
| `get_stats` | `{}` | `{ "totalQueries", "blockedQueries" }` | authenticated user |

### Reserved (not required in Phase 3)

`get_config`, `set_config`, `get_query_log`, `clear_query_log`, `shutdown`

## Error codes

| Code | Meaning |
|------|---------|
| `PROTOCOL_ERROR` | Malformed envelope |
| `PROTOCOL_VERSION` | Unsupported version |
| `PROTOCOL_TOO_LARGE` | Message exceeds limit |
| `METHOD_UNKNOWN` | Not in allow-list |
| `SERVICE_UNAVAILABLE` | Pipe/service not reachable (CLI) |
| `ENGINE_ERROR` | Listener/engine failure |
| `DNS_ERROR` | DNS configure/restore failure |
| `FILTER_ERROR` | Filter download/load failure |
| `CONFLICT` | Port busy / ownership conflict |
| `INTERNAL` | Unexpected |

## Client policy

Production `blockads-cli` **must** use this pipe.

If the service is down → return `SERVICE_UNAVAILABLE`.  
**Never** silently fall back to in-process privileged DNS mutation.

Dev-only `--dev-direct` may bypass IPC for unit tests; it must be explicit.
