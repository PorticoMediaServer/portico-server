# Hosted Connection Runtime Adapters

`@portico/client-core` keeps Hosted route selection and signature verification platform-neutral. Browsers use standards-based globals by default. React Native and other non-browser clients should pass a `runtime` object to `connectHostedServer` rather than installing global polyfills.

## Adapter contract

`HostedConnectionRuntimeAdapters` accepts these facilities:

- `fetch`: WHATWG-compatible networking used only to probe the selected server route. The Hosted and local API clients should receive the same platform transport independently through their own client options.
- `decodeBase64`: decodes standard, padded base64 into bytes. Portico normalizes base64url signatures before calling this adapter.
- `encodeText`: encodes a string as UTF-8 bytes.
- `verifyEd25519`: verifies a detached Ed25519 signature over the supplied message with the supplied 32-byte raw public key. It must fail closed and must not substitute another algorithm.
- `createAbortController`: creates the controller used to cancel a timed-out route probe.
- `setTimeout` and `clearTimeout`: paired platform timer functions. Handles are intentionally opaque.
- `now`: returns the current wall-clock `Date`, used to validate the short-lived Hosted route document.

Every field is optional in browsers. A missing browser global produces a `HostedRuntimeCapabilityError` naming the missing capability and the adapter that the platform must provide. `createHostedConnectionRuntime` can be used to resolve and validate a partial adapter set while retaining browser defaults for the rest.

## React Native integration

React Native clients should construct the adapter once in the application infrastructure layer and reuse it for Hosted connections:

```ts
import { connectHostedServer, type HostedConnectionRuntimeAdapters } from "@portico/client-core/native";

const hostedRuntime: HostedConnectionRuntimeAdapters = {
  fetch: platformFetch,
  decodeBase64: (value) => base64Library.decodeToBytes(value),
  encodeText: (value) => utf8Library.encode(value),
  verifyEd25519: ({ publicKey, signature, message }) =>
    ed25519Library.verify(signature, message, publicKey),
  createAbortController: () => new AbortController(),
  setTimeout: (callback, milliseconds) => setTimeout(callback, milliseconds),
  clearTimeout: (handle) => clearTimeout(handle as ReturnType<typeof setTimeout>),
  now: () => new Date()
};

await connectHostedServer(server, {
  hostedClient,
  localClient,
  sessionStore,
  trustedHostedDocumentKeys,
  runtime: hostedRuntime
});
```

Choose maintained platform libraries that return raw bytes and support detached Ed25519 verification. Keep key and signature values out of logs. Do not bypass signature verification when a capability is unavailable; surface the capability error and block the connection.

The runtime adapter does not own bearer-token persistence, refresh-token rotation, native media playback, certificate trust, or LAN discovery. Those remain explicit platform/client responsibilities.

## Server-Sent Event adapter

The local server client has a separate, optional `eventStream` boundary. It is
separate because React Native fetch implementations do not consistently expose
the browser `Response.body.getReader()` or global `TextDecoder` APIs.

Browsers need no configuration. A native shell supplies an async chunk source;
it may yield already-decoded strings, or bytes together with a stateful UTF-8
decoder:

```ts
const client = createPorticoClient({
  transport: { fetch: platformFetch },
  eventStream: {
    async *read(response, signal) {
      for await (const chunk of nativeStreamingBody(response, signal)) yield chunk;
    },
    decode(chunk, options) {
      return nativeUtf8Decoder.decode(chunk, options);
    },
    flush() {
      return nativeUtf8Decoder.decode();
    }
  }
});
```

The adapter owns only response-body iteration and UTF-8 decoding. Client Core
still owns authentication, one-time token refresh, request IDs, HTTP failure
normalization, SSE framing, JSON validation, and abort lifecycle. An adapter
that yields bytes must provide `decode`; one that yields strings requires no
decoder. Do not buffer an unbounded event stream into `response.text()`.
