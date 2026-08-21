# LAN discovery contract

Portico Server publishes `_portico._tcp.local.` through mDNS/DNS-SD. The SRV port is the server's single configured service port (`32500` by default), which accepts local HTTP and public TLS on the same TCP listener. LAN discovery never invents a second API or playback port.

## TXT schema version 1

| Key | Required | Meaning |
| --- | --- | --- |
| `txtVersion=1` | yes | Version of this TXT contract. Unknown versions are ignored. |
| `scheme=http` | yes | LAN clients use local HTTP. Public plaintext is rejected by the server. |
| `path=/` | yes | Base path for the current Portico Server API and bundled UI. |
| `fingerprint=sha256:…` | yes | SHA-256 fingerprint of the server's persistent Ed25519 public key. |
| `serverId=…` | when claimed | Hosted server ID. It is empty for a local-only or not-yet-claimed server. |
| `name=…` | yes | Human-facing server name. It is display data, never identity. |

Native clients provide resolved SRV/TXT/address observations to `normalizePorticoDiscoveryRecord`. Client Core validates and normalizes records, expires them using the DNS TTL, merges duplicate interface announcements by fingerprint, and generates deterministic `lan` route candidates. Platform adapters own mDNS permissions, browsing, interface events, and app lifecycle through `PorticoLANDiscoveryProvider`.

## Trust rules

An mDNS response is an untrusted reachability hint. A name, `serverId`, or fingerprint merely repeated by the discovered endpoint does not establish trust.

- A Hosted-account client trusts a LAN route only when both the advertised identity and `/api/remote-access/health` match the server ID and fingerprint in its signed Hosted route document.
- A local-only client may trust a fingerprint after an explicit first-use confirmation or pairing ceremony, then persist that pin in platform secure storage.
- A changed pinned fingerprint is a different or reset server and requires explicit recovery; clients must not silently replace it.
- Records with the same fingerprint but conflicting non-empty server IDs, ports, or paths are quarantined with no route candidates.
- Expired records remain displayable only as stale history and cannot produce connection candidates.
- A `serverId` without a trusted fingerprint is never sufficient for authentication.

The connection probe and authentication still enforce the server API's normal authorization. Discovery metadata must never carry credentials, bearer tokens, account email, library names, or media details.

## Native adapter expectations

Adapters should browse `_portico._tcp` continuously while their owning screen/runtime is active, forward found/updated/lost events, preserve IPv6 scope IDs, supply observed time and TTL where the platform exposes them, and stop promptly when the supplied `AbortSignal` is aborted. Providers should not cache records beyond TTL; Client Core also performs its own stale check.

Recommended platform implementations are Network.framework/Bonjour on Apple platforms and Android NSD on Android. Desktop shells may use an equivalent DNS-SD provider. The provider is injected; Client Core has no dependency on a particular native library.
