# Explicit publication types

Root ruling 20 defines three descriptive values: `book`, `magazine`, and `newspaper`. This classification does not change the ebook/comic entity kind or the library kind.

Recognized automatic fields in classifier version 1:

- EPUB: `dc:type` children in the package metadata, with the exact Dublin Core namespace `http://purl.org/dc/elements/1.1/`.
- CBZ: the `Format` element in the unnamespaced `ComicInfo` root of the admitted `ComicInfo.xml` member.

Each value is trimmed and compared case-insensitively with the three exact semantic names. Conflicting recognized values make the type unknown. Generic `periodical` or `serial`, subjects, MIME, file extension, titles, folders, issue dates and cadence supply no type. PDF explicit-type extraction is not yet supported, so its parser supplies no automatic type.

The file fact is adopted only after real source inspection. Its write transaction rechecks the original principal, item, source, inventory object and revision. The accepted automatic fact comes only from the current canonical primary publication and current inventory/root binding. Unsupported formats do not clear facts written by a later supported extractor.

An explicit locked owner value uses the existing metadata edit path and takes precedence. Empty, automatic or unlocked owner edits restore the current file fact or absence. An unknown type never creates a stored Book default, and a file inspection never overwrites the owner row.

This producer does not infer periodical groups or dates and does not make an unclassified ebook disappear from the base Books view. Periodical tabs, counts and groups require their own complete ready-snapshot producers. Official API/schema metadata generation belongs to the Release checkpoint batch.
