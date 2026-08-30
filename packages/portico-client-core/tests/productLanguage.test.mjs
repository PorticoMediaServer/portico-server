import assert from "node:assert/strict";
import test from "node:test";

import {
  interpolateProductText,
  knownProductMessageId,
  knownSemanticIconId,
  productLanguageCatalog,
  productMessage,
  productMessageIdForProblemCode,
  resolveProductProblem,
  safeProductMessage,
  semanticIcon
} from "../dist/productLanguage.js";
import { parseUniqueJSON } from "../scripts/parse-unique-json.mjs";
import { hostedRouteErrorMessage, porticoSessionErrorMessage } from "../dist/hostedServerConnection.js";
import { ApiError } from "../dist/client.js";

test("Product Language generation rejects duplicate keys at any depth", () => {
  assert.throws(
    () => parseUniqueJSON('{"messages":{"same":{},"same":{}}}', "fixture"),
    /duplicate key "same"/
  );
  assert.deepEqual(parseUniqueJSON('{"messages":{"one":{}},"icons":{}}'), {messages: {one: {}}, icons: {}});
  assert.throws(() => parseUniqueJSON('{\u00a0"messages":{}}'), /expected a string/);
});

test("catalog exposes one versioned English product vocabulary", () => {
  assert.equal(productLanguageCatalog.locale, "en-US");
  assert.equal(productLanguageCatalog.fallbackLocale, "en-US");
  assert.equal(productLanguageCatalog.revision, "v1");
  assert.equal(productLanguageCatalog.iconFamily, "lucide");
});

test("reorder and pagination actions keep their canonical meanings", () => {
  assert.equal(semanticIcon("action.move-up").glyph, "ArrowUp");
  assert.equal(semanticIcon("action.move-down").glyph, "ArrowDown");
  assert.equal(productMessage("action.load-more").text, "Load more");
  assert.equal(productMessage("action.load-more").icon, undefined);
});

test("problem codes resolve to stable shared wording and semantic icons", () => {
  const presentation = resolveProductProblem({ code: "server_not_found", status: 404 }, { serverName: "Home Server" });
  assert.equal(presentation.id, "problem.server-unavailable");
  assert.equal(presentation.title, "Server unavailable");
  assert.equal(presentation.body, "Portico couldn't reach Home Server. Your Portico Account remains signed in.");
  assert.equal(presentation.icon, "status.server");
  assert.equal(semanticIcon(presentation.icon).glyph, "ServerOff");
});

test("discovery failures resolve to shared navigation, search, detail, and feedback states", () => {
  assert.equal(resolveProductProblem({ code: "navigation_unavailable", status: 503 }).id, "navigation.unavailable");
  assert.equal(resolveProductProblem({ code: "search_history_unavailable", status: 503 }).id, "search.offline");
  assert.equal(resolveProductProblem({ code: "media_children_unavailable", status: 503 }).id, "media.detail-unavailable");
  assert.equal(resolveProductProblem({ code: "hosted_busy", status: 503 }).id, "problem.hosted-busy");
  assert.equal(resolveProductProblem({ code: "rate_limit_unavailable", status: 503 }).id, "problem.hosted-busy");
  assert.equal(resolveProductProblem({ code: "feedback_not_allowed", status: 403 }).id, "feedback.disabled");
  assert.equal(resolveProductProblem({ code: "feedback_rate_limited", status: 429 }).id, "feedback.rate-limited");
});

test("unknown API details do not replace normalized product copy", () => {
  const presentation = resolveProductProblem({ code: "future_private_error", status: 500, details: { path: "/secret/media" } });
  assert.equal(presentation.id, "problem.request-failed");
  assert.doesNotMatch(presentation.body, /secret|media/i);
});

test("malformed problem identifiers fail closed to reviewed copy without throwing", () => {
  const malformed = resolveProductProblem({
    code: { private: "filesystem path" },
    messageId: ["auth.session-expired"],
    status: 500,
    details: { path: "/secret/media" }
  });
  assert.equal(malformed.id, "problem.request-failed");
  assert.doesNotMatch(malformed.body, /filesystem|secret|media|session/i);
  assert.equal(productMessageIdForProblemCode({ code: "server_not_found" }), undefined);
  assert.equal(knownProductMessageId(["auth.session-expired"]), undefined);
  assert.equal(knownSemanticIconId({ icon: "status.notification" }), undefined);
});

test("unknown runtime message identifiers fail soft instead of crashing a screen", () => {
  const presentation = productMessage("future.contract-message");
  assert.equal(presentation.id, "problem.request-failed");
  assert.equal(presentation.title, "Portico couldn't complete this request");
});

test("a bare 401 does not falsely expire the Portico Account", () => {
  assert.equal(resolveProductProblem({ code: "unauthorized", status: 401 }).id, "problem.request-failed");
  assert.equal(resolveProductProblem({ code: "session_expired", status: 401 }).id, "auth.session-expired");
});

test("profile PIN copy and actions are resolved from the same catalog", () => {
  const presentation = productMessage("auth.profile-pin-required", { profileName: "Kids" });
  assert.equal(presentation.body, "Kids is protected by a four-digit PIN.");
  assert.equal(presentation.icon, "status.locked");
});

test("hosted profile failures resolve to shared grandmother-friendly copy", () => {
  const expected = {
    profile_not_found: "auth.profile-not-found",
    profile_limit_reached: "auth.profile-limit-reached",
    primary_profile_required: "auth.primary-profile-required",
    primary_profile_pin_required: "auth.primary-profile-pin-required",
    primary_profile_pin_in_use: "auth.primary-profile-pin-in-use",
    invalid_profile_restrictions: "auth.invalid-profile-restrictions",
    profile_conflict: "auth.profile-conflict",
    profile_request_failed: "problem.profile-request-failed",
    profile_selection_failed: "auth.profile-selection-failed",
    profile_pin_retry_later: "auth.profile-pin-retry-later",
    device_session_required: "auth.device-session-required",
    interactive_session_required: "auth.device-session-required",
    invalid_password: "account.delete-password-invalid",
    profile_admin_step_up_required: "auth.profile-admin-step-up-required",
    profile_admin_step_up_expired: "auth.profile-admin-step-up-expired",
    profile_admin_recovery_required: "auth.profile-admin-recovery-required"
  };
  for (const [code, id] of Object.entries(expected)) {
    const presentation = resolveProductProblem({ code, status: 400 });
    assert.equal(presentation.id, id);
    assert.ok(presentation.title);
    assert.ok(presentation.icon);
  }
});

test("release-candidate preference, notification, account, and feedback problems use shared copy", () => {
  const expected = {
    automatic_profile_trust_required: "auth.automatic-profile-trust-required",
    feedback_disabled: "feedback.disabled",
    feedback_duplicate: "feedback.duplicate",
    invalid_feedback: "feedback.invalid",
    feedback_rate_limited: "feedback.rate-limited",
    invalid_preferences: "preferences.invalid",
    invalid_profile_policy: "auth.invalid-profile-restrictions",
    invalid_profile: "auth.invalid-profile",
    mfa_required: "account.mfa-required",
    notification_receipt_conflict: "notification.receipt-conflict",
    notification_create_failed: "notification.send-failed",
    notification_load_failed: "notification.load-failed",
    notification_recipients_failed: "notification.recipients-failed",
    notification_update_failed: "notification.receipt-failed",
    owned_servers_require_action: "account.delete-owned-servers",
    preference_conflict: "preferences.conflict",
    preference_revision_conflict: "preferences.conflict",
    profile_admin_proof_required: "auth.profile-admin-step-up-required",
    profile_pin_locked: "auth.profile-temporarily-locked",
    profile_trust_failed: "auth.automatic-profile-trust-failed",
    profiles_managed_by_portico_account: "auth.profiles-managed-by-portico-account",
    unsupported_preference_version: "preferences.unsupported-version",
    unsupported_preferences_version: "preferences.unsupported-version"
  };
  for (const [code, id] of Object.entries(expected)) {
    assert.equal(resolveProductProblem({ code, status: 400 }).id, id);
  }
  assert.equal(productMessage("notification.server-message").icon, "status.notification");
  assert.equal(productMessage("operational-alerts.explainer").title, "Operational alerts");
});

test("a known server message ID takes precedence while an unknown ID safely falls back", () => {
  assert.equal(
    resolveProductProblem({
      code: "future_private_error",
      messageId: "auth.profile-admin-step-up-required",
      status: 403
    }).id,
    "auth.profile-admin-step-up-required"
  );
  assert.equal(
    resolveProductProblem({
      code: "server_not_found",
      messageId: "untrusted.future-message",
      status: 404
    }).id,
    "problem.server-unavailable"
  );
});

test("dynamic notification language and icons fail closed to reviewed catalog values", () => {
  assert.equal(safeProductMessage("notification.server-message", "notification.fallback-title").id, "notification.server-message");
  assert.equal(safeProductMessage("untrusted.future-message", "notification.fallback-title").text, "Notification");
  assert.equal(knownProductMessageId("untrusted.future-message"), undefined);
  assert.equal(knownSemanticIconId("status.notification"), "status.notification");
  assert.equal(knownSemanticIconId("untrusted.future-icon"), undefined);
});

test("interpolation preserves missing placeholders so contract mistakes remain visible", () => {
  assert.equal(interpolateProductText("Open {serverName} for {profileName}.", { serverName: "Home" }), "Open Home for {profileName}.");
});

test("Hosted connection helpers use the same normalized problem catalog", () => {
  assert.equal(
    hostedRouteErrorMessage(new ApiError(503, "server_unavailable", "platform-specific raw detail")),
    "Portico couldn't reach this server. Your Portico Account remains signed in."
  );
  assert.equal(
    porticoSessionErrorMessage(new ApiError(401, "session_expired", "another raw detail")),
    "Your session has expired. Sign in again to continue."
  );
});

test("compatibility failures use canonical update guidance", () => {
  assert.equal(resolveProductProblem({ code: "unsupported_hosted_api_version" }).id, "auth.client-update-required");
  assert.equal(resolveProductProblem({ code: "unsupported_server_api_version" }).id, "auth.server-update-required");
});
