import assert from "node:assert/strict";
import test from "node:test";
import {
  PorticoSSEDecoder,
  PorticoSSEProtocolError,
  assertPorticoEventStreamContentType,
  dispatchPorticoJSONEvent
} from "../dist/internal/sseDecoder.js";

test("incremental SSE decoding handles CRLF, split UTF-8, and multiple data lines", () => {
  const decoder = new PorticoSSEDecoder();
  const encoded = new TextEncoder().encode("data: {\"name\":\"café\"}\r\ndata: second\r\n\r\n");
  const split = encoded.indexOf(0xc3) + 1;
  assert.deepEqual(decoder.push(encoded.slice(0, split)), []);
  assert.deepEqual(decoder.push(encoded.slice(split)), ["{\"name\":\"café\"}\nsecond"]);
  assert.deepEqual(decoder.finish(), []);
});

test("SSE decoder flushes a final frame with no delimiter", () => {
  const decoder = new PorticoSSEDecoder();
  assert.deepEqual(decoder.push("data: {\"ok\":true}"), []);
  assert.deepEqual(decoder.finish(), ["{\"ok\":true}"]);
});

test("SSE decoder enforces content type and every bounded accumulation surface", () => {
  assert.doesNotThrow(() => assertPorticoEventStreamContentType("Text/Event-Stream; charset=utf-8"));
  assert.throws(() => assertPorticoEventStreamContentType("application/json"), error => error.code === "invalid_content_type");
  const cases = [
    [{ maximumLineBytes: 4 }, "data: x", "line_too_large"],
    [{ maximumFrameBytes: 7 }, "x: 1234\n", "frame_too_large"],
    [{ maximumUnreadBytes: 6, maximumFrameBytes: 100 }, "x: 1234", "unread_buffer_too_large"],
    [{ maximumPayloadBytes: 3 }, "data: 1234\n", "payload_too_large"],
    [{ maximumPreambleBytes: 4 }, ":1234\n", "preamble_too_large"]
  ];
  for (const [limits, input, code] of cases) {
    assert.throws(() => new PorticoSSEDecoder(limits).push(input), error => error instanceof PorticoSSEProtocolError && error.code === code);
  }
});

test("SSE decoder rejects oversized no-delimiter frames and invalid UTF-8", () => {
  assert.throws(
    () => new PorticoSSEDecoder({ maximumLineBytes: 8 }).push("data: never-delimited"),
    error => error.code === "line_too_large"
  );
  assert.throws(
    () => new PorticoSSEDecoder().push(Uint8Array.of(0xc3, 0x28)),
    error => error.code === "invalid_utf8"
  );
});

test("SSE framing limits count CRLF bytes exactly, ignore EOF terminators, and accept a BOM", () => {
  assert.doesNotThrow(() => new PorticoSSEDecoder({ maximumFrameBytes: 8 }).push("x: 123\r\n"));
  assert.throws(
    () => new PorticoSSEDecoder({ maximumFrameBytes: 7 }).push("x: 123\r\n"),
    error => error.code === "frame_too_large"
  );
  const noDelimiter = new PorticoSSEDecoder({ maximumFrameBytes: 7 });
  assert.deepEqual(noDelimiter.push("data: x"), []);
  assert.deepEqual(noDelimiter.finish(), ["x"]);
  const bom = new PorticoSSEDecoder();
  assert.deepEqual(bom.push("\uFEFFdata: {\"ok\":true}\n\n"), ["{\"ok\":true}"]);
});

test("SSE cancellation and consumer callback errors remain distinguishable from protocol failures", () => {
  const controller = new AbortController();
  controller.abort();
  assert.throws(() => new PorticoSSEDecoder().push("data: x\n\n", controller.signal), error => error.name === "AbortError");
  assert.throws(
    () => dispatchPorticoJSONEvent("not-json", value => value, () => undefined),
    error => error instanceof PorticoSSEProtocolError && error.code === "invalid_event_payload"
  );
  const consumerError = new Error("render failed");
  assert.throws(
    () => dispatchPorticoJSONEvent("{\"ok\":true}", value => value, () => { throw consumerError; }),
    error => error === consumerError
  );
});
