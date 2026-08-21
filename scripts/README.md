# Build and packaging scripts

Normal development uses `make test` and `make build-server`. Public packages
are produced by `.github/workflows/release.yml`; the platform scripts here are
small, reviewable building blocks used by that workflow.

- `build-web-release.sh` builds the shared client core and browser interface.
- `build-release-payload.sh` cross-compiles the server and combines it with an
  already-qualified FFmpeg/FFprobe bundle.
- `package-linux.sh` creates tar.gz, DEB, and RPM artifacts.
- `package-windows.sh` creates a portable ZIP and unsigned NSIS installer.
- `package-macos.sh` creates an unsigned Apple Silicon application DMG.
- `normalize-ffmpeg-bundle.sh` creates a consistent component layout.
- `verify-ffmpeg-bundle.sh` enforces the GPL/no-nonfree and capability policy.
- `write-public-update-manifest.mjs` emits machine-readable download metadata.

Release engineers should use the GitHub workflows rather than assembling a
public release by hand. The private release runbook explains the approval and
rollback process in plain language.
