import assert from "node:assert/strict";
import test from "node:test";

import {
  dedupePorticoDiscoveryRecords,
  isValidLocalServerRouteURL,
  localServerRouteAddressClass,
  normalizePorticoDiscoveryRecord,
  parsePorticoDiscoveryTXT,
  porticoLANTrustState
} from "../dist/localDiscovery.js";

const fingerprint = "sha256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA";
const observedAt = "2026-07-11T12:00:00.000Z";
const now = new Date("2026-07-11T12:00:30.000Z");

test("accepts canonical localhost, loopback, and RFC1918 route selections without inferring locality", () => {
  assert.deepEqual([
    "http://localhost:32500", "http://127.0.0.1:32500", "http://192.168.1.20:32500",
    "http://10.20.30.40:32500", "http://172.20.1.8:32500"
  ].map(localServerRouteAddressClass), ["localhost", "loopback", "rfc1918-lan", "rfc1918-lan", "rfc1918-lan"]);
  assert.ok(isValidLocalServerRouteURL("http://192.168.1.20:32500"));
  for (const invalid of ["http://8.8.8.8:32500", "https://192.168.1.20:32500", "http://192.168.1.20:32400/path"]) {
    assert.equal(isValidLocalServerRouteURL(invalid), false);
  }
});

function record(overrides = {}) {
  return {
    serviceType: "_portico._tcp.local.",
    instanceName: "EhlerFlix",
    hostname: "ehlerflix.local.",
    port: 32500,
    addresses: ["fe80::1%en0", "192.168.1.20"],
    txt: [
      "txtVersion=1",
      "path=/",
      "scheme=http",
      "serverId=srv_ehlerflix",
      `fingerprint=${fingerprint}`,
      "name=EhlerFlix Test"
    ],
    observedAt,
    ttlSeconds: 120,
    interfaceName: "en0",
    ...overrides
  };
}

test("normalizes a native mDNS record and generates deterministic single-port routes", () => {
  const normalized = normalizePorticoDiscoveryRecord(record(), now);
  assert.equal(normalized.port, 32500);
  assert.equal(normalized.displayName, "EhlerFlix Test");
  assert.equal(normalized.serverId, "srv_ehlerflix");
  assert.equal(normalized.stale, false);
  assert.deepEqual(normalized.routes.map((route) => route.url), [
    "http://192.168.1.20:32500",
    "http://[fe80::1%25en0]:32500",
    "http://ehlerflix.local:32500"
  ]);
  assert.ok(normalized.routes.every((route) => route.serverPublicKeyFingerprint === fingerprint));
});

test("supports an unclaimed local-only server without weakening fingerprint identity", () => {
  const localOnly = normalizePorticoDiscoveryRecord(record({
    txt: {
      txtVersion: "1",
      path: "/",
      scheme: "http",
      serverId: "",
      fingerprint,
      name: "Local only"
    }
  }), now);
  assert.equal(localOnly.serverId, undefined);
  assert.equal(localOnly.serverPublicKeyFingerprint, fingerprint);
  assert.equal(porticoLANTrustState(localOnly), "unverified");
});

test("requires the discovery schema, local HTTP, persistent fingerprint, and LAN addresses", () => {
  assert.throws(() => normalizePorticoDiscoveryRecord(record({ txt: ["scheme=http", `fingerprint=${fingerprint}`] }), now), /TXT version/);
  assert.throws(() => normalizePorticoDiscoveryRecord(record({ txt: ["txtVersion=1", "scheme=https", `fingerprint=${fingerprint}`] }), now), /local HTTP/);
  assert.throws(() => normalizePorticoDiscoveryRecord(record({ txt: ["txtVersion=1", "scheme=http", "fingerprint=nope"] }), now), /fingerprint/);
  assert.throws(() => normalizePorticoDiscoveryRecord(record({ hostname: undefined, addresses: ["8.8.8.8"] }), now), /local network/);
  assert.throws(() => normalizePorticoDiscoveryRecord(record({ hostname: undefined, addresses: ["fdzz::1"] }), now), /local network/);
  assert.throws(() => normalizePorticoDiscoveryRecord(record({ port: 0 }), now), /service port/);
});

test("rejects ambiguous duplicate TXT keys", () => {
  assert.throws(() => parsePorticoDiscoveryTXT(["scheme=http", "SCHEME=https"]), /duplicated/);
});

test("expires stale announcements and removes their connection candidates", () => {
  const stale = normalizePorticoDiscoveryRecord(record({ ttlSeconds: 15 }), new Date("2026-07-11T12:00:16.000Z"));
  assert.equal(stale.stale, true);
  assert.deepEqual(stale.routes, []);
  assert.equal(porticoLANTrustState(stale), "stale");
});

test("deduplicates interface announcements by fingerprint and retains all local routes", () => {
  const first = normalizePorticoDiscoveryRecord(record(), now);
  const second = normalizePorticoDiscoveryRecord(record({
    hostname: undefined,
    addresses: ["10.0.0.8"],
    interfaceName: "en1",
    observedAt: "2026-07-11T12:00:10.000Z"
  }), now);
  const [merged] = dedupePorticoDiscoveryRecords([first, second], now);
  assert.deepEqual(merged.addresses, ["10.0.0.8", "192.168.1.20", "fe80::1%en0"]);
  assert.deepEqual(merged.interfaceNames, ["en0", "en1"]);
  assert.equal(merged.identityConflict, false);
});

test("does not carry expired interface addresses into an otherwise live server record", () => {
  const live = normalizePorticoDiscoveryRecord(record(), now);
  const expired = normalizePorticoDiscoveryRecord(record({
    hostname: undefined,
    addresses: ["10.0.0.9"],
    observedAt: "2026-07-11T11:55:00.000Z",
    ttlSeconds: 15
  }), now);
  const [merged] = dedupePorticoDiscoveryRecords([live, expired], now);
  assert.equal(merged.stale, false);
  assert.ok(!merged.addresses.includes("10.0.0.9"));
});

test("quarantines conflicting server IDs or service endpoints sharing a fingerprint", () => {
  const first = normalizePorticoDiscoveryRecord(record(), now);
  const second = normalizePorticoDiscoveryRecord(record({
    port: 32501,
    txt: ["txtVersion=1", "path=/", "scheme=http", "serverId=srv_other", `fingerprint=${fingerprint}`]
  }), now);
  const [merged] = dedupePorticoDiscoveryRecords([first, second], now);
  assert.equal(merged.identityConflict, true);
  assert.deepEqual(merged.routes, []);
  assert.equal(porticoLANTrustState(merged), "identity-conflict");
});

test("trust requires a matching externally trusted fingerprint and optional server ID", () => {
  const normalized = normalizePorticoDiscoveryRecord(record(), now);
  assert.equal(porticoLANTrustState(normalized), "unverified");
  assert.equal(porticoLANTrustState(normalized, {
    expectedServerId: "srv_ehlerflix",
    expectedServerPublicKeyFingerprint: fingerprint
  }), "trusted");
  assert.equal(porticoLANTrustState(normalized, {
    expectedServerId: "srv_other",
    expectedServerPublicKeyFingerprint: fingerprint
  }), "rejected");
  assert.equal(porticoLANTrustState(normalized, {
    expectedServerPublicKeyFingerprint: "sha256:BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB"
  }), "rejected");
});
