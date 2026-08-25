import assert from "node:assert/strict";
import test from "node:test";
import {
  positiveFullJitterDelay,
  stableCohortHash,
} from "../dist/index.js";

test("persisted retry cohorts retain a bounded stable schedule across restarts", () => {
  const first = positiveFullJitterDelay(5_000, "installation-1", 2, () => 0);
  const restarted = positiveFullJitterDelay(5_000, "installation-1", 2, () => 0.999);
  assert.equal(restarted, first);
  assert.ok(first >= 1 && first <= 5_000);
  assert.notEqual(
    positiveFullJitterDelay(5_000, "installation-2", 2, () => 0),
    first,
  );
  assert.notEqual(
    positiveFullJitterDelay(5_000, "installation-1", 3, () => 0),
    first,
  );
});

test("unpersisted callers receive positive full jitter inside the cap", () => {
  assert.equal(positiveFullJitterDelay(500, "", 0, () => 0), 1);
  assert.equal(positiveFullJitterDelay(500, "", 0, () => 0.999999999), 500);
  assert.equal(positiveFullJitterDelay(0, "", 0, () => 0), 1);
});

test("cohort hash is deterministic and unsigned", () => {
  assert.equal(stableCohortHash("portico"), stableCohortHash("portico"));
  assert.ok(stableCohortHash("portico") >= 0);
  assert.ok(stableCohortHash("portico") <= 0xffffffff);
});
