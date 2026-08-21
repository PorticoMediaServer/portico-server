import assert from "node:assert/strict";
import test from "node:test";
import { createPorticoClient } from "../dist/index.js";

function jsonResponse(body, status = 201) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" }
  });
}

function nativeCredentials() {
  return {
    tokenType: "Bearer",
    accessToken: "local-access",
    accessExpiresAt: "2026-07-16T21:15:00Z",
    refreshToken: "local-refresh",
    refreshExpiresAt: "2027-01-16T21:00:00Z",
    authority: "local",
    accountId: "account-1",
    serverId: "server-1",
    profileId: "profile-1",
    authorizationRevision: "1",
    user: { id: "account-1", profileId: "profile-1" },
    device: { id: "device-1" }
  };
}

test("local pre-profile methods preserve endpoint order and keep passwords, PINs, and grants in request bodies", async () => {
  const calls = [];
  let saved;
  const responses = [
    {
      accountAuthenticationToken: "account-proof",
      expiresAt: "2026-07-16T21:05:00Z",
      directory: {
        authority: "local", accountId: "account-1", serverId: "server-1", profilesAllowed: true,
        profiles: [{
          id: "profile-1", name: "Primary", isPrimary: true, isAccountAdmin: true, hasPIN: true,
          pinRevision: 2, sortOrder: 0,
          policy: {
            version: "v1", maximumAgeRating: 13, allowUnrated: false, blockedLabels: [],
            allowDownloads: false, allowLiveTV: true, allowDvr: false, allowWatchWithFriends: true, allowFeedback: true
          }
        }]
      }
    },
    {
      token: "selection-grant", authority: "local", accountId: "account-1", serverId: "server-1",
      profileId: "profile-1", pinRevision: 2, installationId: "installation-0001", expiresAt: "2026-07-16T21:02:00Z"
    },
    nativeCredentials()
  ];
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    credentialAdapter: {
      save: async (session) => { saved = structuredClone(session); },
      clear: async () => {}
    },
    transport: {
      fetch: async (input, init) => {
        calls.push({ url: String(input), method: init.method, body: JSON.parse(init.body) });
        return jsonResponse(responses.shift());
      }
    }
  });

  const authentication = await client.authenticateLocalProfileAccount({
    login: "viewer@example.test",
    password: "correct horse battery staple",
    purpose: "native",
    installationId: "installation-0001",
    deviceName: "iPhone",
    app: "Portico",
    platform: "ios"
  });
  const selection = await client.selectLocalProfile({ accountAuthenticationToken: "account-proof", profileId: "profile-1", pin: "2468" });
  await client.createNativeProfileSession({
    selectionGrant: "selection-grant",
    installationId: "installation-0001",
    deviceName: "iPhone",
    app: "Portico",
    platform: "ios"
  });

  assert.deepEqual(calls.map(call => call.url), [
    "https://server.example/api/auth/profile-authentications/local",
    "https://server.example/api/auth/profile-selections/local",
    "https://server.example/api/auth/profile-sessions/native"
  ]);
  assert.equal(calls.every(call => !call.url.includes("correct") && !call.url.includes("2468") && !call.url.includes("selection-grant")), true);
  assert.equal(calls[0].body.password, "correct horse battery staple");
  assert.equal(calls[1].body.pin, "2468");
  assert.equal(calls[2].body.selectionGrant, "selection-grant");
  assert.equal(authentication.directory.profiles[0].hasPIN, true);
  assert.equal(selection.authority, "local");
  assert.equal(selection.serverId, authentication.directory.serverId);
  assert.equal(selection.installationId, "installation-0001");
  assert.equal(saved.accessToken, "local-access");
  assert.equal(saved.refreshToken, "local-refresh");
});

test("browser profile finalization does not persist a native credential family", async () => {
  let saved = false;
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    credentialAdapter: {
      save: async () => { saved = true; },
      clear: async () => {}
    },
    transport: {
      fetch: async (input, init) => {
        assert.equal(String(input), "https://server.example/api/auth/profile-sessions/browser");
        assert.deepEqual(JSON.parse(init.body), { selectionGrant: "browser-grant", rememberOnBrowser: true });
        return jsonResponse({ authenticated: true, user: { id: "account-1", profileId: "profile-1" } });
      }
    }
  });
  await client.createBrowserProfileSession({ selectionGrant: "browser-grant", rememberOnBrowser: true });
  assert.equal(saved, false);
});
