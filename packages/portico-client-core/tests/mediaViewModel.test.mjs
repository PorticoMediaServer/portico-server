import assert from "node:assert/strict";
import test from "node:test";

import {
  resolveMediaCardViewModel,
  resolveMediaDetailViewModel,
  resolveMediaViewModel
} from "../dist/mediaViewModel.js";

const contract = {
  search: {
    resultSemantics: {
      kindMappings: [
        { resultKind: "live_channel", entityKind: "live-channel" },
        { resultKind: "interactive_story", entityKind: "interactive-story" }
      ]
    }
  },
  entitySemantics: [
    {
      id: "live-channel",
      container: false,
      playable: true,
      parentKinds: [],
      childKinds: ["live-program"],
      childOrder: ["startTime", "id"],
      defaultDestination: "detail",
      primaryArtworkRole: "logo"
    },
    {
      id: "interactive-story",
      container: true,
      playable: false,
      parentKinds: [],
      childKinds: ["interactive-chapter"],
      childOrder: ["indexNumber", "id"],
      defaultDestination: "children",
      primaryArtworkRole: "banner"
    }
  ],
  artworkRoles: [
    { id: "poster", aspectRatio: 2 / 3, fit: "cover", purpose: "Portrait browsing art" },
    { id: "logo", aspectRatio: 16 / 9, fit: "contain", purpose: "Channel identity" },
    { id: "banner", aspectRatio: 3, fit: "cover", purpose: "Wide story identity" }
  ]
};

test("non-poster artwork role publishes contract geometry and source role", () => {
  const view = resolveMediaCardViewModel(contract, {
    id: "channel-7",
    libraryId: "live-tv",
    entityKind: "live-channel",
    title: "Portico News",
    artwork: { poster: "/poster.jpg", backdrop: "", thumb: "/thumb.jpg" },
    userState: { watched: false, watchlisted: false, favorite: true, progressSeconds: 0 },
    availability: { status: "available", fileCount: 1, missingFileCount: 0 },
    actions: ["play"],
    fields: {}
  });

  assert.equal(view.kind, "live-channel");
  assert.deepEqual(view.destination, { kind: "detail", entityId: "channel-7" });
  assert.deepEqual(view.artwork, {
    role: "logo",
    sourceRole: "thumb",
    url: "/thumb.jpg",
    purpose: "Channel identity",
    accessibilityLabel: "Portico News",
    shape: { aspectRatio: 16 / 9, fit: "contain" }
  });
  assert.equal(view.state.favorite, true);
  assert.deepEqual(view.actionIds, ["play"]);
});

test("future contract kinds require no client kind table", () => {
  const view = resolveMediaDetailViewModel(contract, {
    id: "story-1",
    type: "interactive-story",
    title: "Choose the Stars",
    sortTitle: "Choose the Stars",
    addedAt: "2026-07-16T00:00:00Z",
    genres: [],
    labels: [],
    tags: [],
    images: { poster: "", backdrop: "/story-backdrop.jpg", thumb: "" },
    artwork: { banner: "/story-banner.jpg" },
    state: { watched: false, watchlisted: true, favorite: false, progressSeconds: 120, rating: 0 },
    actions: ["play"]
  });

  assert.equal(view.kind, "interactive-story");
  assert.equal(view.semantics.known, true);
  assert.equal(view.semantics.container, true);
  assert.deepEqual(view.semantics.childKinds, ["interactive-chapter"]);
  assert.deepEqual(view.destination, { kind: "children", entityId: "story-1" });
  assert.equal(view.artwork.role, "banner");
  assert.equal(view.artwork.url, "/story-banner.jpg");
  assert.deepEqual(view.artwork.shape, { aspectRatio: 3, fit: "cover" });
});

test("full detail resources publish canonical availability without a web adapter", () => {
  const partial = resolveMediaDetailViewModel(contract, {
    id: "episode-availability",
    type: "episode",
    title: "The Missing Reel",
    sortTitle: "Missing Reel",
    addedAt: "2026-07-16T00:00:00Z",
    genres: [],
    labels: [],
    tags: [],
    images: { poster: "", backdrop: "", thumb: "" },
    artwork: {},
    missing: false,
    fileCount: 3,
    missingFileCount: 1,
    state: { watched: false, watchlisted: false, favorite: false, progressSeconds: 0, rating: 0 },
    actions: []
  });
  assert.deepEqual(partial.availability, { status: "partial", fileCount: 3, missingFileCount: 1 });

  const unavailable = resolveMediaDetailViewModel(contract, {
    id: "episode-unavailable",
    type: "episode",
    title: "Gone",
    sortTitle: "Gone",
    addedAt: "2026-07-16T00:00:00Z",
    genres: [],
    labels: [],
    tags: [],
    images: { poster: "", backdrop: "", thumb: "" },
    artwork: {},
    missing: true,
    fileCount: 1,
    missingFileCount: 1,
    state: { watched: false, watchlisted: false, favorite: false, progressSeconds: 0, rating: 0 },
    actions: []
  });
  assert.equal(unavailable.availability.status, "unavailable");
});

test("wire aliases resolve through the Product Contract kind mapping", () => {
  const view = resolveMediaViewModel(contract, {
    id: "story-alias-1",
    type: "interactive_story",
    title: "Mapped Future",
    images: { poster: "", backdrop: "", thumb: "" }
  });

  assert.equal(view.sourceKind, "interactive_story");
  assert.equal(view.kind, "interactive-story");
  assert.equal(view.semantics.known, true);
  assert.deepEqual(view.destination, { kind: "children", entityId: "story-alias-1" });
});

test("unpublished kinds remain visible through conservative defaults", () => {
  const view = resolveMediaViewModel(contract, {
    id: "future-raw-1",
    entityKind: "spatial-memory",
    title: "Unknown Tomorrow",
    artwork: { thumb: "/future-thumb.jpg" }
  });

  assert.equal(view.kind, "spatial-memory");
  assert.equal(view.sourceKind, "spatial-memory");
  assert.equal(view.semantics.known, false);
  assert.deepEqual(view.destination, { kind: "detail", entityId: "future-raw-1" });
  assert.equal(view.artwork.role, "poster");
  assert.equal(view.artwork.sourceRole, "thumb");
  assert.equal(view.artwork.url, "/future-thumb.jpg");
  assert.deepEqual(view.artwork.shape, { aspectRatio: 2 / 3, fit: "cover" });
});
