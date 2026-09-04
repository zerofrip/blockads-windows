# DNS Adapter Eligibility Policy

**Principle:** If uncertain whether an adapter should be modified, **do not** modify it.

BlockAds must never call `SetInterfaceDnsSettings` on every interface from `GetAdaptersAddresses`.

## Stable identity

Use **Interface GUID** and/or **LUID**, never display name, as the key in recovery state.

## Categories

| Category | Modify DNS? | Detection hints (initial) |
|----------|-------------|---------------------------|
| Physical / primary Internet (Ethernet, Wi-Fi, WWAN) | **Yes** if Up + has gateway or Internet connectivity | `IfType` ethernet/ieee80211/wwan; oper status Up; not loopback |
| Loopback | **No** | `IF_TYPE_SOFTWARE_LOOPBACK` |
| BlockAds-created adapters | **No** | Future: known description/GUID prefix |
| Wintun | **No** | Description/friendly name contains `Wintun`; WireGuard tunnel type |
| WireGuard / other VPN | **No** | `IF_TYPE_PROP_VIRTUAL`, `IF_TYPE_TUNNEL`, common VPN descriptions (WireGuard, OpenVPN, TAP-Windows, Cisco, NordLynx, etc.) |
| Hyper-V / Default Switch | **No** | Hyper-V descriptions; `vEthernet` |
| WSL | **No** | `WSL`, `Hyper-V Virtual Ethernet Adapter` used by WSL |
| Docker / virtual switches | **No** | DockerNAT, `veth`, bridge descriptions |
| Disconnected / down | **No** | OperStatus != Up |
| Tunnel interfaces (generic) | **No** | `IF_TYPE_TUNNEL` |

## Initial conservative policy (Phase 2)

Include adapter only if **all** hold:

1. OperStatus is Up (or equivalent “connected”)
2. Not loopback
3. Not tunnel / proprietary virtual / WWAN-optional (WWAN may be included later; Phase 2: Ethernet + Wi-Fi only)
4. Description/name does not match denylist keywords: `wintun`, `wireguard`, `hyper-v`, `vethernet`, `wsl`, `docker`, `virtualbox`, `vmware`, `tap-windows`, `openvpn`, `nordlynx`, `zerotier`, `tailscale`, `cloudflare warp`, `blockads`
5. Has at least one unicast IPv4 or IPv6 address suitable for Internet use

## Multi-adapter

When multiple eligible adapters are Up, apply localhost DNS to **each eligible** adapter independently, with per-adapter recovery records. Compare-and-restore is per identity.

## Network change handling

On adapter arrive/depart/reconnect (notification or controlled re-evaluation):

- New eligible Up adapter → snapshot → apply (if controller ACTIVE)
- Adapter gone → drop recovery entry for that identity (do not resurrect stale names)
- Adapter still present but DNS no longer equals BlockAds-applied → mark conflict; do not overwrite

Prefer Windows network change notifications (`NotifyIpInterfaceChange` / related) over aggressive polling. Document chosen mechanism in implementation comments and `DNS_MVP_VALIDATION.md`.

## Policy versioning

Recovery state stores `policyVersion` so future stricter/looser rules can migrate safely.
