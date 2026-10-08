-- 0009: two decisions of 2 Oct 2026.
--
-- One metadata provider per library, as Plex and Jellyfin have it: TMDB by default for films and
-- shows (AniList for anime), TheTVDB as the alternative an owner may choose. Providers are no
-- longer chained. A library that had two keeps the first.
DROP TRIGGER screen_policy_create;
CREATE TRIGGER screen_policy_create AFTER INSERT ON libraries WHEN NEW.kind IN('movie','tv','anime') BEGIN
 INSERT INTO screen_metadata_policies(library_id,providers) VALUES(NEW.id,CASE WHEN NEW.kind='anime' THEN '["anilist"]' ELSE '["tmdb"]' END);
END;
UPDATE screen_metadata_policies SET providers=json_array(json_extract(providers,'$[0]')),revision=revision+1
 WHERE json_valid(providers) AND json_array_length(providers)>1;
UPDATE tvdb_provider_policies SET enabled=0,revision=revision+1
 WHERE enabled=1 AND library_id IN(SELECT library_id FROM screen_metadata_policies WHERE json_valid(providers) AND COALESCE(json_extract(providers,'$[0]'),'')<>'tvdb');
UPDATE metadata_provider_policies SET enabled=0
 WHERE provider='tmdb' AND enabled=1 AND library_id IN(SELECT library_id FROM screen_metadata_policies WHERE json_valid(providers) AND COALESCE(json_extract(providers,'$[0]'),'')<>'tmdb');

-- The owner's viewing statistics are read from the play history, which is kept for as long as
-- the owner says (forever by default), not from playback sessions, which end after thirty days.
-- A play therefore says whether the server converted it.
ALTER TABLE play_history ADD COLUMN converted INTEGER NOT NULL DEFAULT 0 CHECK(converted IN(0,1));
