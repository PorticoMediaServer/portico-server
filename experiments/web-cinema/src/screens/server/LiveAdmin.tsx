import {useState} from 'react';
import {useNavigate, useSearch} from '@tanstack/react-router';
import {useQuery, useServices} from '../../data/hooks';
import type {LibraryChannel, LiveSource} from '../../data/admin';
import {Panel, Row} from '../../ui/Layout';
import {Button} from '../../ui/Button';
import {Checkbox, ChoiceCards, Field, Input, Segmented, Select, Stepper, Switch} from '../../ui/Controls';
import {Badge, Notice, Progress, toast} from '../../ui/Feedback';
import {Icon} from '../../ui/Icon';
import {Art} from '../../ui/Media';
import {ConfirmDialog, Dialog, DialogClose, Menu} from '../../ui/Overlay';
import {plural, relativeTime} from '../shared/items';
import {AdminSkeleton, HealthDot, SectionHead, StatusBadge, UnsavedBar, useAdmin, useAdminForm} from './common';
import type {Sec} from './Server';

export function LiveSection({id, sec}: {id: string; sec: Sec}) {
  switch (id) {
    case 'dvr': return <Dvr sec={sec} />;
    case 'channels': return <LibraryChannels sec={sec} />;
    case 'channel-builder': return <Builder sec={sec} />;
    default: return <LiveTv sec={sec} />;
  }
}

function LiveTv({sec}: {sec: Sec}) {
  const services = useServices();
  const form = useAdminForm('liveTv');
  const channels = useQuery('live.channels', () => services.live.channels());
  const [adding, setAdding] = useState(false);
  const [edit, setEdit] = useState<LiveSource | null>(null);
  const [map, setMap] = useState<LiveSource | null>(null);
  const [kind, setKind] = useState<LiveSource['kind']>('HDHomeRun');
  const [found, setFound] = useState<'idle' | 'looking' | 'found'>('idle');
  const [addr, setAddr] = useState('');
  const l = form.data;
  if (!l) return <AdminSkeleton />;
  const setSource = (s: LiveSource) => form.set({sources: l.sources.map(x => (x.id === s.id ? s : x))});
  return (
    <div className="adm-sections">
      <SectionHead title={sec.label} lede={sec.lede} actions={<Button variant="primary" icon="plus" label="Add a source" onClick={() => setAdding(true)} />} />
      <div className="src-cards">
        {l.sources.map(s => (
          <div key={s.id} className="src-card">
            <div className="lib-card-head">
              <span className="set-icon"><Icon name={s.kind === 'HDHomeRun' ? 'tv' : s.kind === 'Guide only' ? 'calendar' : 'globe'} size={22} /></span>
              <div className="set-text"><strong>{s.name}</strong><span>{s.kind} · {s.kind === 'Guide only' ? 'Guide data' : `${s.channels} channels · ${s.tuners} tuners`}</span></div>
              <HealthDot state={s.health} />
            </div>
            <code className="src-addr">{s.address}</code>
            <p className="faint small">Guide refreshed {relativeTime(s.lastRefresh).toLowerCase()}{s.guideFrom ? ` · guide from ${l.sources.find(x => x.id === s.guideFrom)?.name}` : ''}{s.paused ? ' · Paused' : ''}</p>
            {s.health === 'warn' ? <Notice tone="warn">The last guide refresh didn’t finish; 3 channels have no listings.</Notice> : null}
            <div className="lib-card-foot">
              <Button size="sm" variant="secondary" label="Settings" onClick={() => setEdit(s)} />
              {s.kind !== 'Guide only' ? <Button size="sm" variant="ghost" label="Channels" onClick={() => setMap(s)} /> : null}
              <span className="spacer" />
              <Button size="sm" variant="ghost" icon="refresh" label="Refresh guide" onClick={() => { setSource({...s, lastRefresh: Date.now(), health: 'ok'}); toast(`Refreshing the guide for ${s.name}`); }} />
            </div>
          </div>
        ))}
      </div>
      <Panel title="Recordings" padded={false}>
        <Row label="Recordings folder" help={l.writable ? 'Writable · on the Media volume' : 'Portico can’t write here'}><code>{l.recordingsPath}</code><Button size="sm" variant="ghost" label="Change…" onClick={() => toast('Choose a folder in the folder picker')} /></Row>
        <Row label="Space for recordings" help={`${l.usedGB} GB used of ${l.capGB} GB allowed`}><span style={{width: 180}}><Progress value={l.usedGB / l.capGB} label="Recording space" /></span></Row>
        <Row label="Leave free on the disk"><Stepper label="Leave free" value={l.leaveFreeGB} onChange={v => form.set({leaveFreeGB: v})} min={10} max={1000} step={10} suffix="GB" /></Row>
        <Row label="Keep recordings for" help="Unless kept or a series rule keeps them."><Stepper label="Keep days" value={l.keepDays} onChange={v => form.set({keepDays: v})} min={1} max={365} suffix="days" /></Row>
      </Panel>
      <UnsavedBar form={form} />
      <Dialog open={adding} onOpenChange={o => { setAdding(o); setFound('idle'); }} title="Add a Live TV source" size="lg" footer={<><DialogClose asChild><Button variant="ghost" label="Cancel" /></DialogClose><Button variant="primary" label="Add source" disabled={kind === 'HDHomeRun' ? found !== 'found' : !addr} onClick={() => { form.set({sources: [...l.sources, {id: 'src' + Date.now(), name: kind === 'HDHomeRun' ? 'HDHomeRun FLEX 4K' : addr.replace(/^https?:\/\//, '').split('/')[0] || 'New source', kind, address: kind === 'HDHomeRun' ? '192.168.1.61' : addr, channels: kind === 'Guide only' ? 0 : 36, tuners: kind === 'HDHomeRun' ? 4 : 2, paused: false, lastRefresh: Date.now(), guideFrom: null, numbering: 'asSent', startAt: 1, logos: true, health: 'ok'}]}); setAdding(false); toast('Source added. Save to start using it.'); }} /></>}>
        <div className="stack">
          <ChoiceCards label="Source type" columns={2} value={kind} onChange={setKind} options={[{value: 'HDHomeRun', title: 'Tuner on this network', body: 'HDHomeRun and compatible tuners, found automatically.', icon: 'tv'}, {value: 'M3U', title: 'M3U playlist', body: 'A playlist address from your IPTV provider.', icon: 'list'}, {value: 'Xtream', title: 'Xtream login', body: 'Server, username and password.', icon: 'key'}, {value: 'Guide only', title: 'Guide listings (XMLTV)', body: 'Programme data for channels from another source.', icon: 'calendar'}, {value: 'Upload', title: 'Upload a file', body: 'An M3U or XMLTV file from this computer.', icon: 'upload'}]} />
          {kind === 'HDHomeRun' ? (
            found === 'found' ? <Notice tone="good" title="Found HDHomeRun FLEX 4K at 192.168.1.61">4 tuners · 36 channels after a scan.</Notice> : <Button variant="secondary" icon="search" label={found === 'looking' ? 'Looking on your network…' : 'Look for tuners'} busy={found === 'looking'} onClick={() => { setFound('looking'); setTimeout(() => setFound('found'), 1200); }} />
          ) : kind === 'Upload' ? (
            <label className="drop"><Icon name="upload" size={22} /><span>Drop a .m3u or .xml file here, or <u>choose one</u></span><input type="file" accept=".m3u,.m3u8,.xml" className="sr-only" onChange={e => setAddr(e.target.files?.[0]?.name ?? '')} /></label>
          ) : <Field label={kind === 'Xtream' ? 'Server address' : 'Address'}><Input value={addr} onChange={e => setAddr(e.target.value)} placeholder={kind === 'Guide only' ? 'https://example.com/guide.xml' : 'https://example.com/playlist.m3u'} /></Field>}
        </div>
      </Dialog>
      {edit ? (
        <Dialog open onOpenChange={o => !o && setEdit(null)} title={`${edit.name} settings`} size="md" footer={<><Button variant="danger" label="Remove source" onClick={() => { form.set({sources: l.sources.filter(x => x.id !== edit.id)}); setEdit(null); }} /><span style={{flex: 1}} /><DialogClose asChild><Button variant="ghost" label="Cancel" /></DialogClose><Button variant="primary" label="Done" onClick={() => { setSource(edit); setEdit(null); }} /></>}>
          <div className="stack">
            <Field label="Name"><Input value={edit.name} onChange={e => setEdit({...edit, name: e.target.value})} /></Field>
            <Field label="Guide from"><Select value={edit.guideFrom ?? 'self'} onChange={v => setEdit({...edit, guideFrom: v === 'self' ? null : v})} options={[{value: 'self', label: 'This source'}, ...l.sources.filter(x => x.kind === 'Guide only').map(x => ({value: x.id, label: x.name}))]} /></Field>
            <div className="field"><span className="field-label">Channel numbers</span><Segmented size="sm" label="Numbering" value={edit.numbering} onChange={v => setEdit({...edit, numbering: v})} options={[{value: 'asSent', label: 'As the source sends them'}, {value: 'inOrder', label: 'In order'}]} /></div>
            {edit.numbering === 'inOrder' ? <div className="field"><span className="field-label">Start at</span><Stepper label="Start at" value={edit.startAt} onChange={v => setEdit({...edit, startAt: v})} min={1} max={9999} /></div> : null}
            <Checkbox checked={edit.logos} onChange={v => setEdit({...edit, logos: v})} label="Import channel logos" />
            <Checkbox checked={edit.paused} onChange={v => setEdit({...edit, paused: v})} label="Pause this source (its channels hide until you resume it)" />
          </div>
        </Dialog>
      ) : null}
      <Dialog open={!!map} onOpenChange={o => !o && setMap(null)} title={`Channels from ${map?.name}`} description="Turn channels off to hide them for everyone, or change their numbers." size="lg">
        <ul className="map-list">
          {(channels.data ?? []).filter(c => !c.library).slice(0, 14).map(c => (
            <li key={c.id}><span className="g-logo"><Art src={c.logo} tint={c.tint} /></span><Input defaultValue={c.number} aria-label={`Number for ${c.name}`} className="map-num" /><span className="set-text"><strong>{c.name}</strong><span>{c.group}</span></span><Switch checked={!c.hidden} onChange={v => void services.live.setChannel(c.id, {hidden: !v})} label={`Show ${c.name}`} /></li>
          ))}
        </ul>
      </Dialog>
    </div>
  );
}

function Dvr({sec}: {sec: Sec}) {
  const form = useAdminForm('dvr');
  const navigate = useNavigate();
  const d = form.data;
  if (!d) return <AdminSkeleton />;
  return (
    <div className="adm-sections">
      <SectionHead title={sec.label} lede={sec.lede} />
      <Panel title="Tuners" padded={false}>
        {d.tuners.map(t => <Row key={t.id} icon="tv" label={t.name} help={`${t.source}${t.next ? ` · Next: ${t.next}` : ''}`}>{t.inUse ? <Badge tone="live">{t.inUse}</Badge> : <Badge tone="good">Free</Badge>}</Row>)}
      </Panel>
      <Panel title="Recording defaults" padded={false}>
        <Row label="Start early"><Stepper label="Start early" value={d.startEarly} onChange={v => form.set({startEarly: v})} max={30} suffix="min" /></Row>
        <Row label="End late"><Stepper label="End late" value={d.endLate} onChange={v => form.set({endLate: v})} max={120} suffix="min" /></Row>
        <Row label="Keep"><Segmented size="sm" label="Keep" value={d.keepMode} onChange={v => form.set({keepMode: v})} options={[{value: 'newest', label: `Newest ${d.keepCount}`}, {value: 'days', label: 'By age'}, {value: 'all', label: 'All'}]} /></Row>
        {d.keepMode === 'newest' ? <Row label="Episodes to keep per series"><Stepper label="Episodes" value={d.keepCount} onChange={v => form.set({keepCount: v})} min={1} max={50} /></Row> : null}
        <Row label="After recording" help="Repackaging makes files smaller and easier to play."><Select label="After recording" value={d.afterRecording} onChange={v => form.set({afterRecording: v})} options={[{value: 'none', label: 'Leave as recorded'}, {value: 'remux', label: 'Repackage (no quality loss)'}, {value: 'convert', label: 'Convert to H.264'}]} /></Row>
        {d.afterRecording !== 'none' ? <Row label="Remove the original afterwards"><Switch checked={d.removeOriginal} onChange={v => form.set({removeOriginal: v})} label="Remove original" /></Row> : null}
      </Panel>
      <Panel title="Files" padded={false}>
        <Row label="Shows" help="{show}, {season}, {episode} and {title} are filled in." stack><Input value={d.folderShows} onChange={e => form.set({folderShows: e.target.value})} aria-label="Folder pattern for shows" className="mono" /></Row>
        <Row label="Movies" stack><Input value={d.folderMovies} onChange={e => form.set({folderMovies: e.target.value})} aria-label="Folder pattern for movies" className="mono" /></Row>
        <Row label="Write .nfo details beside recordings" help="For other apps that read them."><Switch checked={d.sidecarNfo} onChange={v => form.set({sidecarNfo: v})} label="Write .nfo" /></Row>
        <Row label="Save artwork beside recordings"><Switch checked={d.sidecarFanart} onChange={v => form.set({sidecarFanart: v})} label="Save artwork" /></Row>
      </Panel>
      <Panel title="Live TV defaults" padded={false}>
        <Row label="Buffer before playing" help="More is smoother on weak signals."><Stepper label="Buffer" value={d.buffer} onChange={v => form.set({buffer: v})} min={1} max={30} suffix="s" /></Row>
        <Row label="Retry a lost signal for"><Stepper label="Retry" value={d.retry} onChange={v => form.set({retry: v})} min={5} max={120} step={5} suffix="s" /></Row>
        <Row label="Guide days to load"><Stepper label="Guide days" value={d.guideDays} onChange={v => form.set({guideDays: v})} min={1} max={21} /></Row>
        <Row label="Find tuners on the network automatically"><Switch checked={d.discovery} onChange={v => form.set({discovery: v})} label="Discovery" /></Row>
      </Panel>
      <Panel title="Who can record" padded={false}>
        {d.recorders.map((r, i) => <Row key={r.profile} icon="profile" label={r.profile} help={`${r.account}${r.inherit ? ' · Follows the account' : ''}`}><Switch checked={r.allowed} onChange={v => form.set({recorders: d.recorders.map((x, j) => (j === i ? {...x, allowed: v, inherit: false} : x))})} label={`${r.profile} can record`} /></Row>)}
      </Panel>
      <Panel title="Series rules on this server" padded={false} actions={<Button size="sm" variant="ghost" label="Open Recordings" onClick={() => void navigate({to: '/channels/$entry', params: {entry: 'recordings'}, search: {tab: 'rules'}})} />}>
        {d.rules.map(r => <Row key={r.id} icon="repeat" label={r.name} help={`${r.owner} · ${r.channel} · ${r.newOnly ? 'New only' : 'All episodes'} · Priority ${r.priority}`}>{r.paused ? <Badge>Paused</Badge> : <Badge tone="accent">On</Badge>}</Row>)}
      </Panel>
      <UnsavedBar form={form} />
    </div>
  );
}

function LibraryChannels({sec}: {sec: Sec}) {
  const services = useServices();
  const navigate = useNavigate();
  const q = useAdmin('channels');
  const [del, setDel] = useState<LibraryChannel | null>(null);
  if (!q.data) return <AdminSkeleton />;
  const set = (list: LibraryChannel[]) => services.admin.set('channels', list);
  return (
    <div className="adm-sections">
      <SectionHead title={sec.label} lede={sec.lede} actions={<Button variant="primary" icon="plus" label="New channel" onClick={() => void navigate({to: '/server/$section', params: {section: 'channel-builder'}})} />} />
      <Panel title="Start from a template" subtitle="A ready-made rule you can change.">
        <div className="chip-row">{['Saturday morning cartoons', '80s action', 'Sitcom marathon', 'Film noir nights', 'Nature documentaries'].map(t => <button key={t} type="button" className="chip" onClick={() => void navigate({to: '/server/$section', params: {section: 'channel-builder'}, search: {tab: t}})}><Icon name="wand" size={14} />{t}</button>)}</div>
      </Panel>
      <Panel padded={false}>
        {q.data.map(c => (
          <Row key={c.id} icon="channels" label={<>{c.number} · {c.name}</>} help={`${c.libraries.join(', ')} · ${c.conditions.map(x => `${x.field} ${x.op} ${x.value}`).join(', ')} · ${c.order} · ${c.candidates} titles · schedule ahead ${c.scheduleHours} h${c.unresolved ? ` · ${c.unresolved} titles couldn’t be placed` : ''}`}>
            <StatusBadge state={c.health} />
            <Switch checked={c.on} onChange={v => void set(q.data!.map(x => (x.id === c.id ? {...x, on: v} : x))).then(() => toast(v ? `${c.name} is on` : `${c.name} is off`))} label={`${c.name} on`} />
            <Menu label={`Options for ${c.name}`} trigger={<button type="button" className="track-more is-shown" aria-label={`Options for ${c.name}`}><Icon name="more" size={18} /></button>} items={[
              {id: 'edit', label: 'Edit rules…', icon: 'edit', onSelect: () => void navigate({to: '/server/$section', params: {section: 'channel-builder'}, search: {id: c.id}})},
              {id: 'regen', label: 'Make the schedule again', icon: 'refresh', onSelect: () => void set(q.data!.map(x => (x.id === c.id ? {...x, health: 'beingMade'} : x))).then(() => toast(`Rebuilding ${c.name}’s schedule`))},
              {id: 'logo', label: 'Change logo…', icon: 'image', onSelect: () => toast('Choose a PNG or SVG logo')},
              {id: 'delete', label: 'Delete channel…', icon: 'trash', danger: true, separatorBefore: true, onSelect: () => setDel(c)},
            ]} />
          </Row>
        ))}
      </Panel>
      <ConfirmDialog open={!!del} onOpenChange={o => !o && setDel(null)} danger title={`Delete ${del?.name}?`} body="The channel and its schedule go. The titles in it stay in your libraries." confirmLabel="Delete channel" onConfirm={() => set(q.data!.filter(x => x.id !== del?.id)).then(() => toast('Channel deleted'))} />
    </div>
  );
}

type Cond = {field: string; op: string; value: string};
function Builder({sec}: {sec: Sec}) {
  const services = useServices();
  const navigate = useNavigate();
  const search = useSearch({from: '/app/server/$section'});
  const q = useAdmin('channels');
  const existing = q.data?.find(c => c.id === search.id);
  const [name, setName] = useState(existing?.name ?? search.tab ?? 'Film noir nights');
  const [number, setNumber] = useState(existing?.number ?? '901');
  const [libs, setLibs] = useState<string[]>(existing?.libraries ?? ['Movies']);
  const [kinds, setKinds] = useState<string[]>(existing?.kinds ?? ['movies']);
  const [conds, setConds] = useState<Cond[]>(existing?.conditions ?? [{field: 'Genre', op: 'is', value: 'Crime'}, {field: 'Decade', op: 'is', value: '1940s'}]);
  const [order, setOrder] = useState(existing?.order ?? 'Shuffle, no repeats');
  const [episodes, setEpisodes] = useState(existing?.episodes ?? 'Mixed with everything else');
  const [hours, setHours] = useState(existing?.scheduleHours ?? 72);
  const preview = useQuery(`builder.preview.${conds.map(c => c.value).join()}`, () => services.catalog.browse({libraryId: 'lib-movies', filters: {quick: [], fields: Object.fromEntries(conds.filter(c => (c.field === 'Genre' || c.field === 'Decade') && c.op === 'is' && c.value).map(c => [c.field === 'Genre' ? 'genre' : 'decade', [c.value]]))}, sort: {field: 'random', direction: 'asc'}, pageSize: 8}));
  return (
    <div className="adm-sections">
      <SectionHead title={existing ? `Edit ${existing.name}` : sec.label} lede={sec.lede} actions={<><Button variant="ghost" label="Cancel" onClick={() => void navigate({to: '/server/$section', params: {section: 'channels'}})} /><Button variant="primary" label={existing ? 'Save channel' : 'Create channel'} disabled={!name.trim() || !libs.length} onClick={() => { const c: LibraryChannel = {id: existing?.id ?? 'ch-lib-' + Date.now(), name, number, on: true, libraries: libs, kinds: kinds as LibraryChannel['kinds'], order, episodes, conditions: conds, scheduleHours: hours, candidates: preview.data?.total ?? 0, unresolved: 0, health: 'beingMade', description: 'Always-on channel built from your library.'}; void services.admin.set('channels', existing ? q.data!.map(x => (x.id === c.id ? c : x)) : [...(q.data ?? []), c]).then(() => { toast(`${name} is being made; it appears in Channels in a minute`); void navigate({to: '/server/$section', params: {section: 'channels'}}); }); }} /></>} />
      <div className="builder">
        <div className="stack">
          <Panel title="Channel">
            <div className="grid-2"><Field label="Name"><Input value={name} onChange={e => setName(e.target.value)} /></Field><Field label="Number"><Input value={number} onChange={e => setNumber(e.target.value)} inputMode="decimal" /></Field></div>
          </Panel>
          <Panel title="What plays">
            <div className="stack">
              <div className="field"><span className="field-label">From these libraries</span><div className="chip-row">{['Movies', 'TV Shows', 'Anime', 'Home Videos'].map(l => <button key={l} type="button" className={`chip${libs.includes(l) ? ' is-on' : ''}`} aria-pressed={libs.includes(l)} onClick={() => setLibs(libs.includes(l) ? libs.filter(x => x !== l) : [...libs, l])}>{l}</button>)}</div></div>
              <div className="field"><span className="field-label">Kinds</span><div className="hstack"><Checkbox checked={kinds.includes('movies')} onChange={v => setKinds(v ? [...kinds, 'movies'] : kinds.filter(k => k !== 'movies'))} label="Films" /><Checkbox checked={kinds.includes('shows')} onChange={v => setKinds(v ? [...kinds, 'shows'] : kinds.filter(k => k !== 'shows'))} label="Episodes" /></div></div>
              <div className="field"><span className="field-label">Only titles where</span>
                <div className="conds">
                  {conds.map((c, i) => (
                    <div key={i} className="cond">
                      <Select label="Field" value={c.field} onChange={v => setConds(conds.map((x, j) => (j === i ? {...x, field: v} : x)))} options={['Genre', 'Decade', 'Studio', 'Rating', 'Mood', 'Actor', 'Unwatched'].map(v => ({value: v, label: v}))} />
                      <Select label="Operator" value={c.op} onChange={v => setConds(conds.map((x, j) => (j === i ? {...x, op: v} : x)))} options={[{value: 'is', label: 'is'}, {value: 'is not', label: 'is not'}, {value: 'contains', label: 'contains'}]} />
                      <Input value={c.value} onChange={e => setConds(conds.map((x, j) => (j === i ? {...x, value: e.target.value} : x)))} aria-label="Value" />
                      <button type="button" className="track-more is-shown" onClick={() => setConds(conds.filter((_, j) => j !== i))} aria-label="Remove condition"><Icon name="close" size={16} /></button>
                    </div>
                  ))}
                  <Button size="sm" variant="ghost" icon="plus" label="Add a condition" onClick={() => setConds([...conds, {field: 'Genre', op: 'is', value: ''}])} />
                </div>
              </div>
            </div>
          </Panel>
          <Panel title="Order">
            <div className="stack">
              <Field label="Order"><Select value={order} onChange={setOrder} options={['Shuffle, no repeats', 'In order, then shuffled', 'Oldest first', 'Newest first', 'By title'].map(v => ({value: v, label: v}))} /></Field>
              <Field label="Episodes"><Select value={episodes} onChange={setEpisodes} options={['Mixed with everything else', 'A few in a row', 'Whole seasons in order'].map(v => ({value: v, label: v}))} /></Field>
              <details className="tech"><summary>Advanced</summary><div className="field" style={{marginTop: 12}}><span className="field-label">Schedule ahead</span><Stepper label="Hours ahead" value={hours} onChange={setHours} min={12} max={168} step={12} suffix="h" /></div></details>
            </div>
          </Panel>
        </div>
        <Panel title="Preview" subtitle={preview.data ? `${plural(preview.data.total, 'title')} ${preview.data.total === 1 ? 'matches' : 'match'} · the first few` : 'Finding titles…'} className="builder-preview">
          <ol className="preview-list">
            {(preview.data?.items ?? []).map((i, n) => <li key={i.id}><span className="tabular faint">{new Date(Date.now() + n * 105 * 60000).toLocaleTimeString('en-US', {hour: 'numeric', minute: '2-digit'})}</span><Art src={i.art.poster} tint={i.tint} ratio="2/3" className="pv-poster" /><span className="set-text"><strong>{i.title}</strong><span>{i.year} · {i.genres.slice(0, 2).join(', ')}</span></span></li>)}
          </ol>
          {preview.data && !preview.data.total ? <Notice tone="warn">Nothing matches these conditions yet.</Notice> : null}
        </Panel>
      </div>
    </div>
  );
}
