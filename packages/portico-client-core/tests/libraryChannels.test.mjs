import assert from "node:assert/strict";
import test from "node:test";
import {
  createPorticoClient,
  libraryChannelMessageIdForError
} from "../dist/index.js";

function response(body = { ok: true }) {
  return new Response(JSON.stringify(body), { status: 200, headers: { "Content-Type": "application/json" } });
}

test("Library Channel consumer requests remain separate from tuner Live TV", async () => {
  const calls = [];
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    transport: { fetch: async (input, init) => {
      calls.push({ input: String(input), init });
      if (String(input).endsWith("/tune")) return response({
        sessionId: "library-channel-session",
        sourceUrl: "/api/library-channels/channel%2Fone/hls/master.m3u8",
        directPlay: false,
        generation: 1,
        nextEventSequence: 1,
        playbackRevision: 0,
        queueRevision: 0,
        decision: {},
        media: {},
        mediaGrant: {token: "grant", expiresAt: "2099-08-07T00:00:00Z"},
        continuationCredential: {token: "continuation", origin: "https://server.example", expiresAt: "2099-08-07T00:00:00Z", generation: 1},
        selectedQualityId: "automatic",
        selectedSubtitleMode: "off",
        resources: [{id: "active", sourceUrl: "/api/library-channels/channel%2Fone/hls/master.m3u8", streamFormat: "hls", qualityId: "automatic", subtitleMode: "off", default: true}],
        audioStreams: [], subtitleStreams: [], chapters: [], qualities: [], queue: []
      });
      return response({ sourceType: "library-channel", items: [], total: 0 });
    } }
  });

  await client.libraryChannels({ limit: 40, cursor: "page-2" });
  await client.libraryChannelsGuide({ from: "2026-07-16T00:00:00Z", to: "2026-07-17T00:00:00Z", limit: 120, cursor: "channels-2" });
  await client.libraryChannelGuide("channel/one", { from: "2026-07-16T00:00:00Z", to: "2026-07-17T00:00:00Z", limit: 80, cursor: "guide-2" });
  await client.tuneLibraryChannel("channel/one", { at: "2026-07-16T12:00:00Z" });

  assert.equal(calls[0].input, "https://server.example/api/library-channels?cursor=page-2&limit=40");
  assert.equal(calls[1].input, "https://server.example/api/library-channels/guide?from=2026-07-16T00%3A00%3A00Z&to=2026-07-17T00%3A00%3A00Z&cursor=channels-2&limit=120");
  assert.equal(calls[2].input, "https://server.example/api/library-channels/channel%2Fone/guide?from=2026-07-16T00%3A00%3A00Z&to=2026-07-17T00%3A00%3A00Z&cursor=guide-2&limit=80");
  assert.equal(calls[3].input, "https://server.example/api/library-channels/channel%2Fone/tune");
  assert.equal(calls[3].init.method, "POST");
  const tuneBody = JSON.parse(calls[3].init.body);
  assert.equal(tuneBody.at, "2026-07-16T12:00:00Z");
  assert.equal(typeof tuneBody.clientInstanceId, "string");
  assert.equal(tuneBody.clientProfile.device, "Portico TypeScript Client");
});

test("Library Channel administration uses explicit admin routes and safe form upload", async () => {
  const calls = [];
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    csrfToken: "csrf",
    transport: { fetch: async (input, init) => {
      calls.push({ input: String(input), init });
      return response();
    } }
  });
  const form = new FormData();
  form.set("file", new Blob(["logo"]), "logo.png");

  await client.regenerateAdminLibraryChannel("channel one");
  await client.uploadAdminLibraryChannelLogo(form);
  await client.deleteAdminLibraryChannelLogo("lca_asset");

  assert.equal(calls[0].input, "https://server.example/api/admin/library-channels/channel%20one/regenerate");
  assert.equal(calls[0].init.method, "POST");
  assert.equal(calls[1].input, "https://server.example/api/admin/library-channels/logos");
  assert.equal(calls[1].init.headers["Content-Type"], undefined);
  assert.equal(calls[1].init.headers["X-Portico-CSRF"], "csrf");
  assert.equal(calls[1].init.body, form);
  assert.equal(calls[2].init.method, "DELETE");
});

test("Library Channel transport errors resolve to shared product language", () => {
  const expected = {
    library_channel_generation_in_progress: "library-channel.generation-in-progress",
    library_channel_generation_timeout: "library-channel.generation-timeout",
    library_channel_invalid_request: "problem.invalid-request",
    library_channel_logo_delete_failed: "problem.request-failed",
    library_channel_logo_in_use: "problem.invalid-request",
    library_channel_logo_invalid: "problem.invalid-request",
    library_channel_logo_not_found: "problem.not-found",
    library_channel_logo_store_failed: "problem.request-failed",
    library_channel_no_applicable_defaults: "library-channel.no-applicable-defaults",
    library_channel_no_playable_schedule: "library-channel.generation-no-playable-schedule",
    library_channel_not_found: "problem.not-found",
    library_channel_program_restricted: "library-channel.program-restricted",
    library_channel_program_unavailable: "library-channel.program-unavailable",
    library_channel_capacity_unavailable: "library-channel.playback-capacity",
    library_channel_segment_starting: "library-channel.playback-capacity",
    library_channel_playback_unavailable: "library-channel.program-unavailable",
    library_channel_logo_bug_overhead_required: "library-channel.logo-processing-overhead",
    library_channel_playback_policy_stale: "library-channel.revision-conflict",
    library_channel_request_failed: "problem.request-failed",
    library_channel_revision_conflict: "library-channel.revision-conflict",
    library_channel_template_exists: "problem.invalid-request"
  };
  for (const [code, messageId] of Object.entries(expected)) {
    assert.equal(libraryChannelMessageIdForError(code), messageId, code);
  }
  assert.equal(libraryChannelMessageIdForError("unknown"), undefined);
});
