export type PlaybackAutomationPreferences = {
  passoutProtection: boolean;
  passoutAfterEpisodes: number;
  inactivityLimitMs?: number;
};

export type PlaybackAutomationState = {
  sessionId?: string;
  automaticAdvances: number;
  lastMeaningfulInteractionAt: number;
};

export type PlaybackAutomationEvent =
  | { type: "session-changed"; sessionId: string; now: number }
  | { type: "meaningful-interaction"; now: number }
  | { type: "automatic-advance-requested"; now: number };

export type PlaybackAutomationEffect = "none" | "advance" | "confirm-still-watching";

export function createPlaybackAutomationState(now = Date.now()): PlaybackAutomationState {
  return { automaticAdvances: 0, lastMeaningfulInteractionAt: now };
}

export function reducePlaybackAutomation(
  state: PlaybackAutomationState,
  event: PlaybackAutomationEvent,
  preferences: PlaybackAutomationPreferences
): { state: PlaybackAutomationState; effect: PlaybackAutomationEffect } {
  if (event.type === "session-changed") {
    return { state: { sessionId: event.sessionId, automaticAdvances: state.automaticAdvances, lastMeaningfulInteractionAt: state.lastMeaningfulInteractionAt || event.now }, effect: "none" };
  }
  if (event.type === "meaningful-interaction") {
    return { state: { ...state, automaticAdvances: 0, lastMeaningfulInteractionAt: event.now }, effect: "none" };
  }
  const inactiveFor = Math.max(0, event.now - state.lastMeaningfulInteractionAt);
  const inactivityLimit = Math.max(1, preferences.inactivityLimitMs ?? 2 * 60 * 60 * 1000);
  if (preferences.passoutProtection && (state.automaticAdvances >= Math.max(1, preferences.passoutAfterEpisodes) || inactiveFor >= inactivityLimit)) {
    return { state, effect: "confirm-still-watching" };
  }
  return { state: { ...state, automaticAdvances: state.automaticAdvances + 1 }, effect: "advance" };
}

export type UpNextCountdownState = {
  phase: "inactive" | "manual" | "countdown" | "cancelled" | "fired";
  deadlineAt?: number;
};

export type UpNextCountdownEvent =
  | { type: "prepared"; now: number; countdownSeconds: number; preparationExpiresAt?: string; expiryMarginMs?: number }
  | { type: "tick"; now: number }
  | { type: "cancel" }
  | { type: "reset" };

export function reduceUpNextCountdown(
  state: UpNextCountdownState,
  event: UpNextCountdownEvent
): { state: UpNextCountdownState; effect: "none" | "handoff" } {
  if (event.type === "reset") return { state: { phase: "inactive" }, effect: "none" };
  if (event.type === "cancel") return { state: { phase: "cancelled" }, effect: "none" };
  if (event.type === "prepared") {
    if (event.countdownSeconds <= 0) return { state: { phase: "manual" }, effect: "none" };
    const configuredDeadline = event.now + Math.max(0, event.countdownSeconds) * 1000;
    const parsedExpiry = Date.parse(event.preparationExpiresAt ?? "");
    const expiryDeadline = Number.isFinite(parsedExpiry) ? parsedExpiry - Math.max(0, event.expiryMarginMs ?? 1500) : configuredDeadline;
    if (expiryDeadline <= event.now) return { state: { phase: "manual" }, effect: "none" };
    const deadlineAt = Math.min(configuredDeadline, expiryDeadline);
    return { state: { phase: "countdown", deadlineAt }, effect: "none" };
  }
  if (state.phase !== "countdown" || state.deadlineAt === undefined || event.now < state.deadlineAt) return { state, effect: "none" };
  return { state: { phase: "fired", deadlineAt: state.deadlineAt }, effect: "handoff" };
}

export type PlaybackSegmentLike = {
  id: string;
  type: string;
  startSeconds: number;
  endSeconds: number;
  automaticSafe?: boolean;
};

export type SegmentAutomationBehavior = "ask" | "automatic" | "off";
export type SegmentAutomationDecision =
  | { type: "none" }
  | { type: "prompt"; segment: PlaybackSegmentLike }
  | { type: "seek"; segment: PlaybackSegmentLike; positionSeconds: number };

export function playbackSegmentAutomationDecision(
  segments: readonly PlaybackSegmentLike[] | undefined,
  positionSeconds: number,
  dismissedSegmentIds: readonly string[],
  behavior: { intro: SegmentAutomationBehavior; credits: SegmentAutomationBehavior },
  isLive = false
): SegmentAutomationDecision {
  if (isLive || !Number.isFinite(positionSeconds)) return { type: "none" };
  const dismissed = new Set(dismissedSegmentIds);
  const segment = segments?.find((candidate) =>
    (candidate.type === "intro" || candidate.type === "credits") &&
    !dismissed.has(candidate.id) &&
    candidate.endSeconds > candidate.startSeconds &&
    positionSeconds >= candidate.startSeconds && positionSeconds < candidate.endSeconds
  );
  if (!segment) return { type: "none" };
  const preference = segment.type === "intro" ? behavior.intro : behavior.credits;
  if (preference === "off") return { type: "none" };
  if (preference === "automatic" && segment.automaticSafe === true) {
    return { type: "seek", segment, positionSeconds: segment.endSeconds };
  }
  return { type: "prompt", segment };
}
