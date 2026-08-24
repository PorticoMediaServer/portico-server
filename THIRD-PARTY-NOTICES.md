# Third-party notices

Portico Media Server depends on third-party software. Each dependency remains
available under its own license; those licenses are not replaced by Portico's
GPL-3.0-or-later license.

The authoritative dependency inventory for the server is `go.mod` and `go.sum`.
The web application and shared client library inventories are their respective
`package.json` and `package-lock.json` files. Copyright and license information
for those dependencies is available in their source distributions and installed
package metadata.

Release packages include a separate FFmpeg and FFprobe bundle built from the
pinned recipe recorded in `media-toolchain/sources.lock.json`. The binaries are
built with GPL and version-3 licensing enabled and without FFmpeg's `nonfree`
configuration. Applicable notices and license texts are included with each
package. The corresponding source archive is attached to the matching FFmpeg
toolchain release in this GitHub repository.

## Metadata services

Portico includes intentionally distributed application credentials for TMDB
and TheTVDB. They identify Portico to those metadata services and are not user
or server secrets. Owners may configure their own credentials. The server UI
includes the providers' required attribution and direct links.

Portico's web interface includes Manrope under the SIL Open Font License 1.1.
The font license is included beside the distributed font files.

The client icon system includes Lucide artwork under the ISC License. The
relevant license and provenance records are retained with the generated assets.
