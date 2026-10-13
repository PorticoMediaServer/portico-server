# Server administration — information architecture

Server administration is the **Server** heading of Settings (since the frontend
rework of 2 October 2026). There is no separate console: the rail stays where it
is, and an owner sees the Server pages listed under the account pages.
Availability is declared per platform; the Integrations and Metadata pages are
web-only in the current settings structure. Phone/tablet renderers support their
available server pages; Apple TV lists the Dashboard (an Apple TV
linked with a code is signed in as whoever approved it, so the owner's TV shows it).

The pages, their sections and their rows are **authored** in
`packages/client-core/src/presentation/settings-structure.ts`. The web and the
Apple app render that structure; neither decides the order or the wording. The
rules and words the panels share are in `packages/client-core/src/server-admin/`.

| Address | Page | Answers | Tabs |
| --- | --- | --- | --- |
| `/settings/server-dashboard` | Dashboard | Is the server healthy? Who is watching? What needs attention? | |
| `/settings/server-history` | Play history | What has been played on this server, by whom, on which device and when? | |
| `/settings/server-libraries` | Libraries | What media does the server know about, in what order, and how is each library scanned and matched? | a library opens with General · Metadata · Scanning · Needs attention · Advanced (`?id=&tab=`) |
| `/settings/server-live` | Live TV & Channels | Which tuners and playlists feed the guide, how are recordings kept, and which channels are built from the libraries? | Sources · Recording · Library Channels |
| `/settings/server-people` | People | Who can use this server and what may they see? | Accounts · Invitations · Devices |
| `/settings/server-integrations` | Integrations (web) | Which keys, tokens, webhooks and assistants may use the local API? | API keys · Members’ tokens · Webhooks · Assistants |
| `/settings/server-metadata` | Metadata (web) | Which server-wide provider credentials and artwork limits apply? | |
| `/settings/server-streaming` | Streaming | May the server convert media, how, and how much at once? Which ways of playing a file is it avoiding? | |
| `/settings/server-remote` | Remote access | How do devices reach this server from outside the home? | |
| `/settings/server-storage` | Storage & backups | Which storage is attached, which titles are missing, and are backups healthy? | |
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
  save from their own dialog or tab and report to the same bar. A panel with
  its own draft (an account's limits, who can record, recording space) joins
  the bar with `useInlineForm`; it never has a Save of its own.
- **One refresh per page**, not one per card. A panel registers with the page
  through `Freshness`, which draws nothing inside a Server page.
- A setting the server does not have is not drawn; a group whose document the
  server does not serve is left out, heading included.
- Raw identifiers, revisions, lanes and attempt counters are never headings.
- Loading, empty and failed panels are independent; one failure never blanks
  the page.

## One pattern for lists

Every list on a Server page is a `SettingsGroup` of `SettingsRow`s:

- **Create.** The group's create action is `secondary sm` in the group heading
  (`action`). Only a page that *is* its list (Libraries, Library Channels) has
  a `primary` create action in its page heading.
- **Row actions.** One visible action at most (`secondary sm`, or `ghost sm`
  for something that takes away), then a `more` menu named
  `server.row.actions` for the rest. Destructive items are last in the menu.
  The control that opens a confirmation is never danger-toned; the dialog's
  confirm button is.
- **Loading.** `SettingsLoading` ("Loading {thing}") where the rows will be,
  only for the first read.
- **Empty.** `SettingsEmpty`: one plain sentence, and a second when there is
  something to do about it.
- **Failed.** A compact `Notice` in a `SettingsNote` with Try again: `error`
  when there is nothing to show, `warning` beside what was read before.
- **Results.** An action's result is a compact success `Notice` in the group.
- **Footer.** `SettingsFooter` for a quiet action on the whole list or a
  caption.
- **Words.** Every state, kind and badge comes from the catalogue
  (`server-admin/*-words.ts`, `console-catalogue.ts`); a code this client has
  no word for reads as a general word, never as the code.

## Confirmations

`ConfirmDialog` (component spec, Confirmations). The title is "Verb object?",
the body states the consequence, the buttons are Cancel and the verb.

- Tier 1 (bounded loss): revoke a key, token or invitation; block a device;
  remove a webhook, a source, a rule, a channel, a person's downloads; delete
  a report or an upgrade copy; stop playback; restore a backup (a copy of the
  current data is kept first). Deleting a person uses the server's preview as
  the body.
- Tier 2 (files, or server-wide loss) adds one field to type a short human
  word or name into: delete from disk and delete one version (the title or
  the count the server gives), remove a library (its name), clean up a folder
  (the folder's name), delete a backup ("delete"), remove missing titles now
  (their count). Never an identifier.

## Staying current

A panel reads through `useRead` (`admin/console.ts`, client-core `QuietRead`):
what is shown stays while it is read again; a read nobody asked for never
spins, blanks or shows its own failure; `reload` is the read the owner asked
for and answers at once. A panel names what it depends on (`on`, for what the
event feed announces) or a pace (`every: QUIET_READ_EVERY_MS`, for what it
does not). A panel that reads through its own service uses `useQuietly` with
the service's `revalidate()`. A document opened in a dialog to be edited is
read once.

A change the server calls out of date (`428 revision_required`, `412`, or a
conflict code) is read again and sent again by itself: `DraftForm.save` for
the settings documents, `sendFresh` for an action on one record,
`rebaseDraft` for a panel's own draft. Only a setting changed both here and
elsewhere, or a record that is gone, is reported.
