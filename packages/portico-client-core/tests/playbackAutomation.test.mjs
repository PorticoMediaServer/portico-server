import assert from "node:assert/strict";
import test from "node:test";
import {
  createPlaybackAutomationState,
  playbackSegmentAutomationDecision,
  reducePlaybackAutomation,
  reduceUpNextCountdown
} from "../dist/playbackAutomation.js";

test("passout protection is deterministic across thresholds and interaction resets", () => {
  let state = createPlaybackAutomationState(1000);
  let result = reducePlaybackAutomation(state, { type: "automatic-advance-requested", now: 2000 }, { passoutProtection: true, passoutAfterEpisodes: 2 });
  assert.equal(result.effect, "advance");
  result = reducePlaybackAutomation(result.state, { type: "automatic-advance-requested", now: 3000 }, { passoutProtection: true, passoutAfterEpisodes: 2 });
  assert.equal(result.effect, "advance");
  result = reducePlaybackAutomation(result.state, { type: "automatic-advance-requested", now: 4000 }, { passoutProtection: true, passoutAfterEpisodes: 2 });
  assert.equal(result.effect, "confirm-still-watching");
  result = reducePlaybackAutomation(result.state, { type: "meaningful-interaction", now: 5000 }, { passoutProtection: true, passoutAfterEpisodes: 2 });
  assert.equal(result.state.automaticAdvances, 0);
  assert.equal(reducePlaybackAutomation(result.state, { type: "automatic-advance-requested", now: 5001 }, { passoutProtection: true, passoutAfterEpisodes: 2 }).effect, "advance");
});

test("passout protection detects two hours without interaction", () => {
  const state = createPlaybackAutomationState(0);
  const result = reducePlaybackAutomation(state, { type: "automatic-advance-requested", now: 2 * 60 * 60 * 1000 }, { passoutProtection: true, passoutAfterEpisodes: 5 });
  assert.equal(result.effect, "confirm-still-watching");
});

test("Up Next fires once and respects preparation expiry and cancellation", () => {
  const prepared = reduceUpNextCountdown({ phase: "inactive" }, { type: "prepared", now: 1000, countdownSeconds: 10, preparationExpiresAt: new Date(6000).toISOString(), expiryMarginMs: 1000 });
  assert.deepEqual(prepared.state, { phase: "countdown", deadlineAt: 5000 });
  assert.equal(reduceUpNextCountdown(prepared.state, { type: "tick", now: 4999 }).effect, "none");
  const fired = reduceUpNextCountdown(prepared.state, { type: "tick", now: 5000 });
  assert.equal(fired.effect, "handoff");
  assert.equal(reduceUpNextCountdown(fired.state, { type: "tick", now: 6000 }).effect, "none");
  const cancelled = reduceUpNextCountdown(prepared.state, { type: "cancel" });
  assert.equal(reduceUpNextCountdown(cancelled.state, { type: "tick", now: 9000 }).effect, "none");
  assert.deepEqual(reduceUpNextCountdown({ phase: "inactive" }, { type: "prepared", now: 1000, countdownSeconds: 0 }).state, { phase: "manual" });
  assert.deepEqual(reduceUpNextCountdown({ phase: "inactive" }, { type: "prepared", now: 6000, countdownSeconds: 10, preparationExpiresAt: new Date(6000).toISOString() }).state, { phase: "manual" });
});

test("segment automation is intro and credits only and skips only trusted markers once", () => {
  const intro = { id: "intro", type: "intro", startSeconds: 10, endSeconds: 45, automaticSafe: true };
  const credits = { id: "credits", type: "credits", startSeconds: 90, endSeconds: 110, automaticSafe: false };
  const ad = { id: "ad", type: "ad", startSeconds: 0, endSeconds: 9, automaticSafe: true };
  assert.equal(playbackSegmentAutomationDecision([intro, credits, ad], 20, [], { intro: "automatic", credits: "ask" }).type, "seek");
  assert.equal(playbackSegmentAutomationDecision([intro], 20, ["intro"], { intro: "automatic", credits: "ask" }).type, "none");
  assert.equal(playbackSegmentAutomationDecision([credits], 95, [], { intro: "ask", credits: "automatic" }).type, "prompt");
  assert.equal(playbackSegmentAutomationDecision([ad], 3, [], { intro: "automatic", credits: "automatic" }).type, "none");
  assert.equal(playbackSegmentAutomationDecision([intro], 20, [], { intro: "automatic", credits: "ask" }, true).type, "none");
});
