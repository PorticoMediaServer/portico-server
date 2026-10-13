# Portico server API

[portico.openapi.json](portico.openapi.json) is the complete API contract. It is generated from the typed registry used by the production router, including conditional features and raw media, upload and event-stream operations. The server and first-party clients use the same API as external clients.

Regenerate the contract from `server/`:

```sh
GOFLAGS=-p=2 go run ./cmd/apigen
```

The same command updates `packages/contracts/src` and the generated admission table. The TypeScript contract exports typed route builders, parameter types, request and response types, schemas and decoders. Supply unescaped parameters to `routePath`; `encodedRoutePath` supports existing callers that have already encoded their parameters. Existing shared-client parsers retain their row-level tolerance and public service interfaces.

Each operation publishes its credential scope (`x-scope`), access class (`x-access`), work class (`x-lane`), database cost class (`x-cost`) and admission pool (`x-admission`). An API key or personal access token never widens the member's authority. Session-only operations cannot be invoked with those credentials. Media grants and other narrow protocol credentials retain their own handler checks.

Plain HTTP and direct access remain supported. Optional features still depend on the services configured by the owner. Read the operation's response schema and error envelope, respect `Retry-After`, and retain opaque cursors and revisions exactly as returned.

For external automation, see the [tool integration guide](guide/README.md). Old hand-authored specifications and retired playback documentation are no longer published.
