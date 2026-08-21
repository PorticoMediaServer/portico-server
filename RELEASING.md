# Public release process

This is a public description of how official Portico builds are produced. It
is not an invitation to publish unofficial builds under Portico's name; see
`TRADEMARKS.md`.

## Two separate release decisions

FFmpeg and Portico are deliberately released independently. A normal Portico
release reuses the currently approved FFmpeg toolchain. FFmpeg changes only
after Justin selects and tests a stable upstream version and manually starts
the **Qualified FFmpeg toolchain** workflow.

An application release starts from a reviewed `vMAJOR.MINOR.PATCH` Git tag.
The release workflow reruns server and web tests, downloads the exact pinned
FFmpeg component release, and builds:

- Windows x64 and ARM64 installers plus portable ZIPs;
- an Apple Silicon macOS DMG;
- Linux x64 and ARM64 archives, DEB files, and RPM files; and
- a Linux amd64/arm64 image in GitHub Container Registry.

It then writes SHA-256 checksums, an SBOM, build provenance, and a small update
manifest before publishing the GitHub Release. The release is intentionally
labelled as a development preview while Portico remains prerelease-quality.

GitHub's `/releases/latest` URL is the permanent human-facing download link.
The versionless asset names and `portico-update-manifest.json` are the stable
machine-facing locations. Automatic installation remains disabled until
packages are signed and the update client has passed rollback testing.

Only a maintainer may create official releases. The operational checklist,
approval records, rollback steps, and service credentials are maintained in
Portico's private internal repository.
