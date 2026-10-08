-- Like/Dislike is gone as a concept: the heart (Favorite), star ratings and Not interested
-- cover it on every client. The column goes outright, with any offline evidence about it;
-- there is nothing worth carrying over. The recommender's trigger named the column, so it is
-- recreated without it.
DROP TRIGGER rec_profile_items_update;
ALTER TABLE personal_items DROP COLUMN reaction;
CREATE TRIGGER rec_profile_items_update AFTER UPDATE OF watchlisted,favorite,rating,watched,not_interested ON personal_items
 WHEN OLD.watchlisted IS NOT NEW.watchlisted OR OLD.favorite IS NOT NEW.favorite OR OLD.rating IS NOT NEW.rating OR OLD.watched IS NOT NEW.watched OR OLD.not_interested IS NOT NEW.not_interested BEGIN
 INSERT INTO rec_profile_jobs(profile_id,work_id,revision) VALUES(NEW.profile_id,COALESCE((SELECT show_id FROM catalog_episodes WHERE entity_id=NEW.item_id),(SELECT album_id FROM catalog_songs WHERE entity_id=NEW.item_id),(SELECT book_id FROM catalog_book_files WHERE entity_id=NEW.item_id),NEW.item_id),1) ON CONFLICT(profile_id,work_id) DO UPDATE SET revision=revision+1;
END;
DELETE FROM personal_conflicts WHERE field='reaction';
DELETE FROM personal_explicit_events WHERE field='reaction';
