-- A rebuildable relational access/sort index, never a preset store. Definitions,
-- grants, resource metadata, pins and arrangements remain canonical. The
-- migration runner installs and backfills this index in one transaction.
-- ANY preserves json_extract types: a historical numeric library/pivot must
-- not start matching a string scope through TEXT affinity.
CREATE TABLE saved_view_picker(
 owner_key TEXT NOT NULL,
 resource_id TEXT NOT NULL REFERENCES saved_resources(id) ON DELETE CASCADE,
 library_id ANY NOT NULL,
 pivot ANY,
 name_key TEXT NOT NULL,
 pinned INTEGER NOT NULL,
 pin_key TEXT NOT NULL,
 PRIMARY KEY(owner_key,resource_id)
) STRICT;
CREATE INDEX saved_view_picker_resource ON saved_view_picker(resource_id);
CREATE INDEX saved_view_picker_library ON saved_view_picker(owner_key,library_id,name_key,resource_id);
CREATE INDEX saved_view_picker_pivot ON saved_view_picker(owner_key,library_id,pivot,name_key,resource_id);
CREATE INDEX saved_view_picker_pinned_library ON saved_view_picker(owner_key,library_id,pin_key,resource_id) WHERE pinned=1;
CREATE INDEX saved_view_picker_pinned_pivot ON saved_view_picker(owner_key,library_id,pivot,pin_key,resource_id) WHERE pinned=1;

CREATE VIEW saved_view_picker_source AS
 SELECT r.owner_key AS owner_key,r.id AS resource_id,
 json_extract(r.definition,'$.libraryId') AS library_id,
 json_extract(r.definition,'$.pivot') AS pivot,
 lower(r.name) AS name_key,COALESCE(p.pinned,0) AS pinned,
 printf('%08d',COALESCE(o.position,99999999))||char(31)||lower(r.name) AS pin_key
 FROM saved_resources r
 LEFT JOIN saved_pins p ON p.owner_key=r.owner_key AND p.kind='view' AND p.resource_id=r.id
 LEFT JOIN saved_pin_order o ON o.owner_key=r.owner_key AND o.kind='view' AND o.resource_id=r.id
 WHERE r.kind='view' AND r.deleted=0
 AND CASE WHEN json_valid(r.definition) THEN json_extract(r.definition,'$.libraryId') END IS NOT NULL
 UNION ALL
 SELECT portico_personal_owner_key(sh.authority,sh.account_id,sh.profile_id) AS owner_key,r.id AS resource_id,
 json_extract(r.definition,'$.libraryId') AS library_id,
 json_extract(r.definition,'$.pivot') AS pivot,
 lower(r.name) AS name_key,COALESCE(p.pinned,0) AS pinned,
 printf('%08d',COALESCE(o.position,99999999))||char(31)||lower(r.name) AS pin_key
 FROM saved_resources r
 JOIN saved_resource_shares sh ON sh.resource_id=r.id
 LEFT JOIN saved_pins p ON p.owner_key=portico_personal_owner_key(sh.authority,sh.account_id,sh.profile_id) AND p.kind='view' AND p.resource_id=r.id
 LEFT JOIN saved_pin_order o ON o.owner_key=portico_personal_owner_key(sh.authority,sh.account_id,sh.profile_id) AND o.kind='view' AND o.resource_id=r.id
 WHERE r.kind='view' AND r.deleted=0
 AND CASE WHEN json_valid(r.definition) THEN json_extract(r.definition,'$.libraryId') END IS NOT NULL
 AND portico_personal_owner_key(sh.authority,sh.account_id,sh.profile_id)<>r.owner_key;

INSERT INTO saved_view_picker SELECT * FROM saved_view_picker_source;

CREATE TRIGGER saved_view_picker_resource_insert AFTER INSERT ON saved_resources BEGIN
 DELETE FROM saved_view_picker WHERE resource_id IN (NEW.id);
 INSERT INTO saved_view_picker SELECT * FROM saved_view_picker_source WHERE resource_id IN (NEW.id);
END;

CREATE TRIGGER saved_view_picker_resource_update AFTER UPDATE OF id,kind,owner_key,name,definition,deleted ON saved_resources BEGIN
 DELETE FROM saved_view_picker WHERE resource_id IN (OLD.id,NEW.id);
 INSERT INTO saved_view_picker SELECT * FROM saved_view_picker_source WHERE resource_id IN (OLD.id,NEW.id);
END;

CREATE TRIGGER saved_view_picker_resource_delete AFTER DELETE ON saved_resources BEGIN
 DELETE FROM saved_view_picker WHERE resource_id IN (OLD.id);
 INSERT INTO saved_view_picker SELECT * FROM saved_view_picker_source WHERE resource_id IN (OLD.id);
END;

CREATE TRIGGER saved_view_picker_share_insert AFTER INSERT ON saved_resource_shares BEGIN
 DELETE FROM saved_view_picker WHERE resource_id IN (NEW.resource_id);
 INSERT INTO saved_view_picker SELECT * FROM saved_view_picker_source WHERE resource_id IN (NEW.resource_id);
END;

CREATE TRIGGER saved_view_picker_share_update AFTER UPDATE ON saved_resource_shares BEGIN
 DELETE FROM saved_view_picker WHERE resource_id IN (OLD.resource_id,NEW.resource_id);
 INSERT INTO saved_view_picker SELECT * FROM saved_view_picker_source WHERE resource_id IN (OLD.resource_id,NEW.resource_id);
END;

CREATE TRIGGER saved_view_picker_share_delete AFTER DELETE ON saved_resource_shares BEGIN
 DELETE FROM saved_view_picker WHERE resource_id IN (OLD.resource_id);
 INSERT INTO saved_view_picker SELECT * FROM saved_view_picker_source WHERE resource_id IN (OLD.resource_id);
END;

CREATE TRIGGER saved_view_picker_pin_insert AFTER INSERT ON saved_pins BEGIN
 UPDATE saved_view_picker SET
  pinned=COALESCE((SELECT p.pinned FROM saved_pins p WHERE p.owner_key=NEW.owner_key AND p.kind='view' AND p.resource_id=NEW.resource_id),0),
  pin_key=(SELECT printf('%08d',COALESCE((SELECT o.position FROM saved_pin_order o WHERE o.owner_key=NEW.owner_key AND o.kind='view' AND o.resource_id=NEW.resource_id),99999999))||char(31)||lower(r.name) FROM saved_resources r WHERE r.id=NEW.resource_id)
 WHERE NEW.kind='view' AND owner_key=NEW.owner_key AND resource_id=NEW.resource_id;
END;

CREATE TRIGGER saved_view_picker_pin_update AFTER UPDATE ON saved_pins BEGIN
 UPDATE saved_view_picker SET
  pinned=COALESCE((SELECT p.pinned FROM saved_pins p WHERE p.owner_key=OLD.owner_key AND p.kind='view' AND p.resource_id=OLD.resource_id),0),
  pin_key=(SELECT printf('%08d',COALESCE((SELECT o.position FROM saved_pin_order o WHERE o.owner_key=OLD.owner_key AND o.kind='view' AND o.resource_id=OLD.resource_id),99999999))||char(31)||lower(r.name) FROM saved_resources r WHERE r.id=OLD.resource_id)
 WHERE OLD.kind='view' AND owner_key=OLD.owner_key AND resource_id=OLD.resource_id;
 UPDATE saved_view_picker SET
  pinned=COALESCE((SELECT p.pinned FROM saved_pins p WHERE p.owner_key=NEW.owner_key AND p.kind='view' AND p.resource_id=NEW.resource_id),0),
  pin_key=(SELECT printf('%08d',COALESCE((SELECT o.position FROM saved_pin_order o WHERE o.owner_key=NEW.owner_key AND o.kind='view' AND o.resource_id=NEW.resource_id),99999999))||char(31)||lower(r.name) FROM saved_resources r WHERE r.id=NEW.resource_id)
 WHERE NEW.kind='view' AND owner_key=NEW.owner_key AND resource_id=NEW.resource_id;
END;

CREATE TRIGGER saved_view_picker_pin_delete AFTER DELETE ON saved_pins BEGIN
 UPDATE saved_view_picker SET
  pinned=COALESCE((SELECT p.pinned FROM saved_pins p WHERE p.owner_key=OLD.owner_key AND p.kind='view' AND p.resource_id=OLD.resource_id),0),
  pin_key=(SELECT printf('%08d',COALESCE((SELECT o.position FROM saved_pin_order o WHERE o.owner_key=OLD.owner_key AND o.kind='view' AND o.resource_id=OLD.resource_id),99999999))||char(31)||lower(r.name) FROM saved_resources r WHERE r.id=OLD.resource_id)
 WHERE OLD.kind='view' AND owner_key=OLD.owner_key AND resource_id=OLD.resource_id;
END;

CREATE TRIGGER saved_view_picker_order_insert AFTER INSERT ON saved_pin_order BEGIN
 UPDATE saved_view_picker SET
  pinned=COALESCE((SELECT p.pinned FROM saved_pins p WHERE p.owner_key=NEW.owner_key AND p.kind='view' AND p.resource_id=NEW.resource_id),0),
  pin_key=(SELECT printf('%08d',COALESCE((SELECT o.position FROM saved_pin_order o WHERE o.owner_key=NEW.owner_key AND o.kind='view' AND o.resource_id=NEW.resource_id),99999999))||char(31)||lower(r.name) FROM saved_resources r WHERE r.id=NEW.resource_id)
 WHERE NEW.kind='view' AND owner_key=NEW.owner_key AND resource_id=NEW.resource_id;
END;

CREATE TRIGGER saved_view_picker_order_update AFTER UPDATE ON saved_pin_order BEGIN
 UPDATE saved_view_picker SET
  pinned=COALESCE((SELECT p.pinned FROM saved_pins p WHERE p.owner_key=OLD.owner_key AND p.kind='view' AND p.resource_id=OLD.resource_id),0),
  pin_key=(SELECT printf('%08d',COALESCE((SELECT o.position FROM saved_pin_order o WHERE o.owner_key=OLD.owner_key AND o.kind='view' AND o.resource_id=OLD.resource_id),99999999))||char(31)||lower(r.name) FROM saved_resources r WHERE r.id=OLD.resource_id)
 WHERE OLD.kind='view' AND owner_key=OLD.owner_key AND resource_id=OLD.resource_id;
 UPDATE saved_view_picker SET
  pinned=COALESCE((SELECT p.pinned FROM saved_pins p WHERE p.owner_key=NEW.owner_key AND p.kind='view' AND p.resource_id=NEW.resource_id),0),
  pin_key=(SELECT printf('%08d',COALESCE((SELECT o.position FROM saved_pin_order o WHERE o.owner_key=NEW.owner_key AND o.kind='view' AND o.resource_id=NEW.resource_id),99999999))||char(31)||lower(r.name) FROM saved_resources r WHERE r.id=NEW.resource_id)
 WHERE NEW.kind='view' AND owner_key=NEW.owner_key AND resource_id=NEW.resource_id;
END;

CREATE TRIGGER saved_view_picker_order_delete AFTER DELETE ON saved_pin_order BEGIN
 UPDATE saved_view_picker SET
  pinned=COALESCE((SELECT p.pinned FROM saved_pins p WHERE p.owner_key=OLD.owner_key AND p.kind='view' AND p.resource_id=OLD.resource_id),0),
  pin_key=(SELECT printf('%08d',COALESCE((SELECT o.position FROM saved_pin_order o WHERE o.owner_key=OLD.owner_key AND o.kind='view' AND o.resource_id=OLD.resource_id),99999999))||char(31)||lower(r.name) FROM saved_resources r WHERE r.id=OLD.resource_id)
 WHERE OLD.kind='view' AND owner_key=OLD.owner_key AND resource_id=OLD.resource_id;
END;
