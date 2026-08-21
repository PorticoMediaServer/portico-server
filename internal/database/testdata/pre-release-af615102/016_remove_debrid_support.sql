DELETE FROM settings WHERE key = 'debrid';
DELETE FROM jobs WHERE type = 'debrid_sync';

DROP TABLE IF EXISTS debrid_virtual_paths;
DROP TABLE IF EXISTS debrid_classification_rules;
DROP TABLE IF EXISTS debrid_virtual_views;
DROP TABLE IF EXISTS debrid_api_counters;
DROP TABLE IF EXISTS debrid_stream_sessions;
DROP TABLE IF EXISTS debrid_provider_files;
DROP TABLE IF EXISTS debrid_provider_items;
DROP TABLE IF EXISTS debrid_provider_accounts;
