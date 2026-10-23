//! Closed app-owned IPC operations. No URL, arbitrary path, shell command or auth token.
use crate::{
    boundary::{Error, Result},
    download_receipt::{DownloadViewer, ReceiptEnvelope, identifier},
    download_store::*,
};
use ring::rand::{SecureRandom, SystemRandom};
use serde::{
    Deserialize, Serialize,
    de::{SeqAccess, Visitor},
};
use std::{
    collections::HashMap,
    fmt,
    path::{Path, PathBuf},
};

#[derive(Clone, Deserialize, Serialize, PartialEq, Eq)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct ViewerScope {
    pub authority: String,
    pub server_id: String,
    pub account_id: String,
    pub profile_id: String,
    pub session_family_id: String,
}
impl ViewerScope {
    fn viewer(&self) -> Result<DownloadViewer> {
        let viewer = DownloadViewer {
            authority: self.authority.clone(),
            server_id: self.server_id.clone(),
            account_id: self.account_id.clone(),
            profile_id: self.profile_id.clone(),
        };
        viewer.validate()?;
        if !identifier(&self.session_family_id) {
            return Err(Error::InvalidCommand);
        }
        Ok(viewer)
    }
}
/// Reject before allocating an unbounded serde Vec, including malformed oversized input.
#[derive(Debug)]
pub struct Bytes<const LIMIT: usize>(pub Vec<u8>);
impl<'de, const LIMIT: usize> Deserialize<'de> for Bytes<LIMIT> {
    fn deserialize<D: serde::Deserializer<'de>>(
        deserializer: D,
    ) -> std::result::Result<Self, D::Error> {
        struct Bounded<const N: usize>;
        impl<'de, const N: usize> Visitor<'de> for Bounded<N> {
            type Value = Bytes<N>;
            fn expecting(&self, f: &mut fmt::Formatter) -> fmt::Result {
                write!(f, "at most {N} bytes")
            }
            fn visit_seq<A: SeqAccess<'de>>(
                self,
                mut sequence: A,
            ) -> std::result::Result<Self::Value, A::Error> {
                if sequence.size_hint().is_some_and(|n| n > N) {
                    return Err(serde::de::Error::custom("invalid_command"));
                }
                let mut bytes = Vec::with_capacity(sequence.size_hint().unwrap_or(0).min(N));
                while let Some(byte) = sequence.next_element::<u8>()? {
                    if bytes.len() == N {
                        return Err(serde::de::Error::custom("invalid_command"));
                    }
                    bytes.push(byte);
                }
                Ok(Bytes(bytes))
            }
        }
        deserializer.deserialize_seq(Bounded::<LIMIT>)
    }
}
#[derive(Deserialize)]
#[serde(
    tag = "action",
    rename_all = "camelCase",
    rename_all_fields = "camelCase",
    deny_unknown_fields
)]
pub enum DownloadOperation {
    PrepareLocalQueue {
        selection: LocalSelection,
    },
    ReleaseLocalQueue {
        preparation_id: String,
    },
    List {
        cursor: Option<DownloadCursor>,
        limit: u32,
    },
    Entry {
        id: String,
    },
    Capacity {},
    Groups {},
    GroupMembers {
        selector: GroupSelector,
    },
    GroupAction {
        selector: GroupSelector,
        expected_revision: String,
        operation: GroupAction,
    },
    PlayableGroup {
        selector: GroupSelector,
        expected_revision: String,
    },
    Rules {},
    SetRules {
        expected_revision: u64,
        value: DownloadRules,
    },
    Enroll {
        preparation_id: String,
        item_id: String,
        quality: String,
        sha256: String,
        bytes: u64,
        title: DownloadTitle,
    },
    Append {
        id: String,
        expected_revision: u64,
        offset: u64,
        bytes: Bytes<262144>,
    },
    Seal {
        id: String,
        expected_revision: u64,
    },
    Hold {
        id: String,
        expected_revision: u64,
        held: bool,
    },
    Retry {
        id: String,
        expected_revision: u64,
    },
    Remove {
        id: String,
        expected_revision: u64,
    },
    ReceiptKey {
        key_id: String,
        public_key: String,
    },
    Receipt {
        id: String,
        envelope: ReceiptEnvelope,
    },
    Revoke {
        receipt_id: String,
    },
    Artwork {
        id: String,
    },
    SaveArtwork {
        id: String,
        expected_revision: u64,
        mime: String,
        bytes: Bytes<4194304>,
    },
    PreviewOtherProfiles {},
    RemoveOtherProfiles {
        snapshot_token: String,
    },
    PendingPlays {},
    AcknowledgePlay {
        play_id: String,
        observed_ms: i64,
    },
    ServerPresence {
        id: String,
        expected_revision: u64,
        on_server: bool,
    },
}
#[derive(Serialize)]
#[serde(tag = "kind", content = "value", rename_all = "camelCase")]
pub enum DownloadReply {
    LocalQueue(PreparedLocalQueue),
    Page(LocalDownloadPage),
    Entry(LocalDownload),
    Capacity(DownloadCapacity),
    Groups(Vec<DownloadGroup>),
    Members(Vec<LocalDownload>),
    Rules(RulesSnapshot),
    Artwork(Option<DownloadArtwork>),
    OtherProfiles(OtherDownloadsPreview),
    PendingPlays(Vec<PendingPlay>),
    Done,
}
struct Binding {
    scope: ViewerScope,
    lease: String,
    references: u32,
}
pub struct DownloadsSession {
    store: DownloadStore,
    binding: Option<Binding>,
    local_sources: HashMap<String, LocalSource>,
    local_preparation: Option<String>,
    active_local: Option<ActiveLocal>,
}
#[derive(Deserialize)]
#[serde(
    tag = "kind",
    rename_all = "camelCase",
    rename_all_fields = "camelCase",
    deny_unknown_fields
)]
pub enum LocalSelection {
    Entry {
        entry_id: String,
        expected_revision: u64,
    },
    Group {
        selector: GroupSelector,
        expected_revision: String,
    },
}
#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
pub struct PreparedLocalQueue {
    pub preparation_id: String,
    pub scope: ViewerScope,
    pub selection_revision: String,
    pub items: Vec<PreparedLocalItem>,
}
#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
pub struct PreparedLocalItem {
    pub id: String,
    pub revision: u64,
    pub item_id: String,
    pub title: DownloadTitle,
    pub duration_ms: u64,
    pub local_source_id: String,
}
struct ActiveLocal {
    viewer: DownloadViewer,
    entry_id: String,
    item_id: String,
    play_id: String,
    generation: u64,
    last_write: std::time::Instant,
    last_position_ms: u64,
}
struct LocalSource {
    entry_id: String,
    entry_revision: u64,
}
fn random_id() -> Result<String> {
    let mut bytes = [0u8; 32];
    SystemRandom::new()
        .fill(&mut bytes)
        .map_err(|_| Error::DownloadsUnavailable)?;
    Ok(bytes.iter().map(|b| format!("{b:02x}")).collect())
}
impl DownloadsSession {
    pub fn open(root: &Path) -> Result<Self> {
        Ok(Self {
            store: DownloadStore::open(
                root,
                &DownloadRules {
                    remove_watched: false,
                    next_episode: false,
                    unmetered_only: true,
                    limit_bytes: 0,
                },
            )?,
            binding: None,
            local_sources: HashMap::new(),
            local_preparation: None,
            active_local: None,
        })
    }
    pub fn bind(&mut self, scope: ViewerScope) -> Result<String> {
        scope.viewer()?;
        if let Some(binding) = self.binding.as_mut() {
            if binding.scope == scope {
                binding.references = binding
                    .references
                    .checked_add(1)
                    .filter(|n| *n <= 1024)
                    .ok_or(Error::DownloadsUnavailable)?;
                return Ok(binding.lease.clone());
            }
        }
        let mut bytes = [0u8; 32];
        SystemRandom::new()
            .fill(&mut bytes)
            .map_err(|_| Error::DownloadsUnavailable)?;
        let lease = bytes.iter().map(|b| format!("{b:02x}")).collect::<String>();
        self.store.retire_confirmations();
        self.binding = Some(Binding {
            scope,
            lease: lease.clone(),
            references: 1,
        });
        self.local_sources.clear();
        self.local_preparation = None;
        self.active_local = None;
        Ok(lease)
    }
    pub fn binding_changes(&self, scope: &ViewerScope) -> Result<bool> {
        scope.viewer()?;
        Ok(self.binding.as_ref().is_some_and(|b| b.scope != *scope))
    }
    pub fn lease_current(&self, lease: &str) -> bool {
        self.binding.as_ref().is_some_and(|b| b.lease == lease)
    }
    pub fn release(&mut self, lease: &str) {
        if let Some(binding) = self.binding.as_mut() {
            if binding.lease == lease {
                binding.references = binding.references.saturating_sub(1);
            }
        }
        self.release_if_unowned();
    }
    fn release_if_unowned(&mut self) {
        if self.binding.as_ref().is_some_and(|b| b.references == 0)
            && self.local_preparation.is_none()
            && self.active_local.is_none()
        {
            self.binding = None;
            self.local_sources.clear();
            self.store.retire_confirmations();
        }
    }
    pub fn force_release(&mut self, lease: &str) {
        if self.lease_current(lease) {
            self.binding = None;
            self.local_sources.clear();
            self.local_preparation = None;
            self.active_local = None;
            self.store.retire_confirmations();
        }
    }
    fn viewer(&self, lease: &str) -> Result<DownloadViewer> {
        self.binding
            .as_ref()
            .filter(|b| b.lease == lease)
            .ok_or(Error::DownloadsConflict)?
            .scope
            .viewer()
    }
    pub fn execute(&mut self, lease: &str, operation: DownloadOperation) -> Result<DownloadReply> {
        let viewer = self.viewer(lease)?;
        use DownloadOperation as O;
        use DownloadReply as R;
        Ok(match operation {
            O::PrepareLocalQueue { selection } => {
                R::LocalQueue(self.prepare_local_queue(lease, selection)?)
            }
            O::ReleaseLocalQueue { preparation_id } => {
                if self.local_preparation.as_deref() == Some(&preparation_id) {
                    self.local_sources.clear();
                    self.local_preparation = None;
                    self.release_if_unowned();
                }
                R::Done
            }
            O::List { cursor, limit } => R::Page(self.store.list(&viewer, cursor, limit)?),
            O::Entry { id } => R::Entry(self.store.entry(&viewer, &id)?),
            O::Capacity {} => R::Capacity(self.store.capacity(&viewer)?),
            O::Groups {} => R::Groups(self.store.groups(&viewer)?),
            O::GroupMembers { selector } => {
                R::Members(self.store.group_members(&viewer, &selector)?)
            }
            O::GroupAction {
                selector,
                expected_revision,
                operation,
            } => {
                self.store
                    .act_group(&viewer, &selector, &expected_revision, operation)?;
                R::Done
            }
            O::PlayableGroup {
                selector,
                expected_revision,
            } => R::Members(
                self.store
                    .playable_group(&viewer, &selector, &expected_revision)?,
            ),
            O::Rules {} => R::Rules(self.store.rules()?),
            O::SetRules {
                expected_revision,
                value,
            } => R::Rules(self.store.set_rules(expected_revision, value)?),
            O::Enroll {
                preparation_id,
                item_id,
                quality,
                sha256,
                bytes,
                title,
            } => R::Entry(self.store.enroll(Enrollment {
                viewer,
                preparation_id,
                item_id,
                quality,
                sha256,
                bytes,
                title,
            })?),
            O::Append {
                id,
                expected_revision,
                offset,
                bytes,
            } => R::Entry(
                self.store
                    .append(&viewer, &id, expected_revision, offset, &bytes.0)?,
            ),
            O::Seal {
                id,
                expected_revision,
            } => R::Entry(self.store.seal(&viewer, &id, expected_revision)?),
            O::Hold {
                id,
                expected_revision,
                held,
            } => R::Entry(self.store.hold(&viewer, &id, expected_revision, held)?),
            O::Retry {
                id,
                expected_revision,
            } => R::Entry(self.store.retry(&viewer, &id, expected_revision)?),
            O::Remove {
                id,
                expected_revision,
            } => {
                self.store.remove(&viewer, &id, expected_revision)?;
                R::Done
            }
            O::ReceiptKey { key_id, public_key } => {
                self.store
                    .install_receipt_key(&viewer, &key_id, &public_key)?;
                R::Done
            }
            O::Receipt { id, envelope } => {
                self.store.save_receipt(&viewer, &id, envelope)?;
                R::Done
            }
            O::Revoke { receipt_id } => {
                self.store.revoke(&viewer, &receipt_id)?;
                R::Done
            }
            O::Artwork { id } => R::Artwork(self.store.artwork(&viewer, &id)?),
            O::SaveArtwork {
                id,
                expected_revision,
                mime,
                bytes,
            } => R::Entry(self.store.save_artwork(
                &viewer,
                &id,
                expected_revision,
                &mime,
                &bytes.0,
            )?),
            O::PreviewOtherProfiles {} => {
                R::OtherProfiles(self.store.preview_other_profiles(&viewer)?)
            }
            O::RemoveOtherProfiles { snapshot_token } => {
                self.store.remove_other_profiles(&viewer, &snapshot_token)?;
                R::Done
            }
            O::PendingPlays {} => R::PendingPlays(self.store.pending_plays(&viewer)?),
            O::AcknowledgePlay {
                play_id,
                observed_ms,
            } => {
                self.store
                    .acknowledge_play(&viewer, &play_id, observed_ms)?;
                R::Done
            }
            O::ServerPresence {
                id,
                expected_revision,
                on_server,
            } => {
                R::Entry(
                    self.store
                        .server_presence(&viewer, &id, expected_revision, on_server)?,
                )
            }
        })
    }
}

impl DownloadsSession {
    fn prepare_local_queue(
        &mut self,
        lease: &str,
        selection: LocalSelection,
    ) -> Result<PreparedLocalQueue> {
        let viewer = self.viewer(lease)?;
        let (members, selection_revision) = match selection {
            LocalSelection::Entry {
                entry_id,
                expected_revision,
            } => {
                let member = self.store.entry(&viewer, &entry_id)?;
                if member.revision != expected_revision {
                    return Err(Error::DownloadsConflict);
                }
                self.store.playback_file(&viewer, &entry_id)?;
                (vec![member], expected_revision.to_string())
            }
            LocalSelection::Group {
                selector,
                expected_revision,
            } => {
                let members = self
                    .store
                    .playable_group(&viewer, &selector, &expected_revision)?;
                (members, expected_revision)
            }
        };
        // Do not replace a usable preparation until the complete requested set verified.
        let preparation_id = random_id()?;
        let mut sources = HashMap::new();
        let mut items = Vec::with_capacity(members.len());
        for member in members {
            let local_source_id = random_id()?;
            sources.insert(
                local_source_id.clone(),
                LocalSource {
                    entry_id: member.id.clone(),
                    entry_revision: member.revision,
                },
            );
            items.push(PreparedLocalItem {
                id: member.id,
                revision: member.revision,
                item_id: member.item_id,
                duration_ms: member.title.duration_ms,
                title: member.title,
                local_source_id,
            });
        }
        self.local_sources = sources;
        self.local_preparation = Some(preparation_id.clone());
        Ok(PreparedLocalQueue {
            preparation_id,
            scope: self
                .binding
                .as_ref()
                .ok_or(Error::DownloadsConflict)?
                .scope
                .clone(),
            selection_revision,
            items,
        })
    }
    pub fn begin_local(&mut self, lease: &str, source_id: &str, generation: u64) -> Result<()> {
        let viewer = self.viewer(lease)?;
        let source = self
            .local_sources
            .get(source_id)
            .ok_or(Error::DownloadsConflict)?;
        let entry = self.store.entry(&viewer, &source.entry_id)?;
        self.active_local = Some(ActiveLocal {
            viewer,
            entry_id: entry.id,
            item_id: entry.item_id,
            play_id: random_id()?,
            generation,
            last_write: std::time::Instant::now(),
            last_position_ms: 0,
        });
        Ok(())
    }
    pub fn active_generation(&self) -> Option<u64> {
        self.active_local.as_ref().map(|p| p.generation)
    }
    pub fn active_file_valid(&self) -> bool {
        self.active_local.as_ref().is_none_or(|p| self.store.playback_file(&p.viewer, &p.entry_id).is_ok())
    }
    pub fn retire_local(&mut self) {
        self.active_local = None;
        self.release_if_unowned();
    }
    /// Progress is produced by actual native observations, never by a settings/queue intent.
    pub fn record_observation(
        &mut self,
        observation: &crate::boundary::Observation,
        flush: bool,
    ) -> Result<()> {
        let Some(active) = self.active_local.as_mut() else {
            return Ok(());
        };
        if observation.generation != active.generation
            || !observation.position_ms.is_finite()
            || observation.position_ms <= 0.0
            || !matches!(
                observation.state.as_str(),
                "playing" | "paused" | "buffering" | "ended"
            )
        {
            return Ok(());
        }
        let position_ms = observation
            .position_ms
            .round()
            .min(crate::download_receipt::SAFE_INTEGER as f64) as u64;
        if !flush
            && observation.state != "ended"
            && (active.last_write.elapsed() < std::time::Duration::from_secs(5)
                || position_ms == active.last_position_ms)
        {
            return Ok(());
        }
        let observed_ms = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .map_err(|_| Error::DownloadsUnavailable)?
            .as_millis()
            .min(i64::MAX as u128) as i64;
        self.store.note_play(
            &active.viewer,
            &active.entry_id,
            PendingPlay {
                play_id: active.play_id.clone(),
                item_id: active.item_id.clone(),
                position_ms,
                ended: observation.state == "ended",
                observed_ms,
            },
        )?;
        active.last_write = std::time::Instant::now();
        active.last_position_ms = position_ms;
        Ok(())
    }
    /// Private native lookup only. The path never enters a serialized reply or web source.
    pub fn local_file(&self, lease: &str, local_source_id: &str) -> Result<PathBuf> {
        let viewer = self.viewer(lease)?;
        let source = self
            .local_sources
            .get(local_source_id)
            .ok_or(Error::DownloadsConflict)?;
        if self.store.entry(&viewer, &source.entry_id)?.revision != source.entry_revision {
            return Err(Error::DownloadsConflict);
        }
        self.store.playback_file(&viewer, &source.entry_id)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    fn scope() -> ViewerScope {
        ViewerScope {
            authority: "local".into(),
            server_id: "server".into(),
            account_id: "account".into(),
            profile_id: "profile".into(),
            session_family_id: "family".into(),
        }
    }
    #[test]
    fn local_handles_require_receipt_and_expire_on_release_rebind_and_row_change() {
        let root = std::env::temp_dir().join(format!(
            "desktop-local-handles-{}-{}",
            std::process::id(),
            std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .unwrap()
                .as_nanos()
        ));
        let mut session = DownloadsSession::open(&root).unwrap();
        let lease = session.bind(scope()).unwrap();
        let viewer = scope().viewer().unwrap();
        let sha = ring::digest::digest(&ring::digest::SHA256, b"media")
            .as_ref()
            .iter()
            .map(|b| format!("{b:02x}"))
            .collect::<String>();
        let entry = session
            .store
            .enroll(Enrollment {
                viewer: viewer.clone(),
                preparation_id: "preparation".into(),
                item_id: "item".into(),
                quality: "original".into(),
                sha256: sha.clone(),
                bytes: 5,
                title: DownloadTitle {
                    kind: "movie".into(),
                    title: "Actual title".into(),
                    artist: String::new(),
                    parent_id: String::new(),
                    parent_title: String::new(),
                    group_id: String::new(),
                    group_title: String::new(),
                    group_number: None,
                    number: None,
                    year: 2026,
                    duration_ms: 1000,
                    on_server: true,
                },
            })
            .unwrap();
        let entry = session
            .store
            .append(&viewer, &entry.id, 0, 0, b"media")
            .unwrap();
        let entry = session
            .store
            .seal(&viewer, &entry.id, entry.revision)
            .unwrap();
        assert!(
            session
                .prepare_local_queue(
                    &lease,
                    LocalSelection::Entry {
                        entry_id: entry.id.clone(),
                        expected_revision: entry.revision
                    }
                )
                .is_err()
        );
        let (receipt, key) = crate::download_receipt::tests::signed(&viewer, &sha, 5);
        session
            .store
            .install_receipt_key(&viewer, "key", &key)
            .unwrap();
        session
            .store
            .save_receipt(&viewer, &entry.id, receipt)
            .unwrap();
        let prepared = session
            .prepare_local_queue(
                &lease,
                LocalSelection::Entry {
                    entry_id: entry.id.clone(),
                    expected_revision: entry.revision,
                },
            )
            .unwrap();
        assert_eq!(prepared.items.len(), 1);
        let source = &prepared.items[0].local_source_id;
        assert_eq!(
            std::fs::read(session.local_file(&lease, source).unwrap()).unwrap(),
            b"media"
        );
        let json = serde_json::to_string(&prepared).unwrap();
        assert!(!json.contains(".media"));
        assert!(!json.contains(root.to_str().unwrap()));
        session.store.revoke(&viewer, "receipt").unwrap();
        assert!(session.local_file(&lease, source).is_err());
        session
            .execute(
                &lease,
                DownloadOperation::ReleaseLocalQueue {
                    preparation_id: prepared.preparation_id,
                },
            )
            .unwrap();
        assert!(session.local_file(&lease, source).is_err());
        let next = session.bind(scope()).unwrap();
        assert!(session.local_file(&next, source).is_err());
        assert!(session.local_file(&lease, source).is_err());
        drop(session);
        std::fs::remove_dir_all(root).unwrap();
    }
    #[test]
    fn closed_commands_bound_byte_allocation_and_reject_extra_paths() {
        assert!(
            serde_json::from_str::<DownloadOperation>(r#"{"action":"capacity","path":"/private"}"#)
                .is_err()
        );
        assert!(serde_json::from_str::<Bytes<2>>("[1,2,3]").is_err());
        assert_eq!(serde_json::from_str::<Bytes<2>>("[1,2]").unwrap().0, [1, 2]);
        assert!(
            serde_json::from_str::<DownloadOperation>(r#"{"action":"run","command":"ffmpeg"}"#)
                .is_err()
        );
    }
    #[test]
    fn shared_bind_reference_counts_and_family_confirmation_retirement() {
        let root=std::env::temp_dir().join(format!("desktop-download-refs-{}-{}",std::process::id(),std::time::SystemTime::now().duration_since(std::time::UNIX_EPOCH).unwrap().as_nanos()));
        let mut session=DownloadsSession::open(&root).unwrap();
        let lease=session.bind(scope()).unwrap();
        assert_eq!(lease,session.bind(scope()).unwrap());
        let token=session.store.preview_other_profiles(&scope().viewer().unwrap()).unwrap().snapshot_token;
        session.release(&lease);
        assert!(session.execute(&lease,DownloadOperation::Rules{}).is_ok());
        let mut next=scope();next.session_family_id="new_family".into();
        let new_lease=session.bind(next).unwrap();
        assert!(matches!(session.execute(&new_lease,DownloadOperation::RemoveOtherProfiles{snapshot_token:token}),Err(Error::DownloadsConflict)));
        session.local_preparation=Some("retained".into());
        session.release(&new_lease);
        assert!(session.lease_current(&new_lease));
        session.execute(&new_lease,DownloadOperation::ReleaseLocalQueue{preparation_id:"retained".into()}).unwrap();
        assert!(!session.lease_current(&new_lease));
        drop(session);std::fs::remove_dir_all(root).unwrap();
    }
    #[test]
    fn native_progress_requires_actual_current_generation_and_rejects_intent_and_nonfinite_time() {
        let root=std::env::temp_dir().join(format!("desktop-download-observation-{}-{}",std::process::id(),std::time::SystemTime::now().duration_since(std::time::UNIX_EPOCH).unwrap().as_nanos()));
        let mut session=DownloadsSession::open(&root).unwrap();let lease=session.bind(scope()).unwrap();let viewer=scope().viewer().unwrap();
        let entry=session.store.enroll(Enrollment{viewer:viewer.clone(),preparation_id:"preparation".into(),item_id:"item".into(),quality:"original".into(),sha256:"a".repeat(64),bytes:5,title:DownloadTitle{kind:"song".into(),title:"Actual title".into(),artist:String::new(),parent_id:String::new(),parent_title:String::new(),group_id:String::new(),group_title:String::new(),group_number:None,number:None,year:2026,duration_ms:10000,on_server:true}}).unwrap();
        session.active_local=Some(ActiveLocal{viewer:viewer.clone(),entry_id:entry.id,item_id:"item".into(),play_id:"play".into(),generation:7,last_write:std::time::Instant::now(),last_position_ms:0});
        let mut observation=crate::boundary::Observation{generation:6,state:"playing".into(),position_ms:1234.0,rate:1.0,volume:1.0,muted:false,dropped_frames:0,native_surface_active:false,eq_applied:false,leveling_applied:false};
        session.record_observation(&observation,true).unwrap();assert!(session.store.pending_plays(&viewer).unwrap().is_empty());
        observation.generation=7;observation.state="loading".into();session.record_observation(&observation,true).unwrap();assert!(session.store.pending_plays(&viewer).unwrap().is_empty());
        observation.state="playing".into();observation.position_ms=f64::NAN;session.record_observation(&observation,true).unwrap();assert!(session.store.pending_plays(&viewer).unwrap().is_empty());
        observation.position_ms=1234.0;session.record_observation(&observation,false).unwrap();assert!(session.store.pending_plays(&viewer).unwrap().is_empty());
        session.record_observation(&observation,true).unwrap();let first=session.store.pending_plays(&viewer).unwrap();assert_eq!(first[0].position_ms,1234);assert!(!first[0].ended);
        observation.state="ended".into();observation.position_ms=10000.0;
        session.store.acknowledge_play(&viewer,"play",first[0].observed_ms).unwrap();
        session.record_observation(&observation,false).unwrap();assert!(session.store.pending_plays(&viewer).unwrap()[0].ended);
        session.force_release(&lease);drop(session);std::fs::remove_dir_all(root).unwrap();
    }
    #[test]
    fn rebound_or_released_family_cannot_write_and_rules_ack_is_durable() {
        let root = std::env::temp_dir().join(format!(
            "desktop-download-ipc-{}-{}",
            std::process::id(),
            std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .unwrap()
                .as_nanos()
        ));
        let mut session = DownloadsSession::open(&root).unwrap();
        let old = session.bind(scope()).unwrap();
        let mut next = scope();
        next.session_family_id = "next".into();
        let current = session.bind(next).unwrap();
        assert!(matches!(
            session.execute(
                &old,
                DownloadOperation::SetRules {
                    expected_revision: 0,
                    value: DownloadRules {
                        remove_watched: true,
                        next_episode: true,
                        unmetered_only: false,
                        limit_bytes: 1234
                    }
                }
            ),
            Err(Error::DownloadsConflict)
        ));
        let reply = session
            .execute(
                &current,
                DownloadOperation::SetRules {
                    expected_revision: 0,
                    value: DownloadRules {
                        remove_watched: true,
                        next_episode: false,
                        unmetered_only: true,
                        limit_bytes: 1234,
                    },
                },
            )
            .unwrap();
        assert!(matches!(
            reply,
            DownloadReply::Rules(RulesSnapshot { revision: 1, .. })
        ));
        session.release(&old);
        assert!(
            session
                .execute(&current, DownloadOperation::Capacity {})
                .is_ok()
        );
        session.release(&current);
        assert!(matches!(
            session.execute(&current, DownloadOperation::Capacity {}),
            Err(Error::DownloadsConflict)
        ));
        drop(session);
        let mut session = DownloadsSession::open(&root).unwrap();
        let lease = session.bind(scope()).unwrap();
        let reply = session
            .execute(&lease, DownloadOperation::Rules {})
            .unwrap();
        assert!(matches!(
            reply,
            DownloadReply::Rules(RulesSnapshot {
                revision: 1,
                rules: DownloadRules {
                    limit_bytes: 1234,
                    remove_watched: true,
                    ..
                }
            })
        ));
        drop(session);
        std::fs::remove_dir_all(root).unwrap();
    }
}
