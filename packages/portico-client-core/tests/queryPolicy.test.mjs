import assert from "node:assert/strict";
import test from "node:test";

import {
  shouldRetryPorticoQuery,
  viewerQueryKey,
  viewerQueryPrefix,
  viewerQueryScopeKey
} from "../dist/queryPolicy.js";

const viewer = {
  authority: "hosted",
  accountId: "account-1",
  serverId: "server-1",
  profileId: "profile-1",
  authorizationRevision: "policy-1"
};

test("query identity is stable for equivalent parameters and fenced by the complete viewer", () => {
  const first = viewerQueryKey(viewer, "library", { filters: { year: 2026, genre: "Drama" }, page: 1 });
  const reordered = viewerQueryKey(viewer, "library", { page: 1, filters: { genre: "Drama", year: 2026 } });
  assert.deepEqual(first, reordered);
  assert.deepEqual(viewerQueryPrefix(viewer).slice(0, 3), ["portico", "v1", "hosted"]);
  assert.notDeepEqual(first, viewerQueryKey({ ...viewer, profileId: "profile-2" }, "library", { filters: { year: 2026, genre: "Drama" }, page: 1 }));
  assert.notEqual(viewerQueryScopeKey(viewer), viewerQueryScopeKey({ ...viewer, authorizationRevision: "policy-2" }));
});

test("read retries are bounded to transient failures", () => {
  assert.equal(shouldRetryPorticoQuery(0, new Error("offline")), true);
  assert.equal(shouldRetryPorticoQuery(0, { status: 503 }), true);
  assert.equal(shouldRetryPorticoQuery(0, { status: 429 }), true);
  assert.equal(shouldRetryPorticoQuery(0, { status: 400 }), false);
  assert.equal(shouldRetryPorticoQuery(0, { status: 401 }), false);
  assert.equal(shouldRetryPorticoQuery(0, { code: "membership_inactive", status: 503 }), false);
  assert.equal(shouldRetryPorticoQuery(0, { name: "AbortError" }), false);
  assert.equal(shouldRetryPorticoQuery(2, { status: 503 }), false);
});
