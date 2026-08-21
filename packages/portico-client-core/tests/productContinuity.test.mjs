import assert from "node:assert/strict";
import test from "node:test";

import {
  reserveOrderedSurfaceSlots,
  resolveProductContinuity,
  resolveReservedSurfaceSlot
} from "../dist/productContinuity.js";

test("successful content stays mounted through refresh and transient failure", () => {
  const data = { rows: ["continue-watching"] };
  assert.deepEqual(resolveProductContinuity({ data, refreshing: true }), {
    phase: "refreshing", presentation: "content", data, failure: undefined, showFailure: false
  });
  const failure = { kind: "transient", messageId: "problem.server-unavailable" };
  assert.deepEqual(resolveProductContinuity({ data, failure }), {
    phase: "stale", presentation: "content", data, failure, showFailure: false
  });
});

test("terminal server failure reports in place while fail-closed identity clears old content", () => {
  const data = { title: "Retained page" };
  const unavailable = { kind: "unavailable", messageId: "problem.server-unavailable" };
  assert.deepEqual(resolveProductContinuity({ data, failure: unavailable }), {
    phase: "unavailable", presentation: "content", data, failure: unavailable, showFailure: true
  });
  const blocked = { kind: "security-blocked" };
  assert.deepEqual(resolveProductContinuity({ data, failure: blocked }), {
    phase: "security-blocked", presentation: "terminal-error", failure: blocked, showFailure: true
  });
});

test("unresolved connections reserve geometry without showing premature errors", () => {
  assert.deepEqual(resolveProductContinuity({ restoring: true }), {
    phase: "restoring", presentation: "reserved", failure: undefined, showFailure: false
  });
  const transient = { kind: "transient" };
  assert.deepEqual(resolveProductContinuity({ connecting: true, failure: transient }), {
    phase: "connecting", presentation: "reserved", failure: transient, showFailure: false
  });
});

test("customized descriptor order reserves every row and retains resolved slots", () => {
  const initial = reserveOrderedSurfaceSlots([
    { id: "custom-a", title: "Custom A" },
    { id: "continue", title: "Continue Watching" },
    { id: "custom-b", title: "Custom B" }
  ], row => row.id);
  const resolved = resolveReservedSurfaceSlot(initial, "continue", "ready", [{ id: "media-1" }]);
  const empty = resolveReservedSurfaceSlot(resolved, "custom-a", "empty");
  const reordered = reserveOrderedSurfaceSlots([
    { id: "custom-b", title: "Renamed B" },
    { id: "continue", title: "Continue Watching" },
    { id: "custom-a", title: "Custom A" }
  ], row => row.id, empty);

  assert.deepEqual(reordered.map(slot => slot.id), ["custom-b", "continue", "custom-a"]);
  assert.equal(reordered[1].resolution, "ready");
  assert.deepEqual(reordered[1].data, [{ id: "media-1" }]);
  assert.equal(reordered[2].resolution, "empty");
  assert.equal(reordered[0].descriptor.title, "Renamed B");
});

test("reserved slots reject ambiguous or invented identifiers", () => {
  assert.throws(() => reserveOrderedSurfaceSlots([{ id: "same" }, { id: "same" }], row => row.id), /duplicate/);
  const slots = reserveOrderedSurfaceSlots([{ id: "known" }], row => row.id);
  assert.throws(() => resolveReservedSurfaceSlot(slots, "invented", "empty"), /not advertised/);
  assert.throws(() => resolveReservedSurfaceSlot(slots, "known", "ready"), /require data/);
});
