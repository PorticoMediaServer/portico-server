import assert from "node:assert/strict";
import test from "node:test";

import { createPorticoClient } from "../dist/index.js";

function response(body, status = 200) {
  return new Response(body === undefined ? null : JSON.stringify(body), {
    status,
    headers: body === undefined ? undefined : { "Content-Type": "application/json" }
  });
}

const policy = {
  version: "v1",
  maximumAgeRating: null,
  allowUnrated: true,
  blockedLabels: [],
  allowDownloads: true,
  allowLiveTV: true,
  allowDvr: true,
  allowWatchWithFriends: true,
  allowFeedback: true
};

const primaryProfile = {
  id: "primary",
  name: "Main",
  isPrimary: true,
  isAccountAdmin: true,
  hasPIN: true,
  pinRevision: 2,
  sortOrder: 0,
  policy
};

const managedDirectory = {
  authority: "local",
  accountId: "account-1",
  serverId: "server-1",
  profilesAllowed: true,
  profiles: [primaryProfile],
  canManage: true
};

test("viewer preferences and local account-profile methods use canonical routes, generated bodies, and proof headers", async () => {
  const calls = [];
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    transport: {
      fetch: async (input, init) => {
        const url = new URL(String(input));
        calls.push({ url, init, body: init.body ? JSON.parse(init.body) : undefined });
        const key = `${init.method} ${url.pathname}`;
        if (key === "GET /api/preferences") return response({ identity: { serverId: "server-1" } });
        if (key === "PATCH /api/preferences/profile-server") return response({ version: "v1", revision: 4, values: {} });
        if (key === "POST /api/preferences/profile-activation") return response({ version: "v1", revision: 5, values: { rememberAccount: true, profileSelection: "last-used", lastProfileId: "primary" } });
        if (key === "GET /api/account/profiles" || key === "PUT /api/account/profiles/order") return response(managedDirectory);
        if (key === "POST /api/account/profile-admin-proofs") return response({ token: "proof-token", expiresAt: "2026-07-16T12:05:00Z" }, 201);
        if (key === "POST /api/account/profiles" || key === "PATCH /api/account/profiles/child%2F1") return response(primaryProfile, key.startsWith("POST") ? 201 : 200);
        if (key === "POST /api/account/profile-trusts") return response({
          version: "v1", purpose: "automatic-profile-selection", token: "trust-token", authority: "local",
          accountId: "account-1", serverId: "server-1", installationId: "installation-0001",
          profileId: "primary", pinRevision: 2, expiresAt: "2026-10-16T12:00:00Z"
        }, 201);
        if (key === "POST /api/account/profile-trusts/redeem") return response(undefined, 204);
        if (init.method === "DELETE" || init.method === "PUT") return response(undefined, 204);
        throw new Error(`Unexpected request ${key}`);
      }
    }
  });

  const signal = new AbortController().signal;
  assert.equal((await client.viewerPreferenceBundle({ deviceClass: "web", installationId: "installation-0001" }, { signal })).identity.serverId, "server-1");
  await client.patchViewerPreferenceDocument("profile-server", { deviceClass: "web", installationId: "installation-0001" }, {
    version: "v1", expectedRevision: 3, changes: { playback: { autoplayNext: false } }
  }, { signal });
  assert.equal((await client.recordViewerProfileActivation({ version: "v1", expectedRevision: 4 }, { signal })).values.lastProfileId, "primary");
  assert.throws(() => client.patchViewerPreferenceDocument("account-server-installation", { deviceClass: "web", installationId: "installation-0001" }, {
    version: "v1", expectedRevision: 4, changes: { lastProfileId: "sibling" }
  }, { signal }), /authoritative profile activation/);
  assert.equal((await client.accountProfiles({ signal })).canManage, true);
  assert.equal((await client.createProfileAdministrationProof({ pin: "2468" }, { signal })).token, "proof-token");
  await client.createAccountProfile({ name: "Kids", policy }, "  admin-proof  ", { signal });
  await client.updateAccountProfile("child/1", { name: "Teen" }, "admin-proof", { signal });
  await client.reorderAccountProfiles({ profileIds: ["primary"] }, "admin-proof", { signal });
  await client.setAccountProfilePIN("child/1", { pin: "8642", password: "current-password" }, "admin-proof", { signal });
  await client.clearAccountProfilePIN("child/1", { password: "current-password" }, "admin-proof", { signal });
  await client.deleteAccountProfile("child/1", "admin-proof", { signal });
  assert.equal((await client.createAutomaticProfileTrust({ installationId: "installation-0001" }, { signal })).purpose, "automatic-profile-selection");
  await client.redeemAutomaticProfileTrust({ token: "trust-token" }, { signal });
  await client.revokeAutomaticProfileTrusts({ installationId: "installation-0001" }, { signal });

  assert.equal(calls[0].url.searchParams.get("deviceClass"), "web");
  assert.equal(calls[0].url.searchParams.get("installationId"), "installation-0001");
  assert.deepEqual(calls[1].body, { version: "v1", expectedRevision: 3, changes: { playback: { autoplayNext: false } } });
  assert.equal(calls.find(call => call.init.method === "POST" && call.url.pathname === "/api/account/profiles").init.headers["X-Portico-Profile-Admin-Proof"], "admin-proof");
  assert.deepEqual(calls.find(call => call.init.method === "PUT" && call.url.pathname.endsWith("/pin")).body, { pin: "8642", password: "current-password" });
  assert.deepEqual(calls.find(call => call.init.method === "DELETE" && call.url.pathname.endsWith("/pin")).body, { password: "current-password" });
  assert.ok(calls.some(call => call.url.pathname === "/api/account/profiles/child%2F1"));
  assert.equal(calls.every(call => call.init.signal instanceof AbortSignal), true);
  await assert.rejects(() => client.createAccountProfile({ name: "Kids", policy }, "   "), /proof (?:is required|token is invalid)/);
});

const recipient = {
  authority: "local",
  accountId: "account-1",
  serverId: "server-1",
  audience: "profile",
  profileId: "primary"
};

function notification(overrides = {}) {
  return {
    id: "notice-1",
    recipient,
    kind: "membership.changed",
    severity: "informational",
    messageId: "notification.membership-changed",
    iconId: "status.server",
    interpolation: {},
    actions: [],
    createdAt: "2026-07-16T12:00:00Z",
    ...overrides
  };
}

function ownerFeedback(overrides = {}) {
  return {
    id: "feedback-1",
    reporter: { authority: "local", accountId: "account-1", accountName: "Main account" },
    kind: "general",
    category: "other",
    status: "new",
    message: "Please add another library.",
    diagnostics: { deviceClass: "web", platform: "web", appVersion: "1.0", occurredAt: "2026-07-16T12:00:00Z" },
    duplicateCount: 0,
    submittedAt: "2026-07-16T12:00:00Z",
    updatedAt: "2026-07-16T12:00:00Z",
    revision: 1,
    ...overrides
  };
}

test("notification, feedback, and owner-intake methods preserve privacy, pagination, receipts, and mutation invalidation", async () => {
  const calls = [];
  const tags = [];
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    onMutation(nextTags) { tags.push(nextTags); },
    transport: {
      fetch: async (input, init) => {
        const url = new URL(String(input));
        const body = init.body ? JSON.parse(init.body) : undefined;
        calls.push({ url, init, body });
        const key = `${init.method} ${url.pathname}`;
        if (key === "GET /api/notifications") return response({ recipient, items: [notification()], unreadCount: 1, revision: 4, pageInfo: { nextCursor: null, hasMore: false, total: 1 } });
        if (key === "POST /api/notifications/receipts") return response({
          recipient, receipts: [{ notificationId: "notice-1", readAt: "2026-07-16T12:01:00Z", archivedAt: null }], unreadCount: 0, revision: 5
        });
        if (key === "POST /api/notifications/read-all") return response({ recipient, unreadCount: 0, revision: 6 });
        if (key === "GET /api/feedback/capabilities") return response({ version: "v1", enabled: true, allowedKinds: ["general", "media"], messageMaxLength: 1000, retentionDays: 180 });
        if (key === "POST /api/feedback") return response({ id: "feedback-1", status: "new", submittedAt: "2026-07-16T12:00:00Z" }, 201);
        if (key === "GET /api/admin/viewer-feedback") return response({
          items: [ownerFeedback()], pageInfo: { nextCursor: null, hasMore: false, total: 1 },
          statusCounts: { new: 1, read: 0, resolved: 0, dismissed: 0 }
        });
        if (key === "GET /api/admin/viewer-notification-recipients") return response({
          profiles: [{ authority: "local", audience: "profile", accountId: "account-1", profileId: "primary", accountName: "Main account", profileName: "Main" }],
          accountAdmins: [{ authority: "local", audience: "account-admin", accountId: "account-1", accountName: "Main account" }]
        });
        if (key === "PATCH /api/admin/viewer-feedback/feedback%2F1") return response(ownerFeedback({ status: "resolved", revision: 2 }));
        if (key === "POST /api/admin/viewer-notifications") return response(notification({
          kind: "server.message", messageId: "notification.server-message",
          content: { title: "Server notice", body: "The library will be offline tonight." }
        }), 201);
        throw new Error(`Unexpected request ${key}`);
      }
    }
  });

  const signal = new AbortController().signal;
  assert.equal((await client.viewerNotifications({ audience: "profile", cursor: "next page", limit: 20, includeArchived: true }, { signal })).items.length, 1);
  const receipt = await client.updateViewerNotificationReceipts({
    version: "v1", recipient, notificationIds: ["notice-1"], action: "mark-read", expectedRevision: 4
  }, { audience: "profile" }, { signal });
  assert.equal(receipt.revision, 5);
  assert.equal((await client.markAllViewerNotificationsRead({ audience: "profile" }, { signal })).unreadCount, 0);
  assert.equal((await client.viewerFeedbackCapabilities({ signal })).retentionDays, 180);
  assert.equal((await client.submitViewerFeedback({
    version: "v1", kind: "general", category: "other", message: "Please add another library.",
    context: { deviceClass: "web", platform: "web", appVersion: "1.0" }
  }, { signal })).id, "feedback-1");
  assert.equal((await client.ownerViewerFeedback({ status: "new", cursor: "owner next", limit: 25 }, { signal })).items[0].reporter.accountName, "Main account");
  assert.equal((await client.ownerViewerNotificationRecipients({ signal })).profiles[0].profileName, "Main");
  assert.equal((await client.updateOwnerViewerFeedback("feedback/1", { version: "v1", expectedRevision: 1, status: "resolved" }, { signal })).status, "resolved");
  assert.equal((await client.createOwnerViewerNotice({ audience: "profile", profileId: "primary", severity: "warning", message: "The library will be offline tonight." }, { signal })).content.body, "The library will be offline tonight.");

  assert.equal(calls[0].url.searchParams.get("cursor"), "next page");
  assert.equal(calls[0].url.searchParams.get("includeArchived"), "true");
  assert.deepEqual(calls[1].body.notificationIds, ["notice-1"]);
  assert.equal(calls[5].url.searchParams.get("status"), "new");
  assert.ok(calls.some(call => call.url.pathname === "/api/admin/viewer-feedback/feedback%2F1"));
  assert.deepEqual(tags, [
    ["notifications"], ["notifications"], ["feedback"], ["feedback", "notifications"], ["notifications"]
  ]);
  assert.equal(calls.every(call => call.init.signal instanceof AbortSignal), true);
});

test("viewer notification invalidations stream through the portable event adapter", async () => {
  const recipient = { authority: "local", accountId: "account-1", serverId: "server-1", audience: "profile", profileId: "primary" };
  const events = [];
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    transport: { fetch: async (input) => {
      assert.equal(new URL(String(input)).searchParams.get("audience"), "profile");
      return new Response(null, { status: 200, headers: { "Content-Type": "text/event-stream" } });
    } },
    eventStream: { async *read() {
      yield `event: notification-invalidation\ndata: ${JSON.stringify({
        version: "v1", kind: "notifications.invalidated", occurredAt: "2026-07-16T12:00:00Z"
      })}\n\n`;
      yield `data: ${JSON.stringify({
        version: "v1", kind: "notifications.invalidated", recipient, occurredAt: "2026-07-16T12:01:00Z"
      })}\n\n`;
    } }
  });
  await client.streamViewerNotificationInvalidations({ audience: "profile" }, new AbortController().signal, event => events.push(event));
  assert.deepEqual(events, [{ version: "v1", kind: "notifications.invalidated", occurredAt: "2026-07-16T12:00:00Z" }]);
});
