# Canonical Product Glossary

Contract ID: `PC-GLOSSARY`

Status: Approved

Revision: portico.october.1. Foundation and document 32 resolve the ownership and playback distinctions below.

This glossary owns cross-contract meanings. A family contract may narrow a term for its domain but MUST NOT redefine it.

| Term | Canonical meaning |
| --- | --- |
| Portico Account | Global identity owned by Hosted Services. It can access zero, one, or many Portico Servers and is not itself a server session. |
| Local account | Identity owned by exactly one Portico Server and authenticated directly with that server. It is not promoted to Hosted authority. |
| Profile | An authority-scoped viewing identity beneath an account: Hosted Services owns Portico Account profiles and a Portico Server owns direct/local profiles. A selected server receives only the permitted projection. A profile owns viewer preferences and state where the applicable contract says so; it is not a login identifier. |
| Server | One Portico Server authority, its media catalogue, local policy, and server-native credential families. |
| Hosted Services | Portico's global identity and coordination control plane. It does not own media authority or carry ordinary media bytes. |
| Control plane | Privacy-minimized global coordination for account identity, server claims/directory membership, signed policy/routes, DNS/certificates, and operator health. It is never the ordinary selected-server interaction or media data path. |
| Data plane | Direct communication between a client and the selected Portico Server for authorized browsing, metadata projection, playback, progress, queues, downloads, Live/DVR, receivers, social playback, and server events. |
| Direct Sign-In | Authentication directly against a selected Portico Server without using a Portico Account for that sign-in. User-facing language is “Sign in directly to a server.” |
| Selected | Durable product state chosen by the user, such as server, profile, library pivot, quality, version, audio track, or subtitle track. |
| Focused | Ephemeral input target that will receive the next activation. Focus is not selection. |
| Current | The one destination or entity presently rendered by the owning navigation authority. Current is not necessarily focused or selected. |
| Automatic | A user-selected policy that delegates a bounded decision to Portico. Automatic is itself an explicit choice and its contract defines which dimensions Portico may adapt. |
| Explicit choice | A concrete selection made by the user. It MUST be preserved until the user changes it or the contract requires a clear failure; it is never silently treated as Automatic. |
| Product page | A full-screen or full-window application destination outside active video playback and transient floating chrome. |
| Video canvas | The region owned by active video presentation. It contains the decoded video and unavoidable letterboxing only; decorative product backgrounds do not enter it. |
| Media backdrop | Artwork intentionally used as environmental context behind a media destination. It takes precedence over the default Portico atmosphere where the Visual Contract permits it. |
| Portico atmosphere | The signature deep blue-black canvas with restrained blue light/glow used when a product page has no applicable media backdrop. It is not a generic gradient decoration. |
| Floating menu | Temporary chrome presented over another destination or over playback. It remains visually subordinate to its underlying context. |
| Direct Play | Delivery of selected source streams without changing their encoded essence or container when the client can consume them as selected. |
| Remux | Repackaging selected streams into a different container without re-encoding their media essence. |
| Transcode | Re-encoding one or more media streams to satisfy the selected delivery plan. |
| Playback plan | Server-authoritative, client-informed decision describing the selected source, tracks, delivery method, constraints, and reasons. |
| Capability | A factual supported behavior with evidence and scope. A capability is not permission and does not authorize an action. |
| Permission | Server- or Hosted-enforced authority granted to a principal. UI capability discovery never replaces authorization. |
| Recoverable | A state from which the promised operation can continue without corrupting, duplicating, or silently changing user intent. Recoverable does not necessarily mean invisible. |
| Terminal | A state that cannot continue without a new user decision or a new operation. Terminal outcomes MUST say what happened and what action is available. |
| Conformance fixture | Platform-neutral input and expected outcome consumed by more than one implementation or verification layer. |
| Stable viewer owner | Authority-qualified account/profile/server identity that owns durable data; authorization revisions are separate fences. |
| Authorization revision | Versioned permission evidence; changes request/grant/cache eligibility without creating a new durable owner. |
| Operation receipt | Immutable evidence of a past committed intent; not a statement that its result is still current or authorized. |

## October vocabulary

| Term | Canonical meaning |
|---|---|
| Title | Logical catalogue identity and its history/state, separate from the files that currently supply it. |
| File | Physical source bytes with observed revision/custody; may disappear or be replaced without changing a recognised title. |
| Version | An available file or file grouping for a title, carrying technical quality/edition/part evidence. Persistent multipart edition grouping remains incomplete. |
| Missing title | A retained title with no available copies, marked missing until the owner's missing-title rule removes it. An unreachable root cannot establish this state. |
| Trash period | Retention of missing catalogue titles; no trash folder or recoverable disk copy. |
| Home networks | The owner's complete network list defining local versus remote. No overlay/proxy detection overrides it. |
| Engine | Native or mpv playback implementation named by the plan; Vega implements its native engine. Capability and permission remain separate. |
| Capability document | The single device document containing engine profiles, display/audio facts, Player selection and overrides. |
| Original | Default quality intent retaining source fidelity where compatible; necessary compatibility conversion does not become a lower fixed quality. |
| Automatic quality | Explicit viewer choice beginning at Original and adapting down/up to connection observations. Distinct from Automatic engine selection. |
| Needs attention | Owner video metadata queue for unmatched/uncertain titles and candidate review. It does not include every music issue. |
| Scope | One credential access category (read, playback, library, admin) required by a route in addition to current role/content permissions. |
| API key | Owner-issued digest-stored bearer credential with named scopes, independent expiry/revocation and current member authority. |
| Personal access token | Scoped integration bearer issued for a member and bounded by that member's current authority. |
| Generation | Published read/media incarnation. Browse/search generation differs from playback media generation and from personal-command revision. |
| Freshness hint | Scoped event asking a client to reread a dependency quietly; not permission or a complete replacement document. |
| Download claim | Server-side request/preparation/list entry. Its retirement does not alone revoke the device receipt. |
| Offline receipt | Signed permission and exact artifact/owner evidence with independent verification deadline; source tidying is not revocation. |
| Shared input | One upstream acquisition per source/channel, with private readers and independent viewer conversions. |
| Shared recording | One same-airing payload with independent profile-private ownership and deletion. |
| Installation ID | Random stable app-installation identifier used with device records/refresh. It is not a private possession key. |
