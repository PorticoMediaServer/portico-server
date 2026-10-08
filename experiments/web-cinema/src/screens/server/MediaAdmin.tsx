import {useState} from 'react';
import {useNavigate, useSearch} from '@tanstack/react-router';
import {useServices} from '../../data/hooks';
import type {AdminLibrary, StorageMount} from '../../data/admin';
import {Panel, Row} from '../../ui/Layout';
import {Button} from '../../ui/Button';
import {ChoiceCards, Field, Input, PasswordInput, Select, Stepper, Switch} from '../../ui/Controls';
import {Badge, Notice, Progress, toast} from '../../ui/Feedback';
import {Icon, type IconName} from '../../ui/Icon';
import {ConfirmDialog, Dialog, DialogClose, Menu, PromptDialog} from '../../ui/Overlay';
import {cx} from '../../ui/cx';
import {relativeTime} from '../shared/items';
import {AdminSkeleton, SectionHead, StatusBadge, UnsavedBar, useAdmin, useAdminForm} from './common';
import type {Sec} from './Server';

export function MediaSection({id, sec}: {id: string; sec: Sec}) {
  switch (id) {
    case 'library-settings': return <LibrarySettings sec={sec} />;
    case 'metadata': return <Metadata sec={sec} />;
    case 'storage': return <Storage sec={sec} />;
    default: return <Libraries sec={sec} />;
  }
}

const KIND_ICON: Record<string, IconName> = {Movies: 'film', TV: 'tv', Anime: 'sparkles', Music: 'music', Audiobooks: 'bookOpen', Photos: 'photos', 'Home videos': 'video'};

function Libraries({sec}: {sec: Sec}) {
  const services = useServices();
  const navigate = useNavigate();
  const q = useAdmin('libraries');
  const [adding, setAdding] = useState(false);
  const [renaming, setRenaming] = useState<AdminLibrary | null>(null);
  const [removing, setRemoving] = useState<AdminLibrary | null>(null);
  const [attention, setAttention] = useState<AdminLibrary | null>(null);
  if (!q.data) return <AdminSkeleton />;
  const set = (list: AdminLibrary[]) => services.admin.set('libraries', list);
  return (
    <div className="adm-sections">
      <SectionHead title={sec.label} lede={sec.lede} actions={<>
        <Button variant="secondary" icon="refresh" label="Scan all" onClick={() => void Promise.all(q.data!.map(l => services.admin.run('scan', {id: l.id}))).then(() => toast('Scanning every library'))} />
        <Button variant="primary" icon="plus" label="Add library" onClick={() => setAdding(true)} />
      </>} />
      <div className="lib-cards">
        {q.data.map(l => (
          <div key={l.id} className="lib-card">
            <div className="lib-card-head">
              <span className="set-icon"><Icon name={KIND_ICON[l.kind] ?? 'library'} size={22} /></span>
              <div className="set-text"><strong>{l.name}</strong><span>{l.kind} · {l.items.toLocaleString('en-US')} items · {l.sizeTB} TB</span></div>
              <Menu label={`Options for ${l.name}`} trigger={<button type="button" className="track-more is-shown" aria-label={`Options for ${l.name}`}><Icon name="more" size={18} /></button>} items={[
                {id: 'settings', label: 'Library settings', icon: 'sliders', onSelect: () => void navigate({to: '/server/$section', params: {section: 'library-settings'}, search: {id: l.id}})},
                {id: 'rename', label: 'Rename…', icon: 'edit', onSelect: () => setRenaming(l)},
                {id: 'open', label: 'Open library', icon: 'library', onSelect: () => void navigate({to: '/library/$libraryId', params: {libraryId: l.id}})},
                {id: 'remove', label: 'Remove library…', icon: 'trash', danger: true, separatorBefore: true, onSelect: () => setRemoving(l)},
              ]} />
            </div>
            {l.scanning ? (
              <div className="lib-scan"><span className="small"><span className="rail-scan"><span /></span> Scanning · {Math.round(l.scanProgress * 100)}%</span><Progress value={l.scanProgress} thin label="Scan progress" /><Button size="sm" variant="ghost" label="Cancel scan" onClick={() => void services.admin.run('cancelScan', {id: l.id}).then(() => toast('Scan canceled'))} /></div>
            ) : <p className="faint small">Last scanned {relativeTime(l.lastScan).toLowerCase()} · {l.rescan.toLowerCase()}</p>}
            <ul className="folder-list">
              {l.folders.map(f => <li key={f.id}><Icon name={f.location === 'Network share' ? 'cloud' : 'folder'} size={15} /><code>{f.path}</code><span className="faint small">{f.location}</span>{f.available ? null : <Badge tone="bad">Missing</Badge>}</li>)}
            </ul>
            <div className="lib-card-foot">
              {l.attention.length ? <Button size="sm" variant="secondary" icon="alert" label={`${l.attention.length} files need attention`} onClick={() => setAttention(l)} /> : <span className="faint small"><Icon name="check" size={13} /> Every file matched</span>}
              <span className="spacer" />
              {!l.scanning ? <Button size="sm" variant="ghost" icon="refresh" label="Scan" onClick={() => void services.admin.run('scan', {id: l.id}).then(() => toast(`Scanning ${l.name}`))} /> : null}
            </div>
          </div>
        ))}
      </div>
      {adding ? <AddLibrary onClose={() => setAdding(false)} onAdd={lib => void set([...q.data!, lib]).then(() => { toast(`Added ${lib.name}. Scanning its folder now.`); setAdding(false); })} /> : null}
      <PromptDialog open={!!renaming} onOpenChange={o => !o && setRenaming(null)} title="Rename library" label="Name" initial={renaming?.name} confirmLabel="Rename" onConfirm={v => set(q.data!.map(l => (l.id === renaming?.id ? {...l, name: v} : l))).then(() => toast('Library renamed'))} />
      <ConfirmDialog open={!!removing} onOpenChange={o => !o && setRemoving(null)} danger title={`Remove ${removing?.name}?`} body="Portico forgets this library and everyone’s progress in it. Your files on disk aren’t touched." typeToConfirm={removing?.name} confirmLabel="Remove library" onConfirm={() => set(q.data!.filter(l => l.id !== removing?.id)).then(() => toast('Library removed'))} />
      <Dialog open={!!attention} onOpenChange={o => !o && setAttention(null)} title={`Files needing attention in ${attention?.name}`} description="Portico couldn’t tell what these are. Rename them to the usual pattern, or match them by hand." size="lg">
        <ul className="attention-list">
          {attention?.attention.map(a => <li key={a.path}><code>{a.path}</code><span className="faint small">{a.reason}</span><Button size="sm" variant="secondary" label="Match…" onClick={() => toast('Opening Fix match for this file')} /></li>)}
        </ul>
      </Dialog>
    </div>
  );
}

const FOLDERS: Record<string, string[]> = {'/': ['Volumes', 'Users', 'srv', 'mnt'], '/Volumes': ['Media', 'Archive', 'Backups'], '/Volumes/Media': ['Movies', 'TV', 'Anime', 'Music', 'Audiobooks', 'Recordings', 'Documentaries'], '/Volumes/Archive': ['Classics', 'Concerts'], '/Users': ['family', 'shared'], '/Users/family': ['Pictures', 'Movies'], '/mnt': ['nas'], '/srv': ['media']};

function AddLibrary({onClose, onAdd}: {onClose: () => void; onAdd: (l: AdminLibrary) => void}) {
  const [step, setStep] = useState(0);
  const [kind, setKind] = useState('Documentaries');
  const [type, setType] = useState('Movies');
  const [name, setName] = useState('Documentaries');
  const [path, setPath] = useState('/Volumes/Media');
  const [picked, setPicked] = useState<string[]>([]);
  const [meta, setMeta] = useState<'online' | 'local'>('online');
  const kids = FOLDERS[path] ?? [];
  const parent = path === '/' ? null : path.split('/').slice(0, -1).join('/') || '/';
  return (
    <Dialog open onOpenChange={o => !o && onClose()} title="Add a library" description={['What’s in it?', 'Where are the files?', 'Where do details come from?'][step]} size="lg" footer={<>
      {step ? <Button variant="ghost" label="Back" onClick={() => setStep(step - 1)} /> : <DialogClose asChild><Button variant="ghost" label="Cancel" /></DialogClose>}
      <span style={{flex: 1}} />
      {step < 2 ? <Button variant="primary" label="Continue" disabled={step === 1 && !picked.length} onClick={() => setStep(step + 1)} /> : <Button variant="primary" label="Add library" onClick={() => onAdd({id: 'lib-' + Date.now(), name, kind: type, items: 0, sizeTB: 0, lastScan: Date.now(), scanning: true, scanProgress: 0.02, folders: picked.map((p, i) => ({id: 'f' + i, path: p, location: 'Local disk', available: true, depth: 6})), attention: [], metadata: meta, language: 'en', country: 'US', rescan: 'Every hour', missingAfter: '1 week', trickplayEvery: 10, trickplayWidth: 320, chapterPictures: true, allowDelete: false, keepDeletedDays: 30, fetchLyrics: type === 'Music'})} />}
    </>}>
      {step === 0 ? (
        <div className="stack">
          <ChoiceCards label="Kind" columns={3} value={type} onChange={v => { setType(v); if (name === kind) { setName(v); setKind(v); } }} options={[
            {value: 'Movies', title: 'Movies', icon: 'film'}, {value: 'TV', title: 'TV shows', icon: 'tv'}, {value: 'Anime', title: 'Anime', icon: 'sparkles'}, {value: 'Music', title: 'Music', icon: 'music'},
            {value: 'Audiobooks', title: 'Audiobooks', icon: 'bookOpen'}, {value: 'Photos', title: 'Photos', icon: 'photos'}, {value: 'Home videos', title: 'Home videos', icon: 'video'},
          ]} />
          <Field label="Name"><Input value={name} onChange={e => setName(e.target.value)} maxLength={40} /></Field>
        </div>
      ) : step === 1 ? (
        <div className="picker">
          <div className="picker-path"><Icon name="drive" size={16} /><code>{path}</code></div>
          <ul className="picker-list">
            {parent !== null ? <li><button type="button" onClick={() => setPath(parent)}><Icon name="back" size={16} />Up one folder</button></li> : null}
            {kids.map(k => {
              const full = `${path === '/' ? '' : path}/${k}`;
              const on = picked.includes(full);
              return (
                <li key={k} className={cx(on && 'is-on')}>
                  <button type="button" onClick={() => (FOLDERS[full] ? setPath(full) : undefined)} disabled={!FOLDERS[full]}><Icon name={on ? 'folderOpen' : 'folder'} size={16} />{k}{FOLDERS[full] ? <Icon name="chevronRight" size={14} className="row-chevron" /> : null}</button>
                  <Button size="sm" variant={on ? 'primary' : 'secondary'} label={on ? 'Added' : 'Use this folder'} onClick={() => setPicked(on ? picked.filter(p => p !== full) : [...picked, full])} />
                </li>
              );
            })}
          </ul>
          {picked.length ? <p className="small">Folders: {picked.map(p => <code key={p} className="chip-code">{p}</code>)}</p> : <p className="faint small">Choose one or more folders. Subfolders are included.</p>}
        </div>
      ) : (
        <ChoiceCards label="Metadata" columns={1} value={meta} onChange={setMeta} options={[{value: 'online', title: 'Look titles up online', body: 'Posters, descriptions, cast and more from TMDB and other providers. Only titles and years are sent.', icon: 'globe'}, {value: 'local', title: 'Use only what’s in the files', body: 'Nothing leaves the server. Good for home videos and photos.', icon: 'lock'}]} />
      )}
    </Dialog>
  );
}

function LibrarySettings({sec}: {sec: Sec}) {
  const search = useSearch({from: '/app/server/$section'});
  const navigate = useNavigate();
  const form = useAdminForm('libraries');
  const libs = form.data;
  if (!libs) return <AdminSkeleton />;
  const lib = libs.find(l => l.id === search.id) ?? libs[0]!;
  const set = (patch: Partial<AdminLibrary>) => form.replace(libs.map(l => (l.id === lib.id ? {...l, ...patch} : l)));
  const video = ['Movies', 'TV', 'Anime', 'Home videos'].includes(lib.kind);
  return (
    <div className="adm-sections">
      <SectionHead title={sec.label} lede={sec.lede} actions={<Select label="Library" value={lib.id} onChange={v => void navigate({to: '/server/$section', params: {section: 'library-settings'}, search: {id: v}, replace: true})} options={libs.map(l => ({value: l.id, label: l.name}))} />} />
      <Panel title="Matching" padded={false}>
        <Row label="Details come from" help={lib.metadata === 'online' ? 'Looked up online by title and year' : 'Only what’s in the files'}><Select label="Details come from" value={lib.metadata} onChange={v => set({metadata: v})} options={[{value: 'online', label: 'Online providers'}, {value: 'local', label: 'The files only'}]} /></Row>
        <Row label="Language" help="For titles, descriptions and artwork."><Select label="Language" value={lib.language} onChange={v => set({language: v})} options={[{value: 'en', label: 'English'}, {value: 'es', label: 'Spanish'}, {value: 'fr', label: 'French'}, {value: 'ja', label: 'Japanese'}]} /></Row>
        <Row label="Country" help="For ratings and release dates."><Select label="Country" value={lib.country} onChange={v => set({country: v})} options={[{value: 'US', label: 'United States'}, {value: 'CA', label: 'Canada'}, {value: 'GB', label: 'United Kingdom'}]} /></Row>
        {lib.kind === 'Music' ? <Row label="Fetch lyrics" help="Look up synced lyrics for songs without them."><Switch checked={lib.fetchLyrics} onChange={v => set({fetchLyrics: v})} label="Fetch lyrics" /></Row> : null}
      </Panel>
      <Panel title="Scanning" padded={false}>
        <Row label="Look for changes"><Select label="Look for changes" value={lib.rescan} onChange={v => set({rescan: v})} options={['When files change', 'Every hour', 'Every 6 hours', 'Every day', 'Only when I scan'].map(v => ({value: v, label: v}))} /></Row>
        <Row label="Mark missing files after" help="Titles whose files disappear stay visible this long, in case a drive is just disconnected."><Select label="Missing after" value={lib.missingAfter} onChange={v => set({missingAfter: v})} options={['1 day', '1 week', '1 month', 'Never'].map(v => ({value: v, label: v}))} /></Row>
      </Panel>
      {video ? (
        <Panel title="Pictures" padded={false}>
          <Row label="Seek-bar pictures" help="A frame every few seconds for previews while scrubbing."><Stepper label="Seconds between pictures" value={lib.trickplayEvery} onChange={v => set({trickplayEvery: v})} min={2} max={30} suffix="s" /></Row>
          <Row label="Picture width"><Select label="Picture width" value={String(lib.trickplayWidth)} onChange={v => set({trickplayWidth: Number(v)})} options={[{value: '240', label: '240 pixels'}, {value: '320', label: '320 pixels'}, {value: '480', label: '480 pixels'}]} /></Row>
          <Row label="Chapter pictures" help="A still for each chapter."><Switch checked={lib.chapterPictures} onChange={v => set({chapterPictures: v})} label="Chapter pictures" /></Row>
        </Panel>
      ) : null}
      <Panel title="Deleting" padded={false}>
        <Row label="Allow deleting from Portico" help="Owners and admins can delete titles, and their files, from the title’s menu."><Switch checked={lib.allowDelete} onChange={v => set({allowDelete: v})} label="Allow deleting" /></Row>
        <Row label="Keep deleted titles for" help="In Maintenance › Deleted titles, so they can be put back."><Stepper label="Days" value={lib.keepDeletedDays} onChange={v => set({keepDeletedDays: v})} min={0} max={90} suffix="days" /></Row>
      </Panel>
      <UnsavedBar form={form} />
    </div>
  );
}

function Metadata({sec}: {sec: Sec}) {
  const form = useAdminForm('metadata');
  const [lookup, setLookup] = useState('');
  const [result, setResult] = useState<string | null>(null);
  const m = form.data;
  if (!m) return <AdminSkeleton />;
  return (
    <div className="adm-sections">
      <SectionHead title={sec.label} lede={sec.lede} />
      {m.consent !== 'granted' ? <Notice tone="warn" title="Online lookups are off" action={<Button size="sm" variant="primary" label="Allow" onClick={() => form.set({consent: 'granted'})} />}>Libraries use only what’s in the files until you allow Portico to look titles up.</Notice> : null}
      <Panel title="Providers" subtitle="Portico asks these in order. Only titles, years and IDs are sent." padded={false}>
        {m.providers.map(p => (
          <Row key={p.id} label={p.name} help={`${p.kinds}${p.key ? ' · Uses your own API key' : ''}`}>
            {p.key ? <Button size="sm" variant="ghost" label="API key…" onClick={() => toast(`${p.name} key saved on the server`)} /> : null}
            <Switch checked={p.enabled} onChange={v => form.set({providers: m.providers.map(x => (x.id === p.id ? {...x, enabled: v} : x))})} label={`Use ${p.name}`} />
          </Row>
        ))}
      </Panel>
      <Panel title="Defaults" padded={false}>
        <Row label="Online lookups"><Select label="Online lookups" value={m.consent} onChange={v => form.set({consent: v})} options={[{value: 'granted', label: 'Allowed'}, {value: 'declined', label: 'Off'}]} /></Row>
        <Row label="Refresh details every"><Stepper label="Refresh interval" value={m.refreshDays} onChange={v => form.set({refreshDays: v})} min={7} max={180} step={7} suffix="days" /></Row>
        <Row label="Lyrics" help="Fetch synced lyrics for music."><Switch checked={m.lyrics} onChange={v => form.set({lyrics: v})} label="Lyrics" /></Row>
      </Panel>
      <Panel title="Look something up" subtitle="Test what the providers return for a title.">
        <form className="hstack" onSubmit={e => { e.preventDefault(); setResult(lookup ? `TMDB: “${lookup}” · 3 candidates · best match 96%` : null); }}>
          <Input value={lookup} onChange={e => setLookup(e.target.value)} placeholder="Charade 1963" aria-label="Title to look up" style={{flex: 1, minWidth: 200}} />
          <Button variant="secondary" icon="search" label="Look up" type="submit" />
        </form>
        {result ? <p className="small" style={{marginTop: 10}}>{result}</p> : null}
      </Panel>
      <UnsavedBar form={form} />
    </div>
  );
}

function Storage({sec}: {sec: Sec}) {
  const services = useServices();
  const q = useAdmin('storage');
  const [adding, setAdding] = useState<'webdav' | 'rclone' | null>(null);
  const [f, setF] = useState({name: '', address: '', user: '', pass: ''});
  const [remove, setRemove] = useState<StorageMount | null>(null);
  if (!q.data) return <AdminSkeleton />;
  const set = (list: StorageMount[]) => services.admin.set('storage', list);
  return (
    <div className="adm-sections">
      <SectionHead title={sec.label} lede={sec.lede} actions={<Menu label="Add storage" trigger={<button type="button" className="btn btn-primary btn-md"><Icon name="plus" size={18} /><span className="btn-label">Add storage</span></button>} items={[{id: 'webdav', label: 'WebDAV share', icon: 'globe', onSelect: () => setAdding('webdav')}, {id: 'rclone', label: 'Cloud storage (rclone)', icon: 'cloud', onSelect: () => setAdding('rclone')}]} />} />
      <Panel padded={false}>
        {q.data.map(s => (
          <Row key={s.id} icon={s.kind === 'webdav' ? 'globe' : 'cloud'} label={s.name} help={<><code>{s.address}</code>{s.libraries.length ? ` · Used by ${s.libraries.join(', ')}` : ' · Not used by a library'}{s.state === 'error' ? ' · The remote refused the saved sign-in' : ''}</>}>
            <StatusBadge state={s.state} />
            <Button size="sm" variant="secondary" label={s.state === 'mounted' ? 'Unmount' : 'Mount'} onClick={() => void set(q.data!.map(x => (x.id === s.id ? {...x, state: x.state === 'mounted' ? 'unmounted' : 'mounted'} : x))).then(() => toast(s.state === 'mounted' ? `${s.name} unmounted` : `${s.name} mounted`))} />
            <Button size="sm" variant="ghost" icon="trash" aria-label={`Remove ${s.name}`} onClick={() => setRemove(s)} />
          </Row>
        ))}
      </Panel>
      <Dialog open={!!adding} onOpenChange={o => !o && setAdding(null)} title={adding === 'webdav' ? 'Add a WebDAV share' : 'Add cloud storage'} size="sm" footer={<><DialogClose asChild><Button variant="ghost" label="Cancel" /></DialogClose><Button variant="primary" label="Add and mount" disabled={!f.name || !f.address} onClick={() => void set([...q.data!, {id: 'st' + Date.now(), kind: adding!, name: f.name, address: f.address, state: 'mounted', usedTB: 0, libraries: []}]).then(() => { toast(`${f.name} mounted`); setAdding(null); setF({name: '', address: '', user: '', pass: ''}); })} /></>}>
        <div className="stack">
          <Field label="Name"><Input value={f.name} onChange={e => setF({...f, name: e.target.value})} autoFocus /></Field>
          <Field label={adding === 'webdav' ? 'Address' : 'Remote and path'} help={adding === 'webdav' ? 'Like https://cloud.example.com/remote.php/dav/files/me' : 'Like b2:bucket/folder, from your rclone configuration'}><Input value={f.address} onChange={e => setF({...f, address: e.target.value})} /></Field>
          {adding === 'webdav' ? <><Field label="Username"><Input value={f.user} onChange={e => setF({...f, user: e.target.value})} /></Field><Field label="Password"><PasswordInput value={f.pass} onChange={e => setF({...f, pass: e.target.value})} /></Field></> : null}
        </div>
      </Dialog>
      <ConfirmDialog open={!!remove} onOpenChange={o => !o && setRemove(null)} danger title={`Remove ${remove?.name}?`} body={remove?.libraries.length ? `${remove.libraries.join(', ')} will lose the folders on it.` : 'Nothing uses it.'} confirmLabel="Remove" onConfirm={() => set(q.data!.filter(x => x.id !== remove?.id)).then(() => toast('Storage removed'))} />
    </div>
  );
}
