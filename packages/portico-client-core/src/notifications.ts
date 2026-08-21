/** Typed, private viewer notifications and bounded server-owner feedback. */

import { productLanguageCatalog, type ProductMessageId, type SemanticIconId } from "./productLanguage.js";

export const VIEWER_FEEDBACK_VERSION = "v1" as const;
export const VIEWER_NOTIFICATION_RECEIPT_VERSION = "v1" as const;
export const VIEWER_NOTIFICATION_EVENT_VERSION = "v1" as const;

export type NotificationAudience = "profile" | "account-admin";
export type NotificationSeverity = "informational" | "success" | "warning" | "error";
export type ViewerNotificationKind =
  | "account.security"
  | "download.ready"
  | "download.failed"
  | "dvr.conflict"
  | "dvr.recording-failed"
  | "feedback.received"
  | "feedback.updated"
  | "library-channel.degraded"
  | "membership.changed"
  | "server.message";

export type NotificationRecipient = {
  authority: "hosted" | "local";
  accountId: string;
  serverId: string;
  audience: NotificationAudience;
  /** Present only for profile-scoped notifications. Account-administrator notifications belong to the account. */
  profileId?: string;
};

export type NotificationAction = {
  id: string;
  labelMessageId: ProductMessageId;
  kind: "navigate" | "command";
  target: NotificationActionTarget | NotificationActionCommand;
  parameters: Record<string, string>;
};

export type NotificationActionTarget =
  | "account.security"
  | "downloads"
  | "dvr.conflicts"
  | "feedback.detail"
  | "media.detail"
  | "notifications";

export type NotificationActionCommand =
  | "download.retry"
  | "notification.archive"
  | "notification.mark-read"
  | "notification.mark-unread";

export type NotificationContent = {
  /** Plain text only. Product surfaces must not interpret owner-provided markup. */
  title?: string;
  body: string;
};

export type ViewerNotification = {
  id: string;
  recipient: NotificationRecipient;
  kind: ViewerNotificationKind;
  severity: NotificationSeverity;
  messageId: ProductMessageId;
  iconId: SemanticIconId;
  interpolation: Record<string, string>;
  actions: NotificationAction[];
  /** Private owner-authored text, fetched only from the audience-aware notification route. */
  content?: NotificationContent;
  createdAt: string;
  readAt?: string | null;
  archivedAt?: string | null;
};

export type NotificationPage = {
  recipient: NotificationRecipient;
  items: ViewerNotification[];
  unreadCount: number;
  revision: number;
  pageInfo: NotificationPageInfo;
};

export type NotificationPageInfo = {
  nextCursor: string | null;
  hasMore: boolean;
  total?: number;
};

export type NotificationReceiptAction = "mark-read" | "mark-unread" | "archive";

export type NotificationReceiptMutation = {
  version: typeof VIEWER_NOTIFICATION_RECEIPT_VERSION;
  recipient: NotificationRecipient;
  notificationIds: string[];
  action: NotificationReceiptAction;
  expectedRevision: number;
};

export type NotificationReceipt = {
  notificationId: string;
  readAt: string | null;
  archivedAt: string | null;
};

export type NotificationReceiptResult = {
  recipient: NotificationRecipient;
  receipts: NotificationReceipt[];
  unreadCount: number;
  revision: number;
};

export type NotificationReadAllResult = {
  recipient: NotificationRecipient;
  unreadCount: number;
  revision: number;
};

export type NotificationInvalidation = {
  version: typeof VIEWER_NOTIFICATION_EVENT_VERSION;
  kind: "notifications.invalidated";
  occurredAt: string;
};

export type ViewerFeedbackKind = "general" | "playback" | "media" | "quality";
export type ViewerFeedbackCategory =
  | "wont-play"
  | "buffering"
  | "playback-stopped"
  | "wrong-video"
  | "wrong-audio"
  | "wrong-subtitles"
  | "incorrect-media-information"
  | "higher-quality-request"
  | "other";
export type ClientDeviceClass = "web" | "mobile" | "television";
export type KnownClientPlatform = "web" | "ios" | "tvos" | "android" | "android-tv" | "fire-tv" | "vega" | "roku" | "webos" | "tizen" | "visionos";
/** Unknown future identifiers remain intact instead of collapsing into `other`. */
export type ClientPlatform = KnownClientPlatform | (string & {});

export type ViewerFeedbackSubmission = {
  version: typeof VIEWER_FEEDBACK_VERSION;
  kind: ViewerFeedbackKind;
  category: ViewerFeedbackCategory;
  message: string;
  context: {
    mediaId?: string;
    playbackSessionId?: string;
    deviceClass: ClientDeviceClass;
    platform: ClientPlatform;
    appVersion: string;
  };
};

export type ViewerFeedbackStatus = "new" | "read" | "resolved" | "dismissed";

export type ViewerFeedbackReceipt = {
  id: string;
  status: ViewerFeedbackStatus;
  submittedAt: string;
  duplicateOfId?: string;
};

export type ViewerFeedbackCapabilities = {
  version: typeof VIEWER_FEEDBACK_VERSION;
  enabled: boolean;
  allowedKinds: ViewerFeedbackKind[];
  messageMaxLength: number;
  retentionDays: number;
};

export type ViewerFeedbackReporter =
  | { authority: "local"; accountId: string; accountName: string }
  | { authority: "hosted"; membershipId: string; accountName: string };

/** Trusted server-constructed diagnostics. Never accept this shape from a viewer submission. */
export type ViewerFeedbackDiagnostics = {
  mediaId?: string;
  playbackDecisionId?: string;
  selectedVersionId?: string;
  deliveryReason?: string;
  deviceClass: ClientDeviceClass;
  platform: ClientPlatform;
  appVersion: string;
  occurredAt: string;
  errorCategory?: string;
};

export type ViewerFeedbackOwnerResponse = {
  message: string;
  respondedAt: string;
};

export type ViewerFeedbackRecord = {
  id: string;
  reporter: ViewerFeedbackReporter;
  kind: ViewerFeedbackKind;
  category: ViewerFeedbackCategory;
  status: ViewerFeedbackStatus;
  message: string;
  diagnostics: ViewerFeedbackDiagnostics;
  duplicateCount: number;
  ownerResponse?: ViewerFeedbackOwnerResponse | null;
  submittedAt: string;
  updatedAt: string;
  revision: number;
};

export type ViewerFeedbackPage = {
  items: ViewerFeedbackRecord[];
  pageInfo: NotificationPageInfo;
  statusCounts: Record<ViewerFeedbackStatus, number>;
};

export type ViewerFeedbackAdminUpdate = {
  version: typeof VIEWER_FEEDBACK_VERSION;
  expectedRevision: number;
  status?: ViewerFeedbackStatus;
  /** A non-empty plain-text response creates a private profile notification. */
  responseMessage?: string;
};

export type OwnerNotificationProfileRecipient = {
  authority: "local";
  audience: "profile";
  accountId: string;
  profileId: string;
  accountName: string;
  profileName: string;
};

export type OwnerNotificationAccountAdminRecipient = {
  authority: "local" | "hosted";
  audience: "account-admin";
  accountId: string;
  accountName: string;
};

export type OwnerNotificationRecipientDirectory = {
  profiles: OwnerNotificationProfileRecipient[];
  accountAdmins: OwnerNotificationAccountAdminRecipient[];
};

const notificationKinds = new Set<ViewerNotificationKind>([
  "account.security", "download.ready", "download.failed", "dvr.conflict", "dvr.recording-failed",
  "feedback.received", "feedback.updated", "library-channel.degraded", "membership.changed", "server.message"
]);
const notificationSeverities = new Set<NotificationSeverity>(["informational", "success", "warning", "error"]);
const kinds = new Set<ViewerFeedbackKind>(["general", "playback", "media", "quality"]);
const categories = new Set<ViewerFeedbackCategory>([
  "wont-play", "buffering", "playback-stopped", "wrong-video", "wrong-audio", "wrong-subtitles",
  "incorrect-media-information", "higher-quality-request", "other"
]);
const categoriesByKind: Readonly<Record<ViewerFeedbackKind, ReadonlySet<ViewerFeedbackCategory>>> = {
  general: new Set(["other"]),
  playback: new Set(["wont-play", "buffering", "playback-stopped", "wrong-video", "wrong-audio", "wrong-subtitles", "other"]),
  media: new Set(["wrong-video", "wrong-audio", "wrong-subtitles", "incorrect-media-information", "other"]),
  quality: new Set(["higher-quality-request", "other"])
};
const deviceClasses = new Set<ClientDeviceClass>(["web", "mobile", "television"]);
const feedbackStatuses = new Set<ViewerFeedbackStatus>(["new", "read", "resolved", "dismissed"]);
const receiptActions = new Set<NotificationReceiptAction>(["mark-read", "mark-unread", "archive"]);
const identifierPattern = /^[a-z][a-z0-9.-]*$/;
const parameterKeyPattern = /^[A-Za-z][A-Za-z0-9.-]*$/;

type NotificationActionSemantic = {
  kind: NotificationAction["kind"];
  labelMessageId: ProductMessageId;
  requiredParameters: readonly string[];
  optionalParameters: readonly string[];
};

function freezeNotificationActionSemantics<T extends Record<string, NotificationActionSemantic>>(value: T): Readonly<T> {
  for (const semantic of Object.values(value)) {
    Object.freeze(semantic.requiredParameters);
    Object.freeze(semantic.optionalParameters);
    Object.freeze(semantic);
  }
  return Object.freeze(value);
}

/** The complete V1 action allowlist. Server-provided URLs and ad-hoc commands are never portable actions. */
export const notificationActionSemantics: Readonly<Record<NotificationActionTarget | NotificationActionCommand, NotificationActionSemantic>> = freezeNotificationActionSemantics({
  "account.security": { kind: "navigate", labelMessageId: "action.open", requiredParameters: [], optionalParameters: [] },
  downloads: { kind: "navigate", labelMessageId: "action.open", requiredParameters: [], optionalParameters: ["downloadId"] },
  "dvr.conflicts": { kind: "navigate", labelMessageId: "action.open", requiredParameters: [], optionalParameters: ["recordingId"] },
  "feedback.detail": { kind: "navigate", labelMessageId: "action.open", requiredParameters: ["feedbackId"], optionalParameters: [] },
  "media.detail": { kind: "navigate", labelMessageId: "action.open", requiredParameters: ["mediaId"], optionalParameters: [] },
  notifications: { kind: "navigate", labelMessageId: "action.open", requiredParameters: [], optionalParameters: [] },
  "download.retry": { kind: "command", labelMessageId: "action.retry", requiredParameters: ["downloadId"], optionalParameters: [] },
  "notification.archive": { kind: "command", labelMessageId: "action.archive", requiredParameters: ["notificationId"], optionalParameters: [] },
  "notification.mark-read": { kind: "command", labelMessageId: "action.mark-read", requiredParameters: ["notificationId"], optionalParameters: [] },
  "notification.mark-unread": { kind: "command", labelMessageId: "action.mark-unread", requiredParameters: ["notificationId"], optionalParameters: [] }
});

function record(value: unknown, name: string): Record<string, unknown> {
  if (!value || typeof value !== "object" || Array.isArray(value)) throw new TypeError(`${name} must be an object`);
  return value as Record<string, unknown>;
}

function exactOrOptionalKeys(source: Record<string, unknown>, required: readonly string[], optional: readonly string[], name: string): void {
  const allowed = new Set([...required, ...optional]);
  if (Object.keys(source).some(key => !allowed.has(key)) || required.some(key => !Object.hasOwn(source, key))) throw new TypeError(`${name} has missing or unknown fields`);
}

function bounded(value: unknown, name: string, maximum: number, optional = false): string | undefined {
  if (value === undefined && optional) return undefined;
  if (typeof value !== "string") throw new TypeError(`${name} must be a string`);
  const normalized = value.trim();
  if ((!normalized && !optional) || Array.from(normalized).length > maximum) throw new TypeError(`${name} is invalid`);
  return normalized || undefined;
}

function boundedPlainText(value: unknown, name: string, maximum: number, optional = false): string | undefined {
  if (value === undefined && optional) return undefined;
  if (typeof value !== "string") throw new TypeError(`${name} must be plain text`);
  const normalized = value.replace(/\r\n?/g, "\n").trim();
  if ((!normalized && !optional) || Array.from(normalized).length > maximum || /[\u0000-\u0008\u000B\u000C\u000E-\u001F\u007F]/.test(normalized)) {
    throw new TypeError(`${name} is invalid`);
  }
  return normalized || undefined;
}

function timestamp(value: unknown, name: string, optional = false): string | null | undefined {
  if (optional && (value === undefined || value === null)) return value as null | undefined;
  const normalized = bounded(value, name, 64)!;
  if (!Number.isFinite(Date.parse(normalized))) throw new TypeError(`${name} is invalid`);
  return normalized;
}

function safeCount(value: unknown, name: string): number {
  if (!Number.isSafeInteger(value) || (value as number) < 0) throw new TypeError(`${name} is invalid`);
  return value as number;
}

function boundedIdentifierList(value: unknown, name: string, maximum: number): string[] {
  if (!Array.isArray(value) || value.length < 1 || value.length > maximum) throw new TypeError(`${name} is invalid`);
  const entries = value.map(entry => bounded(entry, name, 128)!);
  if (new Set(entries).size !== entries.length) throw new TypeError(`${name} contains duplicates`);
  return entries;
}

export function parseNotificationPageInfo(value: unknown): NotificationPageInfo {
  const source = record(value, "notification page info");
  exactOrOptionalKeys(source, ["nextCursor", "hasMore"], ["total"], "notification page info");
  if (typeof source.hasMore !== "boolean") throw new TypeError("notification page hasMore is invalid");
  const nextCursor = source.nextCursor === null ? null : bounded(source.nextCursor, "notification cursor", 4096)!;
  if (source.hasMore && !nextCursor) throw new TypeError("notification page with more items requires a cursor");
  if (!source.hasMore && nextCursor !== null) throw new TypeError("notification final page cannot include a cursor");
  return {
    nextCursor,
    hasMore: source.hasMore,
    ...(source.total === undefined ? {} : { total: safeCount(source.total, "notification total") })
  };
}

export function parseNotificationRecipient(value: unknown): NotificationRecipient {
  const source = record(value, "notification recipient");
  exactOrOptionalKeys(source, ["authority", "accountId", "serverId", "audience"], ["profileId"], "notification recipient");
  if (source.authority !== "hosted" && source.authority !== "local") throw new TypeError("notification authority is invalid");
  if (source.audience !== "profile" && source.audience !== "account-admin") throw new TypeError("notification audience is invalid");
  const profileId = bounded(source.profileId, "notification profile", 128, true);
  if (source.audience === "profile" && !profileId) throw new TypeError("profile notification requires a profile");
  if (source.audience === "account-admin" && profileId) throw new TypeError("account administrator notification cannot be profile-scoped");
  return {
    authority: source.authority,
    accountId: bounded(source.accountId, "notification account", 128)!,
    serverId: bounded(source.serverId, "notification server", 128)!,
    audience: source.audience,
    ...(profileId ? { profileId } : {})
  };
}

export function sameNotificationRecipient(left: NotificationRecipient, right: NotificationRecipient): boolean {
  const a = parseNotificationRecipient(left);
  const b = parseNotificationRecipient(right);
  return a.authority === b.authority && a.accountId === b.accountId && a.serverId === b.serverId && a.profileId === b.profileId && a.audience === b.audience;
}

function knownMessageId(value: unknown, action = false): ProductMessageId {
  const id = bounded(value, action ? "notification action label" : "notification message", 128)!;
  if (!Object.hasOwn(productLanguageCatalog.messages, id)) throw new TypeError("notification message is not in Product Language");
  const definition = productLanguageCatalog.messages[id as ProductMessageId] as { text?: string };
  if (action && (!id.startsWith("action.") || !definition.text)) throw new TypeError("notification action label is invalid");
  return id as ProductMessageId;
}

function knownIconId(value: unknown): SemanticIconId {
  const id = bounded(value, "notification icon", 128)!;
  if (!Object.hasOwn(productLanguageCatalog.icons, id)) throw new TypeError("notification icon is not in Product Language");
  return id as SemanticIconId;
}

function parseStringMap(value: unknown, name: string, maximumProperties: number, maximumValueLength: number): Record<string, string> {
  const source = record(value, name);
  if (Object.keys(source).length > maximumProperties) throw new TypeError(`${name} has too many fields`);
  return Object.fromEntries(Object.entries(source).map(([key, raw]) => {
    if (!parameterKeyPattern.test(key) || key.length > 64 || typeof raw !== "string" || Array.from(raw).length > maximumValueLength) throw new TypeError(`${name} is invalid`);
    return [key, raw];
  }));
}

export function parseNotificationAction(value: unknown): NotificationAction {
  const source = record(value, "notification action");
  exactOrOptionalKeys(source, ["id", "labelMessageId", "kind", "target", "parameters"], [], "notification action");
  if (source.kind !== "navigate" && source.kind !== "command") throw new TypeError("notification action kind is invalid");
  const target = bounded(source.target, "notification action target", 128)!;
  if (!identifierPattern.test(target)) throw new TypeError("notification action target is invalid");
  const parsed: NotificationAction = {
    id: bounded(source.id, "notification action id", 64)!,
    labelMessageId: knownMessageId(source.labelMessageId, true),
    kind: source.kind,
    target: target as NotificationActionTarget | NotificationActionCommand,
    parameters: parseStringMap(source.parameters, "notification action parameters", 12, 256)
  };
  const semantic = notificationActionSemantics[target as NotificationActionTarget | NotificationActionCommand];
  if (!semantic || semantic.kind !== parsed.kind || semantic.labelMessageId !== parsed.labelMessageId) {
    throw new TypeError("notification action is not in the portable allowlist");
  }
  const parameterKeys = Object.keys(parsed.parameters);
  const allowedParameters = new Set([...semantic.requiredParameters, ...semantic.optionalParameters]);
  if (semantic.requiredParameters.some(key => !Object.hasOwn(parsed.parameters, key)) || parameterKeys.some(key => !allowedParameters.has(key))) {
    throw new TypeError("notification action parameters do not match its target");
  }
  for (const key of parameterKeys) parsed.parameters[key] = bounded(parsed.parameters[key], `notification action parameter ${key}`, 128)!;
  return parsed;
}

function parseNotificationContent(value: unknown): NotificationContent {
  const source = record(value, "notification content");
  exactOrOptionalKeys(source, ["body"], ["title"], "notification content");
  const title = boundedPlainText(source.title, "notification title", 120, true);
  return {
    ...(title ? { title } : {}),
    body: boundedPlainText(source.body, "notification body", 2000)!
  };
}

export function parseViewerNotification(value: unknown): ViewerNotification {
  const source = record(value, "viewer notification");
  exactOrOptionalKeys(source, ["id", "recipient", "kind", "severity", "messageId", "iconId", "interpolation", "actions", "createdAt"], ["content", "readAt", "archivedAt"], "viewer notification");
  if (!notificationKinds.has(source.kind as ViewerNotificationKind) || !notificationSeverities.has(source.severity as NotificationSeverity)) throw new TypeError("viewer notification kind or severity is invalid");
  if (!Array.isArray(source.actions) || source.actions.length > 3) throw new TypeError("viewer notification actions are invalid");
  const content = source.content === undefined ? undefined : parseNotificationContent(source.content);
  if (source.kind === "server.message" && !content) throw new TypeError("server message notification requires private content");
  return {
    id: bounded(source.id, "viewer notification id", 128)!,
    recipient: parseNotificationRecipient(source.recipient),
    kind: source.kind as ViewerNotificationKind,
    severity: source.severity as NotificationSeverity,
    messageId: knownMessageId(source.messageId),
    iconId: knownIconId(source.iconId),
    interpolation: parseStringMap(source.interpolation, "notification interpolation", 16, 256),
    actions: source.actions.map(parseNotificationAction),
    ...(content ? { content } : {}),
    createdAt: timestamp(source.createdAt, "notification creation time")!,
    readAt: timestamp(source.readAt, "notification read time", true),
    archivedAt: timestamp(source.archivedAt, "notification archive time", true)
  };
}

export function parseNotificationPage(value: unknown, expectedRecipient?: NotificationRecipient): NotificationPage {
  const source = record(value, "notification page");
  exactOrOptionalKeys(source, ["recipient", "items", "unreadCount", "revision", "pageInfo"], [], "notification page");
  const recipient = parseNotificationRecipient(source.recipient);
  if (expectedRecipient && !sameNotificationRecipient(recipient, expectedRecipient)) throw new TypeError("notification page recipient does not match viewer");
  if (!Array.isArray(source.items) || source.items.length > 200) throw new TypeError("notification page items are invalid");
  const items = source.items.map(parseViewerNotification);
  if (items.some(item => !sameNotificationRecipient(item.recipient, recipient))) throw new TypeError("notification page contains another recipient's item");
  return {
    recipient,
    items,
    unreadCount: safeCount(source.unreadCount, "notification unread count"),
    revision: safeCount(source.revision, "notification revision"),
    pageInfo: parseNotificationPageInfo(source.pageInfo)
  };
}

export function normalizeNotificationReceiptMutation(value: unknown, expectedRecipient?: NotificationRecipient): NotificationReceiptMutation {
  const source = record(value, "notification receipt mutation");
  exactOrOptionalKeys(source, ["version", "recipient", "notificationIds", "action", "expectedRevision"], [], "notification receipt mutation");
  if (source.version !== VIEWER_NOTIFICATION_RECEIPT_VERSION) throw new TypeError("unsupported notification receipt mutation");
  if (!receiptActions.has(source.action as NotificationReceiptAction)) throw new TypeError("notification receipt action is invalid");
  const recipient = parseNotificationRecipient(source.recipient);
  if (expectedRecipient && !sameNotificationRecipient(recipient, expectedRecipient)) throw new TypeError("notification receipt recipient does not match viewer");
  return {
    version: VIEWER_NOTIFICATION_RECEIPT_VERSION,
    recipient,
    notificationIds: boundedIdentifierList(source.notificationIds, "notification receipt identifiers", 100),
    action: source.action as NotificationReceiptAction,
    expectedRevision: safeCount(source.expectedRevision, "notification receipt expected revision")
  };
}

function parseNotificationReceipt(value: unknown): NotificationReceipt {
  const source = record(value, "notification receipt");
  exactOrOptionalKeys(source, ["notificationId", "readAt", "archivedAt"], [], "notification receipt");
  return {
    notificationId: bounded(source.notificationId, "notification receipt identifier", 128)!,
    readAt: timestamp(source.readAt, "notification receipt read time", true) ?? null,
    archivedAt: timestamp(source.archivedAt, "notification receipt archive time", true) ?? null
  };
}

export function parseNotificationReceiptResult(value: unknown, expectedRecipient?: NotificationRecipient): NotificationReceiptResult {
  const source = record(value, "notification receipt result");
  exactOrOptionalKeys(source, ["recipient", "receipts", "unreadCount", "revision"], [], "notification receipt result");
  const recipient = parseNotificationRecipient(source.recipient);
  if (expectedRecipient && !sameNotificationRecipient(recipient, expectedRecipient)) throw new TypeError("notification receipt result recipient does not match viewer");
  if (!Array.isArray(source.receipts) || source.receipts.length > 100) throw new TypeError("notification receipts are invalid");
  const receipts = source.receipts.map(parseNotificationReceipt);
  if (new Set(receipts.map(receipt => receipt.notificationId)).size !== receipts.length) throw new TypeError("notification receipts contain duplicates");
  return {
    recipient,
    receipts,
    unreadCount: safeCount(source.unreadCount, "notification unread count"),
    revision: safeCount(source.revision, "notification revision")
  };
}

export function parseNotificationReadAllResult(value: unknown, expectedRecipient?: NotificationRecipient): NotificationReadAllResult {
  const source = record(value, "notification read-all result");
  exactOrOptionalKeys(source, ["recipient", "unreadCount", "revision"], [], "notification read-all result");
  const recipient = parseNotificationRecipient(source.recipient);
  if (expectedRecipient && !sameNotificationRecipient(recipient, expectedRecipient)) throw new TypeError("notification read-all recipient does not match viewer");
  return {
    recipient,
    unreadCount: safeCount(source.unreadCount, "notification unread count"),
    revision: safeCount(source.revision, "notification revision")
  };
}

/** Strictly validates client-controlled feedback. Trusted diagnostics are added by the server. */
export function normalizeViewerFeedbackSubmission(value: unknown): ViewerFeedbackSubmission {
  const source = record(value, "feedback submission");
  exactOrOptionalKeys(source, ["version", "kind", "category", "message", "context"], [], "feedback submission");
  if (source.version !== VIEWER_FEEDBACK_VERSION) throw new TypeError("unsupported feedback submission");
  if (!kinds.has(source.kind as ViewerFeedbackKind) || !categories.has(source.category as ViewerFeedbackCategory)) throw new TypeError("feedback kind or category is invalid");
  const kind = source.kind as ViewerFeedbackKind;
  const category = source.category as ViewerFeedbackCategory;
  if (!categoriesByKind[kind].has(category)) throw new TypeError("feedback category does not match kind");
  const context = record(source.context, "feedback context");
  exactOrOptionalKeys(context, ["deviceClass", "platform", "appVersion"], ["mediaId", "playbackSessionId"], "feedback context");
  if (!deviceClasses.has(context.deviceClass as ClientDeviceClass)) throw new TypeError("feedback device class is invalid");
  const platform = bounded(context.platform, "platform", 64)!;
  if (!identifierPattern.test(platform)) throw new TypeError("platform is invalid");
  const mediaId = bounded(context.mediaId, "mediaId", 128, true);
  const playbackSessionId = bounded(context.playbackSessionId, "playbackSessionId", 128, true);
  if ((kind === "media" || kind === "quality") && !mediaId) throw new TypeError("media feedback requires mediaId");
  if (kind === "playback" && !playbackSessionId) throw new TypeError("playback feedback requires playbackSessionId");
  const message = boundedPlainText(source.message, "message", 1000, true) ?? "";
  if ((kind === "general" || category === "other") && !message) throw new TypeError("feedback message is required");
  return {
    version: VIEWER_FEEDBACK_VERSION,
    kind,
    category,
    message,
    context: {
      mediaId,
      playbackSessionId,
      deviceClass: context.deviceClass as ClientDeviceClass,
      platform,
      appVersion: bounded(context.appVersion, "appVersion", 64)!
    }
  };
}

export function parseViewerFeedbackReceipt(value: unknown): ViewerFeedbackReceipt {
  const source = record(value, "feedback receipt");
  exactOrOptionalKeys(source, ["id", "status", "submittedAt"], ["duplicateOfId"], "feedback receipt");
  if (!feedbackStatuses.has(source.status as ViewerFeedbackStatus)) throw new TypeError("feedback receipt status is invalid");
  const duplicateOfId = bounded(source.duplicateOfId, "duplicate feedback identifier", 128, true);
  return {
    id: bounded(source.id, "feedback identifier", 128)!,
    status: source.status as ViewerFeedbackStatus,
    submittedAt: timestamp(source.submittedAt, "feedback submission time")!,
    ...(duplicateOfId ? { duplicateOfId } : {})
  };
}

export function parseViewerFeedbackCapabilities(value: unknown): ViewerFeedbackCapabilities {
  const source = record(value, "feedback capabilities");
  exactOrOptionalKeys(source, ["version", "enabled", "allowedKinds", "messageMaxLength", "retentionDays"], [], "feedback capabilities");
  if (source.version !== VIEWER_FEEDBACK_VERSION || typeof source.enabled !== "boolean") throw new TypeError("feedback capabilities are invalid");
  if (!Array.isArray(source.allowedKinds) || source.allowedKinds.length > kinds.size) throw new TypeError("feedback capability kinds are invalid");
  const allowedKinds = source.allowedKinds.map(kind => {
    if (!kinds.has(kind as ViewerFeedbackKind)) throw new TypeError("feedback capability kind is invalid");
    return kind as ViewerFeedbackKind;
  });
  if (new Set(allowedKinds).size !== allowedKinds.length || (source.enabled && allowedKinds.length === 0)) throw new TypeError("feedback capability kinds are invalid");
  const messageMaxLength = safeCount(source.messageMaxLength, "feedback message limit");
  const retentionDays = safeCount(source.retentionDays, "feedback retention");
  if (messageMaxLength < 1 || messageMaxLength > 1000 || retentionDays < 1 || retentionDays > 3650) throw new TypeError("feedback capability bounds are invalid");
  return { version: VIEWER_FEEDBACK_VERSION, enabled: source.enabled, allowedKinds, messageMaxLength, retentionDays };
}

function parseViewerFeedbackReporter(value: unknown): ViewerFeedbackReporter {
  const source = record(value, "feedback reporter");
  exactOrOptionalKeys(source, ["authority", "accountName"], ["accountId", "membershipId"], "feedback reporter");
  if (source.authority !== "hosted" && source.authority !== "local") throw new TypeError("feedback reporter authority is invalid");
  const accountId = bounded(source.accountId, "feedback reporter account", 128, true);
  const membershipId = bounded(source.membershipId, "feedback reporter membership", 128, true);
  if ((source.authority === "local" && (!accountId || membershipId)) || (source.authority === "hosted" && (!membershipId || accountId))) {
    throw new TypeError("feedback reporter identity does not match its authority");
  }
  const accountName = boundedPlainText(source.accountName, "feedback reporter account name", 80)!;
  return source.authority === "local"
    ? { authority: "local", accountId: accountId!, accountName }
    : { authority: "hosted", membershipId: membershipId!, accountName };
}

function parseViewerFeedbackDiagnostics(value: unknown): ViewerFeedbackDiagnostics {
  const source = record(value, "feedback diagnostics");
  exactOrOptionalKeys(source, ["deviceClass", "platform", "appVersion", "occurredAt"], [
    "mediaId", "playbackDecisionId", "selectedVersionId", "deliveryReason", "errorCategory"
  ], "feedback diagnostics");
  if (!deviceClasses.has(source.deviceClass as ClientDeviceClass)) throw new TypeError("feedback diagnostic device class is invalid");
  const platform = bounded(source.platform, "feedback diagnostic platform", 64)!;
  if (!identifierPattern.test(platform)) throw new TypeError("feedback diagnostic platform is invalid");
  const errorCategory = bounded(source.errorCategory, "feedback diagnostic error category", 64, true);
  if (errorCategory && !identifierPattern.test(errorCategory)) throw new TypeError("feedback diagnostic error category is invalid");
  const mediaId = bounded(source.mediaId, "feedback diagnostic media", 128, true);
  const playbackDecisionId = bounded(source.playbackDecisionId, "feedback diagnostic playback decision", 128, true);
  const selectedVersionId = bounded(source.selectedVersionId, "feedback diagnostic selected version", 128, true);
  const deliveryReason = boundedPlainText(source.deliveryReason, "feedback diagnostic delivery reason", 256, true);
  return {
    ...(mediaId ? { mediaId } : {}),
    ...(playbackDecisionId ? { playbackDecisionId } : {}),
    ...(selectedVersionId ? { selectedVersionId } : {}),
    ...(deliveryReason ? { deliveryReason } : {}),
    deviceClass: source.deviceClass as ClientDeviceClass,
    platform,
    appVersion: bounded(source.appVersion, "feedback diagnostic app version", 64)!,
    occurredAt: timestamp(source.occurredAt, "feedback diagnostic time")!,
    ...(errorCategory ? { errorCategory } : {})
  };
}

function parseViewerFeedbackOwnerResponse(value: unknown): ViewerFeedbackOwnerResponse {
  const source = record(value, "feedback owner response");
  exactOrOptionalKeys(source, ["message", "respondedAt"], [], "feedback owner response");
  return {
    message: boundedPlainText(source.message, "feedback owner response", 1000)!,
    respondedAt: timestamp(source.respondedAt, "feedback owner response time")!
  };
}

export function parseViewerFeedbackRecord(value: unknown): ViewerFeedbackRecord {
  const source = record(value, "feedback record");
  exactOrOptionalKeys(source, [
    "id", "reporter", "kind", "category", "status", "message", "diagnostics", "duplicateCount", "submittedAt", "updatedAt", "revision"
  ], ["ownerResponse"], "feedback record");
  if (!kinds.has(source.kind as ViewerFeedbackKind) || !categories.has(source.category as ViewerFeedbackCategory)) throw new TypeError("feedback record kind or category is invalid");
  const kind = source.kind as ViewerFeedbackKind;
  const category = source.category as ViewerFeedbackCategory;
  if (!categoriesByKind[kind].has(category) || !feedbackStatuses.has(source.status as ViewerFeedbackStatus)) throw new TypeError("feedback record state is invalid");
  const ownerResponse = source.ownerResponse === undefined || source.ownerResponse === null ? source.ownerResponse as null | undefined : parseViewerFeedbackOwnerResponse(source.ownerResponse);
  const diagnostics = parseViewerFeedbackDiagnostics(source.diagnostics);
  if ((kind === "media" || kind === "quality") && !diagnostics.mediaId) throw new TypeError("media feedback record requires media diagnostics");
  if (kind === "playback" && !diagnostics.playbackDecisionId) throw new TypeError("playback feedback record requires playback diagnostics");
  return {
    id: bounded(source.id, "feedback identifier", 128)!,
    reporter: parseViewerFeedbackReporter(source.reporter),
    kind,
    category,
    status: source.status as ViewerFeedbackStatus,
    message: boundedPlainText(source.message, "feedback message", 1000, true) ?? "",
    diagnostics,
    duplicateCount: safeCount(source.duplicateCount, "feedback duplicate count"),
    ...(ownerResponse !== undefined ? { ownerResponse } : {}),
    submittedAt: timestamp(source.submittedAt, "feedback submission time")!,
    updatedAt: timestamp(source.updatedAt, "feedback update time")!,
    revision: safeCount(source.revision, "feedback revision")
  };
}

export function parseViewerFeedbackPage(value: unknown): ViewerFeedbackPage {
  const source = record(value, "feedback page");
  exactOrOptionalKeys(source, ["items", "pageInfo", "statusCounts"], [], "feedback page");
  if (!Array.isArray(source.items) || source.items.length > 200) throw new TypeError("feedback page items are invalid");
  const items = source.items.map(parseViewerFeedbackRecord);
  if (new Set(items.map(item => item.id)).size !== items.length) throw new TypeError("feedback page contains duplicate items");
  const rawCounts = record(source.statusCounts, "feedback status counts");
  exactOrOptionalKeys(rawCounts, ["new", "read", "resolved", "dismissed"], [], "feedback status counts");
  return {
    items,
    pageInfo: parseNotificationPageInfo(source.pageInfo),
    statusCounts: {
      new: safeCount(rawCounts.new, "new feedback count"),
      read: safeCount(rawCounts.read, "read feedback count"),
      resolved: safeCount(rawCounts.resolved, "resolved feedback count"),
      dismissed: safeCount(rawCounts.dismissed, "dismissed feedback count")
    }
  };
}

export function normalizeViewerFeedbackAdminUpdate(value: unknown): ViewerFeedbackAdminUpdate {
  const source = record(value, "feedback admin update");
  exactOrOptionalKeys(source, ["version", "expectedRevision"], ["status", "responseMessage"], "feedback admin update");
  if (source.version !== VIEWER_FEEDBACK_VERSION) throw new TypeError("unsupported feedback admin update");
  const status = source.status === undefined ? undefined : source.status as ViewerFeedbackStatus;
  if (status !== undefined && !feedbackStatuses.has(status)) throw new TypeError("feedback admin status is invalid");
  const responseMessage = boundedPlainText(source.responseMessage, "feedback response", 1000, true);
  if (!status && !responseMessage) throw new TypeError("feedback admin update is empty");
  return {
    version: VIEWER_FEEDBACK_VERSION,
    expectedRevision: safeCount(source.expectedRevision, "feedback expected revision"),
    ...(status ? { status } : {}),
    ...(responseMessage ? { responseMessage } : {})
  };
}

export function parseOwnerNotificationRecipientDirectory(value: unknown): OwnerNotificationRecipientDirectory {
  const source = record(value, "owner notification recipient directory");
  exactOrOptionalKeys(source, ["profiles", "accountAdmins"], [], "owner notification recipient directory");
  if (!Array.isArray(source.profiles) || !Array.isArray(source.accountAdmins) || source.profiles.length > 1_000 || source.accountAdmins.length > 1_000) {
    throw new TypeError("owner notification recipient directory is invalid");
  }
  const profiles = source.profiles.map((value): OwnerNotificationProfileRecipient => {
    const recipient = record(value, "owner profile notification recipient");
    exactOrOptionalKeys(recipient, ["authority", "audience", "accountId", "profileId", "accountName", "profileName"], [], "owner profile notification recipient");
    if (recipient.authority !== "local" || recipient.audience !== "profile") throw new TypeError("owner profile notification recipient is invalid");
    return {
      authority: "local",
      audience: "profile",
      accountId: bounded(recipient.accountId, "owner profile recipient account", 128)!,
      profileId: bounded(recipient.profileId, "owner profile recipient profile", 128)!,
      accountName: boundedPlainText(recipient.accountName, "owner profile recipient account name", 80)!,
      profileName: boundedPlainText(recipient.profileName, "owner profile recipient profile name", 80)!
    };
  });
  const accountAdmins = source.accountAdmins.map((value): OwnerNotificationAccountAdminRecipient => {
    const recipient = record(value, "owner account notification recipient");
    exactOrOptionalKeys(recipient, ["authority", "audience", "accountId", "accountName"], [], "owner account notification recipient");
    if ((recipient.authority !== "local" && recipient.authority !== "hosted") || recipient.audience !== "account-admin") {
      throw new TypeError("owner account notification recipient is invalid");
    }
    return {
      authority: recipient.authority,
      audience: "account-admin",
      accountId: bounded(recipient.accountId, "owner account recipient account", 128)!,
      accountName: boundedPlainText(recipient.accountName, "owner account recipient name", 80)!
    };
  });
  if (new Set(profiles.map((recipient) => recipient.profileId)).size !== profiles.length ||
      new Set(accountAdmins.map((recipient) => recipient.accountId)).size !== accountAdmins.length) {
    throw new TypeError("owner notification recipient directory contains duplicate recipients");
  }
  return { profiles, accountAdmins };
}

export function notificationActionIsSafe(action: unknown): boolean {
  try {
    parseNotificationAction(action);
    return true;
  } catch {
    return false;
  }
}

export function parseNotificationInvalidation(value: unknown): NotificationInvalidation {
  const source = record(value, "notification invalidation");
  exactOrOptionalKeys(source, ["version", "kind", "occurredAt"], [], "notification invalidation");
  if (source.version !== VIEWER_NOTIFICATION_EVENT_VERSION || source.kind !== "notifications.invalidated") {
    throw new TypeError("notification invalidation version or kind is invalid");
  }
  return {
    version: VIEWER_NOTIFICATION_EVENT_VERSION,
    kind: "notifications.invalidated",
    occurredAt: timestamp(source.occurredAt, "notification invalidation time")!
  };
}

export function applyNotificationInvalidation(event: unknown): NotificationInvalidation | undefined {
  try {
    return parseNotificationInvalidation(event);
  } catch {
    return undefined;
  }
}
