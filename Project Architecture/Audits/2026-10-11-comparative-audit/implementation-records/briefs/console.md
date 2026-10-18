# Lane: console — the web screens for everything the server lanes just built

Worktree: `/home/justin/projects/Portico/.worktrees/audit-implementation/server-console` (branch `audit-implementation/console`). You can commit. `node_modules` is a symlink; do not install. Your report is `/home/justin/projects/Portico/.worktrees/audit-implementation/_program/reports/console.md`.

Server lanes have added many capabilities with routes, `client-core` services and parsers, and strings, but no screens. Your job is the screens in the web app (and the shared settings structure where a setting belongs there), in Portico's existing design language. Use the frontend conventions already in `web/` and `packages/design`: study two or three existing owner screens (Access, Maintenance, Storage, Connectivity, Libraries) and match them exactly — layout, components, empty and error states, confirmation patterns, string catalogue use, accessibility. Do not invent a new look.

Justin's settings philosophy (programme brief, section 1): good defaults an owner can override, no wall of switches; one drop-down with named choices where that fits, a plain number where the owner knows the number; plain sentences; nothing that needs a manual to understand.

Read each lane's report in `/home/justin/projects/Portico/.worktrees/audit-implementation/_program/reports/`, sections "API changes", "Owner or viewer settings added or wired" and "Needed from others", then build:

1. **API access** (`api-platform.md`): owner API keys (create with name, scopes, optional expiry; the secret shown once with a copy button; last used; revoke), members' personal access tokens (each member's own list in their account area; the owner's list of all members' tokens with revoke), a short plain explanation of scopes, and the per-credential limits setting.
2. **Webhooks** (`api-platform.md`): list, add and edit (address, events as a grouped checklist, secret), enable or disable, send a test, and the delivery log with status and retry state.
3. **MCP** (`api-platform.md`): one small panel that says what it is, shows the address to give an assistant, links to creating a key, and has the enable setting if one exists.
4. **Needs attention** (`providers-matching.md`): per library, the list of unmatched and low-confidence titles with the reason and top candidates, and the four actions (accept a candidate, search by hand, enter a provider id, dismiss); a count badge where libraries are listed.
5. **Metadata settings** (`providers-matching.md`): the provider per library (TMDB by default, others, "local metadata only"), provider keys where a provider needs the owner's own, the local artwork storage limit with current usage, and Background work speed (Gentle, Normal, Fast).
6. **Titles and files** (`identity.md`): on a title's page, the list of versions with a way to choose one to play and (owner) delete one; a "missing" state on cards and pages for titles whose files are gone; the Missing titles panel with the owner's rule (immediately, after N days, never) and "empty now"; remove a library, with a clear confirmation of what goes and what is kept; merge two titles and split one.
7. **Network** (`networking.md`): home networks (the list that defines local versus remote, with add and remove and a note of the visitor's own address and how it is classified), trusted reverse proxies, the external address to advertise, the connection limit, and the owner certificate's status. Sub-path hosting is not supported; do not offer it.
8. Anything else those four reports list as needing a web screen.

## Boundaries
- More server lanes will finish while you work (Live TV, subtitles, conversion, operations and restart, accounts and members, playback quality and player engine settings, downloads, Watch Together). Do not build their screens from guesses; a second pass will bring them. If a report is present for one of them by the time you finish the list above, you may continue with it.
- A **web-subtitles** lane is working in the web player. Stay out of the player.
- Do not change server behaviour. If a route or field you need is missing or awkward, use what exists and write exactly what is needed under "Needed from others".
- Every string through the catalogue; every list tolerant of a bad row (the web-app lane's convention: see `/home/justin/projects/Portico/.worktrees/audit-implementation/_program/reports/web-app.md`); phone width must work.
- Checks: `npx tsc --noEmit` in `web`, the string and design lints if quick, and test files for what you add. Nothing can be checked in a browser by you; leave a click-through list per screen.
