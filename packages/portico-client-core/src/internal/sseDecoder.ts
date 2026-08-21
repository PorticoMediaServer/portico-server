export interface PorticoSSEDecoderLimits {
  maximumUnreadBytes?: number;
  maximumFrameBytes?: number;
  maximumLineBytes?: number;
  maximumPayloadBytes?: number;
  maximumPreambleBytes?: number;
}

export type PorticoSSEProtocolErrorCode =
  | "invalid_content_type"
  | "invalid_event_payload"
  | "invalid_utf8"
  | "unread_buffer_too_large"
  | "frame_too_large"
  | "line_too_large"
  | "payload_too_large"
  | "preamble_too_large";

export class PorticoSSEProtocolError extends Error {
  readonly code: PorticoSSEProtocolErrorCode;

  constructor(code: PorticoSSEProtocolErrorCode, message: string, options?: ErrorOptions) {
    super(message, options);
    this.name = "PorticoSSEProtocolError";
    this.code = code;
  }
}

type RequiredLimits = Required<PorticoSSEDecoderLimits>;

const DEFAULT_LIMITS: RequiredLimits = Object.freeze({
  maximumUnreadBytes: 512 * 1024,
  maximumFrameBytes: 256 * 1024,
  maximumLineBytes: 64 * 1024,
  maximumPayloadBytes: 128 * 1024,
  maximumPreambleBytes: 8 * 1024
});

function limit(value: number | undefined, fallback: number, name: string): number {
  const result = value ?? fallback;
  if (!Number.isSafeInteger(result) || result < 1) throw new RangeError(`${name} is invalid`);
  return result;
}

function abortError(): Error {
  const error = new Error("The operation was aborted.");
  error.name = "AbortError";
  return error;
}

function byteLength(value: string): number {
  let bytes = 0;
  for (let index = 0; index < value.length; index += 1) {
    const code = value.charCodeAt(index);
    if (code < 0x80) bytes += 1;
    else if (code < 0x800) bytes += 2;
    else if (code >= 0xd800 && code <= 0xdbff && index + 1 < value.length && value.charCodeAt(index + 1) >= 0xdc00 && value.charCodeAt(index + 1) <= 0xdfff) {
      bytes += 4;
      index += 1;
    } else bytes += 3;
  }
  return bytes;
}

export function assertPorticoEventStreamContentType(contentType: string | null | undefined): void {
  const mediaType = contentType?.split(";", 1)[0]?.trim().toLowerCase();
  if (mediaType !== "text/event-stream") {
    throw new PorticoSSEProtocolError("invalid_content_type", "Portico event response is not text/event-stream");
  }
}

/** Incremental, bounded SSE data decoder. It emits data payloads, not parsed application events. */
export class PorticoSSEDecoder {
  readonly #limits: RequiredLimits;
  #decoder: TextDecoder | undefined;
  #line = "";
  #lineBytes = 0;
  #dataLines: string[] = [];
  #frameBytes = 0;
  #payloadBytes = 0;
  #preambleBytes = 0;
  #pendingCR = false;
  #atStart = true;
  #sawData = false;
  #finished = false;

  constructor(limits: PorticoSSEDecoderLimits = {}) {
    this.#limits = {
      maximumUnreadBytes: limit(limits.maximumUnreadBytes, DEFAULT_LIMITS.maximumUnreadBytes, "maximum unread bytes"),
      maximumFrameBytes: limit(limits.maximumFrameBytes, DEFAULT_LIMITS.maximumFrameBytes, "maximum frame bytes"),
      maximumLineBytes: limit(limits.maximumLineBytes, DEFAULT_LIMITS.maximumLineBytes, "maximum line bytes"),
      maximumPayloadBytes: limit(limits.maximumPayloadBytes, DEFAULT_LIMITS.maximumPayloadBytes, "maximum payload bytes"),
      maximumPreambleBytes: limit(limits.maximumPreambleBytes, DEFAULT_LIMITS.maximumPreambleBytes, "maximum preamble bytes")
    };
  }

  push(chunk: string | Uint8Array, signal?: AbortSignal): string[] {
    if (signal?.aborted) throw abortError();
    if (this.#finished) throw new TypeError("Portico SSE decoder is already finished");
    let text: string;
    try {
      if (typeof chunk === "string") {
        text = chunk;
      } else {
        if (!this.#decoder) {
          if (typeof globalThis.TextDecoder !== "function") {
            throw new TypeError("No UTF-8 decoder is available for byte event-stream chunks");
          }
          this.#decoder = new globalThis.TextDecoder("utf-8", { fatal: true });
        }
        text = this.#decoder.decode(chunk, { stream: true });
      }
    } catch (cause) {
      throw new PorticoSSEProtocolError("invalid_utf8", "Portico event stream contains invalid UTF-8", { cause });
    }
    return this.#consume(text, signal);
  }

  finish(signal?: AbortSignal): string[] {
    if (signal?.aborted) throw abortError();
    if (this.#finished) return [];
    this.#finished = true;
    let tail: string;
    try {
      tail = this.#decoder?.decode() ?? "";
    } catch (cause) {
      throw new PorticoSSEProtocolError("invalid_utf8", "Portico event stream ends with invalid UTF-8", { cause });
    }
    const events = this.#consume(tail, signal);
    if (this.#pendingCR) {
      this.#pendingCR = false;
      this.#completeLine(events, 1);
    }
    if (this.#line) this.#completeLine(events, 0);
    this.#dispatch(events);
    return events;
  }

  #consume(text: string, signal?: AbortSignal): string[] {
    const events: string[] = [];
    for (let index = 0; index < text.length; index += 1) {
      if (signal?.aborted) throw abortError();
      const character = text[index]!;
      if (this.#atStart) {
        this.#atStart = false;
        if (character === "\uFEFF") continue;
      }
      if (this.#pendingCR) {
        this.#pendingCR = false;
        this.#completeLine(events, character === "\n" ? 2 : 1);
        if (character === "\n") continue;
      }
      if (character === "\r") {
        this.#pendingCR = true;
        if (this.#frameBytes + this.#lineBytes + 1 > this.#limits.maximumUnreadBytes) {
          this.#fail("unread_buffer_too_large", "Portico event-stream unread buffer exceeds its limit");
        }
      } else if (character === "\n") {
        this.#completeLine(events, 1);
      } else {
        this.#line += character;
        this.#lineBytes += byteLength(character);
        if (this.#lineBytes > this.#limits.maximumLineBytes) this.#fail("line_too_large", "Portico event-stream line exceeds its limit");
        if (this.#lineBytes + this.#frameBytes > this.#limits.maximumUnreadBytes) this.#fail("unread_buffer_too_large", "Portico event-stream unread buffer exceeds its limit");
      }
    }
    return events;
  }

  #completeLine(events: string[], terminatorBytes: number): void {
    const line = this.#line;
    this.#line = "";
    const lineBytes = this.#lineBytes + terminatorBytes;
    this.#lineBytes = 0;
    this.#frameBytes += lineBytes;
    if (this.#frameBytes > this.#limits.maximumFrameBytes) this.#fail("frame_too_large", "Portico event-stream frame exceeds its limit");
    if (this.#frameBytes > this.#limits.maximumUnreadBytes) this.#fail("unread_buffer_too_large", "Portico event-stream unread buffer exceeds its limit");
    if (!this.#sawData) {
      this.#preambleBytes += lineBytes;
      if (this.#preambleBytes > this.#limits.maximumPreambleBytes) this.#fail("preamble_too_large", "Portico event-stream preamble exceeds its limit");
    }
    if (line === "") {
      this.#dispatch(events);
      return;
    }
    if (line.startsWith(":")) return;
    const colon = line.indexOf(":");
    const field = colon < 0 ? line : line.slice(0, colon);
    let value = colon < 0 ? "" : line.slice(colon + 1);
    if (value.startsWith(" ")) value = value.slice(1);
    if (field !== "data") return;
    this.#sawData = true;
    const addedBytes = byteLength(value) + (this.#dataLines.length ? 1 : 0);
    this.#payloadBytes += addedBytes;
    if (this.#payloadBytes > this.#limits.maximumPayloadBytes) this.#fail("payload_too_large", "Portico event-stream payload exceeds its limit");
    this.#dataLines.push(value);
  }

  #dispatch(events: string[]): void {
    if (this.#dataLines.length) events.push(this.#dataLines.join("\n"));
    this.#dataLines = [];
    this.#frameBytes = 0;
    this.#payloadBytes = 0;
  }

  #fail(code: PorticoSSEProtocolErrorCode, message: string): never {
    throw new PorticoSSEProtocolError(code, message);
  }
}

/** JSON syntax/schema failures are protocol failures; callback failures pass through unchanged. */
export function dispatchPorticoJSONEvent<T>(
  payload: string,
  decode: (value: unknown) => T,
  consumer: (event: T) => void
): void {
  let event: T;
  try {
    event = decode(JSON.parse(payload) as unknown);
  } catch (cause) {
    throw new PorticoSSEProtocolError("invalid_event_payload", "Portico event payload does not match its advertised JSON contract", { cause });
  }
  consumer(event);
}
