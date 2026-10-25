# Typed periodical issue membership

This endpoint reads one existing accepted series in one book library. It does not infer a publication type, series, issue date or cadence. Every member must be in the same immutable ready attribution generation with the requested accepted `magazine` or `newspaper` type, actual series ID and accepted issue date. Newspaper membership requires day precision; a month never supplies an invented newspaper day. The ordinary current canonical-primary publication and viewer/library/profile restriction predicate run before pagination in the same snapshot.

The fixed canonical order is issue-date string descending, then actual item ID descending. ISO month precision is retained as a month value; mixed precision has deterministic lexical placement and does not imply a calendar day. No client sample sort or view direction affects membership. The type/date/series ready index supports the seek.

The existing signed Reading cursor framing is reused with a distinct periodical-issues-v1 plus requested-type scope. It binds the current ready namespace/input/generation, catalogue and viewer revisions, profile/authority/restriction fence, library, exact series, type, page limit, complete date/item tuple digest and original expiry. Cross-series, cross-type, generic-series and stale-generation reuse refuse. An anchor is resolved again under that exact current eligibility predicate before its full tuple is compared. Subsequent pages retain original expiry.

The response publishes actual library, series and type, dated Reading items and the optional continuation. Items use existing title/cover/author provenance. It does not start playback, write reading progress, create a saved publication identity, or grant source bytes. No group grid, pivot availability, calendar or whole-library periodical completion is claimed by this endpoint alone.

Official API generation belongs to the Release16 checkpoint batch.
