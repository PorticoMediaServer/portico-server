//! Device-owned offline files and normalized bookkeeping. No URLs, sessions or web paths.
use crate::boundary::{Error, Result};
use crate::download_receipt::{
    DownloadViewer, ReceiptBinding, ReceiptEnvelope, SAFE_INTEGER, digest, identifier,
    verify_receipt,
};
use base64::{Engine, engine::general_purpose::URL_SAFE_NO_PAD};
use ring::digest::{Context, SHA256};
use rusqlite::{Connection, OpenFlags, OptionalExtension, Row, TransactionBehavior, params};
use serde::{Deserialize, Serialize};
use std::{
    fs::{File, OpenOptions},
    io::{Read, Seek, SeekFrom, Write},
    path::{Path, PathBuf},
    time::{SystemTime, UNIX_EPOCH},
};

const CHUNK_BYTES: usize = 256 * 1024;
const SQL: &str = "
PRAGMA foreign_keys=ON;
PRAGMA trusted_schema=OFF;
CREATE TABLE IF NOT EXISTS download_scopes (
 id TEXT PRIMARY KEY, authority TEXT NOT NULL CHECK(authority IN ('local','hosted')),
 account_id TEXT NOT NULL, profile_id TEXT NOT NULL, server_id TEXT NOT NULL,
 UNIQUE(authority,account_id,profile_id,server_id)
) STRICT;
CREATE TABLE IF NOT EXISTS download_rules (
 id INTEGER PRIMARY KEY CHECK(id=1),revision INTEGER NOT NULL CHECK(revision BETWEEN 0 AND 9007199254740991),
 remove_watched INTEGER NOT NULL CHECK(remove_watched IN (0,1)),
 next_episode INTEGER NOT NULL CHECK(next_episode IN (0,1)),
 unmetered_only INTEGER NOT NULL CHECK(unmetered_only IN (0,1)),
 limit_bytes INTEGER NOT NULL CHECK(limit_bytes BETWEEN 0 AND 9007199254740991),
 clock_floor_ms INTEGER NOT NULL CHECK(clock_floor_ms>=0)
) STRICT;
CREATE TABLE IF NOT EXISTS download_entries (
 id TEXT PRIMARY KEY,scope_id TEXT NOT NULL REFERENCES download_scopes(id),
 preparation_id TEXT NOT NULL,item_id TEXT NOT NULL,quality TEXT NOT NULL,
 sha256 TEXT NOT NULL CHECK(length(sha256)=64),expected_bytes INTEGER NOT NULL CHECK(expected_bytes>0),
 bytes_done INTEGER NOT NULL CHECK(bytes_done BETWEEN 0 AND expected_bytes),
 state TEXT NOT NULL CHECK(state IN ('queued','downloading','paused','failed','ready')),
 held INTEGER NOT NULL CHECK(held IN (0,1)),revision INTEGER NOT NULL CHECK(revision BETWEEN 0 AND 9007199254740991),
 added_ms INTEGER NOT NULL CHECK(added_ms>=0),UNIQUE(scope_id,preparation_id)
) STRICT;
CREATE INDEX IF NOT EXISTS download_entries_page ON download_entries(scope_id,added_ms DESC,id);
CREATE TABLE IF NOT EXISTS download_titles (
 entry_id TEXT PRIMARY KEY REFERENCES download_entries(id) ON DELETE CASCADE,
 kind TEXT NOT NULL,title TEXT NOT NULL,artist TEXT NOT NULL,parent_id TEXT NOT NULL,parent_title TEXT NOT NULL,
 group_id TEXT NOT NULL,group_title TEXT NOT NULL,group_number INTEGER,number INTEGER,
 year INTEGER NOT NULL,duration_ms INTEGER NOT NULL,on_server INTEGER NOT NULL CHECK(on_server IN (0,1))
) STRICT;
CREATE TABLE IF NOT EXISTS download_keys (
 scope_id TEXT NOT NULL REFERENCES download_scopes(id),key_id TEXT NOT NULL,public_key TEXT NOT NULL,
 PRIMARY KEY(scope_id,key_id)
) STRICT;
CREATE TABLE IF NOT EXISTS download_receipts (
 entry_id TEXT PRIMARY KEY REFERENCES download_entries(id) ON DELETE CASCADE,
 receipt_id TEXT NOT NULL,key_id TEXT NOT NULL,payload TEXT NOT NULL,signature TEXT NOT NULL,
 revision INTEGER NOT NULL CHECK(revision BETWEEN 1 AND 9007199254740991),expires_ms INTEGER NOT NULL
) STRICT;
CREATE TABLE IF NOT EXISTS download_revocations (
 scope_id TEXT NOT NULL REFERENCES download_scopes(id),receipt_id TEXT NOT NULL,
 PRIMARY KEY(scope_id,receipt_id)
) STRICT;
CREATE TABLE IF NOT EXISTS download_plays (
 scope_id TEXT NOT NULL REFERENCES download_scopes(id),play_id TEXT NOT NULL,item_id TEXT NOT NULL,
 position_ms INTEGER NOT NULL CHECK(position_ms>=0),ended INTEGER NOT NULL CHECK(ended IN (0,1)),
 observed_ms INTEGER NOT NULL CHECK(observed_ms>=0),PRIMARY KEY(scope_id,play_id)
) STRICT;
CREATE INDEX IF NOT EXISTS download_plays_pending ON download_plays(scope_id,observed_ms,play_id);
";

#[derive(Clone, Debug, Deserialize, Serialize, PartialEq)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct DownloadRules {
    pub remove_watched: bool,
    pub next_episode: bool,
    pub unmetered_only: bool,
    pub limit_bytes: u64,
}
#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct RulesSnapshot {
    pub revision: u64,
    pub rules: DownloadRules,
}
#[derive(Clone, Debug, Deserialize, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct DownloadTitle {
    pub kind: String,
    pub title: String,
    pub artist: String,
    pub parent_id: String,
    pub parent_title: String,
    pub group_id: String,
    pub group_title: String,
    pub group_number: Option<u32>,
    pub number: Option<u32>,
    pub year: u32,
    pub duration_ms: u64,
    pub on_server: bool,
}
#[derive(Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct Enrollment {
    pub viewer: DownloadViewer,
    pub preparation_id: String,
    pub item_id: String,
    pub quality: String,
    pub sha256: String,
    pub bytes: u64,
    pub title: DownloadTitle,
}
#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct LocalDownload {
    pub id: String,
    pub preparation_id: String,
    pub item_id: String,
    pub quality: String,
    pub sha256: String,
    pub bytes: u64,
    pub bytes_done: u64,
    pub state: String,
    pub held: bool,
    pub revision: u64,
    pub added_ms: i64,
    pub receipt_expires_ms: Option<i64>,
    pub title: DownloadTitle,
}
#[derive(Deserialize, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct DownloadCursor {
    pub added_ms: i64,
    pub id: String,
}
#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
pub struct LocalDownloadPage {
    pub items: Vec<LocalDownload>,
    pub next_cursor: Option<DownloadCursor>,
}
#[derive(Debug, Deserialize, Serialize, PartialEq)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct PendingPlay {
    pub play_id: String,
    pub item_id: String,
    pub position_ms: u64,
    pub ended: bool,
    pub observed_ms: i64,
}

fn unavailable<T>(_: T) -> Error {
    Error::DownloadsUnavailable
}
fn now_ms() -> i64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|d| d.as_millis().min(i64::MAX as u128) as i64)
        .unwrap_or(0)
}
fn hash(bytes: &[u8]) -> String {
    hex(ring::digest::digest(&SHA256, bytes).as_ref())
}
fn hex(bytes: &[u8]) -> String {
    bytes.iter().map(|b| format!("{b:02x}")).collect()
}
fn scope(viewer: &DownloadViewer) -> Result<String> {
    viewer.validate()?;
    Ok(hash(
        format!(
            "{}\0{}\0{}\0{}",
            viewer.authority, viewer.account_id, viewer.profile_id, viewer.server_id
        )
        .as_bytes(),
    ))
}
fn title_valid(title: &DownloadTitle) -> bool {
    [
        &title.kind,
        &title.title,
        &title.artist,
        &title.parent_title,
        &title.group_title,
    ]
    .iter()
    .all(|v| v.len() <= 1024 && !v.chars().any(char::is_control))
        && [&title.parent_id, &title.group_id]
            .iter()
            .all(|v| v.is_empty() || identifier(v))
        && title.duration_ms <= SAFE_INTEGER
        && title.year <= 9999
}
fn private_dir(path: &Path) -> Result<()> {
    match std::fs::symlink_metadata(path) {
        Ok(meta) if meta.is_dir() && !meta.file_type().is_symlink() => Ok(()),
        Ok(_) => Err(Error::DownloadsUnavailable),
        Err(e) if e.kind() == std::io::ErrorKind::NotFound => {
            let mut builder = std::fs::DirBuilder::new();
            builder.recursive(true);
            #[cfg(unix)]
            {
                use std::os::unix::fs::DirBuilderExt;
                builder.mode(0o700);
            }
            builder.create(path).map_err(unavailable)
        }
        Err(_) => Err(Error::DownloadsUnavailable),
    }
}
fn open_file(path: &Path, write: bool, create: bool) -> Result<File> {
    let mut options = OpenOptions::new();
    options.read(true).write(write).create_new(create);
    #[cfg(unix)]
    {
        use std::os::unix::fs::OpenOptionsExt;
        options.mode(0o600).custom_flags(libc::O_NOFOLLOW);
    }
    #[cfg(windows)]
    {
        use std::os::windows::fs::OpenOptionsExt;
        options.custom_flags(0x0020_0000);
    }
    let file = options.open(path).map_err(unavailable)?;
    let metadata = file.metadata().map_err(unavailable)?;
    if !metadata.is_file() {
        return Err(Error::DownloadsUnavailable);
    }
    #[cfg(windows)]
    {
        use std::os::windows::fs::MetadataExt;
        if metadata.file_attributes() & 0x400 != 0 {
            return Err(Error::DownloadsUnavailable);
        }
    }
    Ok(file)
}

const READ_ENTRY: &str = "SELECT e.id,e.preparation_id,e.item_id,e.quality,e.sha256,e.expected_bytes,e.bytes_done,e.state,e.held,e.revision,e.added_ms,r.expires_ms,t.kind,t.title,t.artist,t.parent_id,t.parent_title,t.group_id,t.group_title,t.group_number,t.number,t.year,t.duration_ms,t.on_server FROM download_entries e JOIN download_titles t ON t.entry_id=e.id LEFT JOIN download_receipts r ON r.entry_id=e.id ";
fn count(row: &Row<'_>, index: usize) -> rusqlite::Result<u64> {
    let value = row.get::<_, i64>(index)?;
    if value < 0 || value as u64 > SAFE_INTEGER {
        return Err(rusqlite::Error::IntegralValueOutOfRange(index, value));
    }
    Ok(value as u64)
}
fn row_entry(row: &Row<'_>) -> rusqlite::Result<LocalDownload> {
    Ok(LocalDownload {
        id: row.get(0)?,
        preparation_id: row.get(1)?,
        item_id: row.get(2)?,
        quality: row.get(3)?,
        sha256: row.get(4)?,
        bytes: count(row, 5)?,
        bytes_done: count(row, 6)?,
        state: row.get(7)?,
        held: row.get(8)?,
        revision: count(row, 9)?,
        added_ms: row.get(10)?,
        receipt_expires_ms: row.get(11)?,
        title: DownloadTitle {
            kind: row.get(12)?,
            title: row.get(13)?,
            artist: row.get(14)?,
            parent_id: row.get(15)?,
            parent_title: row.get(16)?,
            group_id: row.get(17)?,
            group_title: row.get(18)?,
            group_number: row.get(19)?,
            number: row.get(20)?,
            year: row.get(21)?,
            duration_ms: count(row, 22)?,
            on_server: row.get(23)?,
        },
    })
}

pub struct DownloadStore {
    connection: Connection,
    files: PathBuf,
}
impl DownloadStore {
    /// Fixed native app-data root, not a path accepted from IPC. Initial rules are supplied by
    /// the authority-bound adapter only on first creation; reopening never overwrites rules.
    pub fn open(root: &Path, initial: &DownloadRules) -> Result<Self> {
        if initial.limit_bytes > SAFE_INTEGER {
            return Err(Error::InvalidCommand);
        }
        private_dir(root)?;
        let files = root.join("files");
        private_dir(&files)?;
        let db = root.join("downloads.sqlite3");
        if !db.exists() {
            open_file(&db, true, true)?;
        }
        let flags = OpenFlags::SQLITE_OPEN_READ_WRITE
            | OpenFlags::SQLITE_OPEN_NO_MUTEX
            | OpenFlags::SQLITE_OPEN_NOFOLLOW;
        let connection = Connection::open_with_flags(db, flags).map_err(unavailable)?;
        connection
            .busy_timeout(std::time::Duration::from_secs(2))
            .map_err(unavailable)?;
        connection.execute_batch(SQL).map_err(unavailable)?;
        connection
            .execute(
                "INSERT OR IGNORE INTO download_rules VALUES(1,0,?1,?2,?3,?4,0)",
                params![
                    initial.remove_watched,
                    initial.next_episode,
                    initial.unmetered_only,
                    initial.limit_bytes as i64
                ],
            )
            .map_err(unavailable)?;
        let store = Self { connection, files };
        store.recover()?;
        Ok(store)
    }
    fn ensure_scope(&self, viewer: &DownloadViewer) -> Result<String> {
        let id = scope(viewer)?;
        self.connection
            .execute(
                "INSERT OR IGNORE INTO download_scopes VALUES(?1,?2,?3,?4,?5)",
                params![
                    id,
                    viewer.authority,
                    viewer.account_id,
                    viewer.profile_id,
                    viewer.server_id
                ],
            )
            .map_err(unavailable)?;
        Ok(id)
    }
    fn path(&self, id: &str, ready: bool) -> Result<PathBuf> {
        if !digest(id) {
            return Err(Error::InvalidCommand);
        }
        private_dir(&self.files)?;
        Ok(self
            .files
            .join(format!("{id}.{}", if ready { "media" } else { "part" })))
    }
    fn recover(&self) -> Result<()> {
        let mut query = self
            .connection
            .prepare("SELECT id,bytes_done FROM download_entries WHERE state!='ready'")
            .map_err(unavailable)?;
        let rows = query
            .query_map([], |r| Ok((r.get::<_, String>(0)?, count(r, 1)?)))
            .map_err(unavailable)?;
        for row in rows {
            let (id, committed) = row.map_err(unavailable)?;
            let partial = self.path(&id, false)?;
            let sealed = self.path(&id, true)?;
            // A crash after rename but before the ready transaction never grants playback.
            if sealed.exists() && !partial.exists() {
                std::fs::rename(&sealed, &partial).map_err(unavailable)?;
            }
            let length = if partial.exists() {
                let file = open_file(&partial, true, false)?;
                let length = file.metadata().map_err(unavailable)?.len().min(committed);
                file.set_len(length).map_err(unavailable)?;
                file.sync_all().map_err(unavailable)?;
                length
            } else {
                0
            };
            self.connection.execute("UPDATE download_entries SET bytes_done=?1,state=CASE WHEN held=1 THEN 'paused' ELSE 'queued' END,revision=revision+1 WHERE id=?2",params![length as i64,id]).map_err(unavailable)?;
        }
        Ok(())
    }
    pub fn rules(&self) -> Result<RulesSnapshot> {
        self.connection.query_row("SELECT revision,remove_watched,next_episode,unmetered_only,limit_bytes FROM download_rules WHERE id=1",[],|r|Ok(RulesSnapshot{revision:count(r,0)?,rules:DownloadRules{remove_watched:r.get(1)?,next_episode:r.get(2)?,unmetered_only:r.get(3)?,limit_bytes:count(r,4)?}})).map_err(unavailable)
    }
    pub fn set_rules(&mut self, expected: u64, rules: DownloadRules) -> Result<RulesSnapshot> {
        if expected >= SAFE_INTEGER || rules.limit_bytes > SAFE_INTEGER {
            return Err(Error::InvalidCommand);
        }
        let tx = self
            .connection
            .transaction_with_behavior(TransactionBehavior::Immediate)
            .map_err(unavailable)?;
        if tx.execute("UPDATE download_rules SET revision=revision+1,remove_watched=?1,next_episode=?2,unmetered_only=?3,limit_bytes=?4 WHERE id=1 AND revision=?5",params![rules.remove_watched,rules.next_episode,rules.unmetered_only,rules.limit_bytes as i64,expected as i64]).map_err(unavailable)?!=1{return Err(Error::DownloadsConflict);}
        tx.commit().map_err(unavailable)?;
        self.rules()
    }
    pub fn enroll(&mut self, input: Enrollment) -> Result<LocalDownload> {
        let owner = self.ensure_scope(&input.viewer)?;
        if !identifier(&input.preparation_id)
            || !identifier(&input.item_id)
            || !identifier(&input.quality)
            || !digest(&input.sha256)
            || input.bytes == 0
            || input.bytes > SAFE_INTEGER
            || !title_valid(&input.title)
        {
            return Err(Error::InvalidCommand);
        }
        let id = hash(format!("{owner}\0{}", input.preparation_id).as_bytes());
        if let Ok(old) = self.entry(&input.viewer, &id) {
            return if old.sha256 == input.sha256
                && old.bytes == input.bytes
                && old.item_id == input.item_id
                && old.quality == input.quality
            {
                Ok(old)
            } else {
                Err(Error::DownloadsConflict)
            };
        }
        let tx = self
            .connection
            .transaction_with_behavior(TransactionBehavior::Immediate)
            .map_err(unavailable)?;
        let (limit,reserved):(u64,u64)=tx.query_row("SELECT limit_bytes,(SELECT coalesce(sum(expected_bytes),0) FROM download_entries) FROM download_rules WHERE id=1",[],|r|Ok((count(r,0)?,count(r,1)?))).map_err(unavailable)?;
        if limit > 0
            && reserved
                .checked_add(input.bytes)
                .filter(|n| *n <= limit)
                .is_none()
        {
            return Err(Error::StorageFull);
        }
        tx.execute(
            "INSERT INTO download_entries VALUES(?1,?2,?3,?4,?5,?6,?7,0,'queued',0,0,?8)",
            params![
                id,
                owner,
                input.preparation_id,
                input.item_id,
                input.quality,
                input.sha256,
                input.bytes as i64,
                now_ms()
            ],
        )
        .map_err(unavailable)?;
        let t = input.title;
        tx.execute(
            "INSERT INTO download_titles VALUES(?1,?2,?3,?4,?5,?6,?7,?8,?9,?10,?11,?12,?13)",
            params![
                id,
                t.kind,
                t.title,
                t.artist,
                t.parent_id,
                t.parent_title,
                t.group_id,
                t.group_title,
                t.group_number,
                t.number,
                t.year,
                t.duration_ms as i64,
                t.on_server
            ],
        )
        .map_err(unavailable)?;
        tx.commit().map_err(unavailable)?;
        self.entry(&input.viewer, &id)
    }
    pub fn entry(&self, viewer: &DownloadViewer, id: &str) -> Result<LocalDownload> {
        if !digest(id) {
            return Err(Error::InvalidCommand);
        }
        self.connection
            .query_row(
                &(READ_ENTRY.to_owned() + "WHERE e.scope_id=?1 AND e.id=?2"),
                params![scope(viewer)?, id],
                row_entry,
            )
            .map_err(unavailable)
    }
    pub fn list(
        &self,
        viewer: &DownloadViewer,
        cursor: Option<DownloadCursor>,
        limit: u32,
    ) -> Result<LocalDownloadPage> {
        if limit == 0 || limit > 200 {
            return Err(Error::InvalidCommand);
        }
        let (time, id) = match cursor {
            Some(v) if v.added_ms >= 0 && digest(&v.id) => (v.added_ms, v.id),
            Some(_) => return Err(Error::InvalidCommand),
            None => (i64::MAX, String::new()),
        };
        let sql = READ_ENTRY.to_owned()
            + "WHERE e.scope_id=?1 AND (e.added_ms<?2 OR (e.added_ms=?2 AND e.id>?3)) ORDER BY e.added_ms DESC,e.id LIMIT ?4";
        let mut statement = self.connection.prepare(&sql).map_err(unavailable)?;
        let mut items = statement
            .query_map(params![scope(viewer)?, time, id, limit + 1], row_entry)
            .map_err(unavailable)?
            .collect::<rusqlite::Result<Vec<_>>>()
            .map_err(unavailable)?;
        let next_cursor = if items.len() > limit as usize {
            items.pop();
            items.last().map(|v| DownloadCursor {
                added_ms: v.added_ms,
                id: v.id.clone(),
            })
        } else {
            None
        };
        Ok(LocalDownloadPage { items, next_cursor })
    }
    pub fn append(
        &mut self,
        viewer: &DownloadViewer,
        id: &str,
        expected_revision: u64,
        offset: u64,
        bytes: &[u8],
    ) -> Result<LocalDownload> {
        let entry = self.entry(viewer, id)?;
        if entry.revision != expected_revision {
            return Err(Error::DownloadsConflict);
        }
        if entry.state == "ready"
            || entry.held
            || bytes.is_empty()
            || bytes.len() > CHUNK_BYTES
            || offset != entry.bytes_done
            || offset
                .checked_add(bytes.len() as u64)
                .filter(|n| *n <= entry.bytes)
                .is_none()
        {
            return Err(Error::InvalidCommand);
        }
        let path = self.path(id, false)?;
        let mut file = open_file(&path, true, !path.exists())?;
        if file.metadata().map_err(unavailable)?.len() < offset {
            return Err(Error::DownloadsConflict);
        }
        file.set_len(offset).map_err(unavailable)?;
        file.seek(SeekFrom::Start(offset)).map_err(unavailable)?;
        file.write_all(bytes).map_err(unavailable)?;
        file.sync_data().map_err(unavailable)?;
        let count=self.connection.execute("UPDATE download_entries SET bytes_done=?1,state='downloading',revision=revision+1 WHERE id=?2 AND revision=?3",params![(offset+bytes.len() as u64) as i64,id,expected_revision as i64]).map_err(unavailable)?;
        if count != 1 {
            file.set_len(offset).map_err(unavailable)?;
            return Err(Error::DownloadsConflict);
        }
        self.entry(viewer, id)
    }
    pub fn seal(
        &mut self,
        viewer: &DownloadViewer,
        id: &str,
        expected_revision: u64,
    ) -> Result<LocalDownload> {
        let entry = self.entry(viewer, id)?;
        if entry.revision != expected_revision {
            return Err(Error::DownloadsConflict);
        }
        if entry.held || entry.state == "ready" || entry.bytes_done != entry.bytes {
            return Err(Error::DownloadNotReady);
        }
        let partial = self.path(id, false)?;
        let sealed = self.path(id, true)?;
        let mut file = open_file(&partial, true, false)?;
        if file.metadata().map_err(unavailable)?.len() != entry.bytes {
            return Err(Error::DownloadNotReady);
        }
        let mut hash = Context::new(&SHA256);
        let mut buffer = [0u8; CHUNK_BYTES];
        loop {
            let count = file.read(&mut buffer).map_err(unavailable)?;
            if count == 0 {
                break;
            }
            hash.update(&buffer[..count]);
        }
        if hex(hash.finish().as_ref()) != entry.sha256 {
            self.connection
                .execute(
                    "UPDATE download_entries SET state='failed',revision=revision+1 WHERE id=?1",
                    [id],
                )
                .map_err(unavailable)?;
            return Err(Error::DownloadVerificationFailed);
        }
        file.sync_all().map_err(unavailable)?;
        drop(file);
        let tx = self
            .connection
            .transaction_with_behavior(TransactionBehavior::Immediate)
            .map_err(unavailable)?;
        if sealed.exists() {
            return Err(Error::DownloadsConflict);
        }
        std::fs::rename(&partial, &sealed).map_err(unavailable)?;
        let stored = (|| -> rusqlite::Result<()> {
            tx.execute("UPDATE download_entries SET state='ready',revision=revision+1 WHERE id=?1 AND revision=?2",params![id,expected_revision as i64])?;
            tx.commit()
        })();
        if stored.is_err() {
            std::fs::rename(&sealed, &partial).map_err(unavailable)?;
            return Err(Error::DownloadsUnavailable);
        }
        self.entry(viewer, id)
    }
    pub fn hold(
        &mut self,
        viewer: &DownloadViewer,
        id: &str,
        expected_revision: u64,
        held: bool,
    ) -> Result<LocalDownload> {
        let entry = self.entry(viewer, id)?;
        if entry.revision != expected_revision {
            return Err(Error::DownloadsConflict);
        }
        if entry.state == "ready" {
            return Err(Error::InvalidCommand);
        }
        self.connection.execute("UPDATE download_entries SET held=?1,state=?2,revision=revision+1 WHERE id=?3 AND revision=?4",params![held,if held{"paused"}else{"queued"},id,expected_revision as i64]).map_err(unavailable)?;
        self.entry(viewer, id)
    }
    /// Authenticated JS transport installs the server's receipt key, independently of a receipt.
    /// A key id cannot silently change its public key; a rotation needs a new key id.
    pub fn install_receipt_key(
        &self,
        viewer: &DownloadViewer,
        key_id: &str,
        public_key: &str,
    ) -> Result<()> {
        let owner = self.ensure_scope(viewer)?;
        if !identifier(key_id)
            || public_key.len() > 128
            || URL_SAFE_NO_PAD
                .decode(public_key)
                .map(|v| v.len() != 32)
                .unwrap_or(true)
        {
            return Err(Error::InvalidReceipt);
        }
        self.connection
            .execute(
                "INSERT OR IGNORE INTO download_keys VALUES(?1,?2,?3)",
                params![owner, key_id, public_key],
            )
            .map_err(unavailable)?;
        let saved: String = self
            .connection
            .query_row(
                "SELECT public_key FROM download_keys WHERE scope_id=?1 AND key_id=?2",
                params![owner, key_id],
                |r| r.get(0),
            )
            .map_err(unavailable)?;
        if saved != public_key {
            return Err(Error::InvalidReceipt);
        }
        Ok(())
    }
    fn receipt_time(&self) -> Result<i64> {
        self.connection
            .execute(
                "UPDATE download_rules SET clock_floor_ms=max(clock_floor_ms,?1) WHERE id=1",
                [now_ms()],
            )
            .map_err(unavailable)?;
        self.connection
            .query_row(
                "SELECT clock_floor_ms FROM download_rules WHERE id=1",
                [],
                |r| r.get(0),
            )
            .map_err(unavailable)
    }
    fn check_receipt(
        &self,
        viewer: &DownloadViewer,
        entry: &LocalDownload,
        receipt: &ReceiptEnvelope,
    ) -> Result<i64> {
        let owner = scope(viewer)?;
        let revoked:bool=self.connection.query_row("SELECT EXISTS(SELECT 1 FROM download_revocations WHERE scope_id=?1 AND receipt_id=?2)",params![owner,receipt.receipt_id],|r|r.get(0)).map_err(unavailable)?;
        if revoked {
            return Err(Error::InvalidReceipt);
        }
        let key: String = self
            .connection
            .query_row(
                "SELECT public_key FROM download_keys WHERE scope_id=?1 AND key_id=?2",
                params![owner, receipt.key_id],
                |r| r.get(0),
            )
            .map_err(|_| Error::InvalidReceipt)?;
        let verified = verify_receipt(
            receipt,
            &key,
            &ReceiptBinding {
                viewer,
                item_id: &entry.item_id,
                preparation_id: &entry.preparation_id,
                quality: &entry.quality,
                sha256: &entry.sha256,
                bytes: entry.bytes,
            },
            self.receipt_time()?,
        )?;
        self.connection
            .execute(
                "UPDATE download_rules SET clock_floor_ms=max(clock_floor_ms,?1) WHERE id=1",
                [verified.issued_ms],
            )
            .map_err(unavailable)?;
        Ok(verified.expires_ms)
    }
    pub fn save_receipt(
        &mut self,
        viewer: &DownloadViewer,
        id: &str,
        receipt: ReceiptEnvelope,
    ) -> Result<()> {
        let entry = self.entry(viewer, id)?;
        let expires = self.check_receipt(viewer, &entry, &receipt)?;
        let old: Option<u64> = self
            .connection
            .query_row(
                "SELECT revision FROM download_receipts WHERE entry_id=?1",
                [id],
                |r| count(r, 0),
            )
            .optional()
            .map_err(unavailable)?;
        if old.is_some_and(|v| v >= receipt.revision) {
            return Err(Error::DownloadsConflict);
        }
        self.connection.execute("INSERT INTO download_receipts VALUES(?1,?2,?3,?4,?5,?6,?7) ON CONFLICT(entry_id) DO UPDATE SET receipt_id=excluded.receipt_id,key_id=excluded.key_id,payload=excluded.payload,signature=excluded.signature,revision=excluded.revision,expires_ms=excluded.expires_ms",params![id,receipt.receipt_id,receipt.key_id,receipt.payload,receipt.signature,receipt.revision as i64,expires]).map_err(unavailable)?;
        Ok(())
    }
    pub fn revoke(&self, viewer: &DownloadViewer, receipt_id: &str) -> Result<()> {
        if !identifier(receipt_id) {
            return Err(Error::InvalidCommand);
        }
        self.connection
            .execute(
                "INSERT OR IGNORE INTO download_revocations VALUES(?1,?2)",
                params![self.ensure_scope(viewer)?, receipt_id],
            )
            .map_err(unavailable)?;
        Ok(())
    }
    /// Native caller passes this path directly to the constrained offline mpv load. Never an IPC
    /// result. A complete file without this viewer's valid signed receipt is still not playable.
    pub fn playback_file(&self, viewer: &DownloadViewer, id: &str) -> Result<PathBuf> {
        let entry = self.entry(viewer, id)?;
        if entry.state != "ready" || entry.bytes_done != entry.bytes {
            return Err(Error::DownloadNotReady);
        }
        let receipt=self.connection.query_row("SELECT receipt_id,key_id,payload,signature,revision FROM download_receipts WHERE entry_id=?1",[id],|r|Ok(ReceiptEnvelope{receipt_id:r.get(0)?,key_id:r.get(1)?,algorithm:"ed25519".into(),payload:r.get(2)?,signature:r.get(3)?,revision:count(r,4)?})).map_err(|_|Error::InvalidReceipt)?;
        self.check_receipt(viewer, &entry, &receipt)?;
        let path = self.path(id, true)?;
        let file = open_file(&path, false, false)?;
        if file.metadata().map_err(unavailable)?.len() != entry.bytes {
            return Err(Error::DownloadVerificationFailed);
        }
        Ok(path)
    }
    pub fn note_play(
        &mut self,
        viewer: &DownloadViewer,
        entry_id: &str,
        play: PendingPlay,
    ) -> Result<()> {
        let entry = self.entry(viewer, entry_id)?;
        if play.item_id != entry.item_id
            || !identifier(&play.play_id)
            || play.position_ms > SAFE_INTEGER
            || play.observed_ms < 0
            || play.observed_ms > now_ms() + 300_000
        {
            return Err(Error::InvalidCommand);
        }
        let owner = scope(viewer)?;
        let tx = self
            .connection
            .transaction_with_behavior(TransactionBehavior::Immediate)
            .map_err(unavailable)?;
        tx.execute("INSERT INTO download_plays VALUES(?1,?2,?3,?4,?5,?6) ON CONFLICT(scope_id,play_id) DO UPDATE SET position_ms=excluded.position_ms,ended=max(download_plays.ended,excluded.ended),observed_ms=excluded.observed_ms WHERE download_plays.item_id=excluded.item_id AND excluded.observed_ms>download_plays.observed_ms",params![owner,play.play_id,play.item_id,play.position_ms as i64,play.ended,play.observed_ms]).map_err(unavailable)?;
        tx.execute("DELETE FROM download_plays WHERE scope_id=?1 AND play_id IN (SELECT play_id FROM download_plays WHERE scope_id=?1 ORDER BY observed_ms DESC,play_id LIMIT -1 OFFSET 400)",[owner]).map_err(unavailable)?;
        tx.commit().map_err(unavailable)
    }
    pub fn pending_plays(&self, viewer: &DownloadViewer) -> Result<Vec<PendingPlay>> {
        let mut query=self.connection.prepare("SELECT play_id,item_id,position_ms,ended,observed_ms FROM download_plays WHERE scope_id=?1 ORDER BY observed_ms,play_id LIMIT 100").map_err(unavailable)?;
        query
            .query_map([scope(viewer)?], |r| {
                Ok(PendingPlay {
                    play_id: r.get(0)?,
                    item_id: r.get(1)?,
                    position_ms: count(r, 2)?,
                    ended: r.get(3)?,
                    observed_ms: r.get(4)?,
                })
            })
            .map_err(unavailable)?
            .collect::<rusqlite::Result<Vec<_>>>()
            .map_err(unavailable)
    }
    /// A newer local observation arriving during the HTTP request survives the old response.
    pub fn acknowledge_play(
        &self,
        viewer: &DownloadViewer,
        play_id: &str,
        observed_ms: i64,
    ) -> Result<()> {
        if !identifier(play_id) || observed_ms < 0 {
            return Err(Error::InvalidCommand);
        }
        self.connection
            .execute(
                "DELETE FROM download_plays WHERE scope_id=?1 AND play_id=?2 AND observed_ms=?3",
                params![scope(viewer)?, play_id, observed_ms],
            )
            .map_err(unavailable)?;
        Ok(())
    }
    pub fn usage(&self, viewer: &DownloadViewer) -> Result<(u64, u64)> {
        self.connection.query_row("SELECT coalesce(sum(CASE WHEN scope_id=?1 THEN bytes_done ELSE 0 END),0),coalesce(sum(CASE WHEN scope_id!=?1 THEN bytes_done ELSE 0 END),0) FROM download_entries",[scope(viewer)?],|r|Ok((count(r,0)?,count(r,1)?))).map_err(unavailable)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    fn rules() -> DownloadRules {
        DownloadRules {
            remove_watched: false,
            next_episode: false,
            unmetered_only: true,
            limit_bytes: 1024,
        }
    }
    fn root() -> PathBuf {
        std::env::temp_dir().join(format!(
            "desktop-download-{}-{}",
            std::process::id(),
            SystemTime::now()
                .duration_since(UNIX_EPOCH)
                .unwrap()
                .as_nanos()
        ))
    }
    fn viewer() -> DownloadViewer {
        DownloadViewer {
            authority: "local".into(),
            account_id: "account".into(),
            profile_id: "profile".into(),
            server_id: "server".into(),
        }
    }
    fn enrollment(bytes: &[u8]) -> Enrollment {
        Enrollment {
            viewer: viewer(),
            preparation_id: "preparation".into(),
            item_id: "item".into(),
            quality: "original".into(),
            sha256: hash(bytes),
            bytes: bytes.len() as u64,
            title: DownloadTitle {
                kind: "movie".into(),
                title: "A genuine title".into(),
                artist: String::new(),
                parent_id: String::new(),
                parent_title: String::new(),
                group_id: String::new(),
                group_title: String::new(),
                group_number: None,
                number: None,
                year: 2026,
                duration_ms: 10_000,
                on_server: true,
            },
        }
    }
    #[test]
    fn durable_chunks_recover_and_ready_requires_exact_hash_then_signed_viewer_receipt() {
        let root = root();
        let mut store = DownloadStore::open(&root, &rules()).unwrap();
        let entry = store.enroll(enrollment(b"hello")).unwrap();
        let partial = store.append(&viewer(), &entry.id, 0, 0, b"hel").unwrap();
        assert_eq!(partial.bytes_done, 3);
        drop(store);
        let mut store = DownloadStore::open(&root, &rules()).unwrap();
        let resumed = store.entry(&viewer(), &entry.id).unwrap();
        let complete = store
            .append(&viewer(), &entry.id, resumed.revision, 3, b"lo")
            .unwrap();
        assert!(store.playback_file(&viewer(), &entry.id).is_err());
        let ready = store.seal(&viewer(), &entry.id, complete.revision).unwrap();
        assert_eq!(ready.state, "ready");
        assert_eq!(
            store.playback_file(&viewer(), &entry.id).unwrap_err(),
            Error::InvalidReceipt
        );
        let (receipt, key) = crate::download_receipt::tests::signed(&viewer(), &ready.sha256, 5);
        store.install_receipt_key(&viewer(), "key", &key).unwrap();
        store.save_receipt(&viewer(), &entry.id, receipt).unwrap();
        assert_eq!(
            std::fs::read(store.playback_file(&viewer(), &entry.id).unwrap()).unwrap(),
            b"hello"
        );
        let other = DownloadViewer {
            profile_id: "other".into(),
            ..viewer()
        };
        assert!(store.playback_file(&other, &entry.id).is_err());
        store.revoke(&viewer(), "receipt").unwrap();
        assert_eq!(
            store.playback_file(&viewer(), &entry.id).unwrap_err(),
            Error::InvalidReceipt
        );
        drop(store);
        std::fs::remove_dir_all(root).unwrap();
    }
    #[test]
    fn offsets_revisions_quota_and_hash_mismatch_never_acknowledge_ready() {
        let root = root();
        let mut store = DownloadStore::open(&root, &rules()).unwrap();
        let entry = store.enroll(enrollment(b"hello")).unwrap();
        assert!(store.append(&viewer(), &entry.id, 0, 1, b"h").is_err());
        let partial = store.append(&viewer(), &entry.id, 0, 0, b"jello").unwrap();
        assert_eq!(
            store.append(&viewer(), &entry.id, 0, 0, b"h").unwrap_err(),
            Error::DownloadsConflict
        );
        assert_eq!(
            store
                .seal(&viewer(), &entry.id, partial.revision)
                .unwrap_err(),
            Error::DownloadVerificationFailed
        );
        assert_ne!(store.entry(&viewer(), &entry.id).unwrap().state, "ready");
        assert_eq!(
            store.enroll(enrollment(&[3; 2048])).unwrap_err(),
            Error::DownloadsConflict
        );
        let mut large = enrollment(&[3; 2048]);
        large.preparation_id = "large".into();
        assert_eq!(store.enroll(large).unwrap_err(), Error::StorageFull);
        drop(store);
        std::fs::remove_dir_all(root).unwrap();
    }
    #[test]
    fn only_committed_bytes_survive_a_crash_and_user_pause_survives_reopen() {
        let root = root();
        let mut store = DownloadStore::open(&root, &rules()).unwrap();
        let entry = store.enroll(enrollment(b"hello")).unwrap();
        let partial = store.append(&viewer(), &entry.id, 0, 0, b"he").unwrap();
        store
            .hold(&viewer(), &entry.id, partial.revision, true)
            .unwrap();
        OpenOptions::new()
            .append(true)
            .open(store.path(&entry.id, false).unwrap())
            .unwrap()
            .write_all(b"uncommitted")
            .unwrap();
        drop(store);
        let store = DownloadStore::open(&root, &rules()).unwrap();
        let saved = store.entry(&viewer(), &entry.id).unwrap();
        assert_eq!(saved.state, "paused");
        assert!(saved.held);
        assert_eq!(saved.bytes_done, 2);
        assert_eq!(
            std::fs::read(store.path(&entry.id, false).unwrap()).unwrap(),
            b"he"
        );
        drop(store);
        std::fs::remove_dir_all(root).unwrap();
    }
    #[test]
    fn deferred_progress_is_viewer_owned_and_a_newer_position_survives_an_old_ack() {
        let root = root();
        let mut store = DownloadStore::open(&root, &rules()).unwrap();
        let entry = store.enroll(enrollment(b"hello")).unwrap();
        let now = now_ms();
        store
            .note_play(
                &viewer(),
                &entry.id,
                PendingPlay {
                    play_id: "play".into(),
                    item_id: "item".into(),
                    position_ms: 1000,
                    ended: false,
                    observed_ms: now,
                },
            )
            .unwrap();
        store
            .note_play(
                &viewer(),
                &entry.id,
                PendingPlay {
                    play_id: "play".into(),
                    item_id: "item".into(),
                    position_ms: 2000,
                    ended: true,
                    observed_ms: now + 1,
                },
            )
            .unwrap();
        store.acknowledge_play(&viewer(), "play", now).unwrap();
        assert_eq!(store.pending_plays(&viewer()).unwrap()[0].position_ms, 2000);
        let other = DownloadViewer {
            account_id: "other".into(),
            ..viewer()
        };
        assert!(store.pending_plays(&other).unwrap().is_empty());
        store.acknowledge_play(&viewer(), "play", now + 1).unwrap();
        assert!(store.pending_plays(&viewer()).unwrap().is_empty());
        drop(store);
        std::fs::remove_dir_all(root).unwrap();
    }
    #[test]
    fn native_rules_revisions_are_committed_and_reopening_never_resets_them() {
        let root = root();
        let mut store = DownloadStore::open(&root, &rules()).unwrap();
        let changed = DownloadRules {
            limit_bytes: 2000,
            ..rules()
        };
        assert_eq!(store.set_rules(0, changed.clone()).unwrap().revision, 1);
        assert_eq!(
            store.set_rules(0, rules()).unwrap_err(),
            Error::DownloadsConflict
        );
        drop(store);
        let store = DownloadStore::open(&root, &rules()).unwrap();
        assert_eq!(store.rules().unwrap().rules, changed);
        drop(store);
        std::fs::remove_dir_all(root).unwrap();
    }
}
