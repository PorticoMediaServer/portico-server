-- 0008: a show library's facets count shows (compactcatalog/derive_facet_counts.go, version 3).
-- A show's facets follow its episodes, so an episode joining, leaving or moving queues its show.
CREATE TRIGGER catalog_facet_episode_insert AFTER INSERT ON catalog_episodes BEGIN
 INSERT INTO catalog_dirty(domain,entity_id,revision) VALUES(33,NEW.show_id,1) ON CONFLICT(domain,entity_id) DO UPDATE SET revision=revision+1;
END;
CREATE TRIGGER catalog_facet_episode_delete AFTER DELETE ON catalog_episodes BEGIN
 INSERT INTO catalog_dirty(domain,entity_id,revision) VALUES(33,OLD.show_id,1) ON CONFLICT(domain,entity_id) DO UPDATE SET revision=revision+1;
END;
CREATE TRIGGER catalog_facet_episode_move AFTER UPDATE OF show_id ON catalog_episodes WHEN OLD.show_id IS NOT NEW.show_id BEGIN
 INSERT INTO catalog_dirty(domain,entity_id,revision) VALUES(33,OLD.show_id,1) ON CONFLICT(domain,entity_id) DO UPDATE SET revision=revision+1;
 INSERT INTO catalog_dirty(domain,entity_id,revision) VALUES(33,NEW.show_id,1) ON CONFLICT(domain,entity_id) DO UPDATE SET revision=revision+1;
END;

-- A show's genres become terms on the show, as a movie's are, so a Shows grid filters by genre
-- through the term index. New publications write them (metadata/screen_publish.go); this carries
-- over the genres already published, which until now lived only in the show's metadata record.
INSERT OR IGNORE INTO catalog_terms(vocab,key,label)
 SELECT 1,lower(trim(json_extract(g.value,'$.name'))),trim(json_extract(g.value,'$.name'))
 FROM screen_metadata_fields sf JOIN catalog_entities e ON e.id=sf.target_id,json_each(sf.value) g
 WHERE sf.target_kind='show' AND sf.field='genres' AND json_valid(sf.value) AND trim(COALESCE(json_extract(g.value,'$.name'),''))<>'';
INSERT OR IGNORE INTO catalog_entity_terms(entity_id,term_id)
 SELECT sf.target_id,t.id
 FROM screen_metadata_fields sf JOIN catalog_entities e ON e.id=sf.target_id,json_each(sf.value) g
 JOIN catalog_terms t ON t.vocab=1 AND t.key=lower(trim(json_extract(g.value,'$.name')))
 WHERE sf.target_kind='show' AND sf.field='genres' AND json_valid(sf.value) AND trim(COALESCE(json_extract(g.value,'$.name'),''))<>'';
INSERT OR IGNORE INTO catalog_term_sources(entity_id,term_id,provider,source_id,label_override,source_name,ordinal)
 SELECT sf.target_id,t.id,sf.provider,
  COALESCE(NULLIF(CAST(json_extract(g.value,'$.id') AS TEXT),''),lower(trim(json_extract(g.value,'$.name')))),
  trim(json_extract(g.value,'$.name')),trim(json_extract(g.value,'$.name')),CAST(g.key AS INTEGER)
 FROM screen_metadata_fields sf JOIN catalog_entities e ON e.id=sf.target_id,json_each(sf.value) g
 JOIN catalog_terms t ON t.vocab=1 AND t.key=lower(trim(json_extract(g.value,'$.name')))
 WHERE sf.target_kind='show' AND sf.field='genres' AND json_valid(sf.value) AND trim(COALESCE(json_extract(g.value,'$.name'),''))<>'';
