package postgres

// 0006: the export, notification and push queues announce new work with NOTIFY
// (A38, A66: their workers used to poll every 15 s, 60 s and 1 s), and the
// offline-server scan uses a per-server marker instead of re-reading every
// offline server each pass (A54).
const migration0006 = `
CREATE FUNCTION portico_notify_queue() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN PERFORM pg_notify(TG_ARGV[0] || '_' || current_schema(), ''); RETURN NULL; END $$;
CREATE TRIGGER portico_export_notify AFTER INSERT ON account_exports
 FOR EACH ROW EXECUTE FUNCTION portico_notify_queue('` + ExportChannel + `');
CREATE TRIGGER portico_notification_notify AFTER INSERT ON account_notification_events
 FOR EACH ROW EXECUTE FUNCTION portico_notify_queue('portico_notifications');
CREATE TRIGGER portico_push_notify AFTER INSERT ON account_push_outbox
 FOR EACH ROW EXECUTE FUNCTION portico_notify_queue('portico_push');
ALTER TABLE servers ADD COLUMN offline_notified_for timestamptz;
DROP INDEX server_offline_detection;
CREATE INDEX server_offline_detection ON servers(presence_expires_at) WHERE revoked_at IS NULL AND offline_notified_for IS DISTINCT FROM last_seen_at;
`
