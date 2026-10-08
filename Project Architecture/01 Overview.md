# Portico: overview

Portico is a personal media server with first-party apps, in the Plex and Jellyfin tradition. It's non-commercial, free, and designed for power users. The owner runs the server on their own hardware; people watch and listen through the web app and the Apple apps. A small hosted service handles only what a home server can't: accounts, sign-in across servers, discovery, and certificates.

This folder describes the system as it is. [Product Contracts](../Product%20Contracts/) say what the product must do; these documents say how it's built. [Backlog](Backlog.md) lists known open work.

## Repositories

Everything lives under `Codex Projects/Portico/`:

| Repository | Visibility | Contents |
|---|---|---|
| `apps/portico-server` | public | The server (Go), the web app, the shared client packages, the Docker image, test fixtures |
| `apps/portico-react-native` | public | The iPhone, iPad and Apple TV app (React Native, `apps/native`); future Android and TV ports go beside it |
| `portico-internal` | private | Hosted services (Hosted API, Hosted web host, DNS), deployment and operations, these documents, product contracts, brand assets, deployment information |
| `getportico-tv` | public | The public website and user documentation |
| `portico-recommendation-enhancement` | — | The title enrichment dataset build (see [Recommendations](05%20Recommendations.md)) |

Shared code has exactly one owner. Copies are identical, and a script checks them:

- **Client packages** (`packages/client-core`, `design`, `i18n`) belong to portico-server. portico-react-native keeps a copy of the parts its apps use; `node scripts/shared-packages.mjs --check` fails on any difference, and `--write` refreshes the copy.
- **apikit**, the Go API toolkit the server and Hosted share, belongs to portico-server. portico-internal keeps a copy at `hosted-services/apikit`; `scripts/shared-apikit.sh` checks it.

Scripts that need another repository find it as a sibling (`../apps/portico-server` from portico-internal, `../portico-server` from portico-react-native) or through `PORTICO_SERVER_REPO` / `PORTICO_REACT_NATIVE_REPO`.

## Components

```
                         ┌───────────────────────────────┐
  web browser ──────────►│ Portico server (owner's host)  │◄──── iPhone / iPad / Apple TV
  (served by the server  │  Go, one SQLite state folder   │      (React Native + native modules)
   or by web.getportico) │  FFmpeg for analysis/transcode │
                         └──────────────┬────────────────┘
                                        │ rare, event-driven
                         ┌──────────────▼────────────────┐
                         │ Hosted (api.getportico.tv)     │  accounts, sign-in, server directory,
                         │ PostgreSQL · web host · DNS    │  claim, certificates (DNS-01), Cast receiver
                         └────────────────────────────────┘
```

- **Server** ([Server](02%20Server.md)): scans libraries, keeps the catalogue ([Catalogue and Metadata](03%20Catalogue%20and%20Metadata.md)), serves the API, plays and transcodes media ([Playback and Media](04%20Playback%20and%20Media.md)), records Live TV, and runs recommendations ([Recommendations](05%20Recommendations.md)). One process, one state folder.
- **Clients** ([Clients](06%20Clients.md)): the web app (served by every server and, in a Hosted-only build, at web.getportico.tv) and the Apple app. Both are thin: the server composes rows, orders and pages; clients render them.
- **Hosted** ([Hosted Services](07%20Hosted%20Services.md)): identity and discovery. Day to day, clients talk to the user's server directly; Hosted sees sign-in, claim, occasional directory refreshes and certificate issuance.
- **Operations** ([Operations](08%20Operations.md)): five VPSs (Hosted, two DNS nodes, the demo, test runners), systemd everywhere, no Docker in production.

## Principles that shape the code

- **Scale.** Libraries of 5–10 million items must feel like small ones. A page load does bounded, indexed work; clients hold only what's visible. Library-wide work happens when the library changes, in background batches, never per request.
- **Fresh.** No time-based caches of derived results. A memo is only ever keyed by the exact revisions it read, so a change shows on the next load.
- **Open data.** The state folder is plain SQLite and files. It can be copied to another machine and just works. Nothing of the owner's is encrypted at rest; folder permissions protect it.
- **Owner choice, low ceremony.** Plain HTTP stays first-class. A correct username and password always sign you in. Background work is deprioritized, never paused. Quality changes only when a person asks.
- **Hosted stays cheap.** No periodic chatter; the server pushes real changes when they happen.
- **Correctness over limits.** Sorts, filters, search and "See all" are complete and correct; a fast common path sits in front of a correct full path.
- **Web and Apple first.** Other platforms come later as ports of the tvOS app, which is the reference experience.
