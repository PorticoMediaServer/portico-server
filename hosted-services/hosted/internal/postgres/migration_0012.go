package postgres

// 0012 (A20): a removed member must not keep resolving the server's
// members-only name c-<label>.<namespace>.direct.getportico.tv. Whenever a
// membership is revoked or deleted, or a profile restriction revokes access,
// the server's member label is replaced with a fresh random one and its
// direct-address row gets a new feed sequence, so the DNS responders drop the
// old name. The server and remaining members learn the new name from their
// next route read. The feed lock (directAddressLock) keeps sequences in commit
// order.
const migration0012 = `
CREATE FUNCTION rotate_member_label(server text) RETURNS void LANGUAGE plpgsql AS $fn$
DECLARE ns text;
BEGIN
  UPDATE certificate_namespaces SET member_label=replace(gen_random_uuid()::text,'-','')
   WHERE server_identity=server AND member_label<>'' RETURNING namespace INTO ns;
  IF ns IS NOT NULL THEN
    PERFORM pg_advisory_xact_lock(8101820098972708609);
    UPDATE direct_addresses SET sequence=nextval('direct_address_sequence'),updated_at=clock_timestamp() WHERE namespace=ns;
  END IF;
END
$fn$;
CREATE FUNCTION rotate_member_label_on_membership() RETURNS trigger LANGUAGE plpgsql AS $fn$
BEGIN
  PERFORM rotate_member_label(OLD.server_id);
  RETURN NULL;
END
$fn$;
CREATE FUNCTION rotate_member_label_on_restriction() RETURNS trigger LANGUAGE plpgsql AS $fn$
BEGIN
  PERFORM rotate_member_label(NEW.server_id);
  RETURN NULL;
END
$fn$;
CREATE TRIGGER memberships_revoked_rotate_label AFTER UPDATE OF revoked ON memberships
 FOR EACH ROW WHEN (NEW.revoked AND NOT OLD.revoked) EXECUTE FUNCTION rotate_member_label_on_membership();
CREATE TRIGGER memberships_deleted_rotate_label AFTER DELETE ON memberships
 FOR EACH ROW WHEN (NOT OLD.revoked) EXECUTE FUNCTION rotate_member_label_on_membership();
CREATE TRIGGER restrictions_inserted_rotate_label AFTER INSERT ON restrictions
 FOR EACH ROW WHEN (NEW.revoked) EXECUTE FUNCTION rotate_member_label_on_restriction();
CREATE TRIGGER restrictions_revoked_rotate_label AFTER UPDATE OF revoked ON restrictions
 FOR EACH ROW WHEN (NEW.revoked AND NOT OLD.revoked) EXECUTE FUNCTION rotate_member_label_on_restriction();
`
