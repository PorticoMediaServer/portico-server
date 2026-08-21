import assert from "node:assert/strict";
import test from "node:test";

import {
  browseFacetOptions,
  contextualMediaPlayAction,
  mediaActionsForSurface,
  resolveBrowseFacetEndpoint,
  resolveMediaActionCommand,
  withBrowseAlphaSeek
} from "../dist/productContract.js";

const base = {
  apiVersion: "v1",
  actionRevision: "v1",
  libraryKinds: [],
  entityKinds: [],
  entitySemantics: [],
  artworkRoles: [],
  browseFields: [],
  browseSorts: [],
  browseOperators: [],
  presentationFields: [],
  queryLimits: { maximumDepth: 5, maximumClauses: 40, defaultLimit: 60, maximumLimit: 200, cursorTtlSeconds: 900 },
  serverCapabilities: []
};

test("resolves API actions with encoded paths, static body, and required inputs", () => {
  const contract = {
    ...base,
    mediaActions: [{
      id: "watched.set",
      mutating: true,
      bulkSupported: true,
      command: {
        kind: "api",
        execution: "per-item",
        method: "POST",
        pathTemplate: "/api/media/{mediaId}/watched",
        requiredInputs: ["watched"],
        resultHandling: "json"
      },
      confirmation: { required: false, tone: "none" },
      invalidates: ["media", "history"]
    }]
  };
  assert.deepEqual(resolveMediaActionCommand(contract, "watched.set", { mediaId: "movie/one", watched: true }), {
    kind: "api",
    execution: "per-item",
    method: "POST",
    path: "/api/media/movie%2Fone/watched",
    body: { watched: true },
    resultHandling: "json",
    confirmation: { required: false, tone: "none" },
    invalidates: ["media", "history"]
  });
  assert.throws(() => resolveMediaActionCommand(contract, "watched.set", { mediaId: "movie" }), /requires: watched/);
});

test("returns client flows without pretending they are HTTP commands", () => {
  const contract = {
    ...base,
    mediaActions: [{
      id: "collection.add",
      mutating: true,
      bulkSupported: true,
      command: { kind: "client-flow", execution: "selection", flowId: "collection-picker", resultHandling: "flow" },
      confirmation: { required: false, tone: "none" },
      invalidates: ["collections"]
    }]
  };
  assert.deepEqual(resolveMediaActionCommand(contract, "collection.add"), {
    kind: "client-flow",
    execution: "selection",
    flowId: "collection-picker",
    resultHandling: "flow",
    confirmation: { required: false, tone: "none" },
    invalidates: ["collections"]
  });
});

test("resolves pivot facet sources into executable endpoints and browse values", () => {
  const source = {
    endpointTemplate: "/api/libraries/{libraryId}/categories",
    filterField: "filter",
    filterPrefix: "genre:",
    valueField: "name",
    labelField: "name",
    countField: "count"
  };
  assert.equal(resolveBrowseFacetEndpoint(source, "lib/movies"), "/api/libraries/lib%2Fmovies/categories");
  assert.deepEqual(browseFacetOptions(source, [
    { filter: "genre:Drama", name: "Drama", count: 14 },
    { filter: "tag:Criterion", name: "Criterion", count: 2 },
    { filter: "genre:Comedy", name: "Comedy", count: 8 }
  ]), [
    { value: "Drama", label: "Drama", count: 14 },
    { value: "Comedy", label: "Comedy", count: 8 }
  ]);
});

test("alphabet seek is one first-page title-ascending request", () => {
  assert.deepEqual(withBrowseAlphaSeek({ pivot: "movies", sort: [{ field: "title", direction: "asc" }] }, "m"), {
    pivot: "movies",
    sort: [{ field: "title", direction: "asc" }],
    seek: { prefix: "M" }
  });
  assert.throws(() => withBrowseAlphaSeek({ pivot: "movies", sort: [{ field: "title", direction: "desc" }] }, "M"), /requires title/);
  assert.deepEqual(withBrowseAlphaSeek({ pivot: "movies", sort: [{ field: "title", direction: "asc" }], cursor: "opaque" }, "M"), {
    pivot: "movies", sort: [{ field: "title", direction: "asc" }], cursor: "opaque", seek: { prefix: "M" }
  });
  assert.throws(() => withBrowseAlphaSeek({ pivot: "movies", sort: [{ field: "title", direction: "asc" }] }, "AA"), /A-Z or #/);
});

test("presents only server-projected actions eligible for the current platform", () => {
  const command = {
    kind: "api",
    execution: "single",
    method: "POST",
    pathTemplate: "/api/playback-sessions",
    requiredInputs: [],
    resultHandling: "json"
  };
  const contract = {
    ...base,
    mediaActions: [
      {
        id: "play",
        mutating: false,
        bulkSupported: false,
        presentation: {
          labelMessageId: "action.play",
          iconId: "action.play",
          group: "playback",
          priority: 100,
          surfaces: ["web", "mobile", "television"]
        },
        command,
        confirmation: { required: false, tone: "none" },
        invalidates: []
      },
      {
        id: "download",
        mutating: false,
        bulkSupported: true,
        presentation: {
          labelMessageId: "action.download",
          iconId: "action.download",
          group: "playback",
          priority: 70,
          surfaces: ["web", "mobile"]
        },
        command,
        confirmation: { required: false, tone: "none" },
        invalidates: []
      },
      {
        id: "media.delete",
        mutating: true,
        bulkSupported: true,
        presentation: {
          labelMessageId: "action.delete-media",
          iconId: "action.delete",
          group: "administration",
          priority: 10,
          surfaces: ["web-admin"]
        },
        command,
        confirmation: { required: true, tone: "destructive" },
        invalidates: []
      }
    ]
  };

  assert.deepEqual(
    mediaActionsForSurface(contract, ["download", "media.delete", "play"], "television").map(({ id, label }) => ({ id, label })),
    [{ id: "play", label: "Play" }]
  );
  assert.deepEqual(
    mediaActionsForSurface(contract, ["download", "play"], "mobile").map(({ id }) => id),
    ["play", "download"]
  );
  assert.deepEqual(mediaActionsForSurface(contract, ["media.delete"], "mobile"), []);
});

test("contextualizes only a server-projected play action with canonical product language", () => {
  const command = {
    kind: "api",
    execution: "single",
    method: "POST",
    pathTemplate: "/api/playback-sessions",
    requiredInputs: [],
    resultHandling: "json"
  };
  const contract = {
    ...base,
    mediaActions: [{
      id: "play",
      mutating: false,
      bulkSupported: false,
      presentation: {
        labelMessageId: "action.play",
        iconId: "action.play",
        group: "playback",
        priority: 100,
        surfaces: ["web", "mobile", "television"]
      },
      command,
      confirmation: { required: false, tone: "none" },
      invalidates: []
    }]
  };
  const [play] = mediaActionsForSurface(contract, ["play"], "web");
  const movieResume = contextualMediaPlayAction(play, { kind: "movie", progressSeconds: 90 });
  const showResume = contextualMediaPlayAction(play, {
    kind: "show",
    playbackTarget: { kind: "episode", seasonNumber: 2, episodeNumber: 10, progressSeconds: 270 }
  });
  const showStart = contextualMediaPlayAction(play, {
    kind: "show",
    playbackTarget: { kind: "episode", seasonNumber: 3, episodeNumber: 1 }
  });
  const albumStart = contextualMediaPlayAction(play, { kind: "album" });

  assert.equal(contextualMediaPlayAction(undefined, { kind: "movie", progressSeconds: 90 }), undefined);
  assert.deepEqual({ label: movieResume?.label, id: movieResume?.labelMessageId }, { label: "Resume", id: "action.resume" });
  assert.deepEqual({ label: showResume?.label, id: showResume?.labelMessageId }, { label: "Resume S2E10", id: "action.resume-episode" });
  assert.deepEqual({ label: showStart?.label, id: showStart?.labelMessageId }, { label: "Play S3E1", id: "action.play-episode" });
  assert.deepEqual({ label: albumStart?.label, id: albumStart?.labelMessageId }, { label: "Play album", id: "action.play-album" });
});
