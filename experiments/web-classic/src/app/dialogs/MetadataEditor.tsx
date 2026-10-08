import {useEffect, useState} from 'react';
import {useClient, useQuery} from '@/data/DataProvider';
import type {Entry} from '@/data/types';
import {Dialog, useToast} from '@/ui/Overlay';
import {Button, IconButton} from '@/ui/Button';
import {Switch, TextField} from '@/ui/Form';
import {Artwork, Badge, Chip, EmptyState, Skeleton, Spinner} from '@/ui/Bits';
import {TabPanel, Tabs} from '@/ui/Tabs';
import {Icon} from '@/ui/Icon';
import {ago, plural} from '@/app/format';
import s from './dialogs.module.css';

type Tab = 'general' | 'artwork' | 'tags' | 'people' | 'match' | 'history';
const FIELDS: {key: string; label: string; multiline?: boolean; full?: boolean}[] = [
  {key: 'title', label: 'Title', full: true}, {key: 'sortTitle', label: 'Sort title'}, {key: 'originalTitle', label: 'Original title'},
  {key: 'year', label: 'Year'}, {key: 'releaseDate', label: 'Release date'}, {key: 'contentRating', label: 'Content rating'}, {key: 'studio', label: 'Studio or network'},
  {key: 'tagline', label: 'Tagline', full: true}, {key: 'summary', label: 'Summary', multiline: true, full: true},
];

/**
 * Edit details for one title, or several at once. Edited fields are locked
 * so a metadata refresh keeps them; each field's lock can be toggled.
 */
export default function MetadataEditor({entries, initialTab, onClose}: {entries: Entry[]; initialTab?: string; onClose: () => void}) {
  const client = useClient();
  const toast = useToast();
  const one = entries.length === 1 ? entries[0]! : undefined;
  const [tab, setTab] = useState<Tab>((initialTab as Tab) ?? 'general');
  const [values, setValues] = useState<Record<string, string>>({});
  const [edited, setEdited] = useState<Set<string>>(new Set());
  const [locked, setLocked] = useState<Set<string>>(new Set(['title']));
  const [genres, setGenres] = useState<string[]>([]);
  const [tagDraft, setTagDraft] = useState('');
  const [poster, setPoster] = useState(0);
  const [background, setBackground] = useState(0);
  const [query, setQuery] = useState('');
  const [search, setSearch] = useState('');
  const [busy, setBusy] = useState(false);
  const meta = useQuery(c => (one ? c.catalog.metadata(one.id) : Promise.resolve({} as Record<string, string>)), [one?.id]);
  const art = useQuery(c => (one && tab === 'artwork' ? c.catalog.artworkCandidates(one.id) : Promise.resolve(undefined)), [one?.id, tab]);
  const candidates = useQuery(c => (one && tab === 'match' ? c.catalog.matchCandidates(one.id, search) : Promise.resolve([])), [one?.id, tab, search]);
  const history = useQuery(c => (one && tab === 'history' ? c.catalog.metadataHistory(one.id) : Promise.resolve([])), [one?.id, tab]);
  const detail = useQuery(c => (one && tab === 'people' && one.kind === 'movie' ? c.catalog.movie(one.id) : Promise.resolve(undefined)), [one?.id, tab]);
  useEffect(() => {
    if (!meta.data) return;
    setValues(meta.data);
    setGenres((meta.data.genres ?? '').split(',').map(x => x.trim()).filter(Boolean));
  }, [meta.data]);
  const set = (k: string, v: string) => { setValues(p => ({...p, [k]: v})); setEdited(p => new Set(p).add(k)); setLocked(p => new Set(p).add(k)); };
  const toggleLock = (k: string) => setLocked(p => { const n = new Set(p); if (n.has(k)) n.delete(k); else n.add(k); return n; });
  const save = async () => {
    setBusy(true);
    const fields: Record<string, string> = {};
    for (const k of edited) fields[k] = values[k] ?? '';
    if (tab === 'tags' || genres.join(', ') !== (meta.data?.genres ?? '')) fields.genres = genres.join(', ');
    await client.catalog.saveMetadata(entries.map(e => e.id), fields, true);
    toast.show(one ? `Saved changes to “${one.title}”` : `Updated ${plural(entries.length, 'item')}`, {icon: 'check'});
    onClose();
  };
  const tabs = one
    ? [{value: 'general' as const, label: 'General'}, {value: 'artwork' as const, label: 'Artwork'}, {value: 'tags' as const, label: 'Tags and genres'}, {value: 'people' as const, label: 'Cast and crew'}, {value: 'match' as const, label: 'Fix match'}, {value: 'history' as const, label: 'History'}]
    : [{value: 'general' as const, label: 'General'}, {value: 'tags' as const, label: 'Tags and genres'}];
  const footer = tab === 'match' || tab === 'history'
    ? <Button variant="ghost" label="Close" onClick={onClose} />
    : <><Button variant="ghost" label="Cancel" onClick={onClose} /><Button variant="primary" label={one ? 'Save changes' : `Apply to ${plural(entries.length, 'item')}`} busy={busy} onClick={() => void save()} /></>;
  return (
    <Dialog open onOpenChange={o => !o && onClose()} title={one ? `Edit ${one.title}` : `Edit ${plural(entries.length, 'item')}`} description={one ? 'Locked fields keep your value when details are refreshed.' : 'Only the fields you change are applied. Everything else stays as it is.'} width={760} footer={footer}>
      <Tabs label="Editor sections" value={tab} onChange={setTab} tabs={tabs}>
        <TabPanel value="general">
          {one && meta.loading && !meta.data ? <div className={s.fields}>{FIELDS.map(f => <Skeleton key={f.key} height={64} className={f.full ? s.full : undefined} />)}</div> : (
            <>
              {one ? (
                <div className={s.lockAll}>
                  <Button size="sm" variant="secondary" icon="lock" label="Lock all" onClick={() => setLocked(new Set(FIELDS.map(f => f.key)))} />
                  <Button size="sm" variant="ghost" icon="unlock" label="Unlock all" onClick={() => setLocked(new Set())} />
                  <span className="t-caption c-3">{plural(locked.size, 'field')} locked</span>
                </div>
              ) : null}
              <div className={s.fields}>
                {FIELDS.map(f => (
                  <div key={f.key} className={`${s.fieldRow} ${f.full ? s.full : ''}`}>
                    <TextField label={f.label} value={values[f.key] ?? ''} onChange={v => set(f.key, v)} multiline={f.multiline} placeholder={one ? undefined : 'Leave unchanged'} />
                    {one ? <IconButton icon={locked.has(f.key) ? 'lock' : 'unlock'} label={locked.has(f.key) ? `${f.label} is locked` : `${f.label} is unlocked`} variant="ghost" className={locked.has(f.key) ? s.lockOn : undefined} onClick={() => toggleLock(f.key)} style={f.multiline ? {alignSelf: 'flex-start', marginTop: 26} : undefined} /> : null}
                  </div>
                ))}
              </div>
            </>
          )}
        </TabPanel>
        {one ? (
          <TabPanel value="artwork">
            {art.loading || !art.data ? <Spinner /> : (
              <div className={s.stack}>
                <div className="t-label">Poster</div>
                <div className={s.artGrid}>{art.data.posters.map((p, i) => <button key={i} type="button" className={s.artPick} aria-pressed={poster === i} aria-label={`Poster ${i + 1}${i === 0 ? ' (current)' : ''}`} onClick={() => { setPoster(i); setEdited(e => new Set(e).add('poster')); }}><Artwork src={p} title={one.title} shape="poster" /></button>)}</div>
                <div className="t-label">Background</div>
                <div className={`${s.artGrid} ${s.artGridWide}`}>{art.data.backgrounds.map((p, i) => <button key={i} type="button" className={s.artPick} aria-pressed={background === i} aria-label={`Background ${i + 1}${i === 0 ? ' (current)' : ''}`} onClick={() => { setBackground(i); setEdited(e => new Set(e).add('background')); }}><Artwork src={p} title={one.title} shape="wide" /></button>)}</div>
                <div className={s.notice}><Icon name="info" size={18} />To use your own picture, put a file named poster.jpg or background.jpg next to the video file. It takes priority over these.</div>
              </div>
            )}
          </TabPanel>
        ) : null}
        <TabPanel value="tags">
          <div className={s.stack}>
            <form className={s.newRow} onSubmit={e => { e.preventDefault(); const t = tagDraft.trim(); if (t && !genres.includes(t)) setGenres([...genres, t]); setTagDraft(''); }}>
              <TextField label="Add a genre or tag" value={tagDraft} onChange={setTagDraft} placeholder="Film Noir" />
              <Button type="submit" variant="secondary" label="Add" disabled={!tagDraft.trim()} />
            </form>
            <div className={s.tags}>{genres.length ? genres.map(g => <Chip key={g} onRemove={() => setGenres(genres.filter(x => x !== g))} title={`Remove ${g}`}>{g}</Chip>) : <span className="t-caption c-3">No genres or tags yet.</span>}</div>
          </div>
        </TabPanel>
        {one ? (
          <TabPanel value="people">
            {one.kind !== 'movie' ? <EmptyState compact icon="user" title="Cast and crew come from the match" body="For shows and music, people are filled in from the metadata provider. Fix the match to change them." /> : !detail.data ? <Spinner /> : (
              <div className={s.list}>
                {[...detail.data.directors, ...detail.data.writers, ...detail.data.cast].map((c, i) => (
                  <div key={`${c.personId}-${i}`} className={s.row}>
                    <Artwork src={c.image} title={c.name} shape="circle" className={s.thumb} />
                    <span className={s.rowText}><span className={s.rowTitle}>{c.name}</span><span className={s.rowSub}>{c.role === 'cast' ? c.character ?? 'Cast' : c.job}</span></span>
                    <IconButton icon="close" label={`Remove ${c.name}`} variant="ghost" size="sm" onClick={() => toast.show(`Removed ${c.name}. Save to keep the change.`)} />
                  </div>
                ))}
              </div>
            )}
          </TabPanel>
        ) : null}
        {one ? (
          <TabPanel value="match">
            <div className={s.stack}>
              <div className={s.notice}><Icon name="info" size={18} />Currently matched with TMDB. Choose the right title to replace the details and artwork. Locked fields keep your values.</div>
              <form className={s.newRow} onSubmit={e => { e.preventDefault(); setSearch(query); }}>
                <TextField label="Search by title" icon="search" value={query} onChange={setQuery} placeholder={one.title} />
                <Button type="submit" variant="secondary" label="Search" />
              </form>
              {candidates.loading ? <Spinner label="Searching" /> : candidates.data?.length ? candidates.data.map(c => (
                <div key={c.id} className={s.candidate}>
                  <Artwork src={c.image} title={c.title} shape="poster" style={{width: 48}} />
                  <span className={s.rowText}><span className={s.rowTitle}>{c.title} ({c.year})</span><span className={s.rowSub}>{c.provider} · <span className={s.score}>{c.score}% match</span></span></span>
                  <Button size="sm" variant="secondary" label="Use this" onClick={async () => { await client.catalog.fixMatch(one.id, c.id); toast.show(`Matched “${one.title}” with ${c.title} (${c.year})`, {icon: 'check'}); onClose(); }} />
                </div>
              )) : <EmptyState compact icon="search" title="No matches" body="Try the original title, or add the year." />}
            </div>
          </TabPanel>
        ) : null}
        {one ? (
          <TabPanel value="history">
            {history.loading && !history.data?.length ? <Spinner /> : history.data?.map(h => (
              <div key={h.id} className={s.history}>
                <Icon name="history" size={18} className="c-3" />
                <span className={s.rowText}><span className={s.rowTitle}>{h.change}</span><span className={s.rowSub}>{h.who} · {ago(h.when)}</span></span>
                {h.who === 'Portico' ? <Badge>Automatic</Badge> : <Button size="sm" variant="ghost" icon="refresh" label="Undo" onClick={async () => { await client.catalog.undoMetadata(one.id, h.id); history.refresh(); toast.show('Change undone', {icon: 'check'}); }} />}
              </div>
            ))}
          </TabPanel>
        ) : null}
      </Tabs>
      {tab === 'general' && one ? <label style={{display: 'flex', alignItems: 'center', gap: 10, marginTop: 18}}><Switch checked={locked.size === FIELDS.length} onChange={v => setLocked(v ? new Set(FIELDS.map(f => f.key)) : new Set())} label="Lock every field" /><span className="t-caption c-2">Keep every field as it is on future refreshes</span></label> : null}
    </Dialog>
  );
}
