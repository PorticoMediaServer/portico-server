#!/usr/bin/env python3
"""SQLite-only I02 integration regressions using actual Go DDL/query literals.

Run: python3 server/tests/i02_schema_test.py
This is not a substitute for persistence.Open or Go worker/provider tests.
"""
import re
import sqlite3
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1] / "internal"

def literal(path, prefix):
    values = re.findall(r"`([^`]*)`", (ROOT / path).read_text())
    matches = [v for v in values if v.strip().startswith(prefix)]
    if len(matches) != 1:
        raise AssertionError(f"Expected one SQL literal: {path}: {prefix}, got {len(matches)}")
    return matches[0]

DDL = [
    ("persistence/database.go", "PRAGMA foreign_keys"),
    ("persistence/revisions.go", "CREATE TABLE IF NOT EXISTS library_revisions"),
    ("persistence/admin.go", "CREATE TABLE IF NOT EXISTS admin_revision"),
    ("persistence/library_inventory.go", "CREATE TABLE IF NOT EXISTS library_sources"),
    ("persistence/console.go", "CREATE TABLE IF NOT EXISTS console_documents"),
    ("mounts/service.go", "CREATE TABLE IF NOT EXISTS managed_mounts"),
    ("mounts/native.go", "CREATE TABLE IF NOT EXISTS mount_backend_configs"),
    ("remotesources/service.go", "CREATE TABLE IF NOT EXISTS remote_sources"),
]

class SchemaIntegration(unittest.TestCase):
    def setUp(self):
        self.db = sqlite3.connect(":memory:")
        self.addCleanup(self.db.close)
        for path, start in DDL:
            self.db.executescript(literal(path, start))
        self.db.execute("INSERT INTO libraries VALUES('lib','Movies','movie','/media')")

    def scalar(self, sql, values=()):
        return self.db.execute(sql, values).fetchone()[0]

    def revision(self):
        return self.scalar("SELECT revision FROM console_settings_revision")

    def test_named_installers_repeat_without_resetting_policy(self):
        self.db.execute("UPDATE library_scan_policies SET tier='file_list_only',operations_json='[]',revision=9")
        for path, start in DDL[3:5] + DDL[7:]:
            self.db.executescript(literal(path, start))
        self.assertEqual(self.scalar("SELECT revision FROM library_scan_policies"), 9)
        self.assertEqual(self.scalar("SELECT tier FROM library_scan_policies"), "file_list_only")
        self.assertEqual(self.db.execute("PRAGMA foreign_key_check").fetchall(), [])

    def test_legacy_settings_writers_advance_console_fence(self):
        r = self.revision()
        self.db.execute("INSERT INTO configuration VALUES('name','Home')")
        self.assertEqual(self.revision(), r+1)
        self.db.execute("UPDATE configuration SET value='Home' WHERE key='name'")
        self.assertEqual(self.revision(), r+1)
        self.db.execute("UPDATE configuration SET value='New home' WHERE key='name'")
        self.assertEqual(self.revision(), r+2)
        self.db.execute("UPDATE playback_owner_policy SET transcoding_enabled=0 WHERE singleton=1")
        self.assertEqual(self.revision(), r+3)
        self.db.execute("INSERT INTO playback_account_caps VALUES('local','owner',2)")
        self.assertEqual(self.revision(), r+4)
        self.db.execute("DELETE FROM playback_account_caps")
        self.assertEqual(self.revision(), r+5)
        self.db.execute("UPDATE console_settings_revision SET revision=?", (r+1,))
        self.assertEqual(self.revision(), r+1, "one console transaction normalizes its own internal writes")

    def job(self, name, state='running', source='lib'):
        self.db.execute("INSERT INTO jobs(id,library_id,status,created_at) VALUES(?,'lib',?,'2026-09-06T00:00:00Z')", (name,state))
        self.db.execute("INSERT INTO inventory_runs(job_id,source_id,source_generation,root_incarnation,root_identity,policy_revision) SELECT ?,id,generation,incarnation,root_identity,1 FROM library_sources WHERE id=?", (name,source))

    def operation(self, name, state):
        self.db.execute("INSERT INTO console_operations(id,kind,resource,actor,trigger,state,phase,revision,attempt,created_ms,updated_ms,next_ms,domain_id,error_code,predecessor,settings_revision) VALUES(?,'library-scan','lib','owner','owner',?,'test',1,0,0,0,0,'','','',1)", (name,state))

    def test_paused_console_work_keeps_unique_claim(self):
        self.operation('one', 'paused')
        with self.assertRaises(sqlite3.IntegrityError):
            self.operation('two', 'queued')
        self.db.execute("UPDATE console_operations SET state='cancelled' WHERE id='one'")
        self.operation('two', 'queued')

    def test_console_upgrade_and_multi_source_membership(self):
        self.job('a')
        self.db.execute("INSERT INTO library_sources(id,library_id,name,configured_root,root) VALUES('second','lib','Second','/other','/other')")
        self.job('b', source='second')
        self.operation('old', 'running')
        self.db.execute("UPDATE console_operations SET domain_id='a' WHERE id='old'")
        self.db.executescript(literal('persistence/console.go','CREATE TABLE IF NOT EXISTS console_documents'))
        self.assertEqual(self.db.execute("SELECT operation_id,job_id FROM inventory_operation_jobs").fetchall(), [('a','a')])
        self.db.executemany("INSERT INTO inventory_operation_jobs VALUES('new',?)", [('a',),('b',)])
        self.assertEqual(self.scalar("SELECT count(*) FROM inventory_operation_jobs WHERE operation_id='new'"), 2)

    def test_remote_generation_fence_uses_current_domain_config(self):
        query = literal('catalog/remote_inventory.go','SELECT EXISTS(SELECT 1 FROM remote_scan_generations')
        self.db.execute("INSERT INTO remote_sources(id,kind,name,root) VALUES('dav','webdav','DAV','/virtual/dav')")
        self.db.execute("INSERT INTO remote_scan_generations VALUES('scan','dav','1')")
        self.assertTrue(self.scalar(query, ('scan',)))
        self.db.execute("UPDATE remote_sources SET generation=2 WHERE id='dav'")
        self.assertFalse(self.scalar(query, ('scan',)))
        self.db.execute("UPDATE remote_scan_generations SET generation='2'")
        self.assertTrue(self.scalar(query, ('scan',)))
        self.db.execute("UPDATE remote_sources SET removed=1 WHERE id='dav'")
        self.assertFalse(self.scalar(query, ('scan',)))
        self.db.execute("INSERT INTO managed_mounts(id,name,executable,digest,remote,mount_path) VALUES('rc','Rclone','/binary','digest','private:root','/virtual/rc')")
        self.db.execute("INSERT INTO remote_sources(id,kind,name,root,mount_id) VALUES('rc','rclone','RC','/virtual/rc','rc')")
        self.db.execute("INSERT INTO remote_scan_generations VALUES('scan2','rc','1')")
        self.assertTrue(self.scalar(query, ('scan2',)))
        self.db.execute("UPDATE mount_backend_configs SET generation=2 WHERE mount_id='rc'")
        self.assertFalse(self.scalar(query, ('scan2',)))

    def test_directory_proof_compares_both_full_sets(self):
        query = literal('remotesources/inventory_proof.go','SELECT EXISTS(SELECT name,snapshot')
        for name in ['old','new']:
            self.db.execute("INSERT INTO remote_listing_sessions(id,job_id,source_id,generation,directory,ready,created_at) VALUES(?,?,'dav','1','/dir',1,0)", (name,name))
        def changed():
            return bool(self.scalar(query, ('old','new','new','old')))
        self.assertFalse(changed())
        self.db.execute("INSERT INTO remote_listing_entries VALUES('old','a','evidence1')")
        self.assertTrue(changed(), 'removed object must invalidate proof')
        self.db.execute("INSERT INTO remote_listing_entries VALUES('new','a','evidence1')")
        self.assertFalse(changed())
        self.db.execute("INSERT INTO remote_listing_entries VALUES('new','b','evidence2')")
        self.assertTrue(changed(), 'added object must invalidate proof')
        self.db.execute("DELETE FROM remote_listing_entries WHERE name='b'")
        self.db.execute("UPDATE remote_listing_entries SET snapshot='evidence-new' WHERE listing_id='new'")
        self.assertTrue(changed(), 'changed evidence invalidates equal names')

    def test_optin_handoff_does_not_change_file_list_tier(self):
        self.db.execute("UPDATE library_scan_policies SET tier='file_list_only',operations_json='[]'")
        self.db.execute("INSERT INTO source_strm_analysis VALUES('lib',1,1)")
        self.db.execute("UPDATE library_scan_policies SET revision=revision+1 WHERE library_id='lib'")
        self.assertEqual(self.scalar("SELECT tier FROM library_scan_policies"), 'file_list_only')
        self.assertEqual(self.scalar("SELECT operations_json FROM library_scan_policies"), '[]')
        self.assertEqual(self.scalar("SELECT revision FROM inventory_policy_pending"), self.scalar("SELECT revision FROM library_scan_policies"))

    def test_partial_run_cannot_become_an_absence_authority(self):
        self.job('scan')
        self.db.execute("INSERT INTO inventory_directories(job_id,relative_path) VALUES('scan','.')")
        sql = literal('ingestion/inventory.go', "UPDATE inventory_runs SET phase='reconciling',authoritative=1")
        self.db.execute(sql, ('2026-09-06T00:00:00Z','scan','scan'))
        self.assertEqual(self.scalar("SELECT authoritative FROM inventory_runs WHERE job_id='scan'"), 0)
        self.db.execute("UPDATE inventory_directories SET state='done' WHERE job_id='scan'")
        self.db.execute(sql, ('2026-09-06T00:00:00Z','scan','scan'))
        self.assertEqual(self.scalar("SELECT authoritative FROM inventory_runs WHERE job_id='scan'"), 1)

if __name__ == '__main__':
    unittest.main(verbosity=2)
