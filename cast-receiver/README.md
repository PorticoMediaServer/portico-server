# The shared Cast receiver — `cast.getportico.tv`

One Google Cast application serves every Portico server. Its receiver is a static page; this
folder says how to publish it. There is no service here and nothing to run: the page never calls
Portico's Hosted API, so it costs nothing per viewer (document 47).

## What is served

Exactly the files in `portico-server/server/receiver/cast/` (`index.html`, `receiver.js` and `assets/`), unchanged.
They are the single source: the same bytes are compiled into every server at `/receiver/cast/`.
Publish them at the root of `https://cast.getportico.tv/`.

## How it finds a viewer's server

The sender (web today; iOS and Android when their Cast SDKs are added) opens a Cast session and
sends, on the `urn:x-cast:tv.getportico.cast` channel:

    {"type": "pair", "origin": "https://current.<token>.direct.getportico.tv:<port>", "code": "ABC123"}

`origin` is the viewer's server, which must be HTTPS; `code` is a one-use pairing code that server
issued seconds ago (`POST /v1/cast/bootstrap`). The page redeems the code with that server for its
own short-lived credential and from then on talks only to that server.

## Who controls the television

The sender that paired is the **controller**. Only the controller can load, play, seek, change
tracks or receive status; any connected sender can pause or stop, as with a TV remote. Other
senders that try to control it get `busy`. On boot the receiver never reconnects to, or obeys,
whichever sender speaks first.

When a second person pairs while someone is casting, the receiver keeps the current pairing and
asks: the television shows who is asking, and the controller receives `takeover-requested` and
answers `takeover {allow}`. No answer within 30 seconds means no. A sender on the same account and
server switches immediately. If the controller has left, the television announces the switch and
makes it after 10 seconds. The replaced sender receives `replaced`.

Cast is offered whenever the viewer's server has a secure HTTPS address, whether the person signed
in with a Portico Account or directly on the server.

## Server side

Every Portico server allows the browser origin `https://cast.getportico.tv` by default
(`cmd/server/main.go`), alongside `https://web.getportico.tv`.

## Headers (from the previous deployment)

    Content-Security-Policy: default-src 'self' https://www.gstatic.com; script-src 'self' https://www.gstatic.com; connect-src https:; media-src https: blob:; img-src https: data:; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'none'
    Referrer-Policy: no-referrer
    Cache-Control: no-store

## Registration (owner task)

In the Google Cast SDK Developer Console, register a **Custom Receiver** whose URL is
`https://cast.getportico.tv/`, publish it, and give the resulting application ID to the clients at
build time: `VITE_CAST_APPLICATION_ID` for the web app. Until that is set, no client offers Cast.
The Default Media Receiver (`CC1AD845`) cannot be used: it cannot pair with a server.

## Not done yet

How the receiver actually plays (HLS through the Cast player rather than a bare video element,
the Cast device's codec profile, subtitles, audio tracks) belongs to the playback lane and is
covered by document 50.
