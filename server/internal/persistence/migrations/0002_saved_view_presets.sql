-- Keep the existing saved view definition as the source of truth. These
-- expression indexes narrow an owner's picker before paging/hydrating it;
-- malformed historical definitions remain manageable in the generic list.
CREATE INDEX saved_view_library_page ON saved_resources(
 owner_key,
 CASE WHEN json_valid(definition) THEN json_extract(definition,'$.libraryId') END,
 lower(name),id
) WHERE kind='view' AND deleted=0;
CREATE INDEX saved_view_pivot_page ON saved_resources(
 owner_key,
 CASE WHEN json_valid(definition) THEN json_extract(definition,'$.libraryId') END,
 CASE WHEN json_valid(definition) THEN json_extract(definition,'$.pivot') END,
 lower(name),id
) WHERE kind='view' AND deleted=0;
