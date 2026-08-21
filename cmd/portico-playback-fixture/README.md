# Portico playback fixture

This explicit developer/test command starts a real Portico HTTP server on an
ephemeral **loopback-only** port. It creates a fresh private database, uses the
normal auth, profile, playback-planning, session, grant, and media-delivery
handlers, and generates five copyright-free two-second sources with FFmpeg:

- `fixture-direct`: H.264/AAC in MP4 (direct-play baseline)
- `fixture-remux`: H.264/AAC in Matroska (container-remux baseline)
- `fixture-transcode`: MPEG-4 Part 2/MP3 in AVI (codec-transcode baseline)
- `fixture-multitrack`: H.264 with English/French AAC tracks and an English
  WebVTT subtitle in Matroska (selection and renegotiation baseline)
- `fixture-music`: audio-only MP3 (music playback baseline)
- `fixture-missing`: a database-visible source whose file is deliberately absent

Run from `apps/portico-server`:

```sh
go run ./cmd/portico-playback-fixture --manifest /private/path/fixture.json
```

The manifest is atomically written with mode `0600` and contains the ephemeral
base URL, random short-lived owner/viewer credentials, stable profile/media
identifiers, and the lifecycle scenarios clients should exercise through the
public API. Stop the process to shut down workers and remove the default
temporary app-data directory. Passing `--app-data` deliberately retains it.
The process also exits automatically when the manifest's two-hour expiry is
reached.

This command deliberately does not add an environment variable or fixture mode
to `porticod`. It does not synthesize API responses. It supplies deterministic
inputs for session creation, media-grant renewal/revocation/expiry behavior, and
renegotiation generation changes; each client remains responsible for asserting
the real API response appropriate to its advertised capability tuple.
For a deterministic server-side smoke assertion, an `avkit` / `tvOS` / version
`18` static capability profile resolves the three sources respectively to
`direct_play`, `direct_stream`, and `transcode_required`. Portico Web sends its
short-lived browser probe while the server retains authority: bounded reviewed
browser bands authorize only Portico's conservative playback baseline.

The fixture requires FFmpeg encoders `libx264`, `aac`, `mpeg4`, `libmp3lame`,
and `webvtt`. Hardware acceleration, HDR/Dolby Vision, live TV, and physical
network/device behavior cannot be truthfully represented by a loopback fixture;
they remain named server conformance-corpus, device-lab, and OS gates. Long-form
stall and throughput adaptation also remain separate soak gates.

The private manifest also contains a random control capability. It can arm one
exact `/api/` path for a single bounded delay, 404, or 410 response. The route
is available only over loopback, returns 404 without the exact bearer secret,
and is implemented only by this command's outer test handler.

The Web supervisor runs the focused real-server browser proof and always
removes its private state and child processes:

```sh
cd web
npm run test:e2e:playback
```

That proof covers browser UI login and direct-player source binding, direct byte
delivery, H.264/AAC Matroska remux to the real managed-HLS baseline, real
MPEG-4/MP3 codec transcode, two-audio/one-subtitle discovery, revision-checked
audio and subtitle renegotiation with fresh producer generations and grants,
audio-only music delivery, truthful missing-source failure, media-grant rotation
and revocation, a bounded one-shot response delay, and a non-cacheable one-shot
resource failure followed by visible manual retry, rebuilt HLS, and a fresh
post-fault session. Live-edge, HDR/Dolby Vision,
hardware acceleration, physical network faults, and long-form stalls remain the
explicit external gates described above.
