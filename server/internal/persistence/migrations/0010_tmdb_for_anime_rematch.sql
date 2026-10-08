-- 0010: TMDB is the default provider for an anime library too (2 Oct 2026): it names every
-- episode, which AniList does not. AniList stays a choice an owner can make. Before any release
-- nobody chose AniList on purpose (it was only the default), so libraries on it move to TMDB.
DROP TRIGGER screen_policy_create;
CREATE TRIGGER screen_policy_create AFTER INSERT ON libraries WHEN NEW.kind IN('movie','tv','anime') BEGIN
 INSERT INTO screen_metadata_policies(library_id,providers) VALUES(NEW.id,'["tmdb"]');
END;
UPDATE screen_metadata_work SET status='pending',generation=generation+1,revision=revision+1,lease='',lease_until='',next_attempt='',error=''
 WHERE library_id IN(SELECT library_id FROM screen_metadata_policies WHERE providers='["anilist"]');
UPDATE metadata_provider_policies SET enabled=1
 WHERE provider='tmdb' AND library_id IN(SELECT library_id FROM screen_metadata_policies WHERE providers='["anilist"]' AND enabled=1);
UPDATE screen_metadata_policies SET providers='["tmdb"]',revision=revision+1 WHERE providers='["anilist"]';

-- A library's titles are matched with the library's one provider. A title matched with another
-- one (the second of an old chain, or the provider the library just left) would fail every
-- refresh, so it goes back to unmatched and is searched for again; what it shows stays until
-- the new match is published.
-- An identity lock on such a title keeps a match the library can no longer refresh (every
-- TheTVDB match used to record one), so it is released with the match.
UPDATE metadata_relationship_decisions SET locked=0 WHERE relationship='identity' AND locked=1 AND (kind,entity_id) IN(
 SELECT w.target_kind,w.target_id FROM screen_metadata_work w WHERE w.provider<>'' AND w.provider<>COALESCE((SELECT json_extract(p.providers,'$[0]') FROM screen_metadata_policies p WHERE p.library_id=w.library_id AND json_valid(p.providers)),w.provider));
UPDATE screen_metadata_work SET provider='',provider_type='',provider_id='',selection_mode='none',episode_order='official',requested_provider='',child_cursor='',
 selection_revision=selection_revision+1,status='pending',generation=generation+1,revision=revision+1,lease='',lease_until='',next_attempt='',error=''
 WHERE provider<>'' AND provider<>COALESCE((SELECT json_extract(p.providers,'$[0]') FROM screen_metadata_policies p WHERE p.library_id=screen_metadata_work.library_id AND json_valid(p.providers)),provider);
