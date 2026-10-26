# Hosted read-only geometry

One bounded measurement pass for the CSS-only Hosted workspace glow artifact
`7021853c7898`, plus the public create/reset/verify stage geometry. It never
edits, submits a public form, deploys, or writes outside the private Hosted
testing folder.

## Use

```sh
PORTICO_WALKS_PLAYWRIGHT=/path/to/playwright/index.mjs \
PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH=/usr/bin/chromium \
node deployment/testing/hosted/geometry.mjs
```

Defaults: origin `https://web.getportico.tv`, credential file
`/home/justin/projects/Portico/.private/hosted-testing/justin-testing.json`
(owned regular file, mode 0600, read by reference only), viewport width 1440,
heights `800,520`, receipt
`/home/justin/projects/Portico/.private/hosted-testing/geometry-7021853c7898.json`.

`--skip-account` measures only the public stages and never opens the credential
file. `--origin`, `--credentials`, `--out`, `--width` and `--heights` are the
only options; credential values are never accepted on the command line.

## What it measures

- For the signed-in account workspace at each height, before and after scrolling
  the document: viewport and scroll values, workspace and glow top/bottom/height
  in document coordinates, the glow's exact computed `inset`, `backgroundImage`
  and background colour, and whether the glow covers the full document and the
  viewport before and after scroll (`coversFullPageAndViewport`). The workspace
  is identified only by its own `workspace` class token and must own a direct
  `glow` child; a glow parent is never inferred as the workspace. Both must be
  visible, otherwise the receipt says `workspace-missing` and `accountMeasured`
  stays false.
- For the public create, reset and verify stages, read-only: stage and backdrop
  boxes, the backdrop's exact computed inset and gradients, and the same coverage
  booleans. No form is submitted.
- Sign-in is the ordinary Hosted UI, with the credential file's `username`
  preferred in the username-or-email field. If a signed-in session lands on the
  Join/directory page, the flow detects the authenticated UI, then navigates
  `/account` and waits for the real workspace; it never submits credentials a
  second time. If an MFA or verification code is asked for, it stops with
  `mfa-required`; it never triggers mail, recovery or a provider action.
- On success it signs out in `finally`, even if geometry throws, through the
  ordinary workspace Sign out control, ending only this browser's current
  family. It never touches all-devices operations. If a credential submission
  started but sign-in did not confirm, the server family cannot be proven absent,
  so the receipt says `signOut: uncertain` rather than `not-signed-in`. Cleanup
  uncertainty is always a closed, explicit fact.

The receipt contains only closed status codes, numbers, exact CSS values and
stage names. It holds no account, email, label, URL, page-error text or
screenshot. A nonpersistent browser context is used throughout.
