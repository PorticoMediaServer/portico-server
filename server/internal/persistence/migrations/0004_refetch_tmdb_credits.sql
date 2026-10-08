-- The TMDB credit budget now keeps a title's director, writers, creators, composer and
-- producers ahead of the rest of its crew (a large production's stunt team used to fill the
-- budget first, leaving a film with no director). Documents stored before that were cut under
-- the old rule, so their freshness is forgotten: each is fetched again, unconditionally, the
-- next time its title refreshes.
DELETE FROM metadata_document_freshness WHERE provider='tmdb';
