# Portico API guide

The complete contract is [portico.openapi.json](../portico.openapi.json), generated from the same typed registry that serves production requests. Route paths, operation identifiers, access rules, credential scopes, admission classes, request limits and wire schemas come from that registry.

These guides explain the API surfaces for external tools:

- [API keys, personal access tokens and scopes](07-keys-tokens-and-scopes.md)
- [Bounded title queries and filesystem change reports](08-for-tools.md)
- [Outbound webhooks](09-webhooks.md)
- [The local MCP server](10-mcp.md)

Check the guide's examples with `node server/api/guide/check-examples.mjs`.

The former hand-written endpoint reference and retired playback protocol examples have been removed. Use the generated contract for all current playback, identity, browsing, personal state, Live TV, DVR, download and administration operations.
