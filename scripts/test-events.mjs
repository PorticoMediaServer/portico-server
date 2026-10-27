// Structured, incremental node:test reporter used by scripts/ci-affected.py.
//
// Every test start/pass/fail/cancel and the per-file summary are appended to
// PORTICO_TEST_EVENTS as one JSON object per line, flushed as they happen. A run
// that is interrupted or killed therefore still shows exactly which tests
// started, passed, failed or were cancelled before it stopped, and the watchdog
// uses the same stream to name a test that is still running when its file hangs.
//
// This replaces the human-readable spec reporter for supervised runs: the caller
// prints the per-file result and failure messages from these events, and the
// watchdog keeps the per-file stdout/stderr log next to them.
import {closeSync, openSync, writeSync} from 'node:fs';

const destination = process.env.PORTICO_TEST_EVENTS;
const live = process.env.PORTICO_TEST_LIVE_EVENTS;

if (!destination) {
  throw new Error('PORTICO_TEST_EVENTS must name the structured event file for a supervised test file');
}

const fd = openSync(destination, 'a');
// One shared, append-only stream for the whole supervised run: if the runner
// itself is killed, the last line still names the file and test that was live.
const liveFd = live ? openSync(live, 'a') : null;
const RECORDED = new Set([
  'test:enqueue', 'test:dequeue', 'test:start', 'test:pass', 'test:fail',
  'test:cancel', 'test:complete', 'test:summary',
]);

const detail = data => {
  const error = data.details && data.details.error;
  return {
    name: data.name,
    nesting: data.nesting,
    file: data.file,
    testNumber: data.testNumber,
    // Per-test duration is on details.duration_ms; the file summary carries it
    // on the event data itself.
    duration_ms: (data.details && data.details.duration_ms) ?? data.duration_ms,
    skip: data.skip,
    todo: data.todo,
    counts: data.counts,
    success: data.success,
    error: error && {
      message: error.message,
      failureType: error.failureType,
      stack: typeof error.stack === 'string' ? error.stack.slice(0, 4000) : undefined,
    },
  };
};

export default async function* report(source) {
  try {
    for await (const event of source) {
      if (!RECORDED.has(event.type)) continue;
      // One writeSync per event: a partial line is impossible, and nothing is
      // buffered in userspace when the run is stopped between two events.
      const line = JSON.stringify({type: event.type, at: Date.now(), pid: process.pid, data: detail(event.data || {})}) + '\n';
      writeSync(fd, line);
      if (liveFd !== null) writeSync(liveFd, line);
    }
  } finally {
    closeSync(fd);
    if (liveFd !== null) closeSync(liveFd);
  }
}
