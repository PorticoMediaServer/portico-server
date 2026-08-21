/**
 * The one cross-platform authority for Client Core transport semantics.
 *
 * Operation classes intentionally use their first public names. Keep this
 * table small and semantic: route-specific code may select a class, but it
 * must not invent another deadline or retry policy beside this authority.
 */
export type OperationClass =
  | "interactive read"
  | "interactive mutation"
  | "form/upload"
  | "polling"
  | "discovery/probe"
  | "long poll"
  | "realtime stream"
  | "media/stream transfer";

export type OperationDeadlineScope = "control-plane-response" | "stream-open" | "none";

export type OperationRetryMode = "request" | "reconnect" | "none";

export type OperationIdempotencyRequirement = "idempotent" | "reconcile-before-retry";

export type OperationCancellationBehavior =
  | "abort-request"
  | "abort-request-and-body"
  | "abort-stream"
  | "abort-transfer";

export type OperationTimeoutAction = "retry" | "reconcile" | "try-next-candidate" | "reconnect" | "resume-or-cancel";

export interface OperationPolicy {
  readonly operationClass: OperationClass;
  /** Null explicitly means that Client Core supplies no default deadline. */
  readonly defaultDeadlineMs: number | null;
  /** Whether the deadline covers a response, only stream opening, or neither. */
  readonly deadlineScope: OperationDeadlineScope;
  readonly retry: {
    readonly eligible: boolean;
    /** Default automatic retry/reconnect attempts when the caller is silent. */
    readonly defaultBudget: number;
    /** Maximum automatic retry/reconnect attempts for this class. */
    readonly ceiling: number;
    readonly mode: OperationRetryMode;
  };
  readonly idempotencyRequirement: OperationIdempotencyRequirement;
  readonly cancellation: OperationCancellationBehavior;
  readonly timeout: {
    readonly problemCode: "request_timeout";
    readonly messageId: "problem.timeout";
    readonly action: OperationTimeoutAction;
  };
}

const MAX_OPERATION_DEADLINE_MS = 5 * 60_000;
const MAX_OPERATION_RETRY_CEILING = 4;

function defineOperationPolicy(policy: OperationPolicy): OperationPolicy {
  if (
    policy.defaultDeadlineMs !== null &&
    (!Number.isSafeInteger(policy.defaultDeadlineMs) || policy.defaultDeadlineMs < 1 || policy.defaultDeadlineMs > MAX_OPERATION_DEADLINE_MS)
  ) {
    throw new TypeError("Client Core operation deadlines must be bounded positive integers or null.");
  }
  if (!Number.isSafeInteger(policy.retry.ceiling) || policy.retry.ceiling < 0 || policy.retry.ceiling > MAX_OPERATION_RETRY_CEILING) {
    throw new TypeError(`Client Core operation retry ceilings must be integers from 0 through ${MAX_OPERATION_RETRY_CEILING}.`);
  }
  if (!Number.isSafeInteger(policy.retry.defaultBudget) || policy.retry.defaultBudget < 0 || policy.retry.defaultBudget > policy.retry.ceiling) {
    throw new TypeError("Client Core operation retry defaults must be bounded by their class ceiling.");
  }
  if (!policy.retry.eligible && policy.retry.ceiling !== 0) {
    throw new TypeError("An ineligible Client Core operation cannot declare a retry ceiling.");
  }
  if (policy.deadlineScope === "none" && policy.defaultDeadlineMs !== null) {
    throw new TypeError("An operation without a deadline scope cannot declare a deadline.");
  }
  if (policy.deadlineScope !== "none" && policy.defaultDeadlineMs === null) {
    throw new TypeError("A deadline-scoped operation must declare a bounded default deadline.");
  }
  return Object.freeze({
    ...policy,
    retry: Object.freeze({ ...policy.retry }),
    timeout: Object.freeze({ ...policy.timeout })
  });
}

/**
 * The complete operation-policy authority. The `satisfies` constraint makes
 * additions/removals deliberate at compile time and keeps the public set
 * exactly aligned with the P06 contract.
 */
export const PORTICO_OPERATION_POLICIES = Object.freeze({
  "interactive read": defineOperationPolicy({
    operationClass: "interactive read",
    defaultDeadlineMs: 30_000,
    deadlineScope: "control-plane-response",
    retry: { eligible: true, defaultBudget: 2, ceiling: 4, mode: "request" },
    idempotencyRequirement: "idempotent",
    cancellation: "abort-request",
    timeout: { problemCode: "request_timeout", messageId: "problem.timeout", action: "retry" }
  }),
  "interactive mutation": defineOperationPolicy({
    operationClass: "interactive mutation",
    defaultDeadlineMs: 30_000,
    deadlineScope: "control-plane-response",
    retry: { eligible: false, defaultBudget: 0, ceiling: 0, mode: "none" },
    idempotencyRequirement: "reconcile-before-retry",
    cancellation: "abort-request-and-body",
    timeout: { problemCode: "request_timeout", messageId: "problem.timeout", action: "reconcile" }
  }),
  "form/upload": defineOperationPolicy({
    operationClass: "form/upload",
    defaultDeadlineMs: 30_000,
    deadlineScope: "control-plane-response",
    retry: { eligible: false, defaultBudget: 0, ceiling: 0, mode: "none" },
    idempotencyRequirement: "reconcile-before-retry",
    cancellation: "abort-request-and-body",
    timeout: { problemCode: "request_timeout", messageId: "problem.timeout", action: "reconcile" }
  }),
  polling: defineOperationPolicy({
    operationClass: "polling",
    defaultDeadlineMs: 15_000,
    deadlineScope: "control-plane-response",
    retry: { eligible: true, defaultBudget: 1, ceiling: 2, mode: "request" },
    idempotencyRequirement: "idempotent",
    cancellation: "abort-request-and-body",
    timeout: { problemCode: "request_timeout", messageId: "problem.timeout", action: "retry" }
  }),
  "discovery/probe": defineOperationPolicy({
    operationClass: "discovery/probe",
    defaultDeadlineMs: 10_000,
    deadlineScope: "control-plane-response",
    retry: { eligible: true, defaultBudget: 1, ceiling: 2, mode: "request" },
    idempotencyRequirement: "idempotent",
    cancellation: "abort-request-and-body",
    timeout: { problemCode: "request_timeout", messageId: "problem.timeout", action: "try-next-candidate" }
  }),
  "long poll": defineOperationPolicy({
    operationClass: "long poll",
    defaultDeadlineMs: 30_000,
    deadlineScope: "control-plane-response",
    retry: { eligible: true, defaultBudget: 0, ceiling: 2, mode: "reconnect" },
    idempotencyRequirement: "idempotent",
    cancellation: "abort-request-and-body",
    timeout: { problemCode: "request_timeout", messageId: "problem.timeout", action: "reconnect" }
  }),
  "realtime stream": defineOperationPolicy({
    operationClass: "realtime stream",
    defaultDeadlineMs: 15_000,
    deadlineScope: "stream-open",
    retry: { eligible: true, defaultBudget: 0, ceiling: 4, mode: "reconnect" },
    idempotencyRequirement: "idempotent",
    cancellation: "abort-stream",
    timeout: { problemCode: "request_timeout", messageId: "problem.timeout", action: "reconnect" }
  }),
  "media/stream transfer": defineOperationPolicy({
    operationClass: "media/stream transfer",
    defaultDeadlineMs: null,
    deadlineScope: "none",
    retry: { eligible: false, defaultBudget: 0, ceiling: 0, mode: "none" },
    idempotencyRequirement: "idempotent",
    cancellation: "abort-transfer",
    timeout: { problemCode: "request_timeout", messageId: "problem.timeout", action: "resume-or-cancel" }
  })
} satisfies Readonly<Record<OperationClass, OperationPolicy>>);

export function getOperationPolicy(operationClass: OperationClass): OperationPolicy {
  const policy = PORTICO_OPERATION_POLICIES[operationClass];
  if (!policy) throw new RangeError(`Unsupported Client Core operation class: ${String(operationClass)}.`);
  return policy;
}
