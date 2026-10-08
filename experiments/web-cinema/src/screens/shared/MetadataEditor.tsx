import {useEffect, useState} from 'react';
import {useQuery, useServices} from '../../data/hooks';
import type {Item, Movie} from '../../data/types';
import {artKeys} from '../../data/art';
import {Dialog, DialogClose} from '../../ui/Overlay';
import {Button} from '../../ui/Button';
import {Checkbox, Field, Input, Select, TextArea} from '../../ui/Controls';
import {Badge, Notice, Skeleton, Spinner, toast} from '../../ui/Feedback';
import {Icon, type IconName} from '../../ui/Icon';
import {Art, Avatar} from '../../ui/Media';
import {cx} from '../../ui/cx';
import {plural, relativeTime} from './items';

type Tab = 'general' | 'artwork' | 'tags' | 'people' | 'matching' | 'history';
const TABS: {id: Tab; label: string; icon: IconName}[] = [
  {id: 'general', label: 'General', icon: 'edit'}, {id: 'artwork', label: 'Artwork', icon: 'image'}, {id: 'tags', label: 'Tags & genres', icon: 'tags'},
  {id: 'people', label: 'Cast & crew', icon: 'people'}, {id: 'matching', label: 'Fix match', icon: 'wand'}, {id: 'history', label: 'History', icon: 'history'},
];

/**
 * The metadata editor (owners and admins): fields with locks so refreshes keep your edits,
 * artwork, genres, credits, fixing the match, and undoable history. With several items it edits
 * them together and leaves mixed values alone.
 */
export function MetadataEditor({ids, initialTab, onOpenChange}: {ids: string[]; initialTab?: string; onOpenChange: (o: boolean) => void}) {
  const services = useServices();
  const items = useQuery(`meta.items.${ids.join(',')}`, () => services.catalog.items(ids));
  const bulk = ids.length > 1;
  const [tab, setTab] = useState<Tab>((TABS.some(t => t.id === initialTab) ? initialTab : 'general') as Tab);
  const first = items.data?.[0];
  const title = bulk ? `Edit ${plural(ids.length, 'item')}` : first ? `Edit “${first.title}”` : 'Edit metadata';
  const tabs = bulk ? TABS.filter(t => t.id === 'general' || t.id === 'tags') : TABS;
  return (
    <Dialog open onOpenChange={onOpenChange} title={title} description={bulk ? 'Only the fields you change are applied; mixed values are left as they are.' : 'Edits are kept on this server and locked so refreshes don’t undo them.'} kind="sheet" size="xl" className="meta-sheet">
      {!first ? <div className="stack"><Skeleton h={40} /><Skeleton h={300} /></div> : (
        <div className="meta">
          <nav className="meta-tabs" role="tablist" aria-label="Metadata sections">
            {tabs.map(t => <button key={t.id} type="button" role="tab" aria-selected={tab === t.id} className={cx('meta-tab', tab === t.id && 'is-on')} onClick={() => setTab(t.id)}><Icon name={t.icon} size={16} />{t.label}</button>)}
          </nav>
          <div className="meta-body">
            {tab === 'general' ? <General items={items.data!} bulk={bulk} onDone={() => onOpenChange(false)} /> : null}
            {tab === 'artwork' ? <Artwork item={first} /> : null}
            {tab === 'tags' ? <Tags items={items.data!} /> : null}
            {tab === 'people' ? <People item={first} /> : null}
            {tab === 'matching' ? <Matching item={first} /> : null}
            {tab === 'history' ? <History item={first} /> : null}
          </div>
        </div>
      )}
    </Dialog>
  );
}

function General({items, bulk, onDone}: {items: Item[]; bulk: boolean; onDone: () => void}) {
  const it = items[0] as Movie;
  const mixed = (get: (i: Item) => unknown) => bulk && new Set(items.map(i => JSON.stringify(get(i)))).size > 1;
  const fields = [
    {key: 'title', label: 'Title', value: it.title, mixed: mixed(i => i.title), locked: true},
    {key: 'sortTitle', label: 'Sort title', value: it.sortTitle, mixed: mixed(i => i.sortTitle), locked: false},
    {key: 'year', label: 'Year', value: String(it.year ?? ''), mixed: mixed(i => i.year), locked: false},
    {key: 'contentRating', label: 'Content rating', value: it.contentRating ?? '', mixed: mixed(i => (i as Movie).contentRating), locked: false},
    {key: 'studio', label: 'Studio', value: it.studio ?? '', mixed: mixed(i => (i as Movie).studio), locked: false},
    {key: 'tagline', label: 'Tagline', value: it.tagline ?? '', mixed: mixed(i => (i as Movie).tagline), locked: false},
  ].filter(f => !bulk || ['year', 'contentRating', 'studio'].includes(f.key));
  const [values, setValues] = useState<Record<string, string>>(Object.fromEntries(fields.map(f => [f.key, f.mixed ? '' : f.value])));
  const [locks, setLocks] = useState<Record<string, boolean>>(Object.fromEntries(fields.map(f => [f.key, f.locked])));
  const [summary, setSummary] = useState(it.synopsis ?? '');
  const [lockEdited, setLockEdited] = useState(true);
  const [error, setError] = useState('');
  const save = () => {
    if (!bulk && !values.title?.trim()) return setError('A title is required.');
    if (values.year && !/^\d{4}$/.test(values.year)) return setError('Enter the year as four digits.');
    toast(bulk ? `Updated ${plural(items.length, 'item')}` : 'Saved. Locked fields keep your edits when metadata refreshes.', {icon: 'check'});
    onDone();
  };
  return (
    <div className="stack">
      {error ? <Notice tone="bad">{error}</Notice> : null}
      <div className="meta-grid">
        {fields.map(f => (
          <div key={f.key} className="meta-field">
            <Field label={<>{f.label}{locks[f.key] ? <span className="meta-locked"><Icon name="lock" size={11} /> Locked</span> : null}</>}>
              <Input value={values[f.key] ?? ''} placeholder={f.mixed ? 'Mixed · leave unchanged' : undefined} onChange={e => { setValues({...values, [f.key]: e.target.value}); if (lockEdited) setLocks({...locks, [f.key]: true}); }} />
            </Field>
            <button type="button" className={cx('lock-btn', locks[f.key] && 'is-on')} onClick={() => setLocks({...locks, [f.key]: !locks[f.key]})} aria-pressed={locks[f.key]} aria-label={locks[f.key] ? `Unlock ${f.label}` : `Lock ${f.label}`} title={locks[f.key] ? 'Locked: refreshes keep this value. Unlock to reset to the automatic value on save.' : 'Unlocked: refreshes can change this value.'}><Icon name="lock" size={15} /></button>
          </div>
        ))}
      </div>
      {!bulk ? <Field label="Summary"><TextArea value={summary} onChange={e => setSummary(e.target.value)} rows={5} maxLength={4000} /></Field> : null}
      <Checkbox checked={lockEdited} onChange={setLockEdited} label="Lock edited fields so refreshes keep them" />
      <div className="meta-foot"><DialogClose asChild><Button variant="ghost" label="Cancel" /></DialogClose><Button variant="primary" label={bulk ? `Apply to ${plural(items.length, 'item')}` : 'Save changes'} onClick={save} /></div>
    </div>
  );
}

function Artwork({item}: {item: Item}) {
  const [kind, setKind] = useState<'poster' | 'backdrop'>('poster');
  const pool = [...artKeys].filter(k => k.startsWith(kind === 'poster' ? 'poster/' : 'backdrop/'));
  const own = kind === 'poster' ? item.art.poster ?? item.art.square : item.art.backdrop;
  const start = Math.abs([...item.id].reduce((a, c) => a * 31 + c.charCodeAt(0), 7)) % Math.max(1, pool.length - 8);
  const choices = [own, ...pool.slice(start, start + 7)].filter((x): x is string => !!x);
  const [chosen, setChosen] = useState(own);
  return (
    <div className="stack">
      <div className="hstack"><button type="button" className={cx('chip', kind === 'poster' && 'is-on')} onClick={() => setKind('poster')}>Poster</button><button type="button" className={cx('chip', kind === 'backdrop' && 'is-on')} onClick={() => setKind('backdrop')}>Background</button><span style={{flex: 1}} /><label className="btn btn-secondary btn-sm"><Icon name="upload" size={15} /><span className="btn-label">Upload…</span><input type="file" accept="image/*" className="sr-only" onChange={e => e.target.files?.[0] && toast(`Uploaded ${e.target.files[0].name}`)} /></label></div>
      <div className={cx('art-choices', kind === 'backdrop' && 'is-wide')}>
        {choices.map((c, i) => (
          <button key={c + i} type="button" className={cx('art-choice', chosen === c && 'is-on')} onClick={() => setChosen(c)} aria-pressed={chosen === c}>
            <Art src={c} ratio={kind === 'poster' ? '2/3' : '16/9'} />
            <span className="art-choice-label">{i === 0 ? 'In use' : i < 4 ? 'TMDB' : 'Fanart'}</span>
          </button>
        ))}
      </div>
      <div className="meta-foot"><Button variant="primary" label="Use this artwork" disabled={chosen === own} onClick={() => toast('Artwork updated and locked')} /></div>
    </div>
  );
}

function Tags({items}: {items: Item[]}) {
  const [genres, setGenres] = useState<string[]>(items[0]!.genres);
  const [text, setText] = useState('');
  const [labels, setLabels] = useState<string[]>(['4K remaster']);
  return (
    <div className="stack">
      <Field label="Genres" help="Press Enter to add.">
        <Input value={text} onChange={e => setText(e.target.value)} placeholder="Add a genre" onKeyDown={e => { if (e.key === 'Enter' && text.trim()) { e.preventDefault(); setGenres([...new Set([...genres, text.trim()])]); setText(''); } }} />
      </Field>
      <div className="chip-row">{genres.map(g => <button key={g} type="button" className="chip is-on" onClick={() => setGenres(genres.filter(x => x !== g))} aria-label={`Remove ${g}`}>{g}<Icon name="close" size={13} /></button>)}{!genres.length ? <span className="faint small">No genres.</span> : null}</div>
      <div className="field"><span className="field-label">Labels</span><div className="chip-row">{labels.map(l => <button key={l} type="button" className="chip" onClick={() => setLabels(labels.filter(x => x !== l))}>{l}<Icon name="close" size={13} /></button>)}<button type="button" className="chip" onClick={() => setLabels([...labels, 'Family favorite'])}><Icon name="plus" size={13} />Add label</button></div><span className="field-help">Labels can hide titles from limited profiles (Settings › Profiles › Limits).</span></div>
      <div className="meta-foot"><Button variant="primary" label={items.length > 1 ? `Apply to ${plural(items.length, 'item')}` : 'Save genres'} onClick={() => toast('Genres and labels saved')} /></div>
    </div>
  );
}

function People({item}: {item: Item}) {
  const credits = 'credits' in item ? (item as Movie).credits : [];
  const [list, setList] = useState(credits);
  const [name, setName] = useState('');
  const [role, setRole] = useState('');
  const [dept, setDept] = useState<'Acting' | 'Directing' | 'Writing' | 'Production' | 'Sound'>('Acting');
  return (
    <div className="stack">
      <ul className="credit-edit">
        {list.map((c, i) => (
          <li key={c.personId + i}><Avatar name={c.name} size={32} /><span className="set-text"><strong>{c.name}</strong><span>{c.character ? `as ${c.character}` : c.job}</span></span><Badge>{c.department}</Badge><button type="button" className="track-more is-shown" onClick={() => setList(list.filter((_, j) => j !== i))} aria-label={`Remove ${c.name}`}><Icon name="close" size={16} /></button></li>
        ))}
        {!list.length ? <li className="faint">No credits yet.</li> : null}
      </ul>
      <div className="add-credit">
        <Input value={name} onChange={e => setName(e.target.value)} placeholder="Name" aria-label="Person" />
        <Input value={role} onChange={e => setRole(e.target.value)} placeholder={dept === 'Acting' ? 'Character' : 'Job'} aria-label="Role" />
        <Select label="Department" value={dept} onChange={setDept} options={['Acting', 'Directing', 'Writing', 'Production', 'Sound'].map(d => ({value: d as typeof dept, label: d}))} />
        <Button variant="secondary" icon="plus" label="Add" disabled={!name.trim()} onClick={() => { setList([...list, {personId: 'new-' + name, name, character: dept === 'Acting' ? role : undefined, job: dept === 'Acting' ? undefined : role, department: dept}]); setName(''); setRole(''); }} />
      </div>
      <div className="meta-foot"><Button variant="primary" label="Save cast & crew" onClick={() => toast('Credits saved and locked')} /></div>
    </div>
  );
}

function Matching({item}: {item: Item}) {
  const [q, setQ] = useState(item.title);
  const [year, setYear] = useState(String(item.year ?? ''));
  const [searching, setSearching] = useState(false);
  const [results, setResults] = useState<{id: string; title: string; year: number; score: number; overview: string; art?: string}[] | null>(null);
  const [using, setUsing] = useState<string | null>(null);
  const run = () => {
    setSearching(true);
    setTimeout(() => {
      setSearching(false);
      const y = Number(year) || item.year || 2000;
      setResults([
        {id: 'tmdb-1', title: q, year: y, score: 96, overview: item.synopsis?.slice(0, 120) ?? '', art: item.art.poster},
        {id: 'tmdb-2', title: q, year: y - 21, score: 71, overview: 'An earlier film with the same name, about a stranger who arrives in town with a secret.', art: [...artKeys].find(k => k.startsWith('poster/m-') && k !== item.art.poster)},
        {id: 'tmdb-3', title: `${q}: Director’s Cut`, year: y + 2, score: 58, overview: 'An extended cut released on home video.'},
      ]);
    }, 900);
  };
  useEffect(() => { run(); }, []); // eslint-disable-line react-hooks/exhaustive-deps
  return (
    <div className="stack">
      <div className="current-match"><span className="eyebrow">Current match</span><strong>{item.title} ({item.year}) · TMDB</strong><span className="faint small">Matched automatically from the file name</span></div>
      <form className="match-search" onSubmit={e => { e.preventDefault(); run(); }}>
        <Input value={q} onChange={e => setQ(e.target.value)} aria-label="Title" placeholder="Title" />
        <Input value={year} onChange={e => setYear(e.target.value)} aria-label="Year" placeholder="Year" inputMode="numeric" style={{width: 100}} />
        <Select label="Provider" value="tmdb" onChange={() => undefined} options={[{value: 'tmdb', label: 'TMDB'}, {value: 'tvdb', label: 'TheTVDB'}]} />
        <Button variant="secondary" icon="search" label="Search" type="submit" />
      </form>
      {searching ? <div className="landing" style={{padding: 24}}><Spinner /></div> : results ? (
        <ul className="candidates">
          {results.map(r => (
            <li key={r.id} className="candidate">
              <Art src={r.art} ratio="2/3" className="cand-art" fallbackIcon="film" />
              <div className="set-text"><strong>{r.title} ({r.year})</strong><span className="clamp-2">{r.overview || 'No description.'}</span><span className="faint small">TMDB · {r.score}% match</span></div>
              <Button size="sm" variant={r.score > 90 ? 'primary' : 'secondary'} label={using === r.id ? 'Using…' : 'Use this match'} busy={using === r.id} onClick={() => { setUsing(r.id); setTimeout(() => { setUsing(null); toast(`Matched to ${r.title} (${r.year}). Details and artwork are updating.`); }, 900); }} />
            </li>
          ))}
        </ul>
      ) : null}
      <p className="faint small">Can’t find it? Try the original title, or check the year in the file name.</p>
    </div>
  );
}

function History({item}: {item: Item}) {
  const now = Date.now();
  const changes = [
    {at: now - 2 * 86400000, who: 'Justin', what: 'Changed the poster', detail: 'Chose TMDB poster 2 of 8'},
    {at: now - 9 * 86400000, who: 'Metadata refresh', what: 'Updated the summary', detail: 'From TMDB'},
    {at: now - 20 * 86400000, who: 'Maya', what: `Changed the title to “${item.title}”`, detail: 'Locked'},
    {at: now - 41 * 86400000, who: 'Scanner', what: 'Matched automatically', detail: `TMDB · ${item.title} (${item.year})`},
  ];
  return (
    <div className="stack">
      <h3 className="eyebrow">Recent changes</h3>
      <ol className="history-edit">
        {changes.map((c, i) => <li key={i}><span className="note-icon"><Icon name="history" size={15} /></span><span className="set-text"><strong>{c.what}</strong><span>{c.who} · {relativeTime(c.at)} · {c.detail}</span></span>{i < changes.length - 1 ? <Button size="sm" variant="ghost" label="Undo to here" onClick={() => toast('Restored to how it was before this change')} /> : null}</li>)}
      </ol>
    </div>
  );
}
