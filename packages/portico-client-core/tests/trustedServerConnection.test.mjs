import assert from "node:assert/strict";
import { generateKeyPairSync, sign, verify } from "node:crypto";
import test from "node:test";
import {
  ApiError,
  connectResilientHostedServer,
  connectTrustedServerRecord,
  createMemorySessionStore,
  discoverHostedServerRoute,
  isTerminalServerAuthorizationFailure,
  refreshTrustedServerRoute,
  TrustedServerCandidateActivationError,
  TrustedServerCredentialPublicationError,
  TrustedServerDurabilityUncertainError,
  TrustedServerPublicationBlockedError
} from "../dist/index.js";
import {
  createAttachmentMethods,
  createAttachmentRuntime,
  testServerIdentity,
  testServerPublicKeyFingerprint
} from "./helpers/porticoAttachment.mjs";

const hostedDocumentTestKeys = generateKeyPairSync("ed25519");
const hostedDocumentTestKeyId = "trusted-connection-test";
const hostedDocumentTestPublicKey = hostedDocumentTestKeys.publicKey.export({ format: "der", type: "spki" }).subarray(-32).toString("base64");

function sortJSONValue(value) {
  if (Array.isArray(value)) return value.map(sortJSONValue);
  if (!value || typeof value !== "object") return value;
  return Object.fromEntries(Object.entries(value).sort(([left], [right]) => left < right ? -1 : left > right ? 1 : 0).map(([key, nested]) => [key, sortJSONValue(nested)]));
}

function signedRouteDocument(document) {
  const unsigned = {
    kind: "route-document",
    documentVersion: 1,
    endpointGeneration: 1,
    audience: "portico-media-server",
    signatureAlgorithm: "ed25519",
    signatureKeyId: hostedDocumentTestKeyId,
    ...document,
    ...testServerIdentity()
  };
  const canonical = `portico-signed-document:route-document:v1\n${JSON.stringify(sortJSONValue(unsigned))}`;
  return {
    ...unsigned,
    signature: sign(null, Buffer.from(canonical), hostedDocumentTestKeys.privateKey).toString("base64url")
  };
}

function jsonResponse(body, init = {}) {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
    ...init
  });
}

function record(overrides = {}) {
  return {
    schemaVersion: 2,
    accountId: "account-1",
    serverId: "server-1",
    profileId: "profile-1",
    serverName: "Home",
    ...testServerIdentity(),
    currentRoute: {
      url: "https://current.direct.getportico.tv:32500",
      type: "public_direct",
      verifiedAt: "2026-07-13T12:00:00.000Z"
    },
    previousRoute: {
      url: "https://previous.direct.getportico.tv:32500",
      type: "public_direct",
      verifiedAt: "2026-07-12T12:00:00.000Z"
    },
    session: {
      serverId: "server-1",
      serverName: "Home",
      apiBaseUrl: "https://current.direct.getportico.tv:32500",
      accessToken: "server-local-access",
      refreshToken: "server-local-refresh",
      authority: "hosted",
      accountId: "account-1",
      profileId: "profile-1",
      serverPublicKeyFingerprint: testServerPublicKeyFingerprint,
      routeType: "public_direct"
    },
    lastSuccessfulConnectionAt: "2026-07-13T12:00:00.000Z",
    mutationVersion: 1,
    ...overrides
  };
}

function adapter(initial = []) {
  const values = new Map(initial.map(item => [`${item.accountId}\n${item.serverId}`, structuredClone(item)]));
  const removed = [];
  return {
    removed,
    values,
    ready: async () => {},
    durability: () => "durable",
    persistencePolicy: "saved-session",
    list: async accountId => [...values.values()].filter(item => item.accountId === accountId),
    load: async (accountId, serverId) => values.get(`${accountId}\n${serverId}`),
    save: async item => { values.set(`${item.accountId}\n${item.serverId}`, structuredClone(item)); },
    compareAndSwap: async (expectedVersion, item) => {
      const key = `${item.accountId}\n${item.serverId}`;
      const current = values.get(key);
      if ((current?.mutationVersion ?? 0) !== expectedVersion) return false;
      values.set(key, structuredClone(item));
      return true;
    },
    removeWithTombstone: async tombstone => {
      removed.push([tombstone.accountId, tombstone.serverId]);
      values.delete(`${tombstone.accountId}\n${tombstone.serverId}`);
    },
    loadRemovalTombstone: async () => undefined,
    remove: async (accountId, serverId) => { removed.push([accountId, serverId]); values.delete(`${accountId}\n${serverId}`); },
    clearAccount: async accountId => {
      for (const [key, item] of values) if (item.accountId === accountId) values.delete(key);
    }
  };
}

function localClient(store, overrides = {}) {
  const issued = {
    tokenType: "Bearer",
    accessToken: "server-local-access",
    accessExpiresAt: "2026-07-14T13:00:00Z",
    refreshToken: "server-local-refresh",
    refreshExpiresAt: "2026-08-14T12:00:00Z",
    authority: "hosted",
    accountId: "account-1",
    serverId: "server-1",
    profileId: "profile-1",
    authorizationRevision: "policy-1",
    user: {id: "viewer-1"},
    device: {id: "device-1"}
  };
  return {
    ...createAttachmentMethods({sessionStore: store, serverId: "server-1", credentials: issued, now: "2026-07-14T12:00:00Z"}),
    checkServerCompatibility: async () => ({ apiVersion: "v1" }),
    checkProductContractCompatibility: async () => ({ apiVersion: "v1" }),
    checkCompatibility: async () => ({ apiVersion: "v1" }),
    me: async () => ({
      authenticated: true,
      authority: "hosted",
      accountId: "account-1",
      serverId: "server-1",
      profileId: "profile-1",
      authorizationRevision: "policy-1",
      user: { id: "viewer-1", displayName: "Viewer" }
    }),
    revokeNativeSession: async () => ({ ok: true }),
    ...overrides
  };
}

const trustedAttachmentRuntime = createAttachmentRuntime("2026-07-14T12:00:00Z");

const stageCandidate = async () => ({
  publish: async () => {},
  fenceRollback: () => {},
  rollback: async () => {}
});

test("fresh route recovery atomically rebases an existing credential without minting a new session", async () => {
  const stored = record();
  const connections = adapter([stored]);
  const active = createMemorySessionStore(stored.session);
  const freshRoute = "https://fresh.direct.getportico.tv:32500";
  let mintCalls = 0;
  const result = await refreshTrustedServerRoute(
    stored,
    { ...testServerIdentity(), id: stored.serverId, name: stored.serverName, preferredAuthMode: "portico" },
    {
      accountId: stored.accountId,
      connectionAdapter: connections,
      sessionStore: active,
      stageCandidate,
      createLocalClient: store => localClient(store),
      hostedClient: {
        checkCompatibility: async () => ({ apiVersion: "v1" }),
        routes: async () => signedRouteDocument({
          serverId: stored.serverId,
          serverPublicKeyFingerprint: stored.serverPublicKeyFingerprint,
          issuedAt: "2026-07-14T11:59:00.000Z",
          expiresAt: "2026-07-14T12:05:00.000Z",
          routes: [{ type: "public_direct", url: freshRoute, quality: "healthy" }]
        }),
        porticoSession: async () => {
          mintCalls += 1;
          throw new Error("route recovery must not mint");
        }
      },
      loadTrustedHostedDocumentKeys: async () => ({ [hostedDocumentTestKeyId]: hostedDocumentTestPublicKey }),
      runtime: trustedAttachmentRuntime,
      routeProbeFetch: async input => {
        assert.match(String(input), /^https:\/\/fresh\.direct\.getportico\.tv:32500\/api\/remote-access\/health/);
        return jsonResponse({
          serverId: stored.serverId,
          serverPublicKeyFingerprint: stored.serverPublicKeyFingerprint,
          remoteAccessEnabled: true
        });
      },
      retryDelaysMs: [],
      routePreference: "public-only",
      now: () => new Date("2026-07-14T12:00:00.000Z")
    }
  );

  assert.equal(mintCalls, 0);
  assert.equal(result.session.apiBaseUrl, freshRoute);
  assert.equal(result.session.accessToken, stored.session.accessToken);
  assert.equal(result.session.refreshToken, stored.session.refreshToken);
  assert.equal(active.get().apiBaseUrl, freshRoute);
  assert.equal(connections.values.get(`${stored.accountId}\n${stored.serverId}`).currentRoute.url, freshRoute);
});

test("cached recovery verifies pinned identity, uses the previous route, and retains the failed current hint", async () => {
  const stored = record();
  const connections = adapter([stored]);
  const active = createMemorySessionStore();
  const probes = [];
  let probeOptions;
  const result = await connectTrustedServerRecord(stored, {
    connectionAdapter: connections,
    sessionStore: active,
    stageCandidate,
    createLocalClient: store => localClient(store),
    routeProbeFetch: async (input, init) => {
      const url = String(input);
      probeOptions = init;
      probes.push(url);
      if (url.startsWith(stored.currentRoute.url)) throw new TypeError("current route offline");
      return jsonResponse({
        serverId: stored.serverId,
        serverPublicKeyFingerprint: stored.serverPublicKeyFingerprint,
        remoteAccessEnabled: true
      });
    },
    now: () => new Date("2026-07-14T12:00:00.000Z")
  });

  assert.equal(result.source, "cached");
  assert.equal(result.session.apiBaseUrl, stored.previousRoute.url);
  assert.equal(result.record.currentRoute.url, stored.previousRoute.url);
  assert.equal(result.record.previousRoute.url, stored.currentRoute.url);
  assert.equal(active.get().accessToken, "server-local-access");
  assert.equal(result.durability, "durable");
  assert.equal(probes.length, 2, "current and previous route hints are attempted together");
  assert.equal(probeOptions.redirect, "error");
  assert.equal(probeOptions.credentials, "omit");
  assert.equal(probeOptions.cache, "no-store");
  assert.deepEqual(connections.removed, []);
});

test("Hosted discovery failure does not block or erase a verified cached server connection", async () => {
  const stored = record({ previousRoute: undefined });
  const connections = adapter([stored]);
  const active = createMemorySessionStore();
  const result = await connectResilientHostedServer(
    { ...testServerIdentity(), id: stored.serverId, name: stored.serverName, preferredAuthMode: "portico" },
    {
      accountId: stored.accountId,
      connectionAdapter: connections,
      hostedClient: {},
      loadTrustedHostedDocumentKeys: async () => { throw new TypeError("Hosted Services unavailable"); },
      clientIdentity: { installationId: "install-1", deviceName: "iPhone", app: "Portico", platform: "iOS" },
      sessionStore: active,
      stageCandidate,
      createLocalClient: store => localClient(store),
      routeProbeFetch: async () => jsonResponse({
        serverId: stored.serverId,
        serverPublicKeyFingerprint: stored.serverPublicKeyFingerprint,
        remoteAccessEnabled: true
      })
    }
  );

  assert.equal(result.source, "cached");
  assert.equal(result.identity.authenticated, true);
  assert.equal(connections.values.size, 1);
  assert.deepEqual(connections.removed, []);
});

test("cached LAN and public routes use a staggered first-success race and cancel the loser", async () => {
  const stored = record({
    previousRoute: {
      url: "https://192-168-1-20.direct.getportico.tv:32500",
      type: "lan_ip_encoded",
      verifiedAt: "2026-07-13T11:00:00.000Z"
    }
  });
  const connections = adapter([stored]);
  let lanAborted = false;
  let hostedDiscoveryCalls = 0;
  const startedAt = Date.now();
  const result = await connectResilientHostedServer(
    { ...testServerIdentity(), id: stored.serverId, name: stored.serverName, preferredAuthMode: "portico" },
    {
      accountId: stored.accountId,
      connectionAdapter: connections,
      hostedClient: {},
      loadTrustedHostedDocumentKeys: async () => {
        hostedDiscoveryCalls += 1;
        throw new Error("Hosted discovery must not start for a quick cached winner.");
      },
      selectionEnvelope: { accountId: stored.accountId, serverId: stored.serverId, profileId: stored.profileId, installationId: "install-1" },
      clientIdentity: { installationId: "install-1", deviceName: "iPhone", app: "Portico", platform: "iOS" },
      routePreference: "lan-first",
      sessionStore: createMemorySessionStore(stored.session),
      stageCandidate,
      createLocalClient: store => localClient(store),
      routeProbeFetch: async (input, init) => {
        if (String(input).startsWith(stored.previousRoute.url)) {
          return new Promise((_, reject) => {
            const abort = () => {
              lanAborted = true;
              reject(new Error("LAN probe cancelled after the public route won."));
            };
            init?.signal?.addEventListener("abort", abort, { once: true });
          });
        }
        return jsonResponse({
          serverId: stored.serverId,
          serverPublicKeyFingerprint: stored.serverPublicKeyFingerprint,
          remoteAccessEnabled: true
        });
      }
    }
  );

  assert.equal(result.source, "cached");
  assert.equal(result.session.apiBaseUrl, stored.currentRoute.url);
  assert.equal(hostedDiscoveryCalls, 0);
  assert.equal(lanAborted, true);
  assert.ok(Date.now() - startedAt < 450, "LAN failure must not impose a full route timeout before public routing");
});

test("Hosted discovery is hedged after 500 ms while a cached probe remains eligible to win", async () => {
  const stored = record({ previousRoute: undefined });
  const connections = adapter([stored]);
  let releaseCachedProbe;
  let hostedStartedAt = 0;
  const startedAt = Date.now();
  const result = await connectResilientHostedServer(
    { ...testServerIdentity(), id: stored.serverId, name: stored.serverName, preferredAuthMode: "portico" },
    {
      accountId: stored.accountId,
      connectionAdapter: connections,
      hostedClient: {},
      loadTrustedHostedDocumentKeys: async () => {
        hostedStartedAt = Date.now();
        releaseCachedProbe(jsonResponse({
          serverId: stored.serverId,
          serverPublicKeyFingerprint: stored.serverPublicKeyFingerprint,
          remoteAccessEnabled: true
        }));
        throw new TypeError("Hosted Services unavailable");
      },
      selectionEnvelope: { accountId: stored.accountId, serverId: stored.serverId, profileId: stored.profileId, installationId: "install-1" },
      clientIdentity: { installationId: "install-1", deviceName: "iPhone", app: "Portico", platform: "iOS" },
      sessionStore: createMemorySessionStore(stored.session),
      stageCandidate,
      createLocalClient: store => localClient(store),
      routeProbeFetch: async () => new Promise(resolve => { releaseCachedProbe = resolve; })
    }
  );

  assert.equal(result.source, "cached");
  assert.ok(hostedStartedAt - startedAt >= 450, "Hosted must not receive a healthy warm-start request before the hedge window");
  assert.ok(hostedStartedAt - startedAt < 800, "Hosted discovery should start close to the 500 ms hedge boundary");
});

test("a conclusive cached failure starts Hosted discovery without waiting for the hedge timer", async () => {
  const stored = record({ previousRoute: undefined });
  const startedAt = Date.now();
  let hostedStartedAt = 0;
  await assert.rejects(connectResilientHostedServer(
    { ...testServerIdentity(), id: stored.serverId, name: stored.serverName, preferredAuthMode: "portico" },
    {
      accountId: stored.accountId,
      connectionAdapter: adapter([stored]),
      hostedClient: {},
      loadTrustedHostedDocumentKeys: async () => {
        hostedStartedAt = Date.now();
        throw new TypeError("Hosted Services unavailable");
      },
      selectionEnvelope: { accountId: stored.accountId, serverId: stored.serverId, profileId: stored.profileId, installationId: "install-1" },
      clientIdentity: { installationId: "install-1", deviceName: "iPhone", app: "Portico", platform: "iOS" },
      sessionStore: createMemorySessionStore(stored.session),
      stageCandidate,
      createLocalClient: store => localClient(store),
      routeProbeFetch: async () => { throw new TypeError("remembered route is offline"); }
    }
  ), /Hosted Services unavailable/);

  assert.ok(hostedStartedAt > 0);
  assert.ok(hostedStartedAt - startedAt < 300, "a definitive cached failure should bypass the 500 ms hedge delay");
});

test("slow nearby discovery does not block a verified public Hosted route", async () => {
  const stored = record();
  const publicRoute = "https://fresh.direct.getportico.tv:32500";
  let localDiscoveryAborted = false;
  const startedAt = Date.now();
  const discovery = await discoverHostedServerRoute(
    { ...testServerIdentity(), id: stored.serverId, name: stored.serverName, preferredAuthMode: "portico" },
    {
      hostedClient: {
        checkCompatibility: async () => ({ apiVersion: "v1" }),
        routes: async () => signedRouteDocument({
          serverId: stored.serverId,
          issuedAt: "2026-07-14T11:59:00.000Z",
          expiresAt: "2026-07-14T12:05:00.000Z",
          routes: [{ type: "public_direct", url: publicRoute, quality: "reachable" }]
        })
      },
      trustedHostedDocumentKeys: { [hostedDocumentTestKeyId]: hostedDocumentTestPublicKey },
      runtime: trustedAttachmentRuntime,
      routePreference: "lan-first",
      retryDelaysMs: [],
      localRouteCandidates: async (_server, _document, signal) => new Promise((_, reject) => {
        signal?.addEventListener("abort", () => {
          localDiscoveryAborted = true;
          reject(new Error("Nearby discovery cancelled after public success."));
        }, { once: true });
      }),
      routeProbeFetch: async () => jsonResponse({
        serverId: stored.serverId,
        serverPublicKeyFingerprint: stored.serverPublicKeyFingerprint,
        remoteAccessEnabled: true
      }),
      now: () => new Date("2026-07-14T12:00:00.000Z")
    }
  );

  assert.equal(discovery.route.url, publicRoute);
  assert.equal(localDiscoveryAborted, true);
  assert.ok(Date.now() - startedAt < 500, "Bonjour discovery must not serialize the public route probe");
});

test("public-first recovery probes a verified LAN rollback route before requiring Hosted discovery", async () => {
  const stored = record({
    previousRoute: {
      url: "https://192-168-1-20.direct.getportico.tv:32500",
      type: "lan_ip_encoded",
      verifiedAt: "2026-07-13T11:00:00.000Z"
    }
  });
  const connections = adapter([stored]);
  const probes = [];
  let hostedDiscoveryCalls = 0;
  const result = await connectResilientHostedServer(
    { ...testServerIdentity(), id: stored.serverId, name: stored.serverName, preferredAuthMode: "portico" },
    {
      accountId: stored.accountId,
      connectionAdapter: connections,
      hostedClient: {},
      loadTrustedHostedDocumentKeys: async () => {
        hostedDiscoveryCalls += 1;
        throw new TypeError("Hosted Services unavailable");
      },
      clientIdentity: { installationId: "install-1", deviceName: "iPhone", app: "Portico", platform: "iOS" },
      routePreference: "public-first",
      sessionStore: createMemorySessionStore(stored.session),
      stageCandidate,
      createLocalClient: store => localClient(store),
      routeProbeFetch: async input => {
        const url = String(input);
        probes.push(url);
        if (url.startsWith(stored.currentRoute.url)) throw new TypeError("public route unavailable");
        return jsonResponse({
          serverId: stored.serverId,
          serverPublicKeyFingerprint: stored.serverPublicKeyFingerprint,
          remoteAccessEnabled: true
        });
      }
    }
  );

  assert.equal(result.source, "cached");
  assert.equal(result.session.apiBaseUrl, stored.previousRoute.url);
  assert.equal(hostedDiscoveryCalls, 0);
  assert.equal(probes.length, 2);
});

test("forced fresh route discovery bypasses cached probes without deleting the trusted rollback record", async () => {
  const stored = record();
  const connections = adapter([stored]);
  let cachedProbeCalls = 0;
  await assert.rejects(connectResilientHostedServer(
    { ...testServerIdentity(), id: stored.serverId, name: stored.serverName, preferredAuthMode: "portico" },
    {
      accountId: stored.accountId,
      connectionAdapter: connections,
      hostedClient: {
        checkCompatibility: async () => ({ apiVersion: "v1" }),
        routes: async () => { throw new TypeError("network transition in progress"); }
      },
      loadTrustedHostedDocumentKeys: async () => ({ [hostedDocumentTestKeyId]: hostedDocumentTestPublicKey }),
      runtime: trustedAttachmentRuntime,
      selectionEnvelope: { accountId: stored.accountId, serverId: stored.serverId, profileId: stored.profileId, installationId: "install-1" },
      clientIdentity: { installationId: "install-1", deviceName: "iPhone", app: "Portico", platform: "iOS" },
      sessionStore: createMemorySessionStore(stored.session),
      stageCandidate,
      createLocalClient: store => localClient(store),
      routeProbeFetch: async () => {
        cachedProbeCalls += 1;
        return jsonResponse({
          serverId: stored.serverId,
          serverPublicKeyFingerprint: stored.serverPublicKeyFingerprint,
          remoteAccessEnabled: true
        });
      },
      retryDelaysMs: [],
      forceFreshRouteDiscovery: true
    }
  ), /network transition/);

  assert.equal(cachedProbeCalls, 0);
  assert.deepEqual(
    connections.values.get(`${stored.accountId}\n${stored.serverId}`),
    stored,
  );
  assert.deepEqual(connections.removed, []);
});

test("a cached connection win never mints an unused Hosted credential family", async () => {
  const stored = record({ previousRoute: undefined });
  const connections = adapter([stored]);
  let mintCalls = 0;
  const hostedClient = {
    checkCompatibility: async () => ({ apiVersion: "v1" }),
    routes: async () => {
      await new Promise(resolve => setTimeout(resolve, 25));
      return signedRouteDocument({
        serverId: stored.serverId,
        serverName: stored.serverName,
        assignedHostname: "fresh.direct.getportico.tv",
        issuedAt: "2026-07-14T11:59:00.000Z",
        expiresAt: "2026-07-14T12:05:00.000Z",
        serverPublicKeyFingerprint: stored.serverPublicKeyFingerprint,
        authModes: ["portico"],
        certificate: { status: "valid" },
        membership: { role: "owner" },
        routes: [{ type: "public_direct", url: "https://fresh.direct.getportico.tv:32500", quality: "reachable" }]
      });
    },
    porticoSession: async () => {
      mintCalls += 1;
      throw new Error("must not mint after cached startup wins");
    },
    reportRouteFailure: async () => ({ ok: true, matched: true })
  };
  const result = await connectResilientHostedServer(
    { ...testServerIdentity(), id: stored.serverId, name: stored.serverName, preferredAuthMode: "portico" },
    {
      accountId: stored.accountId,
      connectionAdapter: connections,
      hostedClient,
      loadTrustedHostedDocumentKeys: async () => ({ [hostedDocumentTestKeyId]: hostedDocumentTestPublicKey }),
      runtime: trustedAttachmentRuntime,
      clientIdentity: { installationId: "install-1", deviceName: "Browser", app: "Portico", platform: "Web" },
      sessionStore: createMemorySessionStore(),
      stageCandidate,
      createLocalClient: store => localClient(store),
      runtime: {
        verifyEd25519: ({ signature, message }) => verify(null, Buffer.from(message), hostedDocumentTestKeys.publicKey, Buffer.from(signature))
      },
      routeProbeFetch: async input => {
        const url = String(input);
        return jsonResponse({
          serverId: stored.serverId,
          serverPublicKeyFingerprint: stored.serverPublicKeyFingerprint,
          remoteAccessEnabled: true,
          route: url
        });
      },
      now: () => new Date("2026-07-14T12:00:00.000Z")
    }
  );

  assert.equal(result.source, "cached");
  await new Promise(resolve => setTimeout(resolve, 60));
  assert.equal(mintCalls, 0);
});

test("only an explicit server-scoped revocation removes that server record", async () => {
  const stored = record({ previousRoute: undefined });
  const connections = adapter([stored]);
  await assert.rejects(connectTrustedServerRecord(stored, {
    connectionAdapter: connections,
    sessionStore: createMemorySessionStore(),
    stageCandidate,
    createLocalClient: store => localClient(store, {
      me: async () => { throw new ApiError(403, "membership_inactive", "Membership revoked."); }
    }),
    routeProbeFetch: async () => jsonResponse({
      serverId: stored.serverId,
      serverPublicKeyFingerprint: stored.serverPublicKeyFingerprint,
      remoteAccessEnabled: true
    })
  }), error => isTerminalServerAuthorizationFailure(error));

  assert.deepEqual(connections.removed, [[stored.accountId, stored.serverId]]);
  assert.equal(connections.values.size, 0);
});

test("a staged candidate fences the old runtime and publishes only after credential and durable publication", async () => {
  const stored = record({ previousRoute: undefined });
  const connections = adapter([stored]);
  const save = connections.save;
  const previousActive = {
    serverId: "server-before",
    apiBaseUrl: "https://before.example",
    accessToken: "before-access",
    refreshToken: "before-refresh"
  };
  const active = createMemorySessionStore(previousActive);
  const ordering = [];
  connections.save = async item => {
    ordering.push("durable-save");
    await save(item);
  };

  const result = await connectTrustedServerRecord(stored, {
    connectionAdapter: connections,
    sessionStore: active,
    createLocalClient: store => localClient(store),
    stageCandidate: async candidate => {
      ordering.push("stage");
      assert.equal(candidate.identity.authenticated, true);
      assert.equal(candidate.session.accessToken, "server-local-access");
      assert.equal(active.get().accessToken, "before-access", "candidate credentials remain isolated while the old runtime is fenced");
      assert.equal(connections.values.get(`${stored.accountId}\n${stored.serverId}`).session.accessToken, "server-local-access");
      return {
        publish: async publication => {
          ordering.push("publish");
          assert.deepEqual(publication, {
            durability: "durable",
            persistencePolicy: "saved-session",
            durabilityError: undefined
          });
          assert.equal(active.get().accessToken, "server-local-access");
          assert.equal(connections.values.get(`${stored.accountId}\n${stored.serverId}`).session.accessToken, "server-local-access");
        },
        fenceRollback: () => {},
        rollback: async () => { throw new Error("successful publication must not roll back"); }
      };
    },
    routeProbeFetch: async () => jsonResponse({
      serverId: stored.serverId,
      serverPublicKeyFingerprint: stored.serverPublicKeyFingerprint,
      remoteAccessEnabled: true
    })
  });

  assert.equal(result.durability, "durable");
  assert.deepEqual(ordering, ["stage", "durable-save", "publish"]);
  assert.equal(active.get().accessToken, "server-local-access");
  assert.equal(connections.values.get(`${stored.accountId}\n${stored.serverId}`).session.accessToken, "server-local-access");
  assert.deepEqual(connections.removed, []);
});

test("the final authenticated viewer revision replaces a stale credential snapshot before publication", async () => {
  const stored = record({
    previousRoute: undefined,
    session: {
      ...record().session,
      authority: "hosted",
      accountId: "account-1",
      profileId: "profile-1",
      authorizationRevision: "policy-1"
    }
  });
  const connections = adapter([stored]);
  const active = createMemorySessionStore(stored.session);
  let stagedRevision;

  const result = await connectTrustedServerRecord(stored, {
    accountId: stored.accountId,
    connectionAdapter: connections,
    sessionStore: active,
    createLocalClient: store => localClient(store, {
      me: async () => ({
        authenticated: true,
        authority: "hosted",
        accountId: stored.accountId,
        serverId: stored.serverId,
        profileId: stored.profileId,
        authorizationRevision: "policy-2",
        user: { id: "viewer-1", displayName: "Viewer" }
      })
    }),
    stageCandidate: async candidate => {
      stagedRevision = candidate.session.authorizationRevision;
      assert.equal(candidate.scope.authorizationRevision, "policy-2");
      assert.equal(candidate.session.authorizationRevision, "policy-2");
      return stageCandidate();
    },
    routeProbeFetch: async () => jsonResponse({
      serverId: stored.serverId,
      serverPublicKeyFingerprint: stored.serverPublicKeyFingerprint,
      remoteAccessEnabled: true
    })
  });

  assert.equal(stagedRevision, "policy-2");
  assert.equal(result.session.authorizationRevision, "policy-2");
  assert.equal(result.record.session.authorizationRevision, "policy-2");
  assert.equal(active.get().authorizationRevision, "policy-2");
  assert.equal(
    connections.values.get(`${stored.accountId}\n${stored.serverId}`).session.authorizationRevision,
    "policy-2"
  );
});

test("cached connection rejects a final viewer scope bound to another profile before activation", async () => {
  const stored = record({ previousRoute: undefined });
  const connections = adapter([stored]);
  const active = createMemorySessionStore({ serverId: "before", apiBaseUrl: "https://before.example", accessToken: "before-access" });
  let stageCalls = 0;

  await assert.rejects(connectTrustedServerRecord(stored, {
    accountId: stored.accountId,
    connectionAdapter: connections,
    sessionStore: active,
    createLocalClient: store => localClient(store, {
      me: async () => ({
        authenticated: true,
        authority: "hosted",
        accountId: stored.accountId,
        serverId: stored.serverId,
        profileId: "profile-2",
        authorizationRevision: "policy-2",
        user: { id: "profile-2", displayName: "Other profile" }
      })
    }),
    stageCandidate: async () => { stageCalls += 1; return stageCandidate(); },
    routeProbeFetch: async () => jsonResponse({
      serverId: stored.serverId,
      serverPublicKeyFingerprint: stored.serverPublicKeyFingerprint,
      remoteAccessEnabled: true
    })
  }), error => error instanceof TrustedServerCandidateActivationError && /viewer scope/.test(error.message));

  assert.equal(stageCalls, 0);
  assert.equal(active.get().accessToken, "before-access");
  assert.deepEqual(connections.values.get(`${stored.accountId}\n${stored.serverId}`), stored);
});

test("a rejected live publication revokes the unpublished credential and preserves prior state", async () => {
  const server = { ...testServerIdentity(), id: "server-1", name: "Home", preferredAuthMode: "portico" };
  const connections = adapter();
  const prior = {
    serverId: "server-before",
    apiBaseUrl: "https://before.example",
    accessToken: "before-access",
    refreshToken: "before-refresh"
  };
  const active = createMemorySessionStore(prior);
  const revoked = [];
  const envelope = { accountId: "account-1", serverId: server.id, profileId: "profile-1", installationId: "install-1" };
  const document = signedRouteDocument({
    serverId: server.id,
    serverName: server.name,
    assignedHostname: "home.direct.getportico.tv",
    issuedAt: "2026-07-14T11:59:00.000Z",
    expiresAt: "2026-07-14T12:05:00.000Z",
    serverPublicKeyFingerprint: "sha256:home",
    authModes: ["portico"],
    certificate: { status: "valid" },
    membership: { role: "owner" },
    routes: [{ type: "public_direct", url: "https://home.direct.getportico.tv:32500", quality: "reachable" }]
  });

  await assert.rejects(connectResilientHostedServer(server, {
    accountId: "account-1",
    connectionAdapter: connections,
    sessionStore: active,
    hostedClient: {
      checkCompatibility: async () => ({ apiVersion: "v1" }),
      routes: async () => document,
      porticoSession: async () => ({ accessToken: "bootstrap-access", accessExpiresAt: "2026-07-14T12:01:00.000Z" }),
      reportRouteFailure: async () => ({ ok: true, matched: true })
    },
    loadTrustedHostedDocumentKeys: async () => ({ [hostedDocumentTestKeyId]: hostedDocumentTestPublicKey }),
    runtime: trustedAttachmentRuntime,
    selectionEnvelope: envelope,
    clientIdentity: { installationId: "install-1", deviceName: "iPhone", app: "Portico", platform: "iOS" },
    createLocalClient: store => localClient(store, {
      acceptPorticoSessionCredentials: async () => {
        const provisional = store.get();
        const credentials = {
          tokenType: "Bearer",
          authority: "hosted",
          accountId: "account-1",
          serverId: server.id,
          profileId: "profile-1",
          authorizationRevision: "policy-1",
          accessExpiresAt: "2026-07-14T12:05:00.000Z",
          refreshExpiresAt: "2027-01-01T00:00:00.000Z",
          accessToken: "new-server-access",
          refreshToken: "new-server-refresh",
          user: { id: "viewer-1" },
          device: { id: "device-1" }
        };
        store.set({
          ...provisional,
          bootstrapAccessToken: undefined,
          accessToken: credentials.accessToken,
          refreshToken: credentials.refreshToken
        });
        return credentials;
      },
      checkProductContractCompatibility: async () => ({ apiVersion: "v1" }),
      revokeNativeSession: async refreshToken => { revoked.push(refreshToken); return { ok: true }; }
    }),
    stageCandidate: async candidate => {
      assert.equal(candidate.session.accessToken, "new-server-access");
      assert.equal(active.get().accessToken, "before-access");
      assert.equal(connections.values.size, 0);
      return {
        publish: async () => { throw new Error("viewer transition rejected"); },
        fenceRollback: () => {},
        rollback: async () => {}
      };
    },
    runtime: trustedAttachmentRuntime,
    routeProbeFetch: async () => jsonResponse({
      serverId: server.id,
      serverPublicKeyFingerprint: testServerPublicKeyFingerprint,
      remoteAccessEnabled: true
    }),
    retryDelaysMs: [],
    now: () => new Date("2026-07-14T12:00:00.000Z")
  }), error => error instanceof TrustedServerCandidateActivationError && /viewer transition rejected/.test(error.message));

  assert.deepEqual(revoked, ["new-server-refresh"]);
  assert.equal(active.get().accessToken, "before-access");
  assert.equal(connections.values.size, 0);
});

test("durable persistence failure is the only memory-only success and publication still happens last", async () => {
  const stored = record({ previousRoute: undefined });
  const base = adapter([stored]);
  const durableBefore = structuredClone(base.values.get(`${stored.accountId}\n${stored.serverId}`));
  const connections = {
    ...base,
    save: async () => { throw new Error("secure storage unavailable"); }
  };
  const active = createMemorySessionStore({
    serverId: "server-before",
    apiBaseUrl: "https://before.example",
    accessToken: "before-access"
  });
  const ordering = [];

  const result = await connectTrustedServerRecord(stored, {
    connectionAdapter: connections,
    sessionStore: active,
    createLocalClient: store => localClient(store),
    stageCandidate: async () => {
      ordering.push("stage");
      assert.equal(active.get().accessToken, "before-access");
      return {
        publish: async () => {
          ordering.push("publish");
          assert.equal(active.get().accessToken, "server-local-access");
        },
        fenceRollback: () => {},
        rollback: async () => { throw new Error("memory-only success must not roll back"); }
      };
    },
    routeProbeFetch: async () => jsonResponse({
      serverId: stored.serverId,
      serverPublicKeyFingerprint: stored.serverPublicKeyFingerprint,
      remoteAccessEnabled: true
    })
  });

  assert.deepEqual(ordering, ["stage", "publish"]);
  assert.equal(result.durability, "memory-only");
  assert.match(result.durabilityError.message, /secure storage unavailable/);
  assert.equal(active.get().accessToken, "server-local-access", "verified session stays active in memory");
  assert.deepEqual(base.values.get(`${stored.accountId}\n${stored.serverId}`), durableBefore, "failed persistence does not corrupt the prior durable record");
});

test("a healthy reauthorize-on-start policy is distinct from unexpected memory-only persistence", async () => {
  const stored = record({ previousRoute: undefined });
  const connections = {
    ...adapter([stored]),
    durability: () => "durable",
    persistencePolicy: "reauthorize-on-start"
  };
  const active = createMemorySessionStore({
    serverId: "server-before",
    apiBaseUrl: "https://before.example",
    accessToken: "before-access"
  });
  const ordering = [];
  const save = connections.save;
  connections.save = async item => {
    ordering.push("save");
    await save(item);
  };

  const result = await connectTrustedServerRecord(stored, {
    connectionAdapter: connections,
    sessionStore: active,
    createLocalClient: store => localClient(store),
    stageCandidate: async () => {
      ordering.push("stage");
      return {
        publish: async () => { ordering.push("publish"); },
        fenceRollback: () => {},
        rollback: async () => { throw new Error("successful ephemeral publication must not roll back"); }
      };
    },
    routeProbeFetch: async () => jsonResponse({
      serverId: stored.serverId,
      serverPublicKeyFingerprint: stored.serverPublicKeyFingerprint,
      remoteAccessEnabled: true
    })
  });

  assert.deepEqual(ordering, ["stage", "save", "publish"]);
  assert.equal(result.durability, "durable");
  assert.equal(result.persistencePolicy, "reauthorize-on-start");
  assert.equal(result.durabilityError, undefined);
  assert.equal(active.get().accessToken, "server-local-access");
});

test("adapter health can report an unexpected memory-only save under a reauthorization policy", async () => {
  const stored = record({ previousRoute: undefined });
  const connections = {
    ...adapter([stored]),
    durability: () => "memory-only",
    persistencePolicy: "reauthorize-on-start"
  };
  const result = await connectTrustedServerRecord(stored, {
    connectionAdapter: connections,
    sessionStore: createMemorySessionStore(),
    createLocalClient: store => localClient(store),
    stageCandidate,
    routeProbeFetch: async () => jsonResponse({
      serverId: stored.serverId,
      serverPublicKeyFingerprint: stored.serverPublicKeyFingerprint,
      remoteAccessEnabled: true
    })
  });

  assert.equal(result.durability, "memory-only");
  assert.equal(result.persistencePolicy, "reauthorize-on-start");
  assert.equal(result.durabilityError, undefined);
});

test("an atomicity-uncertain durable write is fatal and restores the prior transaction", async () => {
  const stored = record({ previousRoute: undefined });
  const connections = adapter([stored]);
  const durableBefore = structuredClone(connections.values.get(`${stored.accountId}\n${stored.serverId}`));
  const save = connections.save;
  const compareAndSwap = connections.compareAndSwap;
  let saveCalls = 0;
  let compareAndSwapCalls = 0;
  connections.save = async item => {
    saveCalls += 1;
    if (saveCalls === 1) {
      await save(item);
      throw new TrustedServerDurabilityUncertainError(new Error("second secure store failed"), [new Error("compensation failed")]);
    }
    await save(item);
  };
  connections.compareAndSwap = async (expectedVersion, item) => {
    compareAndSwapCalls += 1;
    return compareAndSwap(expectedVersion, item);
  };
  const previous = { serverId: "server-before", apiBaseUrl: "https://before.example", accessToken: "before-access" };
  const active = createMemorySessionStore(previous);
  let published = false;
  const rollbackModes = [];

  await assert.rejects(connectTrustedServerRecord(stored, {
    connectionAdapter: connections,
    sessionStore: active,
    createLocalClient: store => localClient(store),
    stageCandidate: async () => ({
      publish: async () => { published = true; },
      fenceRollback: () => {},
      rollback: async mode => { rollbackModes.push(mode ?? "restore-previous"); }
    }),
    routeProbeFetch: async () => jsonResponse({
      serverId: stored.serverId,
      serverPublicKeyFingerprint: stored.serverPublicKeyFingerprint,
      remoteAccessEnabled: true
    })
  }), error => error instanceof TrustedServerCredentialPublicationError
    && error.cause instanceof TrustedServerDurabilityUncertainError
    && error.failClosed === false
    && error.rollbackFailures.some(failure => failure?.message === "compensation failed"));

  assert.equal(saveCalls, 1, "the uncertain candidate write is not replayed");
  assert.equal(compareAndSwapCalls, 1, "the previous durable value is restored with version-fenced CAS");
  assert.equal(published, false);
  assert.deepEqual(rollbackModes, ["restore-previous"]);
  assert.equal(active.get().accessToken, "before-access");
  assert.deepEqual(connections.values.get(`${stored.accountId}\n${stored.serverId}`), durableBefore);
});

test("rollback fences candidate runtime synchronously before credential and paused durable restoration", async () => {
  const stored = record({ previousRoute: undefined });
  const connections = adapter([stored]);
  const save = connections.save;
  const compareAndSwap = connections.compareAndSwap;
  let releaseDurableRestore;
  let announceDurableRestore;
  const durableRestoreStarted = new Promise(resolve => { announceDurableRestore = resolve; });
  const durableRestoreGate = new Promise(resolve => { releaseDurableRestore = resolve; });
  connections.save = async item => {
    await save(item);
    throw new TrustedServerDurabilityUncertainError(new Error("candidate durable write became uncertain"));
  };
  connections.compareAndSwap = async (expectedVersion, item) => {
    assert.equal(runtimeState, "fenced", "B is retracted before durable restoration starts");
    announceDurableRestore();
    await durableRestoreGate;
    assert.equal(runtimeState, "fenced", "neither A nor B runtime is republished while durable restoration waits");
    return compareAndSwap(expectedVersion, item);
  };
  const previous = { serverId: "server-before", apiBaseUrl: "https://before.example", accessToken: "before-access" };
  let current = previous;
  let runtimeState = "A";

  const pending = assert.rejects(connectTrustedServerRecord(stored, {
    connectionAdapter: connections,
    sessionStore: {
      get: () => current,
      set: session => {
        if (session.accessToken === "before-access") {
          assert.equal(runtimeState, "fenced", "B is retracted before A credentials are restored");
        }
        current = session;
      },
      clear: () => { current = undefined; }
    },
    createLocalClient: store => localClient(store),
    stageCandidate: async () => ({
      publish: async () => { runtimeState = "B"; },
      fenceRollback: () => { runtimeState = "fenced"; },
      rollback: async mode => {
        assert.equal(runtimeState, "fenced");
        runtimeState = mode === "fail-closed" ? "closed" : "A";
      }
    }),
    routeProbeFetch: async () => jsonResponse({
      serverId: stored.serverId,
      serverPublicKeyFingerprint: stored.serverPublicKeyFingerprint,
      remoteAccessEnabled: true
    })
  }), error => error instanceof TrustedServerCredentialPublicationError);

  await durableRestoreStarted;
  assert.equal(runtimeState, "fenced");
  assert.equal(current.accessToken, "before-access");
  releaseDurableRestore();
  await pending;
  assert.equal(runtimeState, "A");
});

test("a publication policy fence forces both the candidate and previous runtime closed", async () => {
  const stored = record({ previousRoute: undefined });
  const connections = adapter([stored]);
  connections.save = async () => {
    throw new TrustedServerPublicationBlockedError(new Error("account was signed out in another tab"));
  };
  let current = {
    serverId: "server-before",
    apiBaseUrl: "https://before.example",
    accessToken: "before-access"
  };
  let published = false;
  const rollbackModes = [];

  await assert.rejects(connectTrustedServerRecord(stored, {
    connectionAdapter: connections,
    sessionStore: {
      get: () => current,
      set: session => { current = session; },
      clear: () => { current = undefined; }
    },
    createLocalClient: store => localClient(store),
    stageCandidate: async () => ({
      publish: async () => { published = true; },
      fenceRollback: () => {},
      rollback: async mode => { rollbackModes.push(mode ?? "restore-previous"); }
    }),
    routeProbeFetch: async () => jsonResponse({
      serverId: stored.serverId,
      serverPublicKeyFingerprint: stored.serverPublicKeyFingerprint,
      remoteAccessEnabled: true
    })
  }), error => error instanceof TrustedServerCredentialPublicationError
    && error.cause instanceof TrustedServerPublicationBlockedError
    && error.failClosed === true);

  assert.equal(published, false);
  assert.deepEqual(rollbackModes, ["fail-closed"]);
  assert.equal(current, undefined);
  assert.equal(connections.values.size, 0);
  assert.deepEqual(connections.removed, [[stored.accountId, stored.serverId]]);
});

test("SessionStore publication failure is fatal, restores A, and never publishes B", async () => {
  const stored = record({ previousRoute: undefined });
  const connections = adapter([stored]);
  const before = structuredClone(connections.values.get(`${stored.accountId}\n${stored.serverId}`));
  const previous = { serverId: "server-before", apiBaseUrl: "https://before.example", accessToken: "before-access" };
  let current = previous;
  let published = false;
  const rollbackModes = [];
  await assert.rejects(connectTrustedServerRecord(stored, {
    connectionAdapter: connections,
    sessionStore: {
      get: () => current,
      set: session => {
        if (session.accessToken === "server-local-access") throw new Error("active session store unavailable");
        current = session;
      },
      clear: () => { current = undefined; }
    },
    createLocalClient: store => localClient(store),
    stageCandidate: async () => ({
      publish: async () => { published = true; },
      fenceRollback: () => {},
      rollback: async mode => { rollbackModes.push(mode ?? "restore-previous"); }
    }),
    routeProbeFetch: async () => jsonResponse({
      serverId: stored.serverId,
      serverPublicKeyFingerprint: stored.serverPublicKeyFingerprint,
      remoteAccessEnabled: true
    })
  }), error => error instanceof TrustedServerCredentialPublicationError
    && error.cause?.message === "active session store unavailable"
    && error.failClosed === false);
  assert.equal(published, false);
  assert.deepEqual(rollbackModes, ["restore-previous"]);
  assert.equal(current.accessToken, "before-access");
  assert.deepEqual(connections.values.get(`${stored.accountId}\n${stored.serverId}`), before);
});

test("runtime rollback uncertainty retries fail-closed and removes every candidate publication", async () => {
  const stored = record({ previousRoute: undefined });
  const connections = adapter([stored]);
  const previous = { serverId: "server-before", apiBaseUrl: "https://before.example", accessToken: "before-access" };
  let current = previous;
  const rollbackModes = [];

  await assert.rejects(connectTrustedServerRecord(stored, {
    connectionAdapter: connections,
    sessionStore: {
      get: () => current,
      set: session => {
        if (session.accessToken === "server-local-access") throw new Error("active publication failed");
        current = session;
      },
      clear: () => { current = undefined; }
    },
    createLocalClient: store => localClient(store),
    stageCandidate: async () => ({
      publish: async () => { throw new Error("must not publish"); },
      fenceRollback: () => {},
      rollback: async mode => {
        rollbackModes.push(mode ?? "restore-previous");
        if (mode !== "fail-closed") throw new Error("old runtime restore failed");
      }
    }),
    routeProbeFetch: async () => jsonResponse({
      serverId: stored.serverId,
      serverPublicKeyFingerprint: stored.serverPublicKeyFingerprint,
      remoteAccessEnabled: true
    })
  }), error => error instanceof TrustedServerCredentialPublicationError
    && error.failClosed === true
    && error.rollbackFailures.some(failure => failure?.message === "old runtime restore failed"));

  assert.deepEqual(rollbackModes, ["restore-previous", "fail-closed"]);
  assert.equal(current, undefined);
  assert.equal(connections.values.size, 0);
  assert.deepEqual(connections.removed, [[stored.accountId, stored.serverId]]);
});

test("a superseded choice after staging never publishes credentials or durable state", async () => {
  const stored = record({ previousRoute: undefined });
  const connections = adapter([stored]);
  const durableBefore = structuredClone(connections.values.get(`${stored.accountId}\n${stored.serverId}`));
  const save = connections.save;
  let saveCalls = 0;
  connections.save = async item => { saveCalls += 1; await save(item); };
  const previous = { serverId: "server-before", apiBaseUrl: "https://before.example", accessToken: "before-access" };
  const active = createMemorySessionStore(previous);
  const controller = new AbortController();
  const rollbackModes = [];

  await assert.rejects(connectTrustedServerRecord(stored, {
    connectionAdapter: connections,
    sessionStore: active,
    signal: controller.signal,
    createLocalClient: store => localClient(store),
    stageCandidate: async () => {
      controller.abort();
      return {
        publish: async () => { throw new Error("stale candidate must not publish"); },
        fenceRollback: () => {},
        rollback: async mode => { rollbackModes.push(mode ?? "restore-previous"); }
      };
    },
    routeProbeFetch: async () => jsonResponse({
      serverId: stored.serverId,
      serverPublicKeyFingerprint: stored.serverPublicKeyFingerprint,
      remoteAccessEnabled: true
    })
  }), error => error?.name === "AbortError");

  assert.equal(saveCalls, 0);
  assert.deepEqual(rollbackModes, ["restore-previous"]);
  assert.equal(active.get().accessToken, "before-access");
  assert.deepEqual(connections.values.get(`${stored.accountId}\n${stored.serverId}`), durableBefore);
});

test("an uncertain synchronous rollback fence fails closed before candidate credential publication", async () => {
  const stored = record({ previousRoute: undefined });
  const connections = adapter([stored]);
  const active = createMemorySessionStore({
    serverId: stored.serverId,
    apiBaseUrl: stored.currentRoute.url,
    accessToken: "before-access"
  });
  const controller = new AbortController();
  const fenceModes = [];
  const rollbackModes = [];

  await assert.rejects(connectTrustedServerRecord(stored, {
    connectionAdapter: connections,
    sessionStore: active,
    signal: controller.signal,
    createLocalClient: store => localClient(store),
    stageCandidate: async () => {
      controller.abort();
      return {
        publish: async () => { throw new Error("must not publish"); },
        fenceRollback: mode => {
          fenceModes.push(mode ?? "restore-previous");
          if (mode !== "fail-closed") throw new Error("candidate runtime fence was uncertain");
        },
        rollback: async mode => { rollbackModes.push(mode ?? "restore-previous"); }
      };
    },
    routeProbeFetch: async () => jsonResponse({
      serverId: stored.serverId,
      serverPublicKeyFingerprint: stored.serverPublicKeyFingerprint,
      remoteAccessEnabled: true
    })
  }), error => error instanceof TrustedServerCredentialPublicationError
    && error.failClosed === true
    && error.rollbackFailures.some(failure => failure?.message === "candidate runtime fence was uncertain"));

  assert.deepEqual(fenceModes, ["restore-previous", "fail-closed"]);
  assert.deepEqual(rollbackModes, ["fail-closed"]);
  assert.equal(active.get(), undefined);
  assert.equal(connections.values.size, 0);
});

test("resilient selection preserves a typed fatal rollback failure instead of masking it with AbortError", async () => {
  const stored = record({ previousRoute: undefined });
  const connections = adapter([stored]);
  const active = createMemorySessionStore({
    serverId: "server-before",
    apiBaseUrl: "https://before.example",
    accessToken: "before-access"
  });
  const controller = new AbortController();
  const rollbackModes = [];

  await assert.rejects(connectResilientHostedServer(
    { ...testServerIdentity(), id: stored.serverId, name: stored.serverName, preferredAuthMode: "portico" },
    {
      accountId: stored.accountId,
      connectionAdapter: connections,
      hostedClient: {},
      loadTrustedHostedDocumentKeys: async () => {
        await new Promise(resolve => setTimeout(resolve, 10));
        throw new TypeError("Hosted Services unavailable");
      },
      selectionEnvelope: { accountId: stored.accountId, serverId: stored.serverId, profileId: stored.profileId, installationId: "install-1" },
      clientIdentity: { installationId: "install-1", deviceName: "Phone", app: "Portico", platform: "iOS" },
      sessionStore: active,
      signal: controller.signal,
      createLocalClient: store => localClient(store),
      stageCandidate: async () => {
        controller.abort();
        return {
          publish: async () => { throw new Error("must not publish"); },
          fenceRollback: () => {},
          rollback: async mode => {
            rollbackModes.push(mode ?? "restore-previous");
            if (mode !== "fail-closed") throw new Error("stale runtime rollback failed");
          }
        };
      },
      routeProbeFetch: async () => jsonResponse({
        serverId: stored.serverId,
        serverPublicKeyFingerprint: stored.serverPublicKeyFingerprint,
        remoteAccessEnabled: true
      })
    }
  ), error => error instanceof TrustedServerCredentialPublicationError
    && error.cause?.name === "AbortError"
    && error.failClosed === true
    && error.rollbackFailures.some(failure => failure?.message === "stale runtime rollback failed"));

  assert.deepEqual(rollbackModes, ["restore-previous", "fail-closed"]);
  assert.equal(active.get(), undefined);
  assert.equal(connections.values.size, 0);
});

test("a choice superseded during publish is detected immediately and rolls the full transaction back", async () => {
  const stored = record({ previousRoute: undefined });
  const connections = adapter([stored]);
  const durableBefore = structuredClone(connections.values.get(`${stored.accountId}\n${stored.serverId}`));
  const previous = { serverId: "server-before", apiBaseUrl: "https://before.example", accessToken: "before-access" };
  const active = createMemorySessionStore(previous);
  const controller = new AbortController();
  const rollbackModes = [];

  await assert.rejects(connectTrustedServerRecord(stored, {
    connectionAdapter: connections,
    sessionStore: active,
    signal: controller.signal,
    createLocalClient: store => localClient(store),
    stageCandidate: async () => ({
      publish: async () => {
        assert.equal(active.get().accessToken, "server-local-access");
        controller.abort();
      },
      fenceRollback: () => {},
      rollback: async mode => { rollbackModes.push(mode ?? "restore-previous"); }
    }),
    routeProbeFetch: async () => jsonResponse({
      serverId: stored.serverId,
      serverPublicKeyFingerprint: stored.serverPublicKeyFingerprint,
      remoteAccessEnabled: true
    })
  }), error => error instanceof TrustedServerCandidateActivationError
    && error.cause?.name === "AbortError");

  assert.deepEqual(rollbackModes, ["restore-previous"]);
  assert.equal(active.get().accessToken, "before-access");
  assert.deepEqual(connections.values.get(`${stored.accountId}\n${stored.serverId}`), durableBefore);
});

test("the staged runtime fence remains closed throughout an unresolved durable save", async () => {
  const stored = record({ previousRoute: undefined });
  const connections = adapter([stored]);
  let releaseSave;
  const saveBlocked = new Promise(resolve => { releaseSave = resolve; });
  let fenced = false;
  let published = false;
  let observedCredentialFromOldAction;
  const active = createMemorySessionStore({
    serverId: "server-before",
    apiBaseUrl: "https://before.example",
    accessToken: "before-access"
  });
  connections.save = async item => {
    await saveBlocked;
    connections.values.set(`${item.accountId}\n${item.serverId}`, structuredClone(item));
  };

  const pending = connectTrustedServerRecord(stored, {
    connectionAdapter: connections,
    sessionStore: active,
    createLocalClient: store => localClient(store),
    stageCandidate: async () => {
      fenced = true;
      return {
        publish: async () => { published = true; fenced = false; },
        fenceRollback: () => {},
        rollback: async () => { fenced = false; }
      };
    },
    routeProbeFetch: async () => jsonResponse({
      serverId: stored.serverId,
      serverPublicKeyFingerprint: stored.serverPublicKeyFingerprint,
      remoteAccessEnabled: true
    })
  });

  await new Promise(resolve => setImmediate(resolve));
  if (!fenced) observedCredentialFromOldAction = active.get()?.accessToken;
  assert.equal(active.get().accessToken, "server-local-access", "credential publication precedes durable save");
  assert.equal(published, false, "candidate UI remains unpublished while durability is unresolved");
  assert.equal(observedCredentialFromOldAction, undefined, "a fenced old action cannot observe or use candidate credentials");
  releaseSave();
  const result = await pending;
  assert.equal(result.durability, "durable");
  assert.equal(published, true);
});

test("last-connected preference failure is best effort after a successful commit", async () => {
  const stored = record({ previousRoute: undefined });
  const connections = adapter([stored]);
  const active = createMemorySessionStore();
  const result = await connectResilientHostedServer(
    { ...testServerIdentity(), id: stored.serverId, name: stored.serverName, preferredAuthMode: "portico" },
    {
      accountId: stored.accountId,
      connectionAdapter: connections,
      hostedClient: {},
      loadTrustedHostedDocumentKeys: async () => { throw new TypeError("Hosted Services unavailable"); },
      selectionEnvelope: { accountId: stored.accountId, serverId: stored.serverId, profileId: stored.profileId, installationId: "install-1" },
      clientIdentity: { installationId: "install-1", deviceName: "iPhone", app: "Portico", platform: "iOS" },
      sessionStore: active,
      stageCandidate,
      rememberLastConnectedServer: () => { throw new Error("preference store unavailable"); },
      createLocalClient: store => localClient(store),
      routeProbeFetch: async () => jsonResponse({
        serverId: stored.serverId,
        serverPublicKeyFingerprint: stored.serverPublicKeyFingerprint,
        remoteAccessEnabled: true
      })
    }
  );
  assert.equal(result.durability, "durable");
  assert.equal(active.get().accessToken, "server-local-access");
});

test("cached-winner publication veto stops the resilient race without fallback", async () => {
  const stored = record({ previousRoute: undefined });
  const connections = adapter([stored]);
  const durableBefore = structuredClone(connections.values.get(`${stored.accountId}\n${stored.serverId}`));
  const active = createMemorySessionStore({ serverId: "before", apiBaseUrl: "https://before.example", accessToken: "before-access" });
  let publicationCalls = 0;
  let mintCalls = 0;
  let hostedRouteCalls = 0;
  let hostedCompatibilityCalls = 0;
  let hostedSigningKeyCalls = 0;
  await assert.rejects(connectResilientHostedServer(
    { ...testServerIdentity(), id: stored.serverId, name: stored.serverName, preferredAuthMode: "portico" },
    {
      accountId: stored.accountId,
      connectionAdapter: connections,
      hostedClient: {
        checkCompatibility: async () => { hostedCompatibilityCalls += 1; return { apiVersion: "v1" }; },
        routes: async () => { hostedRouteCalls += 1; await new Promise(resolve => setTimeout(resolve, 30)); throw new Error("discovery should remain read only"); },
        documentSigningKeys: async () => { hostedSigningKeyCalls += 1; return {}; },
        porticoSession: async () => { mintCalls += 1; throw new Error("must not mint"); }
      },
      loadTrustedHostedDocumentKeys: async () => { hostedSigningKeyCalls += 1; return ({ [hostedDocumentTestKeyId]: hostedDocumentTestPublicKey }); },
      runtime: trustedAttachmentRuntime,
      selectionEnvelope: { accountId: stored.accountId, serverId: stored.serverId, profileId: stored.profileId, installationId: "install-1" },
      clientIdentity: { installationId: "install-1", deviceName: "TV", app: "Portico", platform: "tvOS" },
      sessionStore: active,
      createLocalClient: store => localClient(store),
      stageCandidate: async () => ({
        publish: async () => { publicationCalls += 1; throw new Error("explicit runtime veto"); },
        fenceRollback: () => {},
        rollback: async () => {}
      }),
      retryDelaysMs: [],
      routeProbeFetch: async () => jsonResponse({
        serverId: stored.serverId,
        serverPublicKeyFingerprint: stored.serverPublicKeyFingerprint,
        remoteAccessEnabled: true
      })
    }
  ), error => error instanceof TrustedServerCandidateActivationError);
  await new Promise(resolve => setTimeout(resolve, 50));
  assert.equal(publicationCalls, 1);
  assert.equal(mintCalls, 0);
  assert.equal(hostedRouteCalls, 0);
  assert.equal(hostedCompatibilityCalls, 0);
  assert.equal(hostedSigningKeyCalls, 0);
  assert.equal(active.get().accessToken, "before-access");
  assert.deepEqual(connections.values.get(`${stored.accountId}\n${stored.serverId}`), durableBefore);
});

test("Hosted recovery publication veto stops direct-first connection after cached failure", async () => {
  const stored = record({ previousRoute: undefined });
  const connections = adapter([stored]);
  const durableBefore = structuredClone(connections.values.get(`${stored.accountId}\n${stored.serverId}`));
  const active = createMemorySessionStore({ serverId: "before", apiBaseUrl: "https://before.example", accessToken: "before-access" });
  const revoked = [];
  let publicationCalls = 0;
  const document = signedRouteDocument({
    serverId: stored.serverId,
    serverName: stored.serverName,
    assignedHostname: "fresh.direct.getportico.tv",
    issuedAt: "2026-07-14T11:59:00.000Z",
    expiresAt: "2026-07-14T12:05:00.000Z",
    serverPublicKeyFingerprint: stored.serverPublicKeyFingerprint,
    authModes: ["portico"],
    certificate: { status: "valid" },
    membership: { role: "owner" },
    routes: [{ type: "public_direct", url: "https://fresh.direct.getportico.tv:32500", quality: "reachable" }]
  });
  await assert.rejects(connectResilientHostedServer(
    { ...testServerIdentity(), id: stored.serverId, name: stored.serverName, preferredAuthMode: "portico" },
    {
      accountId: stored.accountId,
      connectionAdapter: connections,
      hostedClient: {
        checkCompatibility: async () => ({ apiVersion: "v1" }),
        routes: async () => document,
        porticoSession: async () => ({ accessToken: "bootstrap-access", accessExpiresAt: "2026-07-14T12:01:00.000Z" }),
        reportRouteFailure: async () => ({ ok: true, matched: true })
      },
      loadTrustedHostedDocumentKeys: async () => ({ [hostedDocumentTestKeyId]: hostedDocumentTestPublicKey }),
      runtime: trustedAttachmentRuntime,
      selectionEnvelope: { accountId: stored.accountId, serverId: stored.serverId, profileId: stored.profileId, installationId: "install-1" },
      clientIdentity: { installationId: "install-1", deviceName: "TV", app: "Portico", platform: "tvOS" },
      sessionStore: active,
      createLocalClient: store => localClient(store, {
        acceptPorticoSessionCredentials: async () => {
          const provisional = store.get();
          store.set({ ...provisional, bootstrapAccessToken: undefined, accessToken: "new-access", refreshToken: "new-refresh" });
          return {
            tokenType: "Bearer",
            authority: "hosted",
            accountId: stored.accountId,
            serverId: stored.serverId,
            profileId: stored.profileId,
            authorizationRevision: "policy-1",
            accessToken: "new-access",
            accessExpiresAt: "2026-07-14T12:05:00.000Z",
            refreshToken: "new-refresh",
            refreshExpiresAt: "2027-01-01T00:00:00.000Z",
            user: { id: "viewer-1" },
            device: { id: "device-1" }
          };
        },
        checkProductContractCompatibility: async () => ({ apiVersion: "v1" }),
        revokeNativeSession: async token => { revoked.push(token); return { ok: true }; }
      }),
      stageCandidate: async () => ({
        publish: async () => { publicationCalls += 1; throw new Error("explicit runtime veto"); },
        fenceRollback: () => {},
        rollback: async () => {}
      }),
      runtime: trustedAttachmentRuntime,
      routeProbeFetch: async input => {
        if (String(input).startsWith(stored.currentRoute.url)) throw new TypeError("remembered route is offline");
        return jsonResponse({
          serverId: stored.serverId,
          serverPublicKeyFingerprint: stored.serverPublicKeyFingerprint,
          remoteAccessEnabled: true
        });
      },
      retryDelaysMs: [],
      now: () => new Date("2026-07-14T12:00:00.000Z")
    }
  ), error => error instanceof TrustedServerCandidateActivationError);
  await new Promise(resolve => setTimeout(resolve, 60));
  assert.equal(publicationCalls, 1);
  assert.deepEqual(revoked, ["new-refresh"]);
  assert.equal(active.get().accessToken, "before-access");
  assert.deepEqual(connections.values.get(`${stored.accountId}\n${stored.serverId}`), durableBefore);
});

test("generic auth errors and transient failures are not treated as revocation", () => {
  assert.equal(isTerminalServerAuthorizationFailure(new ApiError(401, "invalid_refresh_token", "Expired.")), false);
  assert.equal(isTerminalServerAuthorizationFailure(new ApiError(503, "temporarily_unavailable", "Offline.")), false);
  for (const code of ["credential_revoked", "refresh_reused", "account_deleted", "profile_deleted", "membership_removed"]) {
    assert.equal(isTerminalServerAuthorizationFailure(new ApiError(401, code, "Revoked.")), true, code);
  }
  assert.equal(isTerminalServerAuthorizationFailure(new ApiError(403, "membership_inactive", "Revoked.")), true);
  assert.equal(isTerminalServerAuthorizationFailure(new TypeError("Network request failed")), false);
});
