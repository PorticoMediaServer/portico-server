# Client Core Distribution and Release

`@porticomediaserver/client-core` is a separately versioned package whose source
stays beside the canonical Portico Server OpenAPI contract. Keeping the source
here prevents contract drift; releasing a package artifact lets every client
build from an immutable dependency without checking out the server repository.

Portico publishes verified `.tgz` files on dedicated GitHub prereleases. Client
Core is not published to npm and requires no registry credentials. Its release
tags use `client-core-vMAJOR.MINOR.PATCH`, which keeps these prereleases out of
the Portico Media Server `latest` release channel.

## Entrypoints

- `@porticomediaserver/client-core/core` contains deterministic product types and helpers.
  It typechecks with only the ECMAScript library and must not expose DOM or
  platform framework globals.
- `@porticomediaserver/client-core/browser` is the browser-capable surface and may use
  standards-based browser defaults.
- `@porticomediaserver/client-core/native` is the React Native/native TypeScript surface.
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
5. Push the reviewed commit, then create and push the matching
   `client-core-vMAJOR.MINOR.PATCH` tag. The Client Core release workflow builds,
   verifies, checksums, and publishes the tarball as a GitHub prerelease.
6. Pin consumers to the exact tagged asset URL and commit their package lock.
   Never use a moving URL for a build dependency.

The consumer fixture intentionally has no dependency on this checkout. The
gate installs the tarball it just built, so source-tree resolution cannot hide
missing files, invalid exports, or developer-local dependency ranges.
## Release verification

Run `npm run release:verify` from this package. The gate checks Server OpenAPI
freshness without rewriting committed files, Product Language, types, build,
tests, the public-export snapshot, and the npm dependency audit.

The source is GPL-3.0-or-later. The package remains `private: true` only to
prevent accidental publication to npm. Every package contains the compiled
JavaScript, declaration files, documentation, conformance fixtures, README,
and GPL license. The GitHub release also links to the complete tagged source.

Native shells must provide the standard WHATWG networking primitives declared
by their platform (`fetch`, URL, request/response, abort, form-data, and blob).
The no-DOM fixture models those host declarations instead of loading the DOM
library.
