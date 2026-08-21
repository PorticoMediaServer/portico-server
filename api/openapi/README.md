# Portico server contract generation

`portico-server.openapi.json` is the published OpenAPI 3.1 contract. Do not edit it directly.

Run:

```sh
make api-generate
```

This normalizes the schema catalog, requires a unique operation ID for every operation, adds the standard RFC problem responses, writes the embedded runtime contract, writes the published OpenAPI document, and regenerates both TypeScript consumers.

`make api-server-check` is the server-only generated-artifact no-diff gate.
`make contract-check` also validates representative Product Contract, Home row,
and playback payloads against the published schemas. CI runs this combined gate.
`make api-check` extends it by regenerating the TypeScript consumer and checking
that the committed client-core types are unchanged.

## Contract boundary

The Go route registry is mandatory for every `/api` route and owns exact-method dispatch, authentication classification, operation inventory, and the runtime/OpenAPI drift boundary. DLNA and static web routes remain outside it.

`openapi.yaml` is generator input, not the published artifact. Every successful operation must name a concrete request and response schema where applicable. Generation fails when an operation would require an untyped or synthetic fallback.

Multiplexed handlers can be declared under `x-portico-complete-handler-families` only when every supported branch and method is represented by a concrete path with a named success schema. Complete families do not publish or mount a catch-all adapter; unknown child resources receive the registry's standard problem-details `404`.
