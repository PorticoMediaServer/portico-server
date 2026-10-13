# Portico server API

Start with the [developer guide](guide/README.md). The route registry produces
[portico.openapi.json](portico.openapi.json) and the TypeScript contract in
`packages/contracts`. Registered operations publish `x-access`, `x-scope`, body
bounds and their request/response shapes.

The current registry is not yet the whole router. Authentication and some
catalogue routes remain hand-registered. For those, the guide cites the handler
and service source. The hand-written YAML files are historical descriptions,
not a second authoritative API; some tests and the existing example checker
still read them. They must remain until those consumers move to the registry.
Do not build a new integration from their obsolete sign-in or playback shapes.

This is a prerelease API with no compatibility promise. The current public
routes use `/v1`; the former `/v2` playback model has been removed. There are no
legacy sign-in shortcuts to preserve. Library and content screens are composed
by the server. Tools can use the additional bounded title query.

The checked-in generated contract also needs regeneration as source-registered
routes move forward. For example, current account-session routes are registered
in `internal/httpapi/foundation.go` but are missing from this generated snapshot.
Use their documented current source until that snapshot is refreshed.
