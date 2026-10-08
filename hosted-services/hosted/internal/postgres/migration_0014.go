package postgres

// 0014 (A86): rotating the members-only label (0012) used to drop the old
// c-<label> name from DNS at once, while the server and remaining members
// still used it until they re-read their routes. The previous label now keeps
// resolving for a grace period (MemberLabelGrace, 24 h); the server picks up
// the new label on its next presence renewal (at most 15 minutes) and members
// on their next server-list read. A removed member loses authorization at
// once (policy) and the old name when the grace ends; it is never told the new
// label.
const migration0014 = `
ALTER TABLE certificate_namespaces
 ADD COLUMN previous_member_label text NOT NULL DEFAULT '' CONSTRAINT certificate_namespaces_previous_member_label_check CHECK(previous_member_label='' OR previous_member_label ~ '^[0-9a-f]{32}$'),
 ADD COLUMN previous_label_until timestamptz;
CREATE OR REPLACE FUNCTION rotate_member_label(server text) RETURNS void LANGUAGE plpgsql AS $fn$
DECLARE ns text;
BEGIN
  UPDATE certificate_namespaces SET previous_member_label=member_label,previous_label_until=clock_timestamp()+interval '24 hours',member_label=replace(gen_random_uuid()::text,'-','')
   WHERE server_identity=server AND member_label<>'' RETURNING namespace INTO ns;
  IF ns IS NOT NULL THEN
    PERFORM pg_advisory_xact_lock(8101820098972708609);
    UPDATE direct_addresses SET sequence=nextval('direct_address_sequence'),updated_at=clock_timestamp() WHERE namespace=ns;
  END IF;
END
$fn$;
`
