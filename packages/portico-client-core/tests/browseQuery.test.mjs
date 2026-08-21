import assert from "node:assert/strict";
import test from "node:test";

import {
  availableFields,
  availableSorts,
  compileFilter,
  decodeExpression,
  decodeSorts,
  encodeExpression,
  encodeSorts,
  expressionToFilter,
  isBrowseExpression,
  queryChips,
  removeExpressionAtPath,
  resolveBrowseWorkspaceQuery,
  savedViewWorkspaceQuery,
  serializeSavedViewDraft
} from "../dist/browseQuery.js";

const capabilities = {
  apiVersion: "v1",
  library: { id: "lib-movies", name: "Movies", kind: "movies" },
  pivots: [
    {
      id: "movies",
      label: "Movies",
      entityKinds: ["movie"],
      browseSupported: true,
      endpointTemplate: "/api/libraries/{libraryId}/browse",
      defaultSort: [{ field: "title", direction: "asc" }],
      defaultView: "grid",
      supportedViews: ["grid", "list", "table"],
      presentationFields: ["year"]
    },
    {
      id: "albums",
      label: "Albums",
      entityKinds: ["album"],
      browseSupported: true,
      endpointTemplate: "/api/libraries/{libraryId}/browse",
      defaultSort: [{ field: "releaseDate", direction: "desc" }],
      defaultView: "list",
      supportedViews: ["list"],
      presentationFields: ["releaseDate"]
    }
  ],
  fields: [
    { id: "title", label: "Title", valueType: "string", operators: ["contains", "starts-with"], applicableKinds: ["movie", "album"], controlHint: "text", complexity: "quick", cost: "indexed" },
    { id: "year", label: "Year", valueType: "number", operators: ["equals", "between"], applicableKinds: ["movie"], controlHint: "number-range", complexity: "quick", cost: "indexed" },
    { id: "artist", label: "Artist", valueType: "identity", operators: ["in"], applicableKinds: ["album"], controlHint: "facet-multi-select", complexity: "standard", cost: "indexed-join" }
  ],
  sorts: [
    { id: "title", label: "Title", defaultDirection: "asc", directions: ["asc", "desc"], expensive: false, applicableKinds: ["movie", "album"] },
    { id: "year", label: "Year", defaultDirection: "desc", directions: ["desc"], expensive: false, applicableKinds: ["movie"] },
    { id: "releaseDate", label: "Release date", defaultDirection: "desc", directions: ["asc", "desc"], expensive: false, applicableKinds: ["album"] }
  ],
  presentationFields: ["year", "releaseDate"],
  actions: ["browse"],
  queryLimits: { defaultLimit: 60, maximumLimit: 200, maximumClauses: 20, maximumDepth: 4, maximumBytes: 65_536, cursorTtlSeconds: 300 }
};

test("browse expressions parse, encode, and compile through one portable model", () => {
  const expression = {
    all: [
      { field: "title", operator: "contains", value: "Fargo" },
      { not: { any: [{ field: "year", operator: "between", value: [1990, 1999] }] } }
    ]
  };
  assert.equal(isBrowseExpression(expression), true);
  assert.deepEqual(decodeExpression(encodeExpression(expression)), expression);
  assert.deepEqual(compileFilter(expressionToFilter(expression, capabilities.fields), capabilities.fields), expression);
});

test("portable list editor state preserves identity values containing commas", () => {
  const expression = { field: "artist", operator: "in", value: ["person:Doe, Jane", "person:Smith, John"] };
  const editor = expressionToFilter(expression, capabilities.fields);
  assert.equal(editor.children[0].kind, "condition");
  assert.deepEqual(editor.children[0].rawValues, expression.value);
  assert.equal(editor.children[0].rawValue, "");
  assert.deepEqual(compileFilter(editor, capabilities.fields), { all: [expression] });
});

test("expression parsing rejects malformed and out-of-contract JSON", () => {
  assert.equal(decodeExpression("{"), undefined);
  assert.equal(decodeExpression(JSON.stringify({ all: [] })), undefined);
  assert.equal(decodeExpression(JSON.stringify({ field: "title", operator: "contains" })), undefined);
  assert.equal(decodeExpression(JSON.stringify({ field: "title", operator: "contains", value: "Fargo", extra: true })), undefined);
  assert.equal(decodeExpression(JSON.stringify({ field: "releaseDate", operator: "is-present", value: true })), undefined);
  assert.deepEqual(decodeExpression(JSON.stringify({ field: "releaseDate", operator: "is-present", value: null })), { field: "releaseDate", operator: "is-present", value: null });
  assert.equal(decodeExpression(JSON.stringify({ not: { field: "title", operator: "contains", value: Number.NaN } })), undefined);
  let tooDeep = { field: "title", operator: "contains", value: "Fargo" };
  for (let depth = 0; depth < 65; depth += 1) tooDeep = { not: tooDeep };
  assert.equal(isBrowseExpression(tooDeep), false);
});

test("presence conditions compile to the server's sole null-valued wire shape", () => {
  const fields = [{
    id: "releaseDate",
    label: "Release date",
    valueType: "date",
    operators: ["equals", "is-present", "is-missing"],
    applicableKinds: ["movie"],
    controlHint: "date-range",
    complexity: "standard",
    cost: "indexed"
  }];
  assert.deepEqual(compileFilter({
    id: "group-1",
    kind: "group",
    mode: "all",
    negated: false,
    children: [{ id: "condition-1", kind: "condition", field: "releaseDate", operator: "is-missing", rawValue: "", negated: false }]
  }, fields), { all: [{ field: "releaseDate", operator: "is-missing", value: null }] });
});

test("workspace normalization is capability-owned and deterministic", () => {
  const resolved = resolveBrowseWorkspaceQuery({
    pivot: "movies",
    filters: JSON.stringify({ field: "year", operator: "equals", value: 2014 }),
    sort: "year:asc,title:desc,title:asc,missing:asc",
    view: "facets"
  }, capabilities);
  assert.deepEqual(resolved, {
    pivot: capabilities.pivots[0],
    expression: { field: "year", operator: "equals", value: 2014 },
    expressionInvalid: false,
    sorts: [{ field: "title", direction: "desc" }],
    presentation: "grid"
  });
  assert.deepEqual(availableFields(capabilities, capabilities.pivots[1]).map(field => field.id), ["title", "artist"]);
  assert.deepEqual(availableSorts(capabilities, capabilities.pivots[1]).map(sort => sort.id), ["title", "releaseDate"]);
  assert.deepEqual(decodeSorts(encodeSorts([{ field: "releaseDate", direction: "desc" }])), [{ field: "releaseDate", direction: "desc" }]);
  assert.deepEqual(resolveBrowseWorkspaceQuery({
    pivot: "movies",
    filters: JSON.stringify({ field: "artist", operator: "in", value: ["person-1"] })
  }, capabilities), {
    pivot: capabilities.pivots[0],
    expression: undefined,
    expressionInvalid: true,
    sorts: [{ field: "title", direction: "asc" }],
    presentation: "grid"
  });
  assert.equal(resolveBrowseWorkspaceQuery({ filters: JSON.stringify({ field: "year", operator: "equals", value: "2014" }) }, capabilities)?.expressionInvalid, true);
});

test("portable chips remove only the selected leaf", () => {
  const expression = { all: [
    { field: "title", operator: "contains", value: "Fargo" },
    { field: "year", operator: "equals", value: 2014 }
  ] };
  const chips = queryChips(expression, capabilities.fields);
  assert.deepEqual(chips.map(chip => chip.label), ["Title contains Fargo", "Year equals 2014"]);
  assert.deepEqual(removeExpressionAtPath(expression, chips[0].path), { all: [{ field: "year", operator: "equals", value: 2014 }] });
});

test("saved-view definitions serialize to the canonical request and restore browse state", () => {
  const request = serializeSavedViewDraft({
    title: "  Unwatched movies  ",
    libraryId: "lib-movies",
    pivot: "movies",
    query: { field: "year", operator: "equals", value: 2014 },
    sort: [{ field: "title", direction: "asc" }],
    presentationFields: ["year", "year"],
    isPinned: true
  });
  assert.deepEqual(request, {
    title: "Unwatched movies",
    libraryId: "lib-movies",
    pivot: "movies",
    query: { field: "year", operator: "equals", value: 2014 },
    sort: [{ field: "title", direction: "asc" }],
    presentation: { fields: ["year"] },
    isPinned: true
  });
  assert.deepEqual(savedViewWorkspaceQuery({
    id: "view-1",
    ownerUserId: "user-1",
    libraryId: "lib-movies",
    libraryName: "Movies",
    title: request.title,
    pivot: request.pivot,
    query: request.query,
    sort: request.sort ?? [],
    presentation: request.presentation ?? {},
    isPinned: true,
    createdAt: "2026-07-16T00:00:00Z",
    updatedAt: "2026-07-16T00:00:00Z"
  }), {
    pivot: "movies",
    query: request.query,
    sort: [{ field: "title", direction: "asc" }],
    presentation: { fields: ["year"] }
  });
  assert.throws(() => serializeSavedViewDraft({ title: " ", libraryId: "lib-movies", pivot: "movies" }), /title is required/);
  assert.throws(() => serializeSavedViewDraft({ title: "View", libraryId: "lib-movies", pivot: "movies", sort: Array(4).fill({ field: "title", direction: "asc" }) }), /at most three/);
});
