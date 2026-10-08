import {useEffect, useState} from 'react';
import {useQuery, useServices} from '../../data/hooks';
import type {FilterState, Library, LibraryKind, SortState} from '../../data/types';
import {Dialog, DialogClose} from '../../ui/Overlay';
import {Button} from '../../ui/Button';
import {Chip, Field, Input, SearchInput, Switch} from '../../ui/Controls';
import {Skeleton, toast} from '../../ui/Feedback';
import type {IconName} from '../../ui/Icon';

const VIDEO: LibraryKind[] = ['movies', 'shows', 'anime'];
const ALL: LibraryKind[] = ['movies', 'shows', 'anime', 'music', 'audiobooks', 'photos', 'homevideos'];

export const QUICK: {id: string; label: string; icon?: IconName; kinds: LibraryKind[]}[] = [
  {id: 'unwatched', label: 'Unwatched', kinds: [...VIDEO, 'homevideos']},
  {id: 'inProgress', label: 'In progress', kinds: [...VIDEO, 'audiobooks', 'homevideos']},
  {id: 'watched', label: 'Watched', kinds: VIDEO},
  {id: 'recentlyAdded', label: 'Recently added', kinds: ALL},
  {id: 'favorites', label: 'Favorites', icon: 'heart', kinds: ALL},
  {id: 'watchlist', label: 'Watchlist', icon: 'bookmark', kinds: [...VIDEO, 'audiobooks']},
  {id: 'uhd', label: '4K', kinds: ['movies']},
  {id: 'hd', label: 'HD', kinds: VIDEO},
];

export const SORTS: {id: string; label: string; kinds: LibraryKind[]}[] = [
  {id: 'added', label: 'Date added', kinds: ALL},
  {id: 'title', label: 'Title', kinds: ALL},
  {id: 'year', label: 'Release year', kinds: ALL},
  {id: 'rating', label: 'Critic rating', kinds: ['movies']},
  {id: 'runtime', label: 'Length', kinds: ['movies']},
  {id: 'played', label: 'Last played', kinds: ALL},
  {id: 'random', label: 'Random', kinds: ALL},
];

const FIELDS: {id: string; label: string; kinds: LibraryKind[]}[] = [
  {id: 'genre', label: 'Genre', kinds: ALL},
  {id: 'decade', label: 'Decade', kinds: ALL},
  {id: 'contentRating', label: 'Content rating', kinds: VIDEO},
  {id: 'mood', label: 'Mood', kinds: ['movies']},
  {id: 'resolution', label: 'Resolution', kinds: VIDEO},
  {id: 'language', label: 'Audio language', kinds: ['movies']},
  {id: 'studio', label: 'Studio', kinds: ['movies']},
  {id: 'network', label: 'Network', kinds: ['shows', 'anime']},
];

export const describeField = (f: string, v: string) => (f === 'duration' ? (v === 'short' ? 'Under 90 minutes' : 'Over 2 hr 20 min') : f === 'decade' ? `The ${v}` : v);

/** The full filter set: every field with counts, a find box for long lists, year range and length. */
export function FiltersSheet({open, onOpenChange, lib, filters, onApply}: {open: boolean; onOpenChange: (o: boolean) => void; lib: Library; filters: FilterState; onApply: (f: FilterState) => void}) {
  const services = useServices();
  const facets = useQuery(open ? `facets.${lib.id}` : null, () => services.catalog.facets(lib.id));
  const [draft, setDraft] = useState<FilterState>(filters);
  const [find, setFind] = useState<Record<string, string>>({});
  useEffect(() => { if (open) setDraft(filters); }, [open, filters]);
  const toggle = (f: string, v: string) => {
    const cur = draft.fields[f] ?? [];
    setDraft({...draft, fields: {...draft.fields, [f]: cur.includes(v) ? cur.filter(x => x !== v) : [...cur, v]}});
  };
  const fields = FIELDS.filter(f => f.kinds.includes(lib.kind));
  const count = draft.quick.length + Object.values(draft.fields).reduce((a, v) => a + v.length, 0) + (draft.yearFrom || draft.yearTo ? 1 : 0);
  return (
    <Dialog open={open} onOpenChange={onOpenChange} kind="sheet" size="md" title="Filters" description={`Narrow ${lib.name} by anything Portico knows about it.`} footer={
      <>
        <Button variant="ghost" label="Clear all" onClick={() => setDraft({quick: [], fields: {}})} disabled={!count} />
        <span style={{flex: 1}} />
        <DialogClose asChild><Button variant="ghost" label="Cancel" /></DialogClose>
        <Button variant="primary" label={count ? `Show results · ${count} ${count === 1 ? 'filter' : 'filters'}` : 'Show results'} onClick={() => { onApply(draft); onOpenChange(false); }} />
      </>
    }>
      <div className="filter-sheet">
        <section>
          <h3 className="eyebrow">Quick filters</h3>
          <div className="chip-row">
            {QUICK.filter(q => q.kinds.includes(lib.kind)).map(q => <Chip key={q.id} on={draft.quick.includes(q.id)} onClick={() => setDraft({...draft, quick: draft.quick.includes(q.id) ? draft.quick.filter(x => x !== q.id) : [...draft.quick, q.id]})}>{q.label}</Chip>)}
          </div>
        </section>
        {fields.map(f => {
          const values = facets.data?.[f.id] ?? [];
          if (facets.data && !values.length) return null;
          const q = (find[f.id] ?? '').toLowerCase();
          const shown = values.filter(v => !q || v.value.toLowerCase().includes(q));
          return (
            <section key={f.id}>
              <div className="filter-head"><h3 className="eyebrow">{f.label}</h3>{(draft.fields[f.id] ?? []).length ? <button type="button" className="link-btn" onClick={() => setDraft({...draft, fields: {...draft.fields, [f.id]: []}})}>Clear</button> : null}</div>
              {values.length > 12 ? <SearchInput label={`Find a ${f.label.toLowerCase()}`} placeholder={`Find a ${f.label.toLowerCase()}`} value={find[f.id] ?? ''} onChange={v => setFind({...find, [f.id]: v})} className="filter-find" /> : null}
              <div className="chip-row">
                {!facets.data ? [0, 1, 2, 3, 4].map(i => <Skeleton key={i} w={80} h={34} r={17} />) : null}
                {shown.slice(0, q ? 40 : 18).map(v => <Chip key={v.value} on={(draft.fields[f.id] ?? []).includes(v.value)} onClick={() => toggle(f.id, v.value)}>{describeField(f.id, v.value)} <span className="chip-count">{v.count}</span></Chip>)}
                {facets.data && !shown.length ? <span className="faint small">No {f.label.toLowerCase()} matches “{find[f.id]}”.</span> : null}
              </div>
            </section>
          );
        })}
        {lib.kind === 'movies' ? (
          <section>
            <h3 className="eyebrow">Length</h3>
            <div className="chip-row">
              {['short', 'long'].map(v => <Chip key={v} on={(draft.fields.duration ?? []).includes(v)} onClick={() => toggle('duration', v)}>{describeField('duration', v)}</Chip>)}
            </div>
          </section>
        ) : null}
        <section>
          <h3 className="eyebrow">Released</h3>
          <div className="year-range">
            <Field label="From"><Input inputMode="numeric" placeholder="1920" value={draft.yearFrom ?? ''} onChange={e => setDraft({...draft, yearFrom: e.target.value ? Number(e.target.value.replace(/\D/g, '')) || undefined : undefined})} maxLength={4} /></Field>
            <span className="faint">to</span>
            <Field label="To"><Input inputMode="numeric" placeholder={String(new Date().getFullYear())} value={draft.yearTo ?? ''} onChange={e => setDraft({...draft, yearTo: e.target.value ? Number(e.target.value.replace(/\D/g, '')) || undefined : undefined})} maxLength={4} /></Field>
          </div>
        </section>
      </div>
    </Dialog>
  );
}

/** Save the current filters and sort as a named view (optionally on Home, optionally shared). */
export function SaveViewDialog({open, onOpenChange, lib, filters, sort}: {open: boolean; onOpenChange: (o: boolean) => void; lib: Library; filters: FilterState; sort: SortState}) {
  const services = useServices();
  const [title, setTitle] = useState('');
  const [onHome, setOnHome] = useState(true);
  const [shared, setShared] = useState(false);
  const [busy, setBusy] = useState(false);
  useEffect(() => { if (open) setTitle(''); }, [open]);
  const save = async () => {
    if (!title.trim()) return;
    setBusy(true);
    await services.lists.saveView({title: title.trim(), libraryId: lib.id, filters, sort, shared, onHome});
    setBusy(false);
    onOpenChange(false);
    toast(`Saved “${title.trim()}”${onHome ? ' and added it to Home' : ''}`, {icon: 'save'});
  };
  return (
    <Dialog open={open} onOpenChange={onOpenChange} title="Save this view" description="Keep these filters and this order under a name. Saved views live in Saved, and can be a row on Home." size="sm" footer={
      <>
        <DialogClose asChild><Button variant="ghost" label="Cancel" /></DialogClose>
        <Button variant="primary" label="Save view" disabled={!title.trim()} busy={busy} onClick={() => void save()} />
      </>
    }>
      <form onSubmit={e => { e.preventDefault(); void save(); }} className="stack">
        <Field label="Name"><Input value={title} onChange={e => setTitle(e.target.value)} placeholder="Unwatched in 4K" autoFocus maxLength={60} /></Field>
        <label className="row" style={{padding: 0, border: 0, minHeight: 44}}><span className="row-text"><span className="row-label">Show on Home</span><span className="row-help">As a row you can move in Customize Home.</span></span><Switch checked={onHome} onChange={setOnHome} label="Show on Home" /></label>
        <label className="row" style={{padding: 0, border: 0, minHeight: 44}}><span className="row-text"><span className="row-label">Share with everyone on this server</span><span className="row-help">Other profiles see it in their Saved.</span></span><Switch checked={shared} onChange={setShared} label="Share with everyone" /></label>
      </form>
    </Dialog>
  );
}


/** A saved view in words: "Unwatched, 4K · sorted by date added". */
export function describeView(v: {filters: FilterState; sort: SortState}) {
  const parts = [...v.filters.quick.map(q => QUICK.find(x => x.id === q)?.label ?? q), ...Object.entries(v.filters.fields).flatMap(([f, vals]) => vals.map(x => describeField(f, x)))];
  const sort = SORTS.find(s => s.id === v.sort.field)?.label.toLowerCase() ?? v.sort.field;
  return {filters: parts.length ? parts.join(', ') : 'All titles', sort: `Sorted by ${sort}`};
}
