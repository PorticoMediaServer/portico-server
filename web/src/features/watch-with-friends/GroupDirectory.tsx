import type { WatchWithFriendsCreateRequest, WatchWithFriendsGroup } from '@porticomediaserver/client-core';
import { StatusLoadingIcon, ActionAddIcon, ActionRefreshIcon, AccountWatchTogetherIcon } from '#portico-icons';
import { useEffect, useState, type FormEvent } from 'react';
import { PrimaryButton, SecondaryButton } from '../../components/controls/Buttons';
import { MediaSearchPicker } from './MediaSearchPicker';
import { groupIncludesViewer, type WatchWithFriendsViewer } from './watchWithFriendsSource';

export function CreateGroupForm({
  initialMediaId,
  busy,
  onCancel,
  onCreate,
}: {
  initialMediaId: string;
  busy: boolean;
  onCancel: () => void;
  onCreate: (request: WatchWithFriendsCreateRequest) => Promise<void>;
}) {
  const [mediaId, setMediaId] = useState(initialMediaId);
  const [name, setName] = useState('');

  useEffect(() => setMediaId(initialMediaId), [initialMediaId]);

  const submit = (event: FormEvent) => {
    event.preventDefault();
    const normalizedMediaId = mediaId.trim();
    if (!normalizedMediaId) return;
    void onCreate({ mediaId: normalizedMediaId, ...(name.trim() ? { name: name.trim() } : {}) });
  };

  return <form className="watch-create-form" onSubmit={submit}>
    <div className="watch-create-fields">
      <MediaSearchPicker value={mediaId} onChange={setMediaId} autoFocus />
      <label>Group name<input value={name} onChange={(event) => setName(event.target.value)} placeholder="Optional" maxLength={80} /></label>
    </div>
    <div className="watch-create-actions">
      <SecondaryButton onClick={onCancel} disabled={busy}>Cancel</SecondaryButton>
      <PrimaryButton type="submit" disabled={busy || !mediaId.trim()}>{busy ? <StatusLoadingIcon className="watch-spin" /> : <ActionAddIcon />} Create group</PrimaryButton>
    </div>
  </form>;
}

export function GroupDirectory({
  groups,
  selectedId,
  viewer,
  busy,
  onSelect,
  onJoin,
  onRefresh,
}: {
  groups: WatchWithFriendsGroup[];
  selectedId: string;
  viewer: WatchWithFriendsViewer;
  busy: string;
  onSelect: (group: WatchWithFriendsGroup) => void;
  onJoin: (group: WatchWithFriendsGroup) => Promise<boolean>;
  onRefresh: () => void;
}) {
  return <aside className="watch-directory" aria-label="Active Watch With Friends groups">
    <header><div><h2>Active groups</h2><span>{groups.length} available</span></div><button type="button" onClick={onRefresh} aria-label="Refresh active groups" title="Refresh active groups"><ActionRefreshIcon /></button></header>
    {groups.length === 0
      ? <div className="watch-directory-empty"><AccountWatchTogetherIcon /><strong>No active groups</strong><span>Create one for a media item to begin.</span></div>
      : <div className="watch-group-list">{groups.map((group) => {
        const joined = groupIncludesViewer(group, viewer);
        return <article className={selectedId === group.id ? 'selected' : ''} key={group.id}>
          <button className="watch-group-select" type="button" onClick={() => onSelect(group)} aria-current={selectedId === group.id ? 'true' : undefined}>
            <span className={`watch-play-state ${group.state}`} aria-hidden="true" />
            <span><strong>{group.name}</strong><span>{group.mediaTitle}</span><span>{group.ownerName} · {group.members.length} watching</span></span>
          </button>
          {!joined && <button className="watch-join-small" type="button" disabled={busy === `join:${group.id}`} onClick={() => void onJoin(group)}>{busy === `join:${group.id}` ? 'Joining…' : 'Join'}</button>}
        </article>;
      })}</div>}
  </aside>;
}
