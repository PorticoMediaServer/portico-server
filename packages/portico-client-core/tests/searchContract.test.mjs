import assert from "node:assert/strict";
import test from "node:test";

import {
  orderSearchGroups,
  resolveSearchRequest,
  resolveSearchResultSemantic,
  searchGroup,
  searchSort
} from "../dist/searchContract.js";

const groupOrder = ["movies", "shows", "episodes", "people", "music", "audiobooks", "live-tv"];
const titleGroups = groupOrder.slice(0, -1);
const mediaGroups = titleGroups.filter(group => group !== "people");
const contract = {
  apiVersion: "v1",
  entitySemantics: [
    { id: "movie", container: false, playable: true, parentKinds: [], childKinds: [], childOrder: ["title", "id"], defaultDestination: "detail", primaryArtworkRole: "poster" },
    { id: "show", container: true, playable: false, parentKinds: [], childKinds: ["season"], childOrder: ["seasonNumber", "id"], defaultDestination: "detail", primaryArtworkRole: "poster" },
    { id: "person", container: true, playable: false, parentKinds: [], childKinds: ["movie", "show", "episode"], childOrder: ["releaseDate", "title", "id"], defaultDestination: "detail", primaryArtworkRole: "poster" },
    { id: "live-channel", container: false, playable: true, parentKinds: [], childKinds: ["live-program"], childOrder: ["startTime", "id"], defaultDestination: "detail", primaryArtworkRole: "logo" }
  ],
  search: {
    revision: "v1",
    endpoint: "/api/search",
    groupOrder,
    groups: [
      { id: "movies", title: "Movies", entityKind: "movie", resultKinds: ["movie"], supportsLibraryScope: true, sorts: ["relevance", "title", "releaseYear", "dateAdded"] },
      { id: "shows", title: "Shows", entityKind: "show", resultKinds: ["show", "anime", "season"], supportsLibraryScope: true, sorts: ["relevance", "title", "releaseYear", "dateAdded"] },
      { id: "episodes", title: "Episodes", entityKind: "episode", resultKinds: ["episode"], supportsLibraryScope: true, sorts: ["relevance", "title", "releaseYear", "dateAdded"] },
      { id: "people", title: "People", entityKind: "person", resultKinds: ["person"], supportsLibraryScope: true, sorts: ["relevance", "title"] },
      { id: "music", title: "Music", entityKind: "track", resultKinds: ["artist", "album", "track"], supportsLibraryScope: true, sorts: ["relevance", "title", "releaseYear", "dateAdded"] },
      { id: "audiobooks", title: "Audiobooks", entityKind: "book", resultKinds: ["audiobook"], supportsLibraryScope: true, sorts: ["relevance", "title", "releaseYear", "dateAdded"] },
      { id: "live-tv", title: "Live TV Channels", entityKind: "live-channel", resultKinds: ["live_channel"], supportsLibraryScope: false, sorts: ["relevance"] }
    ],
    sorts: [
      { id: "relevance", label: "Best match", directions: ["desc"], defaultDirection: "desc", applicableGroups: groupOrder },
      { id: "title", label: "Title", directions: ["asc", "desc"], defaultDirection: "asc", applicableGroups: titleGroups },
      { id: "releaseYear", label: "Release year", directions: ["asc", "desc"], defaultDirection: "desc", applicableGroups: mediaGroups },
      { id: "dateAdded", label: "Date added", directions: ["asc", "desc"], defaultDirection: "desc", applicableGroups: mediaGroups }
    ],
    filters: [
      { id: "entityKinds", label: "Result types", valueType: "enum", multiple: true, allowedValues: ["movies", "movie", "shows", "show", "episodes", "episode", "people", "person", "music", "audiobooks", "audiobook", "live-tv"] },
      { id: "libraryIds", label: "Libraries", valueType: "identity", multiple: true, source: { endpoint: "/api/libraries", valueField: "id", labelField: "name" } },
      { id: "group", label: "Result group", valueType: "enum", multiple: false, allowedValues: groupOrder }
    ],
    facetMode: "none",
    limits: { minimumQueryLength: 1, maximumQueryLength: 120, defaultGroupLimit: 8, maximumGroupLimit: 50, quickInitialGroupLimit: 3, quickMaximumGroups: 6, quickMaximumItemsPerGroup: 6, fullDefaultGroupLimit: 50 },
    cursor: { mode: "independent-group", opaque: true, requiresSingleGroup: true, principalBound: true, scopeFields: ["query", "group", "libraryIds", "sort", "direction"], ttlSeconds: 3600, expiredErrorCode: "cursor_expired", invalidErrorCode: "invalid_cursor" },
    resultSemantics: {
      destinationSource: "entitySemantics.defaultDestination",
      hierarchySource: "entitySemantics.parentKinds+childKinds+childOrder",
      artworkRoleSource: "entitySemantics.primaryArtworkRole",
      kindMappings: [
        { resultKind: "movie", entityKind: "movie" },
        { resultKind: "anime", entityKind: "show" },
        { resultKind: "person", entityKind: "person" },
        { resultKind: "live_channel", entityKind: "live-channel" }
      ]
    }
  }
};

test("search request defaults are contract-owned for quick and full surfaces", () => {
  assert.deepEqual(resolveSearchRequest(contract, "  Fargo  ", { mode: "quick" }), {
    query: "Fargo",
    sort: "relevance",
    direction: "desc",
    limit: 3
  });
  assert.deepEqual(resolveSearchRequest(contract, "Fargo", { mode: "full", sort: "title" }), {
    query: "Fargo",
    sort: "title",
    direction: "asc",
    limit: 50,
    recordHistory: true,
    entityKinds: titleGroups
  });
});

test("continuation, scope, sort, and result-kind validation follow the published contract", () => {
  assert.equal(searchGroup(contract, "live-tv")?.supportsLibraryScope, false);
  assert.equal(searchSort(contract, "title")?.defaultDirection, "asc");
  assert.throws(() => resolveSearchRequest(contract, "Fargo", { cursor: "opaque" }), /requires one result group/);
  assert.equal(resolveSearchRequest(contract, "Fargo", { mode: "full", group: "movies", cursor: "opaque" }).recordHistory, undefined);
  assert.throws(() => resolveSearchRequest(contract, "Fargo", { group: "live-tv", sort: "title" }), /not available/);
  assert.deepEqual(resolveSearchRequest(contract, "Frances", { group: "people", entityKinds: ["person"], mode: "full" }), {
    query: "Frances", sort: "relevance", direction: "desc", limit: 50, recordHistory: true, group: "people", entityKinds: ["person"]
  });
  assert.throws(() => resolveSearchRequest(contract, " "), /1-120 characters/);
});

test("group ordering and hierarchy/artwork destination resolution are deterministic", () => {
  const groups = [
    { id: "live-tv", title: "Live TV", entityKind: "live-tv", items: [], hasMore: false },
    { id: "movies", title: "Movies", entityKind: "movie", items: [], hasMore: false },
    { id: "music", title: "Music", entityKind: "music", items: [], hasMore: false }
  ];
  assert.deepEqual(orderSearchGroups(contract, groups, 2).map((group) => group.id), ["movies", "music"]);
  assert.deepEqual(resolveSearchResultSemantic(contract, "anime"), {
    resultKind: "anime",
    entityKind: "show",
    destination: "detail",
    parentKinds: [],
    childKinds: ["season"],
    childOrder: ["seasonNumber", "id"],
    artworkRole: "poster"
  });
  assert.equal(resolveSearchResultSemantic(contract, "unknown"), undefined);
});
