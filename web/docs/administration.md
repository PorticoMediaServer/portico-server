# Server administration — information architecture

Server administration is the **Server** heading of Settings (since the frontend
rework of 2 October 2026). There is no separate console: the rail stays where it
is, and an owner sees the Server pages listed under the account pages. The same
pages are native on iPhone and iPad; Apple TV lists the Dashboard (an Apple TV
linked with a code is signed in as whoever approved it, so the owner's TV shows it).

The pages, their sections and their rows are **authored** in
`packages/client-core/src/presentation/settings-structure.ts`. The web and the
Apple app render that structure; neither decides the order or the wording. The
rules and words the panels share are in `packages/client-core/src/server-admin/`.

| Address | Page | Answers | Tabs |
| --- | --- | --- | --- |
| `/settings/server-dashboard` | Dashboard | Is the server healthy? Who is watching? What needs attention? | |
| `/settings/server-history` | Play history | What has been played on this server, by whom, on which device and when? | |
| `/settings/server-libraries` | Libraries | What media does the server know about, in what order, and how is each library scanned and matched? | a library opens with General · Metadata · Scanning · Advanced (`?id=&tab=`) |
| `/settings/server-live` | Live TV & Channels | Which tuners and playlists feed the guide, how are recordings kept, and which channels are built from the libraries? | Sources · Recording · Library Channels |
| `/settings/server-people` | People | Who can use this server and what may they see? | Accounts · Invitations · API keys |
| `/settings/server-streaming` | Playback | May the server convert media, how, and how much at once? | |
| `/settings/server-remote` | Remote access | How do devices reach this server from outside the home? | |
| `/settings/server-storage` | Storage & backups | Which storage is attached, what is in the trash, and are backups healthy? | |
| `/settings/server-schedule` | Schedule | What runs in the background, and when may heavy work run? | |
| `/settings/server-troubleshooting` | Troubleshooting | What happened, and how do I send it to support? | |
| `/settings/server-general` | General | What is this server called, and which account owns it? | |

The former `/server/<section>` addresses redirect to the page that took the
section over (`web/src/app/server-pages.ts`).

Rules applied throughout:

- **One Save per page.** A plain setting is a `server-setting` row bound to one
  of the server's settings documents (`server-admin/server-forms.ts`); a page's
  forms share one Save and one Discard in a bar at the bottom of the page.
  Records with their own revision (an account's role, a library's settings)
  save from their own dialog or tab and report to the same bar.
- **One refresh per page**, not one per card.
- A setting the server does not have is not drawn; a group whose document the
  server does not serve is left out, heading included.
- Raw identifiers, revisions, lanes and attempt counters are never headings.
- Destructive actions use `ConfirmDialog` with the target name, impact and,
  for irreversible operations, a typed confirmation.
- Loading, empty and failed panels are independent; one failure never blanks
  the page.
