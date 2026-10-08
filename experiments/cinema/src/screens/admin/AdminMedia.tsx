import React, {useState} from 'react';
import {View} from 'react-native';
import type {AdminLibrary, LibraryChannel} from '../../data/admin';
import {Button} from '../../design/Button';
import {Badge, Chip, ListRow, Meter, Panel, Segmented, Stepper, TextField, ToggleRow} from '../../design/Controls';
import {ui} from '../../design/overlays';
import {RowsSkeleton} from '../../design/States';
import {Text} from '../../design/Text';
import {colors, u} from '../../design/tokens';
import {ago, KV, useAdmin} from './common';
import {plural} from '../../design/format';

const KINDS = [{id: 'Movies', help: 'Films, one per file or folder'}, {id: 'TV', help: 'Series organized by season'}, {id: 'Anime', help: 'Series matched with AniList naming and absolute episode numbers'}, {id: 'Music', help: 'Artists and albums'}, {id: 'Audiobooks', help: 'Books with chapters'}, {id: 'Photos', help: 'Pictures by album and date'}, {id: 'Home videos', help: 'Your own recordings, by date'}];

function FolderPicker({onPick}: {onPick: (p: string) => void}) {
  const [path, setPath] = useState('/Volumes/Media');
  const tree: Record<string, string[]> = {'/': ['Volumes', 'Users', 'srv'], '/Volumes': ['Media', 'Archive', 'Backup'], '/Volumes/Media': ['Movies', 'TV', 'Anime', 'Music', 'Audiobooks', 'Recordings', 'Documentaries'], '/Volumes/Archive': ['Classics', 'Concerts']};
  const children = tree[path] ?? [];
  return (
    <View>
      <TextField label="Folder path" value={path} onChangeText={setPath} />
      <ListRow icon="arrowUp" label="Up" onPress={() => setPath(path.split('/').slice(0, -1).join('/') || '/')} compact />
      {children.length ? children.map((c, i) => <ListRow key={c} icon="folder" label={c} compact hasTVPreferredFocus={i === 0} onPress={() => setPath(`${path === '/' ? '' : path}/${c}`)} />) : <Text variant="caption" tone="tertiary" style={{margin: 10}}>No folders inside this one.</Text>}
      <Button label="Use this folder" variant="primary" style={{marginTop: 10}} onPress={() => onPick(path)} />
    </View>
  );
}

export function openFolderPicker(onPick: (p: string) => void) {
  ui.sheet({title: 'Choose a folder', subtitle: 'On the server', render: close => <FolderPicker onPick={p => { onPick(p); close(); }} />});
}

export function LibrariesSection() {
  const {data, save, run} = useAdmin('libraries');
  if (!data) return <RowsSkeleton rows={1} />;
  const add = () => ui.sheet({title: 'Add a library', subtitle: 'Portico scans the folder and keeps it up to date.', render: close => <AddLibrary onDone={lib => { save([...data, lib]); close(); ui.toast('Library added. Scanning…'); }} />});
  const detail = (l: AdminLibrary) => ui.sheet({title: l.name, subtitle: `${l.kind} · ${plural(l.items, 'item', 'items')} · ${l.sizeTB} TB`, wide: true, render: close => (
    <View>
      <Panel title="Scan" lede="Scans find new, changed and missing files and keep metadata current.">
        <KV k="Last scan" v={ago(l.lastScan)} />
        {l.scanning ? <Meter value={l.scanProgress} style={{margin: u(14)}} /> : null}
        <View style={{flexDirection: 'row', gap: 8, margin: u(10), flexWrap: 'wrap'}}>
          {l.scanning ? <Button label="Cancel scan" size="sm" onPress={() => { run('cancelScan', {id: l.id}); close(); }} /> : <Button label="Scan now" icon="refresh" size="sm" variant="primary" hasTVPreferredFocus onPress={() => { run('scan', {id: l.id}); ui.toast('Scan started.'); close(); }} />}
          <Button label="Scan all files" size="sm" onPress={() => ui.toast('Full scan started. Every file is read again, changed or not.')} />
        </View>
      </Panel>
      <Panel title="Folders" lede="Where this library’s files live. Each folder is checked and scanned on its own.">
        {l.folders.map(f => <ListRow key={f.id} icon="folder" label={f.path} help={`${f.location} · depth ${f.depth} · ${f.available ? 'Available' : 'Unavailable'}`} onPress={() => ui.menu({title: `Folder ${f.path}`, actions: [{label: 'Scan this folder', icon: 'refresh', onPress: () => ui.toast('Scan started.')}, {label: 'Check availability', icon: 'checkCircle', onPress: () => ui.toast('Folder is available.')}, {label: 'Edit folder…', icon: 'edit', onPress: () => ui.toast('Network folders are checked more patiently before files are marked missing.')}, {label: 'Remove folder', icon: 'trash', destructive: true, onPress: () => ui.confirm({title: `Remove folder “${f.path}”?`, body: 'Titles found only in this folder become unavailable. Files on disk are not deleted.', confirmLabel: 'Remove folder', destructive: true, onConfirm: () => save(data.map(x => (x.id === l.id ? {...x, folders: x.folders.filter(y => y.id !== f.id)} : x)))})}]})} />)}
        <Button label="Add folder" icon="plus" size="sm" style={{margin: u(10), alignSelf: 'flex-start'}} onPress={() => openFolderPicker(p => save(data.map(x => (x.id === l.id ? {...x, folders: [...x.folders, {id: l.id + p, path: p, location: 'Local disk', available: true, depth: 6}]} : x))))} />
      </Panel>
      {l.attention.length ? (
        <Panel title={`${plural(l.attention.length, 'file', 'files')} ${l.attention.length === 1 ? 'needs' : 'need'} attention`} lede="Portico couldn’t tell which show, season or episode these files are. Rename them, then scan again.">
          {l.attention.slice(0, 6).map(a => <ListRow key={a.path} icon="alertCircle" label={a.path.split('/').pop() ?? a.path} help={a.reason} compact />)}
        </Panel>
      ) : null}
      <View style={{flexDirection: 'row', gap: 10}}>
        <Button label="Rename" icon="edit" size="sm" onPress={() => ui.prompt({title: 'Rename library', fields: [{key: 'n', label: 'Name', initial: l.name}], submitLabel: 'Save', onSubmit: v => { save(data.map(x => (x.id === l.id ? {...x, name: v.n} : x))); return null; }})} />
        <Button label="Library settings" icon="settings" size="sm" onPress={() => { close(); ui.toast('Open Library settings in the console sidebar.'); }} />
      </View>
    </View>
  )});
  return (
    <View>
      <Button label="Add library" icon="plus" variant="primary" onPress={add} style={{alignSelf: 'flex-start', marginBottom: u(12)}} hasTVPreferredFocus />
      <Panel>
        {data.map(l => (
          <ListRow key={l.id} icon={l.scanning ? 'refresh' : 'library'} label={l.name} help={`${l.kind} · ${l.folders.map(f => f.path).join(', ')} · ${plural(l.items, 'item', 'items')} · ${l.sizeTB} TB · scanned ${ago(l.lastScan)}`} onPress={() => detail(l)}
            right={l.scanning ? <Badge label={`Scanning ${Math.round(l.scanProgress * 100)}%`} /> : l.attention.length ? <Badge label={`${l.attention.length} need attention`} tone="amber" /> : <Badge label={l.metadata === 'online' ? 'Online metadata' : 'Local metadata'} tone="neutral" />} />
        ))}
      </Panel>
    </View>
  );
}

function AddLibrary({onDone}: {onDone: (l: AdminLibrary) => void}) {
  const [kind, setKind] = useState('Movies');
  const [name, setName] = useState('Documentaries');
  const [folder, setFolder] = useState('/Volumes/Media/Documentaries');
  const [source, setSource] = useState<'online' | 'local'>('online');
  return (
    <View>
      <Text variant="captionStrong" tone="secondary">Kind · decides how titles are matched and presented</Text>
      {KINDS.map((k, i) => <ListRow key={k.id} label={k.id} help={k.help} compact chevron={false} hasTVPreferredFocus={i === 0} onPress={() => setKind(k.id)} right={kind === k.id ? <Badge label="Selected" /> : undefined} />)}
      <TextField label="Name" value={name} onChangeText={setName} autoCapitalize="words" style={{marginTop: 10}} />
      <ListRow icon="folder" label="Folder on the server" help={folder} onPress={() => openFolderPicker(setFolder)} />
      <Text variant="captionStrong" tone="secondary" style={{marginTop: 10}}>Metadata</Text>
      <ListRow label="Portico online metadata" help="Looks up titles, artwork, cast and ratings online. Your own files and NFO details are used first." chevron={false} onPress={() => setSource('online')} right={source === 'online' ? <Badge label="Selected" /> : undefined} />
      <ListRow label="Local metadata only" help="Uses only file and folder names, embedded tags, NFO and other sidecar files. Portico never looks anything up online for this library." chevron={false} onPress={() => setSource('local')} right={source === 'local' ? <Badge label="Selected" /> : undefined} />
      <Text variant="caption" tone="tertiary">You can change this later in the library’s settings.</Text>
      <Button label="Add library" variant="primary" style={{marginTop: 12}} onPress={() => onDone({id: 'lib-new-' + Date.now(), name, kind, items: 0, sizeTB: 0, lastScan: Date.now(), scanning: true, scanProgress: 0.02, folders: [{id: 'f', path: folder, location: 'Local disk', available: true, depth: 6}], attention: [], metadata: source, language: 'en', country: 'US', rescan: 'Every hour', missingAfter: '1 week', trickplayEvery: 10, trickplayWidth: 320, chapterPictures: true, allowDelete: false, keepDeletedDays: 30, fetchLyrics: false})} />
    </View>
  );
}

export function LibrarySettingsSection() {
  const {data, save} = useAdmin('libraries');
  const [id, setId] = useState<string | null>(null);
  if (!data) return <RowsSkeleton rows={1} />;
  const l = data.find(x => x.id === (id ?? data[0].id))!;
  const patch = (p: Partial<AdminLibrary>) => save(data.map(x => (x.id === l.id ? {...x, ...p} : x)));
  return (
    <View>
      <View style={{flexDirection: 'row', flexWrap: 'wrap', gap: 8, marginBottom: u(12)}}>{data.map((x, i) => <Chip key={x.id} label={x.name} active={x.id === l.id} onPress={() => setId(x.id)} hasTVPreferredFocus={i === 0} />)}</View>
      <Text variant="title" style={{marginBottom: 8}}>{l.name} settings</Text>
      <Panel title="Matching" lede="Where Portico looks up titles, artwork and descriptions for this library.">
        <ListRow label="Source" value={l.metadata === 'online' ? 'Portico online metadata' : 'Local metadata only'} onPress={() => ui.confirm(l.metadata === 'online' ? {title: 'Stop looking this library up online?', body: 'Titles and artwork already found stay until you refresh them. New titles will use only your files.', confirmLabel: 'Use local metadata only', onConfirm: () => patch({metadata: 'local'})} : {title: 'Look this library up online?', body: 'Portico will match titles with online providers in the background.', confirmLabel: 'Look up online', onConfirm: () => patch({metadata: 'online'})})} />
        <ListRow label="Metadata language" help="Titles, summaries and artwork text are fetched in this language when available." value={l.language} onPress={() => ui.menu({title: 'Metadata language', actions: ['en', 'fr', 'de', 'es', 'ja'].map(x => ({label: x, checked: l.language === x, onPress: () => patch({language: x})}))})} />
        <ListRow label="Country or region" value={l.country} onPress={() => ui.menu({title: 'Country', actions: ['US', 'GB', 'CA', 'DE', 'JP'].map(x => ({label: x, checked: l.country === x, onPress: () => patch({country: x})}))})} />
        {l.kind === 'Music' ? <ToggleRow label="Fetch missing lyrics" value={l.fetchLyrics} onChange={v => patch({fetchLyrics: v})} /> : null}
      </Panel>
      <Panel title="Scrubbing and chapters" lede="Pictures made for the seek bar and the chapter list.">
        <Stepper label="A picture every" help="Seconds between seek-bar pictures. Smaller is smoother and uses more disk." value={l.trickplayEvery} onChange={v => patch({trickplayEvery: v})} min={2} max={60} unit="s" />
        <Stepper label="Picture width" help="In pixels." value={l.trickplayWidth} onChange={v => patch({trickplayWidth: v})} min={160} max={640} step={40} />
        <ToggleRow label="Chapter pictures" value={l.chapterPictures} onChange={v => patch({chapterPictures: v})} />
        <ToggleRow label="Hover previews" help="Video previews are not available on this server." value={false} onChange={() => {}} disabled />
      </Panel>
      <Panel title="Scanning">
        <ListRow label="Rescan interval" value={l.rescan} onPress={() => ui.menu({title: 'Rescan interval', actions: ['Every 15 minutes', 'Every hour', 'Every 6 hours', 'Daily', 'Only when I ask'].map(x => ({label: x, checked: l.rescan === x, onPress: () => patch({rescan: x})}))})} />
        <ListRow label="Mark missing files after" help="Gives unplugged drives and offline shares time to come back." value={l.missingAfter} onPress={() => ui.menu({title: 'Mark missing files after', actions: ['Immediately', '1 day', '1 week'].map(x => ({label: x, checked: l.missingAfter === x, onPress: () => patch({missingAfter: x})}))})} />
      </Panel>
      <Panel title="Deleting" lede="Off by default: nothing in Portico can remove a file from this library until you allow it here.">
        <ToggleRow label="Allow deleting files" value={l.allowDelete} onChange={v => patch({allowDelete: v})} />
        <Stepper label="Keep deleted files for" help="Days before a deleted title is removed for good. Zero removes at once." value={l.keepDeletedDays} onChange={v => patch({keepDeletedDays: v})} min={0} max={90} unit="days" />
      </Panel>
    </View>
  );
}

export function MetadataSection() {
  const {data, save} = useAdmin('metadata');
  if (!data) return <RowsSkeleton rows={1} />;
  return (
    <View>
      <Panel title="Online lookups" right={<Badge label={data.consent === 'granted' ? 'On' : data.consent === 'waiting' ? 'Waiting' : 'Off'} tone={data.consent === 'granted' ? 'healthy' : 'amber'} />}>
        <Text variant="body" tone="secondary" style={{marginHorizontal: u(14), marginBottom: 8}}>{data.consent === 'granted' ? 'Titles, artwork and details are looked up online (TMDB, TheTVDB, AniList, MusicBrainz).' : 'Not matching yet: online lookups are waiting for your OK.'}</Text>
        <ToggleRow label="Allow online lookups" value={data.consent === 'granted'} onChange={v => save({consent: v ? 'granted' : 'declined'})} />
      </Panel>
      <Panel title="Providers">
        {data.providers.map((p, i) => <ToggleRow key={p.id} label={p.name} help={`${p.kinds}${p.key ? ' · Your own key is set' : ''}`} value={p.enabled} onChange={v => save({providers: data.providers.map(x => (x.id === p.id ? {...x, enabled: v} : x))})} />)}
      </Panel>
      <Panel title="Defaults">
        <ListRow label="Language" value={data.language} onPress={() => ui.menu({title: 'Language', actions: ['en', 'fr', 'de', 'es'].map(x => ({label: x, checked: data.language === x, onPress: () => save({language: x})}))})} />
        <ListRow label="Country or region" value={data.country} onPress={() => ui.menu({title: 'Country', actions: ['US', 'GB', 'CA'].map(x => ({label: x, checked: data.country === x, onPress: () => save({country: x})}))})} />
        <Stepper label="Refresh details every" value={data.refreshDays} onChange={v => save({refreshDays: v})} min={7} max={180} step={7} unit="days" />
        <ToggleRow label="Fetch lyrics" value={data.lyrics} onChange={v => save({lyrics: v})} />
      </Panel>
    </View>
  );
}

export function StorageSection() {
  const {data, save} = useAdmin('storage');
  if (!data) return <RowsSkeleton rows={1} />;
  const add = (kind: 'webdav' | 'rclone') => ui.prompt({
    title: kind === 'webdav' ? 'Add a WebDAV share' : 'Add an rclone remote',
    body: kind === 'webdav' ? 'Nextcloud, ownCloud… The password is stored on the server and never shown again.' : 'Portico runs your rclone binary in a private sandbox and mounts the remote for libraries to use.',
    fields: kind === 'webdav' ? [{key: 'n', label: 'Name'}, {key: 'a', label: 'Address', placeholder: 'https://'}, {key: 'u', label: 'Username'}, {key: 'p', label: 'Password', secure: true}] : [{key: 'n', label: 'Name'}, {key: 'a', label: 'Remote', help: 'The remote name from your rclone config, with an optional path.'}, {key: 'c', label: 'rclone configuration', multiline: true, help: 'Pasted once and stored on the server. Secrets inside are never shown again.'}],
    submitLabel: kind === 'webdav' ? 'Add share' : 'Add remote',
    onSubmit: v => { if (!v.n || !v.a) return 'Enter a name and an address.'; save([...data, {id: 'st' + Date.now(), kind, name: v.n, address: v.a, state: 'mounted', usedTB: 0, libraries: []}]); return null; },
  });
  return (
    <View>
      <View style={{flexDirection: 'row', gap: 10, marginBottom: 12}}>
        <Button label="WebDAV share" icon="plus" onPress={() => add('webdav')} hasTVPreferredFocus />
        <Button label="rclone remote" icon="plus" onPress={() => add('rclone')} />
      </View>
      <Panel title="Remote sources" lede="Library folders on remote storage are added under Libraries once the source is mounted and healthy.">
        {data.map(m => (
          <ListRow key={m.id} icon={m.kind === 'webdav' ? 'cloud' : 'hardDrive'} label={m.name} help={`${m.kind === 'webdav' ? 'WebDAV' : 'rclone'} · ${m.address} · ${m.usedTB} TB${m.libraries.length ? ' · used by ' + m.libraries.join(', ') : ''}`}
            right={<Badge label={m.state} tone={m.state === 'mounted' ? 'healthy' : m.state === 'error' ? 'danger' : 'neutral'} />}
            onPress={() => ui.menu({title: m.name, actions: [
              {label: m.state === 'mounted' ? 'Unmount' : 'Mount', icon: 'plug', onPress: () => save(data.map(x => (x.id === m.id ? {...x, state: x.state === 'mounted' ? 'unmounted' : 'mounted'} : x)))},
              {label: 'Restart', icon: 'refresh', onPress: () => ui.toast('Restarting the mount…')},
              {label: 'Edit connection', icon: 'edit', onPress: () => ui.toast('Leave the password empty to keep the saved one.')},
              {label: 'Remove', icon: 'trash', destructive: true, onPress: () => ui.confirm({title: `Remove “${m.name}”?`, body: 'The mount is stopped and its saved configuration removed. Libraries using it lose access to those files; nothing on the remote is deleted.', confirmLabel: 'Remove', destructive: true, onConfirm: () => save(data.filter(x => x.id !== m.id))})},
            ]})} />
        ))}
      </Panel>
    </View>
  );
}

export function LiveTVSection() {
  const {data, save} = useAdmin('liveTv');
  if (!data) return <RowsSkeleton rows={1} />;
  const add = () => ui.menu({title: 'Add a live TV source', subtitle: 'Portico checks the source first and shows what it found before saving.', actions: [
    {label: 'Playlist URL or tuner', icon: 'link', detail: 'M3U, Xtream, HDHomeRun', onPress: () => ui.prompt({title: 'Playlist URL or tuner', fields: [{key: 'u', label: 'Playlist URL', placeholder: 'https://'}, {key: 'g', label: 'Guide (XMLTV) URL'}], submitLabel: 'Check source', onSubmit: v => { save({sources: [...data.sources, {id: 's' + Date.now(), name: 'New playlist', kind: 'M3U', address: v.u || 'https://example.net/list.m3u', channels: 36, tuners: 2, paused: false, lastRefresh: Date.now(), guideFrom: null, numbering: 'inOrder', startAt: 200, logos: true, health: 'ok'}]}); ui.toast('36 channels · 2,410 programs found. Source added.'); return null; }})},
    {label: 'Find tuners on the network', icon: 'search', detail: 'HDHomeRun', onPress: () => ui.sheet({title: 'Find tuners on your network', subtitle: 'Looks for HDHomeRun and similar tuners near the server. It takes about five seconds.', render: close => <View><ListRow icon="tv" label="HDHomeRun FLEX 4K · 192.168.1.44" help="4 tuners · Already added" /><ListRow icon="tv" label="HDHomeRun CONNECT · 192.168.1.61" help="2 tuners" onPress={() => { ui.toast('Tuner added.'); close(); }} hasTVPreferredFocus /></View>})},
    {label: 'Upload playlist files', icon: 'upload', detail: 'M3U + XMLTV', onPress: () => ui.toast('Choose a .m3u playlist and an optional .xml guide.')},
  ]});
  return (
    <View>
      <Button label="Add source" icon="plus" variant="primary" onPress={add} style={{alignSelf: 'flex-start', marginBottom: 12}} hasTVPreferredFocus />
      <Panel title="Sources">
        {data.sources.map(src => (
          <ListRow key={src.id} icon={src.kind === 'HDHomeRun' ? 'live' : 'list'} label={src.name} help={`${src.kind} · ${src.address} · ${plural(src.channels, 'channel', 'channels')} · ${plural(src.tuners, 'tuner', 'tuners')} · refreshed ${ago(src.lastRefresh)}${src.guideFrom ? ' · guide from XMLTV listings' : ''}`}
            right={<Badge label={src.paused ? 'Paused' : src.health === 'ok' ? 'Healthy' : 'Check'} tone={src.paused ? 'neutral' : src.health === 'ok' ? 'healthy' : 'amber'} />}
            onPress={() => ui.sheet({title: src.name, subtitle: 'Settings for this source', render: close => (
              <View>
                <ListRow label="Guide from" help="Use another source’s XMLTV listings for these channels" value={src.guideFrom ? 'XMLTV listings' : 'This source’s own guide'} />
                <ListRow label="Channel numbering" value={src.numbering === 'asSent' ? 'As the source sends them' : `Number them in order from ${src.startAt}`} onPress={() => save({sources: data.sources.map(x => (x.id === src.id ? {...x, numbering: x.numbering === 'asSent' ? 'inOrder' : 'asSent'} : x))})} />
                <ToggleRow label="Bring in the logos this source publishes" value={src.logos} onChange={v => save({sources: data.sources.map(x => (x.id === src.id ? {...x, logos: v} : x))})} />
                <ListRow label="Channels and numbers" help="Give a channel its own number, hide it, or match it to a guide listing" onPress={() => ui.toast('Save 3 changes')} />
                <View style={{flexDirection: 'row', flexWrap: 'wrap', gap: 8, marginTop: 10}}>
                  <Button label="Refresh guide now" size="sm" icon="refresh" hasTVPreferredFocus onPress={() => ui.toast('Refresh started.')} />
                  <Button label={src.paused ? 'Enable' : 'Pause'} size="sm" onPress={() => { save({sources: data.sources.map(x => (x.id === src.id ? {...x, paused: !x.paused} : x))}); close(); }} />
                  <Button label="Remove source" size="sm" variant="danger" onPress={() => ui.confirm({title: `Remove “${src.name}”?`, body: 'Its channels leave the guide and any saved credentials are deleted. Recordings already made are kept.', confirmLabel: 'Remove source', destructive: true, onConfirm: () => { save({sources: data.sources.filter(x => x.id !== src.id)}); close(); }})} />
                </View>
              </View>
            )})} />
        ))}
      </Panel>
      <Panel title="Recordings" lede="Where recordings live and how much space they may use.">
        <KV k="Folder" v={data.recordingsPath} />
        <KV k="Space" v={`${data.usedGB} GB used of ${data.totalGB} GB`} />
        <Meter value={data.usedGB / data.totalGB} style={{margin: u(14)}} />
        <Stepper label="Keep recordings for" help="0 keeps them until you delete them." value={data.keepDays} onChange={v => save({keepDays: v})} min={0} max={365} unit="days" />
        <Stepper label="Episodes to keep per series" help="0 keeps every episode." value={data.episodesPerSeries} onChange={v => save({episodesPerSeries: v})} min={0} max={50} />
        <Stepper label="Always leave free" help="Recording stops before the volume drops below this." value={data.leaveFreeGB} onChange={v => save({leaveFreeGB: v})} min={0} max={1000} step={25} unit="GB" />
        <Stepper label="Recordings may use up to" help="0 means no cap beyond the free-space floor." value={data.capGB} onChange={v => save({capGB: v})} min={0} max={8000} step={250} unit="GB" />
      </Panel>
    </View>
  );
}

export function DVRSection() {
  const {data, save} = useAdmin('dvr');
  if (!data) return <RowsSkeleton rows={1} />;
  return (
    <View>
      <Panel title="Recording defaults" lede="What a new recording does unless you say otherwise.">
        <Stepper label="Start early" help="Minutes before the listed start." value={data.startEarly} onChange={v => save({startEarly: v})} min={0} max={30} unit="min" />
        <Stepper label="Finish late" help="Minutes after the listed end. Live sport often overruns." value={data.endLate} onChange={v => save({endLate: v})} min={0} max={120} step={5} unit="min" />
        <View style={{padding: u(10)}}><Segmented value={data.keepMode} onChange={v => save({keepMode: v})} options={[{value: 'newest', label: 'The newest'}, {value: 'days', label: 'For a number of days'}, {value: 'all', label: 'Keep all'}]} /></View>
        {data.keepMode !== 'all' ? <Stepper label={data.keepMode === 'days' ? 'How many days' : 'How many'} value={data.keepCount} onChange={v => save({keepCount: v})} min={1} max={60} /> : null}
        <ListRow label="After recording" value={{none: 'Leave as recorded', remux: 'Repackage', convert: 'Convert to a smaller file'}[data.afterRecording]} help={{none: 'Kept exactly as broadcast.', remux: 'Repackaged so it seeks and plays everywhere; picture and sound untouched.', convert: 'Converted to a smaller file. This takes server time.'}[data.afterRecording]} onPress={() => ui.menu({title: 'After recording', actions: (['none', 'remux', 'convert'] as const).map(x => ({label: {none: 'Leave as recorded', remux: 'Repackage', convert: 'Convert to a smaller file'}[x], checked: data.afterRecording === x, onPress: () => save({afterRecording: x})}))})} />
        <ToggleRow label="Remove the original afterwards" value={data.removeOriginal} onChange={v => save({removeOriginal: v})} />
        <TextField label="Folder for shows" value={data.folderShows} onChangeText={v => save({folderShows: v})} help="Pieces you can use: {show} {season} {episode} {title} {year} {channel}" style={{marginHorizontal: u(14)}} />
        <TextField label="Folder for movies" value={data.folderMovies} onChangeText={v => save({folderMovies: v})} style={{marginHorizontal: u(14)}} />
        <ToggleRow label="Save a details file alongside each recording" value={data.sidecarNfo} onChange={v => save({sidecarNfo: v})} />
        <ToggleRow label="Save a background picture alongside" value={data.sidecarFanart} onChange={v => save({sidecarFanart: v})} />
      </Panel>
      <Panel title="Tuners" lede="In use now and over the next 4 hours.">
        {data.tuners.map(t => <ListRow key={t.id} icon="live" label={t.name} help={`${t.source}${t.next ? ' · next ' + t.next : ''}`} right={<Badge label={t.inUse ? t.inUse : 'Free'} tone={t.inUse ? 'record' : 'healthy'} />} />)}
      </Panel>
      <Panel title="Live TV defaults" lede="How the server tunes and keeps a stream going.">
        <Stepper label="Buffer" help="Seconds held before playing. More rides out a shaky source; less is closer to live." value={data.buffer} onChange={v => save({buffer: v})} min={0} max={30} unit="s" />
        <Stepper label="Keep retrying for" help="Seconds to keep trying a source that drops before giving up." value={data.retry} onChange={v => save({retry: v})} min={5} max={120} step={5} unit="s" />
        <Stepper label="Guide days" help="How far ahead the guide is kept." value={data.guideDays} onChange={v => save({guideDays: v})} min={1} max={21} />
        <ToggleRow label="Fetch channel logos" value={data.logos} onChange={v => save({logos: v})} />
        <ToggleRow label="Look for tuners on the network" help="Finds HDHomeRun devices by themselves." value={data.discovery} onChange={v => save({discovery: v})} />
      </Panel>
      <Panel title="Who can record" lede="Choose which profiles may schedule recordings. A setting on a profile takes priority over its account’s setting.">
        {data.recorders.map(r => <ToggleRow key={r.profile} label={`${r.profile} (${r.account})`} help={r.inherit ? 'Other profiles on this account inherit this' : undefined} value={r.allowed} onChange={v => save({recorders: data.recorders.map(x => (x.profile === r.profile ? {...x, allowed: v} : x))})} />)}
      </Panel>
      <Panel title="Recording rules" lede="Record a series from the guide for a permitted profile. A rule’s own options replace the recording defaults.">
        {data.rules.map(r => <ListRow key={r.id} icon="layers" label={r.name} help={`${r.owner} · ${r.channel} · ${r.newOnly ? 'new episodes only' : 'all episodes'} · priority ${r.priority}`} right={<Badge label={r.paused ? 'Paused' : 'On'} tone={r.paused ? 'neutral' : 'healthy'} />} onPress={() => ui.menu({title: r.name, actions: [{label: r.paused ? 'Resume' : 'Pause', icon: r.paused ? 'play' : 'pause', onPress: () => save({rules: data.rules.map(x => (x.id === r.id ? {...x, paused: !x.paused} : x))})}, {label: 'Remove rule', icon: 'trash', destructive: true, onPress: () => ui.confirm({title: `Remove “${r.name}”?`, body: 'It stops scheduling new recordings. Recordings it already scheduled or made are kept.', confirmLabel: 'Remove rule', destructive: true, onConfirm: () => save({rules: data.rules.filter(x => x.id !== r.id)})})}]})} />)}
        <Button label="New rule" icon="plus" size="sm" style={{margin: u(10), alignSelf: 'flex-start'}} onPress={() => ui.prompt({title: 'New recording rule', fields: [{key: 'n', label: 'Rule name'}, {key: 'o', label: 'Record for', initial: 'Justin'}], submitLabel: 'Create', onSubmit: v => { save({rules: [...data.rules, {id: 'r' + Date.now(), name: v.n || 'New rule', owner: v.o, channel: 'Any channel', newOnly: true, paused: false, priority: 1}]}); return null; }})} />
      </Panel>
    </View>
  );
}

export function ChannelsSection() {
  const {data, save} = useAdmin('channels');
  if (!data) return <RowsSkeleton rows={1} />;
  const open = (c: LibraryChannel) => ui.sheet({title: `${c.number} ${c.name}`, subtitle: c.description, render: close => (
    <View>
      <Panel title="Channel health" right={<Badge label={c.health === 'upToDate' ? 'Up to date' : c.health === 'beingMade' ? 'Being made now' : 'Not generated yet'} tone={c.health === 'upToDate' ? 'healthy' : 'amber'} />}>
        <KV k="Status" v={c.on ? 'This channel is on the air.' : 'This channel isn’t playing.'} />
        <KV k="Schedule reaches" v={`${plural(c.scheduleHours, 'hour', 'hours')} ahead`} />
        <KV k="Titles it can choose from" v={String(c.candidates)} />
        <KV k="Titles it could not place" v={String(c.unresolved)} tone={c.unresolved ? 'amber' : undefined} />
      </Panel>
      <View style={{flexDirection: 'row', flexWrap: 'wrap', gap: 8}}>
        <Button label={c.on ? 'Turn off' : 'Turn on'} size="sm" hasTVPreferredFocus onPress={() => { save(data.map(x => (x.id === c.id ? {...x, on: !x.on} : x))); close(); }} />
        <Button label="Regenerate schedule" size="sm" icon="refresh" onPress={() => ui.toast('New schedule being made.')} />
        <Button label="Check health" size="sm" onPress={() => ui.toast('Channel is healthy.')} />
        <Button label="Upload a logo…" size="sm" icon="upload" onPress={() => ui.toast('Use a JPEG, PNG or WebP between 16 and 4096 pixels.')} />
        <Button label="Delete channel" size="sm" variant="danger" onPress={() => ui.confirm({title: `Delete “${c.name}”?`, body: 'The channel disappears from Live TV for everyone. Your media is not affected.', confirmLabel: 'Delete channel', destructive: true, onConfirm: () => { save(data.filter(x => x.id !== c.id)); close(); }})} />
      </View>
    </View>
  )});
  return (
    <View>
      <Panel title="Templates" lede="Start from a template, or from scratch in Channel Builder.">
        <View style={{flexDirection: 'row', flexWrap: 'wrap', gap: 8, padding: u(10)}}>
          {['Saturday Matinee', 'Midnight Horror', 'Sitcom Marathon', 'Nature & Science', 'Decade: The 80s', 'Start from scratch'].map((t, i) => <Chip key={t} label={t} hasTVPreferredFocus={i === 0} onPress={() => ui.toast(`Review “${t}” before creating it in Channel Builder.`)} />)}
        </View>
      </Panel>
      <Panel title="Channels">
        {data.map(c => <ListRow key={c.id} icon="tv" label={`${c.number} ${c.name}`} help={`${c.libraries.join(', ')} · ${c.order} · ${c.conditions.map(x => `${x.field} ${x.op} ${x.value}`).join(', ')}`} right={<Badge label={c.on ? 'On air' : 'Off'} tone={c.on ? 'healthy' : 'neutral'} />} onPress={() => open(c)} />)}
      </Panel>
    </View>
  );
}

export function ChannelBuilderSection() {
  const [name, setName] = useState('Sunday Noir');
  const [desc, setDesc] = useState('Shadows, rain and bad decisions, all day on Sunday.');
  const [on, setOn] = useState(true);
  const [kinds, setKinds] = useState({movies: true, shows: false});
  const [libs, setLibs] = useState(['Movies']);
  const [mode, setMode] = useState<'all' | 'any'>('all');
  const [conds, setConds] = useState([{field: 'Genre', op: 'is', value: 'Crime'}, {field: 'Year', op: 'before', value: '1970'}]);
  const [order, setOrder] = useState('Shuffle, no repeats');
  const [advanced, setAdvanced] = useState(false);
  const [repeats, setRepeats] = useState(12);
  return (
    <View>
      <Panel title="New channel">
        <TextField label="Name" value={name} onChangeText={setName} autoCapitalize="words" style={{marginHorizontal: u(14)}} />
        <TextField label="Description" value={desc} onChangeText={setDesc} style={{marginHorizontal: u(14)}} autoCapitalize="sentences" />
        <ToggleRow label="Channel is on" help="Everyone with access to these libraries sees this channel." value={on} onChange={setOn} />
      </Panel>
      <Panel title="What plays">
        <Text variant="captionStrong" tone="secondary" style={{marginLeft: u(14)}}>From these libraries</Text>
        <View style={{flexDirection: 'row', gap: 8, padding: u(10)}}>{['Movies', 'TV Shows', 'Anime'].map((l, i) => <Chip key={l} label={l} active={libs.includes(l)} hasTVPreferredFocus={i === 0} onPress={() => setLibs(libs.includes(l) ? libs.filter(x => x !== l) : [...libs, l])} />)}</View>
        <ToggleRow label="Movies" value={kinds.movies} onChange={v => setKinds({...kinds, movies: v})} />
        <ToggleRow label="TV shows" help="For TV shows, conditions apply to the show (its genre, network, rating) and every episode plays." value={kinds.shows} onChange={v => setKinds({...kinds, shows: v})} />
        <Text variant="captionStrong" tone="secondary" style={{marginLeft: u(14), marginTop: 10}}>Only titles that match</Text>
        <View style={{padding: u(10)}}><Segmented value={mode} onChange={setMode} options={[{value: 'all', label: 'all of these'}, {value: 'any', label: 'any of these'}]} /></View>
        {conds.map((c, i) => <ListRow key={i} label={`${c.field} ${c.op} ${c.value}`} compact onPress={() => ui.menu({title: 'Condition', actions: [{label: 'Remove', icon: 'x', destructive: true, onPress: () => setConds(conds.filter((_, k) => k !== i))}]})} />)}
        <View style={{flexDirection: 'row', gap: 8, padding: u(10)}}>
          <Button label="Add a condition" icon="plus" size="sm" onPress={() => ui.menu({title: 'Add a condition', actions: ['Genre is Horror', 'Mood is Eerie', 'Rating is PG-13', 'Added in the last 30 days', 'Title or summary mentions “ship”'].map(x => ({label: x, onPress: () => setConds([...conds, {field: x.split(' ')[0], op: x.split(' ').slice(1, -1).join(' '), value: x.split(' ').slice(-1)[0]}])}))})} />
          <Button label="Add a group" size="sm" variant="ghost" onPress={() => ui.toast('Group: any of these')} />
        </View>
        <ListRow label="Always include" value="Detour, The Hitch-Hiker" onPress={() => ui.toast('Find a movie or show')} />
        <ListRow label="Never include" value="None" onPress={() => ui.toast('Find a movie or show')} />
      </Panel>
      <Panel title="Play order">
        <ListRow label="Order" value={order} onPress={() => ui.menu({title: 'Play order', actions: ['Shuffle, no repeats', 'Random (repeats allowed)', 'In order', 'In order, then shuffled'].map(o => ({label: o, checked: order === o, onPress: () => setOrder(o)}))})} />
        <ListRow label="Episodes" value="Mixed with everything else" onPress={() => ui.menu({title: 'Episodes', actions: ['Mixed with everything else', 'Each show in episode order', 'Marathon: one show at a time', 'One episode of each show in turn', 'Random episodes'].map(o => ({label: o, onPress: () => {}}))})} />
        <Button label={advanced ? 'Hide advanced' : 'Advanced'} size="sm" variant="ghost" onPress={() => setAdvanced(!advanced)} style={{margin: u(10), alignSelf: 'flex-start'}} />
        {advanced ? (
          <>
            <ListRow label="When everything has played" value="Start again" />
            <Stepper label="Avoid repeats within" help="Programs" value={repeats} onChange={setRepeats} min={0} max={50} />
            <ListRow label="Quality" value="Automatic" />
            <ListRow label="Time zone" value="America/Los_Angeles" />
          </>
        ) : null}
      </Panel>
      <Panel title="Preview" right={<Badge label="18 titles match" tone="healthy" />}>
        <Text variant="body" tone="secondary" style={{marginHorizontal: u(14)}}>About 27 hours of programming. Some of what plays: Detour, D.O.A., The Hitch-Hiker, Scarlet Street, Kansas City Confidential.</Text>
        <Text variant="captionStrong" tone="secondary" style={{margin: u(14), marginBottom: 4}}>Today’s schedule</Text>
        {['6:00 PM · Detour', '7:10 PM · The Hitch-Hiker', '8:25 PM · Night Shift at Pier 9', '10:05 PM · Charade'].map(l => <Text key={l} variant="caption" tone="tertiary" style={{marginHorizontal: u(14)}}>{l}</Text>)}
      </Panel>
      <View style={{flexDirection: 'row', gap: 10}}>
        <Button label="Cancel" />
        <Button label="Create channel" variant="primary" onPress={() => ui.toast(`${name} created. Its schedule is being made.`)} />
      </View>
    </View>
  );
}

export {colors};
