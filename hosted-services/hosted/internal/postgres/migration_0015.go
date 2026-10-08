package postgres

// 0015 (A87): the member-label rotation triggers (0012) took the fleet-wide
// direct-address feed lock inside the membership, restriction or deletion
// transaction that removed a member, so every server's address writes queued
// behind user transactions. The triggers now only queue the server; a worker
// rotates each queued server in its own short transaction
// (RotateQueuedMemberLabels). A removed member still loses access through
// policy at once; the rotation follows within seconds.
const migration0015 = `
CREATE TABLE member_label_rotations(server_id text PRIMARY KEY,requested_at timestamptz NOT NULL DEFAULT clock_timestamp());
CREATE OR REPLACE FUNCTION rotate_member_label_on_membership() RETURNS trigger LANGUAGE plpgsql AS $fn$
BEGIN
  INSERT INTO member_label_rotations(server_id) VALUES(OLD.server_id) ON CONFLICT DO NOTHING;
  RETURN NULL;
END
$fn$;
CREATE OR REPLACE FUNCTION rotate_member_label_on_restriction() RETURNS trigger LANGUAGE plpgsql AS $fn$
BEGIN
  INSERT INTO member_label_rotations(server_id) VALUES(NEW.server_id) ON CONFLICT DO NOTHING;
  RETURN NULL;
END
$fn$;
CREATE TRIGGER member_label_rotation_notify AFTER INSERT ON member_label_rotations
 FOR EACH ROW EXECUTE FUNCTION portico_notify_queue('` + LabelRotationChannel + `');
`
