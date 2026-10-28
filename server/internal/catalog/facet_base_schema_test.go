package catalog

// Isolated base schema and source-integration extensions from private draft.
// Neither constant is a production schema installer.
const facetBaseTestDDL = `-- PRIVATE draft for parent baseline integration; not a migration.
-- No query/results/job/handle tables. Only library/field/policy base facts.
-- No raw-source triggers: coherent producer adapter remains a required gate.
-- Logical charges are admission estimates, not dbstat/physical size claims.
CREATE TABLE catalog_facet_base_usage (
 id INTEGER PRIMARY KEY CHECK(id=1), used_bytes INTEGER NOT NULL DEFAULT 0 CHECK(used_bytes>=0)
) STRICT;
INSERT INTO catalog_facet_base_usage(id) VALUES(1);
CREATE TABLE catalog_facet_base_classes (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 library_id INTEGER NOT NULL, library TEXT NOT NULL, field TEXT NOT NULL,
 policy TEXT NOT NULL, source TEXT NOT NULL, active_base INTEGER,
 FOREIGN KEY(active_base,id) REFERENCES catalog_facet_bases(id,class_id) DEFERRABLE INITIALLY DEFERRED,
 UNIQUE(library_id,field,policy,source)
) STRICT;
CREATE INDEX catalog_facet_base_classes_active ON catalog_facet_base_classes(active_base,id);
CREATE TABLE catalog_facet_base_values (
 id INTEGER PRIMARY KEY AUTOINCREMENT, library_id INTEGER NOT NULL, field TEXT NOT NULL,
 value TEXT NOT NULL CHECK(value<>''), UNIQUE(library_id,field,value)
) STRICT;
CREATE TABLE catalog_facet_base_texts (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 value_id INTEGER NOT NULL REFERENCES catalog_facet_base_values(id),
 label TEXT NOT NULL, folded TEXT NOT NULL, UNIQUE(value_id,label), UNIQUE(id,value_id)
) STRICT;
CREATE TABLE catalog_facet_bases (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 class_id INTEGER NOT NULL REFERENCES catalog_facet_base_classes(id),
 witness TEXT NOT NULL, authority TEXT NOT NULL, complete INTEGER NOT NULL DEFAULT 0 CHECK(complete IN(0,1)),
 total INTEGER NOT NULL DEFAULT 0 CHECK(total>=0),
 charged_bytes INTEGER NOT NULL DEFAULT 0 CHECK(charged_bytes>=0), UNIQUE(id,class_id)
) STRICT;
CREATE INDEX catalog_facet_bases_class ON catalog_facet_bases(class_id,id);
CREATE INDEX catalog_facet_bases_demand ON catalog_facet_bases(class_id,witness,authority,id);
CREATE TABLE catalog_facet_base_rows (
 base_id INTEGER NOT NULL REFERENCES catalog_facet_bases(id),
 value_id INTEGER NOT NULL REFERENCES catalog_facet_base_values(id),
 text_id INTEGER NOT NULL,
 negative_count INTEGER NOT NULL CHECK(negative_count<0),
 label_order TEXT NOT NULL COLLATE BINARY, value_order TEXT NOT NULL COLLATE BINARY,
 charged_bytes INTEGER NOT NULL CHECK(charged_bytes>0),
 PRIMARY KEY(base_id,value_id),
 FOREIGN KEY(text_id,value_id) REFERENCES catalog_facet_base_texts(id,value_id)
) STRICT, WITHOUT ROWID;
CREATE INDEX catalog_facet_base_order ON catalog_facet_base_rows(base_id,negative_count,label_order,value_order,value_id);
CREATE INDEX catalog_facet_base_rows_value ON catalog_facet_base_rows(value_id);
CREATE INDEX catalog_facet_base_rows_text ON catalog_facet_base_rows(text_id,value_id);
CREATE TABLE catalog_facet_base_work (
 base_id INTEGER PRIMARY KEY REFERENCES catalog_facet_bases(id), cursor TEXT NOT NULL
) STRICT;
CREATE TRIGGER catalog_facet_base_row_insert AFTER INSERT ON catalog_facet_base_rows BEGIN
 UPDATE catalog_facet_bases SET total=total+1,charged_bytes=charged_bytes+NEW.charged_bytes WHERE id=NEW.base_id;
 UPDATE catalog_facet_base_usage SET used_bytes=used_bytes+NEW.charged_bytes WHERE id=1;
END;
CREATE TRIGGER catalog_facet_base_row_delete AFTER DELETE ON catalog_facet_base_rows BEGIN
 UPDATE catalog_facet_bases SET total=total-1,charged_bytes=charged_bytes-OLD.charged_bytes WHERE id=OLD.base_id;
 UPDATE catalog_facet_base_usage SET used_bytes=used_bytes-OLD.charged_bytes WHERE id=1;
END;
CREATE TRIGGER catalog_facet_base_value_insert AFTER INSERT ON catalog_facet_base_values BEGIN
 UPDATE catalog_facet_base_usage SET used_bytes=used_bytes+64+2*length(CAST(NEW.value AS BLOB)) WHERE id=1;
END;
CREATE TRIGGER catalog_facet_base_value_delete AFTER DELETE ON catalog_facet_base_values BEGIN
 UPDATE catalog_facet_base_usage SET used_bytes=used_bytes-64-2*length(CAST(OLD.value AS BLOB)) WHERE id=1;
END;
CREATE TRIGGER catalog_facet_base_text_insert AFTER INSERT ON catalog_facet_base_texts BEGIN
 UPDATE catalog_facet_base_usage SET used_bytes=used_bytes+64+2*length(CAST(NEW.label AS BLOB))+length(CAST(NEW.folded AS BLOB)) WHERE id=1;
END;
CREATE TRIGGER catalog_facet_base_text_delete AFTER DELETE ON catalog_facet_base_texts BEGIN
 UPDATE catalog_facet_base_usage SET used_bytes=used_bytes-64-2*length(CAST(OLD.label AS BLOB))-length(CAST(OLD.folded AS BLOB)) WHERE id=1;
END;
-- COMPLETE bases/texts are immutable. Only bounded retirement may remove rows;
-- readers revalidate basis existence and live authority after composition.
-- Deliberately no ON DELETE CASCADE through this potentially large projection.

CREATE TRIGGER catalog_facet_base_row_immutable_insert BEFORE INSERT ON catalog_facet_base_rows
WHEN (SELECT complete FROM catalog_facet_bases WHERE id=NEW.base_id)=1 BEGIN SELECT RAISE(ABORT,'immutable complete facet base'); END;
CREATE TRIGGER catalog_facet_base_row_immutable_update BEFORE UPDATE ON catalog_facet_base_rows
BEGIN SELECT RAISE(ABORT,'replace staged facet rows instead of updating'); END;
CREATE TRIGGER catalog_facet_base_row_immutable_delete BEFORE DELETE ON catalog_facet_base_rows
WHEN (SELECT complete FROM catalog_facet_bases WHERE id=OLD.base_id)=1 BEGIN SELECT RAISE(ABORT,'retire facet base before removing rows'); END;
CREATE TRIGGER catalog_facet_base_text_immutable BEFORE UPDATE ON catalog_facet_base_texts
WHEN NEW.value_id<>OLD.value_id OR NEW.label<>OLD.label OR NEW.folded<>OLD.folded
BEGIN SELECT RAISE(ABORT,'immutable shared facet text'); END;
CREATE TRIGGER catalog_facet_base_value_immutable BEFORE UPDATE ON catalog_facet_base_values
WHEN NEW.library_id<>OLD.library_id OR NEW.field<>OLD.field OR NEW.value<>OLD.value
BEGIN SELECT RAISE(ABORT,'immutable shared facet identity'); END;
-- active_base is a deferred composite FK back to this class's own base; NULL
-- means unavailable. Activation and complete flip are in one transaction.
-- RetireStep refuses active bases, marks retained input incomplete before bounded
-- removal, and live/basis readback makes racing or evicted preparations stale.
CREATE TRIGGER catalog_facet_base_class_identity BEFORE UPDATE OF library_id,library,field,policy,source ON catalog_facet_base_classes
WHEN NEW.library_id<>OLD.library_id OR NEW.library<>OLD.library OR NEW.field<>OLD.field OR NEW.policy<>OLD.policy OR NEW.source<>OLD.source
BEGIN SELECT RAISE(ABORT,'immutable facet class identity'); END;
CREATE TRIGGER catalog_facet_base_active_complete BEFORE UPDATE OF active_base ON catalog_facet_base_classes
WHEN NEW.active_base IS NOT NULL AND NOT EXISTS(SELECT 1 FROM catalog_facet_bases WHERE id=NEW.active_base AND class_id=NEW.id AND complete=1)
BEGIN SELECT RAISE(ABORT,'facet activation requires own COMPLETE base'); END;
CREATE TRIGGER catalog_facet_base_image_identity BEFORE UPDATE OF class_id,witness,authority ON catalog_facet_bases
WHEN NEW.class_id<>OLD.class_id OR NEW.witness<>OLD.witness OR NEW.authority<>OLD.authority
BEGIN SELECT RAISE(ABORT,'immutable facet base image'); END;
CREATE TRIGGER catalog_facet_base_class_charge_insert AFTER INSERT ON catalog_facet_base_classes BEGIN
 UPDATE catalog_facet_base_usage SET used_bytes=used_bytes+192+length(CAST(NEW.library AS BLOB))+length(CAST(NEW.field AS BLOB))+length(CAST(NEW.policy AS BLOB))+length(CAST(NEW.source AS BLOB)) WHERE id=1;
END;
CREATE TRIGGER catalog_facet_base_class_charge_delete AFTER DELETE ON catalog_facet_base_classes BEGIN
 UPDATE catalog_facet_base_usage SET used_bytes=used_bytes-192-length(CAST(OLD.library AS BLOB))-length(CAST(OLD.field AS BLOB))-length(CAST(OLD.policy AS BLOB))-length(CAST(OLD.source AS BLOB)) WHERE id=1;
END;
CREATE TRIGGER catalog_facet_base_image_charge_insert AFTER INSERT ON catalog_facet_bases BEGIN
 UPDATE catalog_facet_base_usage SET used_bytes=used_bytes+192+length(CAST(NEW.witness AS BLOB))+length(CAST(NEW.authority AS BLOB)) WHERE id=1;
END;
CREATE TRIGGER catalog_facet_base_image_charge_delete AFTER DELETE ON catalog_facet_bases BEGIN
 UPDATE catalog_facet_base_usage SET used_bytes=used_bytes-192-length(CAST(OLD.witness AS BLOB))-length(CAST(OLD.authority AS BLOB)) WHERE id=1;
END;
CREATE TRIGGER catalog_facet_base_work_charge_insert AFTER INSERT ON catalog_facet_base_work BEGIN
 UPDATE catalog_facet_base_usage SET used_bytes=used_bytes+128+length(CAST(NEW.cursor AS BLOB)) WHERE id=1;
END;
CREATE TRIGGER catalog_facet_base_work_charge_delete AFTER DELETE ON catalog_facet_base_work BEGIN
 UPDATE catalog_facet_base_usage SET used_bytes=used_bytes-128-length(CAST(OLD.cursor AS BLOB)) WHERE id=1;
END;

CREATE TRIGGER catalog_facet_base_work_charge_update AFTER UPDATE OF cursor ON catalog_facet_base_work BEGIN
 UPDATE catalog_facet_base_usage SET used_bytes=used_bytes+length(CAST(NEW.cursor AS BLOB))-length(CAST(OLD.cursor AS BLOB)) WHERE id=1;
END;
`

const facetBaseSourceIntegrationDDL = `-- Linked-decade BASE inputs must have an accepting stamp even if a future
-- browse producer writes memberships without touching its existing work key.
CREATE TRIGGER catalog_facet_base_membership_source_insert AFTER INSERT ON catalog_browse_memberships BEGIN
 UPDATE catalog_facet_source_state SET source_epoch=source_epoch+1 WHERE singleton=1;
END;
CREATE TRIGGER catalog_facet_base_membership_source_update AFTER UPDATE ON catalog_browse_memberships BEGIN
 UPDATE catalog_facet_source_state SET source_epoch=source_epoch+1 WHERE singleton=1;
END;
CREATE TRIGGER catalog_facet_base_membership_source_delete AFTER DELETE ON catalog_browse_memberships BEGIN
 UPDATE catalog_facet_source_state SET source_epoch=source_epoch+1 WHERE singleton=1;
END;
`
