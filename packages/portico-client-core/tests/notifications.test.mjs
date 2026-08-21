import assert from "node:assert/strict";
import test from "node:test";

import {
  applyNotificationInvalidation,
  normalizeNotificationReceiptMutation,
  normalizeViewerFeedbackAdminUpdate,
  normalizeViewerFeedbackSubmission,
  notificationActionIsSafe,
  notificationActionSemantics,
  parseNotificationPage,
  parseNotificationReceiptResult,
  parseNotificationRecipient,
  parseOwnerNotificationRecipientDirectory,
  parseViewerFeedbackCapabilities,
  parseViewerFeedbackPage,
  parseViewerFeedbackReceipt,
  parseViewerNotification
} from "../dist/notifications.js";

const recipient = { authority: "hosted", accountId: "account", serverId: "server", profileId: "profile", audience: "profile" };

test("feedback accepts bounded typed and extensible client context", () => {
  const feedback = normalizeViewerFeedbackSubmission({
    version: "v1", kind: "playback", category: "buffering", message: "Stopped twice.",
    context: { playbackSessionId: "session", mediaId: "movie", deviceClass: "mobile", platform: "future-mobile-os", appVersion: "1.0" }
  });
  assert.equal(feedback.context.playbackSessionId, "session");
  assert.equal(feedback.context.platform, "future-mobile-os");
  assert.throws(() => normalizeViewerFeedbackSubmission({ ...feedback, context: { deviceClass: "mobile", platform: "ios", appVersion: "1.0", rawLogs: "secret" } }), /context/);
  assert.throws(() => normalizeViewerFeedbackSubmission({ ...feedback, context: { ...feedback.context, errorCategory: "network.timeout" } }), /context/);
  assert.throws(() => normalizeViewerFeedbackSubmission({ ...feedback, version: "future" }), /unsupported/);
});

test("feedback kind/category and identity requirements match the published contract", () => {
  assert.throws(() => normalizeViewerFeedbackSubmission({ version: "v1", kind: "media", category: "wrong-video", message: "", context: { deviceClass: "web", platform: "web", appVersion: "1" } }), /mediaId/);
  assert.throws(() => normalizeViewerFeedbackSubmission({ version: "v1", kind: "playback", category: "wont-play", message: "", context: { deviceClass: "television", platform: "tvos", appVersion: "1" } }), /playbackSessionId/);
  assert.throws(() => normalizeViewerFeedbackSubmission({ version: "v1", kind: "quality", category: "buffering", message: "", context: { mediaId: "m", deviceClass: "mobile", platform: "ios", appVersion: "1" } }), /does not match/);
  assert.throws(() => normalizeViewerFeedbackSubmission({ version: "v1", kind: "general", category: "other", message: "   ", context: { deviceClass: "web", platform: "web", appVersion: "1" } }), /message/);
});

test("notification actions reject malformed kinds, unknown wording, URLs, and commands", () => {
  assert.equal(notificationActionIsSafe({ id: "open", labelMessageId: "action.open", kind: "navigate", target: "media.detail", parameters: { mediaId: "1" } }), true);
  assert.equal(notificationActionIsSafe({ id: "bad", labelMessageId: "action.open", kind: "navigate", target: "media.detail", parameters: {} }), false);
  assert.equal(notificationActionIsSafe({ id: "bad", labelMessageId: "action.open", kind: "navigate", target: "https://evil.example", parameters: {} }), false);
  assert.equal(notificationActionIsSafe({ id: "bad", labelMessageId: "action.open", kind: "not-a-kind", target: "download.retry", parameters: { downloadId: "1" } }), false);
  assert.equal(notificationActionIsSafe({ id: "bad", labelMessageId: "action.not-in-catalog", kind: "command", target: "download.retry", parameters: { downloadId: "1" } }), false);
  assert.equal(notificationActionIsSafe({ id: "bad", labelMessageId: "action.open", kind: "command", target: "server.delete", parameters: {} }), false);
  assert.equal(notificationActionIsSafe({ id: "bad", labelMessageId: "action.open", kind: "navigate", target: "downloads", parameters: { url: "https://evil.example" } }), false);
  assert.equal(notificationActionIsSafe({ id: "bad", labelMessageId: "action.open", kind: "navigate", target: "media.detail", parameters: { mediaId: "   " } }), false);
  assert.throws(() => { notificationActionSemantics["media.detail"].requiredParameters.push("url"); }, TypeError);
});

function notification(overrides = {}) {
  return {
    id: "notice", recipient, kind: "membership.changed", severity: "informational",
    messageId: "notification.empty", iconId: "status.notification", interpolation: {}, actions: [],
    createdAt: "2026-07-16T12:00:00Z", ...overrides
  };
}

test("notification parsing enforces Product Language and recipient identity", () => {
  assert.equal(parseViewerNotification(notification()).messageId, "notification.empty");
  assert.throws(() => parseViewerNotification(notification({ messageId: "notification.missing" })), /Product Language/);
  assert.throws(() => parseViewerNotification(notification({ iconId: "status.missing" })), /Product Language/);
  const page = parseNotificationPage({ recipient, items: [notification()], unreadCount: 1, revision: 2, pageInfo: { nextCursor: null, hasMore: false, total: 1 } }, recipient);
  assert.equal(page.items.length, 1);
  assert.equal(page.pageInfo.total, 1);
  assert.throws(() => parseNotificationPage({ recipient, items: [notification({ recipient: { ...recipient, profileId: "other" } })], unreadCount: 1, revision: 2, pageInfo: { nextCursor: null, hasMore: false } }, recipient), /another recipient/);
  assert.throws(() => parseNotificationPage({ recipient, items: [], unreadCount: 0, revision: 2, pageInfo: { nextCursor: null, hasMore: true } }, recipient), /requires a cursor/);
  assert.equal(parseViewerNotification(notification({ kind: "server.message", content: { title: "From your server owner", body: "The library will be offline tonight." } })).content.body, "The library will be offline tonight.");
  assert.throws(() => parseViewerNotification(notification({ kind: "server.message" })), /requires private content/);
  assert.throws(() => parseNotificationRecipient({ ...recipient, profileId: undefined }), /requires a profile/);
  const accountAdmin = parseNotificationRecipient({ authority: "local", accountId: "account-1", serverId: "server-1", audience: "account-admin" });
  assert.equal(accountAdmin.profileId, undefined);
  assert.throws(() => parseNotificationRecipient({ ...accountAdmin, profileId: "primary-profile" }), /cannot be profile-scoped/);
});

test("notification invalidations are strictly content-free", () => {
  const event = { version: "v1", kind: "notifications.invalidated", occurredAt: "2026-07-16T12:01:00Z" };
  assert.deepEqual(applyNotificationInvalidation(event), event);
  assert.equal(applyNotificationInvalidation({ ...event, revision: 6 }), undefined);
  assert.equal(applyNotificationInvalidation({ ...event, unreadCount: 1 }), undefined);
  assert.equal(applyNotificationInvalidation({ ...event, recipient }), undefined);
  assert.equal(applyNotificationInvalidation({ ...event, privateContent: "leak" }), undefined);
});

test("notification receipt batches are versioned, recipient-bound, and revision-aware", () => {
  const mutation = normalizeNotificationReceiptMutation({
    version: "v1", recipient, notificationIds: ["notice-1", "notice-2"], action: "mark-read", expectedRevision: 7
  }, recipient);
  assert.equal(mutation.notificationIds.length, 2);
  assert.throws(() => normalizeNotificationReceiptMutation({ ...mutation, notificationIds: ["notice-1", "notice-1"] }), /duplicates/);
  assert.throws(() => normalizeNotificationReceiptMutation({ ...mutation, recipient: { ...recipient, profileId: "other" } }, recipient), /does not match/);
  const result = parseNotificationReceiptResult({
    recipient,
    receipts: [{ notificationId: "notice-1", readAt: "2026-07-16T12:02:00Z", archivedAt: null }],
    unreadCount: 1,
    revision: 8
  }, recipient);
  assert.equal(result.receipts[0].readAt, "2026-07-16T12:02:00Z");
});

function feedbackRecord(overrides = {}) {
  return {
    id: "feedback-1",
    reporter: { authority: "hosted", membershipId: "membership-1", accountName: "Taylor's account" },
    kind: "playback",
    category: "buffering",
    status: "new",
    message: "Playback stopped twice.",
    diagnostics: {
      mediaId: "media-1", playbackDecisionId: "decision-1", selectedVersionId: "version-1",
      deliveryReason: "Network capacity changed.", deviceClass: "mobile", platform: "ios", appVersion: "1.2.3",
      occurredAt: "2026-07-16T12:00:00Z", errorCategory: "network.timeout"
    },
    duplicateCount: 1,
    submittedAt: "2026-07-16T12:00:00Z",
    updatedAt: "2026-07-16T12:00:00Z",
    revision: 3,
    ...overrides
  };
}

test("feedback capabilities, receipts, admin records, and responses stay bounded and private", () => {
  const capabilities = parseViewerFeedbackCapabilities({
    version: "v1", enabled: true, allowedKinds: ["general", "playback", "media", "quality"], messageMaxLength: 1000, retentionDays: 180
  });
  assert.equal(capabilities.retentionDays, 180);
  assert.deepEqual(parseViewerFeedbackReceipt({ id: "feedback-1", status: "new", submittedAt: "2026-07-16T12:00:00Z", duplicateOfId: "feedback-0" }).duplicateOfId, "feedback-0");
  const page = parseViewerFeedbackPage({
    items: [feedbackRecord({ ownerResponse: { message: "I replaced the file.", respondedAt: "2026-07-16T13:00:00Z" } })],
    pageInfo: { nextCursor: null, hasMore: false, total: 1 },
    statusCounts: { new: 1, read: 0, resolved: 0, dismissed: 0 }
  });
  assert.equal(page.items[0].ownerResponse.message, "I replaced the file.");
  assert.throws(() => parseViewerFeedbackPage({
    items: [feedbackRecord({ diagnostics: { ...feedbackRecord().diagnostics, rawLogs: "secret" } })],
    pageInfo: { nextCursor: null, hasMore: false }, statusCounts: { new: 1, read: 0, resolved: 0, dismissed: 0 }
  }), /diagnostics/);
  assert.deepEqual(normalizeViewerFeedbackAdminUpdate({ version: "v1", expectedRevision: 3, status: "resolved", responseMessage: "Fixed." }), {
    version: "v1", expectedRevision: 3, status: "resolved", responseMessage: "Fixed."
  });
  assert.throws(() => normalizeViewerFeedbackAdminUpdate({ version: "v1", expectedRevision: 3 }), /empty/);
  assert.throws(() => normalizeViewerFeedbackAdminUpdate({ version: "v1", expectedRevision: 3, responseMessage: "\u0000secret" }), /invalid/);
});

test("owner notification recipients are exact, bounded, deduplicated, and never include hosted profiles", () => {
  const directory = parseOwnerNotificationRecipientDirectory({
    profiles: [{ authority: "local", audience: "profile", accountId: "account-1", profileId: "child-1", accountName: "Rivera household", profileName: "Kids" }],
    accountAdmins: [{ authority: "hosted", audience: "account-admin", accountId: "account-2", accountName: "Sam Rivera" }]
  });
  assert.equal(directory.profiles[0].profileName, "Kids");
  assert.throws(() => parseOwnerNotificationRecipientDirectory({
    ...directory,
    profiles: [{ ...directory.profiles[0], authority: "hosted" }]
  }), /profile notification recipient/);
  assert.throws(() => parseOwnerNotificationRecipientDirectory({
    ...directory,
    accountAdmins: [directory.accountAdmins[0], directory.accountAdmins[0]]
  }), /duplicate/);
  assert.throws(() => parseOwnerNotificationRecipientDirectory({ ...directory, internalPath: "/private" }), /fields/);
});
