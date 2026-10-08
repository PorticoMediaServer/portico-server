
### Immutable artwork representations

Selected artwork URLs carry `v` (the content digest) and `w=400`, `w=800`, or
`w=1920` (maximum long edge, without enlargement). The default is 1920. The
published thumbnail URLs use 400. Images preserve aspect ratio and alpha.
The same selected display digest may be used for all three widths; each response
has an ETag for its exact bytes. Versioned responses use
`Cache-Control: public, max-age=31536000, immutable`; unversioned responses use
`private, no-cache`. Authorization still precedes every server response.
A replaced version returns 410 rather than new bytes under an old address.
Pending or missing installed artwork returns 404 with `Retry-After: 5` and
`Cache-Control: no-store`; repair is queued outside the request's read snapshot.

Catalog sorting uses a stored Unicode-folded key. An explicit `sortTitle` takes
precedence. Otherwise the leading article is stripped according to the entity's
`metadataLanguage` (a BCP 47 language tag, also editable in the metadata editor).
An unknown language retains the title's words. Accents fold for ordering and the
letter index; non-Latin letters retain their own index group. Screen-provider
publication records the selected metadata language, respecting an owner lock.
