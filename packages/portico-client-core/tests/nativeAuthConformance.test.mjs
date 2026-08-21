import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import test from "node:test";
import {
  ApiError,
  connectHostedServer,
  createHostedServicesClient,
  createMemorySessionStore,
  createPorticoClient
} from "../dist/index.js";
import {
  createAttachmentMethods,
  createAttachmentRuntime,
  testServerIdentity,
  testServerPublicKeyFingerprint
} from "./helpers/porticoAttachment.mjs";

const fixture = JSON.parse(fs.readFileSync(
  path.resolve(import.meta.dirname, "../fixtures/native-auth-connection.v1.json"),
  "utf8"
));

const hostedSystemFixture = Object.freeze(JSON.parse(fs.readFileSync(new URL("../fixtures/hosted-api-v1-conformance.json", import.meta.url), "utf8")).system);

function jsonResponse(body, init = {}) {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
    ...init
  });
}

function credentials(accessToken, refreshToken, overrides = {}) {
  return {
    tokenType: "Bearer",
    accessToken,
    accessExpiresAt: "2026-07-11T01:00:00Z",
    refreshToken,
    refreshExpiresAt: "2026-08-11T00:00:00Z",
    user: { id: "usr_1" },
    device: { id: "dev_1" },
    authority: "hosted",
    accountId: "account-1",
    serverId: "server-1",
    profileId: "profile-1",
    authorizationRevision: "policy-1",
    ...overrides
  };
}

function hostedProfileBinding(serverId, installationId = "installation-test") {
  return {
    clientIdentity: {installationId, deviceName: "Test Device", app: "Portico Test", platform: "test"},
    selectionEnvelope: {accountId: "account-1", serverId, profileId: "profile-1", installationId}
  };
}

function routeDocument(serverId, routes) {
  return {
    documentVersion: 1,
    audience: "portico-media-server",
    signatureAlgorithm: "ed25519",
    signatureKeyId: "fixture-key",
    signature: Buffer.alloc(64, 0x41).toString("base64url"),
    serverId,
    serverName: serverId,
    assignedHostname: `${serverId}.direct.getportico.tv`,
    issuedAt: "2026-07-11T00:00:00Z",
    expiresAt: "2026-07-11T00:05:00Z",
    ...testServerIdentity(),
    authModes: ["portico"],
    certificate: { status: "valid" },
    membership: { role: "owner" },
    routes
  };
}

const routeRuntime = {
  ...createAttachmentRuntime("2026-07-11T00:01:00Z"),
  decodeBase64: (value) => Uint8Array.from(Buffer.from(value, "base64")),
  encodeText: (value) => Uint8Array.from(Buffer.from(value, "utf8")),
  verifyEd25519: () => true,
  now: () => new Date("2026-07-11T00:01:00Z")
};
const trustedKeys = { "fixture-key": Buffer.alloc(32).toString("base64") };

function attachmentLocalClient(store, serverId, issued, overrides = {}) {
  return {
    ...createAttachmentMethods({sessionStore: store, serverId, credentials: issued, now: "2026-07-11T00:01:00Z"}),
    checkServerCompatibility: async () => ({ apiVersion: "v1" }),
    checkCompatibility: async () => ({ apiVersion: "v1" }),
    ...overrides
  };
}

test("published native auth fixture is complete and security-preserving", () => {
  assert.equal(fixture.schemaVersion, 1);
  assert.deepEqual(Object.keys(fixture.flows).sort(), [
    "hostedAccountSession", "localNativeSession", "porticoServerSession", "tvPairing"
  ]);
  assert.equal(fixture.refreshPolicy.singleFlight, true);
  assert.equal(fixture.refreshPolicy.maxAutomaticRetries, 1);
  assert.equal(fixture.switchPolicy.restorePreviousOnFailure, true);
  assert.equal(fixture.switchPolicy.runtimeFenceBeforeCredentialPublication, true);
  assert.equal(fixture.switchPolicy.activeCredentialPublicationFailure, "rollback-fatal");
  assert.equal(fixture.switchPolicy.atomicDurableFailure, "memory-only-with-warning");
  assert.equal(fixture.switchPolicy.uncertainDurableFailure, "rollback-or-fail-closed");
  assert.equal(fixture.switchPolicy.latestChoiceWins, true);
  assert.equal(fixture.switchPolicy.authorizationRevisionSource, "final-auth-me");
  assert.ok(fixture.securityInvariants.includes("route-health-must-match-server-id-and-fingerprint"));
  assert.ok(fixture.securityInvariants.includes("hosted-bootstrap-and-server-credentials-use-authenticated-encryption-on-every-route"));
  assert.ok(fixture.securityInvariants.includes("hosted-bootstrap-is-never-installed-in-the-selected-server-session-store"));
  assert.ok(fixture.securityInvariants.includes("rotated-refresh-token-is-durable-before-request-retry"));
  assert.ok(fixture.securityInvariants.includes("final-me-must-match-authority-account-server-profile"));
  assert.ok(fixture.securityInvariants.includes("credential-cleanup-uncertainty-is-typed-and-fail-closed"));
  assert.ok(fixture.securityInvariants.includes("old-runtime-is-fenced-before-candidate-credential-publication"));
});

test("local native creation is durable and restores after an app restart", async () => {
  let durable;
  let saveCalls = 0;
  const adapter = {
    load: async () => durable,
    save: async (session) => { saveCalls++; durable = structuredClone(session); },
    clear: async () => { durable = undefined; }
  };
  const first = createPorticoClient({
    apiBaseUrl: "https://local.example",
    credentialAdapter: adapter,
    transport: { fetch: async () => jsonResponse(credentials("local-access", "local-refresh"), { status: 201 }) }
  });
  await first.createNativeSession({
    login: "owner", password: "secret", installationId: "install-1",
    deviceName: "iPhone", app: "Portico", platform: "ios"
  });
  assert.equal(saveCalls, 1);
  assert.equal(durable.apiBaseUrl, "https://local.example");
  assert.equal(durable.accessToken, "local-access");
  assert.equal(durable.bootstrapAccessToken, undefined);

  const restoredCalls = [];
  const restarted = createPorticoClient({
    credentialAdapter: adapter,
    transport: { fetch: async (input, init) => {
      restoredCalls.push({ url: String(input), authorization: init.headers.Authorization });
      return jsonResponse({ authenticated: true, user: { id: "usr_1" } });
    } }
  });
  await restarted.me();
  assert.deepEqual(restoredCalls, [{
    url: "https://local.example/api/auth/me",
    authorization: "Bearer local-access"
  }]);
});

test("TV grant redemption persists credentials before returning to the app", async () => {
  let saved;
  const client = createPorticoClient({
    apiBaseUrl: "https://tv-server.example",
    credentialAdapter: {
      save: async (session) => { saved = structuredClone(session); },
      clear: async () => {}
    },
    transport: { fetch: async () => jsonResponse(credentials("tv-access", "tv-refresh")) }
  });
  await client.redeemTVSetupGrant({ setupSessionId: "setup-1", grantSecret: "grant-secret" });
  assert.equal(saved.apiBaseUrl, "https://tv-server.example");
  assert.equal(saved.accessToken, "tv-access");
  assert.equal(saved.refreshToken, "tv-refresh");
});

test("one-time Portico bootstrap never falls back to a Hosted server refresh", async () => {
  const store = createMemorySessionStore({
    serverId: "srv_1",
    apiBaseUrl: "https://srv-1.direct.getportico.tv",
    bootstrapAccessToken: "portico-old",
  });
  const calls = [];
  const client = createPorticoClient({
    sessionStore: store,
    hostedApiBaseUrl: "https://hosted.example",
    transport: { fetch: async (input, init) => {
      const url = String(input);
      calls.push({ url, authorization: init.headers.Authorization, csrf: init.headers["X-Portico-CSRF"] });
      return jsonResponse({ code: "token_expired", detail: "Expired" }, { status: 401 });
    } }
  });

  await assert.rejects(() => client.system(), (error) => error?.code === "token_expired");
  assert.deepEqual(calls.map((call) => call.url), [
    "https://srv-1.direct.getportico.tv/api/system"
  ]);
  assert.equal(calls[0].authorization, "Bearer portico-old");
});

test("hosted account tokens can switch without rebuilding the client and scoped secrets stay in bodies", async () => {
  let activeAccountToken = "account-a";
  const calls = [];
  const hosted = createHostedServicesClient({
    hostedApiBaseUrl: "https://hosted.example",
    accessToken: () => activeAccountToken,
    transport: { fetch: async (input, init) => {
      calls.push({ url: String(input), authorization: init.headers.Authorization, body: init.body });
      if (String(input).endsWith("/api/system")) {
        return jsonResponse(hostedSystemFixture);
      }
      if (String(input).endsWith("/api/auth/sessions")) return jsonResponse(credentials("hosted-access", "hosted-refresh"), { status: 201 });
      if (String(input).endsWith("/refresh")) return jsonResponse(credentials("hosted-access-2", "hosted-refresh-2"));
      return jsonResponse({ authenticated: true, ok: true, user: { id: activeAccountToken } });
    } }
  });

  await hosted.me();
  activeAccountToken = "account-b";
  await hosted.me();
  await hosted.createNativeSession({ login: "b@example.test", password: "secret", deviceName: "iPhone", devicePlatform: "ios" });
  await hosted.refreshNativeSession({ refreshToken: "hosted-refresh", rotationKey: "A".repeat(43) });
  await hosted.revokeNativeSession("hosted-refresh-2");
  assert.equal(calls[0].authorization, undefined, "compatibility probe must not disclose account credentials");
  assert.deepEqual(calls.slice(1, 3).map((call) => call.authorization), ["Bearer account-a", "Bearer account-b"]);
  assert.equal(calls.every((call) => !call.url.includes("hosted-refresh")), true);
	assert.equal(calls.some((call) => call.url.includes("/portico-sessions/")), false);
});

test("hosted route failover verifies identity and commits a selected server atomically", async () => {
  const previous = {
    serverId: "srv_previous", apiBaseUrl: "https://previous.direct.getportico.tv",
    bootstrapAccessToken: "previous-access", refreshToken: "previous-refresh"
  };
  const store = createMemorySessionStore(previous);
  const saved = [];
  const probeCalls = [];
  const server = { ...testServerIdentity(), id: "srv_next", name: "Next", preferredAuthMode: "portico" };
  const document = routeDocument("srv_next", [
    { type: "lan_ip_encoded", url: "https://10-0-0-8.direct.getportico.tv:32500", quality: "reported" },
    { type: "public_direct", url: "https://srv-next.direct.getportico.tv:32500", quality: "reachable" }
  ]);
  await connectHostedServer(server, {
    ...hostedProfileBinding(server.id),
    hostedClient: {
      routes: async () => document,
      porticoSession: async () => credentials("next-access", "next-refresh"),
      reportRouteFailure: async () => ({ ok: true, matched: true })
    },
    localClient: attachmentLocalClient(store, "srv_next", credentials("server-access", "server-refresh", {serverId: "srv_next"}), {
      checkServerCompatibility: async () => {
        const provisional = store.get();
        store.set({
          ...provisional,
          bootstrapAccessToken: "next-access-rotated",
          refreshToken: "next-refresh-rotated"
        });
        return { apiVersion: "v1" };
      },
      me: async () => ({
        authenticated: true,
        authority: "hosted",
        accountId: "account-1",
        serverId: "srv_next",
        profileId: "profile-1",
        authorizationRevision: "policy-2",
        user: { id: "usr_1" }
      }),
      checkProductContractCompatibility: async () => ({ apiVersion: "v1" })
    }),
    sessionStore: store,
    credentialAdapter: {
      save: async (session) => { saved.push(structuredClone(session)); },
      clear: async () => { throw new Error("must not clear a successful switch"); }
    },
    runtime: routeRuntime,
    routeProbeFetch: async (input) => {
      const url = String(input);
      probeCalls.push(url);
      if (url.includes("10-0-0-8")) throw new TypeError("LAN unavailable");
      return jsonResponse({
        serverId: "srv_next", serverPublicKeyFingerprint: testServerPublicKeyFingerprint, remoteAccessEnabled: true
      });
    },
    retryDelaysMs: [],
    trustedHostedDocumentKeys: trustedKeys
  });
  assert.equal(probeCalls.length, 2);
  assert.equal(store.get().serverId, "srv_next");
  assert.equal(store.get().bootstrapAccessToken, undefined);
  assert.equal(store.get().accessToken, "server-access");
  assert.equal(saved.length, 1);
  assert.equal(saved[0].serverId, "srv_next");
  assert.equal(saved[0].refreshToken, "server-refresh");
});

test("Hosted attachment preserves structured ApiError recovery metadata", async () => {
  const server = { ...testServerIdentity(), id: "srv_metadata", name: "Metadata", preferredAuthMode: "portico" };
  const store = createMemorySessionStore();
  const structured = new ApiError(429, "rate_limited", "Retry later", { operation: "attach" }, {
    messageId: "problem.rate-limited",
    requestId: "request-attach-1",
    retryAfter: "7",
    retryAfterMs: 7000,
    retryAt: "2026-07-11T00:01:07.000Z"
  });
  await assert.rejects(connectHostedServer(server, {
    ...hostedProfileBinding(server.id),
    hostedClient: {
      routes: async () => routeDocument(server.id, [
        { type: "public_direct", url: "https://srv-metadata.direct.getportico.tv:32500", quality: "reachable" }
      ]),
      porticoSession: async () => { throw structured; },
      reportRouteFailure: async () => ({ ok: true, matched: true })
    },
    localClient: attachmentLocalClient(store, server.id, credentials("unused", "unused", {serverId: server.id}), {
      checkServerCompatibility: async () => ({apiVersion: "v1"}),
      me: async () => ({authenticated: true}),
      checkProductContractCompatibility: async () => ({apiVersion: "v1"})
    }),
    sessionStore: store,
    runtime: routeRuntime,
    routeProbeFetch: async () => jsonResponse({
      serverId: server.id,
      serverPublicKeyFingerprint: testServerPublicKeyFingerprint,
      remoteAccessEnabled: true
    }),
    retryDelaysMs: [],
    trustedHostedDocumentKeys: trustedKeys
  }), (error) => {
    assert.equal(error, structured);
    assert.equal(error.requestId, "request-attach-1");
    assert.equal(error.retryAfterMs, 7000);
    assert.equal(error.messageId, "problem.rate-limited");
    return true;
  });
});

test("final /me rejects every wrong immutable candidate identity field before publication", async (t) => {
  const expected = {
    authority: "hosted",
    accountId: "account-1",
    serverId: "srv_target",
    profileId: "profile-1"
  };
  const wrongValues = {
    authority: "local",
    accountId: "account-attacker",
    serverId: "srv_attacker",
    profileId: "profile-attacker"
  };

  for (const [field, wrongValue] of Object.entries(wrongValues)) {
    await t.test(field, async () => {
      const previous = {
        serverId: "srv_previous",
        apiBaseUrl: "https://previous.direct.getportico.tv",
        accessToken: "previous-access",
        refreshToken: "previous-refresh"
      };
      const store = createMemorySessionStore(previous);
      const durableSaves = [];
      const revoked = [];
      const issued = credentials("server-access", "server-refresh", {serverId: expected.serverId});
      const finalIdentity = {
        authenticated: true,
        ...expected,
        [field]: wrongValue,
        authorizationRevision: "policy-2",
        user: { id: "usr_1" }
      };

      await assert.rejects(connectHostedServer(
        { ...testServerIdentity(), id: expected.serverId, name: "Target", preferredAuthMode: "portico" },
        {
          ...hostedProfileBinding(expected.serverId),
          hostedClient: {
            routes: async () => routeDocument(expected.serverId, [
              { type: "public_direct", url: "https://target.direct.getportico.tv:32500", quality: "reachable" }
            ]),
            porticoSession: async () => ({ accessToken: "bootstrap-access", accessExpiresAt: "2026-07-11T00:03:00Z" }),
            reportRouteFailure: async () => ({ ok: true, matched: true })
          },
          localClient: attachmentLocalClient(store, expected.serverId, issued, {
            checkServerCompatibility: async () => ({ apiVersion: "v1" }),
            acceptPorticoSessionCredentials: async () => {
              store.set({
                ...store.get(),
                bootstrapAccessToken: undefined,
                accessToken: issued.accessToken,
                refreshToken: issued.refreshToken
              });
              return issued;
            },
            me: async () => finalIdentity,
            checkProductContractCompatibility: async () => { throw new Error("must not run after identity mismatch"); },
            revokeNativeSession: async token => { revoked.push(token); return { ok: true }; }
          }),
          sessionStore: store,
          credentialAdapter: {
            save: async session => { durableSaves.push(structuredClone(session)); },
            clear: async () => { throw new Error("the previous session must be restored"); }
          },
          runtime: routeRuntime,
          routeProbeFetch: async () => jsonResponse({
            serverId: expected.serverId,
            serverPublicKeyFingerprint: testServerPublicKeyFingerprint,
            remoteAccessEnabled: true
          }),
          retryDelaysMs: [],
          trustedHostedDocumentKeys: trustedKeys
        }
      ), /viewer scope do not match|expected authority, account, server, and profile/);

      assert.deepEqual(store.get(), previous);
      assert.deepEqual(durableSaves, [previous]);
      assert.deepEqual(revoked, [issued.refreshToken]);
    });
  }
});

test("a hung best-effort revoke cannot block rollback after final identity rejection", async () => {
  const previous = {
    serverId: "srv_previous",
    apiBaseUrl: "https://previous.direct.getportico.tv",
    accessToken: "previous-access",
    refreshToken: "previous-refresh"
  };
  const store = createMemorySessionStore(previous);
  const restored = [];
  let revokeStarted = false;
  const issued = credentials("candidate-access", "candidate-refresh", {serverId: "srv_candidate"});

  const attempt = connectHostedServer(
    {...testServerIdentity(), id: "srv_candidate", name: "Candidate", preferredAuthMode: "portico"},
    {
      ...hostedProfileBinding("srv_candidate"),
      hostedClient: {
        routes: async () => routeDocument("srv_candidate", [
          {type: "public_direct", url: "https://candidate.direct.getportico.tv:32500", quality: "reachable"}
        ]),
        porticoSession: async () => ({accessToken: "bootstrap-access", accessExpiresAt: "2026-07-11T00:03:00Z"}),
        reportRouteFailure: async () => ({ok: true, matched: true})
      },
      localClient: attachmentLocalClient(store, "srv_candidate", issued, {
        checkServerCompatibility: async () => ({apiVersion: "v1"}),
        acceptPorticoSessionCredentials: async () => {
          store.set({...store.get(), bootstrapAccessToken: undefined, accessToken: issued.accessToken, refreshToken: issued.refreshToken});
          return issued;
        },
        me: async () => ({
          authenticated: true,
          authority: "hosted",
          accountId: "wrong-account",
          serverId: "srv_candidate",
          profileId: "profile-1",
          authorizationRevision: "policy-2",
          user: {id: "usr_1"}
        }),
        checkProductContractCompatibility: async () => { throw new Error("must not run"); },
        revokeNativeSession: async () => {
          revokeStarted = true;
          await new Promise(() => {});
          return {ok: true};
        }
      }),
      sessionStore: store,
      credentialAdapter: {
        save: async session => { restored.push(structuredClone(session)); },
        clear: async () => { throw new Error("the previous session must be restored"); }
      },
      runtime: routeRuntime,
      routeProbeFetch: async () => jsonResponse({
        serverId: "srv_candidate",
        serverPublicKeyFingerprint: testServerPublicKeyFingerprint,
        remoteAccessEnabled: true
      }),
      retryDelaysMs: [],
      trustedHostedDocumentKeys: trustedKeys
    }
  );

  const outcome = await Promise.race([
    attempt.then(
      () => ({kind: "resolved"}),
      error => ({kind: "rejected", error})
    ),
    new Promise(resolve => setTimeout(() => resolve({kind: "timeout"}), 100))
  ]);

  assert.equal(outcome.kind, "rejected", "local rollback must not wait for remote revocation");
  assert.match(outcome.error.message, /viewer scope do not match|expected authority, account, server, and profile/);
  assert.equal(revokeStarted, true);
  assert.deepEqual(store.get(), previous);
  assert.deepEqual(restored, [previous]);
});

test("failed server bootstrap restores the previous account/server selection", async () => {
  const previous = {
    serverId: "srv_previous", apiBaseUrl: "https://previous.direct.getportico.tv",
    bootstrapAccessToken: "previous-access", refreshToken: "previous-refresh"
  };
  const store = createMemorySessionStore(previous);
  const saved = [];
  await assert.rejects(connectHostedServer(
    { ...testServerIdentity(), id: "srv_denied", name: "Denied", preferredAuthMode: "portico" },
    {
      ...hostedProfileBinding("srv_denied"),
      hostedClient: {
        routes: async () => routeDocument("srv_denied", [
          { type: "public_direct", url: "https://denied.direct.getportico.tv:32500", quality: "reachable" }
        ]),
        porticoSession: async () => credentials("denied-access", "denied-refresh"),
        reportRouteFailure: async () => ({ ok: true, matched: true })
      },
      localClient: attachmentLocalClient(store, "srv_denied", credentials("server-access", "server-refresh", {serverId: "srv_denied"}), {
        checkServerCompatibility: async () => ({ apiVersion: "v1" }),
        me: async () => ({ authenticated: false }),
        checkProductContractCompatibility: async () => { throw new Error("must not run"); }
      }),
      sessionStore: store,
      credentialAdapter: {
        save: async (session) => { saved.push(structuredClone(session)); },
        clear: async () => { throw new Error("must restore, not clear"); }
      },
      runtime: routeRuntime,
      routeProbeFetch: async () => jsonResponse({
        serverId: "srv_denied", serverPublicKeyFingerprint: testServerPublicKeyFingerprint, remoteAccessEnabled: true
      }),
      retryDelaysMs: [],
      trustedHostedDocumentKeys: trustedKeys
    }
  ), /does not have access/);
  assert.deepEqual(store.get(), previous);
  assert.deepEqual(saved, [previous]);
});

test("route fingerprint mismatch fails closed before credentials are issued", async () => {
  const previous = {
    serverId: "srv_previous", apiBaseUrl: "https://previous.direct.getportico.tv",
    bootstrapAccessToken: "previous-access", refreshToken: "previous-refresh"
  };
  const store = createMemorySessionStore(previous);
  let sessionIssueCalls = 0;
  await assert.rejects(connectHostedServer(
    { ...testServerIdentity(), id: "srv_target", name: "Target", preferredAuthMode: "portico" },
    {
      ...hostedProfileBinding("srv_target"),
      hostedClient: {
        routes: async () => routeDocument("srv_target", [
          { type: "public_direct", url: "https://target.direct.getportico.tv:32500", quality: "reachable" }
        ]),
        porticoSession: async () => { sessionIssueCalls++; return credentials("target-access", "target-refresh"); },
        reportRouteFailure: async () => ({ ok: true, matched: true })
      },
      localClient: {
        checkServerCompatibility: async () => { throw new Error("must not bootstrap"); },
        me: async () => ({ authenticated: true }),
        checkProductContractCompatibility: async () => ({})
      },
      sessionStore: store,
      runtime: routeRuntime,
      routeProbeFetch: async () => jsonResponse({
        serverId: "srv_target", serverPublicKeyFingerprint: "sha256:attacker", remoteAccessEnabled: true
      }),
      retryDelaysMs: [],
      trustedHostedDocumentKeys: trustedKeys
    }
  ), /invalid health evidence|expected server key fingerprint/);
  assert.equal(sessionIssueCalls, 0);
  assert.deepEqual(store.get(), previous);
});

test("fixture terminal and transient classes match credential cleanup behavior", async () => {
  for (const code of fixture.refreshPolicy.terminalCodes) {
    const store = createMemorySessionStore({
      apiBaseUrl: "https://local.example", accessToken: "expired", refreshToken: "refresh"
    });
    const client = createPorticoClient({
      sessionStore: store,
      credentialAdapter: { save: async () => {}, clear: async () => {} },
      transport: { fetch: async (input) => String(input).endsWith("/refresh")
        ? jsonResponse({ code, detail: "Revoked" }, { status: 403 })
        : jsonResponse({ code: "token_expired" }, { status: 401 }) }
    });
    await assert.rejects(client.system(), (error) => error instanceof ApiError && error.code === code);
    assert.equal(store.get(), undefined);
  }
  for (const code of ["refresh_expired", "refresh_rejected"]) {
    const original = { apiBaseUrl: "https://local.example", accessToken: "expired", refreshToken: "refresh" };
    const store = createMemorySessionStore(original);
    const client = createPorticoClient({
      sessionStore: store,
      credentialAdapter: { save: async () => {}, clear: async () => {} },
      transport: { fetch: async (input) => String(input).endsWith("/refresh")
        ? jsonResponse({ code, detail: "Authentication failed" }, { status: 401 })
        : jsonResponse({ code: "token_expired" }, { status: 401 }) }
    });
    await assert.rejects(client.system(), (error) => error instanceof ApiError && error.code === code);
    assert.deepEqual(store.get(), original);
  }
  for (const status of [429, 503]) {
    const original = { apiBaseUrl: "https://local.example", accessToken: "expired", refreshToken: "refresh" };
    const store = createMemorySessionStore(original);
    const client = createPorticoClient({
      sessionStore: store,
      credentialAdapter: { save: async () => {}, clear: async () => {} },
      transport: { fetch: async (input) => String(input).endsWith("/refresh")
        ? jsonResponse({ code: "temporarily_unavailable", detail: "Retry" }, { status })
        : jsonResponse({ code: "token_expired" }, { status: 401 }) }
    });
    await assert.rejects(client.system(), (error) => error instanceof ApiError && error.status === status);
    assert.deepEqual(store.get(), original);
  }
});
