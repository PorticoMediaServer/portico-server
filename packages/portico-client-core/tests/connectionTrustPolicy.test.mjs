import assert from "node:assert/strict";
import test from "node:test";

import {
  createSerializedTrustedServerConnectionAdapter,
  createTrustedServerCredentialAdapter
} from "../dist/trustedServerConnection.js";
import { porticoRouteTransport, validatePorticoUrl } from "../dist/urlPolicy.js";

function connection(overrides = {}) {
  return {
    schemaVersion: 2, accountId: "account", serverId: "server", profileId: "profile", serverName: "Home",
    serverPublicKeyFingerprint: "sha256:key", mutationVersion: 1,
    currentRoute: {url: "https://home.example", type: "public_direct", verifiedAt: "2026-08-06T00:00:00Z"},
    session: {serverId: "server", apiBaseUrl: "https://home.example", accessToken: "access-1", refreshToken: "refresh-1"},
    lastSuccessfulConnectionAt: "2026-08-06T00:00:00Z", ...overrides
  };
}

function durableAdapter(initial = connection()) {
  let value = structuredClone(initial);
  let tombstone;
  return {
    persistencePolicy: "saved-session",
    ready: async () => {}, durability: () => "durable",
    list: async () => value ? [structuredClone(value)] : [],
    load: async () => value ? structuredClone(value) : undefined,
    save: async next => { value = structuredClone(next); },
    remove: async () => { value = undefined; }, clearAccount: async () => { value = undefined; },
    compareAndSwap: async (expected, next) => {
      if ((value?.mutationVersion ?? 0) !== expected) return false;
      value = structuredClone(next); return true;
    },
    removeWithTombstone: async next => { value = undefined; tombstone = structuredClone(next); },
    loadRemovalTombstone: async () => tombstone ? structuredClone(tombstone) : undefined,
    snapshot: () => ({value, tombstone})
  };
}

test("purpose-specific URL policy preserves clean LAN HTTP but rejects it for public trust", () => {
  assert.equal(validatePorticoUrl("http://192.168.1.20:32500", "lan-server-route"), "http://192.168.1.20:32500");
  assert.equal(porticoRouteTransport("http://192.168.1.20:32500", "lan-server-route"), "lan-http");
  assert.equal(porticoRouteTransport("https://home.example:32500", "trusted-server-route"), "https");
  assert.throws(() => validatePorticoUrl("http://192.168.1.20:32500", "trusted-server-route"), /HTTPS/);
  assert.throws(() => validatePorticoUrl("http://user@192.168.1.20", "lan-server-route"), /clean/);
  assert.throws(() => validatePorticoUrl("https://home.example/api?token=x", "trusted-server-route"), /clean/);
  assert.equal(validatePorticoUrl("/api/media/file?grant=opaque", "download-grant", {trustedOrigin: "https://home.example"}), "/api/media/file?grant=opaque");
  assert.equal(validatePorticoUrl("/api/media/file?grant=opaque", "download-grant", {trustedOrigin: "http://192.168.1.20:32500"}), "/api/media/file?grant=opaque");
  assert.equal(validatePorticoUrl("http://192.168.1.20:32500/api/media/file", "download-grant", {trustedOrigin: "http://192.168.1.20:32500"}), "http://192.168.1.20:32500/api/media/file");
  assert.throws(() => validatePorticoUrl("https://attacker.example/file", "download-grant", {trustedOrigin: "https://home.example"}), /not bound/);
  assert.throws(() => validatePorticoUrl("http://attacker.example/file", "download-grant", {trustedOrigin: "http://192.168.1.20:32500"}), /unsafe/);
});

test("credential hydration completes before a cold-start trusted record is exposed", async () => {
  let hydrated = false;
  const durable = durableAdapter();
  durable.ready = async () => { await Promise.resolve(); hydrated = true; };
  const credentials = createTrustedServerCredentialAdapter("account", "server", durable);
  const session = await credentials.load();
  assert.equal(hydrated, true);
  assert.equal(session.accessToken, "access-1");
});

test("durable removal tombstone wins an older refresh and remains authoritative after restart", async () => {
  const durable = durableAdapter();
  const firstProcess = createSerializedTrustedServerConnectionAdapter(durable);
  const stale = await firstProcess.load("account", "server");
  await Promise.all([
    firstProcess.save({...stale, session: {...stale.session, accessToken: "rotated"}, mutationVersion: 2}),
    firstProcess.remove("account", "server")
  ]);
  const restarted = createSerializedTrustedServerConnectionAdapter(durable);
  assert.equal(await restarted.load("account", "server"), undefined);
  assert.ok(durable.snapshot().tombstone);
  assert.ok(durable.snapshot().tombstone.mutationVersion >= 3);
});

test("a stale route publication cannot overwrite a newer token family", async () => {
  const durable = durableAdapter();
  const adapter = createSerializedTrustedServerConnectionAdapter(durable);
  const routeBase = await adapter.load("account", "server");
  const tokenBase = await adapter.load("account", "server");
  await adapter.save({...tokenBase, session: {...tokenBase.session, accessToken: "access-2", refreshToken: "refresh-2"}, mutationVersion: 2});
  await assert.rejects(
    adapter.save({...routeBase, currentRoute: {url: "https://new.example", type: "public_direct", verifiedAt: "2026-08-06T01:00:00Z"}, mutationVersion: 2}),
    /publication was blocked/
  );
  const final = await adapter.load("account", "server");
  assert.notEqual(final.currentRoute.url, "https://new.example");
  assert.equal(final.session.accessToken, "access-2");
  assert.equal(final.session.refreshToken, "refresh-2");
});
