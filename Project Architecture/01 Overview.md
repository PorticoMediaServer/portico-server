# Portico: overview

Portico is a free, pre-release self-hosted media server. The owner runs a Go server with SQLite. The web app, React Native apps and Vega app browse and play directly from that server. Hosted owns global account sign-in and coordination; it does not carry ordinary media or metadata.

This folder describes implementation reported in the October programme. [Product Contracts](../Product%20Contracts/decisions.md) record required behaviour. [Backlog](Backlog.md) records unfinished implementation and acceptance. A source change or cross-compile is not proof of physical-device operation.

## Repositories

The active Ubuntu workspace is `/home/justin/projects/Portico`. The implementation programme uses isolated worktrees. Original checkouts are not integration worktrees.

| Repository | Contents |
|---|---|
| apps/portico-server | Go server, web app, canonical client-core/design/i18n packages, API toolkit and packaging |
| apps/portico-react-native | iPhone, iPad, Apple TV, Android, Android TV and Fire TV apps; Vega variant |
| portico-internal | Hosted API, Hosted web host, DNS, private operations and product documents |
| getportico-tv | Public website and documentation |
| portico-recommendation-enhancement | Private title enrichment dataset |

Shared packages belong to portico-server. Native copies are refreshed by `scripts/shared-packages.mjs`; they are not a second authority. Hosted's apikit copy follows the server toolkit. [Native sync](../../_program/reports/native-sync.md) and [Hosted](../../_program/reports/hosted.md) record the checks and integration boundaries.

## Product boundaries

- The local server owns the catalogue, metadata, playback, state, downloads, Live TV, DVR, discovery and screen composition. Clients render server-selected content.
- Titles are separate from files and versions. Missing files do not erase titles or watch state. Deleting a disk file and emptying retained missing titles are different actions.
- Intake is tolerant. A bad row, line, file or subfolder cannot discard its healthy neighbours. Bounds, authorisation and storage confinement remain enforced.
- Platform players remain primary. mpv is an additional engine on Apple and Android/Fire clients. One device capability document drives the planner.
- Original is the quality default. A viewer may select Automatic, which starts at Original and adapts to the connection. Server load does not silently reduce a fixed choice.
- Browse and search answer during background rebuilds. Server events drive quiet refresh; the console and sign-in have protected admission capacity.
- Plain HTTP works from any address. Home networks alone define local and remote. Hosted advertises TLS routes. Own-hostname proxies, overlays and IPv6 are supported designs; sub-path hosting is excluded.
- Open integration means scoped API credentials, outbound webhooks and a local MCP endpoint. There are no plugins, themes or custom CSS.
- State and backups are plain files protected by filesystem permissions. Sessions are bearer credentials tied to installation records, not device possession keys.
- No email or consumer push. Server-to-server signed wakes are coordination messages. Relay is future work. Roku, CarPlay, Android Auto and light theme are outside this programme.

## Implementation and acceptance

[Server](02%20Server.md), [Catalogue](03%20Catalogue%20and%20Metadata.md), [Playback](04%20Playback%20and%20Media.md), [Clients](06%20Clients.md), [Hosted](07%20Hosted%20Services.md) and [Operations](08%20Operations.md) give the reported implementation. Complete first-party delivery remains a release requirement. Platform capabilities that cannot work are hidden. Native engines, packaging, real networks and physical devices still need the acceptance recorded in their reports; no deployment is inferred here.

Vega sign-in, restore, control and event streams work through its origin-bound transport; the residual redirect risk was accepted by Justin on 14 Oct ([vega-transport](../../_program/reports/vega-transport.md)). The same follow-up supplies Apple scene/reinstall/admission and inactive-page source fixes; earlier native reports' gap lists must be read with it.
