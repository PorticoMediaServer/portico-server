import assert from "node:assert/strict";
import test from "node:test";

import {
  assertViewerScopeMatchesCredentials,
  transitionViewerRuntime,
  ViewerRuntimeTeardownError,
  viewerCacheKey,
  viewerScopeFromAuthMe,
  viewerScopeFromNativeCredentials
} from "../dist/viewerScope.js";

const first = { authority: "hosted", accountId: "account", serverId: "server", profileId: "profile-a", authorizationRevision: "policy-1" };
const second = { ...first, profileId: "profile-b" };

test("authenticated identity and native credentials bind immutable identity while final authorization may advance", () => {
  const identity = {authenticated: true, setupRequired: false, ...first};
  const credentials = {
    tokenType: "Bearer", accessToken: "access", accessExpiresAt: "2026-07-16T21:00:00Z",
    refreshToken: "refresh", refreshExpiresAt: "2026-08-16T21:00:00Z", user: {}, device: {}, ...first
  };
  assert.deepEqual(viewerScopeFromAuthMe(identity), first);
  assert.deepEqual(viewerScopeFromNativeCredentials(credentials), first);
  assert.deepEqual(assertViewerScopeMatchesCredentials(identity, credentials), first);
  assert.throws(() => viewerScopeFromAuthMe({authenticated: false, setupRequired: false}), /authenticated viewer/);
  assert.throws(() => viewerScopeFromAuthMe({...identity, profileId: undefined}), /profileId/);
  assert.deepEqual(
    assertViewerScopeMatchesCredentials({...identity, authorizationRevision: "policy-2"}, credentials),
    {...first, authorizationRevision: "policy-2"}
  );
  for (const [field, value] of [
    ["authority", "local"],
    ["accountId", "other-account"],
    ["serverId", "other-server"],
    ["profileId", "other-profile"]
  ]) {
    assert.throws(() => assertViewerScopeMatchesCredentials({...identity, [field]: value}, credentials), /do not match/);
  }
});

test("viewer cache keys include authority, profile, authorization, and semantic dimensions", () => {
  const a = viewerCacheKey({ ...first, contractRevision: "v1", resource: "home", parameters: { page: 1, filters: { b: 2, a: 1 } } });
  const reordered = viewerCacheKey({ ...first, contractRevision: "v1", resource: "home", parameters: { filters: { a: 1, b: 2 }, page: 1 } });
  assert.equal(a, reordered);
  assert.notEqual(a, viewerCacheKey({ ...second, contractRevision: "v1", resource: "home" }));
  assert.notEqual(a, viewerCacheKey({ ...first, authority: "local", contractRevision: "v1", resource: "home", parameters: { page: 1, filters: { a: 1, b: 2 } } }));
  assert.notEqual(a, viewerCacheKey({ ...first, authorizationRevision: "policy-2", contractRevision: "v1", resource: "home", parameters: { page: 1, filters: { a: 1, b: 2 } } }));
  assert.throws(() => viewerCacheKey({ ...first, contractRevision: "v1", resource: "home", parameters: { date: new Date() } }), /plain JSON/);
  const cyclic = {}; cyclic.self = cyclic;
  assert.throws(() => viewerCacheKey({ ...first, contractRevision: "v1", resource: "home", parameters: cyclic }), /cycles/);
});

function adapter(log, failure) {
  const operation = name => async () => {
    log.push(name);
    if (failure === name) throw new Error("failed");
  };
  return {
    beginTransition: operation("beginTransition"),
    cancelRequests: operation("cancelRequests"),
    stopPlayback: operation("stopPlayback"),
    closeRealtime: operation("closeRealtime"),
    clearOptimisticMutations: operation("clearOptimisticMutations"),
    clearQueryCaches: operation("clearQueryCaches"),
    clearArtworkState: operation("clearArtworkState"),
    closeOverlays: operation("closeOverlays"),
    clearFocusRestoration: operation("clearFocusRestoration"),
    clearProfileLocalState: operation("clearProfileLocalState"),
    activateProfile: operation("activateProfile")
  };
}

test("profile transition fences writes and drains producers before clearing caches", async () => {
  const log = [];
  let release;
  const blocked = adapter(log);
  blocked.cancelRequests = async () => {
    log.push("cancelRequests");
    await new Promise(resolve => { release = resolve; });
  };
  const transition = transitionViewerRuntime(blocked, first, second);
  await new Promise(resolve => setImmediate(resolve));
  assert.equal(log[0], "beginTransition");
  assert.equal(log.includes("clearQueryCaches"), false);
  release();
  await transition;
  assert.ok(log.indexOf("clearQueryCaches") > log.indexOf("cancelRequests"));
  assert.equal(log.at(-1), "activateProfile");
});

test("profile transition fails closed while still attempting every clearing operation", async () => {
  const log = [];
  await assert.rejects(() => transitionViewerRuntime(adapter(log, "clearQueryCaches"), first, second), error => {
    assert.ok(error instanceof ViewerRuntimeTeardownError);
    assert.deepEqual(error.failures.map(failure => failure.operation), ["clearQueryCaches"]);
    return true;
  });
  assert.equal(log.includes("activateProfile"), false);
  assert.equal(new Set(log).size, 10);
});

test("ordinary same-scope selection is a no-op but revocation always tears down", async () => {
  const ordinary = [];
  await transitionViewerRuntime(adapter(ordinary), first, { ...first });
  assert.deepEqual(ordinary, []);
  const revoked = [];
  await transitionViewerRuntime(adapter(revoked), first, { ...first }, "profile-revoked");
  assert.equal(revoked[0], "beginTransition");
  assert.equal(revoked.includes("activateProfile"), false);
});
