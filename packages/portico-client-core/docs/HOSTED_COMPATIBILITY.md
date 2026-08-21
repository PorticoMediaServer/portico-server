# Hosted Services compatibility negotiation

Every `HostedServicesClient` checks the public `GET /api/system` contract before
its first authentication, account-data, server-data, or mutation request. The
check is single-flight and cached for that client instance. An explicit
`checkCompatibility()` method is also available for startup screens; `system()`
returns the unvalidated public document for diagnostics.

Portico is unreleased, so Hosted Services and every client use the single API
token `v1`. Client Core exports it as `PORTICO_API_VERSION` and requires an
exact match. Database migrations remain internal and independently numbered.

Failures throw `HostedCompatibilityError` with a stable `code`, the actual
version, and the required version.
An endpoint that does not identify a healthy Portico service fails closed before
credentials are sent.

Unknown response properties do not make a compatible document fail. Optional
capabilities can evolve without creating another API version before release.

The published fixture
`@portico/client-core/fixtures/hosted-api-v1-conformance.json` is byte-equivalent
to the canonical Cloud fixture under
`api/openapi/fixtures/hosted-api-v1-conformance.json`. Separate client
repositories should use the packaged fixture in their compatibility tests.
