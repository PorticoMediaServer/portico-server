package compactcatalog

import (
	"context"
	"database/sql"
)

// ClearLibraryTx clears one bounded batch, child projections before their class.
// Source titles must already be removed by the library removal job.
func ClearLibraryTx(ctx context.Context, tx *sql.Tx, library string, limit int) (bool, error) {
	for _, query := range []string{
		`DELETE FROM catalog_book_group_visible_counts WHERE (class_id,generation,group_id) IN(SELECT class_id,generation,group_id FROM catalog_book_group_visible_counts WHERE class_id IN(SELECT id FROM compact_visibility_classes WHERE library_id=(SELECT id FROM catalog_libraries WHERE library_id=?1)) LIMIT ?2)`,
		`DELETE FROM catalog_book_group_visible_edges WHERE (class_id,generation,group_id,book_id) IN(SELECT class_id,generation,group_id,book_id FROM catalog_book_group_visible_edges WHERE class_id IN(SELECT id FROM compact_visibility_classes WHERE library_id=(SELECT id FROM catalog_libraries WHERE library_id=?1)) LIMIT ?2)`,
		`DELETE FROM catalog_book_visible_counts WHERE (class_id,generation,book_id) IN(SELECT class_id,generation,book_id FROM catalog_book_visible_counts WHERE class_id IN(SELECT id FROM compact_visibility_classes WHERE library_id=(SELECT id FROM catalog_libraries WHERE library_id=?1)) LIMIT ?2)`,
		`DELETE FROM catalog_book_visible_file_edges WHERE (class_id,generation,file_id) IN(SELECT class_id,generation,file_id FROM catalog_book_visible_file_edges WHERE class_id IN(SELECT id FROM compact_visibility_classes WHERE library_id=(SELECT id FROM catalog_libraries WHERE library_id=?1)) LIMIT ?2)`,
		`DELETE FROM catalog_collection_visible_counts WHERE (class_id,generation,collection_id) IN(SELECT class_id,generation,collection_id FROM catalog_collection_visible_counts WHERE class_id IN(SELECT id FROM compact_visibility_classes WHERE library_id=(SELECT id FROM catalog_libraries WHERE library_id=?1)) LIMIT ?2)`,
		`DELETE FROM catalog_collection_visible_edges WHERE (class_id,generation,collection_id,item_id) IN(SELECT class_id,generation,collection_id,item_id FROM catalog_collection_visible_edges WHERE class_id IN(SELECT id FROM compact_visibility_classes WHERE library_id=(SELECT id FROM catalog_libraries WHERE library_id=?1)) LIMIT ?2)`,
		`DELETE FROM catalog_collection_visible_item_jobs WHERE (class_id,generation,item_id) IN(SELECT class_id,generation,item_id FROM catalog_collection_visible_item_jobs WHERE class_id IN(SELECT id FROM compact_visibility_classes WHERE library_id=(SELECT id FROM catalog_libraries WHERE library_id=?1)) LIMIT ?2)`,
		`DELETE FROM catalog_movie_category_visible_edges WHERE (class_id,generation,library_id,kind,value,item_id) IN(SELECT class_id,generation,library_id,kind,value,item_id FROM catalog_movie_category_visible_edges WHERE class_id IN(SELECT id FROM compact_visibility_classes WHERE library_id=(SELECT id FROM catalog_libraries WHERE library_id=?1)) LIMIT ?2)`,
		`DELETE FROM catalog_movie_category_visible_item_jobs WHERE (class_id,generation,item_id) IN(SELECT class_id,generation,item_id FROM catalog_movie_category_visible_item_jobs WHERE class_id IN(SELECT id FROM compact_visibility_classes WHERE library_id=(SELECT id FROM catalog_libraries WHERE library_id=?1)) LIMIT ?2)`,
		`DELETE FROM catalog_movie_category_visible_summaries WHERE (class_id,generation,library_id,kind,value) IN(SELECT class_id,generation,library_id,kind,value FROM catalog_movie_category_visible_summaries WHERE class_id IN(SELECT id FROM compact_visibility_classes WHERE library_id=(SELECT id FROM catalog_libraries WHERE library_id=?1)) LIMIT ?2)`,
		`DELETE FROM catalog_personal_visibility_sort_buckets WHERE (profile_id,class_id,generation,library_id,kind,axis,value) IN(SELECT profile_id,class_id,generation,library_id,kind,axis,value FROM catalog_personal_visibility_sort_buckets WHERE class_id IN(SELECT id FROM compact_visibility_classes WHERE library_id=(SELECT id FROM catalog_libraries WHERE library_id=?1)) LIMIT ?2)`,
		`DELETE FROM catalog_personal_visibility_sort_class_jobs WHERE (class_id,generation,entity_id) IN(SELECT class_id,generation,entity_id FROM catalog_personal_visibility_sort_class_jobs WHERE class_id IN(SELECT id FROM compact_visibility_classes WHERE library_id=(SELECT id FROM catalog_libraries WHERE library_id=?1)) LIMIT ?2)`,
		`DELETE FROM catalog_personal_visibility_sort_edges WHERE (profile_id,class_id,generation,entity_id,axis) IN(SELECT profile_id,class_id,generation,entity_id,axis FROM catalog_personal_visibility_sort_edges WHERE class_id IN(SELECT id FROM compact_visibility_classes WHERE library_id=(SELECT id FROM catalog_libraries WHERE library_id=?1)) LIMIT ?2)`,
		`DELETE FROM catalog_visibility_sort_buckets WHERE (class_id,generation,library_id,kind,axis,value) IN(SELECT class_id,generation,library_id,kind,axis,value FROM catalog_visibility_sort_buckets WHERE class_id IN(SELECT id FROM compact_visibility_classes WHERE library_id=(SELECT id FROM catalog_libraries WHERE library_id=?1)) LIMIT ?2)`,
		`DELETE FROM catalog_visibility_sort_edges WHERE (class_id,generation,entity_id,axis) IN(SELECT class_id,generation,entity_id,axis FROM catalog_visibility_sort_edges WHERE class_id IN(SELECT id FROM compact_visibility_classes WHERE library_id=(SELECT id FROM catalog_libraries WHERE library_id=?1)) LIMIT ?2)`,
		`DELETE FROM catalog_visibility_sort_pair_jobs WHERE (class_id,generation,entity_id) IN(SELECT class_id,generation,entity_id FROM catalog_visibility_sort_pair_jobs WHERE class_id IN(SELECT id FROM compact_visibility_classes WHERE library_id=(SELECT id FROM catalog_libraries WHERE library_id=?1)) LIMIT ?2)`,
		`DELETE FROM compact_visibility_blocks WHERE (class_id,generation,library_id,kind,ordering,sort_value,entity_id) IN(SELECT class_id,generation,library_id,kind,ordering,sort_value,entity_id FROM compact_visibility_blocks WHERE class_id IN(SELECT id FROM compact_visibility_classes WHERE library_id=(SELECT id FROM catalog_libraries WHERE library_id=?1)) LIMIT ?2)`,
		`DELETE FROM compact_visibility_counts WHERE (class_id,generation,library_id,kind,head,decade) IN(SELECT class_id,generation,library_id,kind,head,decade FROM compact_visibility_counts WHERE class_id IN(SELECT id FROM compact_visibility_classes WHERE library_id=(SELECT id FROM catalog_libraries WHERE library_id=?1)) LIMIT ?2)`,
		`DELETE FROM compact_visibility_dirty WHERE (class_id,entity_id) IN(SELECT class_id,entity_id FROM compact_visibility_dirty WHERE class_id IN(SELECT id FROM compact_visibility_classes WHERE library_id=(SELECT id FROM catalog_libraries WHERE library_id=?1)) LIMIT ?2)`,
		`DELETE FROM compact_visibility_rows WHERE (class_id,generation,entity_id) IN(SELECT class_id,generation,entity_id FROM compact_visibility_rows WHERE class_id IN(SELECT id FROM compact_visibility_classes WHERE library_id=(SELECT id FROM catalog_libraries WHERE library_id=?1)) LIMIT ?2)`,
		`DELETE FROM compact_visibility_unavailable WHERE (class_id,generation,entity_id) IN(SELECT class_id,generation,entity_id FROM compact_visibility_unavailable WHERE class_id IN(SELECT id FROM compact_visibility_classes WHERE library_id=(SELECT id FROM catalog_libraries WHERE library_id=?1)) LIMIT ?2)`,
		`DELETE FROM compact_visibility_unavailable_counts WHERE (class_id,generation,library_id,kind) IN(SELECT class_id,generation,library_id,kind FROM compact_visibility_unavailable_counts WHERE class_id IN(SELECT id FROM compact_visibility_classes WHERE library_id=(SELECT id FROM catalog_libraries WHERE library_id=?1)) LIMIT ?2)`,
		`DELETE FROM catalog_asset_issues WHERE (library_id,asset_id) IN(SELECT library_id,asset_id FROM catalog_asset_issues WHERE library_id=(SELECT id FROM catalog_libraries WHERE library_id=?1) LIMIT ?2)`,
		`DELETE FROM catalog_book_context WHERE (book_id) IN(SELECT book_id FROM catalog_book_context WHERE library_id=(SELECT id FROM catalog_libraries WHERE library_id=?1) LIMIT ?2)`,
		`DELETE FROM catalog_book_groups WHERE (id) IN(SELECT id FROM catalog_book_groups WHERE library_id=(SELECT id FROM catalog_libraries WHERE library_id=?1) LIMIT ?2)`,
		`DELETE FROM catalog_browse_blocks WHERE (library_id,kind,ordering,sort_value,entity_id) IN(SELECT library_id,kind,ordering,sort_value,entity_id FROM catalog_browse_blocks WHERE library_id=(SELECT id FROM catalog_libraries WHERE library_id=?1) LIMIT ?2)`,
		`DELETE FROM catalog_browse_buckets WHERE (library_id,kind,container,axis,value,rating_key,label_key) IN(SELECT library_id,kind,container,axis,value,rating_key,label_key FROM catalog_browse_buckets WHERE library_id=(SELECT id FROM catalog_libraries WHERE library_id=?1) LIMIT ?2)`,
		`DELETE FROM catalog_browse_counted_rows WHERE (entity_id) IN(SELECT entity_id FROM catalog_browse_counted_rows WHERE library_id=(SELECT id FROM catalog_libraries WHERE library_id=?1) LIMIT ?2)`,
		`DELETE FROM catalog_browse_rows WHERE (entity_id) IN(SELECT entity_id FROM catalog_browse_rows WHERE library_id=(SELECT id FROM catalog_libraries WHERE library_id=?1) LIMIT ?2)`,
		`DELETE FROM catalog_content_eligible_counts WHERE (library_id,kind) IN(SELECT library_id,kind FROM catalog_content_eligible_counts WHERE library_id=(SELECT id FROM catalog_libraries WHERE library_id=?1) LIMIT ?2)`,
		`DELETE FROM catalog_content_eligible_rows WHERE (entity_id) IN(SELECT entity_id FROM catalog_content_eligible_rows WHERE library_id=(SELECT id FROM catalog_libraries WHERE library_id=?1) LIMIT ?2)`,
		`DELETE FROM catalog_facet_counts WHERE (library_id,field,value,rating_key,label_key) IN(SELECT library_id,field,value,rating_key,label_key FROM catalog_facet_counts WHERE library_id=(SELECT id FROM catalog_libraries WHERE library_id=?1) LIMIT ?2)`,
		`DELETE FROM catalog_facet_members WHERE (entity_id,field,value) IN(SELECT entity_id,field,value FROM catalog_facet_members WHERE library_id=(SELECT id FROM catalog_libraries WHERE library_id=?1) LIMIT ?2)`,
		`DELETE FROM catalog_home_buckets WHERE (library_id,kind,available,recent,backdrop,rating_key,label_key,dated,retired) IN(SELECT library_id,kind,available,recent,backdrop,rating_key,label_key,dated,retired FROM catalog_home_buckets WHERE library_id=(SELECT id FROM catalog_libraries WHERE library_id=?1) LIMIT ?2)`,
		`DELETE FROM catalog_home_counted_rows WHERE (entity_id) IN(SELECT entity_id FROM catalog_home_counted_rows WHERE library_id=(SELECT id FROM catalog_libraries WHERE library_id=?1) LIMIT ?2)`,
		`DELETE FROM catalog_missing_titles WHERE (entity_id) IN(SELECT entity_id FROM catalog_missing_titles WHERE library_id=(SELECT id FROM catalog_libraries WHERE library_id=?1) LIMIT ?2)`,
		`DELETE FROM catalog_movie_category_members WHERE (library_id,kind,value,item_id) IN(SELECT library_id,kind,value,item_id FROM catalog_movie_category_members WHERE library_id=(SELECT id FROM catalog_libraries WHERE library_id=?1) LIMIT ?2)`,
		`DELETE FROM catalog_movie_category_summaries WHERE (library_id,kind,value) IN(SELECT library_id,kind,value FROM catalog_movie_category_summaries WHERE library_id=(SELECT id FROM catalog_libraries WHERE library_id=?1) LIMIT ?2)`,
		`DELETE FROM catalog_movie_category_visible_edges WHERE (class_id,generation,library_id,kind,value,item_id) IN(SELECT class_id,generation,library_id,kind,value,item_id FROM catalog_movie_category_visible_edges WHERE library_id=(SELECT id FROM catalog_libraries WHERE library_id=?1) LIMIT ?2)`,
		`DELETE FROM catalog_movie_category_visible_pair_jobs WHERE (library_id,kind,value,item_id) IN(SELECT library_id,kind,value,item_id FROM catalog_movie_category_visible_pair_jobs WHERE library_id=(SELECT id FROM catalog_libraries WHERE library_id=?1) LIMIT ?2)`,
		`DELETE FROM catalog_movie_category_visible_summaries WHERE (class_id,generation,library_id,kind,value) IN(SELECT class_id,generation,library_id,kind,value FROM catalog_movie_category_visible_summaries WHERE library_id=(SELECT id FROM catalog_libraries WHERE library_id=?1) LIMIT ?2)`,
		`DELETE FROM catalog_personal_sort_buckets WHERE (profile_id,library_id,kind,axis,value) IN(SELECT profile_id,library_id,kind,axis,value FROM catalog_personal_sort_buckets WHERE library_id=(SELECT id FROM catalog_libraries WHERE library_id=?1) LIMIT ?2)`,
		`DELETE FROM catalog_personal_sort_rows WHERE (profile_id,entity_id) IN(SELECT profile_id,entity_id FROM catalog_personal_sort_rows WHERE library_id=(SELECT id FROM catalog_libraries WHERE library_id=?1) LIMIT ?2)`,
		`DELETE FROM catalog_personal_visibility_sort_buckets WHERE (profile_id,class_id,generation,library_id,kind,axis,value) IN(SELECT profile_id,class_id,generation,library_id,kind,axis,value FROM catalog_personal_visibility_sort_buckets WHERE library_id=(SELECT id FROM catalog_libraries WHERE library_id=?1) LIMIT ?2)`,
		`DELETE FROM catalog_personal_visibility_sort_edges WHERE (profile_id,class_id,generation,entity_id,axis) IN(SELECT profile_id,class_id,generation,entity_id,axis FROM catalog_personal_visibility_sort_edges WHERE library_id=(SELECT id FROM catalog_libraries WHERE library_id=?1) LIMIT ?2)`,
		`DELETE FROM catalog_rec_postings WHERE (facet,library_id,quality,entity_id) IN(SELECT facet,library_id,quality,entity_id FROM catalog_rec_postings WHERE library_id=(SELECT id FROM catalog_libraries WHERE library_id=?1) LIMIT ?2)`,
		`DELETE FROM catalog_related_facets WHERE (entity_id,relation,provider,facet_id) IN(SELECT entity_id,relation,provider,facet_id FROM catalog_related_facets WHERE library_id=(SELECT id FROM catalog_libraries WHERE library_id=?1) LIMIT ?2)`,
		`DELETE FROM catalog_visibility_sort_buckets WHERE (class_id,generation,library_id,kind,axis,value) IN(SELECT class_id,generation,library_id,kind,axis,value FROM catalog_visibility_sort_buckets WHERE library_id=(SELECT id FROM catalog_libraries WHERE library_id=?1) LIMIT ?2)`,
		`DELETE FROM catalog_visibility_sort_edges WHERE (class_id,generation,entity_id,axis) IN(SELECT class_id,generation,entity_id,axis FROM catalog_visibility_sort_edges WHERE library_id=(SELECT id FROM catalog_libraries WHERE library_id=?1) LIMIT ?2)`,
		`DELETE FROM compact_visibility_blocks WHERE (class_id,generation,library_id,kind,ordering,sort_value,entity_id) IN(SELECT class_id,generation,library_id,kind,ordering,sort_value,entity_id FROM compact_visibility_blocks WHERE library_id=(SELECT id FROM catalog_libraries WHERE library_id=?1) LIMIT ?2)`,
		`DELETE FROM compact_visibility_classes WHERE (id) IN(SELECT id FROM compact_visibility_classes WHERE library_id=(SELECT id FROM catalog_libraries WHERE library_id=?1) LIMIT ?2)`,
		`DELETE FROM compact_visibility_counts WHERE (class_id,generation,library_id,kind,head,decade) IN(SELECT class_id,generation,library_id,kind,head,decade FROM compact_visibility_counts WHERE library_id=(SELECT id FROM catalog_libraries WHERE library_id=?1) LIMIT ?2)`,
		`DELETE FROM compact_visibility_rows WHERE (class_id,generation,entity_id) IN(SELECT class_id,generation,entity_id FROM compact_visibility_rows WHERE library_id=(SELECT id FROM catalog_libraries WHERE library_id=?1) LIMIT ?2)`,
		`DELETE FROM compact_visibility_unavailable WHERE (class_id,generation,entity_id) IN(SELECT class_id,generation,entity_id FROM compact_visibility_unavailable WHERE library_id=(SELECT id FROM catalog_libraries WHERE library_id=?1) LIMIT ?2)`,
		`DELETE FROM compact_visibility_unavailable_counts WHERE (class_id,generation,library_id,kind) IN(SELECT class_id,generation,library_id,kind FROM compact_visibility_unavailable_counts WHERE library_id=(SELECT id FROM catalog_libraries WHERE library_id=?1) LIMIT ?2)`,
	} {
		result, err := tx.ExecContext(ctx, query, library, limit)
		if err != nil {
			return false, err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return false, err
		}
		if n > 0 {
			return false, nil
		}
	}
	return true, nil
}
