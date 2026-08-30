import { productLanguageCatalog } from "./productLanguageCatalog.generated.js";

export { productLanguageCatalog };

export type ProductMessageId = keyof typeof productLanguageCatalog.messages;
export type SemanticIconId = keyof typeof productLanguageCatalog.icons;
export type ProductMessageTone = "neutral" | "success" | "warning" | "error";
export type ProductMessageVariables = Readonly<Record<string, string | number | undefined>>;

export interface ProductMessagePresentation {
  id: ProductMessageId;
  title?: string;
  body?: string;
  text?: string;
  icon?: SemanticIconId;
  tone: ProductMessageTone;
  actions: ReadonlyArray<{ id: ProductMessageId; label: string }>;
}

export interface ProductProblemLike {
  code?: string;
  messageId?: string;
  status?: number;
  details?: Readonly<Record<string, unknown>>;
}

const problemCodeMessages: Readonly<Record<string, ProductMessageId>> = Object.freeze({
  account_admin_required: "problem.forbidden",
  account_delete_failed: "account.delete-failed",
  account_update_failed: "account.save-failed",
  automatic_profile_trust_expired: "auth.automatic-profile-trust-required",
  automatic_profile_trust_required: "auth.automatic-profile-trust-required",
  connection_failed: "problem.connection-failed",
  csrf_failed: "problem.request-verification-failed",
  download_failed: "download.failed",
  download_storage_full: "download.storage-full",
  dvr_schedule_conflict: "dvr.conflict",
  dvr_recording_failed: "dvr.recording-failed",
  dvr_revision_conflict: "dvr.revision-conflict",
  dvr_storage_full: "dvr.storage-full",
  dvr_storage_pressure: "dvr.storage-pressure",
  device_session_required: "auth.device-session-required",
  feedback_not_allowed: "feedback.disabled",
  feedback_rate_limited: "feedback.rate-limited",
  feedback_conflict: "feedback.conflict",
  feedback_disabled: "feedback.disabled",
  feedback_duplicate: "feedback.duplicate",
  invalid_feedback: "feedback.invalid",
  feedback_not_found: "problem.not-found",
  feedback_response_failed: "feedback.response-failed",
  feedback_unavailable: "problem.server-unavailable",
  invalid_cursor: "problem.invalid-request",
  invalid_feedback_context: "problem.invalid-request",
  invalid_credentials: "auth.invalid-credentials",
  invalid_display_name: "account.invalid-display-name",
  invalid_email_or_password: "auth.invalid-credentials",
  interactive_session_required: "auth.device-session-required",
  invalid_password: "account.delete-password-invalid",
  reauthentication_required: "account.delete-reauthentication-required",
  invalid_preferences: "preferences.invalid",
  invalid_profile: "auth.invalid-profile",
  invalid_profile_policy: "auth.invalid-profile-restrictions",
  invalid_profile_restrictions: "auth.invalid-profile-restrictions",
  invalid_user_field: "problem.invalid-request",
  hosted_busy: "problem.hosted-busy",
  rate_limit_unavailable: "problem.hosted-busy",
  credential_verification_unavailable: "problem.hosted-busy",
  local_profile_pin_invalid: "auth.local-profile-pin-invalid",
  // Library Channel problem codes map to the same current Product Language
  // entries used by their server-issued message identifiers.
  library_channel_generation_in_progress: "library-channel.generation-in-progress",
  library_channel_generation_timeout: "library-channel.generation-timeout",
  library_channel_invalid_request: "problem.invalid-request",
  library_channel_logo_delete_failed: "problem.request-failed",
  library_channel_logo_in_use: "problem.invalid-request",
  library_channel_logo_invalid: "problem.invalid-request",
  library_channel_logo_not_found: "problem.not-found",
  library_channel_logo_store_failed: "problem.request-failed",
  library_channel_no_applicable_defaults: "library-channel.no-applicable-defaults",
  library_channel_default_disabled_no_playable_media: "library-channel.default-disabled-no-playable-media",
  library_channel_no_playable_schedule: "library-channel.generation-no-playable-schedule",
  library_channel_not_found: "problem.not-found",
  library_channel_program_restricted: "library-channel.program-restricted",
  library_channel_program_unavailable: "library-channel.program-unavailable",
  library_channel_capacity_unavailable: "library-channel.playback-capacity",
  library_channel_segment_starting: "library-channel.playback-capacity",
  library_channel_playback_unavailable: "library-channel.program-unavailable",
  library_channel_logo_bug_overhead_required: "library-channel.logo-processing-overhead",
  library_channel_playback_policy_stale: "library-channel.revision-conflict",
  library_channel_request_failed: "problem.request-failed",
  library_channel_revision_conflict: "library-channel.revision-conflict",
  library_channel_template_exists: "problem.invalid-request",
  media_not_found: "problem.not-found",
  media_children_cursor_failed: "problem.request-failed",
  media_children_unavailable: "media.detail-unavailable",
  navigation_unavailable: "navigation.unavailable",
  live_tv_channel_busy: "live-tv.channel-busy",
  live_tv_guide_unavailable: "live-tv.guide-unavailable",
  live_tv_program_unavailable: "live-tv.program-unavailable",
  live_tv_tuner_capacity: "live-tv.channel-busy",
  live_tv_tuner_allocation_failed: "live-tv.offline",
  dvr_running_playback_session_required: "live-tv.channel-busy",
  membership_revoked: "problem.forbidden",
  mfa_required: "account.mfa-required",
  mfa_invalid: "account.mfa-invalid",
  notification_not_found: "problem.not-found",
  notification_create_failed: "notification.send-failed",
  notification_load_failed: "notification.load-failed",
  notification_recipients_failed: "notification.recipients-failed",
  notification_update_failed: "notification.receipt-failed",
  notification_receipt_conflict: "notification.receipt-conflict",
  owner_messaging_disabled: "feedback.disabled",
  owned_servers_require_action: "account.delete-owned-servers",
  playback_capacity_reached: "playback.capacity-reached",
  playback_failed: "playback.failed",
  playback_unavailable: "playback.unavailable",
  primary_profile_pin_required: "auth.primary-profile-pin-required",
  primary_profile_pin_in_use: "auth.primary-profile-pin-in-use",
  primary_profile_required: "auth.primary-profile-required",
  preference_conflict: "preferences.conflict",
  preference_revision_conflict: "preferences.conflict",
  preferences_failed: "preferences.request-failed",
  profile_conflict: "auth.profile-conflict",
  profile_limit_reached: "auth.profile-limit-reached",
  profile_not_found: "auth.profile-not-found",
  profile_not_available_on_server: "auth.profile-not-available",
  profile_pin_invalid: "auth.profile-pin-invalid",
  profile_pin_retry_later: "auth.profile-pin-retry-later",
  profile_pin_required: "auth.profile-pin-required",
  profile_selection_required: "auth.profile-selection-required",
  profile_selection_failed: "auth.profile-selection-failed",
  profile_temporarily_locked: "auth.profile-temporarily-locked",
  person_cursor_failed: "problem.request-failed",
  person_detail_unavailable: "media.detail-unavailable",
  person_not_found: "problem.not-found",
  profile_request_failed: "problem.profile-request-failed",
  profile_admin_step_up_required: "auth.profile-admin-step-up-required",
  profile_admin_step_up_expired: "auth.profile-admin-step-up-expired",
  profile_admin_recovery_required: "auth.profile-admin-recovery-required",
  profile_admin_proof_expired: "auth.profile-admin-step-up-expired",
  profile_admin_proof_required: "auth.profile-admin-step-up-required",
  profile_directory_failed: "problem.profile-request-failed",
  profile_pin_locked: "auth.profile-temporarily-locked",
  profile_proof_failed: "problem.profile-request-failed",
  profile_trust_failed: "auth.automatic-profile-trust-failed",
  profiles_managed_by_portico_account: "auth.profiles-managed-by-portico-account",
  hosted_unavailable: "problem.cloud-unavailable",
  // Authority freshness failures are safe, retryable continuity states. Keep
  // them in Product Language instead of leaking a generic transport error;
  // the server remains fail-closed until a fresh signed policy is available.
  hosted_authority_stale: "problem.cloud-unavailable",
  hosted_authority_clock_invalid: "problem.server-unavailable",
  rate_limited: "problem.rate-limited",
  recent_reauthentication_required: "auth.session-expired",
  remote_access_disabled: "problem.server-unavailable",
  request_timeout: "problem.timeout",
  search_history_unavailable: "search.offline",
  search_history_update_failed: "problem.request-failed",
  server_not_found: "problem.server-unavailable",
  server_unavailable: "problem.server-unavailable",
  invalid_refresh_token: "auth.session-expired",
  refresh_token_reuse: "auth.session-expired",
  session_expired: "auth.session-expired",
  unsupported_hosted_api_version: "auth.client-update-required",
  unsupported_server_api_version: "auth.server-update-required",
  invalid_product_contract_api_version: "auth.server-update-required",
  incompatible_hosted_api_version: "auth.client-update-required",
  incompatible_server_api_version: "auth.server-update-required",
  unsupported_preference_version: "preferences.unsupported-version",
  unsupported_preferences_version: "preferences.unsupported-version",
  watch_with_friends_create_failed: "watch-with-friends.create-failed",
  watch_with_friends_end_failed: "watch-with-friends.command-failed",
  watch_with_friends_failed: "watch-with-friends.profile-unavailable",
  watch_with_friends_host_required: "watch-with-friends.command-failed",
  watch_with_friends_invalid_request: "problem.invalid-request",
  watch_with_friends_invalid_revision: "problem.invalid-request",
  watch_with_friends_join_failed: "watch-with-friends.join-failed",
  watch_with_friends_leave_failed: "watch-with-friends.command-failed",
  watch_with_friends_member_state_failed: "watch-with-friends.command-failed",
  watch_with_friends_not_found: "watch-with-friends.session-ended",
  watch_with_friends_queue_failed: "watch-with-friends.command-failed",
  watch_with_friends_revision_conflict: "watch-with-friends.revision-conflict",
  watch_with_friends_settings_failed: "watch-with-friends.command-failed",
  watch_with_friends_state_failed: "watch-with-friends.command-failed"
});

export function productMessage(
  id: ProductMessageId,
  variables: ProductMessageVariables = {}
): ProductMessagePresentation {
  const definition = productLanguageCatalog.messages[id]
    ?? productLanguageCatalog.messages["problem.request-failed"];
  const raw = definition as {
    text?: string;
    title?: string;
    body?: string;
    icon?: SemanticIconId;
    tone?: ProductMessageTone;
    actions?: readonly ProductMessageId[];
  };
  return {
    id: productLanguageCatalog.messages[id] ? id : "problem.request-failed",
    ...(raw.title ? { title: interpolateProductText(raw.title, variables) } : {}),
    ...(raw.body ? { body: interpolateProductText(raw.body, variables) } : {}),
    ...(raw.text ? { text: interpolateProductText(raw.text, variables) } : {}),
    ...(raw.icon ? { icon: raw.icon } : {}),
    tone: raw.tone ?? "neutral",
    actions: (raw.actions ?? []).map((actionId) => {
      const action = productLanguageCatalog.messages[actionId] as { text?: string };
      if (!action.text) throw new Error(`Product action ${actionId} does not define text.`);
      return { id: actionId, label: interpolateProductText(action.text, variables) };
    })
  };
}

/** Resolves untrusted/dynamic message IDs without exposing transport detail or throwing. */
export function safeProductMessage(
  id: string | undefined,
  fallbackId: ProductMessageId,
  variables: ProductMessageVariables = {}
): ProductMessagePresentation {
  return productMessage(knownProductMessageId(id) ?? fallbackId, variables);
}

export function resolveProductProblem(
  problem: ProductProblemLike,
  variables: ProductMessageVariables = {}
): ProductMessagePresentation {
  const messageId = knownProductMessageId(problem.messageId);
  if (messageId) return productMessage(messageId, variablesFromProblem(problem, variables));
  const code = normalizedProductIdentifier(problem.code);
  const explicit = productMessageIdForProblemCode(code);
  if (explicit) return productMessage(explicit, variablesFromProblem(problem, variables));
  // A bare 401 is contextual: it can mean invalid login credentials, a stale
  // server-local credential, profile continuation rejection, or proxy auth.
  // Only stable codes/message IDs above may declare the Portico Account
  // session expired.
  if (problem.status === 401) return productMessage("problem.request-failed", variables);
  if (problem.status === 403) return productMessage("problem.forbidden", variables);
  if (problem.status === 404) return productMessage("problem.not-found", variables);
  if (problem.status === 408 || problem.status === 504) return productMessage("problem.timeout", variables);
  if (problem.status === 429) return productMessage("problem.rate-limited", variables);
  if (problem.status === 502 || problem.status === 503) return productMessage("problem.server-unavailable", variablesFromProblem(problem, variables));
  return productMessage("problem.request-failed", variables);
}

export function productMessageIdForProblemCode(code: unknown): ProductMessageId | undefined {
  const normalized = normalizedProductIdentifier(code);
  return normalized ? problemCodeMessages[normalized] : undefined;
}

export function knownProductMessageId(value: unknown): ProductMessageId | undefined {
  const normalized = normalizedProductIdentifier(value);
  if (!normalized || !Object.prototype.hasOwnProperty.call(productLanguageCatalog.messages, normalized)) return undefined;
  return normalized as ProductMessageId;
}

export function semanticIcon(id: SemanticIconId) {
  return productLanguageCatalog.icons[id];
}

export function knownSemanticIconId(value: unknown): SemanticIconId | undefined {
  const normalized = normalizedProductIdentifier(value);
  if (!normalized || !Object.prototype.hasOwnProperty.call(productLanguageCatalog.icons, normalized)) return undefined;
  return normalized as SemanticIconId;
}

function normalizedProductIdentifier(value: unknown): string | undefined {
  if (typeof value !== "string") return undefined;
  const normalized = value.trim();
  return normalized || undefined;
}

export function interpolateProductText(template: string, variables: ProductMessageVariables): string {
  return template.replace(/\{([A-Za-z][A-Za-z0-9]*)\}/g, (placeholder, name: string) => {
    const value = variables[name];
    return value === undefined ? placeholder : String(value);
  });
}

function variablesFromProblem(problem: ProductProblemLike, variables: ProductMessageVariables): ProductMessageVariables {
  const details = problem.details ?? {};
  return {
    serverName: safeString(details.serverName) ?? variables.serverName ?? "this server",
    profileName: safeString(details.profileName) ?? variables.profileName ?? "This profile",
    capacity: safeNumber(details.capacity) ?? variables.capacity,
    demand: safeNumber(details.demand) ?? variables.demand,
    ...variables
  };
}

function safeString(value: unknown): string | undefined {
  if (typeof value !== "string") return undefined;
  const normalized = value.replace(/[\u0000-\u001F\u007F]/g, "").trim();
  return normalized ? normalized.slice(0, 120) : undefined;
}

function safeNumber(value: unknown): number | undefined {
  return typeof value === "number" && Number.isFinite(value) ? value : undefined;
}
