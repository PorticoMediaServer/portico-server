import assert from "node:assert/strict";
import fs from "node:fs";
import test from "node:test";
import {
  HostedCompatibilityError, PORTICO_API_VERSION, PORTICO_FOUNDATION_COMPATIBILITY,
  PorticoCompatibilityError, assertHostedServicesCompatibility, assertProductContractCompatibility, assertServerAPICompatibility,
  createHostedServicesClient, createPorticoClient, evaluatePorticoCompatibility, supportsServerCapability
} from "../dist/index.js";

const hostedFixture = JSON.parse(fs.readFileSync(new URL("../fixtures/hosted-api-v1-conformance.json", import.meta.url), "utf8"));
const canonicalHostedFixture = JSON.parse(fs.readFileSync(new URL("../../../api/openapi/hosted/hosted-api-v1-conformance.json", import.meta.url), "utf8"));

function envelope(overrides = {}) {
  return {
    envelopeRevision: PORTICO_FOUNDATION_COMPATIBILITY.envelopeRevision,
    supportedClientProtocol: { ...PORTICO_FOUNDATION_COMPATIBILITY.supportedClientProtocol },
    apiContract: { ...PORTICO_FOUNDATION_COMPATIBILITY.apiContract },
    build: { ...PORTICO_FOUNDATION_COMPATIBILITY.build },
    semanticRevisions: { ...PORTICO_FOUNDATION_COMPATIBILITY.semanticRevisions },
    capabilities: [
      { id: "library.canonical-browse", revision: 1, state: "available", requiredSemantics: ["product"] },
      { id: "remote-access.direct", revision: 1, state: "available", requiredSemantics: ["viewerProfileAuthority"] }
    ],
    requiredSemantics: Object.keys(PORTICO_FOUNDATION_COMPATIBILITY.semanticRevisions),
    forwardCompatibility: { ...PORTICO_FOUNDATION_COMPATIBILITY.forwardCompatibility },
    ...overrides
  };
}
function system(compatibility = envelope()) { return { name: "Portico", status: "ok", apiVersion: PORTICO_API_VERSION, compatibility }; }
function product(compatibility = envelope()) { return { apiVersion: PORTICO_API_VERSION, serverCapabilities: compatibility.capabilities.filter(({state}) => state === "available").map(({id}) => id), compatibility }; }
function releaseBuild(version = "1.0.0", commit = "1".repeat(40)) { return { version, buildNumber: "1", channel: "staging", commit, timestamp: "2026-08-17T00:00:00Z" }; }

test("older compatible client accepts a newer build and preserves unknown optional capabilities", () => {
  const compatibility = assertServerAPICompatibility(system(envelope({ capabilities: [
		{ id: "remote-access.direct", revision: 1, state: "available", requiredSemantics: ["viewerProfileAuthority"] },
		{ id: "future.optional", revision: 1, state: "available", requiredSemantics: ["product"] }
  ] })));
  assert.equal(compatibility.build.version, PORTICO_FOUNDATION_COMPATIBILITY.build.version);
  assert.equal(supportsServerCapability(compatibility, "future.optional"), true);
});

test("newer compatible client accepts an older build when the protocol and required semantics overlap", () => {
	assert.doesNotThrow(() => assertServerAPICompatibility(system(envelope({ build: releaseBuild("0.9.0") }))));
});

test("unsupported minimum protocol fails before feature use", () => {
  assert.throws(() => assertServerAPICompatibility(system(envelope({ supportedClientProtocol: { minimum: 2, maximum: 3 } }))), (error) => error instanceof PorticoCompatibilityError && error.code === "unsupported_client_protocol");
});

test("unknown required semantics reject actionably", () => {
  assert.throws(() => assertServerAPICompatibility(system(envelope({ requiredSemantics: ["future.authorization"] }))), (error) => error instanceof PorticoCompatibilityError && error.code === "required_semantic_unknown" && /future.authorization/.test(error.message));
});

test("known required semantic revision mismatch rejects", () => {
  const semanticRevisions = { ...PORTICO_FOUNDATION_COMPATIBILITY.semanticRevisions, playback: 2 };
  assert.throws(() => assertServerAPICompatibility(system(envelope({ semanticRevisions, requiredSemantics: ["playback"] }))), (error) => error instanceof PorticoCompatibilityError && error.code === "required_semantic_incompatible");
});

test("valid API digest mismatch is retained as an additive-overlap diagnostic", () => {
  const result = assertServerAPICompatibility(system(envelope({ apiContract: { ...PORTICO_FOUNDATION_COMPATIBILITY.apiContract, digest: "0".repeat(64) } })));
  assert.deepEqual(result.diagnostics.map(({code}) => code), ["api_contract_digest_mismatch"]);
});

test("partial upgrade can never broaden authorization", () => {
  assert.throws(() => assertServerAPICompatibility(system(envelope({ forwardCompatibility: { ...PORTICO_FOUNDATION_COMPATIBILITY.forwardCompatibility, authorizationOnPartialUpgrade: "allow" } }))), (error) => error instanceof PorticoCompatibilityError && error.code === "unsafe_forward_compatibility_policy");
});

test("system and Product Contract must come from the same build", () => {
  const status = system();
	const contractEnvelope = envelope({ build: releaseBuild("1.0.0", "2".repeat(40)) });
  assert.throws(() => evaluatePorticoCompatibility(status, product(contractEnvelope)), (error) => error instanceof PorticoCompatibilityError && error.code === "system_product_contract_mismatch");
});

test("Hosted uses the same Foundation envelope and typed failures", () => {
  assert.equal(assertHostedServicesCompatibility(system()).clientProtocol, PORTICO_FOUNDATION_COMPATIBILITY.supportedClientProtocol.maximum);
  assert.throws(() => assertHostedServicesCompatibility(system(envelope({ supportedClientProtocol: { minimum: 4, maximum: 4 } }))), (error) => error instanceof HostedCompatibilityError && error.code === "unsupported_client_protocol");
});

test("packaged and Cloud Hosted compatibility fixtures remain byte-equivalent in meaning", () => {
  assert.deepEqual(hostedFixture, canonicalHostedFixture);
  assert.equal(assertHostedServicesCompatibility(hostedFixture.system).apiVersion, "v1");
});

test("malformed and incomplete envelopes fail closed without silently inventing capabilities", () => {
  for (const compatibility of [null, {}, envelope({ capabilities: undefined }), envelope({ requiredSemantics: [] }), envelope({ semanticRevisions: {} })]) {
    assert.throws(() => assertServerAPICompatibility(system(compatibility)), (error) => error instanceof PorticoCompatibilityError && error.code === "invalid_compatibility_envelope");
  }
});

test("build identity accepts only the exact development tuple or a complete release stamp", () => {
	assert.doesNotThrow(() => assertServerAPICompatibility(system(envelope())));
	assert.doesNotThrow(() => assertServerAPICompatibility(system(envelope({ build: releaseBuild() }))));
	for (const build of [
		{ ...PORTICO_FOUNDATION_COMPATIBILITY.build, version: "0.0.0-other" },
		{ ...releaseBuild(), buildNumber: "0" },
		{ ...releaseBuild(), channel: "development" },
		{ ...releaseBuild(), commit: "short" },
		{ ...releaseBuild(), timestamp: "not-rfc3339" }
	]) assert.throws(() => assertServerAPICompatibility(system(envelope({ build }))), PorticoCompatibilityError);
});

test("every API token other than the sole current v1 contract is rejected", () => {
  for (const apiVersion of ["v2", "development-revision", ""]) {
    assert.throws(() => assertServerAPICompatibility({ apiVersion, compatibility: envelope() }), (error) => error instanceof PorticoCompatibilityError && error.code === "invalid_compatibility_envelope");
    assert.throws(() => assertHostedServicesCompatibility({ ...system(), apiVersion }), (error) => error instanceof HostedCompatibilityError && error.code === "invalid_compatibility_envelope");
  }
});

test("Product Contract requires exact available capability projection", () => {
  assert.doesNotThrow(() => assertProductContractCompatibility(product()));
  assert.throws(() => assertProductContractCompatibility({ ...product(), serverCapabilities: ["library.canonical-browse"] }), (error) => error instanceof PorticoCompatibilityError && error.code === "system_product_contract_mismatch");
});

test("PorticoClient compatibility bootstrap checks public System then authenticated Product Contract", async () => {
  const calls = [];
  const client = createPorticoClient({ apiBaseUrl: "https://server.example", transport: { fetch: async (input) => {
    const url = String(input); calls.push(url);
    if (url.endsWith("/api/system")) return new Response(JSON.stringify(system()), { headers: { "Content-Type": "application/json" } });
    return new Response(JSON.stringify(product()), { headers: { "Content-Type": "application/json" } });
  } } });
  await client.checkCompatibility();
  assert.deepEqual(calls, ["https://server.example/api/system", "https://server.example/api/product-contract"]);
});

test("Hosted rejects a reverse proxy or unhealthy identity before auth", () => {
  for (const changed of [{ name: "Reverse Proxy" }, { status: "degraded" }]) {
    assert.throws(() => assertHostedServicesCompatibility({ ...hostedFixture.system, ...changed }), (error) => error instanceof HostedCompatibilityError && error.code === "invalid_compatibility_envelope");
  }
});

test("Hosted compatibility bootstrap is single-flight and cached before ordinary reads", async () => {
  const calls = []; let systemAttempts = 0;
  const hosted = createHostedServicesClient({ hostedApiBaseUrl: "https://hosted.example", transport: { fetch: async (input) => {
    const url = String(input); calls.push(url);
    if (url.endsWith("/api/system")) { systemAttempts++; return new Response(JSON.stringify(hostedFixture.system), { headers: { "Content-Type": "application/json" } }); }
    if (url.endsWith("/api/auth/me")) return new Response(JSON.stringify({ authenticated: false }), { headers: { "Content-Type": "application/json" } });
    return new Response(JSON.stringify({ items: [], pageInfo: { nextCursor: null, hasMore: false } }), { headers: { "Content-Type": "application/json" } });
  } } });
  await Promise.all([hosted.me(), hosted.servers()]);
  assert.equal(systemAttempts, 1);
  assert.equal(calls[0], "https://hosted.example/api/system");
});

test("Hosted retries safe compatibility and read requests after transient failures", async () => {
  let systemAttempts = 0; let serverAttempts = 0;
  const hosted = createHostedServicesClient({ hostedApiBaseUrl: "https://hosted.example", transport: { fetch: async (input) => {
    const url = String(input);
    if (url.endsWith("/api/system")) { if (++systemAttempts === 1) throw new TypeError("temporary network failure"); return new Response(JSON.stringify(hostedFixture.system), { headers: { "Content-Type": "application/json" } }); }
    if (++serverAttempts === 1) return new Response(JSON.stringify({ code: "hosted_unavailable" }), { status: 503, headers: { "Content-Type": "application/problem+json" } });
    return new Response(JSON.stringify({ items: [], pageInfo: { nextCursor: null, hasMore: false } }), { headers: { "Content-Type": "application/json" } });
  } } });
  await hosted.servers();
  assert.equal(systemAttempts, 2); assert.equal(serverAttempts, 2);
});

test("Hosted never automatically replays a mutation after an ambiguous failure", async () => {
  let attempts = 0;
  const hosted = createHostedServicesClient({ hostedApiBaseUrl: "https://hosted.example", transport: { fetch: async (input) => {
    if (String(input).endsWith("/api/system")) return new Response(JSON.stringify(hostedFixture.system), { headers: { "Content-Type": "application/json" } });
    attempts++; throw new TypeError("ambiguous login failure");
  } } });
  await assert.rejects(hosted.login({ login: "owner@example.test", password: "Password123!", installationId: "browser-installation-0001" }), /ambiguous login failure/);
  assert.equal(attempts, 1);
});

test("Hosted blocks authentication when compatibility negotiation fails", async () => {
  const calls = [];
  const hosted = createHostedServicesClient({ hostedApiBaseUrl: "https://hosted.example", transport: { fetch: async (input) => {
    calls.push(String(input)); return new Response(JSON.stringify({ ...hostedFixture.system, compatibility: { ...hostedFixture.system.compatibility, requiredSemantics: ["future.authorization"] } }), { headers: { "Content-Type": "application/json" } });
  } } });
  await assert.rejects(hosted.login({ login: "owner@example.test", password: "secret", installationId: "browser-installation-0001" }), (error) => error instanceof HostedCompatibilityError && error.code === "required_semantic_unknown");
  assert.deepEqual(calls, ["https://hosted.example/api/system"]);
});
