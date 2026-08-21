# Client Core Distribution and Release

`@portico/client-core` is a versioned package consumed from a packed artifact.
The package is not published by any local or CI command. External publication
requires an explicit release decision, registry credentials, and a separately
authorized release job.

## Entrypoints

- `@portico/client-core/core` contains deterministic product types and helpers.
  It typechecks with only the ECMAScript library and must not expose DOM or
  platform framework globals.
- `@portico/client-core/browser` is the browser-capable surface and may use
  standards-based browser defaults.
- `@portico/client-core/native` is the React Native/native TypeScript surface.
  Platform networking, secure storage, cryptography, discovery, and player
  implementations remain injected adapters.
- The root entrypoint preserves the browser/web integration surface. Native
  applications should import the explicit `/native` or `/core` entrypoint.

## Compatibility and semver

Client Core and the server contract are related but independently versioned.
The package version follows semantic versioning. Before `1.0.0`, consumers
should pin an exact version: patch releases remain compatible, while a minor
release may contain an intentional pre-launch breaking change. At and after
`1.0.0`, breaking package API changes require a major version.

Server compatibility is negotiated at runtime with the API revision and
Product Contract schema ranges exported by `compatibility.ts`. Adding an
optional server capability does not require a Client Core major version.
Changing a required wire shape requires coordinated OpenAPI generation,
runtime conformance, Client Core changes, and a package version decision.

## Release procedure

1. Regenerate and verify the public Server OpenAPI contract. Hosted contract
   regeneration is performed only in Portico's private integration workspace.
2. Decide the semantic version change and update `package.json` and its lock.
3. Run `npm run release:verify` in this package to verify the public API,
   Product Language catalog, types, tests, exports, and dependency audit.
4. Run the workspace release gate and record the commit, contract revisions,
   package version, commands, and results as release evidence.
5. Store or publish the exact verified tarball only through a separately
   authorized release workflow. Never publish from an uncommitted workstation.

The consumer fixture intentionally has no dependency on this checkout. The
gate installs the tarball it just built, so source-tree resolution cannot hide
missing files, invalid exports, or developer-local dependency ranges.
## Release verification

Run `npm run release:verify` from this package. The gate checks Server OpenAPI
freshness without rewriting committed files, Product Language, types, build,
tests, the public-export snapshot, and the npm dependency audit.

The source is GPL-3.0-or-later. The package remains `private: true` only to
prevent accidental publication to npm; Portico currently distributes it as
source within this repository rather than as a registry package.

Native shells must provide the standard WHATWG networking primitives declared
by their platform (`fetch`, URL, request/response, abort, form-data, and blob).
The no-DOM fixture models those host declarations instead of loading the DOM
library.
